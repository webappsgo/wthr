package handler

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/gin-gonic/gin"

	paths "github.com/webappsgo/wthr/src/path"
	utils "github.com/webappsgo/wthr/src/util"
)

// newSetupTestHandler builds a SetupHandler against a fresh in-memory
// server.db/users.db pair wired into the package-level global DB, since
// SetupHandler's methods read via database.GetServerDB() rather than an
// injected field.
func newSetupTestHandler(t *testing.T) (*SetupHandler, *sql.DB) {
	t.Helper()
	serverDB := newTestServerDB(t)
	usersDB := newTestUsersDB(t)
	setGlobalTestDualDB(t, serverDB, usersDB)
	return &SetupHandler{DB: serverDB}, serverDB
}

// writeRealSetupToken writes a real setup_token.txt at the process-wide
// resolved paths.GetConfigDir() location (a sync.Once singleton with no
// per-test override hook), and removes it again on cleanup so tests never
// leak state into each other or the real filesystem.
func writeRealSetupToken(t *testing.T, token string) {
	t.Helper()
	configDir := paths.GetConfigDir()
	if err := utils.SaveSetupToken(configDir, token); err != nil {
		t.Fatalf("save real setup token: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Remove(filepath.Join(configDir, "setup_token.txt"))
	})
}

// TestVerifySetupToken covers the API setup-token-verification endpoint:
// a valid token sets the setup_token_verified cookie, a missing token is a
// 400, and an unrecognized token is a 401.
func TestVerifySetupToken(t *testing.T) {
	t.Run("valid token sets the verified cookie", func(t *testing.T) {
		h, _ := newSetupTestHandler(t)
		writeRealSetupToken(t, "correct-setup-token")

		c, w := newTestContextJSON(t, http.MethodPost, "/api/v1/setup/verify-token", map[string]string{
			"setup_token": "correct-setup-token",
		})
		h.VerifySetupToken(c)

		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
		}
		found := false
		for _, ck := range w.Result().Cookies() {
			if ck.Name == "setup_token_verified" && ck.Value == "true" {
				found = true
			}
		}
		if !found {
			t.Fatalf("expected setup_token_verified=true cookie, got %v", w.Result().Cookies())
		}
	})

	t.Run("missing token returns 400", func(t *testing.T) {
		h, _ := newSetupTestHandler(t)
		c, w := newTestContextJSON(t, http.MethodPost, "/api/v1/setup/verify-token", map[string]string{})
		h.VerifySetupToken(c)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400; body=%s", w.Code, w.Body.String())
		}
	})

	t.Run("wrong token returns 401", func(t *testing.T) {
		h, _ := newSetupTestHandler(t)
		writeRealSetupToken(t, "correct-setup-token")

		c, w := newTestContextJSON(t, http.MethodPost, "/api/v1/setup/verify-token", map[string]string{
			"setup_token": "totally-wrong",
		})
		h.VerifySetupToken(c)
		if w.Code != http.StatusUnauthorized {
			t.Fatalf("status = %d, want 401; body=%s", w.Code, w.Body.String())
		}
	})
}

// newVerifiedSetupContext builds a JSON gin context for CreateAdmin that
// already carries a valid setup_token_verified cookie, mirroring what a
// real browser would send after VerifySetupToken succeeded.
func newVerifiedSetupContext(t *testing.T, body map[string]interface{}) (*gin.Context, *httptest.ResponseRecorder) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	raw, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal body: %v", err)
	}
	c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/setup/admin", bytes.NewReader(raw))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Request.Header.Set("Accept", "application/json")
	c.Request.AddCookie(&http.Cookie{Name: "setup_token_verified", Value: "true"})
	return c, w
}

