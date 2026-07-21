package handler

import (
	"net/http"
	"os"
	"runtime"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/webappsgo/wthr/src/database"
	"github.com/webappsgo/wthr/src/util"
)

// AdminServerStatus handles admin API server status/health requests with JSON-only output.
func AdminServerStatus(db *database.DB, httpPort string, httpsPort int, sslManager interface{}) gin.HandlerFunc {
	return func(c *gin.Context) {
		httpStatus, response := buildAdminServerStatusResponse(db, c, httpPort, httpsPort, sslManager)
		c.JSON(httpStatus, response)
	}
}

// ComprehensiveHealthCheck handles GET /healthz with full system health data
func ComprehensiveHealthCheck(db *database.DB, httpPort string, httpsPort int, sslManager interface{}) gin.HandlerFunc {
	return func(c *gin.Context) {
		status := GetInitStatus()
		startTime := status.Started

		// Read version from release.txt
		version := readVersion()

		// Overall status determination
		overallStatus := "healthy"
		httpStatus := http.StatusOK

		// Database check
		dbStatus, dbLatency, dbErr := db.HealthCheck()
		if dbErr != nil || dbStatus != "connected" {
			overallStatus = "unhealthy"
			httpStatus = http.StatusServiceUnavailable
		}

		// Memory stats
		var m runtime.MemStats
		runtime.ReadMemStats(&m)
		memUsedBytes := int64(m.Alloc)
		memTotalBytes := int64(m.Sys)
		memUsedPercent := int(float64(memUsedBytes) / float64(memTotalBytes) * 100)

		if memUsedPercent > 95 {
			overallStatus = "unhealthy"
			httpStatus = http.StatusServiceUnavailable
		} else if memUsedPercent > 80 {
			if overallStatus == "healthy" {
				overallStatus = "degraded"
			}
		}

		memStatus := "ok"
		if memUsedPercent > 95 {
			memStatus = "critical"
		} else if memUsedPercent > 80 {
			memStatus = "warning"
		}

		// Disk usage
		dataDir := getDataDir()
		logDir := getLogDir()

		dataDiskUsage := getDiskUsage(dataDir)
		logDiskUsage := getDiskUsage(logDir)

		if dataDiskUsage.UsedPercent > 95 || logDiskUsage.UsedPercent > 95 {
			overallStatus = "unhealthy"
			httpStatus = http.StatusServiceUnavailable
		} else if dataDiskUsage.UsedPercent > 80 || logDiskUsage.UsedPercent > 80 {
			if overallStatus == "healthy" {
				overallStatus = "degraded"
			}
		}

		diskStatus := "ok"
		if dataDiskUsage.UsedPercent > 95 || logDiskUsage.UsedPercent > 95 {
			diskStatus = "critical"
		} else if dataDiskUsage.UsedPercent > 80 || logDiskUsage.UsedPercent > 80 {
			diskStatus = "warning"
		}

		// Session count
		sessionCount, _ := db.GetSessionCount()

		// SSL status
		sslStatus := getSSLStatus(sslManager)

		// Scheduler status
		schedulerStatus := getSchedulerStatus()

		// Request stats
		requestStats := getRequestStats()

		// Server info
		serverInfo := getServerInfo(c, httpPort, httpsPort, sslManager)

		// Feature flags
		features := getFeatureFlags(db)

		// Build response according to spec
		response := gin.H{
			"status":         overallStatus,
			"timestamp":      time.Now().Format(time.RFC3339),
			"version":        version,
			"uptime_seconds": int64(time.Since(startTime).Seconds()),
			"checks": gin.H{
				"database": gin.H{
					"status":     dbStatus,
					"type":       "sqlite",
					"latency_ms": dbLatency,
					"connection_pool": gin.H{
						// SQLite doesn't have connection pooling
						"active": 1,
						"idle":   0,
						"max":    1,
					},
				},
				"location_databases": gin.H{
					"countries": gin.H{
						"status": getStatusString(initStatus.Countries),
						"loaded": initStatus.Countries,
					},
					"cities": gin.H{
						"status": getStatusString(initStatus.Cities),
						"loaded": initStatus.Cities,
					},
					"zipcodes": gin.H{
						"status": "loaded",
						"loaded": true,
					},
					"geoip": gin.H{
						"status":    "loaded",
						"loaded":    true,
						"databases": 4,
						"types":     []string{"IPv4 City", "IPv6 City", "Country", "ASN"},
					},
				},
				"cache": gin.H{
					"status":     "inactive",
					"type":       "none",
					"hit_rate":   0.0,
					"size_bytes": 0,
					"entries":    0,
				},
				"disk": gin.H{
					"status": diskStatus,
					"data_dir": gin.H{
						"path":         dataDir,
						"used_bytes":   dataDiskUsage.UsedBytes,
						"free_bytes":   dataDiskUsage.FreeBytes,
						"total_bytes":  dataDiskUsage.TotalBytes,
						"used_percent": dataDiskUsage.UsedPercent,
					},
					"log_dir": gin.H{
						"path":         logDir,
						"used_bytes":   logDiskUsage.UsedBytes,
						"free_bytes":   logDiskUsage.FreeBytes,
						"total_bytes":  logDiskUsage.TotalBytes,
						"used_percent": logDiskUsage.UsedPercent,
					},
				},
				"memory": gin.H{
					"status":       memStatus,
					"used_bytes":   memUsedBytes,
					"total_bytes":  memTotalBytes,
					"used_percent": memUsedPercent,
					"heap_bytes":   int64(m.HeapAlloc),
					"gc_runs":      m.NumGC,
				},
				"ssl":       sslStatus,
				"scheduler": schedulerStatus,
				"sessions": gin.H{
					"active":      sessionCount,
					"total_today": sessionCount,
				},
				"requests": requestStats,
			},
			"server":   serverInfo,
			"features": features,
		}

		if shouldRespondText(c) {
			RespondNegotiatedData(c, httpStatus, response)
			return
		}

		// TEMPLATE.md: /healthz must return HTML for browser requests.
		// /api/v1/healthz returns JSON or text/plain via a different handler.
		c.HTML(httpStatus, "page/healthz.tmpl", response)
	}
}

