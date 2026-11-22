package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"runtime"
	"time"

	"github.com/webappsgo/wthr/src/config"
	"github.com/webappsgo/wthr/src/path"
	"github.com/webappsgo/wthr/src/server/middleware"
)

var adminSystemStartedAt = time.Now()

// ShowAdminSystemInfo renders the core server/runtime details without requiring JavaScript.
func ShowAdminSystemInfo(w http.ResponseWriter, r *http.Request) {
	middleware.RenderHTML(w, r, http.StatusOK, "admin/admin_system.tmpl", AdminTemplateData(r, adminSystemInfoData()))
}

func adminSystemInfoData() map[string]interface{} {
	var stats runtime.MemStats
	runtime.ReadMemStats(&stats)
	hostname, _ := os.Hostname()
	workingDir, _ := os.Getwd()
	cfg := config.GetGlobalConfig()
	mode, port, fqdn := "", "", ""
	httpsEnabled, torEnabled := false, false
	if cfg != nil {
		mode = cfg.Server.Mode
		port = fmt.Sprint(cfg.Server.Port)
		fqdn = cfg.Server.FQDN
		httpsEnabled = cfg.Server.SSL.Enabled
		torEnabled = cfg.Server.Tor.Enabled
	}
	data := map[string]interface{}{
		"title": "Server Information - Admin", "page": "info", "breadcrumb": "Server Info", "server_info": true,
		"version": Version, "build_date": BuildDate, "go_version": runtime.Version(),
		"os": runtime.GOOS, "arch": runtime.GOARCH, "hostname": hostname, "working_dir": workingDir,
		"pid": os.Getpid(), "uptime": time.Since(adminSystemStartedAt).Round(time.Second).String(),
		"memory_alloc": stats.Alloc, "memory_sys": stats.Sys, "goroutines": runtime.NumGoroutine(), "gc_runs": stats.NumGC,
		"http_port": port, "https_port": func() string {
			if httpsEnabled {
				return port
			}
			return "-"
		}(),
		"https_enabled": httpsEnabled, "tor_enabled": torEnabled, "server_url": fqdn,
		"mode": mode, "debug": mode == "development", "data_dir": path.GetDataDir(), "config_dir": path.GetConfigDir(),
		"log_dir": path.GetLogDir(), "timezone": time.Local.String(),
	}
	return data
}

// ExportAdminSystemInfo emits the safe, explicit runtime fields as JSON.
func ExportAdminSystemInfo(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Disposition", "attachment; filename=system-info.json")
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(adminSystemInfoData()); err != nil {
		http.Error(w, "failed to encode system information", http.StatusInternalServerError)
	}
}

// RunAdminGarbageCollection performs the operator-requested garbage collection using a session form.
func RunAdminGarbageCollection(w http.ResponseWriter, r *http.Request) {
	runtime.GC()
	http.Redirect(w, r, adminPagePath(r, "/info"), http.StatusSeeOther)
}

// ReloadAdminConfigurationForm redirects after the server's live settings reload.
func ReloadAdminConfigurationForm(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, adminPagePath(r, "/info")+"?reloaded=1", http.StatusSeeOther)
}