// TestCreateAdmin covers the Primary Admin creation success path (session +
// API token cookies issued, row present in server_admin_credentials), the
// unverified-setup-token 401 guard, and the too-short-password validation
// error.
func TestCreateAdmin(t *testing.T) {
	t.Run("success creates the primary admin and issues cookies", func(t *testing.T) {
		h, serverDB := newSetupTestHandler(t)

		c, w := newVerifiedSetupContext(t, map[string]interface{}{
			"username":         "administrator",
			"email":            "admin@example.com",
			"password":         "correct-horse-battery-staple",
			"confirm_password": "correct-horse-battery-staple",
		})
		h.CreateAdmin(c)

		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
		}
		var count int
		if err := serverDB.QueryRow(`SELECT COUNT(*) FROM server_admin_credentials WHERE username = ?`, "administrator").Scan(&count); err != nil {
			t.Fatalf("query admin count: %v", err)
		}
		if count != 1 {
			t.Fatalf("expected 1 admin row, got %d", count)
		}

		hasAPIToken := false
		for _, ck := range w.Result().Cookies() {
			if ck.Name == "setup_api_token" && ck.Value != "" {
				hasAPIToken = true
			}
		}
		if !hasAPIToken {
			t.Fatalf("expected setup_api_token cookie to be set, got %v", w.Result().Cookies())
		}
	})

	t.Run("unverified setup token is rejected with 401", func(t *testing.T) {
		h, _ := newSetupTestHandler(t)
		c, w := newTestContextJSON(t, http.MethodPost, "/api/v1/setup/admin", map[string]interface{}{
			"username":         "administrator",
			"email":            "admin@example.com",
			"password":         "correct-horse-battery-staple",
			"confirm_password": "correct-horse-battery-staple",
		})
		c.Request.Header.Set("Accept", "application/json")
		h.CreateAdmin(c)
		if w.Code != http.StatusUnauthorized {
			t.Fatalf("status = %d, want 401; body=%s", w.Code, w.Body.String())
		}
	})

	t.Run("too-short password is rejected with 400 before touching the DB", func(t *testing.T) {
		h, serverDB := newSetupTestHandler(t)
		c, w := newVerifiedSetupContext(t, map[string]interface{}{
			"username":         "administrator",
			"email":            "admin@example.com",
			"password":         "short",
			"confirm_password": "short",
		})
		h.CreateAdmin(c)

		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400; body=%s", w.Code, w.Body.String())
		}
		var count int
		if err := serverDB.QueryRow(`SELECT COUNT(*) FROM server_admin_credentials`).Scan(&count); err != nil {
			t.Fatalf("query admin count: %v", err)
		}
		if count != 0 {
			t.Fatalf("expected no admin row to be created, got %d", count)
		}
	})
}

