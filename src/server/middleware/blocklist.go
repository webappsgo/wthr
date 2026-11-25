package middleware

import (
	"context"
	"database/sql"
	"log"
	"net/http"
	"net/netip"
	"strings"
	"sync/atomic"
	"time"

	"github.com/webappsgo/wthr/src/common/dbtime"
	httpclient "github.com/webappsgo/wthr/src/common/httputil"
	"github.com/webappsgo/wthr/src/database"
	"github.com/webappsgo/wthr/src/server/reqctx"
	"github.com/webappsgo/wthr/src/util"
)

var blocklistUnavailable atomic.Bool

// BlocklistMiddleware checks active IP and CIDR blocks before rate limiting.
// Database errors fail open and are logged once per outage; authentication
// still runs for every request that continues.
func BlocklistMiddleware(store *database.Store) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if store == nil || reqctx.GetBool(r.Context(), "allowlisted") {
				next.ServeHTTP(w, r)
				return
			}
			address, err := netip.ParseAddr(util.TrustedGetClientIP(r))
			if err != nil {
				next.ServeHTTP(w, r)
				return
			}
			blocked, err := ipAddressBlocked(r.Context(), store, address.Unmap())
			if err != nil {
				if blocklistUnavailable.CompareAndSwap(false, true) {
					log.Printf("WARNING: IP blocklist lookup unavailable; requests continue through the remaining security checks: %v", err)
				}
				next.ServeHTTP(w, r)
				return
			}
			blocklistUnavailable.Store(false)
			if !blocked {
				next.ServeHTTP(w, r)
				return
			}
			log.Printf("SECURITY: ip_blocked client_ip=%s path=%s", address, r.URL.Path)
			writeIPBlockError(w, r)
		})
	}
}

func writeIPBlockError(w http.ResponseWriter, r *http.Request) {
	const message = "Request is blocked by the server security policy"
	if strings.HasPrefix(r.URL.Path, "/api/") {
		if strings.Contains(r.Header.Get("Accept"), "text/plain") {
			w.Header().Set("Content-Type", "text/plain; charset=utf-8")
			w.WriteHeader(http.StatusForbidden)
			_, _ = w.Write([]byte(message + "\n"))
			return
		}
		writeAPIError(w, http.StatusForbidden, "IP_BLOCKED", message)
		return
	}
	if strings.Contains(r.Header.Get("Accept"), "application/json") {
		writeAPIError(w, http.StatusForbidden, "IP_BLOCKED", message)
		return
	}
	if httpclient.FrontendWantsText(r) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(message + "\n"))
		return
	}
	if RenderHTML != nil {
		RenderHTML(w, r, http.StatusForbidden, "page/error.tmpl", util.TemplateData(r, map[string]interface{}{"error": message}))
		return
	}
	http.Error(w, message, http.StatusForbidden)
}

func ipAddressBlocked(ctx context.Context, store *database.Store, address netip.Addr) (bool, error) {
	rows, err := database.QueryContext(ctx, store, database.TimeoutSimpleSelect,
		`SELECT type, value, expires_at FROM server_blocklists
		 WHERE type IN ('ip', 'cidr') ORDER BY id`)
	if err != nil {
		return false, err
	}
	defer func() {
		if err := rows.Close(); err != nil {
			log.Printf("WARNING: failed to close blocklist rows: %v", err)
		}
	}()
	for rows.Next() {
		var kind, value string
		var expiresAt sql.NullString
		if err := rows.Scan(&kind, &value, &expiresAt); err != nil {
			return false, err
		}
		if expiresAt.Valid {
			if expiry, ok := dbtime.ParseStoredTimestamp(expiresAt.String); ok && !expiry.After(time.Now()) {
				continue
			}
		}
		value = strings.TrimSpace(value)
		if kind == "cidr" {
			prefix, err := netip.ParsePrefix(value)
			if err == nil && prefix.Masked().Contains(address) {
				return true, nil
			}
			continue
		}
		blockedIP, err := netip.ParseAddr(value)
		if err == nil && blockedIP.Unmap() == address {
			return true, nil
		}
	}
	if err := rows.Err(); err != nil {
		return false, err
	}
	return false, nil
}
