package middleware

import (
	"log"
	"net/http"
	"net/netip"
	"strings"
	"sync/atomic"

	"github.com/webappsgo/wthr/src/common/i18n"
	"github.com/webappsgo/wthr/src/config"
	"github.com/webappsgo/wthr/src/server/reqctx"
	"github.com/webappsgo/wthr/src/util"
)

var geoIPLookupUnavailable atomic.Bool

// GeoIPCountryBlocker applies the configured deny-only country policy. Lookup
// errors and private addresses fail open so authentication and authorization
// remain responsible for access control.
func GeoIPCountryBlocker(lookup func(string) (string, error)) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			cfg := config.GetGlobalConfig()
			if cfg == nil || !cfg.Server.GeoIP.Enabled || lookup == nil {
				next.ServeHTTP(w, r)
				return
			}
			clientIP := util.TrustedGetClientIP(r)
			address, err := netip.ParseAddr(clientIP)
			if err != nil {
				next.ServeHTTP(w, r)
				return
			}
			address = address.Unmap()
			if address.IsPrivate() || address.IsLoopback() || address.IsLinkLocalUnicast() || address.IsLinkLocalMulticast() || address.IsUnspecified() || config.IsIPAllowlisted(clientIP) {
				next.ServeHTTP(w, r)
				return
			}
			country, err := lookup(clientIP)
			if err != nil {
				if geoIPLookupUnavailable.CompareAndSwap(false, true) {
					log.Printf("WARNING: GeoIP country lookup unavailable; country blocking is skipped: %v", err)
				}
				next.ServeHTTP(w, r)
				return
			}
			geoIPLookupUnavailable.Store(false)
			if !cfg.Server.GeoIP.BlocksCountry(country) {
				next.ServeHTTP(w, r)
				return
			}
			log.Printf("SECURITY: geoip_country_blocked country=%s client_ip=%s path=%s", country, clientIP, r.URL.Path)
			message := "Request is not allowed from this country"
			if translations := i18n.GetGlobalI18n(); translations != nil {
				language := reqctx.GetString(r.Context(), "lang")
				if language == "" {
					language = "en"
				}
				message = translations.T(language, "errors.geoip.country_blocked")
			}
			if strings.Contains(strings.ToLower(r.Header.Get("Accept")), "text/html") && RenderHTML != nil {
				RenderHTML(w, r, http.StatusForbidden, "page/error.tmpl", util.TemplateData(r, map[string]interface{}{"error": message}))
				return
			}
			writeAPIError(w, http.StatusForbidden, "GEOIP_BLOCKED", message)
		})
	}
}