// TestGetSetupStatus covers the three status stages: no admin yet (not
// started), admin created but wizard incomplete, and fully completed.
func TestGetSetupStatus(t *testing.T) {
	t.Run("no admin yet reports not_started", func(t *testing.T) {
		h, _ := newSetupTestHandler(t)
		c, w := newTestContext(http.MethodGet, "/api/v1/setup/status")
		h.GetSetupStatus(c)

		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
		}
		var body struct {
			Status     string `json:"status"`
			IsComplete bool   `json:"is_complete"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		if body.Status != "not_started" || body.IsComplete {
			t.Fatalf("got status=%q is_complete=%v, want not_started/false", body.Status, body.IsComplete)
		}
	})

	t.Run("setup marked completed reports completed", func(t *testing.T) {
		h, serverDB := newSetupTestHandler(t)
		if _, err := serverDB.Exec(`
			INSERT INTO server_admin_credentials (username, email, password_hash, is_super_admin, is_active, created_at, updated_at)
			VALUES ('administrator', 'admin@example.com', 'hash', 1, 1, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
		`); err != nil {
			t.Fatalf("seed admin: %v", err)
		}
		if _, err := serverDB.Exec(`
			INSERT INTO server_config (key, value, updated_at) VALUES ('setup.completed', 'true', datetime('now'))
		`); err != nil {
			t.Fatalf("seed setup.completed: %v", err)
		}

		c, w := newTestContext(http.MethodGet, "/api/v1/setup/status")
		h.GetSetupStatus(c)

		var body struct {
			Status     string `json:"status"`
			IsComplete bool   `json:"is_complete"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		if body.Status != "completed" || !body.IsComplete {
			t.Fatalf("got status=%q is_complete=%v, want completed/true", body.Status, body.IsComplete)
		}
	})

	t.Run("DB failure returns 500", func(t *testing.T) {
		h, serverDB := newSetupTestHandler(t)
		if _, err := serverDB.Exec(`DROP TABLE server_admin_credentials`); err != nil {
			t.Fatalf("drop table: %v", err)
		}

		c, w := newTestContext(http.MethodGet, "/api/v1/setup/status")
		h.GetSetupStatus(c)

		if w.Code != http.StatusInternalServerError {
			t.Fatalf("status = %d, want 500; body=%s", w.Code, w.Body.String())
		}
	})
}

// --- setup_wizard.go (net/http, plain ResponseWriter) ---

// newSetupWizardTestDBs wires a fresh in-memory server/users DB pair into
// the global dual DB, matching the gin-based tests' convention, since
// SetupStatusHandler/SetupWizardHandler also read via database.GetServerDB().
func newSetupWizardTestDBs(t *testing.T) *sql.DB {
	t.Helper()
	serverDB := newTestServerDB(t)
	usersDB := newTestUsersDB(t)
	setGlobalTestDualDB(t, serverDB, usersDB)
	return serverDB
}

// TestSetupStatusHandler_Wizard covers the plain net/http status endpoint's
// method-not-allowed guard and its "not done" JSON payload.
func TestSetupStatusHandler_Wizard(t *testing.T) {
	t.Run("GET with no admins reports setup_done=false", func(t *testing.T) {
		newSetupWizardTestDBs(t)
		r := httptest.NewRequest(http.MethodGet, "/api/setup/status", nil)
		w := httptest.NewRecorder()
		SetupStatusHandler(w, r)

		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
		}
		var body SetupWizardResponse
		if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		if !body.Ok || body.SetupDone {
			t.Fatalf("got ok=%v setup_done=%v, want ok=true setup_done=false", body.Ok, body.SetupDone)
		}
	})

	t.Run("POST is rejected with 405", func(t *testing.T) {
		newSetupWizardTestDBs(t)
		r := httptest.NewRequest(http.MethodPost, "/api/setup/status", nil)
		w := httptest.NewRecorder()
		SetupStatusHandler(w, r)
		if w.Code != http.StatusMethodNotAllowed {
			t.Fatalf("status = %d, want 405", w.Code)
		}
	})
}

