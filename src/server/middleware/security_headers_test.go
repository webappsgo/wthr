package middleware

import (
	"net/http"
	"net/http/httptest"
	"strings"
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
// header values AI.md PART 11 "Security Headers" mandates, not just "some CSP
// present". Wrong values silently defeat the protection.
func TestSecurityHeaders_ExactValues(t *testing.T) {
	tests := []struct {
		header string
		want   string
	}{
		{"X-Content-Type-Options", "nosniff"},
		{"X-Frame-Options", "SAMEORIGIN"},
		{"X-XSS-Protection", "1; mode=block"},
		{"Referrer-Policy", "strict-origin-when-cross-origin"},
		{"X-Permitted-Cross-Domain-Policies", "none"},
		{"Origin-Agent-Cluster", "?1"},
		{"Cross-Origin-Embedder-Policy", "unsafe-none"},
		{"Cross-Origin-Opener-Policy", "unsafe-none"},
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

	// Server header must be blanked (info disclosure).
	if got := w.Header().Get("Server"); got != "" {
		t.Errorf("Server header = %q, want empty (must not leak server software)", got)
	}
}

// TestSecurityHeaders_ReportingEndpoints verifies the Reporting API headers
// AI.md PART 11 requires on every response, all pointing at the same group.
func TestSecurityHeaders_ReportingEndpoints(t *testing.T) {
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Host = "wthr.example.com"
	SecurityHeaders()(stubOKHandler).ServeHTTP(w, req)

	wantEndpoint := "http://wthr.example.com/api/v1/server/reports/default"
	if got := w.Header().Get("Reporting-Endpoints"); got != `default="`+wantEndpoint+`"` {
		t.Errorf("Reporting-Endpoints = %q, want default=%q", got, wantEndpoint)
	}
	if got := w.Header().Get("Report-To"); !strings.Contains(got, wantEndpoint) {
		t.Errorf("Report-To = %q, want it to contain %q", got, wantEndpoint)
	}
	if got := w.Header().Get("NEL"); got != `{"report_to":"default","max_age":2592000,"include_subdomains":true}` {
		t.Errorf("NEL = %q, want the default reporting policy", got)
	}
}

// TestSecurityHeaders_RejectsForgedHost ensures a Host header carrying header
// injection characters is never reflected into the reporting headers.
func TestSecurityHeaders_RejectsForgedHost(t *testing.T) {
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Host = `evil.example.com" , attacker="http://evil`
	SecurityHeaders()(stubOKHandler).ServeHTTP(w, req)

	if got := w.Header().Get("Reporting-Endpoints"); got != "" {
		t.Errorf("Reporting-Endpoints = %q for an unsafe Host, want empty", got)
	}
	if got := w.Header().Get("Report-To"); got != "" {
		t.Errorf("Report-To = %q for an unsafe Host, want empty", got)
	}
}

// TestSecurityHeaders_PermissionsPolicy verifies the generated policy locks
// sensors and advertising proposals while leaving the features this project
// actually uses scoped to self (AI.md PART 11 Permissions-Policy).
func TestSecurityHeaders_PermissionsPolicy(t *testing.T) {
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	SecurityHeaders()(stubOKHandler).ServeHTTP(w, req)

	policy := w.Header().Get("Permissions-Policy")
	for _, want := range []string{
		"camera=()",
		"microphone=()",
		"usb=()",
		"attribution-reporting=()",
		"browsing-topics=()",
		"interest-cohort=()",
		// IDEA.md declares IP/browser location as a product feature, so
		// geolocation is scoped to self rather than locked.
		"geolocation=(self)",
		"payment=(self)",
		"publickey-credentials-get=(self)",
	} {
		if !strings.Contains(policy, want) {
			t.Errorf("Permissions-Policy = %q, want it to contain %q", policy, want)
		}
	}
}

// TestSecurityHeaders_CSPDirectives verifies each directive of the AI.md
// PART 11 default policy is present, including the ones whose absence is a
// real vulnerability (object-src, frame-ancestors, base-uri, form-action).
func TestSecurityHeaders_CSPDirectives(t *testing.T) {
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	SecurityHeaders()(stubOKHandler).ServeHTTP(w, req)

	csp := w.Header().Get("Content-Security-Policy")
	if csp == "" {
		csp = w.Header().Get("Content-Security-Policy-Report-Only")
	}
	for _, want := range []string{
		"default-src 'self'",
		"script-src 'self'",
		"style-src 'self' 'unsafe-inline'",
		"img-src 'self' data: blob: https:",
		"media-src 'self' blob:",
		"worker-src 'self' blob:",
		"manifest-src 'self'",
		"frame-src 'self'",
		"frame-ancestors 'self'",
		"base-uri 'self'",
		"form-action 'self'",
		"object-src 'none'",
		"report-to default",
		"report-uri /api/v1/server/reports/csp",
	} {
		if !strings.Contains(csp, want) {
			t.Errorf("CSP = %q, want it to contain %q", csp, want)
		}
	}
}

// TestSecurityHeaders_CSPReportOnlyMode verifies the report-only mode switches
// the header name without changing the policy itself.
func TestSecurityHeaders_CSPReportOnlyMode(t *testing.T) {
	cfg := DefaultSecurityHeadersConfig()
	cfg.CSP.Mode = "report-only"

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	SecurityHeadersWithConfig(cfg)(stubOKHandler).ServeHTTP(w, req)

	if got := w.Header().Get("Content-Security-Policy"); got != "" {
		t.Errorf("Content-Security-Policy = %q in report-only mode, want empty", got)
	}
	if got := w.Header().Get("Content-Security-Policy-Report-Only"); got == "" {
		t.Error("Content-Security-Policy-Report-Only is empty in report-only mode")
	}
}

// TestSecurityHeaders_CSPExtraSources verifies operator-configured extras are
// appended to the spec default rather than replacing it.
func TestSecurityHeaders_CSPExtraSources(t *testing.T) {
	cfg := DefaultSecurityHeadersConfig()
	cfg.CSP.ScriptSrcExtra = "https://cdn.example.com"

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	SecurityHeadersWithConfig(cfg)(stubOKHandler).ServeHTTP(w, req)

	csp := w.Header().Get("Content-Security-Policy")
	if !strings.Contains(csp, "script-src 'self' https://cdn.example.com") {
		t.Errorf("CSP = %q, want script-src to keep 'self' and append the extra", csp)
	}
}

// TestSecurityHeaders_HSTS ensures HSTS uses the two-year preload value and is
// only set for requests that genuinely arrived over TLS — a forged
// X-Forwarded-Proto from an untrusted peer must not pin HSTS on the host.
func TestSecurityHeaders_HSTS(t *testing.T) {
	t.Run("plain HTTP - HSTS absent", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		SecurityHeaders()(stubOKHandler).ServeHTTP(w, req)

		if got := w.Header().Get("Strict-Transport-Security"); got != "" {
			t.Errorf("HSTS = %q on plain HTTP, want empty", got)
		}
	})

	t.Run("forwarded proto from untrusted peer - HSTS absent", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		// TEST-NET-1, outside every trusted range.
		req.RemoteAddr = "203.0.113.7:41234"
		req.Header.Set("X-Forwarded-Proto", "https")
		SecurityHeaders()(stubOKHandler).ServeHTTP(w, req)

		if got := w.Header().Get("Strict-Transport-Security"); got != "" {
			t.Errorf("HSTS = %q for a forged forwarded proto, want empty", got)
		}
	})

	t.Run("forwarded proto from trusted peer - HSTS present", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.RemoteAddr = "127.0.0.1:41234"
		req.Header.Set("X-Forwarded-Proto", "https")
		SecurityHeaders()(stubOKHandler).ServeHTTP(w, req)

		if got := w.Header().Get("Strict-Transport-Security"); got != hstsValue {
			t.Errorf("HSTS = %q, want %q", got, hstsValue)
		}
	})
}

