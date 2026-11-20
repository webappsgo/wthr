package middleware

import (
	"database/sql"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"

	_ "modernc.org/sqlite"

	"github.com/webappsgo/wthr/src/secret"
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
// require a token, per AI.md PART 16 - safe methods must not be blocked.
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

// TestCSRFProtection_SkipsUnauthenticatedPublicEndpoints verifies the
// endpoints AI.md PART 16 marks public are exempt, since there is no
// authenticated identity for a cross-site page to ride.
func TestCSRFProtection_SkipsUnauthenticatedPublicEndpoints(t *testing.T) {
	cfg := DefaultCSRFConfig()

	for _, requestPath := range []string{"/server/healthz", "/healthz", "/metrics", "/server/docs/swagger"} {
		t.Run(requestPath, func(t *testing.T) {
			handler := CSRFProtection(cfg)(okHandler)

			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, requestPath, nil)
			ctx := reqctx.SetValue(req.Context(), "admin_id", 1)
			req = req.WithContext(ctx)
			handler.ServeHTTP(w, req)

			if w.Code != http.StatusOK {
				t.Errorf("status = %d, want 200 for public endpoint %s", w.Code, requestPath)
			}
		})
	}
}

// TestCSRFProtection_BackendIsOutOfScope verifies AI.md PART 16 "Scope - Frontend Only":
// no /api/** route (or MCP alias) is ever CSRF-checked, even for a session-authenticated
// caller, and no token is issued there.
func TestCSRFProtection_BackendIsOutOfScope(t *testing.T) {
	cfg := DefaultCSRFConfig()
	handler := CSRFProtection(cfg)(okHandler)

	for _, p := range []string{"/api/v1/locations", "/api/mcp", "/mcp"} {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, p, nil)
		ctx := reqctx.SetValue(req.Context(), UserContextKey, "session-authenticated-user")
		ctx = reqctx.SetValue(ctx, UserIDContextKey, 7)
		handler.ServeHTTP(w, req.WithContext(ctx))

		if w.Code != http.StatusOK {
			t.Errorf("%s: status = %d, want 200 - the backend is never CSRF-checked", p, w.Code)
		}
		if len(w.Result().Cookies()) != 0 {
			t.Errorf("%s: no CSRF cookie may be issued on the backend", p)
		}
	}
}

// TestCSRFProtection_FrontendStillProtected keeps the frontend rule: a cookie-authenticated
// state-changing page request without a token is rejected, and a cross-site Sec-Fetch-Site
// is rejected even for an unauthenticated form post.
func TestCSRFProtection_FrontendStillProtected(t *testing.T) {
	cfg := DefaultCSRFConfig()
	handler := CSRFProtection(cfg)(okHandler)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/users/settings", nil)
	ctx := reqctx.SetValue(req.Context(), UserContextKey, "session-authenticated-user")
	handler.ServeHTTP(w, req.WithContext(ctx))
	if w.Code != http.StatusForbidden {
		t.Errorf("tokenless frontend POST status = %d, want 403", w.Code)
	}

	w = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/server/contact", nil)
	req.Header.Set("Sec-Fetch-Site", "cross-site")
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Errorf("cross-site frontend POST status = %d, want 403", w.Code)
	}

	w = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/server/contact", nil)
	req.Header.Set("Sec-Fetch-Site", "same-origin")
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("same-origin unauthenticated POST status = %d, want 200", w.Code)
	}
}

// TestCSRFProtection_BypassesBearerCredentials verifies the AI.md PART 16
// bypass rows that rest on the caller presenting a credential the browser
// never attaches on its own, plus the WebSocket upgrade row.
func TestCSRFProtection_BypassesBearerCredentials(t *testing.T) {
	cfg := DefaultCSRFConfig()

	cases := []struct {
		name   string
		header string
		value  string
	}{
		{"Authorization Bearer", "Authorization", "Bearer usr_abcdef0123456789abcdef0123456789"},
		{"WebSocket upgrade", "Upgrade", "websocket"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			handler := CSRFProtection(cfg)(okHandler)

			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, "/users/settings", nil)
			req.Header.Set(tc.header, tc.value)
			ctx := reqctx.SetValue(req.Context(), UserContextKey, "token-authenticated-user")
			req = req.WithContext(ctx)
			handler.ServeHTTP(w, req)

			if w.Code != http.StatusOK {
				t.Errorf("status = %d, want 200 - %s must bypass CSRF", w.Code, tc.name)
			}
		})
	}
}