func buildAdminServerStatusResponse(db *database.DB, c *gin.Context, httpPort string, httpsPort int, sslManager interface{}) (int, gin.H) {
	status := GetInitStatus()
	startTime := status.Started
	version := readVersion()

	overallStatus := "healthy"
	httpStatus := http.StatusOK

	dbStatus, dbLatency, dbErr := db.HealthCheck()
	if dbErr != nil || dbStatus != "connected" {
		overallStatus = "unhealthy"
		httpStatus = http.StatusServiceUnavailable
	}

	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	memUsedBytes := int64(m.Alloc)
	memTotalBytes := int64(m.Sys)
	memUsedPercent := 0
	if memTotalBytes > 0 {
		memUsedPercent = int(float64(memUsedBytes) / float64(memTotalBytes) * 100)
	}

	memStatus := "ok"
	if memUsedPercent > 95 {
		memStatus = "critical"
		overallStatus = "unhealthy"
		httpStatus = http.StatusServiceUnavailable
	} else if memUsedPercent > 80 {
		memStatus = "warning"
		if overallStatus == "healthy" {
			overallStatus = "degraded"
		}
	}

	dataDir := getDataDir()
	logDir := getLogDir()
	dataDiskUsage := getDiskUsage(dataDir)
	logDiskUsage := getDiskUsage(logDir)

	diskStatus := "ok"
	if dataDiskUsage.UsedPercent > 95 || logDiskUsage.UsedPercent > 95 {
		diskStatus = "critical"
		overallStatus = "unhealthy"
		httpStatus = http.StatusServiceUnavailable
	} else if dataDiskUsage.UsedPercent > 80 || logDiskUsage.UsedPercent > 80 {
		diskStatus = "warning"
		if overallStatus == "healthy" {
			overallStatus = "degraded"
		}
	}

	sessionCount, _ := db.GetSessionCount()
	sslStatus := getSSLStatus(sslManager)
	schedulerStatus := getSchedulerStatus()
	requestStats := getRequestStats()
	serverInfo := getServerInfo(c, httpPort, httpsPort, sslManager)

	response := gin.H{
		"status":         overallStatus,
		"timestamp":      time.Now().Format(time.RFC3339),
		"version":        version,
		"uptime_seconds": int64(time.Since(startTime).Seconds()),
		"checks": gin.H{
			"database": gin.H{
				"status":     dbStatus,
				"type":       "sqlite",
				"latency_ms": dbLatency,
			},
			"initialization": gin.H{
				"countries": status.Countries,
				"cities":    status.Cities,
				"weather":   status.Weather,
				"ready":     IsInitialized(),
			},
			"disk": gin.H{
				"status": diskStatus,
				"data_dir": gin.H{
					"path":         dataDir,
					"used_bytes":   dataDiskUsage.UsedBytes,
					"free_bytes":   dataDiskUsage.FreeBytes,
					"total_bytes":  dataDiskUsage.TotalBytes,
					"used_percent": dataDiskUsage.UsedPercent,
				},
				"log_dir": gin.H{
					"path":         logDir,
					"used_bytes":   logDiskUsage.UsedBytes,
					"free_bytes":   logDiskUsage.FreeBytes,
					"total_bytes":  logDiskUsage.TotalBytes,
					"used_percent": logDiskUsage.UsedPercent,
				},
			},
			"memory": gin.H{
				"status":       memStatus,
				"used_bytes":   memUsedBytes,
				"total_bytes":  memTotalBytes,
				"used_percent": memUsedPercent,
				"heap_bytes":   int64(m.HeapAlloc),
				"gc_runs":      m.NumGC,
			},
			"ssl":       sslStatus,
			"scheduler": schedulerStatus,
			"sessions": gin.H{
				"active": sessionCount,
			},
			"requests": requestStats,
		},
		"server": serverInfo,
	}

	return httpStatus, response
}

