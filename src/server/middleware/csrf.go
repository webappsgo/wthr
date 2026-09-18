// Package middleware - CSRF protection per AI.md PART 16 (Web Frontend -> CSRF Protection)
package middleware

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"net/http"
	"path"
	"strings"

	"github.com/webappsgo/wthr/src/server/reqctx"
	"github.com/webappsgo/wthr/src/server/service"
	"github.com/webappsgo/wthr/src/util"
)

// CSRFConfig holds CSRF protection configuration per AI.md PART 16
type CSRFConfig struct {
	Enabled     bool
	TokenLength int
	CookieName  string
	HeaderName  string
	Secure      string
	// ExemptPaths is the web.csrf.exempt_paths allow-list. Patterns are
	// matched with path.Match semantics, and a pattern ending in /* also
	// matches every deeper path below it.
	ExemptPaths []string
}

// DefaultCSRFConfig returns the default CSRF configuration per AI.md PART 16
func DefaultCSRFConfig() CSRFConfig {
	return CSRFConfig{
		Enabled:     true,
		TokenLength: 32,
		CookieName:  "csrf_token",
		HeaderName:  "X-CSRF-Token",
		Secure:      "auto",
		// AI.md PART 16 default exempt paths: OIDC providers post the
		// callback cross-site, and webhook senders authenticate with their
		// own signature rather than a browser session.
		ExemptPaths: []string{
			"/api/*/server/auth/oidc/*/callback",
			"/api/*/webhooks/*",
		},
	}
}