// TestCSRFProtection_ExemptPaths verifies the web.csrf.exempt_paths allow-list
// from AI.md PART 16, including the rule that a pattern ending in /* also
// covers deeper paths beneath it.
func TestCSRFProtection_ExemptPaths(t *testing.T) {
	cfg := DefaultCSRFConfig()
	cfg.ExemptPaths = []string{"/pay/callback/*"}

	cases := []struct {
		requestPath string
		wantStatus  int
	}{
		{"/pay/callback/provider", http.StatusOK},
		{"/pay/callback/provider/events/replay", http.StatusOK},
		{"/pay/callback", http.StatusForbidden},
		{"/users/settings", http.StatusForbidden},
	}

	for _, tc := range cases {
		t.Run(tc.requestPath, func(t *testing.T) {
			handler := CSRFProtection(cfg)(okHandler)

			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, tc.requestPath, nil)
			ctx := reqctx.SetValue(req.Context(), UserContextKey, "session-authenticated-user")
			req = req.WithContext(ctx)
			handler.ServeHTTP(w, req)

			if w.Code != tc.wantStatus {
				t.Errorf("%s: status = %d, want %d", tc.requestPath, w.Code, tc.wantStatus)
			}
		})
	}
}

// TestCSRFProtection_CookieAttributes verifies AI.md PART 16's requirement that
// SameSite is set explicitly to Strict rather than left to the browser default.
func TestCSRFProtection_CookieAttributes(t *testing.T) {
	cfg := DefaultCSRFConfig()

	handler := CSRFProtection(cfg)(okHandler)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/page", nil)
	handler.ServeHTTP(w, req)

	var issued *http.Cookie
	for _, cookie := range w.Result().Cookies() {
		if cookie.Name == cfg.CookieName {
			issued = cookie
			break
		}
	}
	if issued == nil {
		t.Fatalf("no %s cookie issued on a safe-method request", cfg.CookieName)
	}
	if issued.SameSite != http.SameSiteStrictMode {
		t.Errorf("SameSite = %v, want Strict", issued.SameSite)
	}
	if issued.Value == "" {
		t.Error("issued CSRF cookie has an empty value")
	}
	if issued.HttpOnly {
		t.Error("the CSRF cookie must be HttpOnly=false: the page script reads it (AI.md PART 16)")
	}
	if issued.MaxAge != 0 {
		t.Errorf("MaxAge = %d, want 0 - a fixed expiry silently invalidates tokens "+
			"already rendered into open forms", issued.MaxAge)
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
	ctx := reqctx.SetValue(req.Context(), "admin_id", 1)
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
	ctx := reqctx.SetValue(req.Context(), "admin_id", 1)
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
	ctx := reqctx.SetValue(req.Context(), "admin_id", 1)
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
	ctx := reqctx.SetValue(req.Context(), UserContextKey, "some-authenticated-user")
	ctx = reqctx.SetValue(ctx, UserIDContextKey, 7)
	req = req.WithContext(ctx)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Errorf("status = %d, want 403 - a session-authenticated regular user with no CSRF "+
			"token must be rejected; if this fails, AuthMiddleware has stopped setting "+
			"UserIDContextKey and regular users bypass CSRF on every mutating request", w.Code)
	}
}

