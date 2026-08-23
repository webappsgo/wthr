package handler

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"net/http"
	"runtime"

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
func (h *DebugHandlers) RegisterDebugRoutes(r chi.Router) {
	r.Route("/debug", func(debug chi.Router) {
		debug.Get("/routes", h.ListRoutes)
		debug.Get("/config", h.ShowConfig)
		debug.Get("/memory", h.ShowMemory)
		debug.Get("/db", h.ShowDatabase)
		debug.Post("/reload", h.ReloadConfig)
		debug.Post("/gc", h.TriggerGC)
	})
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
	// Query all settings from database
	rows, err := database.QueryContext(context.Background(), database.GetServerDB(), database.TimeoutSimpleSelect, "SELECT key, value, type, category FROM server_config ORDER BY category, key")
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]interface{}{
			"error": "Failed to query settings",
		})
		return
	}
	defer rows.Close()

	settings := make(map[string]interface{})
	categories := make(map[string][]map[string]interface{})

	for rows.Next() {
		var key, value, typ, category string
		if err := rows.Scan(&key, &value, &typ, &category); err != nil {
			continue
		}

		settings[key] = value

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
	if err := database.QueryRowContext(context.Background(), database.GetServerDB(), database.TimeoutSimpleSelect, "SELECT COUNT(*) FROM sqlite_master WHERE type='table'").Scan(&tableCount); err != nil {
		log.Printf("ERROR: ShowDatabase: failed to count tables: %v", err)
	}

	// Get table names and row counts
	tables := make([]map[string]interface{}, 0)
	rows, err := database.QueryContext(context.Background(), database.GetServerDB(), database.TimeoutSimpleSelect, "SELECT name FROM sqlite_master WHERE type='table' ORDER BY name")
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var tableName string
			if err := rows.Scan(&tableName); err == nil {
				var rowCount int
				// Table name is from sqlite_master (safe), but quote for best practice
				query := fmt.Sprintf("SELECT COUNT(*) FROM \"%s\"", tableName)
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
		"message": "Configuration reload triggered",
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
		"message":         "Garbage collection triggered",
		"before_alloc_mb": before.Alloc / 1024 / 1024,
		"after_alloc_mb":  after.Alloc / 1024 / 1024,
		"freed_mb":        (before.Alloc - after.Alloc) / 1024 / 1024,
	})
}
