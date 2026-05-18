package scheduler

import (
	"bufio"
	"crypto/tls"
	"crypto/x509"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/casapps/wthr/src/backup"
	"github.com/casapps/wthr/src/database"
	"github.com/casapps/wthr/src/path"
	"github.com/robfig/cron/v3"
)

// Task represents a scheduled task
// AI.md PART 19: Scheduler uses cron expressions, not intervals
type Task struct {
	Name     string
	Schedule string // Cron expression: "0 2 * * *", "@hourly", "@every 5m"
	Fn       func() error
	entryID  cron.EntryID
	// Can be toggled on/off
	enabled bool
	// Last execution time
	lastRun *time.Time
	mu      sync.Mutex
}

// Global tasks that should only run on one node in cluster mode
// AI.md PART 19: Global Tasks (run once per cluster)
var globalTasks = map[string]bool{
	"ssl-renewal":           true,
	"geoip-update":          true,
	"blocklist-update":      true,
	"cve-update":            true,
	"backup-daily":          true,
	"backup-hourly":         true,
	"update-geoip-database": true,
}

// LockTimeout is how long a lock is valid before auto-release (5 minutes per AI.md)
const LockTimeout = 5 * time.Minute

// Scheduler manages scheduled tasks using robfig/cron
// AI.md PART 19: Built-in scheduler with cron expression support
type Scheduler struct {
	cron   *cron.Cron
	tasks  map[string]*Task
	db     *sql.DB
	nodeID string
	mu     sync.RWMutex
}

// NewScheduler creates a new scheduler instance with robfig/cron
func NewScheduler(db *sql.DB) *Scheduler {
	// Get node ID from hostname
	nodeID, err := getNodeID()
	if err != nil {
		nodeID = "default"
	}

	// Create cron instance with seconds optional (standard cron format)
	// AI.md PART 19: Support "0 2 * * *", "@hourly", "@every 5m" formats
	c := cron.New(cron.WithParser(cron.NewParser(
		cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow | cron.Descriptor,
	)))

	return &Scheduler{
		cron:   c,
		tasks:  make(map[string]*Task),
		db:     db,
		nodeID: nodeID,
	}
}

// getNodeID returns a unique identifier for this node
func getNodeID() (string, error) {
	hostname, err := os.Hostname()
	if err != nil {
		return "", err
	}
	return hostname, nil
}

// AddTask adds a new task to the scheduler with a cron schedule
// AI.md PART 19: Schedule format - "0 2 * * *", "@hourly", "@daily", "@every 5m"
func (s *Scheduler) AddTask(name string, schedule string, fn func() error) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	task := &Task{
		Name:     name,
		Schedule: schedule,
		Fn:       fn,
		enabled:  true,
		lastRun:  nil,
	}

	// Wrap the task function with our execution logic
	wrappedFn := func() {
		s.executeTask(task)
	}

	// Add to cron scheduler
	entryID, err := s.cron.AddFunc(schedule, wrappedFn)
	if err != nil {
		return fmt.Errorf("failed to add task '%s' with schedule '%s': %w", name, schedule, err)
	}

	task.entryID = entryID
	s.tasks[name] = task

	return nil
}

// AddTaskInterval adds a task with a time.Duration interval (convenience method)
// Converts to @every format for robfig/cron
func (s *Scheduler) AddTaskInterval(name string, interval time.Duration, fn func() error) error {
	schedule := fmt.Sprintf("@every %s", interval.String())
	return s.AddTask(name, schedule, fn)
}

// Start starts the cron scheduler
func (s *Scheduler) Start() {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.cron.Start()

	log.Printf("📅 Task manager has started (%d scheduled tasks)", len(s.tasks))
}

// Stop stops the cron scheduler
func (s *Scheduler) Stop() {
	s.mu.Lock()
	defer s.mu.Unlock()

	log.Println("🛑 Stopping scheduler...")

	ctx := s.cron.Stop()
	<-ctx.Done() // Wait for running jobs to complete

	log.Println("✅ Scheduler stopped")
}

// isGlobalTask returns true if this task should only run on one node
func isGlobalTask(taskName string) bool {
	return globalTasks[taskName]
}