// csrfStoreFixture installs a secret store backed by a scratch database for one test.
func csrfStoreFixture(t *testing.T) *secret.Store {
	t.Helper()
	db, err := sql.Open("sqlite", "file:"+filepath.Join(t.TempDir(), "server.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if _, err := db.Exec(`CREATE TABLE app_secrets (name TEXT PRIMARY KEY, value TEXT NOT NULL,
		created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP, rotated_at DATETIME)`); err != nil {
		t.Fatal(err)
	}
	store := secret.NewStore(db)
	if err := store.EnsureAll(); err != nil {
		t.Fatal(err)
	}
	secret.SetDefault(store)
	t.Cleanup(func() { secret.SetDefault(nil) })
	return store
}

func TestCSRFTokenIsSignedAndVerified(t *testing.T) {
	csrfStoreFixture(t)

	token, err := newCSRFToken(32)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(token, csrfSignatureSeparator) || !isCSRFTokenAuthentic(token) {
		t.Fatalf("a freshly issued token must be signed and verify, got %q", token)
	}

	random, _, _ := strings.Cut(token, csrfSignatureSeparator)
	for _, forged := range []string{random, random + ".", random + ".AAAA", "x." + strings.SplitN(token, ".", 2)[1], ""} {
		if isCSRFTokenAuthentic(forged) {
			t.Errorf("a forged token must not verify: %q", forged)
		}
	}
}

func TestCSRFTokenStopsVerifyingAfterTheSecretRotates(t *testing.T) {
	store := csrfStoreFixture(t)
	token, _ := newCSRFToken(32)
	if err := store.Rotate(secret.CSRFTokenSecret, secret.RotateOptions{}); err != nil {
		t.Fatal(err)
	}
	if isCSRFTokenAuthentic(token) {
		t.Error("a token signed before the rotation must stop verifying")
	}
}

func TestCSRFProtection_RejectsAPlantedUnsignedCookie(t *testing.T) {
	csrfStoreFixture(t)
	cfg := DefaultCSRFConfig()
	handler := CSRFProtection(cfg)(okHandler)

	req := httptest.NewRequest(http.MethodPost, "/server/administration/settings", strings.NewReader("csrf_token=planted"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: cfg.CookieName, Value: "planted"})
	req = req.WithContext(reqctx.SetValue(req.Context(), "admin_id", 1))
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Errorf("a cookie and form value the attacker chose must be rejected, got %d", w.Code)
	}
}

func TestCSRFProtection_AcceptsASignedToken(t *testing.T) {
	csrfStoreFixture(t)
	cfg := DefaultCSRFConfig()
	handler := CSRFProtection(cfg)(okHandler)
	token, _ := newCSRFToken(32)

	req := httptest.NewRequest(http.MethodPost, "/server/administration/settings", strings.NewReader("csrf_token="+token))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: cfg.CookieName, Value: token})
	req = req.WithContext(reqctx.SetValue(req.Context(), "admin_id", 1))
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("a matching signed token must be accepted, got %d", w.Code)
	}
}

func TestCSRFProtection_NewTokenWhenTheSessionCookieChanges(t *testing.T) {
	cfg := DefaultCSRFConfig()
	login := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.SetCookie(w, &http.Cookie{Name: UserSessionCookieName(), Value: "new-session", Path: "/"})
		_, _ = w.Write([]byte("ok"))
	})
	handler := CSRFProtection(cfg)(login)

	req := httptest.NewRequest(http.MethodGet, "/server/auth/login", nil)
	req.AddCookie(&http.Cookie{Name: cfg.CookieName, Value: "old-token"})
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	var tokens []string
	for _, cookie := range w.Result().Cookies() {
		if cookie.Name == cfg.CookieName {
			tokens = append(tokens, cookie.Value)
		}
	}
	if len(tokens) == 0 || tokens[len(tokens)-1] == "old-token" {
		t.Errorf("a login response must carry a new CSRF token, got %v", tokens)
	}
}

func TestCSRFProtection_KeepsTheTokenWhenNoSessionCookieChanges(t *testing.T) {
	cfg := DefaultCSRFConfig()
	handler := CSRFProtection(cfg)(okHandler)

	req := httptest.NewRequest(http.MethodGet, "/page", nil)
	req.AddCookie(&http.Cookie{Name: cfg.CookieName, Value: "keep-me"})
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	for _, cookie := range w.Result().Cookies() {
		if cookie.Name == cfg.CookieName && cookie.Value != "keep-me" {
			t.Errorf("an ordinary response must not replace the token, got %q", cookie.Value)
		}
	}
}
