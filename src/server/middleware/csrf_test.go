package middleware

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

// TestCSRFProtection_SkipsWhenDisabled verifies the config.Enabled=false
// escape hatch is a true no-op (no cookie set, no validation).
func TestCSRFProtection_SkipsWhenDisabled(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cfg := DefaultCSRFConfig()
	cfg.Enabled = false

	router := gin.New()
	router.Use(CSRFProtection(cfg))
	router.POST("/mutate", func(c *gin.Context) { c.String(http.StatusOK, "ok") })

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/mutate", nil)
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200 when CSRF disabled", w.Code)
	}
}

// TestCSRFProtection_SkipsSafeMethods verifies GET/HEAD/OPTIONS never
// require a token, per AI.md PART 22 - safe methods must not be blocked.
func TestCSRFProtection_SkipsSafeMethods(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cfg := DefaultCSRFConfig()

	for _, method := range []string{http.MethodGet, http.MethodHead, http.MethodOptions} {
		t.Run(method, func(t *testing.T) {
			router := gin.New()
			router.Use(CSRFProtection(cfg))
			router.Handle(method, "/page", func(c *gin.Context) { c.String(http.StatusOK, "ok") })

			w := httptest.NewRecorder()
			req := httptest.NewRequest(method, "/page", nil)
			router.ServeHTTP(w, req)

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
	gin.SetMode(gin.TestMode)
	cfg := DefaultCSRFConfig()

	router := gin.New()
	router.Use(CSRFProtection(cfg))
	router.POST("/api/v1/tokens", func(c *gin.Context) { c.String(http.StatusOK, "ok") })

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/tokens", nil)
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200 for public API path", w.Code)
	}
}

// TestCSRFProtection_SkipsTrulyUnauthenticatedRequests verifies that a
// mutating request with neither "user_id" nor "admin_id" set in context is
// treated as unauthenticated and exempted (no session = no CSRF risk).
func TestCSRFProtection_SkipsTrulyUnauthenticatedRequests(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cfg := DefaultCSRFConfig()

	router := gin.New()
	router.Use(CSRFProtection(cfg))
	router.POST("/mutate", func(c *gin.Context) { c.String(http.StatusOK, "ok") })

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/mutate", nil)
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200 for unauthenticated mutating request", w.Code)
	}
}

// TestCSRFProtection_RejectsAuthenticatedAdminWithoutToken verifies an
// admin-authenticated mutating request without any CSRF cookie is rejected
// with 403 - this is the one authenticated path the gate correctly covers
// today (admin_auth.go does set "admin_id").
func TestCSRFProtection_RejectsAuthenticatedAdminWithoutToken(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cfg := DefaultCSRFConfig()

	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("admin_id", 1)
		c.Next()
	})
	router.Use(CSRFProtection(cfg))
	router.POST("/mutate", func(c *gin.Context) { c.String(http.StatusOK, "ok") })

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/mutate", nil)
	router.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Errorf("status = %d, want 403 for admin-authenticated request with no CSRF cookie", w.Code)
	}
}

// TestCSRFProtection_RejectsMismatchedToken verifies a request carrying a
// CSRF cookie but a different form/header token is rejected.
func TestCSRFProtection_RejectsMismatchedToken(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cfg := DefaultCSRFConfig()

	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("admin_id", 1)
		c.Next()
	})
	router.Use(CSRFProtection(cfg))
	router.POST("/mutate", func(c *gin.Context) { c.String(http.StatusOK, "ok") })

	form := url.Values{"csrf_token": {"wrong-token"}}
	req := httptest.NewRequest(http.MethodPost, "/mutate", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: cfg.CookieName, Value: "correct-token"})

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Errorf("status = %d, want 403 for mismatched CSRF token", w.Code)
	}
}

// TestCSRFProtection_AcceptsMatchingHeaderToken verifies a request whose
// X-CSRF-Token header matches the cookie is accepted.
func TestCSRFProtection_AcceptsMatchingHeaderToken(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cfg := DefaultCSRFConfig()

	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("admin_id", 1)
		c.Next()
	})
	router.Use(CSRFProtection(cfg))
	router.POST("/mutate", func(c *gin.Context) { c.String(http.StatusOK, "ok") })

	req := httptest.NewRequest(http.MethodPost, "/mutate", nil)
	req.Header.Set(cfg.HeaderName, "matching-token")
	req.AddCookie(&http.Cookie{Name: cfg.CookieName, Value: "matching-token"})

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200 for matching CSRF header/cookie", w.Code)
	}
}

// TestCSRFProtection_SessionAuthenticatedRegularUserBypassesCSRF documents a
// real production bug: CSRFProtection (csrf.go:70-75) gates its
// authenticated-request check on c.Get("user_id"), but no middleware in this
// package ever sets that key. auth.go's AuthMiddleware sets the context key
// "user" (to a *models.User), never "user_id". The same is true for regular
// session-authenticated users going through RequireAuth/OptionalAuth - they
// never populate "user_id" or "admin_id".
//
// Consequence: CSRFProtection treats every session-authenticated regular
// user as if they were unauthenticated and skips CSRF validation entirely
// for their mutating requests (POST/PUT/PATCH/DELETE to /users/... routes),
// even with no CSRF cookie or token present at all. Regular users receive
// no CSRF protection whatsoever.
//
// This test encodes CORRECT expected behavior (a mutating request from a
// session-authenticated regular user, with no CSRF token supplied, must be
// rejected) and is expected to FAIL against the current implementation.
func TestCSRFProtection_SessionAuthenticatedRegularUserBypassesCSRF(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cfg := DefaultCSRFConfig()

	router := gin.New()
	router.Use(func(c *gin.Context) {
		// Mirrors what auth.go's AuthMiddleware actually sets for a
		// session-authenticated regular user: the "user" key, not "user_id".
		c.Set("user", "some-authenticated-user")
		c.Next()
	})
	router.Use(CSRFProtection(cfg))
	router.POST("/users/settings", func(c *gin.Context) { c.String(http.StatusOK, "ok") })

	req := httptest.NewRequest(http.MethodPost, "/users/settings", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Errorf("status = %d, want 403 - csrf.go:70-75 checks c.Get(\"user_id\"), a key no "+
			"middleware ever sets (auth.go sets \"user\"), so session-authenticated regular "+
			"users bypass CSRF validation entirely for every mutating request", w.Code)
	}
}
