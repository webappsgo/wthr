// Package middleware - Security middleware per AI.md PART 22
// AI.md Reference: Lines 17693-19697 (Security & Logging)
package middleware

import (
	"crypto/rand"
	"encoding/base64"
	"net/http"

	"github.com/webappsgo/wthr/src/server/service"
	"github.com/gin-gonic/gin"
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
func CSRFProtection(cfg CSRFConfig) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Skip if disabled
		if !cfg.Enabled {
			c.Next()
			return
		}

		// Always generate/provide token for templates
		token, err := c.Cookie(cfg.CookieName)
		if err != nil || token == "" {
			token = generateCSRFToken(cfg.TokenLength)
			setCSRFCookie(c, cfg, token)
		}
		c.Set("csrf_token", token)

		// GET, HEAD, OPTIONS are safe methods - no validation needed
		if c.Request.Method == "GET" || c.Request.Method == "HEAD" || c.Request.Method == "OPTIONS" {
			c.Next()
			return
		}

		// Skip CSRF for public API endpoints (they use API tokens instead)
		path := c.Request.URL.Path
		if isPublicEndpoint(path) {
			c.Next()
			return
		}

		// Skip CSRF for unauthenticated users on public pages
		// CSRF protects against session hijacking - no session = no risk
		if _, exists := c.Get("user_id"); !exists {
			if _, adminExists := c.Get("admin_id"); !adminExists {
				c.Next()
				return
			}
		}

		// Validate CSRF token for authenticated state-changing requests
		cookieToken, err := c.Cookie(cfg.CookieName)
		if err != nil || cookieToken == "" {
			logCSRFFailure(c, "CSRF token missing")
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
				"error": "CSRF token missing",
			})
			return
		}

		// Check token from form or header
		formToken := c.PostForm("csrf_token")
		headerToken := c.GetHeader(cfg.HeaderName)

		requestToken := formToken
		if requestToken == "" {
			requestToken = headerToken
		}

		if requestToken == "" {
			logCSRFFailure(c, "CSRF token not provided")
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
				"error": "CSRF token not provided",
			})
			return
		}

		if requestToken != cookieToken {
			logCSRFFailure(c, "CSRF token validation failed")
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
				"error": "CSRF token validation failed",
			})
			return
		}

		c.Next()
	}
}

// isPublicEndpoint checks if the endpoint is a public API that doesn't need CSRF
func isPublicEndpoint(path string) bool {
	// Public API endpoints use API tokens for auth, not sessions
	publicPrefixes := []string{
		"/api/",
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
func setCSRFCookie(c *gin.Context, cfg CSRFConfig, token string) {
	secure := false
	if cfg.Secure == "auto" {
		// Auto-detect based on scheme
		secure = c.Request.TLS != nil
	} else if cfg.Secure == "true" {
		secure = true
	}

	c.SetCookie(
		cfg.CookieName,
		token,
		3600,
		"/",
		"",
		secure,
		true,
	)
}

// RegenerateCSRFToken regenerates CSRF token (call on login)
// Per AI.md line 14806: "Tokens regenerated on login"
func RegenerateCSRFToken(c *gin.Context, cfg CSRFConfig) {
	token := generateCSRFToken(cfg.TokenLength)
	setCSRFCookie(c, cfg, token)
	c.Set("csrf_token", token)
}

// logCSRFFailure logs CSRF validation failure to audit log
// Per AI.md PART 11: All security events must be logged
func logCSRFFailure(c *gin.Context, reason string) {
	// Get audit logger from context
	if auditLogger, exists := c.Get("auditLogger"); exists {
		if logger, ok := auditLogger.(*service.AuditLogger); ok {
			logger.LogFailure(
				string(service.EventSecurityCSRFDetected),
				"security",
				"api",
				"",
				c.ClientIP(),
				reason,
				map[string]interface{}{
					"endpoint":   c.Request.URL.Path,
					"method":     c.Request.Method,
					"user_agent": c.Request.UserAgent(),
				},
			)
		}
	}
}
