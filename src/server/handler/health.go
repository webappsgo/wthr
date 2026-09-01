package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"github.com/webappsgo/wthr/src/server/middleware"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/webappsgo/wthr/src/common/dbtime"
	"github.com/webappsgo/wthr/src/config"
	"github.com/webappsgo/wthr/src/database"
	models "github.com/webappsgo/wthr/src/server/model"
	"github.com/webappsgo/wthr/src/util"
)

var (
	initStatus = &util.InitializationStatus{
		Countries: false,
		Cities:    false,
		Weather:   false,
		Started:   time.Now(),
	}
	initMutex sync.RWMutex

	// TorStatusGetter interface for getting Tor service status
	torStatusGetter TorStatusProvider
	torMutex        sync.RWMutex

	// I2PStatusGetter interface for getting I2P eepsite status (AI.md PART 32.2)
	i2pStatusGetter I2PStatusProvider
	i2pMutex        sync.RWMutex
)

// TorStatusProvider is an interface for getting Tor service status
type TorStatusProvider interface {
	IsRunning() bool
	GetOnionAddress() string
}

// SetTorStatusProvider sets the global Tor status provider
func SetTorStatusProvider(provider TorStatusProvider) {
	torMutex.Lock()
	defer torMutex.Unlock()
	torStatusGetter = provider
}

// GetTorStatus returns the current Tor service status
func GetTorStatus() (running bool, onionAddress string) {
	torMutex.RLock()
	defer torMutex.RUnlock()
	if torStatusGetter == nil {
		return false, ""
	}
	return torStatusGetter.IsRunning(), torStatusGetter.GetOnionAddress()
}

// I2PStatusProvider is an interface for getting I2P eepsite status per AI.md PART 32.2
type I2PStatusProvider interface {
	IsRunning() bool
	Status() string
	Provider() string
	EepsiteAddress() string
}

// SetI2PStatusProvider sets the global I2P status provider
func SetI2PStatusProvider(provider I2PStatusProvider) {
	i2pMutex.Lock()
	defer i2pMutex.Unlock()
	i2pStatusGetter = provider
}

// GetI2PStatus returns the current I2P eepsite status
func GetI2PStatus() (running bool, status, provider, address string) {
	i2pMutex.RLock()
	defer i2pMutex.RUnlock()
	if i2pStatusGetter == nil {
		return false, "disabled", "none", ""
	}
	return i2pStatusGetter.IsRunning(), i2pStatusGetter.Status(), i2pStatusGetter.Provider(), i2pStatusGetter.EepsiteAddress()
}

// SetInitStatus updates initialization status
func SetInitStatus(countries, cities, weather bool) {
	initMutex.Lock()
	defer initMutex.Unlock()

	initStatus.Countries = countries
	initStatus.Cities = cities
	initStatus.Weather = weather
}

// IsInitialized checks if all services are initialized
func IsInitialized() bool {
	initMutex.RLock()
	defer initMutex.RUnlock()

	return initStatus.Countries && initStatus.Cities && initStatus.Weather
}

// GetInitStatus returns current initialization status
func GetInitStatus() *util.InitializationStatus {
	initMutex.RLock()
	defer initMutex.RUnlock()

	return &util.InitializationStatus{
		Countries: initStatus.Countries,
		Cities:    initStatus.Cities,
		Weather:   initStatus.Weather,
		Started:   initStatus.Started,
	}
}

type publicHealthProject struct {
	Name        string `json:"name"`
	Tagline     string `json:"tagline,omitempty"`
	Description string `json:"description"`
}

type publicHealthBuild struct {
	Commit string `json:"commit"`
	Date   string `json:"date"`
}

type publicHealthCluster struct {
	Enabled   bool     `json:"enabled"`
	Status    string   `json:"status,omitempty"`
	Primary   string   `json:"primary"`
	Nodes     []string `json:"nodes"`
	NodeCount int      `json:"node_count,omitempty"`
	Role      string   `json:"role,omitempty"`
}