// CSRFProtection provides CSRF protection middleware
// Per AI.md: "Security should never get in the way of usability"
// CSRF is required for authenticated state-changing operations only
func CSRFProtection(cfg CSRFConfig) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Skip if disabled
			if !cfg.Enabled {
				next.ServeHTTP(w, r)
				return
			}

			// Always generate/provide token for templates
			tokenCookie, err := r.Cookie(cfg.CookieName)
			token := ""
			if err == nil {
				token = tokenCookie.Value
			}
			if token == "" {
				generated, err := generateCSRFToken(cfg.TokenLength)
				if err != nil {
					writeAPIError(w, http.StatusInternalServerError, "SERVER_ERROR", "Internal server error")
					return
				}
				token = generated
				setCSRFCookie(w, r, cfg, token)
			}
			ctx := reqctx.SetValue(r.Context(), "csrf_token", token)
			r = r.WithContext(ctx)

			// GET, HEAD, OPTIONS are safe methods - no validation needed
			if r.Method == "GET" || r.Method == "HEAD" || r.Method == "OPTIONS" {
				next.ServeHTTP(w, r)
				return
			}

			// AI.md PART 16 bypasses CSRF only for callers a browser cannot
			// forge a request on behalf of: bearer/API-token credentials,
			// WebSocket upgrades, genuinely public endpoints, and the
			// configured exempt-path allow-list. A path prefix alone is not
			// a bypass condition -- session cookies authenticate /api/
			// routes too, and those are exactly the cross-site fetch() calls
			// this middleware has to stop.
			if hasBearerCredential(r) || isWebSocketUpgrade(r) ||
				isPublicEndpoint(r.URL.Path) || matchesExemptPath(r.URL.Path, cfg.ExemptPaths) {
				next.ServeHTTP(w, r)
				return
			}

			// Skip CSRF for unauthenticated users on public pages
			// CSRF protects against session hijacking - no session = no risk
			if _, exists := reqctx.GetValue(r.Context(), UserContextKey); !exists {
				if _, adminExists := reqctx.GetValue(r.Context(), "admin_id"); !adminExists {
					next.ServeHTTP(w, r)
					return
				}
			}

			// Validate CSRF token for authenticated state-changing requests
			cookieToken := ""
			if cookie, err := r.Cookie(cfg.CookieName); err == nil {
				cookieToken = cookie.Value
			}
			if cookieToken == "" {
				logCSRFFailure(r, "CSRF token missing")
				writeCSRFForbidden(w, "CSRF token missing")
				return
			}

			// Check token from form or header
			formToken := r.PostFormValue("csrf_token")
			headerToken := r.Header.Get(cfg.HeaderName)

			requestToken := formToken
			if requestToken == "" {
				requestToken = headerToken
			}

			if requestToken == "" {
				logCSRFFailure(r, "CSRF token not provided")
				writeCSRFForbidden(w, "CSRF token not provided")
				return
			}

			if subtle.ConstantTimeCompare([]byte(requestToken), []byte(cookieToken)) != 1 {
				logCSRFFailure(r, "CSRF token validation failed")
				writeCSRFForbidden(w, "CSRF token validation failed")
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// writeCSRFForbidden writes the canonical CSRF-rejection body using the
// CSRF_FAILED error code from AI.md PART 14's standard error code table.
func writeCSRFForbidden(w http.ResponseWriter, message string) {
	writeAPIError(w, http.StatusForbidden, "CSRF_FAILED", message)
}

// hasBearerCredential reports whether the caller presented a credential the
// browser does not attach on its own. AI.md PART 16 bypasses CSRF for these,
// because a cross-site page cannot make the victim's browser send them.
func hasBearerCredential(r *http.Request) bool {
	if strings.HasPrefix(r.Header.Get("Authorization"), "Bearer ") {
		return true
	}

	for _, header := range []string{"X-API-Token", "X-Api-Key", "X-Auth-Token"} {
		if r.Header.Get(header) != "" {
			return true
		}
	}

	return false
}

// isWebSocketUpgrade reports whether this is a WebSocket handshake, which
// AI.md PART 16 exempts because the handshake is authenticated by the
// subsequent protocol rather than by a form post.
func isWebSocketUpgrade(r *http.Request) bool {
	return strings.EqualFold(r.Header.Get("Upgrade"), "websocket")
}

// isPublicEndpoint reports whether the endpoint requires no authentication at
// all, so there is no session for an attacker to ride.
func isPublicEndpoint(requestPath string) bool {
	publicPrefixes := []string{
		"/server/healthz",
		"/healthz",
		"/metrics",
		"/openapi",
	}
	for _, prefix := range publicPrefixes {
		if strings.HasPrefix(requestPath, prefix) {
			return true
		}
	}

	return false
}

// matchesExemptPath reports whether the request path is in the configured
// web.csrf.exempt_paths allow-list. A pattern ending in /* also matches every
// deeper path beneath it, so /api/*/webhooks/* covers nested webhook routes.
func matchesExemptPath(requestPath string, patterns []string) bool {
	requestSegments := strings.Split(strings.Trim(requestPath, "/"), "/")

	for _, pattern := range patterns {
		if pattern == "" {
			continue
		}
		patternSegments := strings.Split(strings.Trim(pattern, "/"), "/")
		trailingWildcard := patternSegments[len(patternSegments)-1] == "*"
		if len(requestSegments) < len(patternSegments) ||
			(len(requestSegments) > len(patternSegments) && !trailingWildcard) {
			continue
		}

		matched := true
		for i, segment := range patternSegments {
			if ok, err := path.Match(segment, requestSegments[i]); err != nil || !ok {
				matched = false
				break
			}
		}
		if matched {
			return true
		}
	}

	return false
}

// generateCSRFToken generates a random CSRF token. A CSPRNG failure must fail
// closed rather than yield a predictable token, so the error is returned.
func generateCSRFToken(length int) (string, error) {
	b := make([]byte, length)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.URLEncoding.EncodeToString(b), nil
}

// setCSRFCookie sets the CSRF token cookie
func setCSRFCookie(w http.ResponseWriter, r *http.Request, cfg CSRFConfig, token string) {
	secure := false
	if cfg.Secure == "auto" {
		// Auto-detect based on scheme
		secure = r.TLS != nil
	} else if cfg.Secure == "true" {
		secure = true
	}

	// AI.md PART 16 requires SameSite to be set explicitly rather than left
	// to the browser default, and the token cookie to be Strict. MaxAge is
	// omitted so the cookie lives as long as the browsing session: a fixed
	// one-hour expiry silently invalidated tokens already rendered into open
	// forms, which surfaced as spurious 403s on long-lived admin pages.
	http.SetCookie(w, &http.Cookie{
		Name:     cfg.CookieName,
		Value:    token,
		Path:     "/",
		Domain:   "",
		Secure:   secure,
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
	})
}

// RegenerateCSRFToken regenerates the CSRF token and must be called on login.
// AI.md PART 16 requires the token to change on login, logout, and privilege change.
func RegenerateCSRFToken(w http.ResponseWriter, r *http.Request, cfg CSRFConfig) error {
	token, err := generateCSRFToken(cfg.TokenLength)
	if err != nil {
		return err
	}
	setCSRFCookie(w, r, cfg, token)
	ctx := reqctx.SetValue(r.Context(), "csrf_token", token)
	*r = *r.WithContext(ctx)
	return nil
}

// logCSRFFailure logs CSRF validation failure to audit log
// Per AI.md PART 11: All security events must be logged
func logCSRFFailure(r *http.Request, reason string) {
	// Get audit logger from context
	if auditLogger, exists := reqctx.GetValue(r.Context(), "auditLogger"); exists {
		if logger, ok := auditLogger.(*service.AuditLogger); ok {
			logger.LogFailure(
				string(service.EventSecurityCSRFDetected),
				"security",
				"api",
				"",
				util.TrustedGetClientIP(r),
				reason,
				map[string]interface{}{
					"endpoint":   r.URL.Path,
					"method":     r.Method,
					"user_agent": r.UserAgent(),
				},
			)
		}
	}
}