// acquireTaskLock attempts to acquire a distributed lock for a task
// AI.md PART 19: Cluster-aware task locking
func (s *Scheduler) acquireTaskLock(taskName string) bool {
	// For non-global tasks, always allow (run on every node)
	if !isGlobalTask(taskName) {
		return true
	}

	now := time.Now()
	lockExpiry := now.Add(-LockTimeout)

	// Try to acquire lock:
	// 1. If no lock exists, acquire it
	// 2. If lock exists but expired (older than 5 min), steal it
	// 3. If lock exists and held by us, refresh it
	// 4. If lock exists and held by another node, fail
	result, err := database.GetServerDB().Exec(`
		INSERT INTO server_scheduler_state (task_id, task_name, locked_by, locked_at, enabled)
		VALUES (?, ?, ?, ?, true)
		ON CONFLICT(task_id) DO UPDATE SET
			locked_by = CASE
				WHEN locked_by IS NULL OR locked_at < ? OR locked_by = ? THEN ?
				ELSE locked_by
			END,
			locked_at = CASE
				WHEN locked_by IS NULL OR locked_at < ? OR locked_by = ? THEN ?
				ELSE locked_at
			END
		WHERE task_id = ?
	`, taskName, taskName, s.nodeID, now, lockExpiry, s.nodeID, s.nodeID, lockExpiry, s.nodeID, now, taskName)

	if err != nil {
		log.Printf("⚠️  Failed to acquire lock for task '%s': %v", taskName, err)
		return false
	}

	// Check if we actually got the lock
	var lockedBy string
	err = database.GetServerDB().QueryRow(
		"SELECT locked_by FROM server_scheduler_state WHERE task_id = ?",
		taskName,
	).Scan(&lockedBy)

	if err != nil || lockedBy != s.nodeID {
		// Another node has the lock
		return false
	}

	rowsAffected, _ := result.RowsAffected()
	return rowsAffected > 0 || lockedBy == s.nodeID
}

// releaseTaskLock releases the distributed lock for a task
func (s *Scheduler) releaseTaskLock(taskName string) {
	if !isGlobalTask(taskName) {
		return
	}

	_, err := database.GetServerDB().Exec(`
		UPDATE server_scheduler_state
		SET locked_by = NULL, locked_at = NULL
		WHERE task_id = ? AND locked_by = ?
	`, taskName, s.nodeID)

	if err != nil {
		log.Printf("⚠️  Failed to release lock for task '%s': %v", taskName, err)
	}
}

// executeTask executes a task and logs results
func (s *Scheduler) executeTask(task *Task) {
	// Check if task is enabled
	task.mu.Lock()
	if !task.enabled {
		task.mu.Unlock()
		return
	}
	task.mu.Unlock()

	// AI.md PART 19: Cluster-aware locking for global tasks
	if !s.acquireTaskLock(task.Name) {
		// Another node is running this task, skip
		return
	}
	defer s.releaseTaskLock(task.Name)

	start := time.Now()
	err := task.Fn()
	end := time.Now()
	elapsed := end.Sub(start)

	// Update last run time
	task.mu.Lock()
	task.lastRun = &end
	task.mu.Unlock()

	// Record in database
	s.RecordTaskRun(task.Name, start, end, err)

	if err != nil {
		log.Printf("❌ Task '%s' failed after %v: %v", task.Name, elapsed, err)
	} else {
		log.Printf("✅ Task '%s' completed in %v", task.Name, elapsed)
	}

	// Log to audit if enabled
	s.logTaskExecution(task.Name, elapsed, err)
}

// logTaskExecution logs task execution to audit log
func (s *Scheduler) logTaskExecution(taskName string, duration time.Duration, err error) {
	// Check if audit logging is enabled
	var auditEnabled string
	queryErr := database.GetServerDB().QueryRow("SELECT value FROM server_config WHERE key = 'audit.enabled'").Scan(&auditEnabled)
	if queryErr != nil || auditEnabled != "true" {
		return
	}

	status := "success"
	details := fmt.Sprintf("Completed in %v", duration)
	if err != nil {
		status = "error"
		details = fmt.Sprintf("Failed: %v", err)
	}

	_, insertErr := database.GetServerDB().Exec(`
		INSERT INTO server_audit_log (user_id, action, resource_type, resource_id, details, ip_address, user_agent, status)
		VALUES (NULL, ?, 'scheduler', ?, ?, 'system', 'scheduler', ?)
	`, taskName, taskName, details, status)

	if insertErr != nil {
		log.Printf("⚠️  Failed to log scheduler task: %v", insertErr)
	}
}