type publicHealthTor struct {
	Enabled  bool   `json:"enabled"`
	Running  bool   `json:"running"`
	Status   string `json:"status,omitempty"`
	Hostname string `json:"hostname,omitempty"`
}

// publicHealthI2P mirrors AI.md PART 32.2 I2PInfo - opt-in, so it stays
// disabled unless features.i2p is turned on.
type publicHealthI2P struct {
	Enabled  bool   `json:"enabled"`
	Running  bool   `json:"running"`
	Status   string `json:"status,omitempty"`
	Hostname string `json:"hostname,omitempty"`
	Provider string `json:"provider,omitempty"`
}

type publicHealthFeatures struct {
	MultiUser bool            `json:"multi_user"`
	Tor       publicHealthTor `json:"tor"`
	I2P       publicHealthI2P `json:"i2p"`
	GeoIP     bool            `json:"geoip"`
}

type publicHealthChecks struct {
	Database  string `json:"database"`
	Cache     string `json:"cache"`
	Disk      string `json:"disk"`
	Scheduler string `json:"scheduler"`
	Cluster   string `json:"cluster,omitempty"`
	Tor       string `json:"tor,omitempty"`
	I2P       string `json:"i2p,omitempty"`
}

type publicHealthStats struct {
	RequestsTotal     int `json:"requests_total"`
	Requests24H       int `json:"requests_24h"`
	ActiveConnections int `json:"active_connections"`
}

type publicHealthMaintenance struct {
	Reason  string `json:"reason"`
	Message string `json:"message"`
}

type publicHealthResponse struct {
	Project     publicHealthProject      `json:"project"`
	Status      string                   `json:"status"`
	Version     string                   `json:"version"`
	GoVersion   string                   `json:"go_version"`
	Build       publicHealthBuild        `json:"build"`
	Uptime      string                   `json:"uptime"`
	Mode        string                   `json:"mode"`
	Timestamp   string                   `json:"timestamp"`
	Cluster     publicHealthCluster      `json:"cluster"`
	Features    publicHealthFeatures     `json:"features"`
	Checks      publicHealthChecks       `json:"checks"`
	Stats       publicHealthStats        `json:"stats"`
	Maintenance *publicHealthMaintenance `json:"maintenance,omitempty"`
}

// HealthCheck handles GET /server/healthz with browser/html, CLI/text, and API/json negotiation.
// @Summary Health status
// @Description Public health status. Responds with HTML for browsers, plain text for CLI, JSON for API clients.
// @Tags System
// @Produce json
// @Success 200 {object} map[string]interface{} "Service healthy"
// @Failure 503 {object} map[string]interface{} "Service unhealthy"
// @Router /server/healthz [get]
func HealthCheck(db *database.DB, startTime time.Time) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		statusCode, response := buildPublicHealthResponse(db, startTime, r)

		switch {
		case shouldRespondText(r):
			w.Header().Set("Content-Type", "text/plain; charset=utf-8")
			w.WriteHeader(statusCode)
			_, _ = w.Write([]byte(formatPublicHealthText(response)))
		case wantsExplicitJSON(r):
			renderIndentedJSON(w, statusCode, response)
		case util.IsBrowser(r):
			middleware.RenderHTML(w, r, statusCode, "page/healthz.tmpl", util.TemplateData(r, map[string]interface{}{
				"title":               "Health Status",
				"page":                "healthz",
				"health":              response,
				"health_status_class": publicHealthStatusClass(response.Status),
				"health_status_text":  publicHealthStatusText(response.Status),
			}))
		default:
			w.Header().Set("Content-Type", "text/plain; charset=utf-8")
			w.WriteHeader(statusCode)
			_, _ = w.Write([]byte(formatPublicHealthText(response)))
		}
	}
}

