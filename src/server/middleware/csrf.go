// Package middleware - Security middleware per AI.md PART 22
// AI.md Reference: Lines 17693-19697 (Security & Logging)
package middleware

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"net/http"

	"github.com/webappsgo/wthr/src/server/reqctx"
	"github.com/webappsgo/wthr/src/server/service"
	"github.com/webappsgo/wthr/src/util"
)

// CSRFConfig holds CSRF protection configuration
// Per AI.md lines 14787-14799
type CSRFConfig struct {
	Enabled     bool
	TokenLength int
	CookieName  string
	HeaderName  string
	Secure      string
}

// DefaultCSRFConfig returns default CSRF configuration
// Per AI.md lines 14791-14798
func DefaultCSRFConfig() CSRFConfig {
	return CSRFConfig{
		Enabled:     true,
		TokenLength: 32,
		CookieName:  "csrf_token",
		HeaderName:  "X-CSRF-Token",
		Secure:      "auto",
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
				token = generateCSRFToken(cfg.TokenLength)
				setCSRFCookie(w, r, cfg, token)
			}
			ctx := reqctx.Set(r.Context(), "csrf_token", token)
			r = r.WithContext(ctx)

			// GET, HEAD, OPTIONS are safe methods - no validation needed
			if r.Method == "GET" || r.Method == "HEAD" || r.Method == "OPTIONS" {
				next.ServeHTTP(w, r)
				return
			}

			// Skip CSRF for public API endpoints (they use API tokens instead)
			path := r.URL.Path
			if isPublicEndpoint(path) {
				next.ServeHTTP(w, r)
				return
			}

			// Skip CSRF for unauthenticated users on public pages
			// CSRF protects against session hijacking - no session = no risk
			if _, exists := reqctx.Get(r.Context(), UserContextKey); !exists {
				if _, adminExists := reqctx.Get(r.Context(), "admin_id"); !adminExists {
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

			if requestToken != cookieToken {
				logCSRFFailure(r, "CSRF token validation failed")
				writeCSRFForbidden(w, "CSRF token validation failed")
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// writeCSRFForbidden writes the canonical CSRF-rejection body, preserving the
// original ad-hoc {"error": "..."} shape rather than the newer canonical
// {"ok":false,"error":"CODE","message":"..."} shape, since this is a
// mechanical framework-API translation and must not change response bodies.
func writeCSRFForbidden(w http.ResponseWriter, message string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusForbidden)
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"error": message,
	})
}

// isPublicEndpoint checks if the endpoint is a public API that doesn't need CSRF
func isPublicEndpoint(path string) bool {
	// Public API endpoints use API tokens for auth, not sessions
	publicPrefixes := []string{
		"/api/",
		"/server/healthz",
		"/healthz",
		"/metrics",
		"/openapi",
	}
	for _, prefix := range publicPrefixes {
		if len(path) >= len(prefix) && path[:len(prefix)] == prefix {
			return true
		}
	}
	return false
}

// generateCSRFToken generates a random CSRF token
func generateCSRFToken(length int) string {
	b := make([]byte, length)
	rand.Read(b)
	return base64.URLEncoding.EncodeToString(b)
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

	http.SetCookie(w, &http.Cookie{
		Name:     cfg.CookieName,
		Value:    token,
		MaxAge:   3600,
		Path:     "/",
		Domain:   "",
		Secure:   secure,
		HttpOnly: true,
	})
}

// RegenerateCSRFToken regenerates CSRF token (call on login)
// Per AI.md line 14806: "Tokens regenerated on login"
func RegenerateCSRFToken(w http.ResponseWriter, r *http.Request, cfg CSRFConfig) {
	token := generateCSRFToken(cfg.TokenLength)
	setCSRFCookie(w, r, cfg, token)
	ctx := reqctx.Set(r.Context(), "csrf_token", token)
	*r = *r.WithContext(ctx)
}

// logCSRFFailure logs CSRF validation failure to audit log
// Per AI.md PART 11: All security events must be logged
func logCSRFFailure(r *http.Request, reason string) {
	// Get audit logger from context
	if auditLogger, exists := reqctx.Get(r.Context(), "auditLogger"); exists {
		if logger, ok := auditLogger.(*service.AuditLogger); ok {
			logger.LogFailure(
				string(service.EventSecurityCSRFDetected),
				"security",
				"api",
				"",
				util.GetClientIP(r),
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
