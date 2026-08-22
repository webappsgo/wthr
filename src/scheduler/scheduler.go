package scheduler

import (
	"bufio"
	"context"
	"crypto/rand"
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

	"github.com/oklog/ulid/v2"

	"github.com/webappsgo/wthr/src/backup"
	"github.com/webappsgo/wthr/src/common/dbtime"
	"github.com/webappsgo/wthr/src/common/display"
	"github.com/webappsgo/wthr/src/database"
	"github.com/webappsgo/wthr/src/path"
	"github.com/webappsgo/wthr/src/server/model"
	"github.com/webappsgo/wthr/src/util"
)

// Task represents a scheduled task
// AI.md PART 19: Scheduler uses cron expressions, not intervals
type Task struct {
	Name     string
	Schedule string // Cron expression: "0 2 * * *", "@hourly", "@every 5m"
	Fn       func() error
	schedule Schedule
	nextRun  time.Time
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

// tickInterval is how often the scheduler loop wakes up to check for due
// tasks. AI.md PART 19: "Use Go's time/ticker - No external cron libraries
// required" - one second gives sub-minute schedules (e.g. "@every 30s") the
// precision they need while staying cheap to poll.
const tickInterval = 1 * time.Second

// Scheduler manages scheduled tasks using a built-in time.Ticker loop
// AI.md PART 19: Built-in scheduler with cron expression support
type Scheduler struct {
	ticker  *time.Ticker
	stopCh  chan struct{}
	stopped chan struct{}
	tasks   map[string]*Task
	db      *sql.DB
	nodeID  string
	mu      sync.RWMutex
	running bool
}

// NewScheduler creates a new scheduler instance backed by a built-in
// time.Ticker loop instead of an external cron library.
func NewScheduler(db *sql.DB) *Scheduler {
	// Get node ID from hostname
	nodeID, err := getNodeID()
	if err != nil {
		nodeID = "default"
	}

	return &Scheduler{
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

	parsed, err := parseSchedule(schedule)
	if err != nil {
		return fmt.Errorf("failed to add task '%s' with schedule '%s': %w", name, schedule, err)
	}

	task := &Task{
		Name:     name,
		Schedule: schedule,
		Fn:       fn,
		schedule: parsed,
		nextRun:  parsed.Next(time.Now()),
		enabled:  true,
		lastRun:  nil,
	}

	s.tasks[name] = task

	return nil
}

// AddTaskInterval adds a task with a time.Duration interval (convenience method)
// Converts to @every format for the built-in scheduler
func (s *Scheduler) AddTaskInterval(name string, interval time.Duration, fn func() error) error {
	schedule := fmt.Sprintf("@every %s", interval.String())
	return s.AddTask(name, schedule, fn)
}

// Start starts the scheduler's ticker loop
// AI.md PART 19: "Use Go's time/ticker - No external cron libraries required"
func (s *Scheduler) Start() {
	s.mu.Lock()
	if s.running {
		s.mu.Unlock()
		return
	}
	s.ticker = time.NewTicker(tickInterval)
	s.stopCh = make(chan struct{})
	s.stopped = make(chan struct{})
	s.running = true
	taskCount := len(s.tasks)
	s.mu.Unlock()

	go s.run()

	log.Printf("INFO: Task manager has started (%d scheduled tasks)", taskCount)
}

// run is the scheduler's main loop, driven by a time.Ticker.
func (s *Scheduler) run() {
	defer close(s.stopped)

	for {
		select {
		case <-s.stopCh:
			return
		case now := <-s.ticker.C:
			s.runDueTasks(now)
		}
	}
}

// runDueTasks executes (asynchronously) every task whose nextRun has passed,
// and advances each executed task's nextRun using its Schedule.
func (s *Scheduler) runDueTasks(now time.Time) {
	s.mu.RLock()
	due := make([]*Task, 0)
	for _, task := range s.tasks {
		task.mu.Lock()
		if task.enabled && !task.nextRun.IsZero() && !now.Before(task.nextRun) {
			task.nextRun = task.schedule.Next(now)
			due = append(due, task)
		}
		task.mu.Unlock()
	}
	s.mu.RUnlock()

	for _, task := range due {
		go s.executeTask(task)
	}
}

// Stop stops the scheduler's ticker loop, waiting for the loop goroutine to
// exit. Running task executions are not forcibly cancelled - they complete
// on their own goroutines per AI.md PART 19's graceful-shutdown requirement.
func (s *Scheduler) Stop() {
	s.mu.Lock()
	if !s.running {
		s.mu.Unlock()
		return
	}
	log.Println("INFO: Stopping scheduler...")
	s.ticker.Stop()
	close(s.stopCh)
	stopped := s.stopped
	s.running = false
	s.mu.Unlock()

	<-stopped // Wait for the loop goroutine to exit

	log.Println("OK: Scheduler stopped")
}

// isGlobalTask returns true if this task should only run on one node
func isGlobalTask(taskName string) bool {
	return globalTasks[taskName]
}

// acquireTaskLock attempts to acquire a distributed lock for a task.
// AI.md PART 19: Cluster-aware task locking.
// schedule is the task's own cron expression: server_scheduler_state.schedule
// is NOT NULL, and this insert is the row's first writer on a fresh database,
// so the real expression has to travel with the lock request rather than be
// backfilled later by whoever happens to update the row next.
func (s *Scheduler) acquireTaskLock(taskName, schedule string) bool {
	// For non-global tasks, always allow (run on every node)
	if !isGlobalTask(taskName) {
		return true
	}

	db := database.GetServerDB()
	ctx := context.Background()
	nowText := time.Now().UTC().Format(sqlTimestampLayout)
	lockExpiry := time.Now().UTC().Add(-LockTimeout)

	// Read the current holder first. The staleness test used to live in SQL
	// ("locked_at < ?" against a bound time.Time), which compared two different
	// text encodings whenever the row had been written by a different producer
	// and therefore expired locks early, late or never. Every comparison now
	// happens in Go against parsed instants; only the canonical text written
	// here ever lands in locked_at.
	var lockedBy sql.NullString
	var lockedAt interface{}
	err := database.QueryRowContext(ctx, db, database.TimeoutSimpleSelect,
		"SELECT locked_by, locked_at FROM server_scheduler_state WHERE task_id = ?",
		taskName,
	).Scan(&lockedBy, &lockedAt)

	// No row yet: this node is the row's first writer. ON CONFLICT DO NOTHING
	// keeps the insert atomic, so a node that loses the race to create the row
	// simply does not get the lock this tick.
	if err == sql.ErrNoRows {
		result, insertErr := database.ExecContext(ctx, db, database.TimeoutWrite, `
			INSERT INTO server_scheduler_state (task_id, task_name, schedule, locked_by, locked_at, enabled)
			VALUES (?, ?, ?, ?, ?, true)
			ON CONFLICT(task_id) DO NOTHING
		`, taskName, taskName, schedule, s.nodeID, nowText)
		if insertErr != nil {
			log.Printf("WARNING: Failed to acquire lock for task '%s': %v", taskName, insertErr)
			return false
		}

		inserted, _ := result.RowsAffected()
		return inserted > 0
	}

	if err != nil {
		log.Printf("WARNING: Failed to read lock state for task '%s': %v", taskName, err)
		return false
	}

	// Decide acquirability from parsed instants:
	// 1. Nobody holds it, take it
	// 2. We hold it, refresh it
	// 3. Somebody else holds it, take it only when their lock is provably older
	//    than LockTimeout. A NULL or unparseable locked_at is treated as HELD,
	//    never as stale: guessing "stale" would let two nodes run the same
	//    global task at once, while guessing "held" only skips a tick.
	if lockedBy.Valid && lockedBy.String != s.nodeID {
		heldSince, ok := parseStoredTimestamp(lockedAt)
		if !ok || !heldSince.Before(lockExpiry) {
			return false
		}
	}

	// Compare-and-swap on the holder observed above so a concurrent stealer
	// loses: whichever node's UPDATE runs first flips locked_by, and the other
	// node's WHERE clause no longer matches.
	var result sql.Result
	if lockedBy.Valid {
		result, err = database.ExecContext(ctx, db, database.TimeoutWrite, `
			UPDATE server_scheduler_state
			SET schedule = ?, locked_by = ?, locked_at = ?
			WHERE task_id = ? AND locked_by = ?
		`, schedule, s.nodeID, nowText, taskName, lockedBy.String)
	} else {
		result, err = database.ExecContext(ctx, db, database.TimeoutWrite, `
			UPDATE server_scheduler_state
			SET schedule = ?, locked_by = ?, locked_at = ?
			WHERE task_id = ? AND locked_by IS NULL
		`, schedule, s.nodeID, nowText, taskName)
	}

	if err != nil {
		log.Printf("WARNING: Failed to acquire lock for task '%s': %v", taskName, err)
		return false
	}

	if rowsAffected, affectedErr := result.RowsAffected(); affectedErr == nil && rowsAffected > 0 {
		return true
	}

	// Zero affected rows means either a competitor won the row or the driver
	// reports "no change" for an update that rewrote identical values (MySQL
	// does this when we refresh our own lock inside the same second). Re-read
	// the holder to tell the two apart.
	var confirmed sql.NullString
	if err := database.QueryRowContext(ctx, db, database.TimeoutSimpleSelect,
		"SELECT locked_by FROM server_scheduler_state WHERE task_id = ?",
		taskName,
	).Scan(&confirmed); err != nil {
		return false
	}

	return confirmed.Valid && confirmed.String == s.nodeID
}

// releaseTaskLock releases the distributed lock for a task
func (s *Scheduler) releaseTaskLock(taskName string) {
	if !isGlobalTask(taskName) {
		return
	}

	_, err := database.ExecContext(context.Background(), database.GetServerDB(), database.TimeoutWrite, `
		UPDATE server_scheduler_state
		SET locked_by = NULL, locked_at = NULL
		WHERE task_id = ? AND locked_by = ?
	`, taskName, s.nodeID)

	if err != nil {
		log.Printf("WARNING: Failed to release lock for task '%s': %v", taskName, err)
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
	if !s.acquireTaskLock(task.Name, task.Schedule) {
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

	if err != nil {
		log.Printf("ERROR: Task '%s' failed after %v: %v", task.Name, elapsed, err)
	} else {
		log.Printf("OK: Task '%s' completed in %v", task.Name, elapsed)
	}

	// Log to audit if enabled
	s.logTaskExecution(task.Name, elapsed, err)

	// Record in database last, so that once this row becomes visible to a
	// caller polling GetLastTaskRun, every side effect of the run has
	// already completed - avoids a caller tearing down the DB (e.g. test
	// cleanup) while logTaskExecution is still in flight.
	s.RecordTaskRun(task.Name, start, end, err)
}

// logTaskExecution logs task execution to audit log
func (s *Scheduler) logTaskExecution(taskName string, duration time.Duration, err error) {
	// Check if audit logging is enabled
	var auditEnabled string
	queryErr := database.QueryRowContext(context.Background(), database.GetServerDB(), database.TimeoutSimpleSelect, "SELECT value FROM server_config WHERE key = 'audit.enabled'").Scan(&auditEnabled)
	if queryErr != nil || auditEnabled != "true" {
		return
	}

	status := "success"
	details := fmt.Sprintf("Completed in %v", duration)
	if err != nil {
		status = "error"
		details = fmt.Sprintf("Failed: %v", err)
	}

	// server_audit_log has no user_id column: the actor is described by the
	// actor_type/actor_id pair, and ulid is UNIQUE NOT NULL. This mirrors the
	// canonical writer in src/server/middleware/audit.go, including its
	// crypto/rand-seeded ULID, so both writers produce identical row shapes.
	now := time.Now()
	auditID := ulid.MustNew(ulid.Timestamp(now), rand.Reader).String()

	_, insertErr := database.ExecContext(context.Background(), database.GetServerDB(), database.TimeoutWrite, `
		INSERT INTO server_audit_log (ulid, timestamp, actor_type, actor_id, action, resource_type, resource_id, details, ip_address, user_agent, status)
		VALUES (?, ?, 'scheduler', ?, ?, 'scheduler', ?, ?, 'system', 'scheduler', ?)
	`, auditID, now.UTC().Format(sqlTimestampLayout), s.nodeID, taskName, taskName, details, status)

	if insertErr != nil {
		log.Printf("WARNING: Failed to log scheduler task: %v", insertErr)
	}
}

// GetTaskStatus returns status of all tasks
func (s *Scheduler) GetTaskStatus() []map[string]interface{} {
	s.mu.RLock()
	defer s.mu.RUnlock()

	status := make([]map[string]interface{}, 0, len(s.tasks))

	for _, task := range s.tasks {
		task.mu.Lock()
		nextRun := task.nextRun
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
// user_sessions.expires_at has more than one producer (a bound Go time.Time in
// the local zone, and canonical UTC text), so the expiry test runs in Go
// against a UTC cutoff instead of SQLite's datetime('now') - comparing those
// mixed layouts as text deleted sessions that had not actually expired.
func CleanupOldSessions(db *sql.DB) error {
	rowsAffected, err := deleteRowsWithTimestampBefore(database.GetUsersDB(), "user_sessions", "id", "expires_at", time.Now().UTC())
	if err != nil {
		return fmt.Errorf("failed to cleanup sessions: %w", err)
	}

	if rowsAffected > 0 {
		log.Printf("INFO: Cleaned up %d expired sessions", rowsAffected)
	}

	return nil
}

// CleanupOldAuditLogs removes audit logs older than retention period
func CleanupOldAuditLogs(db *sql.DB) error {
	// Get retention days from settings
	var retentionDays int
	err := database.QueryRowContext(context.Background(), database.GetServerDB(), database.TimeoutSimpleSelect, "SELECT value FROM server_config WHERE key = 'audit.retention_days'").Scan(&retentionDays)
	if err != nil {
		// Default to 90 days
		retentionDays = 90
	}

	// The retained window is measured against server_audit_log.timestamp (the
	// table has no created_at column). That column has two producers - SQLite's
	// CURRENT_TIMESTAMP default and the explicit writes from this package and
	// src/server/middleware/audit.go - so the age test runs in Go against a UTC
	// cutoff instead of as an SQL text comparison, and rows whose timestamp is
	// NULL or unparseable are left alone rather than deleted.
	cutoff := time.Now().UTC().AddDate(0, 0, -retentionDays)

	rowsAffected, err := deleteRowsWithTimestampBefore(database.GetServerDB(), "server_audit_log", "id", "timestamp", cutoff)
	if err != nil {
		return fmt.Errorf("failed to cleanup audit logs: %w", err)
	}

	if rowsAffected > 0 {
		log.Printf("INFO: Cleaned up %d old audit logs (retention: %d days)", rowsAffected, retentionDays)
	}

	return nil
}

// CheckWeatherAlerts checks for weather alerts on saved locations
func CheckWeatherAlerts() error {
	// Get all locations with alerts enabled
	rows, err := database.QueryContext(context.Background(), database.GetUsersDB(), database.TimeoutComplexSelect, `
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

	// Bound each Open-Meteo fetch so a stalled upstream can't hang the task.
	client := &http.Client{Timeout: 30 * time.Second}

	for rows.Next() {
		var locationID int
		var name string
		var latitude, longitude float64
		var userID int

		if err := rows.Scan(&locationID, &name, &latitude, &longitude, &userID); err != nil {
			log.Printf("WARNING: Failed to scan location: %v", err)
			continue
		}

		// Fetch weather data from Open-Meteo API
		url := fmt.Sprintf("https://api.open-meteo.com/v1/forecast?latitude=%.4f&longitude=%.4f&current=temperature_2m,wind_speed_10m,precipitation,weather_code&temperature_unit=fahrenheit&wind_speed_unit=mph&precipitation_unit=inch",
			latitude, longitude)

		var weatherData struct {
			Current struct {
				Temperature   float64 `json:"temperature_2m"`
				WindSpeed     float64 `json:"wind_speed_10m"`
				Precipitation float64 `json:"precipitation"`
				WeatherCode   int     `json:"weather_code"`
			} `json:"current"`
		}

		// Fetch and decode in a closure so the response body is closed on every
		// iteration rather than deferred until the whole loop returns.
		fetchErr := func() error {
			resp, err := client.Get(url)
			if err != nil {
				return err
			}
			defer resp.Body.Close()
			return json.NewDecoder(resp.Body).Decode(&weatherData)
		}()
		if fetchErr != nil {
			log.Printf("WARNING: Failed to fetch weather for %s: %v", name, fetchErr)
			continue
		}

		// Check for alert conditions and create notifications
		created := checkAndCreateAlerts(userID, locationID, name, weatherData)
		alertCount += created
	}

	if alertCount > 0 {
		log.Printf("INFO: Created %d weather alerts", alertCount)
	}

	return nil
}

// checkAndCreateAlerts checks weather conditions and creates notifications
func checkAndCreateAlerts(userID, locationID int, locationName string, weather struct {
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
		createNotification(userID, model.NotificationTypeError, "Freezing Temperature Alert",
			fmt.Sprintf("%s: Temperature is %.1f°F. Bundle up!", locationName, weather.Current.Temperature),
			fmt.Sprintf("/dashboard?location=%d", locationID))
		alertCount++
	}

	// Check for extreme heat (above 95°F / 35°C)
	if weather.Current.Temperature > 95 {
		createNotification(userID, model.NotificationTypeError, "Heat Alert",
			fmt.Sprintf("%s: Temperature is %.1f°F. Stay hydrated!", locationName, weather.Current.Temperature),
			fmt.Sprintf("/dashboard?location=%d", locationID))
		alertCount++
	}

	// Check for high winds (above 40 mph)
	if weather.Current.WindSpeed > 40 {
		createNotification(userID, model.NotificationTypeWarning, "High Wind Alert",
			fmt.Sprintf("%s: Wind speed is %.0f mph. Secure loose objects!", locationName, weather.Current.WindSpeed),
			fmt.Sprintf("/dashboard?location=%d", locationID))
		alertCount++
	}

	// Check for heavy precipitation (above 0.5 inches)
	if weather.Current.Precipitation > 0.5 {
		createNotification(userID, model.NotificationTypeInfo, "Heavy Rain Alert",
			fmt.Sprintf("%s: Heavy precipitation detected (%.1f in). Prepare for flooding!", locationName, weather.Current.Precipitation),
			fmt.Sprintf("/dashboard?location=%d", locationID))
		alertCount++
	}

	// Check for severe weather codes (thunderstorms, snow, etc.)
	if weather.Current.WeatherCode >= 95 {
		createNotification(userID, model.NotificationTypeError, "Severe Weather Alert",
			fmt.Sprintf("%s: Severe weather detected. Stay safe!", locationName),
			fmt.Sprintf("/dashboard?location=%d", locationID))
		alertCount++
	}

	return alertCount
}

// createNotification creates a user notification for a weather alert.
//
// user_notifications has no link column and constrains type with a CHECK, so
// the row is written through model.UserNotificationModel.Create - the project's
// single writer for this table. That gives the row its ULID primary key, its
// display value and its 30-day expiry, and carries the deep link in action_json
// as a NotificationAction, which is where the schema actually keeps a link.
func createNotification(userID int, notifType model.NotificationType, title, message, link string) {
	notifications := &model.UserNotificationModel{DB: database.GetUsersDB()}

	// src/server/service/notification_service.go renders every non-security
	// severity as a toast; weather alerts follow the same mapping.
	action := &model.NotificationAction{Label: "View forecast", URL: link}

	if _, err := notifications.Create(userID, notifType, model.NotificationDisplayToast, title, message, action); err != nil {
		log.Printf("WARNING: Failed to create notification: %v", err)
	}
}

// CreateSystemBackup creates a backup of the database
// AI.md PART 19/25: backup_daily task - creates verified backups
func CreateSystemBackup(db *sql.DB) error {
	// Get backup settings
	var backupEnabled string
	err := database.QueryRowContext(context.Background(), database.GetServerDB(), database.TimeoutSimpleSelect, "SELECT value FROM server_config WHERE key = 'backup.enabled'").Scan(&backupEnabled)
	if err != nil || backupEnabled != "true" {
		// Backups disabled, skip silently
		return nil
	}

	// Get paths per AI.md PART 4
	p := path.GetDefaultPaths("wthr")
	if p == nil {
		return fmt.Errorf("failed to get default paths for backup")
	}

	// Create backup service per AI.md PART 25
	svc := backup.New(p.ConfigDir, p.DataDir)

	// Check for encryption password from settings
	var encryptionPassword string
	_ = database.QueryRowContext(context.Background(), database.GetServerDB(), database.TimeoutSimpleSelect, "SELECT value FROM server_config WHERE key = 'backup.encryption_password'").Scan(&encryptionPassword)

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

	log.Println("INFO: Starting automated backup...")
	backupPath, err := svc.Create(opts)
	if err != nil {
		log.Printf("ERROR: Automated backup failed: %v", err)
		return fmt.Errorf("backup failed: %w", err)
	}

	log.Printf("OK: Automated backup completed: %s", backupPath)
	return nil
}

// CleanupExpiredTokens removes expired API tokens
// AI.md PART 19: token cleanup every 15 minutes
// user_tokens (database.UsersSchema) is this project's only API-token table and
// it lives in the users database, so the cleanup runs against
// database.GetUsersDB(). The one-time admin setup token is a file at
// {config_dir}/setup_token.txt (src/util/firstrun.go), not a database row, so
// there is nothing for a scheduled task to prune for it.
// Expiry is evaluated in Go against a UTC cutoff (see CleanupOldSessions) so a
// token whose expires_at was stored in a different layout or timezone than
// SQLite's datetime('now') is never deleted early. Rows with a NULL expires_at
// never expire and are always left in place.
func CleanupExpiredTokens(db *sql.DB) error {
	now := time.Now().UTC()

	rowsAffected, err := deleteRowsWithTimestampBefore(database.GetUsersDB(), "user_tokens", "id", "expires_at", now)
	if err != nil {
		return fmt.Errorf("failed to cleanup expired API tokens: %w", err)
	}

	if rowsAffected > 0 {
		log.Printf("INFO: Cleaned up %d expired API tokens", rowsAffected)
	}

	return nil
}

// CheckSSLRenewal checks if SSL certificates need renewal
// AI.md PART 19: SSL renewal daily at 03:00, renew 7 days before expiry
func CheckSSLRenewal() error {
	// Check if SSL is enabled via Let's Encrypt
	var sslEnabled string
	err := database.QueryRowContext(context.Background(), database.GetServerDB(), database.TimeoutSimpleSelect, "SELECT value FROM server_config WHERE key = 'ssl.letsencrypt.enabled'").Scan(&sslEnabled)
	if err != nil || sslEnabled != "true" {
		// SSL not using Let's Encrypt, skip renewal
		return nil
	}

	// Get the domain from settings
	var domain string
	domainErr := database.QueryRowContext(context.Background(), database.GetServerDB(), database.TimeoutSimpleSelect, "SELECT value FROM server_config WHERE key = 'ssl.domain'").Scan(&domain)
	if domainErr != nil || domain == "" {
		// No domain configured, skip
		return nil
	}

	log.Println("INFO: Checking SSL certificate renewal status...")

	// Get paths
	p := path.GetDefaultPaths("wthr")
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
		log.Println("WARNING: SSL renewal check: No certificate found")
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
		log.Printf("CRITICAL: SSL certificate EXPIRED on %s", x509Cert.NotAfter.Format("2006-01-02"))
		return fmt.Errorf("SSL certificate expired")
	} else if daysRemaining <= 7 {
		log.Printf("WARNING: SSL certificate expires in %d days (renewing at 7 days)", daysRemaining)
		// Note: Actual renewal is triggered by LetsEncryptService's auto-renewal
		// This task just logs the status - renewal is handled by the service
		return fmt.Errorf("SSL certificate needs renewal (%d days remaining)", daysRemaining)
	} else if daysRemaining <= 30 {
		log.Printf("%s SSL certificate expires in %d days", display.Emoji("ℹ️", "INFO:"), daysRemaining)
	} else {
		log.Printf("OK: SSL certificate valid for %d days", daysRemaining)
	}

	return nil
}

// SelfHealthCheck performs internal health verification
// AI.md PART 19: healthcheck_self every 5 minutes
func SelfHealthCheck() error {
	// Check database connectivity
	err := database.PingWithTimeout(database.GetServerDB())
	if err != nil {
		log.Printf("WARNING: Self health check: database ping failed: %v", err)
		return fmt.Errorf("database health check failed: %w", err)
	}

	// Check disk space - AI.md: alert when disk usage > 85%
	diskPercent, err := getDiskUsagePercent("/")
	if err != nil {
		log.Printf("WARNING: Self health check: disk space check failed: %v", err)
		// Don't fail health check if disk check fails, just log it
	} else {
		if diskPercent > 95 {
			log.Printf("CRITICAL: Self health check: disk usage at %d%%", diskPercent)
			return fmt.Errorf("critical disk usage: %d%% (threshold: 95%%)", diskPercent)
		} else if diskPercent > 85 {
			log.Printf("WARNING: Self health check: disk usage at %d%%", diskPercent)
			// Log warning but don't fail (AI.md: alert at 85%, critical at 95%)
		}
	}

	// Check users database connectivity too
	usersDB := database.GetUsersDB()
	if usersDB != nil {
		if err := database.PingWithTimeout(usersDB); err != nil {
			log.Printf("WARNING: Self health check: users database ping failed: %v", err)
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
	queryErr := database.QueryRowContext(context.Background(), database.GetServerDB(), database.TimeoutSimpleSelect, "SELECT value FROM server_config WHERE key = 'tor.enabled'").Scan(&torEnabled)
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
		log.Printf("WARNING: Tor health check: Tor process not running")

		// Check if auto-restart is enabled
		var restartOnFail string
		queryErr := database.QueryRowContext(context.Background(), database.GetServerDB(), database.TimeoutSimpleSelect, "SELECT value FROM server_config WHERE key = 'tor.restart_on_fail'").Scan(&restartOnFail)
		if queryErr == nil && restartOnFail == "true" {
			log.Printf("INFO: Attempting to restart Tor service...")
			// Note: The actual restart is handled by TorService, we just log the status
			// The TorService has its own monitoring loop that handles restarts
			return fmt.Errorf("tor process not running (configured for auto-restart)")
		}

		return fmt.Errorf("tor process not running at %s", torPath)
	}

	// Check if onion address is configured (indicates successful Tor initialization)
	var onionAddress string
	addrErr := database.QueryRowContext(context.Background(), database.GetServerDB(), database.TimeoutSimpleSelect, "SELECT value FROM server_config WHERE key = 'tor.onion_address'").Scan(&onionAddress)
	if addrErr != nil || onionAddress == "" {
		log.Printf("WARNING: Tor health check: No .onion address configured")
		// This might be normal during startup, don't fail
	}

	return nil
}

// CleanupRateLimitCounters resets rate limit counters
// The 1-hour cutoff is computed in Go and each window_start is compared as a
// UTC instant (see CleanupOldSessions), so counters inside the current window
// are never dropped because of a layout or timezone mismatch.
func CleanupRateLimitCounters(db *sql.DB) error {
	// Reset hourly counters that are older than 1 hour
	cutoff := time.Now().UTC().Add(-1 * time.Hour)

	rowsAffected, err := deleteRowsWithTimestampBefore(database.GetServerDB(), "server_rate_limits", "id", "window_start", cutoff)
	if err != nil {
		return fmt.Errorf("failed to cleanup rate limits: %w", err)
	}

	if rowsAffected > 0 {
		log.Printf("INFO: Cleaned up %d old rate limit counters", rowsAffected)
	}

	return nil
}

// UpdateBlocklist updates the IP blocklist database
// AI.md PART 19: blocklist_update daily at 04:00
func UpdateBlocklist() error {
	log.Println("INFO: Updating IP blocklist database...")

	// Check if blocklist is enabled
	var blocklistEnabled string
	err := database.QueryRowContext(context.Background(), database.GetServerDB(), database.TimeoutSimpleSelect, "SELECT value FROM server_config WHERE key = 'security.blocklist.enabled'").Scan(&blocklistEnabled)
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
	_, err = database.ExecContext(context.Background(), db, database.TimeoutMigration, `
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
			log.Printf("WARNING: Failed to fetch %s: %v", source.name, err)
			continue
		}

		if resp.StatusCode != http.StatusOK {
			resp.Body.Close()
			log.Printf("WARNING: Failed to fetch %s: HTTP %d", source.name, resp.StatusCode)
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

			// Insert or update. The timestamp is computed in Go and bound
			// twice rather than left to CURRENT_TIMESTAMP: on SQLite the
			// literal yields canonical UTC text, but PostgreSQL and MySQL
			// render it in the server's own zone and type, which would mix
			// layouts inside one column and break the cutoff comparison below.
			blocklistNow := dbtime.FormatSQLTimestamp(time.Now())
			_, err = database.ExecContext(context.Background(), db, database.TimeoutWrite, `
				INSERT INTO server_ip_blocklist (source, ip_range, description, updated_at)
				VALUES (?, ?, ?, ?)
				ON CONFLICT(source, ip_range) DO UPDATE SET
					description = excluded.description,
					updated_at = ?
			`, source.name, ipRange, description, blocklistNow, blocklistNow)
			if err == nil {
				totalAdded++
			}
		}
		resp.Body.Close()
	}

	// Clean up old entries (older than 7 days and not in latest update)
	// updated_at is only ever written as canonical UTC text above, so a bound
	// canonical UTC cutoff compares exactly and works on every driver.
	blocklistCutoff := time.Now().UTC().AddDate(0, 0, -7).Format(sqlTimestampLayout)
	if _, err := database.ExecContext(context.Background(), db, database.TimeoutBulk, `
		DELETE FROM server_ip_blocklist
		WHERE updated_at < ?
	`, blocklistCutoff); err != nil {
		log.Printf("WARNING: Failed to prune old blocklist entries: %v", err)
	}

	log.Printf("INFO: Blocklist update complete: %d entries processed", totalAdded)
	return nil
}

// nvdPublishedLayouts lists the shapes the NVD API 2.0 has been observed to
// render its "published" field in. The documented form carries milliseconds and
// no zone marker and is UTC by definition; the other two are accepted because a
// feed that starts emitting them should not silently produce NULL publication
// dates. Zone-less layouts are parsed with time.Parse, which yields UTC.
var nvdPublishedLayouts = []string{
	"2006-01-02T15:04:05.000",
	"2006-01-02T15:04:05",
	time.RFC3339,
}

// parseNVDPublished converts an NVD publication string to an instant. It never
// falls back to "now" or to the zero time on failure - the caller stores NULL
// instead, so a value this function cannot read stays visibly absent rather than
// becoming a plausible-looking wrong date.
func parseNVDPublished(published string) (time.Time, error) {
	trimmed := strings.TrimSpace(published)
	if trimmed == "" {
		return time.Time{}, fmt.Errorf("empty published timestamp")
	}

	for _, layout := range nvdPublishedLayouts {
		if parsed, err := time.Parse(layout, trimmed); err == nil {
			return parsed.UTC(), nil
		}
	}

	return time.Time{}, fmt.Errorf("unrecognized published timestamp %q", trimmed)
}

// UpdateCVEDatabase updates the CVE vulnerability database
// AI.md PART 19: cve_update daily at 05:00
func UpdateCVEDatabase() error {
	log.Println("INFO: Updating CVE database...")

	// Check if CVE monitoring is enabled
	var cveEnabled string
	err := database.QueryRowContext(context.Background(), database.GetServerDB(), database.TimeoutSimpleSelect, "SELECT value FROM server_config WHERE key = 'security.cve.enabled'").Scan(&cveEnabled)
	if err != nil || cveEnabled != "true" {
		// CVE monitoring not enabled, skip
		return nil
	}

	db := database.GetServerDB()

	// server_cve_alerts is declared in database.ServerSchema, which
	// database.InitDualDB executes on startup. The CREATE TABLE that used to run
	// here at task time named a column "references" - a reserved word no
	// supported driver accepts as a bare identifier - so this task aborted on its
	// very first statement every time CVE monitoring was enabled, and no CVE was
	// ever stored. The schema owns the definition now; this check only turns a
	// missing table into a named error.
	var cveTablePresent int
	if err := database.QueryRowContext(context.Background(), db, database.TimeoutSimpleSelect,
		"SELECT COUNT(*) FROM server_cve_alerts WHERE 1 = 0",
	).Scan(&cveTablePresent); err != nil {
		return fmt.Errorf("server_cve_alerts is missing from the server database schema: %w", err)
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
				ID string `json:"id"`
				// NVD API 2.0 returns cve.descriptions as an array of
				// {lang, value} objects directly. It was previously decoded as an
				// object wrapping a "description_data" array - the shape of the
				// retired 1.0 feed - so the array never bound and every stored CVE
				// had an empty description.
				Descriptions []struct {
					Lang  string `json:"lang"`
					Value string `json:"value"`
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
		for _, desc := range cve.Descriptions {
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

		// published_at was previously bound straight from the NVD API's own
		// string, putting a third layout into a column every reader parses as
		// canonical UTC text. It is converted here instead. A value the API
		// renders in some other shape is stored as NULL rather than guessed at:
		// the column is nullable, and a wrong instant is worse than a missing one.
		var publishedAt interface{}
		if parsed, perr := parseNVDPublished(cve.Published); perr == nil {
			publishedAt = dbtime.FormatSQLTimestamp(parsed)
		} else {
			log.Printf("WARN: CVE %s has an unrecognized published timestamp %q; storing NULL", cve.ID, cve.Published)
		}

		// updated_at is bound as canonical UTC text for the same cross-driver
		// reason as the blocklist upsert above.
		cveNow := dbtime.FormatSQLTimestamp(time.Now())
		_, err = database.ExecContext(context.Background(), db, database.TimeoutWrite, `
			INSERT INTO server_cve_alerts (cve_id, description, severity, cvss_score, published_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?)
			ON CONFLICT(cve_id) DO UPDATE SET
				description = excluded.description,
				severity = excluded.severity,
				cvss_score = excluded.cvss_score,
				published_at = excluded.published_at,
				updated_at = ?
		`, cve.ID, description, severity, cvssScore, publishedAt, cveNow, cveNow)
		if err == nil {
			added++
		}
	}

	log.Printf("INFO: CVE database update complete: %d CVEs processed", added)
	return nil
}

// ClusterHeartbeat sends a heartbeat to indicate this node is alive
// AI.md PART 19 line 24792: cluster.heartbeat every 30 seconds (cluster mode only)
func ClusterHeartbeat(nodeID string) error {
	// Check if cluster mode is enabled
	var clusterEnabled string
	err := database.QueryRowContext(context.Background(), database.GetServerDB(), database.TimeoutSimpleSelect, "SELECT value FROM server_config WHERE key = 'cluster.enabled'").Scan(&clusterEnabled)
	if err != nil || clusterEnabled != "true" {
		// Not in cluster mode, skip silently
		return nil
	}

	// Update node heartbeat in cluster nodes table
	// Per AI.md lines 22616-22620
	// The heartbeat instant is computed in Go, converted to UTC and bound as
	// canonical text rather than emitted by SQLite's datetime('now'): readers
	// compare last_heartbeat against a Go-side UTC cutoff, and datetime() is not
	// available on PostgreSQL/MySQL, both of which this project supports.
	heartbeat := time.Now().UTC().Format(sqlTimestampLayout)

	// server_nodes.hostname is NOT NULL, and this upsert is the only writer that
	// ever creates the row, so it has to supply the name. util.GetFQDN is the
	// project's single host-resolution helper (proxy/DOMAIN aware, falling back
	// through os.Hostname to a routable address), so a node identifies itself the
	// same way here as everywhere else the app names itself.
	_, err = database.ExecContext(context.Background(), database.GetServerDB(), database.TimeoutWrite, `
		INSERT INTO server_nodes (node_id, hostname, last_heartbeat, status)
		VALUES (?, ?, ?, 'online')
		ON CONFLICT(node_id) DO UPDATE SET
			hostname = excluded.hostname,
			last_heartbeat = excluded.last_heartbeat,
			status = 'online'
	`, nodeID, util.GetFQDN(), heartbeat)

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
	log.Printf("OK: Task '%s' enabled", taskName)
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
	log.Printf("INFO: Task '%s' disabled", taskName)
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

	log.Printf("INFO: Manually triggering task '%s'", taskName)
	go s.executeTask(task)
	return nil
}

// GetTask returns a task by name
func (s *Scheduler) GetTask(taskName string) *Task {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return s.tasks[taskName]
}