// LivenessCheck handles GET /health — simple liveness probe per AI.md PART 13.
// Returns 200 as long as the server process is alive, 503 only if startup panicked.
// @Summary Liveness probe
// @Description Kubernetes/container liveness probe. 200 = process alive.
// @Tags System
// @Produce json
// @Success 200 {object} map[string]interface{} "Alive"
// @Router /health [get]
func LivenessCheck(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]interface{}{"status": "alive"})
}

// ReadinessCheck handles GET /health/ready — readiness probe per AI.md PART 13.
// Returns 503 until fully initialized and the database is reachable.
// @Summary Readiness probe
// @Description Kubernetes/container readiness probe. 200 = ready to serve traffic.
// @Tags System
// @Produce json
// @Success 200 {object} map[string]interface{} "Ready"
// @Failure 503 {object} map[string]interface{} "Not ready"
// @Router /health/ready [get]
func ReadinessCheck(db *database.DB, startTime time.Time) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !IsInitialized() {
			writeJSON(w, http.StatusServiceUnavailable, map[string]interface{}{"status": "not_ready", "reason": "initializing"})
			return
		}
		_, _, dbErr := db.HealthCheck()
		if dbErr != nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]interface{}{"status": "not_ready", "reason": "database_unavailable"})
			return
		}
		writeJSON(w, http.StatusOK, map[string]interface{}{"status": "ready", "uptime": formatUptime(time.Since(startTime))})
	}
}

// FullHealthCheck handles GET /health/full — comprehensive JSON status per AI.md PART 13.
// Always returns JSON (same payload as /server/healthz with explicit JSON accept).
// @Summary Full health status
// @Description Comprehensive health status as JSON — includes DB, scheduler, GeoIP, Tor, cluster state.
// @Tags System
// @Produce json
// @Success 200 {object} map[string]interface{} "Full health payload"
// @Failure 503 {object} map[string]interface{} "Service unhealthy"
// @Router /health/full [get]
func FullHealthCheck(db *database.DB, startTime time.Time) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		statusCode, response := buildPublicHealthResponse(db, startTime, r)
		renderIndentedJSON(w, statusCode, response)
	}
}

// DebugInfo handles GET /debug/info
func DebugInfo(w http.ResponseWriter, r *http.Request) {
	status := GetInitStatus()
	uptime := time.Since(status.Started)

	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	// Name and version come from the same sources /server/healthz uses -
	// branding config and the ldflags-injected Version - so the two surfaces
	// can never disagree.
	serviceName := "wthr"
	if cfg := config.GetGlobalConfig(); cfg != nil && strings.TrimSpace(cfg.Server.Branding.Title) != "" {
		serviceName = strings.TrimSpace(cfg.Server.Branding.Title)
	}
	serviceVersion := strings.TrimSpace(Version)
	if serviceVersion == "" {
		serviceVersion = "dev"
	}

	info := map[string]interface{}{
		"service": map[string]interface{}{
			"name":    serviceName,
			"version": serviceVersion,
			"uptime":  uptime.String(),
			"started": status.Started.Format(time.RFC3339),
		},
		"initialization": map[string]interface{}{
			"ready":     IsInitialized(),
			"countries": status.Countries,
			"cities":    status.Cities,
			"weather":   status.Weather,
		},
		"runtime": map[string]interface{}{
			"go_version":    runtime.Version(),
			"num_cpu":       runtime.NumCPU(),
			"num_goroutine": runtime.NumGoroutine(),
		},
		"memory": map[string]interface{}{
			"alloc_mb":       fmt.Sprintf("%.2f", float64(m.Alloc)/1024/1024),
			"total_alloc_mb": fmt.Sprintf("%.2f", float64(m.TotalAlloc)/1024/1024),
			"sys_mb":         fmt.Sprintf("%.2f", float64(m.Sys)/1024/1024),
			"num_gc":         m.NumGC,
		},
		"timestamp": util.Now(),
	}

	writeJSON(w, http.StatusOK, info)
}