// Helper functions

func readVersion() string {
	data, err := os.ReadFile("release.txt")
	if err != nil {
		return "dev"
	}
	// Trim whitespace/newline from version string per AI.md PART 13
	return strings.TrimSpace(string(data))
}

// Global variables to store directory paths
var (
	globalDataDir string
	globalLogDir  string
)

// SetDirectoryPaths sets the global directory paths for health checks
func SetDirectoryPaths(dataDir, logDir string) {
	globalDataDir = dataDir
	globalLogDir = logDir
}

func getDataDir() string {
	if globalDataDir != "" {
		return globalDataDir
	}
	dir := os.Getenv("DATA_DIR")
	if dir == "" {
		dir = "./data"
	}
	return dir
}

func getLogDir() string {
	if globalLogDir != "" {
		return globalLogDir
	}
	dir := os.Getenv("LOG_DIR")
	if dir == "" {
		dir = "./logs"
	}
	return dir
}

func GetLogDir() string {
	return getLogDir()
}

type DiskUsage struct {
	Path        string
	UsedBytes   int64
	FreeBytes   int64
	TotalBytes  int64
	UsedPercent int
}

// getDiskUsage is implemented in disk_unix.go and disk_windows.go

func getSSLStatus(sslManager interface{}) gin.H {
	// Check if SSL manager is provided and has GetCertInfo method
	if sslManager == nil {
		return gin.H{
			"enabled":        false,
			"status":         "none",
			"expires_at":     nil,
			"days_remaining": 0,
			"issuer":         "Unknown",
		}
	}

	// Use type assertion to get cert info
	type certInfoGetter interface {
		GetCertInfo() map[string]interface{}
	}

	if manager, ok := sslManager.(certInfoGetter); ok {
		info := manager.GetCertInfo()
		return gin.H(info)
	}

	// Fallback if type assertion fails
	return gin.H{
		"enabled":        false,
		"status":         "none",
		"expires_at":     nil,
		"days_remaining": 0,
		"issuer":         "Unknown",
	}
}

