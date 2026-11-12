// Package pwa serves the service worker, web app manifest and offline page
// (AI.md PART 16 PWA Support).
package pwa

import (
	_ "embed"
	"net/http"
	"strings"

	"github.com/webappsgo/wthr/src/server/buildstamp"
)

//go:embed sw.js
var serviceWorkerSource string

//go:embed manifest.json
var manifestSource []byte

//go:embed offline.html
var offlineSource []byte

const appName = "wthr"

// revalidate marks a response no-cache with the build-stamp ETag, so a changed
// service worker or manifest is always seen on the next update check, and
// answers 304 when the client already holds this build.
func revalidate(w http.ResponseWriter, r *http.Request, contentType string) bool {
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("ETag", buildstamp.ETag())
	if r.Header.Get("If-None-Match") == buildstamp.ETag() {
		w.WriteHeader(http.StatusNotModified)
		return true
	}

	return false
}

// ServiceWorker serves /sw.js rendered with the running build stamp.
func ServiceWorker(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Service-Worker-Allowed", "/")
	if revalidate(w, r, "text/javascript; charset=utf-8") {
		return
	}
	body := strings.NewReplacer(
		"{asset_stamp}", buildstamp.Stamp(),
		"{app_name}", appName,
	).Replace(serviceWorkerSource)
	_, _ = w.Write([]byte(body))
}

// Manifest serves /manifest.json.
func Manifest(w http.ResponseWriter, r *http.Request) {
	if revalidate(w, r, "application/manifest+json; charset=utf-8") {
		return
	}
	_, _ = w.Write(manifestSource)
}

// Offline serves /offline.html, the precached offline page.
func Offline(w http.ResponseWriter, r *http.Request) {
	if revalidate(w, r, "text/html; charset=utf-8") {
		return
	}
	_, _ = w.Write(offlineSource)
}

// StaticCache wraps the static file server: a request whose ?v= equals the
// running build stamp is immutable for a year; any other request is always
// revalidated (AI.md PART 9 Asset Version-Busting).
func StaticCache(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("v") == buildstamp.Stamp() {
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		} else {
			w.Header().Set("Cache-Control", "no-cache")
			w.Header().Set("ETag", buildstamp.ETag())
		}
		next.ServeHTTP(w, r)
	})
}