// ServeLoadingPage renders the loading/initialization page
func ServeLoadingPage(w http.ResponseWriter, r *http.Request) {
	status := GetInitStatus()
	uptime := time.Since(status.Started)

	// Check if it's a console client (curl/wget) first: these tools also
	// match WantsJSON's UA sniffing, which would otherwise make this
	// ASCII-banner branch unreachable. Explicit JSON requests (Accept:
	// application/json, ?format=json, or /api/ routes) still take
	// priority and are handled below via WantsJSON.
	userAgent := r.Header.Get("User-Agent")
	isCurl := contains(userAgent, "curl") || contains(userAgent, "wget") || contains(userAgent, "HTTPie")
	accept := r.Header.Get("Accept")
	explicitJSON := strings.Contains(accept, "application/json") ||
		r.URL.Query().Get("format") == "json" ||
		strings.HasPrefix(r.URL.Path, "/api/")

	// Check if it's an API request (wants JSON)
	if explicitJSON || (WantsJSON(r) && !isCurl) {
		RespondNegotiatedData(w, r, http.StatusServiceUnavailable, map[string]interface{}{
			"status":  "Initializing",
			"message": "Services are starting up. Please wait a moment.",
			"initialization": map[string]interface{}{
				"countries": status.Countries,
				"cities":    status.Cities,
				"weather":   status.Weather,
			},
			"uptime":    uptime.String(),
			"timestamp": util.Now(),
		})
		return
	}

	if isCurl {
		// Console-friendly ASCII output
		output := fmt.Sprintf(`🚀 Weather - Starting Up

Services Initialization:
  [%s] Countries Database
  [%s] Cities Database
  [%s] Weather

Uptime: %s

⏳ Please wait a moment and try again...

Tip: Check status with:
  curl -q -LSs %s/server/healthz
`,
			checkmark(status.Countries),
			checkmark(status.Cities),
			checkmark(status.Weather),
			uptime.Round(time.Second).String(),
			util.GetHostInfo(r).FullHost,
		)

		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte(output))
		return
	}

	// Browser gets HTML loading page
	hostInfo := util.GetHostInfo(r)

	middleware.RenderHTML(w, r, http.StatusServiceUnavailable, "component/loading.tmpl", util.TemplateData(r, map[string]interface{}{
		"Title":    "Starting Up - Weather",
		"Status":   status,
		"Uptime":   uptime.String(),
		"HostInfo": hostInfo,
	}))
}

// Helper functions

func checkmark(ready bool) string {
	if ready {
		return "✓"
	}
	return "⋯"
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && findSubstring(s, substr)
}

func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// APIHealthCheck handles GET /api/{api_version}/server/healthz - same JSON as /server/healthz, always JSON.
func APIHealthCheck(db *database.DB, startTime time.Time) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		statusCode, response := buildPublicHealthResponse(db, startTime, r)
		renderIndentedJSON(w, statusCode, response)
	}
}

// formatUptime converts duration to human-readable format (e.g., "2d 5h 30m")
func formatUptime(d time.Duration) string {
	days := int(d.Hours()) / 24
	hours := int(d.Hours()) % 24
	minutes := int(d.Minutes()) % 60

	if days > 0 {
		return fmt.Sprintf("%dd %dh %dm", days, hours, minutes)
	}
	if hours > 0 {
		return fmt.Sprintf("%dh %dm", hours, minutes)
	}
	return fmt.Sprintf("%dm", minutes)
}

