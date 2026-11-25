package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/webappsgo/wthr/src/database"

	"github.com/webappsgo/wthr/src/config"
	models "github.com/webappsgo/wthr/src/server/model"
	"github.com/webappsgo/wthr/src/server/reqctx"
	"github.com/webappsgo/wthr/src/server/service"
)

// AdminSettingsHandler handles admin settings API
type AdminSettingsHandler struct {
	DB                  *database.Store
	NotificationService *service.NotificationService
}

// GetAllSettings returns all settings
func (h *AdminSettingsHandler) GetAllSettings(w http.ResponseWriter, r *http.Request) {
	listed, err := (&models.SettingsModel{}).List()
	if err != nil {
		InternalError(w, r, "Failed to fetch settings")
		return
	}

	settings := make(map[string]interface{})
	categories := make(map[string][]map[string]interface{})
	for _, setting := range listed {
		parsedValue := adminSettingValue(setting)
		settings[setting.Key] = parsedValue
		category := setting.Category
		if category == "" {
			category = "other"
		}
		if categories[category] == nil {
			categories[category] = make([]map[string]interface{}, 0)
		}
		categories[category] = append(categories[category], map[string]interface{}{
			"key":         setting.Key,
			"value":       parsedValue,
			"type":        setting.Type,
			"description": setting.Description,
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
		// A blank secret means "keep the stored value"; secrets are never echoed to the form
		if value == "" && config.CanonicalConfigOwner(key) == "secrets.yml" {
			continue
		}
		if textValue, ok := value.(string); ok {
			if err := (&models.SettingsModel{}).SetString(key, textValue); err != nil {
				failed[key] = "update failed"
				continue
			}
			applied = append(applied, key)
			continue
		}
		encoded, err := json.Marshal(value)
		if err != nil {
			failed[key] = "invalid setting value"
			continue
		}
		settingType := "json"
		switch value.(type) {
		case bool:
			settingType = "boolean"
		case float64, int, int64:
			settingType = "number"
		}
		if err := (&models.SettingsModel{}).SetSetting(key, string(encoded), settingType); err != nil {
			failed[key] = "update failed"
			continue
		}
		applied = append(applied, key)
	}

	// Send success notification to admin (AI.md PART 18 - WebUI Notifications)
	if h.NotificationService != nil && len(applied) > 0 {
		adminIDInterface, exists := reqctx.GetValue(r.Context(), "admin_id")
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
	if err := config.ResetAreaSettings(); err != nil {
		InternalError(w, r, "Failed to restore default settings")
		return
	}

	RespondSuccess(w, r, "Settings reset to defaults")
}

// ExportSettings exports configuration as JSON
func (h *AdminSettingsHandler) ExportSettings(w http.ResponseWriter, r *http.Request) {
	listed, err := (&models.SettingsModel{}).List()
	if err != nil {
		InternalError(w, r, "Failed to export settings")
		return
	}
	settings := make(map[string]interface{}, len(listed))
	for _, setting := range listed {
		settings[setting.Key] = adminSettingValue(setting)
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
		Settings map[string]any `json:"settings"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		BadRequest(w, r, "Invalid request body")
		return
	}

	imported := 0
	model := &models.SettingsModel{}
	for key, value := range req.Settings {
		var setErr error
		if textValue, ok := value.(string); ok {
			setErr = model.SetString(key, textValue)
		} else {
			encoded, encodeErr := json.Marshal(value)
			if encodeErr != nil {
				continue
			}
			settingType := "json"
			switch value.(type) {
			case bool:
				settingType = "boolean"
			case float64, int, int64:
				settingType = "number"
			}
			setErr = model.SetSetting(key, string(encoded), settingType)
		}
		if setErr == nil {
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
	cfg, err := config.LoadConfigStrict()
	if err != nil {
		InternalError(w, r, "Failed to reload configuration")
		return
	}
	config.SetGlobalConfig(cfg)

	RespondSuccess(w, r, "Configuration reload triggered", map[string]interface{}{
		"note": "Settings are reloaded from the configuration files",
	})
}

func adminSettingValue(setting *models.Setting) any {
	switch setting.Type {
	case "boolean":
		return config.IsTruthy(setting.Value)
	case "number", "json":
		var value any
		if json.Unmarshal([]byte(setting.Value), &value) == nil {
			return value
		}
	}
	return setting.Value
}