// TestSetupWizardHandler covers the full first-run admin creation success
// path (with a real setup token file), the invalid-setup-token 401, the
// validation-error 400 (weak password), and the already-completed 403.
func TestSetupWizardHandler(t *testing.T) {
	t.Run("valid request creates the first admin", func(t *testing.T) {
		serverDB := newSetupWizardTestDBs(t)
		writeRealSetupToken(t, "wizard-setup-token")

		reqBody, _ := json.Marshal(SetupWizardRequest{
			SetupToken:      "wizard-setup-token",
			Username:        "administrator",
			Email:           "admin@example.com",
			Password:        "CorrectHorse123",
			ConfirmPassword: "CorrectHorse123",
		})
		r := httptest.NewRequest(http.MethodPost, "/api/setup", bytes.NewReader(reqBody))
		w := httptest.NewRecorder()
		SetupWizardHandler(w, r)

		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
		}
		var count int
		if err := serverDB.QueryRow(`SELECT COUNT(*) FROM server_admin_credentials WHERE username = ?`, "administrator").Scan(&count); err != nil {
			t.Fatalf("query admin count: %v", err)
		}
		if count != 1 {
			t.Fatalf("expected 1 admin row, got %d", count)
		}
	})

	t.Run("invalid setup token is rejected with 401", func(t *testing.T) {
		newSetupWizardTestDBs(t)
		writeRealSetupToken(t, "wizard-setup-token")

		reqBody, _ := json.Marshal(SetupWizardRequest{
			SetupToken:      "wrong-token",
			Username:        "administrator",
			Email:           "admin@example.com",
			Password:        "CorrectHorse123",
			ConfirmPassword: "CorrectHorse123",
		})
		r := httptest.NewRequest(http.MethodPost, "/api/setup", bytes.NewReader(reqBody))
		w := httptest.NewRecorder()
		SetupWizardHandler(w, r)

		if w.Code != http.StatusUnauthorized {
			t.Fatalf("status = %d, want 401; body=%s", w.Code, w.Body.String())
		}
	})

	t.Run("weak password fails validation with 400", func(t *testing.T) {
		newSetupWizardTestDBs(t)
		writeRealSetupToken(t, "wizard-setup-token")

		reqBody, _ := json.Marshal(SetupWizardRequest{
			SetupToken:      "wizard-setup-token",
			Username:        "administrator",
			Email:           "admin@example.com",
			Password:        "weak",
			ConfirmPassword: "weak",
		})
		r := httptest.NewRequest(http.MethodPost, "/api/setup", bytes.NewReader(reqBody))
		w := httptest.NewRecorder()
		SetupWizardHandler(w, r)

		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400; body=%s", w.Code, w.Body.String())
		}
	})

	t.Run("setup already completed is rejected with 403", func(t *testing.T) {
		serverDB := newSetupWizardTestDBs(t)
		if _, err := serverDB.Exec(`
			INSERT INTO server_admin_credentials (username, email, password_hash, is_super_admin, is_active, created_at, updated_at)
			VALUES ('existing', 'existing@example.com', 'hash', 1, 1, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
		`); err != nil {
			t.Fatalf("seed existing admin: %v", err)
		}

		reqBody, _ := json.Marshal(SetupWizardRequest{
			SetupToken:      "irrelevant",
			Username:        "administrator",
			Email:           "admin@example.com",
			Password:        "CorrectHorse123",
			ConfirmPassword: "CorrectHorse123",
		})
		r := httptest.NewRequest(http.MethodPost, "/api/setup", bytes.NewReader(reqBody))
		w := httptest.NewRecorder()
		SetupWizardHandler(w, r)

		if w.Code != http.StatusForbidden {
			t.Fatalf("status = %d, want 403; body=%s", w.Code, w.Body.String())
		}
	})
}

// TestValidateSetupRequest is a table-driven test of the pure validation
// function backing SetupWizardHandler, covering every rejection branch plus
// the happy path.
func TestValidateSetupRequest(t *testing.T) {
	base := SetupWizardRequest{
		Username:        "administrator",
		Email:           "admin@example.com",
		Password:        "CorrectHorse123",
		ConfirmPassword: "CorrectHorse123",
	}

	tests := []struct {
		name    string
		mutate  func(r SetupWizardRequest) SetupWizardRequest
		wantErr bool
	}{
		{"valid request", func(r SetupWizardRequest) SetupWizardRequest { return r }, false},
		{"empty username", func(r SetupWizardRequest) SetupWizardRequest { r.Username = ""; return r }, true},
		{"username too short", func(r SetupWizardRequest) SetupWizardRequest { r.Username = "ab"; return r }, true},
		{"invalid username characters", func(r SetupWizardRequest) SetupWizardRequest { r.Username = "admin!"; return r }, true},
		{"empty email", func(r SetupWizardRequest) SetupWizardRequest { r.Email = ""; return r }, true},
		{"invalid email format", func(r SetupWizardRequest) SetupWizardRequest { r.Email = "not-an-email"; return r }, true},
		{"empty password", func(r SetupWizardRequest) SetupWizardRequest { r.Password = ""; r.ConfirmPassword = ""; return r }, true},
		{"password with leading whitespace", func(r SetupWizardRequest) SetupWizardRequest {
			r.Password = " CorrectHorse123"
			r.ConfirmPassword = " CorrectHorse123"
			return r
		}, true},
		{"password too short", func(r SetupWizardRequest) SetupWizardRequest { r.Password = "Ab1"; r.ConfirmPassword = "Ab1"; return r }, true},
		{"password missing complexity", func(r SetupWizardRequest) SetupWizardRequest {
			r.Password = "alllowercase1"
			r.ConfirmPassword = "alllowercase1"
			return r
		}, true},
		{"password confirmation mismatch", func(r SetupWizardRequest) SetupWizardRequest { r.ConfirmPassword = "Different123"; return r }, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := tt.mutate(base)
			err := validateSetupRequest(&req)
			if tt.wantErr && err == nil {
				t.Fatalf("expected an error, got nil")
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("expected no error, got %v", err)
			}
		})
	}
}