func buildPublicHealthResponse(db *database.DB, startTime time.Time, r *http.Request) (int, publicHealthResponse) {
	cfg := config.GetGlobalConfig()
	brandingTitle := "wthr"
	brandingTagline := ""
	brandingDescription := "Weather information service"
	modeName := "production"
	if cfg != nil {
		if strings.TrimSpace(cfg.Server.Branding.Title) != "" {
			brandingTitle = strings.TrimSpace(cfg.Server.Branding.Title)
		}
		brandingTagline = strings.TrimSpace(cfg.Server.Branding.Tagline)
		if strings.TrimSpace(cfg.Server.Branding.Description) != "" {
			brandingDescription = strings.TrimSpace(cfg.Server.Branding.Description)
		}
		if strings.TrimSpace(cfg.Server.Mode) != "" {
			modeName = strings.TrimSpace(cfg.Server.Mode)
		}
	} else if envMode := strings.TrimSpace(os.Getenv("MODE")); envMode != "" {
		modeName = envMode
	}

	version := strings.TrimSpace(Version)
	if version == "" {
		version = readVersion()
	}
	buildDate := strings.TrimSpace(BuildDate)
	if buildDate == "" {
		buildDate = "unknown"
	}
	buildCommit := strings.TrimSpace(CommitID)
	if buildCommit == "" {
		buildCommit = "unknown"
	}

	dbStatus, _, dbErr := db.HealthCheck()
	dbCheck := "ok"
	if dbErr != nil || dbStatus != "connected" {
		dbCheck = "error"
	}

	diskCheck := getPublicDiskCheck()
	schedulerCheck := getPublicSchedulerCheck()
	cluster := getPublicClusterInfo(db, r)
	geoIPEnabled := getPublicGeoIPStatus(db)
	torFeature, torCheck := getPublicTorStatus(cfg)
	i2pFeature, i2pCheck := getPublicI2PStatus(cfg)
	stats := getPublicStats(db)
	maintenanceMode := getMaintenanceMode(db)

	response := publicHealthResponse{
		Project: publicHealthProject{
			Name:        brandingTitle,
			Tagline:     brandingTagline,
			Description: brandingDescription,
		},
		Status:    "healthy",
		Version:   version,
		GoVersion: runtime.Version(),
		Build: publicHealthBuild{
			Commit: buildCommit,
			Date:   buildDate,
		},
		Uptime:    formatUptime(time.Since(startTime)),
		Mode:      modeName,
		Timestamp: util.Now(),
		Cluster:   cluster,
		Features: publicHealthFeatures{
			MultiUser: config.IsMultiUserEnabled(),
			Tor:       torFeature,
			I2P:       i2pFeature,
			GeoIP:     geoIPEnabled,
		},
		Checks: publicHealthChecks{
			Database:  dbCheck,
			Cache:     "ok",
			Disk:      diskCheck,
			Scheduler: schedulerCheck,
			Cluster:   "",
			Tor:       torCheck,
			I2P:       i2pCheck,
		},
		Stats: stats,
	}

	if !cluster.Enabled {
		response.Checks.Cluster = ""
	} else {
		response.Checks.Cluster = clusterCheckFromStatus(cluster.Status)
	}

	if !torFeature.Enabled {
		response.Checks.Tor = ""
	}

	if !i2pFeature.Enabled {
		response.Checks.I2P = ""
	}

	statusCode := http.StatusOK
	switch {
	case maintenanceMode:
		response.Status = "maintenance"
		response.Mode = "maintenance"
		response.Maintenance = &publicHealthMaintenance{
			Reason:  "maintenance_mode",
			Message: "Server is in maintenance mode",
		}
		statusCode = http.StatusServiceUnavailable
	case !IsInitialized() || dbCheck == "error":
		response.Status = "unhealthy"
		statusCode = http.StatusServiceUnavailable
	case response.Checks.Disk == "degraded" ||
		response.Checks.Disk == "error" ||
		response.Checks.Scheduler == "degraded" ||
		response.Checks.Scheduler == "error" ||
		response.Checks.Cluster == "degraded" ||
		response.Checks.Cluster == "error" ||
		response.Checks.Tor == "error" ||
		response.Checks.I2P == "error":
		response.Status = "degraded"
	}

	return statusCode, response
}

