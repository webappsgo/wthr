package handler

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/webappsgo/wthr/src/common/dbtime"
	"github.com/webappsgo/wthr/src/database"
	models "github.com/webappsgo/wthr/src/server/model"
	"github.com/webappsgo/wthr/src/server/reqctx"
	"github.com/webappsgo/wthr/src/server/service"
)

// AdminSettingsHandler handles admin settings API
type AdminSettingsHandler struct {
	DB                  *sql.DB
	NotificationService *service.NotificationService
}

// GetAllSettings returns all settings
func (h *AdminSettingsHandler) GetAllSettings(w http.ResponseWriter, r *http.Request) {
	rows, err := database.QueryContext(context.Background(), database.GetServerDB(), database.TimeoutSimpleSelect, `
		SELECT key, value, type, COALESCE(description, '') as description
		FROM server_config
		ORDER BY key
	`)
	if err != nil {
		InternalError(w, r, "Failed to fetch settings")
		return
	}
	defer rows.Close()

	settings := make(map[string]interface{})
	categories := make(map[string][]map[string]interface{})

	for rows.Next() {
		var key, value, typ, description string
		if err := rows.Scan(&key, &value, &typ, &description); err != nil {
			continue
		}

		// Extract category from key prefix (e.g., "smtp.host" → "smtp")
		category := "other"
		if idx := len(key); idx > 0 {
			for i, ch := range key {
				if ch == '.' {
					category = key[:i]
					break
				}
			}
		}

		// Parse value based on type
		var parsedValue interface{}
		switch typ {
		case "boolean":
			parsedValue = value == "true"
		case "number":
			var num float64
			if err := json.Unmarshal([]byte(value), &num); err != nil {
				log.Printf("ERROR: GetAllSettings: failed to parse number setting %q: %v", key, err)
			}
			parsedValue = num
		case "json":
			if err := json.Unmarshal([]byte(value), &parsedValue); err != nil {
				log.Printf("ERROR: GetAllSettings: failed to parse json setting %q: %v", key, err)
			}
		default:
			parsedValue = value
		}

		settings[key] = parsedValue

		if categories[category] == nil {
			categories[category] = make([]map[string]interface{}, 0)
		}
		categories[category] = append(categories[category], map[string]interface{}{
			"key":         key,
			"value":       parsedValue,
			"type":        typ,
			"description": description,
		})
	}

	RespondSuccess(w, r, "", map[string]interface{}{
		"settings":   settings,
		"categories": categories,
	})
}

