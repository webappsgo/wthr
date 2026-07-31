package handler

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/webappsgo/wthr/src/database"
	models "github.com/webappsgo/wthr/src/server/model"
	"github.com/webappsgo/wthr/src/server/service"

	"github.com/gin-gonic/gin"
)

// AdminSettingsHandler handles admin settings API
type AdminSettingsHandler struct {
	DB                  *sql.DB
	NotificationService *service.NotificationService
}

// GetAllSettings returns all settings
func (h *AdminSettingsHandler) GetAllSettings(c *gin.Context) {
	rows, err := database.GetServerDB().Query(`
		SELECT key, value, type, COALESCE(description, '') as description
		FROM server_config
		ORDER BY key
	`)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Failed to fetch settings",
		})
		return
	}
	defer rows.Close()

	settings := make(map[string]interface{})
	categories := make(map[string][]gin.H)

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
			json.Unmarshal([]byte(value), &num)
			parsedValue = num
		case "json":
			json.Unmarshal([]byte(value), &parsedValue)
		default:
			parsedValue = value
		}

		settings[key] = parsedValue

		if categories[category] == nil {
			categories[category] = make([]gin.H, 0)
		}
		categories[category] = append(categories[category], gin.H{
			"key":         key,
			"value":       parsedValue,
			"type":        typ,
			"description": description,
		})
	}

	c.JSON(http.StatusOK, gin.H{
		"settings":   settings,
		"categories": categories,
	})
}

// UpdateSettings updates multiple settings at once
func (h *AdminSettingsHandler) UpdateSettings(c *gin.Context) {
	var req struct {
		Settings map[string]interface{} `json:"settings"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Invalid request body",
		})
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

		// Update in database
		result, err := database.GetServerDB().Exec(`
			UPDATE server_config
			SET value = ?, updated_at = CURRENT_TIMESTAMP
			WHERE key = ?
		`, valueStr, key)

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

	// Send success notification to admin (TEMPLATE.md Part 25 - WebUI Notifications)
	if h.NotificationService != nil && len(applied) > 0 {
		adminIDInterface, exists := c.Get("admin_id")
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

	c.JSON(http.StatusOK, gin.H{
		"applied":          applied,
		"failed":           failed,
		// All settings apply live
		"requires_restart": []string{},
		"message":          "Settings applied successfully. Changes are live.",
	})
}

// ResetSettings resets all settings to defaults
func (h *AdminSettingsHandler) ResetSettings(c *gin.Context) {
	settingsModel := &models.SettingsModel{DB: database.GetServerDB()}
	backupPath := settingsModel.GetString("backup.location", "/data/backups")

	tx, err := database.GetServerDB().Begin()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Failed to start settings reset",
		})
		return
	}
	defer tx.Rollback()

	if _, err := tx.Exec("DELETE FROM server_config"); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Failed to clear settings",
		})
		return
	}

	if err := tx.Commit(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Failed to commit settings reset",
		})
		return
	}

	if err := settingsModel.InitializeDefaults(backupPath); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Failed to restore default settings",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Settings reset to defaults",
	})
}

// ExportSettings exports configuration as JSON
func (h *AdminSettingsHandler) ExportSettings(c *gin.Context) {
	rows, err := database.GetServerDB().Query("SELECT key, value FROM server_config ORDER BY key")
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Failed to export settings",
		})
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

	c.Header("Content-Disposition", "attachment; filename=weather-settings.json")
	c.JSON(http.StatusOK, gin.H{
		"version":     readVersion(),
		"exported_at": time.Now().UTC().Format(time.RFC3339),
		"settings":    settings,
	})
}

// ImportSettings imports configuration from JSON
func (h *AdminSettingsHandler) ImportSettings(c *gin.Context) {
	var req struct {
		Settings map[string]string `json:"settings"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Invalid request body",
		})
		return
	}

	imported := 0
	for key, value := range req.Settings {
		_, err := database.GetServerDB().Exec(`
			UPDATE server_config
			SET value = ?, updated_at = CURRENT_TIMESTAMP
			WHERE key = ?
		`, value, key)

		if err == nil {
			imported++
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"imported": imported,
		"total":    len(req.Settings),
		"message":  "Settings imported successfully",
	})
}

// ReloadConfig triggers a server configuration reload (similar to SIGHUP)
func (h *AdminSettingsHandler) ReloadConfig(c *gin.Context) {
	// Settings are stored in the database and are already live-reloaded on every
	// request via SettingsModel, so no file I/O is needed here.

	c.JSON(http.StatusOK, gin.H{
		"message": "Configuration reload triggered",
		"note":    "Settings are live-reloaded from database automatically",
	})
}