func getPublicDiskCheck() string {
	dataUsage := getDiskUsage(getDataDir())
	logUsage := getDiskUsage(getLogDir())
	if dataUsage.TotalBytes == 0 || logUsage.TotalBytes == 0 {
		return "degraded"
	}

	maxUsed := dataUsage.UsedPercent
	if logUsage.UsedPercent > maxUsed {
		maxUsed = logUsage.UsedPercent
	}

	switch {
	case maxUsed > 95:
		return "error"
	case maxUsed > 80:
		return "degraded"
	default:
		return "ok"
	}
}

func getPublicSchedulerCheck() string {
	schedulerStatus := getSchedulerStatus()
	status, _ := schedulerStatus["status"].(string)
	switch status {
	case "running":
		return "ok"
	case "unknown":
		return "error"
	default:
		return "degraded"
	}
}

func getPublicClusterInfo(db *database.DB, r *http.Request) publicHealthCluster {
	cluster := publicHealthCluster{
		Enabled: false,
		Primary: "",
		Nodes:   []string{},
	}

	var clusterEnabled string
	if err := database.QueryRowContext(context.Background(), db.DB, database.TimeoutSimpleSelect, "SELECT value FROM server_config WHERE key = 'cluster.enabled'").Scan(&clusterEnabled); err != nil || clusterEnabled != "true" {
		return cluster
	}

	hostInfo := util.GetHostInfo(r)
	cluster.Enabled = true
	cluster.Primary = hostInfo.FullHost
	cluster.Nodes = []string{hostInfo.FullHost}
	cluster.Role = "member"

	var nodeCount int
	if err := database.QueryRowContext(context.Background(), db.DB, database.TimeoutSimpleSelect, "SELECT COUNT(*) FROM server_nodes WHERE status IN ('online', 'active')").Scan(&nodeCount); err != nil || nodeCount < 1 {
		nodeCount = 1
	}
	cluster.NodeCount = nodeCount
	if nodeCount > 0 {
		cluster.Status = "connected"
	} else {
		cluster.Status = "degraded"
	}

	return cluster
}

func getPublicGeoIPStatus(db *database.DB) bool {
	settingsModel := &models.SettingsModel{DB: database.GetServerDB()}
	return settingsModel.GetBool("geoip.enabled", true)
}

func getPublicTorStatus(cfg *config.AppConfig) (publicHealthTor, string) {
	torRunning, torHostname := GetTorStatus()
	torEnabled := torHostname != "" || torRunning
	if !torEnabled && cfg != nil {
		torEnabled = cfg.Server.Tor.Enabled
	}
	if !torEnabled {
		if _, err := exec.LookPath("tor"); err == nil {
			torEnabled = true
		}
	}

	torStatus := ""
	torCheck := ""
	if torEnabled {
		if torRunning {
			torStatus = "healthy"
			torCheck = "ok"
		} else {
			torStatus = "error"
			torCheck = "error"
		}
	}

	return publicHealthTor{
		Enabled:  torEnabled,
		Running:  torRunning,
		Status:   torStatus,
		Hostname: torHostname,
	}, torCheck
}

// getPublicI2PStatus reports the eepsite state per AI.md PART 32.2. I2P is
// opt-in, so a disabled feature reports enabled=false and contributes no check.
func getPublicI2PStatus(cfg *config.AppConfig) (publicHealthI2P, string) {
	i2pEnabled := false
	if cfg != nil {
		i2pEnabled = cfg.Server.Features.I2P || cfg.Server.I2P.Enabled
	}
	if !i2pEnabled {
		return publicHealthI2P{Enabled: false, Status: "disabled", Provider: "none"}, ""
	}

	running, status, provider, address := GetI2PStatus()
	i2pCheck := "ok"
	if !running {
		i2pCheck = "error"
	}

	return publicHealthI2P{
		Enabled:  true,
		Running:  running,
		Status:   status,
		Hostname: address,
		Provider: provider,
	}, i2pCheck
}

