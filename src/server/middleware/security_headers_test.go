package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// stubOKHandler is the next handler used across these tests — it never gets
// to write anything itself, so any status/body assertions below reflect only
// what the middleware under test wrote to the ResponseWriter.
var stubOKHandler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok"))
})

// TestSecurityHeaders_ExactValues verifies SecurityHeaders sets the exact
// documented header values (AI.md: XSS/clickjacking/MIME-sniffing/CORP
// protections), not just "some CSP present". Wrong values silently defeat
// the protection.
func TestSecurityHeaders_ExactValues(t *testing.T) {
	tests := []struct {
		header string
		want   string
	}{
		{"X-Content-Type-Options", "nosniff"},
		{"X-Frame-Options", "SAMEORIGIN"},
		{"X-XSS-Protection", "1; mode=block"},
		{"Referrer-Policy", "strict-origin-when-cross-origin"},
		{"Permissions-Policy", "geolocation=(), microphone=(), camera=(), payment=()"},
		{"Cross-Origin-Embedder-Policy", "credentialless"},
		{"Cross-Origin-Opener-Policy", "same-origin"},
		{"Cross-Origin-Resource-Policy", "cross-origin"},
	}

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	SecurityHeaders()(stubOKHandler).ServeHTTP(w, req)

	for _, tt := range tests {
		t.Run(tt.header, func(t *testing.T) {
			got := w.Header().Get(tt.header)
			if got != tt.want {
				t.Errorf("header %s = %q, want %q", tt.header, got, tt.want)
			}
		})
	}

	// CSP must reference 'self' as default-src; a regression that widens
	// this (e.g. to '*') would be a real security bug.
	csp := w.Header().Get("Content-Security-Policy")
	if csp == "" || !containsSubstr(csp, "default-src 'self'") {
		t.Errorf("CSP = %q, want it to contain default-src 'self'", csp)
	}

	// Server header must be blanked (info disclosure).
	if got := w.Header().Get("Server"); got != "" {
		t.Errorf("Server header = %q, want empty (must not leak server software)", got)
	}
}

// TestSecurityHeaders_HSTSOnlyOnHTTPS ensures HSTS is only set when the
// request is detected as HTTPS (TLS or X-Forwarded-Proto), never on plain
// HTTP - setting it unconditionally would be a bug (breaks local HTTP dev).
func TestSecurityHeaders_HSTSOnlyOnHTTPS(t *testing.T) {
	t.Run("no forwarded proto - HSTS absent", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		SecurityHeaders()(stubOKHandler).ServeHTTP(w, req)

		if got := w.Header().Get("Strict-Transport-Security"); got != "" {
			t.Errorf("HSTS = %q on plain HTTP, want empty", got)
		}
	})

	t.Run("X-Forwarded-Proto https - HSTS present", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Header.Set("X-Forwarded-Proto", "https")
		SecurityHeaders()(stubOKHandler).ServeHTTP(w, req)

		want := "max-age=31536000; includeSubDomains"
		if got := w.Header().Get("Strict-Transport-Security"); got != want {
			t.Errorf("HSTS = %q, want %q", got, want)
		}
	})
}

// TestSecurityHeadersAPI_ExactValues checks the API-specific variant, which
// is intentionally more restrictive (default-src 'none', no-store caching).
func TestSecurityHeadersAPI_ExactValues(t *testing.T) {
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	SecurityHeadersAPI()(stubOKHandler).ServeHTTP(w, req)

	tests := []struct {
		header string
		want   string
	}{
		{"X-Frame-Options", "DENY"},
		{"Content-Security-Policy", "default-src 'none'"},
		{"Cache-Control", "no-store, no-cache, must-revalidate, private"},
		{"Pragma", "no-cache"},
		{"Expires", "0"},
		{"Referrer-Policy", "no-referrer"},
	}
	for _, tt := range tests {
		t.Run(tt.header, func(t *testing.T) {
			if got := w.Header().Get(tt.header); got != tt.want {
				t.Errorf("header %s = %q, want %q", tt.header, got, tt.want)
			}
		})
	}
}

func containsSubstr(s, substr string) bool {
	return len(s) >= len(substr) && (func() bool {
		for i := 0; i+len(substr) <= len(s); i++ {
			if s[i:i+len(substr)] == substr {
				return true
			}
		}
		return false
	})()
}
