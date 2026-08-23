package middleware

import (
	"context"
	"crypto/rand"
	"database/sql"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/webappsgo/wthr/src/common/dbtime"
	"github.com/webappsgo/wthr/src/config"
	"github.com/webappsgo/wthr/src/database"
	"github.com/webappsgo/wthr/src/util"

	chimw "github.com/go-chi/chi/v5/middleware"
	"github.com/oklog/ulid/v2"
)

// AuditLogger logs admin actions to the server_audit_log table
func AuditLogger(db *sql.DB) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Only log admin routes
			if !isAdminRoute(r.URL.Path) {
				next.ServeHTTP(w, r)
				return
			}

			// Skip GET requests (only log modifications)
			if r.Method == "GET" || r.Method == "OPTIONS" {
				next.ServeHTTP(w, r)
				return
			}

			// Get user info
			user, exists := GetCurrentUser(r)
			var userID *int64
			if exists {
				userID = &user.ID
			}

			// Get client info
			clientIP := util.GetClientIP(r)
			userAgent := r.UserAgent()

			// Capture the response
			ww := chimw.NewWrapResponseWriter(w, r.ProtoMajor)
			next.ServeHTTP(ww, r)

			// Determine action from method and path
			action := getActionFromRequest(r.Method, r.URL.Path)
			resource := getResourceFromPath(r.URL.Path)

			// Log the action
			success := ww.Status() >= 200 && ww.Status() < 400
			status := "success"
			if !success {
				status = "failure"
			}

			actorType := "anonymous"
			var actorID string
			if userID != nil {
				actorType = "user"
				actorID = strconv.FormatInt(*userID, 10)
			}

			now := time.Now()
			id := ulid.MustNew(ulid.Timestamp(now), rand.Reader).String()

			// server_audit_log.timestamp is also written by src/scheduler/scheduler.go
			// and src/server/handler/admin_passkey.go as canonical UTC text. Binding a
			// raw time.Time here would make modernc.org/sqlite store the host-local
			// time.Time.String() form instead, leaving one column with two layouts that
			// no SQL-side comparison or ORDER BY can reconcile - format through dbtime
			// so every producer of this column agrees.
			_, err := database.ExecContext(context.Background(), db, database.TimeoutWrite, `
				INSERT INTO server_audit_log (ulid, timestamp, actor_type, actor_id, action, resource_type, resource_id, ip_address, user_agent, status)
				VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
			`, id, dbtime.FormatSQLTimestamp(now), actorType, actorID, action, resource, "", clientIP, userAgent, status)

			if err != nil {
				// A failed audit write must never fail the request, but it must
				// also never be silent: there is nothing in this project that reads
				// a per-request error list, so a broken audit table would drop
				// every admin action on the floor with no operator-visible signal.
				// PART 11 requires security-relevant actions to be recorded, so the
				// failure to record one is itself a security event and belongs in
				// the log at full detail (console and log files are the audience
				// allowed to see internal errors).
				log.Printf("audit: failed to record admin action %q on %q by %s %q from %s: %v",
					action, resource, actorType, actorID, clientIP, err)
			}
		})
	}
}

// isAdminRoute checks if the path is an admin route, using the configured
// admin.path (AI.md config-rules.md: admin path is configurable, default
// "admin") rather than a hardcoded "/server/admin" prefix - matching the
// resolution pattern already used by auth.go's RestrictAdminToAdminRoutes.
func isAdminRoute(path string) bool {
	webPrefix, apiPrefix := adminRoutePrefixes()
	return strings.HasPrefix(path, webPrefix) || strings.HasPrefix(path, apiPrefix)
}

// adminRoutePrefixes returns the browser and API admin route prefixes for the
// currently configured admin path, in that order. Both the route test and the
// resource extraction below must agree on these strings: if one of them used a
// hardcoded "/server/admin" while the operator had configured a different
// admin.path, the middleware would either log nothing at all or strip the wrong
// number of characters and report a garbled resource name.
func adminRoutePrefixes() (webPrefix, apiPrefix string) {
	webPrefix = "/server/admin"
	if cfg, err := config.LoadConfig(); err == nil && cfg != nil {
		webPrefix = "/server/" + cfg.GetAdminPath()
	}
	return webPrefix, "/api/v1" + webPrefix
}

// getActionFromRequest determines the action type from method and path
func getActionFromRequest(method, path string) string {
	switch method {
	case "POST":
		if strings.Contains(path, "/login") {
			return "login"
		}
		if strings.Contains(path, "/logout") {
			return "logout"
		}
		return "create"
	case "PUT", "PATCH":
		return "update"
	case "DELETE":
		return "delete"
	default:
		return method
	}
}

// getResourceFromPath extracts the resource name from the path.
//
// The prefixes come from the same configured admin path isAdminRoute uses, so a
// deployment with a non-default admin.path reports real resource names instead
// of the mangled remainder a fixed-length prefix strip would leave behind. The
// API prefix is tested first because it contains the web prefix as a suffix.
func getResourceFromPath(path string) string {
	webPrefix, apiPrefix := adminRoutePrefixes()
	if strings.HasPrefix(path, apiPrefix) {
		path = strings.TrimPrefix(path, apiPrefix)
	} else if strings.HasPrefix(path, webPrefix) {
		path = strings.TrimPrefix(path, webPrefix)
	}

	// Remove leading slash
	path = strings.TrimPrefix(path, "/")

	// Extract first path segment
	if segment, _, found := strings.Cut(path, "/"); found {
		return segment
	}

	if path == "" {
		return "dashboard"
	}

	return path
}
