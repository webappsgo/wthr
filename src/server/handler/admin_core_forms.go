package handler

import (
	"net/http"
	"strconv"

	"github.com/webappsgo/wthr/src/config"
	"github.com/webappsgo/wthr/src/database"
	"github.com/webappsgo/wthr/src/server/model"
)

// SaveEmailForm persists the SMTP settings submitted by the session-authenticated admin page.
func SaveEmailForm(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		redirectAdminForm(w, r, adminPagePath(r, "/email"), "flash.settings_saved", "flash.settings_save_failed", err)
		return
	}
	s := &model.SettingsModel{DB: database.GetServerDB()}
	values := map[string]string{"smtp.host": r.FormValue("smtp.host"), "smtp.port": r.FormValue("smtp.port"), "smtp.username": r.FormValue("smtp.username"), "smtp.tls": r.FormValue("smtp.tls"), "from.email": r.FormValue("smtp.from_address"), "from.name": r.FormValue("smtp.from_name"), "smtp.enabled": strconv.FormatBool(r.FormValue("smtp.enabled") != "")}
	port, portErr := strconv.Atoi(values["smtp.port"])
	if portErr != nil || port < 1 || port > 65535 {
		redirectAdminForm(w, r, adminPagePath(r, "/email"), "flash.settings_saved", "flash.settings_save_failed", strconv.ErrSyntax)
		return
	}
	if tlsMode := values["smtp.tls"]; tlsMode != "auto" && tlsMode != "starttls" && tlsMode != "tls" && tlsMode != "none" {
		redirectAdminForm(w, r, adminPagePath(r, "/email"), "flash.settings_saved", "flash.settings_save_failed", strconv.ErrSyntax)
		return
	}
	var err error
	for key, value := range values {
		if err = s.SetSetting(key, value, "string"); err != nil {
			break
		}
	}
	if err == nil && r.FormValue("smtp.password") != "" {
		err = config.WriteSecret("server.notifications.email.smtp.password", r.FormValue("smtp.password"))
	}
	redirectAdminForm(w, r, adminPagePath(r, "/email"), "flash.settings_saved", "flash.settings_save_failed", err)
}

// SaveDatabaseForm persists validated database settings from either database page form.
func SaveDatabaseForm(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		redirectAdminForm(w, r, adminPagePath(r, "/database"), "flash.settings_saved", "flash.settings_save_failed", err)
		return
	}
	s := &model.SettingsModel{DB: database.GetServerDB()}
	values := map[string]string{}
	for _, key := range []string{"driver", "host", "port", "name", "username", "sslmode", "pool_size", "timeout"} {
		if v := r.FormValue("database." + key); v != "" {
			values[key] = v
		}
	}
	if r.FormValue("database.auto_optimize") != "" {
		values["auto_optimize"] = "true"
	} else {
		values["auto_optimize"] = "false"
	}
	if port := values["port"]; port != "" {
		n, err := strconv.Atoi(port)
		if err != nil || n < 1 || n > 65535 {
			redirectAdminForm(w, r, adminPagePath(r, "/database"), "flash.settings_saved", "flash.settings_save_failed", strconv.ErrSyntax)
			return
		}
	}
	if password := r.FormValue("database.password"); password != "" {
		if err := config.WriteSecret("server.database.password", password); err != nil {
			redirectAdminForm(w, r, adminPagePath(r, "/database"), "flash.settings_saved", "flash.settings_save_failed", err)
			return
		}
	}
	var err error
	for key, value := range values {
		if err = s.SetSetting("database."+key, value, "string"); err != nil {
			break
		}
	}
	redirectAdminForm(w, r, adminPagePath(r, "/database"), "flash.settings_saved", "flash.settings_save_failed", err)
}

// SaveMetricsForm persists the metrics page's plain HTML form.
func SaveMetricsForm(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		redirectAdminForm(w, r, adminPagePath(r, "/metrics"), "flash.settings_saved", "flash.settings_save_failed", err)
		return
	}
	cfg := config.GetGlobalConfig()
	if cfg == nil {
		redirectAdminForm(w, r, adminPagePath(r, "/metrics"), "flash.settings_saved", "flash.settings_save_failed", http.ErrServerClosed)
		return
	}
	next := cfg.Server.Metrics
	next.Enabled = r.FormValue("enabled") != ""
	next.Root.Enabled = r.FormValue("root_alias_enabled") != ""
	next.IncludeSystem = r.FormValue("include_system") != ""
	next.IncludeRuntime = r.FormValue("include_runtime") != ""
	next.Auth.AllowUnauthenticated = r.FormValue("allow_unauthenticated") != ""
	if n, err := strconv.Atoi(r.FormValue("loki_max_entries")); err != nil || n < 1 {
		redirectAdminForm(w, r, adminPagePath(r, "/metrics"), "flash.settings_saved", "flash.settings_save_failed", strconv.ErrSyntax)
		return
	} else {
		next.Loki.MaxEntries = n
	}
	if age := r.FormValue("loki_max_age"); age != "" {
		next.Loki.MaxAge = age
	}
	if next.Auth.Tokens == nil {
		next.Auth.Tokens = map[string]string{}
	}
	for _, service := range []string{"prometheus", "grafana", "loki"} {
		if token := r.FormValue("token_" + service); token != "" {
			next.Auth.Tokens[service] = token
		}
	}
	var err error
	if errs := config.UpdateMetricsConfig(next); len(errs) > 0 {
		err = configError(errs[0].Message)
	}
	redirectAdminForm(w, r, adminPagePath(r, "/metrics"), "flash.settings_saved", "flash.settings_save_failed", err)
}

type configError string

func (e configError) Error() string { return string(e) }