// TestSecurityHeadersAPI_ExactValues checks the API-specific variant, which is
// intentionally more restrictive (default-src 'none', no-store caching) while
// still carrying the mandatory PART 11 header set.
func TestSecurityHeadersAPI_ExactValues(t *testing.T) {
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	SecurityHeadersAPI()(stubOKHandler).ServeHTTP(w, req)

	tests := []struct {
		header string
		want   string
	}{
		{"X-Frame-Options", "DENY"},
		{"Content-Security-Policy", "default-src 'none'; frame-ancestors 'none'"},
		{"Cache-Control", "no-store, no-cache, must-revalidate, private"},
		{"Pragma", "no-cache"},
		{"Expires", "0"},
		{"Referrer-Policy", "no-referrer"},
		{"X-Content-Type-Options", "nosniff"},
		{"X-Permitted-Cross-Domain-Policies", "none"},
		{"Origin-Agent-Cluster", "?1"},
		{"Cross-Origin-Resource-Policy", "cross-origin"},
	}
	for _, tt := range tests {
		t.Run(tt.header, func(t *testing.T) {
			if got := w.Header().Get(tt.header); got != tt.want {
				t.Errorf("header %s = %q, want %q", tt.header, got, tt.want)
			}
		})
	}

	if got := w.Header().Get("Permissions-Policy"); got == "" {
		t.Error("Permissions-Policy is empty on API responses, want the generated policy")
	}
}