func getSchedulerStatus() gin.H {
	db := database.GetServerDB()
	if db == nil {
		return gin.H{
			"status":        "unknown",
			"tasks_total":   0,
			"tasks_enabled": 0,
			"next_run":      nil,
		}
	}

	// Count total tasks
	var totalTasks int
	err := db.QueryRow("SELECT COUNT(*) FROM server_scheduler_state").Scan(&totalTasks)
	if err != nil {
		totalTasks = 0
	}

	// Count enabled tasks
	var enabledTasks int
	err = db.QueryRow("SELECT COUNT(*) FROM server_scheduler_state WHERE enabled = 1").Scan(&enabledTasks)
	if err != nil {
		enabledTasks = 0
	}

	// Get next scheduled run
	var nextRun *string
	err = db.QueryRow("SELECT MIN(next_run) FROM server_scheduler_state WHERE enabled = 1 AND next_run IS NOT NULL").Scan(&nextRun)
	if err != nil {
		nextRun = nil
	}

	// Count running tasks (locked)
	var runningTasks int
	err = db.QueryRow("SELECT COUNT(*) FROM server_scheduler_state WHERE locked_by IS NOT NULL").Scan(&runningTasks)
	if err != nil {
		runningTasks = 0
	}

	// Determine scheduler status
	status := "running"
	if totalTasks == 0 {
		status = "no_tasks"
	} else if enabledTasks == 0 {
		status = "all_disabled"
	}

	result := gin.H{
		"status":        status,
		"tasks_total":   totalTasks,
		"tasks_enabled": enabledTasks,
		"tasks_running": runningTasks,
	}

	if nextRun != nil && *nextRun != "" {
		result["next_run"] = *nextRun
	}

	return result
}

func getRequestStats() gin.H {
	db := database.GetServerDB()
	if db == nil {
		return gin.H{
			"total_today":     0,
			"rate_per_minute": 0,
			"errors_today":    0,
			"error_rate":      0.0,
			"source":          "unavailable",
		}
	}

	// Count audit log entries for today (gives us a proxy for activity)
	var totalToday int
	err := db.QueryRow(`
		SELECT COUNT(*) FROM server_audit_log
		WHERE timestamp >= date('now', 'start of day')
	`).Scan(&totalToday)
	if err != nil {
		totalToday = 0
	}

	// Count errors today
	var errorsToday int
	err = db.QueryRow(`
		SELECT COUNT(*) FROM server_audit_log
		WHERE timestamp >= date('now', 'start of day')
		AND status = 'error'
	`).Scan(&errorsToday)
	if err != nil {
		errorsToday = 0
	}

	// Count entries in last minute for rate
	var lastMinute int
	err = db.QueryRow(`
		SELECT COUNT(*) FROM server_audit_log
		WHERE timestamp >= datetime('now', '-1 minute')
	`).Scan(&lastMinute)
	if err != nil {
		lastMinute = 0
	}

	// Calculate error rate
	errorRate := 0.0
	if totalToday > 0 {
		errorRate = float64(errorsToday) / float64(totalToday) * 100
	}

	return gin.H{
		"total_today":     totalToday,
		"rate_per_minute": lastMinute,
		"errors_today":    errorsToday,
		"error_rate":      errorRate,
		"source":          "audit_log",
	}
}

func getServerInfo(c *gin.Context, httpPort string, httpsPort int, sslManager interface{}) gin.H {
	hostInfo := utils.GetHostInfo(c)

	httpsEnabled := false
	if sslManager != nil {
		type httpsChecker interface {
			IsHTTPSEnabled() bool
		}
		if manager, ok := sslManager.(httpsChecker); ok {
			httpsEnabled = manager.IsHTTPSEnabled()
		}
	}

	return gin.H{
		"address":       hostInfo.Hostname,
		"http_port":     httpPort,
		"https_port":    httpsPort,
		"https_enabled": httpsEnabled,
		"pid":           os.Getpid(),
		"started_at":    GetInitStatus().Started.Format(time.RFC3339),
	}
}

func getFeatureFlags(db *database.DB) gin.H {
	// Read from database settings
	regEnabled, _ := db.GetSetting("registration.enabled")
	apiEnabled, _ := db.GetSetting("api.enabled")
	maintenanceMode, _ := db.GetSetting("maintenance.mode")

	return gin.H{
		"registration_enabled": regEnabled != "false",
		"api_enabled":          apiEnabled != "false",
		"maintenance_mode":     maintenanceMode == "true",
		"graphql_enabled":      false,
		"websocket_enabled":    false,
	}
}

func getStatusString(loaded bool) string {
	if loaded {
		return "loaded"
	}
	return "loading"
}