func getPublicStats(db *database.DB) publicHealthStats {
	stats := publicHealthStats{}

	_ = database.QueryRowContext(context.Background(), db.DB, database.TimeoutSimpleSelect, "SELECT COUNT(*) FROM server_audit_log").Scan(&stats.RequestsTotal)
	// The cutoff is computed in Go and bound as canonical UTC text rather than
	// left to SQLite's datetime('now', '-24 hours'). Every producer of
	// server_audit_log.timestamp writes dbtime.FormatSQLTimestamp output, which
	// is fixed-width UTC, so a plain text comparison against a cutoff in the
	// same layout orders correctly - and unlike datetime(), it does not depend
	// on a SQLite-only function this project's other supported drivers
	// (PostgreSQL, MySQL) spell differently.
	cutoff := dbtime.FormatSQLTimestamp(time.Now().Add(-24 * time.Hour))
	_ = database.QueryRowContext(context.Background(), db.DB, database.TimeoutSimpleSelect, "SELECT COUNT(*) FROM server_audit_log WHERE timestamp >= ?", cutoff).Scan(&stats.Requests24H)

	sessionCount, err := db.GetSessionCount()
	if err == nil {
		stats.ActiveConnections = sessionCount
	}

	return stats
}

func getMaintenanceMode(db *database.DB) bool {
	settingsModel := &models.SettingsModel{DB: database.GetServerDB()}
	return settingsModel.GetBool("maintenance.mode", false)
}

func clusterCheckFromStatus(status string) string {
	switch status {
	case "connected":
		return "ok"
	case "degraded":
		return "degraded"
	default:
		return "error"
	}
}

func renderIndentedJSON(w http.ResponseWriter, status int, data interface{}) {
	payload, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("INTERNAL_ERROR: failed to render response\n"))
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_, _ = w.Write(append(payload, '\n'))
}

func wantsExplicitJSON(r *http.Request) bool {
	accept := r.Header.Get("Accept")
	return strings.Contains(accept, "application/json") || r.URL.Query().Get("format") == "json"
}

