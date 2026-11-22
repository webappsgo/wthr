package middleware

import (
	"net/http"

	"github.com/webappsgo/wthr/src/config"
	"github.com/webappsgo/wthr/src/server/reqctx"
	"github.com/webappsgo/wthr/src/util"
)

// AllowlistMiddleware marks trusted source IPs so the blocklist, rate limits,
// and GeoIP policy can bypass them without bypassing authentication.
func AllowlistMiddleware() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if config.IsIPAllowlisted(util.TrustedGetClientIP(r)) {
				ctx := reqctx.SetValue(r.Context(), "allowlisted", true)
				r = r.WithContext(ctx)
			}
			next.ServeHTTP(w, r)
		})
	}
}
