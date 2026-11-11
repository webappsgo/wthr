package middleware

import (
	"context"
	"crypto/rand"
	"database/sql"
	"net/http"
	"strings"
	"time"

	"github.com/oklog/ulid/v2"
	"github.com/webappsgo/wthr/src/common/dbtime"
	"github.com/webappsgo/wthr/src/database"
	"github.com/webappsgo/wthr/src/server/reqctx"
	"github.com/webappsgo/wthr/src/util"
)

const gpcOptOutContextKey = "gpc_opt_out"

// GlobalPrivacyControl records and exposes the browser's privacy opt-out for
// the full request lifecycle.
func GlobalPrivacyControl(db *sql.DB) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Header.Get("Sec-GPC") != "1" {
				next.ServeHTTP(w, r)
				return
			}

			ctx := reqctx.SetValue(r.Context(), gpcOptOutContextKey, true)
			request := r.Clone(ctx)
			request.Header.Set("Cookie", cookiesWithoutOptionalPreferences(r.Cookies()))
			next.ServeHTTP(w, request)
			if db == nil {
				return
			}

			now := time.Now()
			_, err := database.ExecContext(context.Background(), db, database.TimeoutWrite, `
				INSERT INTO server_audit_log (ulid, timestamp, actor_type, actor_id, action, resource_type, resource_id, ip_address, user_agent, source, status)
				VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
			`, ulid.MustNew(ulid.Timestamp(now), rand.Reader).String(), dbtime.FormatSQLTimestamp(now), "anonymous", "", "compliance.gpc_honored", "request", r.URL.Path, util.TrustedGetClientIP(r), r.UserAgent(), "web", "success")
			if err != nil {
				util.LogError("audit: failed to record Global Privacy Control for %s: %v", r.URL.Path, err)
			}
		})
	}
}

func cookiesWithoutOptionalPreferences(cookies []*http.Cookie) string {
	kept := make([]string, 0, len(cookies))
	for _, cookie := range cookies {
		if isOptionalPreferenceCookie(cookie.Name) {
			continue
		}
		kept = append(kept, cookie.Name+"="+cookie.Value)
	}
	return strings.Join(kept, "; ")
}

func isOptionalPreferenceCookie(name string) bool {
	if strings.HasPrefix(name, "wthr_pref_") {
		return true
	}
	switch name {
	case "theme", "lang", "admin_nav", "flash", "user_lat", "user_lon", "user_location_name", "user_location_set_at":
		return true
	default:
		return false
	}
}

// GPCOptOut reports whether the current request carries the honored GPC
// signal, including handlers invoked directly without the router middleware.
func GPCOptOut(r *http.Request) bool {
	if reqctx.GetBool(r.Context(), gpcOptOutContextKey) {
		return true
	}
	return r.Header.Get("Sec-GPC") == "1"
}
