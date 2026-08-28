package handler

import (
	"encoding/json"
	"github.com/webappsgo/wthr/src/server/middleware"
	"net/http"

	"github.com/webappsgo/wthr/src/util"
)

// AdminNotificationsHandler handles notification preferences
type AdminNotificationsHandler struct {
	ConfigPath string
}

// ShowNotificationSettings displays notification settings page
func (h *AdminNotificationsHandler) ShowNotificationSettings(w http.ResponseWriter, r *http.Request) {
	middleware.RenderHTML(w, r, http.StatusOK, "admin/admin_notifications.tmpl", util.TemplateData(r, map[string]interface{}{
		"title": Translate(r, "admin.notifications.notification_settings"),
	}))
}

// UpdateNotificationSettings updates notification settings
func (h *AdminNotificationsHandler) UpdateNotificationSettings(w http.ResponseWriter, r *http.Request) {
	var req struct {
		// Email Events
		EmailStartup        bool `json:"email_startup"`
		EmailShutdown       bool `json:"email_shutdown"`
		EmailBackupComplete bool `json:"email_backup_complete"`
		EmailBackupFailed   bool `json:"email_backup_failed"`
		EmailCertRenewal    bool `json:"email_cert_renewal"`
		// Webhook
		WebhookEnabled bool     `json:"webhook_enabled"`
		WebhookURL     string   `json:"webhook_url"`
		WebhookEvents  []string `json:"webhook_events"`
		// WebUI
		WebUIPosition      string `json:"webui_position"`
		WebUIDuration      int    `json:"webui_duration"`
		WebUIMaxStored     int    `json:"webui_max_stored"`
		WebUIRetentionDays int    `json:"webui_retention_days"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		BadRequest(w, r, err.Error())
		return
	}

	updates := map[string]interface{}{
		"server.notifications.email.events.startup":         req.EmailStartup,
		"server.notifications.email.events.shutdown":        req.EmailShutdown,
		"server.notifications.email.events.backup_complete": req.EmailBackupComplete,
		"server.notifications.email.events.backup_failed":   req.EmailBackupFailed,
		"server.notifications.email.events.cert_renewal":    req.EmailCertRenewal,
		"server.notifications.webhook.enabled":              req.WebhookEnabled,
		"server.notifications.webhook.url":                  req.WebhookURL,
		"server.notifications.webui.position":               req.WebUIPosition,
		"server.notifications.webui.duration":               req.WebUIDuration,
		"server.notifications.webui.max_stored":             req.WebUIMaxStored,
		"server.notifications.webui.retention_days":         req.WebUIRetentionDays,
	}

	if err := util.UpdateYAMLConfig(h.ConfigPath, updates); err != nil {
		InternalError(w, r, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{"ok": true})
}
