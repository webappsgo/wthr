package middleware

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/webappsgo/wthr/src/server/reqctx"
)

// TestCSRFProtection_SkipsWhenDisabled verifies the config.Enabled=false
// escape hatch is a true no-op (no cookie set, no validation).
func TestCSRFProtection_SkipsWhenDisabled(t *testing.T) {
	cfg := DefaultCSRFConfig()
	cfg.Enabled = false

	handler := CSRFProtection(cfg)(okHandler)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/mutate", nil)
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200 when CSRF disabled", w.Code)
	}
}

// TestCSRFProtection_SkipsSafeMethods verifies GET/HEAD/OPTIONS never
// require a token, per AI.md PART 22 - safe methods must not be blocked.
func TestCSRFProtection_SkipsSafeMethods(t *testing.T) {
	cfg := DefaultCSRFConfig()

	for _, method := range []string{http.MethodGet, http.MethodHead, http.MethodOptions} {
		t.Run(method, func(t *testing.T) {
			handler := CSRFProtection(cfg)(okHandler)

			w := httptest.NewRecorder()
			req := httptest.NewRequest(method, "/page", nil)
			handler.ServeHTTP(w, req)

			if w.Code != http.StatusOK {
				t.Errorf("%s: status = %d, want 200", method, w.Code)
			}
		})
	}
}

// TestCSRFProtection_SkipsPublicAPIEndpoints verifies /api/, /healthz,
// /metrics, /openapi paths are exempted since they authenticate via API
// tokens rather than cookies.
func TestCSRFProtection_SkipsPublicAPIEndpoints(t *testing.T) {
	cfg := DefaultCSRFConfig()

	handler := CSRFProtection(cfg)(okHandler)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/tokens", nil)
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200 for public API path", w.Code)
	}
}

// TestCSRFProtection_SkipsTrulyUnauthenticatedRequests verifies that a
// mutating request with neither "user_id" nor "admin_id" set in context is
// treated as unauthenticated and exempted (no session = no CSRF risk).
func TestCSRFProtection_SkipsTrulyUnauthenticatedRequests(t *testing.T) {
	cfg := DefaultCSRFConfig()

	handler := CSRFProtection(cfg)(okHandler)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/mutate", nil)
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200 for unauthenticated mutating request", w.Code)
	}
}

// TestCSRFProtection_RejectsAuthenticatedAdminWithoutToken verifies an
// admin-authenticated mutating request without any CSRF cookie is rejected
// with 403 - this is the one authenticated path the gate correctly covers
// today (admin_auth.go does set "admin_id").
func TestCSRFProtection_RejectsAuthenticatedAdminWithoutToken(t *testing.T) {
	cfg := DefaultCSRFConfig()

	handler := CSRFProtection(cfg)(okHandler)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/mutate", nil)
	ctx := reqctx.Set(req.Context(), "admin_id", 1)
	req = req.WithContext(ctx)
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Errorf("status = %d, want 403 for admin-authenticated request with no CSRF cookie", w.Code)
	}
}

// TestCSRFProtection_RejectsMismatchedToken verifies a request carrying a
// CSRF cookie but a different form/header token is rejected.
func TestCSRFProtection_RejectsMismatchedToken(t *testing.T) {
	cfg := DefaultCSRFConfig()

	handler := CSRFProtection(cfg)(okHandler)

	form := url.Values{"csrf_token": {"wrong-token"}}
	req := httptest.NewRequest(http.MethodPost, "/mutate", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: cfg.CookieName, Value: "correct-token"})
	ctx := reqctx.Set(req.Context(), "admin_id", 1)
	req = req.WithContext(ctx)

	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Errorf("status = %d, want 403 for mismatched CSRF token", w.Code)
	}
}

// TestCSRFProtection_AcceptsMatchingHeaderToken verifies a request whose
// X-CSRF-Token header matches the cookie is accepted.
func TestCSRFProtection_AcceptsMatchingHeaderToken(t *testing.T) {
	cfg := DefaultCSRFConfig()

	handler := CSRFProtection(cfg)(okHandler)

	req := httptest.NewRequest(http.MethodPost, "/mutate", nil)
	req.Header.Set(cfg.HeaderName, "matching-token")
	req.AddCookie(&http.Cookie{Name: cfg.CookieName, Value: "matching-token"})
	ctx := reqctx.Set(req.Context(), "admin_id", 1)
	req = req.WithContext(ctx)

	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200 for matching CSRF header/cookie", w.Code)
	}
}

// TestCSRFProtection_SessionAuthenticatedRegularUserIsProtected is a
// regression test for a real production bug: CSRFProtection gated its
// authenticated-request check on a bare "user_id" context key that no
// middleware in this package ever set, so it treated every
// session-authenticated regular user as unauthenticated and skipped CSRF
// validation entirely for their mutating requests (POST/PUT/PATCH/DELETE to
// /users/... routes), even with no CSRF cookie or token present at all.
//
// The gate now keys on UserContextKey, and AuthMiddleware sets both
// UserContextKey and UserIDContextKey at every authentication site. This test
// mirrors that pair and asserts the mutating request is rejected per AI.md
// PART 16 - CSRF Protection.
func TestCSRFProtection_SessionAuthenticatedRegularUserIsProtected(t *testing.T) {
	cfg := DefaultCSRFConfig()

	handler := CSRFProtection(cfg)(okHandler)

	req := httptest.NewRequest(http.MethodPost, "/users/settings", nil)
	// Mirrors what auth.go's AuthMiddleware sets for a session-authenticated
	// regular user: both the user object and its numeric id.
	ctx := reqctx.Set(req.Context(), UserContextKey, "some-authenticated-user")
	ctx = reqctx.Set(ctx, UserIDContextKey, 7)
	req = req.WithContext(ctx)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Errorf("status = %d, want 403 - a session-authenticated regular user with no CSRF "+
			"token must be rejected; if this fails, AuthMiddleware has stopped setting "+
			"UserIDContextKey and regular users bypass CSRF on every mutating request", w.Code)
	}
}
