package handler

import (
	"fmt"
	"net/http"

	"github.com/webappsgo/wthr/src/config"
	"github.com/webappsgo/wthr/src/server/middleware"
	"github.com/webappsgo/wthr/src/server/service"
)

// ServeTrackingScript serves the same-origin analytics script body so pages never
// carry inline JavaScript (AI.md PART 12 "Tracking and CSP"). It answers 404 when
// tracking is off, the provider has no script body, consent is missing, or the
// request arrived through an onion or I2P address.
// GET /server/tracking.js
func ServeTrackingScript(w http.ResponseWriter, r *http.Request) {
	cfg := config.GetGlobalConfig()
	if cfg == nil || !middleware.TrackingAllowed(r) {
		http.NotFound(w, r)
		return
	}
	body := service.TrackingScriptBody(cfg.Server.Tracking)
	if body == "" {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "application/javascript; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	fmt.Fprint(w, body)
}