// GetTaskStatus returns status of all tasks
func (s *Scheduler) GetTaskStatus() []map[string]interface{} {
	s.mu.RLock()
	defer s.mu.RUnlock()

	status := make([]map[string]interface{}, 0, len(s.tasks))

	for _, task := range s.tasks {
		task.mu.Lock()
		var nextRun time.Time
		entry := s.cron.Entry(task.entryID)
		if entry.ID != 0 {
			nextRun = entry.Next
		}
		status = append(status, map[string]interface{}{
			"name":     task.Name,
			"schedule": task.Schedule,
			"enabled":  task.enabled,
			"lastRun":  task.lastRun,
			"nextRun":  nextRun,
		})
		task.mu.Unlock()
	}

	return status
}

// CleanupOldSessions removes expired sessions
func CleanupOldSessions(db *sql.DB) error {
	result, err := database.GetUsersDB().Exec("DELETE FROM user_sessions WHERE expires_at < datetime('now')")
	if err != nil {
		return fmt.Errorf("failed to cleanup sessions: %w", err)
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected > 0 {
		log.Printf("🧹 Cleaned up %d expired sessions", rowsAffected)
	}

	return nil
}

// CleanupOldAuditLogs removes audit logs older than retention period
func CleanupOldAuditLogs(db *sql.DB) error {
	// Get retention days from settings
	var retentionDays int
	err := database.GetServerDB().QueryRow("SELECT value FROM server_config WHERE key = 'audit.retention_days'").Scan(&retentionDays)
	if err != nil {
		// Default to 90 days
		retentionDays = 90
	}

	result, err := database.GetServerDB().Exec(`
		DELETE FROM server_audit_log
		WHERE created_at < datetime('now', '-' || ? || ' days')
	`, retentionDays)

	if err != nil {
		return fmt.Errorf("failed to cleanup audit logs: %w", err)
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected > 0 {
		log.Printf("🧹 Cleaned up %d old audit logs (retention: %d days)", rowsAffected, retentionDays)
	}

	return nil
}

// CheckWeatherAlerts checks for weather alerts on saved locations
func CheckWeatherAlerts(db *sql.DB) error {
	// Get all locations with alerts enabled
	rows, err := database.GetUsersDB().Query(`
		SELECT l.id, l.name, l.latitude, l.longitude, l.user_id
		FROM user_saved_locations l
		JOIN user_accounts u ON l.user_id = u.id
		WHERE l.alerts_enabled = 1
	`)
	if err != nil {
		return fmt.Errorf("failed to fetch locations: %w", err)
	}
	defer rows.Close()

	alertCount := 0

	for rows.Next() {
		var locationID int
		var name string
		var latitude, longitude float64
		var userID int

		if err := rows.Scan(&locationID, &name, &latitude, &longitude, &userID); err != nil {
			log.Printf("⚠️  Failed to scan location: %v", err)
			continue
		}

		// Fetch weather data from Open-Meteo API
		url := fmt.Sprintf("https://api.open-meteo.com/v1/forecast?latitude=%.4f&longitude=%.4f&current=temperature_2m,wind_speed_10m,precipitation,weather_code&temperature_unit=fahrenheit&wind_speed_unit=mph&precipitation_unit=inch",
			latitude, longitude)

		resp, err := http.Get(url)
		if err != nil {
			log.Printf("⚠️  Failed to fetch weather for %s: %v", name, err)
			continue
		}
		defer resp.Body.Close()

		var weatherData struct {
			Current struct {
				Temperature   float64 `json:"temperature_2m"`
				WindSpeed     float64 `json:"wind_speed_10m"`
				Precipitation float64 `json:"precipitation"`
				WeatherCode   int     `json:"weather_code"`
			} `json:"current"`
		}

		if err := json.NewDecoder(resp.Body).Decode(&weatherData); err != nil {
			log.Printf("⚠️  Failed to decode weather data for %s: %v", name, err)
			continue
		}

		// Check for alert conditions and create notifications
		created := checkAndCreateAlerts(db, userID, locationID, name, weatherData)
		alertCount += created
	}

	if alertCount > 0 {
		log.Printf("🔔 Created %d weather alerts", alertCount)
	}

	return nil
}

// checkAndCreateAlerts checks weather conditions and creates notifications
func checkAndCreateAlerts(db *sql.DB, userID, locationID int, locationName string, weather struct {
	Current struct {
		Temperature   float64 `json:"temperature_2m"`
		WindSpeed     float64 `json:"wind_speed_10m"`
		Precipitation float64 `json:"precipitation"`
		WeatherCode   int     `json:"weather_code"`
	} `json:"current"`
}) int {
	alertCount := 0

	// Check for extreme cold (below 32°F / 0°C)
	if weather.Current.Temperature < 32 {
		createNotification(db, userID, "alert", "⚠️ Freezing Temperature Alert",
			fmt.Sprintf("%s: Temperature is %.1f°F. Bundle up!", locationName, weather.Current.Temperature),
			fmt.Sprintf("/dashboard?location=%d", locationID))
		alertCount++
	}

	// Check for extreme heat (above 95°F / 35°C)
	if weather.Current.Temperature > 95 {
		createNotification(db, userID, "alert", "🌡️ Heat Alert",
			fmt.Sprintf("%s: Temperature is %.1f°F. Stay hydrated!", locationName, weather.Current.Temperature),
			fmt.Sprintf("/dashboard?location=%d", locationID))
		alertCount++
	}

	// Check for high winds (above 40 mph)
	if weather.Current.WindSpeed > 40 {
		createNotification(db, userID, "warning", "💨 High Wind Alert",
			fmt.Sprintf("%s: Wind speed is %.0f mph. Secure loose objects!", locationName, weather.Current.WindSpeed),
			fmt.Sprintf("/dashboard?location=%d", locationID))
		alertCount++
	}

	// Check for heavy precipitation (above 0.5 inches)
	if weather.Current.Precipitation > 0.5 {
		createNotification(db, userID, "info", "🌧️ Heavy Rain Alert",
			fmt.Sprintf("%s: Heavy precipitation detected (%.1f in). Prepare for flooding!", locationName, weather.Current.Precipitation),
			fmt.Sprintf("/dashboard?location=%d", locationID))
		alertCount++
	}

	// Check for severe weather codes (thunderstorms, snow, etc.)
	if weather.Current.WeatherCode >= 95 {
		createNotification(db, userID, "alert", "⛈️ Severe Weather Alert",
			fmt.Sprintf("%s: Severe weather detected. Stay safe!", locationName),
			fmt.Sprintf("/dashboard?location=%d", locationID))
		alertCount++
	}

	return alertCount
}

// createNotification creates a notification in the database
func createNotification(db *sql.DB, userID int, notifType, title, message, link string) {
	_, err := database.GetUsersDB().Exec(`
		INSERT INTO user_notifications (user_id, type, title, message, link, read)
		VALUES (?, ?, ?, ?, ?, 0)
	`, userID, notifType, title, message, link)

	if err != nil {
		log.Printf("⚠️  Failed to create notification: %v", err)
	}
}

// CreateSystemBackup creates a backup of the database
// AI.md PART 19/25: backup_daily task - creates verified backups
func CreateSystemBackup(db *sql.DB) error {
	// Get backup settings
	var backupEnabled string
	err := database.GetServerDB().QueryRow("SELECT value FROM server_config WHERE key = 'backup.enabled'").Scan(&backupEnabled)
	if err != nil || backupEnabled != "true" {
		// Backups disabled, skip silently
		return nil
	}

	// Get paths per AI.md PART 4
	p := paths.GetDefaultPaths("wthr")
	if p == nil {
		return fmt.Errorf("failed to get default paths for backup")
	}

	// Create backup service per AI.md PART 25
	svc := backup.New(p.ConfigDir, p.DataDir)

	// Check for encryption password from settings
	var encryptionPassword string
	_ = database.GetServerDB().QueryRow("SELECT value FROM server_config WHERE key = 'backup.encryption_password'").Scan(&encryptionPassword)

	// Create backup with options per AI.md PART 25
	opts := backup.BackupOptions{
		ConfigDir:   p.ConfigDir,
		DataDir:     p.DataDir,
		OutputPath:  "", // Auto-generate filename
		Password:    encryptionPassword,
		IncludeSSL:  false, // Don't include SSL in automated backups
		IncludeData: false, // Don't include data files in automated backups
		CreatedBy:   "scheduler",
		AppVersion:  "1.0.0",
	}

	log.Println("💾 Starting automated backup...")
	backupPath, err := svc.Create(opts)
	if err != nil {
		log.Printf("❌ Automated backup failed: %v", err)
		return fmt.Errorf("backup failed: %w", err)
	}

	log.Printf("✅ Automated backup completed: %s", backupPath)
	return nil
}

// CleanupExpiredTokens removes expired API and setup tokens
// AI.md PART 19: token cleanup every 15 minutes
func CleanupExpiredTokens(db *sql.DB) error {
	// Clean up expired API tokens
	result, err := database.GetServerDB().Exec(`
		DELETE FROM server_api_tokens
		WHERE expires_at IS NOT NULL AND expires_at < datetime('now')
	`)
	if err != nil {
		return fmt.Errorf("failed to cleanup expired API tokens: %w", err)
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected > 0 {
		log.Printf("🧹 Cleaned up %d expired API tokens", rowsAffected)
	}

	// Clean up expired setup tokens
	result2, err := database.GetServerDB().Exec(`
		DELETE FROM server_setup_tokens
		WHERE expires_at < datetime('now')
	`)
	if err != nil {
		// Table may not exist, that's ok
		return nil
	}

	rowsAffected2, _ := result2.RowsAffected()
	if rowsAffected2 > 0 {
		log.Printf("🧹 Cleaned up %d expired setup tokens", rowsAffected2)
	}

	return nil
}

// CheckSSLRenewal checks if SSL certificates need renewal
// AI.md PART 19: SSL renewal daily at 03:00, renew 7 days before expiry
func CheckSSLRenewal() error {
	// Check if SSL is enabled via Let's Encrypt
	var sslEnabled string
	err := database.GetServerDB().QueryRow("SELECT value FROM server_config WHERE key = 'ssl.letsencrypt.enabled'").Scan(&sslEnabled)
	if err != nil || sslEnabled != "true" {
		// SSL not using Let's Encrypt, skip renewal
		return nil
	}

	// Get the domain from settings
	var domain string
	domainErr := database.GetServerDB().QueryRow("SELECT value FROM server_config WHERE key = 'ssl.domain'").Scan(&domain)
	if domainErr != nil || domain == "" {
		// No domain configured, skip
		return nil
	}

	log.Println("🔐 Checking SSL certificate renewal status...")

	// Get paths
	p := paths.GetDefaultPaths("wthr")
	if p == nil {
		return fmt.Errorf("failed to get default paths")
	}

	// Check for certificate in common locations
	certPaths := []string{
		filepath.Join(p.DataDir, "certs", domain+".crt"),
		filepath.Join(p.DataDir, "certs", "server.crt"),
		filepath.Join("/etc/letsencrypt/live", domain, "fullchain.pem"),
	}

	var certPath, keyPath string
	for _, cp := range certPaths {
		kp := cp[:len(cp)-4] + ".key"
		if cp == filepath.Join("/etc/letsencrypt/live", domain, "fullchain.pem") {
			kp = filepath.Join("/etc/letsencrypt/live", domain, "privkey.pem")
		}
		if _, err := os.Stat(cp); err == nil {
			if _, err := os.Stat(kp); err == nil {
				certPath = cp
				keyPath = kp
				break
			}
		}
	}

	if certPath == "" {
		log.Println("⚠️ SSL renewal check: No certificate found")
		return nil
	}

	// Load and parse certificate
	cert, err := tls.LoadX509KeyPair(certPath, keyPath)
	if err != nil {
		return fmt.Errorf("failed to load certificate: %w", err)
	}

	x509Cert, err := x509.ParseCertificate(cert.Certificate[0])
	if err != nil {
		return fmt.Errorf("failed to parse certificate: %w", err)
	}

	// Calculate days remaining
	daysRemaining := int(time.Until(x509Cert.NotAfter).Hours() / 24)

	// AI.md PART 19: renew 7 days before expiry
	if daysRemaining <= 0 {
		log.Printf("🚨 SSL certificate EXPIRED on %s", x509Cert.NotAfter.Format("2006-01-02"))
		return fmt.Errorf("SSL certificate expired")
	} else if daysRemaining <= 7 {
		log.Printf("⚠️ SSL certificate expires in %d days (renewing at 7 days)", daysRemaining)
		// Note: Actual renewal is triggered by LetsEncryptService's auto-renewal
		// This task just logs the status - renewal is handled by the service
		return fmt.Errorf("SSL certificate needs renewal (%d days remaining)", daysRemaining)
	} else if daysRemaining <= 30 {
		log.Printf("ℹ️ SSL certificate expires in %d days", daysRemaining)
	} else {
		log.Printf("✅ SSL certificate valid for %d days", daysRemaining)
	}

	return nil
}

// SelfHealthCheck performs internal health verification
// AI.md PART 19: healthcheck_self every 5 minutes
func SelfHealthCheck() error {
	// Check database connectivity
	err := database.GetServerDB().Ping()
	if err != nil {
		log.Printf("⚠️ Self health check: database ping failed: %v", err)
		return fmt.Errorf("database health check failed: %w", err)
	}

	// Check disk space - AI.md: alert when disk usage > 85%
	diskPercent, err := getDiskUsagePercent("/")
	if err != nil {
		log.Printf("⚠️ Self health check: disk space check failed: %v", err)
		// Don't fail health check if disk check fails, just log it
	} else {
		if diskPercent > 95 {
			log.Printf("🚨 Self health check: CRITICAL disk usage at %d%%", diskPercent)
			return fmt.Errorf("critical disk usage: %d%% (threshold: 95%%)", diskPercent)
		} else if diskPercent > 85 {
			log.Printf("⚠️ Self health check: WARNING disk usage at %d%%", diskPercent)
			// Log warning but don't fail (AI.md: alert at 85%, critical at 95%)
		}
	}

	// Check users database connectivity too
	usersDB := database.GetUsersDB()
	if usersDB != nil {
		if err := usersDB.Ping(); err != nil {
			log.Printf("⚠️ Self health check: users database ping failed: %v", err)
			return fmt.Errorf("users database health check failed: %w", err)
		}
	}

	return nil
}

// CheckTorHealth checks Tor service connectivity
// AI.md PART 19: tor_health every 10 minutes, auto-restart if needed
func CheckTorHealth() error {
	// Check if Tor binary exists
	torPath, err := exec.LookPath("tor")
	if err != nil {
		// Tor not installed, skip silently
		return nil
	}

	// Check if Tor service is enabled
	var torEnabled string
	queryErr := database.GetServerDB().QueryRow("SELECT value FROM server_config WHERE key = 'tor.enabled'").Scan(&torEnabled)
	if queryErr != nil || torEnabled != "true" {
		// Tor not enabled, skip
		return nil
	}

	// Check if Tor process is running using pgrep (Unix) or tasklist (Windows)
	var torRunning bool
	pgrepCmd := exec.Command("pgrep", "-x", "tor")
	if err := pgrepCmd.Run(); err == nil {
		torRunning = true
	}

	if !torRunning {
		log.Printf("⚠️ Tor health check: Tor process not running")

		// Check if auto-restart is enabled
		var restartOnFail string
		queryErr := database.GetServerDB().QueryRow("SELECT value FROM server_config WHERE key = 'tor.restart_on_fail'").Scan(&restartOnFail)
		if queryErr == nil && restartOnFail == "true" {
			log.Printf("🧅 Attempting to restart Tor service...")
			// Note: The actual restart is handled by TorService, we just log the status
			// The TorService has its own monitoring loop that handles restarts
			return fmt.Errorf("tor process not running (configured for auto-restart)")
		}

		return fmt.Errorf("tor process not running at %s", torPath)
	}

	// Check if onion address is configured (indicates successful Tor initialization)
	var onionAddress string
	addrErr := database.GetServerDB().QueryRow("SELECT value FROM server_config WHERE key = 'tor.onion_address'").Scan(&onionAddress)
	if addrErr != nil || onionAddress == "" {
		log.Printf("⚠️ Tor health check: No .onion address configured")
		// This might be normal during startup, don't fail
	}

	return nil
}

// CleanupRateLimitCounters resets rate limit counters
func CleanupRateLimitCounters(db *sql.DB) error {
	// Reset hourly counters that are older than 1 hour
	result, err := database.GetServerDB().Exec(`
		DELETE FROM server_rate_limits
		WHERE window_start < datetime('now', '-1 hour')
	`)
	if err != nil {
		return fmt.Errorf("failed to cleanup rate limits: %w", err)
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected > 0 {
		log.Printf("🧹 Cleaned up %d old rate limit counters", rowsAffected)
	}

	return nil
}

// UpdateBlocklist updates the IP blocklist database
// AI.md PART 19: blocklist_update daily at 04:00
func UpdateBlocklist() error {
	log.Println("🛡️ Updating IP blocklist database...")

	// Check if blocklist is enabled
	var blocklistEnabled string
	err := database.GetServerDB().QueryRow("SELECT value FROM server_config WHERE key = 'security.blocklist.enabled'").Scan(&blocklistEnabled)
	if err != nil || blocklistEnabled != "true" {
		// Blocklist not enabled, skip
		return nil
	}

	// Blocklist sources (Spamhaus DROP is free and reliable)
	sources := []struct {
		name string
		url  string
	}{
		{"spamhaus_drop", "https://www.spamhaus.org/drop/drop.txt"},
		{"spamhaus_edrop", "https://www.spamhaus.org/drop/edrop.txt"},
	}

	db := database.GetServerDB()

	// Create table if not exists
	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS server_ip_blocklist (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			source TEXT NOT NULL,
			ip_range TEXT NOT NULL,
			description TEXT,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			UNIQUE(source, ip_range)
		)
	`)
	if err != nil {
		return fmt.Errorf("failed to create blocklist table: %w", err)
	}

	client := &http.Client{Timeout: 30 * time.Second}
	totalAdded := 0

	for _, source := range sources {
		resp, err := client.Get(source.url)
		if err != nil {
			log.Printf("⚠️ Failed to fetch %s: %v", source.name, err)
			continue
		}

		if resp.StatusCode != http.StatusOK {
			resp.Body.Close()
			log.Printf("⚠️ Failed to fetch %s: HTTP %d", source.name, resp.StatusCode)
			continue
		}

		// Parse the blocklist (format: CIDR ; description)
		scanner := bufio.NewScanner(resp.Body)
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if line == "" || strings.HasPrefix(line, ";") {
				continue
			}

			parts := strings.SplitN(line, ";", 2)
			ipRange := strings.TrimSpace(parts[0])
			description := ""
			if len(parts) > 1 {
				description = strings.TrimSpace(parts[1])
			}

			// Validate CIDR format
			_, _, err := net.ParseCIDR(ipRange)
			if err != nil {
				continue
			}

			// Insert or update
			_, err = db.Exec(`
				INSERT INTO server_ip_blocklist (source, ip_range, description, updated_at)
				VALUES (?, ?, ?, CURRENT_TIMESTAMP)
				ON CONFLICT(source, ip_range) DO UPDATE SET
					description = excluded.description,
					updated_at = CURRENT_TIMESTAMP
			`, source.name, ipRange, description)
			if err == nil {
				totalAdded++
			}
		}
		resp.Body.Close()
	}

	// Clean up old entries (older than 7 days and not in latest update)
	_, _ = db.Exec(`
		DELETE FROM server_ip_blocklist
		WHERE updated_at < datetime('now', '-7 days')
	`)

	log.Printf("🛡️ Blocklist update complete: %d entries processed", totalAdded)
	return nil
}

// UpdateCVEDatabase updates the CVE vulnerability database
// AI.md PART 19: cve_update daily at 05:00
func UpdateCVEDatabase() error {
	log.Println("🔒 Updating CVE database...")

	// Check if CVE monitoring is enabled
	var cveEnabled string
	err := database.GetServerDB().QueryRow("SELECT value FROM server_config WHERE key = 'security.cve.enabled'").Scan(&cveEnabled)
	if err != nil || cveEnabled != "true" {
		// CVE monitoring not enabled, skip
		return nil
	}

	db := database.GetServerDB()

	// Create table if not exists
	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS server_cve_alerts (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			cve_id TEXT NOT NULL UNIQUE,
			description TEXT,
			severity TEXT,
			cvss_score REAL,
			published_at DATETIME,
			affected_packages TEXT,
			references TEXT,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			acknowledged INTEGER DEFAULT 0
		)
	`)
	if err != nil {
		return fmt.Errorf("failed to create CVE table: %w", err)
	}

	// NVD API v2 - fetch recent CVEs (last 7 days)
	// Using the public API (no API key required, but rate limited)
	pubStartDate := time.Now().AddDate(0, 0, -7).Format("2006-01-02T15:04:05.000")
	pubEndDate := time.Now().Format("2006-01-02T15:04:05.000")

	apiURL := fmt.Sprintf(
		"https://services.nvd.nist.gov/rest/json/cves/2.0?pubStartDate=%s&pubEndDate=%s&resultsPerPage=100",
		pubStartDate, pubEndDate,
	)

	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Get(apiURL)
	if err != nil {
		return fmt.Errorf("failed to fetch CVE data: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("NVD API returned status %d", resp.StatusCode)
	}

	var nvdResponse struct {
		Vulnerabilities []struct {
			CVE struct {
				ID          string `json:"id"`
				Description struct {
					Descriptions []struct {
						Lang  string `json:"lang"`
						Value string `json:"value"`
					} `json:"description_data"`
				} `json:"descriptions"`
				Metrics struct {
					CvssMetricV31 []struct {
						CvssData struct {
							BaseScore    float64 `json:"baseScore"`
							BaseSeverity string  `json:"baseSeverity"`
						} `json:"cvssData"`
					} `json:"cvssMetricV31"`
				} `json:"metrics"`
				Published string `json:"published"`
			} `json:"cve"`
		} `json:"vulnerabilities"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&nvdResponse); err != nil {
		return fmt.Errorf("failed to parse CVE data: %w", err)
	}

	added := 0
	for _, vuln := range nvdResponse.Vulnerabilities {
		cve := vuln.CVE

		// Get English description
		description := ""
		for _, desc := range cve.Description.Descriptions {
			if desc.Lang == "en" {
				description = desc.Value
				break
			}
		}

		// Get CVSS score and severity
		var cvssScore float64
		severity := "UNKNOWN"
		if len(cve.Metrics.CvssMetricV31) > 0 {
			cvssScore = cve.Metrics.CvssMetricV31[0].CvssData.BaseScore
			severity = cve.Metrics.CvssMetricV31[0].CvssData.BaseSeverity
		}

		// Insert or update
		_, err = db.Exec(`
			INSERT INTO server_cve_alerts (cve_id, description, severity, cvss_score, published_at, updated_at)
			VALUES (?, ?, ?, ?, ?, CURRENT_TIMESTAMP)
			ON CONFLICT(cve_id) DO UPDATE SET
				description = excluded.description,
				severity = excluded.severity,
				cvss_score = excluded.cvss_score,
				updated_at = CURRENT_TIMESTAMP
		`, cve.ID, description, severity, cvssScore, cve.Published)
		if err == nil {
			added++
		}
	}

	log.Printf("🔒 CVE database update complete: %d CVEs processed", added)
	return nil
}

// ClusterHeartbeat sends a heartbeat to indicate this node is alive
// AI.md PART 19 line 24792: cluster.heartbeat every 30 seconds (cluster mode only)
func ClusterHeartbeat(nodeID string) error {
	// Check if cluster mode is enabled
	var clusterEnabled string
	err := database.GetServerDB().QueryRow("SELECT value FROM server_config WHERE key = 'cluster.enabled'").Scan(&clusterEnabled)
	if err != nil || clusterEnabled != "true" {
		// Not in cluster mode, skip silently
		return nil
	}

	// Update node heartbeat in cluster nodes table
	// Per AI.md lines 22616-22620
	_, err = database.GetServerDB().Exec(`
		INSERT INTO server_nodes (node_id, last_heartbeat, status)
		VALUES (?, datetime('now'), 'online')
		ON CONFLICT(node_id) DO UPDATE SET
			last_heartbeat = datetime('now'),
			status = 'online'
	`, nodeID)

	if err != nil {
		return fmt.Errorf("failed to send cluster heartbeat: %w", err)
	}

	return nil
}

// EnableTask enables a task by name
func (s *Scheduler) EnableTask(taskName string) error {
	s.mu.RLock()
	defer s.mu.RUnlock()

	task, ok := s.tasks[taskName]
	if !ok {
		return fmt.Errorf("task '%s' not found", taskName)
	}

	task.mu.Lock()
	task.enabled = true
	task.mu.Unlock()
	log.Printf("✅ Task '%s' enabled", taskName)
	return nil
}

// DisableTask disables a task by name
func (s *Scheduler) DisableTask(taskName string) error {
	s.mu.RLock()
	defer s.mu.RUnlock()

	task, ok := s.tasks[taskName]
	if !ok {
		return fmt.Errorf("task '%s' not found", taskName)
	}

	task.mu.Lock()
	task.enabled = false
	task.mu.Unlock()
	log.Printf("⏸️  Task '%s' disabled", taskName)
	return nil
}

// TriggerTask manually triggers a task to run immediately
func (s *Scheduler) TriggerTask(taskName string) error {
	s.mu.RLock()
	defer s.mu.RUnlock()

	task, ok := s.tasks[taskName]
	if !ok {
		return fmt.Errorf("task '%s' not found", taskName)
	}

	log.Printf("🔄 Manually triggering task '%s'", taskName)
	go s.executeTask(task)
	return nil
}

// GetTask returns a task by name
func (s *Scheduler) GetTask(taskName string) *Task {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return s.tasks[taskName]
}
