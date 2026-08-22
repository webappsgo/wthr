package handler

import (
	"context"
	"net/http"
	"os"
	"runtime"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/webappsgo/wthr/src/common/dbtime"
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

// readVersion returns the application version resolved at build time via
// ldflags (-X main.Version), propagated into this package by
// handler.SetBuildInfo at startup. It never reads a file at runtime, keeping
// the binary self-contained per AI.md PART 1/8; the package-level Version
// defaults to "dev" when no ldflags value was injected.
func readVersion() string {
	return Version
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
	err := database.QueryRowContext(context.Background(), db, database.TimeoutSimpleSelect, "SELECT COUNT(*) FROM server_scheduler_state").Scan(&totalTasks)
	if err != nil {
		totalTasks = 0
	}

	// Count enabled tasks
	var enabledTasks int
	err = database.QueryRowContext(context.Background(), db, database.TimeoutSimpleSelect, "SELECT COUNT(*) FROM server_scheduler_state WHERE enabled = 1").Scan(&enabledTasks)
	if err != nil {
		enabledTasks = 0
	}

	// Get next scheduled run
	var nextRun *string
	err = database.QueryRowContext(context.Background(), db, database.TimeoutSimpleSelect, "SELECT MIN(next_run) FROM server_scheduler_state WHERE enabled = 1 AND next_run IS NOT NULL").Scan(&nextRun)
	if err != nil {
		nextRun = nil
	}

	// Count running tasks (locked)
	var runningTasks int
	err = database.QueryRowContext(context.Background(), db, database.TimeoutSimpleSelect, "SELECT COUNT(*) FROM server_scheduler_state WHERE locked_by IS NOT NULL").Scan(&runningTasks)
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

	// Today's audit-log activity, today's errors and the last minute's rate are
	// counted in one pass with the cutoffs applied in Go. SQLite's
	// date('now', 'start of day') and datetime('now', '-1 minute') return NULL
	// for the local-zone layout the driver writes for a bound time.Time, so
	// every such row silently dropped out of all three counts, and neither
	// function exists on PostgreSQL or MySQL.
	var totalToday, errorsToday, lastMinute int

	startOfDay := time.Now().UTC().Truncate(24 * time.Hour)
	lastMinuteCutoff := time.Now().UTC().Add(-time.Minute)

	rows, err := database.QueryContext(context.Background(), db, database.TimeoutSimpleSelect, `
		SELECT status, timestamp
		FROM server_audit_log
		WHERE timestamp IS NOT NULL
	`)
	if err == nil {
		defer rows.Close()

		for rows.Next() {
			var status string
			var storedTimestamp interface{}
			if scanErr := rows.Scan(&status, &storedTimestamp); scanErr != nil {
				break
			}

			entryTime, ok := dbtime.ParseStoredTimestamp(storedTimestamp)
			if !ok {
				continue
			}

			if !entryTime.Before(startOfDay) {
				totalToday++
				if status == "error" {
					errorsToday++
				}
			}

			if !entryTime.Before(lastMinuteCutoff) {
				lastMinute++
			}
		}
		if rowsErr := rows.Err(); rowsErr != nil {
			totalToday, errorsToday, lastMinute = 0, 0, 0
		}
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
	hostInfo := util.GetHostInfo(c)

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
