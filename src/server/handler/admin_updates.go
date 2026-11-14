package handler

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/webappsgo/wthr/src/config"
	"github.com/webappsgo/wthr/src/database"
	"github.com/webappsgo/wthr/src/server/middleware"
)

// UpdateSettings saves the release channel and scheduled update policy.
func UpdateSettings(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		BadRequest(w, r, "invalid update settings")
		return
	}
	branch := strings.ToLower(strings.TrimSpace(r.FormValue("branch")))
	switch branch {
	case "stable", "beta", "devel":
	default:
		BadRequest(w, r, "branch must be stable, beta, or devel")
		return
	}
	deferDays, err := strconv.Atoi(r.FormValue("defer_days"))
	if err != nil || deferDays < 0 || deferDays > 365 {
		BadRequest(w, r, "defer days must be between 0 and 365")
		return
	}
	cfg, err := config.LoadConfig()
	if err != nil {
		InternalError(w, r, err.Error())
		return
	}
	cfg.Server.Update.Branch = branch
	cfg.Server.Update.DeferDays = deferDays
	cfg.Server.Update.AutoInstall = r.FormValue("auto_install") == "true"
	if err := config.SaveConfig(cfg); err != nil {
		InternalError(w, r, err.Error())
		return
	}
	config.SetGlobalConfig(cfg)
	http.Redirect(w, r, "/server/"+cfg.GetAdminPath()+"/updates?saved=1", http.StatusSeeOther)
}

// ShowUpdateSettings renders update configuration and the current release.
func ShowUpdateSettings(w http.ResponseWriter, r *http.Request) {
	cfg := config.GetGlobalConfig()
	if cfg == nil {
		InternalError(w, r, "configuration is unavailable")
		return
	}
	availableVersion := ""
	if db := database.GetServerDB(); db != nil {
		_ = db.QueryRow(`SELECT message FROM server_admin_notifications WHERE title = ? ORDER BY created_at DESC LIMIT 1`, "Update available").Scan(&availableVersion)
	}
	data := map[string]interface{}{
		"title":             "Updates - Admin",
		"page":              "server-updates",
		"version":           Version,
		"update_branch":     cfg.Server.Update.Branch,
		"auto_install":      cfg.Server.Update.AutoInstall,
		"defer_days":        cfg.Server.Update.DeferDays,
		"saved":             r.URL.Query().Get("saved") == "1",
		"available_version": availableVersion,
	}
	middleware.RenderHTML(w, r, http.StatusOK, "admin/admin_updates.tmpl", AdminTemplateData(r, data))
}
