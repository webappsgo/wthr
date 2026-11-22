package handler

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/webappsgo/wthr/src/database"
	"github.com/webappsgo/wthr/src/server/model"
)

var adminSecurityFormFields = map[string][]string{
	"headers":   {"security.csp", "security.hsts", "security.x_frame_options", "security.x_content_type", "security.referrer_policy", "security.permissions_policy"},
	"cors":      {"security.cors_enabled", "security.cors_origins", "security.cors_methods", "security.cors_headers", "security.cors_credentials", "security.cors_max_age"},
	"ratelimit": {"rate_limit.enabled", "rate_limit.global", "rate_limit.per_ip", "rate_limit.api", "rate_limit.admin"},
	"auth":      {"security.session_timeout", "security.max_login_attempts", "security.lockout_duration", "security.password_min_length"},
}

// SaveSecuritySettingsForm saves one section of settings submitted from the
// no-JavaScript security page and redirects back using POST/Redirect/GET.
func SaveSecuritySettingsForm(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		redirectAdminForm(w, r, adminPagePath(r, "/security"), "flash.settings_saved", "flash.settings_save_failed", err)
		return
	}
	section := r.PostForm.Get("section")
	fields, ok := adminSecurityFormFields[section]
	if !ok {
		redirectAdminForm(w, r, adminPagePath(r, "/security"), "flash.settings_saved", "flash.settings_save_failed", fmt.Errorf("invalid security settings section"))
		return
	}
	values := make(map[string]string, len(fields))
	for _, key := range fields {
		value := strings.TrimSpace(r.PostForm.Get(key))
		if key == "security.cors_enabled" || key == "security.cors_credentials" || key == "rate_limit.enabled" {
			value = strconv.FormatBool(r.PostForm.Get(key) != "")
		}
		if value == "" && ((strings.HasPrefix(key, "rate_limit.") && key != "rate_limit.enabled") || strings.HasPrefix(key, "security.session_") || strings.HasPrefix(key, "security.max_login_") || strings.HasPrefix(key, "security.lockout_") || strings.HasPrefix(key, "security.password_min_")) {
			redirectAdminForm(w, r, adminPagePath(r, "/security"), "flash.settings_saved", "flash.settings_save_failed", fmt.Errorf("%s is required", key))
			return
		}
		if strings.HasPrefix(key, "rate_limit.") && key != "rate_limit.enabled" || key == "security.cors_max_age" || key == "security.session_timeout" || key == "security.max_login_attempts" || key == "security.lockout_duration" || key == "security.password_min_length" {
			n, err := strconv.Atoi(value)
			min := 0
			max := 1000000
			if strings.HasPrefix(key, "rate_limit.") {
				min, max = 1, 10000
			}
			if key == "security.cors_max_age" {
				max = 86400
			}
			if key == "security.session_timeout" {
				min, max = 300, 31536000
			}
			if key == "security.max_login_attempts" {
				min, max = 1, 20
			}
			if key == "security.lockout_duration" {
				min, max = 1, 1440
			}
			if key == "security.password_min_length" {
				min, max = 6, 64
			}
			if err != nil || n < min || n > max {
				redirectAdminForm(w, r, adminPagePath(r, "/security"), "flash.settings_saved", "flash.settings_save_failed", fmt.Errorf("%s is outside the allowed range", key))
				return
			}
		}
		if key == "security.x_frame_options" && value != "DENY" && value != "SAMEORIGIN" && value != "ALLOW-FROM" {
			redirectAdminForm(w, r, adminPagePath(r, "/security"), "flash.settings_saved", "flash.settings_save_failed", fmt.Errorf("invalid X-Frame-Options value"))
			return
		}
		values[key] = value
	}
	settings := &model.SettingsModel{DB: database.GetServerDB()}
	for key, value := range values {
		if err := settings.SetSetting(key, value, "string"); err != nil {
			redirectAdminForm(w, r, adminPagePath(r, "/security"), "flash.settings_saved", "flash.settings_save_failed", err)
			return
		}
	}
	redirectAdminForm(w, r, adminPagePath(r, "/security"), "flash.settings_saved", "flash.settings_save_failed", nil)
}
