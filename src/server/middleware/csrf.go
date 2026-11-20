// Package middleware - CSRF protection per AI.md PART 16 (Web Frontend -> CSRF Protection)
package middleware

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"net/http"
	"path"
	"strings"

	"github.com/webappsgo/wthr/src/config"
	"github.com/webappsgo/wthr/src/secret"
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
		// AI.md PART 16: exempt_paths is for frontend paths only; the backend
		// (/api/**, compatibility layers) is outside CSRF scope and needs no entry.
		ExemptPaths: []string{},
	}
}

// CurrentCSRFConfig returns the live web.csrf settings, falling back to the defaults
// before any configuration is installed (tests, early startup).
func CurrentCSRFConfig() CSRFConfig {
	cfg := config.GetGlobalConfig()
	if cfg == nil {
		return DefaultCSRFConfig()
	}
	web := cfg.Web.CSRF
	return CSRFConfig{
		Enabled:     web.Enabled,
		TokenLength: web.TokenLength,
		CookieName:  web.CookieName,
		HeaderName:  web.HeaderName,
		Secure:      web.Secure,
		ExemptPaths: web.ExemptPaths,
	}
}

// CSRFProtectionLive applies CSRF protection with the live web.csrf settings, so an
// admin change takes effect on the next request without a restart (AI.md PART 0 "Live Reload").
func CSRFProtectionLive() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			CSRFProtection(CurrentCSRFConfig())(next).ServeHTTP(w, r)
		})
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

			// AI.md PART 16 "Scope - Frontend Only": CSRF never applies to the backend.
			// No token is issued, required or checked on /api/** or the MCP aliases;
			// SameSite=Strict cookies and CORS still stop cross-site browsers.
			if isBackendRoute(r.URL.Path) {
				next.ServeHTTP(w, r)
				return
			}

			// AI.md PART 16: the token is regenerated on login, logout and privilege change, so a
			// response that sets or clears a session cookie also issues a fresh token
			w = &csrfSessionWriter{ResponseWriter: w, request: r, config: cfg}

			// Always generate/provide token for templates
			tokenCookie, err := r.Cookie(cfg.CookieName)
			token := ""
			if err == nil {
				token = tokenCookie.Value
			}
			// A token that does not verify under csrf_token_secret (forged, or signed before a
			// rotation) is replaced for rendering and never accepted for validation
			if token != "" && !isCSRFTokenAuthentic(token) {
				token = ""
			}
			if token == "" {
				generated, err := newCSRFToken(cfg.TokenLength)
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

			// AI.md PART 11 Sec-Fetch-* layer: a cross-site state-changing request to a
			// frontend route is rejected outright
			if strings.EqualFold(r.Header.Get("Sec-Fetch-Site"), "cross-site") {
				logCSRFFailure(r, "cross-site request rejected by Sec-Fetch-Site")
				writeCSRFForbidden(w, "Cross-site request rejected")
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

			if subtle.ConstantTimeCompare([]byte(requestToken), []byte(cookieToken)) != 1 || !isCSRFTokenAuthentic(cookieToken) {
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
	return strings.HasPrefix(r.Header.Get("Authorization"), "Bearer ")
}

// isBackendRoute reports whether the path belongs to the backend (every /api/**
// route and the MCP aliases), which is outside CSRF scope (AI.md PART 16).
func isBackendRoute(requestPath string) bool {
	return requestPath == "/api" || strings.HasPrefix(requestPath, "/api/") ||
		requestPath == "/mcp" || strings.HasPrefix(requestPath, "/mcp/")
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
		"/server/docs",
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

// sessionCookieNames are the browser session cookies whose change means a login or a logout.
func sessionCookieNames() []string {
	return []string{UserSessionCookieName(), AdminSessionCookieName()}
}

// csrfSessionWriter issues a new CSRF token on a response that sets or clears a session cookie.
type csrfSessionWriter struct {
	http.ResponseWriter
	request     *http.Request
	config      CSRFConfig
	wroteHeader bool
}

// WriteHeader checks the Set-Cookie headers once, just before the status line goes out.
func (c *csrfSessionWriter) WriteHeader(status int) {
	if !c.wroteHeader {
		c.wroteHeader = true
		c.rotateOnSessionChange()
	}
	c.ResponseWriter.WriteHeader(status)
}

// Write sends the implicit 200 through WriteHeader so the check also runs for it.
func (c *csrfSessionWriter) Write(body []byte) (int, error) {
	if !c.wroteHeader {
		c.WriteHeader(http.StatusOK)
	}
	return c.ResponseWriter.Write(body)
}

// Unwrap exposes the underlying writer so http.ResponseController, Flush and Hijack still work.
func (c *csrfSessionWriter) Unwrap() http.ResponseWriter { return c.ResponseWriter }

// rotateOnSessionChange sets a fresh CSRF cookie when the response carries a session cookie
// and does not already carry a CSRF cookie of its own.
func (c *csrfSessionWriter) rotateOnSessionChange() {
	changed := false
	for _, header := range c.ResponseWriter.Header().Values("Set-Cookie") {
		name, _, _ := strings.Cut(header, "=")
		name = strings.TrimSpace(name)
		if name == c.config.CookieName {
			return
		}
		for _, session := range sessionCookieNames() {
			if name == session {
				changed = true
			}
		}
	}
	if !changed {
		return
	}
	if token, err := newCSRFToken(c.config.TokenLength); err == nil {
		setCSRFCookie(c.ResponseWriter, c.request, c.config, token)
	}
}

// csrfSignatureSeparator splits the random part of a token from its HMAC.
const csrfSignatureSeparator = "."

// newCSRFToken returns a double-submit token: random bytes plus an HMAC-SHA256 over them
// under csrf_token_secret (AI.md PART 11 "Other Project-Level Secrets"), so a token an
// attacker plants in a cookie cannot be forged. Before a secret store exists (tests,
// early startup) the token is the random part alone.
func newCSRFToken(length int) (string, error) {
	random, err := generateCSRFToken(length)
	if err != nil {
		return "", err
	}
	store := secret.Default()
	if store == nil {
		return random, nil
	}
	key, err := store.Current(secret.CSRFTokenSecret)
	if err != nil {
		return "", err
	}
	return random + csrfSignatureSeparator + signCSRFRandom(key, random), nil
}

// signCSRFRandom returns the URL-safe HMAC of a token's random part.
func signCSRFRandom(key []byte, random string) string {
	mac := hmac.New(sha256.New, key)
	mac.Write([]byte(random))
	return base64.URLEncoding.EncodeToString(mac.Sum(nil))
}

// isCSRFTokenAuthentic verifies the HMAC of a token under the current secret and, during
// rotation, the previous one. Without a secret store every token passes this check.
func isCSRFTokenAuthentic(token string) bool {
	store := secret.Default()
	if store == nil {
		return true
	}
	random, signature, found := strings.Cut(token, csrfSignatureSeparator)
	if !found || random == "" || signature == "" {
		return false
	}
	candidates, err := store.Candidates(secret.CSRFTokenSecret)
	if err != nil {
		return false
	}
	for _, key := range candidates {
		if subtle.ConstantTimeCompare([]byte(signCSRFRandom(key, random)), []byte(signature)) == 1 {
			return true
		}
	}
	return false
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
		Name:   cfg.CookieName,
		Value:  token,
		Path:   "/",
		Domain: "",
		Secure: secure,
		// AI.md PART 16 "Implementation Rules": HttpOnly=false, because the page script reads
		// the token to send it back in the X-CSRF-Token header
		HttpOnly: false,
		SameSite: http.SameSiteStrictMode,
	})
}

// RegenerateCSRFToken regenerates the CSRF token and must be called on login.
// AI.md PART 16 requires the token to change on login, logout, and privilege change.
func RegenerateCSRFToken(w http.ResponseWriter, r *http.Request, cfg CSRFConfig) error {
	token, err := newCSRFToken(cfg.TokenLength)
	if err != nil {
		return err
	}
	setCSRFCookie(w, r, cfg, token)
	ctx := reqctx.SetValue(r.Context(), "csrf_token", token)
	*r = *r.WithContext(ctx)
	return nil
}

// RegenerateCSRFTokenLive regenerates the token with the live web.csrf settings. Login,
// logout and every privilege change call it, so a token never survives the change it guards.
func RegenerateCSRFTokenLive(w http.ResponseWriter, r *http.Request) error {
	return RegenerateCSRFToken(w, r, CurrentCSRFConfig())
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
