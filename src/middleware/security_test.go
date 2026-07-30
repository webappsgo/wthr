// Tests for security.go per AI.md PART 22 (Security headers) / PART 29 (Testing)
package middleware

import (
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

// TestSecurityHeaders_AlwaysPresent verifies that every response, regardless
// of SSL setting, includes the baseline security headers with exact values
// per AI.md lines 17697-17706.
func TestSecurityHeaders_AlwaysPresent(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name       string
		sslEnabled bool
	}{
		{"ssl_disabled", false},
		{"ssl_enabled", true},
	}

	wantHeaders := map[string]string{
		"X-Content-Type-Options":   "nosniff",
		"X-Frame-Options":          "SAMEORIGIN",
		"X-XSS-Protection":         "1; mode=block",
		"Referrer-Policy":          "strict-origin-when-cross-origin",
		"Content-Security-Policy":  "default-src 'self'; script-src 'self' 'unsafe-inline'; style-src 'self' 'unsafe-inline'",
		"Permissions-Policy":       "geolocation=(), microphone=(), camera=()",
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := gin.New()
			r.Use(SecurityHeaders(tt.sslEnabled))
			r.GET("/", func(c *gin.Context) { c.String(200, "ok") })

			req := httptest.NewRequest("GET", "/", nil)
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)

			for header, want := range wantHeaders {
				got := w.Header().Get(header)
				if got != want {
					t.Errorf("header %s = %q, want %q", header, got, want)
				}
			}
		})
	}
}

// TestSecurityHeaders_HSTS verifies Strict-Transport-Security is set only
// when SSL is enabled, and absent otherwise — this is a conditional branch
// that a naive refactor could easily invert.
func TestSecurityHeaders_HSTS(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("ssl_enabled_sets_hsts", func(t *testing.T) {
		r := gin.New()
		r.Use(SecurityHeaders(true))
		r.GET("/", func(c *gin.Context) { c.String(200, "ok") })

		req := httptest.NewRequest("GET", "/", nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		want := "max-age=31536000; includeSubDomains"
		if got := w.Header().Get("Strict-Transport-Security"); got != want {
			t.Errorf("Strict-Transport-Security = %q, want %q", got, want)
		}
	})

	t.Run("ssl_disabled_omits_hsts", func(t *testing.T) {
		r := gin.New()
		r.Use(SecurityHeaders(false))
		r.GET("/", func(c *gin.Context) { c.String(200, "ok") })

		req := httptest.NewRequest("GET", "/", nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		if got := w.Header().Get("Strict-Transport-Security"); got != "" {
			t.Errorf("Strict-Transport-Security = %q, want empty when SSL disabled", got)
		}
	})
}

// TestSecurityHeaders_CallsNext verifies the middleware calls c.Next() so
// downstream handlers still run (a missing c.Next() would silently break
// every route using this middleware).
func TestSecurityHeaders_CallsNext(t *testing.T) {
	gin.SetMode(gin.TestMode)

	r := gin.New()
	r.Use(SecurityHeaders(false))

	handlerRan := false
	r.GET("/", func(c *gin.Context) {
		handlerRan = true
		c.String(200, "ok")
	})

	req := httptest.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if !handlerRan {
		t.Error("downstream handler did not run; SecurityHeaders may not call c.Next()")
	}
	if w.Code != 200 {
		t.Errorf("status = %d, want 200", w.Code)
	}
}