// UpdateSettings updates multiple settings at once
func (h *AdminSettingsHandler) UpdateSettings(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Settings map[string]interface{} `json:"settings"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		BadRequest(w, r, "Invalid request body")
		return
	}

	applied := make([]string, 0)
	failed := make(map[string]string)

	for key, value := range req.Settings {
		// Convert value to string for storage
		var valueStr string
		switch v := value.(type) {
		case bool:
			if v {
				valueStr = "true"
			} else {
				valueStr = "false"
			}
		case string:
			valueStr = v
		default:
			jsonBytes, _ := json.Marshal(v)
			valueStr = string(jsonBytes)
		}

		// Update in database.
		// updated_at is bound as canonical UTC text rather than produced by SQL's
		// CURRENT_TIMESTAMP, which yields a different type and zone on PostgreSQL,
		// MySQL and SQL Server than it does on SQLite.
		result, err := database.ExecContext(context.Background(), database.GetServerDB(), database.TimeoutWrite, `
			UPDATE server_config
			SET value = ?, updated_at = ?
			WHERE key = ?
		`, valueStr, dbtime.FormatSQLTimestamp(time.Now()), key)

		if err != nil {
			failed[key] = err.Error()
			continue
		}

		rowsAffected, _ := result.RowsAffected()
		if rowsAffected > 0 {
			applied = append(applied, key)
		} else {
			failed[key] = "Setting not found"
		}
	}

	// Send success notification to admin (AI.md PART 18 - WebUI Notifications)
	if h.NotificationService != nil && len(applied) > 0 {
		adminIDInterface, exists := reqctx.Get(r.Context(), "admin_id")
		if exists {
			adminID, ok := adminIDInterface.(int)
			if ok {
				// Send success toast notification
				message := fmt.Sprintf("Successfully updated %d setting(s)", len(applied))
				if len(failed) > 0 {
					message += fmt.Sprintf(", %d failed", len(failed))
				}

				_, _ = h.NotificationService.SendAdminNotification(
					adminID,
					models.NotificationTypeSuccess,
					models.NotificationDisplayToast,
					"Settings Updated",
					message,
					nil,
				)
			}
		}
	}

	RespondSuccess(w, r, "Settings applied successfully. Changes are live.", map[string]interface{}{
		"applied": applied,
		"failed":  failed,
		// All settings apply live
		"requires_restart": []string{},
	})
}

// ResetSettings resets all settings to defaults
func (h *AdminSettingsHandler) ResetSettings(w http.ResponseWriter, r *http.Request) {
	settingsModel := &models.SettingsModel{DB: database.GetServerDB()}
	backupPath := settingsModel.GetString("backup.location", "/data/backups")

	tx, err := database.GetServerDB().Begin()
	if err != nil {
		InternalError(w, r, "Failed to start settings reset")
		return
	}
	defer tx.Rollback()

	txCtx, txCancel := database.WithTimeout(database.TimeoutWrite)
	defer txCancel()

	if _, err := tx.ExecContext(txCtx, "DELETE FROM server_config"); err != nil {
		InternalError(w, r, "Failed to clear settings")
		return
	}

	if err := tx.Commit(); err != nil {
		InternalError(w, r, "Failed to commit settings reset")
		return
	}

	if err := settingsModel.InitializeDefaults(backupPath); err != nil {
		InternalError(w, r, "Failed to restore default settings")
		return
	}

	RespondSuccess(w, r, "Settings reset to defaults")
}

// ExportSettings exports configuration as JSON
func (h *AdminSettingsHandler) ExportSettings(w http.ResponseWriter, r *http.Request) {
	rows, err := database.QueryContext(context.Background(), database.GetServerDB(), database.TimeoutSimpleSelect, "SELECT key, value FROM server_config ORDER BY key")
	if err != nil {
		InternalError(w, r, "Failed to export settings")
		return
	}
	defer rows.Close()

	settings := make(map[string]string)
	for rows.Next() {
		var key, value string
		if err := rows.Scan(&key, &value); err == nil {
			settings[key] = value
		}
	}

	w.Header().Set("Content-Disposition", "attachment; filename=weather-settings.json")
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"version":     readVersion(),
		"exported_at": time.Now().UTC().Format(time.RFC3339),
		"settings":    settings,
	})
}

// ImportSettings imports configuration from JSON
func (h *AdminSettingsHandler) ImportSettings(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Settings map[string]string `json:"settings"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		BadRequest(w, r, "Invalid request body")
		return
	}

	imported := 0
	for key, value := range req.Settings {
		// updated_at is bound as canonical UTC text rather than produced by SQL's
		// CURRENT_TIMESTAMP, which yields a different type and zone on PostgreSQL,
		// MySQL and SQL Server than it does on SQLite.
		_, err := database.ExecContext(context.Background(), database.GetServerDB(), database.TimeoutWrite, `
			UPDATE server_config
			SET value = ?, updated_at = ?
			WHERE key = ?
		`, value, dbtime.FormatSQLTimestamp(time.Now()), key)

		if err == nil {
			imported++
		}
	}

	RespondSuccess(w, r, "Settings imported successfully", map[string]interface{}{
		"imported": imported,
		"total":    len(req.Settings),
	})
}

// ReloadConfig triggers a server configuration reload (similar to SIGHUP)
func (h *AdminSettingsHandler) ReloadConfig(w http.ResponseWriter, r *http.Request) {
	// Settings are stored in the database and are already live-reloaded on every
	// request via SettingsModel, so no file I/O is needed here.

	RespondSuccess(w, r, "Configuration reload triggered", map[string]interface{}{
		"note": "Settings are live-reloaded from database automatically",
	})
}