// TestSetupRequiredMiddleware covers the three branches: setup-endpoint
// bypass, API redirect (JSON 503), and web redirect (303), plus the
// pass-through once setup is complete.
func TestSetupRequiredMiddleware(t *testing.T) {
	called := false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	})

	t.Run("setup endpoint bypasses the check even with no admin", func(t *testing.T) {
		newSetupWizardTestDBs(t)
		called = false
		r := httptest.NewRequest(http.MethodGet, "/setup/status", nil)
		w := httptest.NewRecorder()
		SetupRequiredMiddleware(next).ServeHTTP(w, r)
		if !called {
			t.Fatalf("expected the setup endpoint to bypass the middleware and call next")
		}
	})

	t.Run("API request with no admin gets a 503 JSON response", func(t *testing.T) {
		newSetupWizardTestDBs(t)
		called = false
		r := httptest.NewRequest(http.MethodGet, "/api/v1/weather", nil)
		w := httptest.NewRecorder()
		SetupRequiredMiddleware(next).ServeHTTP(w, r)

		if called {
			t.Fatalf("expected next NOT to be called before setup is complete")
		}
		if w.Code != http.StatusServiceUnavailable {
			t.Fatalf("status = %d, want 503; body=%s", w.Code, w.Body.String())
		}
	})

	t.Run("web request with no admin is redirected to /setup", func(t *testing.T) {
		newSetupWizardTestDBs(t)
		called = false
		r := httptest.NewRequest(http.MethodGet, "/dashboard", nil)
		w := httptest.NewRecorder()
		SetupRequiredMiddleware(next).ServeHTTP(w, r)

		if called {
			t.Fatalf("expected next NOT to be called before setup is complete")
		}
		if w.Code != http.StatusSeeOther {
			t.Fatalf("status = %d, want 303; body=%s", w.Code, w.Body.String())
		}
		if loc := w.Header().Get("Location"); loc != "/setup" {
			t.Fatalf("Location = %q, want /setup", loc)
		}
	})

	t.Run("once an admin exists, requests pass through", func(t *testing.T) {
		serverDB := newSetupWizardTestDBs(t)
		if _, err := serverDB.Exec(`
			INSERT INTO server_admin_credentials (username, email, password_hash, is_super_admin, is_active, created_at, updated_at)
			VALUES ('administrator', 'admin@example.com', 'hash', 1, 1, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
		`); err != nil {
			t.Fatalf("seed admin: %v", err)
		}
		called = false
		r := httptest.NewRequest(http.MethodGet, "/dashboard", nil)
		w := httptest.NewRecorder()
		SetupRequiredMiddleware(next).ServeHTTP(w, r)

		if !called {
			t.Fatalf("expected next to be called once setup is complete")
		}
		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200", w.Code)
		}
	})
}
