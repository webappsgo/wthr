package handler

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"net/http"
	"runtime"
	"strings"

	"github.com/webappsgo/wthr/src/config"
	"github.com/webappsgo/wthr/src/database"

	"github.com/go-chi/chi/v5"
)

// DebugHandlers provides debug endpoints when DEBUG mode is enabled
type DebugHandlers struct {
	db     *sql.DB
	router chi.Router
}

// NewDebugHandlers creates debug endpoint handlers
func NewDebugHandlers(db *sql.DB, router chi.Router) *DebugHandlers {
	return &DebugHandlers{
		db:     db,
		router: router,
	}
}

// RegisterDebugRoutes registers all debug endpoints
func (h *DebugHandlers) RegisterDebugRoutes(debug chi.Router) {
	debug.Get("/routes", h.ListRoutes)
	debug.Get("/config", h.ShowConfig)
	debug.Get("/memory", h.ShowMemory)
	debug.Get("/db", h.ShowDatabase)
	debug.Post("/reload", h.ReloadConfig)
	debug.Post("/gc", h.TriggerGC)
}

// ListRoutes shows all registered routes
func (h *DebugHandlers) ListRoutes(w http.ResponseWriter, r *http.Request) {
	routeList := make([]map[string]interface{}, 0)
	_ = chi.Walk(h.router, func(method, path string, handler http.Handler, middlewares ...func(http.Handler) http.Handler) error {
		routeList = append(routeList, map[string]interface{}{
			"method":  method,
			"path":    path,
			"handler": fmt.Sprintf("%T", handler),
		})
		return nil
	})

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"routes": routeList,
		"count":  len(routeList),
	})
}

// ShowConfig shows current configuration
func (h *DebugHandlers) ShowConfig(w http.ResponseWriter, r *http.Request) {
	settings, err := config.ReadAllSettings()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]interface{}{
			"error": "Failed to query settings",
		})
		return
	}
	categories := make(map[string][]map[string]interface{})
	for key, value := range settings {
		parts := strings.Split(key, ".")
		category := parts[0]
		typ := "string"
		switch value.(type) {
		case bool:
			typ = "boolean"
		case int, int64, float64:
			typ = "number"
		case map[string]interface{}, []interface{}:
			typ = "json"
		}
		if categories[category] == nil {
			categories[category] = make([]map[string]interface{}, 0)
		}
		categories[category] = append(categories[category], map[string]interface{}{
			"key":   key,
			"value": value,
			"type":  typ,
		})
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"settings":   settings,
		"categories": categories,
		"count":      len(settings),
	})
}

// ShowMemory shows memory usage statistics
func (h *DebugHandlers) ShowMemory(w http.ResponseWriter, r *http.Request) {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	writeJSON(w, http.StatusOK, map[string]interface{}{
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
func (h *DebugHandlers) ShowDatabase(w http.ResponseWriter, r *http.Request) {
	stats := database.GetServerDB().Stats()

	// Count tables
	var tableCount int
	// Get table names and row counts
	tables := make([]map[string]interface{}, 0)
	tableQuery := "SELECT name FROM sqlite_master WHERE type='table' ORDER BY name"
	if database.IsRemoteDatabase(database.GetServerDB()) {
		if database.RemoteDatabaseDialect(database.GetServerDB()) == "mysql" || database.RemoteDatabaseDialect(database.GetServerDB()) == "mariadb" {
			tableQuery = `SELECT table_name FROM information_schema.tables WHERE table_schema = DATABASE() AND (table_name LIKE 'srv!_%' ESCAPE '!' OR table_name LIKE 'usr!_%' ESCAPE '!') ORDER BY table_name`
		} else {
			tableQuery = `SELECT tablename FROM pg_tables WHERE schemaname = current_schema() AND (tablename LIKE 'srv!_%' ESCAPE '!' OR tablename LIKE 'usr!_%' ESCAPE '!') ORDER BY tablename`
		}
	}
	rows, err := database.QueryContext(context.Background(), database.GetServerDB(), database.TimeoutSimpleSelect, tableQuery)
	if err == nil {
		defer func() {
			if err := rows.Close(); err != nil {
				log.Printf("WARNING: failed to close database table rows: %v", err)
			}
		}()
		for rows.Next() {
			var tableName string
			if err := rows.Scan(&tableName); err == nil {
				tableCount++
				var rowCount int
				query := fmt.Sprintf("SELECT COUNT(*) FROM \"%s\"", tableName)
				if database.RemoteDatabaseDialect(database.GetServerDB()) == "mysql" || database.RemoteDatabaseDialect(database.GetServerDB()) == "mariadb" {
					query = fmt.Sprintf("SELECT COUNT(*) FROM `%s`", tableName)
				}
				if err := database.QueryRowContext(context.Background(), database.GetServerDB(), database.TimeoutSimpleSelect, query).Scan(&rowCount); err != nil {
					log.Printf("ERROR: ShowDatabase: failed to count rows in table %s: %v", tableName, err)
				}

				tables = append(tables, map[string]interface{}{
					"name": tableName,
					"rows": rowCount,
				})
			}
		}
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"connection_stats": map[string]interface{}{
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
func (h *DebugHandlers) ReloadConfig(w http.ResponseWriter, r *http.Request) {
	// This would trigger SIGHUP internally
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"message": Translate(r, "success.admin.debug.configuration_reload_triggered"),
		"note":    "Send SIGHUP to process for full reload",
	})
}

// TriggerGC triggers garbage collection
func (h *DebugHandlers) TriggerGC(w http.ResponseWriter, r *http.Request) {
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)

	runtime.GC()

	runtime.ReadMemStats(&after)

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"message":         Translate(r, "success.admin.debug.garbage_collection_triggered"),
		"before_alloc_mb": before.Alloc / 1024 / 1024,
		"after_alloc_mb":  after.Alloc / 1024 / 1024,
		"freed_mb":        (before.Alloc - after.Alloc) / 1024 / 1024,
	})
}