func formatPublicHealthText(health publicHealthResponse) string {
	var out bytes.Buffer

	fmt.Fprintf(&out, "# 1. Project\n")
	fmt.Fprintf(&out, "project.name: %s\n", health.Project.Name)
	if health.Project.Tagline != "" {
		fmt.Fprintf(&out, "project.tagline: %s\n", health.Project.Tagline)
	}
	fmt.Fprintf(&out, "project.description: %s\n\n", health.Project.Description)

	fmt.Fprintf(&out, "# 2. Status\n")
	fmt.Fprintf(&out, "status: %s\n\n", health.Status)

	fmt.Fprintf(&out, "# 3. Version & Build\n")
	fmt.Fprintf(&out, "version: %s\n", health.Version)
	fmt.Fprintf(&out, "go_version: %s\n", health.GoVersion)
	fmt.Fprintf(&out, "build.commit: %s\n", health.Build.Commit)
	fmt.Fprintf(&out, "build.date: %s\n\n", health.Build.Date)

	fmt.Fprintf(&out, "# 4. Runtime\n")
	fmt.Fprintf(&out, "uptime: %s\n", health.Uptime)
	fmt.Fprintf(&out, "mode: %s\n", health.Mode)
	fmt.Fprintf(&out, "timestamp: %s\n", health.Timestamp)
	if health.Maintenance != nil {
		fmt.Fprintf(&out, "maintenance.reason: %s\n", health.Maintenance.Reason)
		fmt.Fprintf(&out, "maintenance.message: %s\n", health.Maintenance.Message)
	}
	fmt.Fprintf(&out, "\n")

	fmt.Fprintf(&out, "# 5. Cluster\n")
	fmt.Fprintf(&out, "cluster.enabled: %t\n", health.Cluster.Enabled)
	if health.Cluster.Status != "" {
		fmt.Fprintf(&out, "cluster.status: %s\n", health.Cluster.Status)
	}
	fmt.Fprintf(&out, "cluster.primary: %s\n", health.Cluster.Primary)
	fmt.Fprintf(&out, "cluster.nodes: %s\n", strings.Join(health.Cluster.Nodes, ", "))
	if health.Cluster.NodeCount > 0 {
		fmt.Fprintf(&out, "cluster.node_count: %d\n", health.Cluster.NodeCount)
	}
	if health.Cluster.Role != "" {
		fmt.Fprintf(&out, "cluster.role: %s\n", health.Cluster.Role)
	}
	fmt.Fprintf(&out, "\n")

	fmt.Fprintf(&out, "# 6. Features\n")
	fmt.Fprintf(&out, "features.multi_user: %t\n", health.Features.MultiUser)
	fmt.Fprintf(&out, "features.tor.enabled: %t\n", health.Features.Tor.Enabled)
	fmt.Fprintf(&out, "features.tor.running: %t\n", health.Features.Tor.Running)
	fmt.Fprintf(&out, "features.tor.status: %s\n", health.Features.Tor.Status)
	fmt.Fprintf(&out, "features.tor.hostname: %s\n", health.Features.Tor.Hostname)
	fmt.Fprintf(&out, "features.i2p.enabled: %t\n", health.Features.I2P.Enabled)
	fmt.Fprintf(&out, "features.i2p.running: %t\n", health.Features.I2P.Running)
	fmt.Fprintf(&out, "features.i2p.status: %s\n", health.Features.I2P.Status)
	fmt.Fprintf(&out, "features.i2p.hostname: %s\n", health.Features.I2P.Hostname)
	fmt.Fprintf(&out, "features.i2p.provider: %s\n", health.Features.I2P.Provider)
	fmt.Fprintf(&out, "features.geoip: %t\n\n", health.Features.GeoIP)

	fmt.Fprintf(&out, "# 7. Checks\n")
	fmt.Fprintf(&out, "checks.database: %s\n", health.Checks.Database)
	fmt.Fprintf(&out, "checks.cache: %s\n", health.Checks.Cache)
	fmt.Fprintf(&out, "checks.disk: %s\n", health.Checks.Disk)
	fmt.Fprintf(&out, "checks.scheduler: %s\n", health.Checks.Scheduler)
	if health.Checks.Cluster != "" {
		fmt.Fprintf(&out, "checks.cluster: %s\n", health.Checks.Cluster)
	}
	if health.Checks.I2P != "" {
		fmt.Fprintf(&out, "checks.i2p: %s\n", health.Checks.I2P)
	}
	if health.Checks.Tor != "" {
		fmt.Fprintf(&out, "checks.tor: %s\n", health.Checks.Tor)
	}
	fmt.Fprintf(&out, "\n")

	fmt.Fprintf(&out, "# 8. Stats\n")
	fmt.Fprintf(&out, "stats.requests_total: %d\n", health.Stats.RequestsTotal)
	fmt.Fprintf(&out, "stats.requests_24h: %d\n", health.Stats.Requests24H)
	fmt.Fprintf(&out, "stats.active_connections: %d\n", health.Stats.ActiveConnections)

	return out.String() + "\n"
}

func publicHealthStatusClass(status string) string {
	switch status {
	case "healthy":
		return "status-ok"
	case "degraded":
		return "status-warning"
	default:
		return "status-error"
	}
}

func publicHealthStatusText(status string) string {
	switch status {
	case "healthy":
		return "All Systems Operational"
	case "degraded":
		return "Service Degraded"
	case "maintenance":
		return "Maintenance Mode"
	default:
		return "Service Unavailable"
	}
}
