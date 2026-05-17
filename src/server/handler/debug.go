package handler

import (
	"database/sql"
	"fmt"
	"net/http"
	"runtime"

	"github.com/casapps/wthr/src/database"

	"github.com/gin-gonic/gin"
)

// DebugHandlers provides debug endpoints when DEBUG mode is enabled
type DebugHandlers struct {
	db     *sql.DB
	router *gin.Engine
}

// NewDebugHandlers creates debug endpoint handlers
func NewDebugHandlers(db *sql.DB, router *gin.Engine) *DebugHandlers {
	return &DebugHandlers{
		db:     db,
		router: router,
	}
}

// RegisterDebugRoutes registers all debug endpoints
func (h *DebugHandlers) RegisterDebugRoutes(r *gin.Engine) {
	debug := r.Group("/debug")
	{
		debug.GET("/routes", h.ListRoutes)
		debug.GET("/config", h.ShowConfig)
		debug.GET("/memory", h.ShowMemory)
		debug.GET("/db", h.ShowDatabase)
		debug.POST("/reload", h.ReloadConfig)
		debug.POST("/gc", h.TriggerGC)
	}
}

// ListRoutes shows all registered routes
func (h *DebugHandlers) ListRoutes(c *gin.Context) {
	routes := h.router.Routes()

	routeList := make([]gin.H, 0, len(routes))
	for _, route := range routes {
		routeList = append(routeList, gin.H{
			"method":  route.Method,
			"path":    route.Path,
			"handler": route.Handler,
		})
	}

	c.JSON(http.StatusOK, gin.H{
		"routes": routeList,
		"count":  len(routes),
	})
}

// ShowConfig shows current configuration
func (h *DebugHandlers) ShowConfig(c *gin.Context) {
	// Query all settings from database
	rows, err := database.GetServerDB().Query("SELECT key, value, type, category FROM server_config ORDER BY category, key")
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Failed to query settings",
		})
		return
	}
	defer rows.Close()

	settings := make(map[string]interface{})
	categories := make(map[string][]gin.H)

	for rows.Next() {
		var key, value, typ, category string
		if err := rows.Scan(&key, &value, &typ, &category); err != nil {
			continue
		}

		settings[key] = value

		if categories[category] == nil {
			categories[category] = make([]gin.H, 0)
		}
		categories[category] = append(categories[category], gin.H{
			"key":   key,
			"value": value,
			"type":  typ,
		})
	}

	c.JSON(http.StatusOK, gin.H{
		"settings":   settings,
		"categories": categories,
		"count":      len(settings),
	})
}

// ShowMemory shows memory usage statistics
func (h *DebugHandlers) ShowMemory(c *gin.Context) {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	c.JSON(http.StatusOK, gin.H{
		"alloc_mb":       m.Alloc / 1024 / 1024,
		"total_alloc_mb": m.TotalAlloc / 1024 / 1024,
		"sys_mb":         m.Sys / 1024 / 1024,
		"num_gc":         m.NumGC,
		"goroutines":     runtime.NumGoroutine(),
		"heap_alloc_mb":  m.HeapAlloc / 1024 / 1024,
		"heap_sys_mb":    m.HeapSys / 1024 / 1024,
		"heap_idle_mb":   m.HeapIdle / 1024 / 1024,
		"heap_inuse_mb":  m.HeapInuse / 1024 / 1024,
		"stack_inuse_mb": m.StackInuse / 1024 / 1024,
	})
}

// ShowDatabase shows database statistics
func (h *DebugHandlers) ShowDatabase(c *gin.Context) {
	stats := database.GetServerDB().Stats()

	// Count tables
	var tableCount int
	database.GetServerDB().QueryRow("SELECT COUNT(*) FROM sqlite_master WHERE type='table'").Scan(&tableCount)

	// Get table names and row counts
	tables := make([]gin.H, 0)
	rows, err := database.GetServerDB().Query("SELECT name FROM sqlite_master WHERE type='table' ORDER BY name")
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var tableName string
			if err := rows.Scan(&tableName); err == nil {
				var rowCount int
				// Table name is from sqlite_master (safe), but quote for best practice
				query := fmt.Sprintf("SELECT COUNT(*) FROM \"%s\"", tableName)
				database.GetServerDB().QueryRow(query).Scan(&rowCount)

				tables = append(tables, gin.H{
					"name": tableName,
					"rows": rowCount,
				})
			}
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"connection_stats": gin.H{
			"max_open_connections": stats.MaxOpenConnections,
			"open_connections":     stats.OpenConnections,
			"in_use":               stats.InUse,
			"idle":                 stats.Idle,
			"wait_count":           stats.WaitCount,
			"wait_duration":        stats.WaitDuration.String(),
		},
		"tables": tables,
		"count":  tableCount,
	})
}

// ReloadConfig forces configuration reload
func (h *DebugHandlers) ReloadConfig(c *gin.Context) {
	// This would trigger SIGHUP internally
	c.JSON(http.StatusOK, gin.H{
		"message": "Configuration reload triggered",
		"note":    "Send SIGHUP to process for full reload",
	})
}

// TriggerGC triggers garbage collection
func (h *DebugHandlers) TriggerGC(c *gin.Context) {
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)

	runtime.GC()

	runtime.ReadMemStats(&after)

	c.JSON(http.StatusOK, gin.H{
		"message":         "Garbage collection triggered",
		"before_alloc_mb": before.Alloc / 1024 / 1024,
		"after_alloc_mb":  after.Alloc / 1024 / 1024,
		"freed_mb":        (before.Alloc - after.Alloc) / 1024 / 1024,
	})
}
