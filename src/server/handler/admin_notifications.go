package handler

import (
	"database/sql"
	"encoding/json"
	"github.com/webappsgo/wthr/src/server/middleware"
	"github.com/webappsgo/wthr/src/server/model"
	"net/http"
	"strconv"
	"strings"

	"github.com/webappsgo/wthr/src/util"
)

// AdminNotificationsHandler handles notification preferences
type AdminNotificationsHandler struct {
	ConfigPath string
	UsersDB    *sql.DB
}

// ShowNotificationSettings displays notification settings page
func (h *AdminNotificationsHandler) ShowNotificationSettings(w http.ResponseWriter, r *http.Request) {
	c := readAdminConfig(h.ConfigPath)
	settings := map[string]interface{}{"webui_position": "top-right", "webui_duration": 5, "webui_max_stored": 100, "webui_retention_days": 30}
	paths := map[string]string{"email_startup": "server.notifications.email.events.startup", "email_shutdown": "server.notifications.email.events.shutdown", "email_backup_complete": "server.notifications.email.events.backup_complete", "email_backup_failed": "server.notifications.email.events.backup_failed", "email_cert_renewal": "server.notifications.email.events.cert_renewal", "webhook_enabled": "server.notifications.webhook.enabled", "webhook_url": "server.notifications.webhook.url", "webui_position": "server.notifications.webui.position", "webui_duration": "server.notifications.webui.duration", "webui_max_stored": "server.notifications.webui.max_stored", "webui_retention_days": "server.notifications.webui.retention_days"}
	for key, path := range paths {
		if v := adminConfigValue(c, path); v != nil {
			settings[key] = v
		}
	}
	events := []string{}
	if v, ok := adminConfigValue(c, "server.notifications.webhook.events").([]interface{}); ok {
		for _, item := range v {
			if text, ok := item.(string); ok {
				events = append(events, text)
			}
		}
	}
	settings["webhook_events"] = strings.Join(events, ",")
	adminID := int(AdminID(r))
	prefs := &model.NotificationPreferences{AdminID: &adminID, EnableToast: true, EnableBanner: true, EnableCenter: true, ToastDurationSuccess: 5, ToastDurationInfo: 5, ToastDurationWarning: 10}
	if id := AdminID(r); id > 0 {
		if stored, err := (&model.NotificationPreferencesModel{UserDB: h.UsersDB}).GetAdminPreferences(int(id)); err == nil && stored != nil {
			prefs = stored
		}
	}
	data := map[string]interface{}{
		"title":    Translate(r, "admin.notifications.notification_settings"),
		"settings": settings, "admin_prefs": prefs, "page": "notifications", "flash": TakeFlash(w, r),
	}
	middleware.RenderHTML(w, r, http.StatusOK, "admin/admin_notifications.tmpl", AdminTemplateData(r, data))
}

// SaveAdminPreferencesForm stores the logged-in admin's personal display preferences.
func (h *AdminNotificationsHandler) SaveAdminPreferencesForm(w http.ResponseWriter, r *http.Request) {
	target := strings.TrimSuffix(r.URL.Path, "/notifications/preferences") + "/notifications"
	if err := r.ParseForm(); err != nil {
		redirectAdminForm(w, r, target, "flash.settings_saved", "flash.settings_save_failed", err)
		return
	}
	adminID := AdminID(r)
	if adminID <= 0 {
		redirectAdminForm(w, r, target, "flash.settings_saved", "flash.settings_save_failed", errInvalidAdminForm)
		return
	}
	if r.PostForm.Get("reset") == "true" {
		adminIDInt := int(adminID)
		prefs := &model.NotificationPreferences{
			AdminID: &adminIDInt, EnableToast: true, EnableBanner: true, EnableCenter: true,
			EnableSound: false, ToastDurationSuccess: 5, ToastDurationInfo: 5, ToastDurationWarning: 10,
		}
		err := (&model.NotificationPreferencesModel{UserDB: h.UsersDB}).UpdateAdminPreferences(int(adminID), prefs)
		redirectAdminForm(w, r, target, "flash.settings_saved", "flash.settings_save_failed", err)
		return
	}
	readDuration := func(name string) (int, bool) {
		v, err := strconv.Atoi(r.FormValue(name))
		return v, err == nil && v >= 1 && v <= 60
	}
	success, okSuccess := readDuration("toast_duration_success")
	info, okInfo := readDuration("toast_duration_info")
	warning, okWarning := readDuration("toast_duration_warning")
	if !okSuccess || !okInfo || !okWarning {
		redirectAdminForm(w, r, target, "flash.settings_saved", "flash.settings_save_failed", errInvalidAdminForm)
		return
	}
	prefs := &model.NotificationPreferences{AdminID: new(int), EnableToast: r.FormValue("enable_toast") == "true", EnableBanner: r.FormValue("enable_banner") == "true", EnableCenter: r.FormValue("enable_center") == "true", EnableSound: r.FormValue("enable_sound") == "true", ToastDurationSuccess: success, ToastDurationInfo: info, ToastDurationWarning: warning}
	*prefs.AdminID = int(adminID)
	err := (&model.NotificationPreferencesModel{UserDB: h.UsersDB}).UpdateAdminPreferences(int(adminID), prefs)
	redirectAdminForm(w, r, target, "flash.settings_saved", "flash.settings_save_failed", err)
}

// UpdateNotificationSettingsForm handles server-wide notification settings.
func (h *AdminNotificationsHandler) UpdateNotificationSettingsForm(w http.ResponseWriter, r *http.Request) {
	target := adminPagePath(r, "/notifications")
	if err := r.ParseForm(); err != nil {
		redirectAdminForm(w, r, target, "flash.settings_saved", "flash.settings_save_failed", err)
		return
	}
	ints := map[string]int{}
	for _, name := range []string{"webui_duration", "webui_max_stored", "webui_retention_days"} {
		v, err := strconv.Atoi(r.FormValue(name))
		if err != nil || v < 0 {
			redirectAdminForm(w, r, target, "flash.settings_saved", "flash.settings_save_failed", errInvalidAdminForm)
			return
		}
		ints[name] = v
	}
	updates := map[string]interface{}{"server.notifications.email.events.startup": r.FormValue("email_startup") == "true", "server.notifications.email.events.shutdown": r.FormValue("email_shutdown") == "true", "server.notifications.email.events.backup_complete": r.FormValue("email_backup_complete") == "true", "server.notifications.email.events.backup_failed": r.FormValue("email_backup_failed") == "true", "server.notifications.email.events.cert_renewal": r.FormValue("email_cert_renewal") == "true", "server.notifications.webhook.enabled": r.FormValue("webhook_enabled") == "true", "server.notifications.webhook.url": r.FormValue("webhook_url"), "server.notifications.webhook.events": strings.FieldsFunc(r.FormValue("webhook_events"), func(r rune) bool { return r == ',' || r == ' ' }), "server.notifications.webui.duration": ints["webui_duration"], "server.notifications.webui.max_stored": ints["webui_max_stored"], "server.notifications.webui.retention_days": ints["webui_retention_days"]}
	err := util.UpdateYAMLConfig(h.ConfigPath, updates)
	redirectAdminForm(w, r, target, "flash.settings_saved", "flash.settings_save_failed", err)
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
