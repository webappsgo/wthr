package handler

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	models "github.com/webappsgo/wthr/src/server/model"
)

// newAdminAuthTestDB wires the global dual DB (admin_auth.go handlers call
// database.GetServerDB() directly) and returns the raw *sql.DB for seeding.
func newAdminAuthTestDB(t *testing.T) *sql.DB {
	t.Helper()
	serverDB := newTestServerDB(t)
	usersDB := newTestUsersDB(t)
	setGlobalTestDualDB(t, serverDB, usersDB)
	return serverDB
}

// seedTestAdmin creates a real admin row directly via SQL (Argon2id hashing
// still goes through models.HashPassword, matching production logic for the
// credentials row).
//
// It deliberately does NOT call AdminModel.Create: see
// TestAdminModel_Create_PreferencesSchemaMismatch below for why (Bug #7)
// Create is currently broken against the real server_admin_preferences
// schema and would fail every seed call here with an unrelated SQL error,
// masking the actual behavior under test in this file.
func seedTestAdmin(t *testing.T, db *sql.DB, username, email, password string) *models.Admin {
	t.Helper()
	passwordHash, err := models.HashPassword(password)
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	apiToken, err := models.GenerateAPIToken()
	if err != nil {
		t.Fatalf("generate api token: %v", err)
	}
	tokenHash := models.HashAPIToken(apiToken)
	tokenPrefix := models.GetAPITokenPrefix(apiToken)

	result, err := db.Exec(`
		INSERT INTO server_admin_credentials (username, email, password_hash, api_token_hash, api_token_prefix, is_super_admin, is_active, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, 0, 1, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
	`, username, email, passwordHash, tokenHash, tokenPrefix)
	if err != nil {
		t.Fatalf("seed admin: %v", err)
	}
	id, err := result.LastInsertId()
	if err != nil {
		t.Fatalf("seed admin id: %v", err)
	}
	admin, err := (&models.AdminModel{DB: db}).GetByID(id)
	if err != nil {
		t.Fatalf("seed admin reload: %v", err)
	}
	return admin
}

// TestAdminModel_Create_PreferencesSchemaMismatch is a regression test
// documenting a genuine production bug: src/server/model/admin.go's
// AdminModel.Create (around line 436) inserts into
// server_admin_preferences using columns
// (admin_id, theme, language, timezone, notifications_enabled, email_notifications, created_at, updated_at),
// but the real table defined in src/database/server_schema.go (around line
// 241) only has columns (admin_id, preferences TEXT, updated_at). Every call
// to AdminModel.Create against the real schema fails with:
//
//	failed to create admin preferences: SQL logic error: table server_admin_preferences has no column named theme (1)
//
// This means admin creation is completely broken in production, not just in
// this test suite. This test encodes the CORRECT expected behavior (Create
// succeeds and returns a usable Admin) and is expected to fail until the
// production bug is fixed.
func TestAdminModel_Create_PreferencesSchemaMismatch(t *testing.T) {
	db := newAdminAuthTestDB(t)
	admin, err := (&models.AdminModel{DB: db}).Create("bug7admin", "bug7admin@example.com", "correct-horse-battery", false)
	if err != nil {
		t.Errorf("BUG src/server/model/admin.go AdminModel.Create: got error %v, want nil (server_admin_preferences INSERT uses columns theme/language/timezone/notifications_enabled/email_notifications that do not exist in the real schema, which only has admin_id/preferences/updated_at)", err)
		return
	}
	if admin == nil || admin.Username != "bug7admin" {
		t.Errorf("Create returned unexpected admin: %+v", admin)
	}
}

func decodeAdminLoginResponse(t *testing.T, w *httptest.ResponseRecorder) AdminLoginResponse {
	t.Helper()
	var resp AdminLoginResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v; body=%s", err, w.Body.String())
	}
	return resp
}

// TestAdminLoginHandler covers success, wrong HTTP method, malformed JSON,
// missing fields, and wrong password.
func TestAdminLoginHandler(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		db := newAdminAuthTestDB(t)
		seedTestAdmin(t, db, "loginadmin", "loginadmin@example.com", "correct-horse-battery")

		body, _ := json.Marshal(AdminLoginRequest{Username: "loginadmin", Password: "correct-horse-battery"})
		r := httptest.NewRequest(http.MethodPost, "/server/auth/login", bytes.NewReader(body))
		w := httptest.NewRecorder()
		AdminLoginHandler(w, r)

		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
		}
		resp := decodeAdminLoginResponse(t, w)
		if !resp.Ok {
			t.Errorf("Ok = false, want true")
		}
		if w.Result().Cookies() == nil {
			t.Errorf("expected admin_session cookie to be set")
		}
	})

	t.Run("wrong method", func(t *testing.T) {
		r := httptest.NewRequest(http.MethodGet, "/server/auth/login", nil)
		w := httptest.NewRecorder()
		AdminLoginHandler(w, r)

		if w.Code != http.StatusMethodNotAllowed {
			t.Fatalf("status = %d, want 405", w.Code)
		}
	})

	t.Run("malformed JSON", func(t *testing.T) {
		newAdminAuthTestDB(t)
		r := httptest.NewRequest(http.MethodPost, "/server/auth/login", bytes.NewReader([]byte("not json")))
		w := httptest.NewRecorder()
		AdminLoginHandler(w, r)

		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400", w.Code)
		}
	})

	t.Run("missing fields", func(t *testing.T) {
		newAdminAuthTestDB(t)
		body, _ := json.Marshal(AdminLoginRequest{Username: "", Password: ""})
		r := httptest.NewRequest(http.MethodPost, "/server/auth/login", bytes.NewReader(body))
		w := httptest.NewRecorder()
		AdminLoginHandler(w, r)

		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400", w.Code)
		}
	})

	t.Run("wrong password", func(t *testing.T) {
		db := newAdminAuthTestDB(t)
		seedTestAdmin(t, db, "wrongpwadmin", "wrongpwadmin@example.com", "correct-horse-battery")

		body, _ := json.Marshal(AdminLoginRequest{Username: "wrongpwadmin", Password: "totally-wrong"})
		r := httptest.NewRequest(http.MethodPost, "/server/auth/login", bytes.NewReader(body))
		w := httptest.NewRecorder()
		AdminLoginHandler(w, r)

		if w.Code != http.StatusUnauthorized {
			t.Fatalf("status = %d, want 401; body=%s", w.Code, w.Body.String())
		}
	})
}

// TestAdminLogoutHandler covers logout with and without a session cookie.
func TestAdminLogoutHandler(t *testing.T) {
	t.Run("no cookie", func(t *testing.T) {
		newAdminAuthTestDB(t)
		r := httptest.NewRequest(http.MethodPost, "/server/auth/logout", nil)
		w := httptest.NewRecorder()
		AdminLogoutHandler(w, r)

		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200", w.Code)
		}
	})

	t.Run("with cookie", func(t *testing.T) {
		db := newAdminAuthTestDB(t)
		admin := seedTestAdmin(t, db, "logoutadmin", "logoutadmin@example.com", "correct-horse-battery")
		session, err := (&models.AdminSessionModel{DB: db}).CreateSession(admin.ID, "127.0.0.1", "test-agent", AdminSessionDuration)
		if err != nil {
			t.Fatalf("create session: %v", err)
		}

		r := httptest.NewRequest(http.MethodPost, "/server/auth/logout", nil)
		r.AddCookie(&http.Cookie{Name: AdminSessionCookieName, Value: session.SessionID})
		w := httptest.NewRecorder()
		AdminLogoutHandler(w, r)

		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200", w.Code)
		}
		if _, err := (&models.AdminSessionModel{DB: db}).GetSession(session.SessionID); err == nil {
			t.Errorf("session still retrievable after logout")
		}
	})
}

// withAdminContext injects an *models.Admin under adminContextKey, matching
// admin_auth.go's r.Context().Value(adminContextKey) lookups.
func withAdminContext(r *http.Request, admin *models.Admin) *http.Request {
	return r.WithContext(context.WithValue(r.Context(), adminContextKey, admin))
}

// TestAdminMeHandler covers the authorized and unauthorized paths.
func TestAdminMeHandler(t *testing.T) {
	t.Run("authorized", func(t *testing.T) {
		db := newAdminAuthTestDB(t)
		admin := seedTestAdmin(t, db, "meadmin", "meadmin@example.com", "correct-horse-battery")

		r := withAdminContext(httptest.NewRequest(http.MethodGet, "/server/auth/me", nil), admin)
		w := httptest.NewRecorder()
		AdminMeHandler(w, r)

		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
		}
	})

	t.Run("unauthorized", func(t *testing.T) {
		r := httptest.NewRequest(http.MethodGet, "/server/auth/me", nil)
		w := httptest.NewRecorder()
		AdminMeHandler(w, r)

		if w.Code != http.StatusUnauthorized {
			t.Fatalf("status = %d, want 401", w.Code)
		}
	})
}

// TestAdminSessionsHandler covers listing sessions for the current admin
// and the unauthorized path.
//
// The "authorized" subtest documents a second genuine production bug
// (Bug #8): src/server/model/admin.go's AdminSessionModel.GetSession,
// UpdateSessionLastUsed, DeleteSession, and GetActiveSessions (lines
// ~825-910) all SELECT/WHERE on a "session_id" column and a
// "last_used_at" column, but the real server_admin_sessions table in
// src/database/server_schema.go (~line 32) only has columns
// (id, admin_id, data, expires_at, created_at, ip_address, user_agent) —
// the session identifier column is named "id", not "session_id", and
// there is no "last_used_at" column at all. CreateSession (which inserts
// into "id") works, but every subsequent read/update/delete of that
// session fails with "SQL logic error: no such column: session_id".
// Concretely: AdminSessionsHandler (admin_auth.go) calls
// AdminSessionModel.GetActiveSessions and gets a 500 every time a real
// admin lists their sessions. This also means TestAdminLogoutHandler's
// "with_cookie" subtest above passes for the wrong reason: DeleteSession
// silently fails on the bad WHERE clause, and GetSession's post-logout
// call fails with this same schema error rather than a genuine
// not-found, so the assertion "session still retrievable after logout"
// coincidentally holds despite the session row never being deleted.
func TestAdminSessionsHandler(t *testing.T) {
	t.Run("authorized", func(t *testing.T) {
		db := newAdminAuthTestDB(t)
		admin := seedTestAdmin(t, db, "sessionsadmin", "sessionsadmin@example.com", "correct-horse-battery")
		if _, err := (&models.AdminSessionModel{DB: db}).CreateSession(admin.ID, "127.0.0.1", "test-agent", AdminSessionDuration); err != nil {
			t.Fatalf("create session: %v", err)
		}

		r := withAdminContext(httptest.NewRequest(http.MethodGet, "/server/auth/sessions", nil), admin)
		w := httptest.NewRecorder()
		AdminSessionsHandler(w, r)

		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
		}
	})

	t.Run("unauthorized", func(t *testing.T) {
		r := httptest.NewRequest(http.MethodGet, "/server/auth/sessions", nil)
		w := httptest.NewRecorder()
		AdminSessionsHandler(w, r)

		if w.Code != http.StatusUnauthorized {
			t.Fatalf("status = %d, want 401", w.Code)
		}
	})
}

// TestAdminChangePasswordHandler covers success plus every validation and
// auth-failure edge case.
func TestAdminChangePasswordHandler(t *testing.T) {
	newRequest := func(admin *models.Admin, req AdminChangePasswordRequest) *http.Request {
		body, _ := json.Marshal(req)
		r := httptest.NewRequest(http.MethodPost, "/server/auth/change-password", bytes.NewReader(body))
		if admin != nil {
			r = withAdminContext(r, admin)
		}
		return r
	}

	t.Run("success", func(t *testing.T) {
		db := newAdminAuthTestDB(t)
		admin := seedTestAdmin(t, db, "cpwadmin", "cpwadmin@example.com", "old-password-1")

		w := httptest.NewRecorder()
		AdminChangePasswordHandler(w, newRequest(admin, AdminChangePasswordRequest{
			CurrentPassword: "old-password-1",
			NewPassword:     "new-password-2",
			ConfirmPassword: "new-password-2",
		}))

		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
		}
	})

	t.Run("unauthorized", func(t *testing.T) {
		newAdminAuthTestDB(t)
		w := httptest.NewRecorder()
		AdminChangePasswordHandler(w, newRequest(nil, AdminChangePasswordRequest{
			CurrentPassword: "a", NewPassword: "new-password-2", ConfirmPassword: "new-password-2",
		}))

		if w.Code != http.StatusUnauthorized {
			t.Fatalf("status = %d, want 401", w.Code)
		}
	})

	t.Run("mismatched confirm password", func(t *testing.T) {
		db := newAdminAuthTestDB(t)
		admin := seedTestAdmin(t, db, "cpwadmin2", "cpwadmin2@example.com", "old-password-1")

		w := httptest.NewRecorder()
		AdminChangePasswordHandler(w, newRequest(admin, AdminChangePasswordRequest{
			CurrentPassword: "old-password-1",
			NewPassword:     "new-password-2",
			ConfirmPassword: "does-not-match",
		}))

		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400; body=%s", w.Code, w.Body.String())
		}
	})

	t.Run("new password too short", func(t *testing.T) {
		db := newAdminAuthTestDB(t)
		admin := seedTestAdmin(t, db, "cpwadmin3", "cpwadmin3@example.com", "old-password-1")

		w := httptest.NewRecorder()
		AdminChangePasswordHandler(w, newRequest(admin, AdminChangePasswordRequest{
			CurrentPassword: "old-password-1",
			NewPassword:     "short",
			ConfirmPassword: "short",
		}))

		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400; body=%s", w.Code, w.Body.String())
		}
	})

	t.Run("wrong current password", func(t *testing.T) {
		db := newAdminAuthTestDB(t)
		admin := seedTestAdmin(t, db, "cpwadmin4", "cpwadmin4@example.com", "old-password-1")

		w := httptest.NewRecorder()
		AdminChangePasswordHandler(w, newRequest(admin, AdminChangePasswordRequest{
			CurrentPassword: "totally-wrong",
			NewPassword:     "new-password-2",
			ConfirmPassword: "new-password-2",
		}))

		if w.Code != http.StatusUnauthorized {
			t.Fatalf("status = %d, want 401; body=%s", w.Code, w.Body.String())
		}
	})
}

// TestAdminRegenerateAPITokenHandler covers success and the unauthorized
// path.
func TestAdminRegenerateAPITokenHandler(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		db := newAdminAuthTestDB(t)
		admin := seedTestAdmin(t, db, "tokenadmin", "tokenadmin@example.com", "correct-horse-battery")

		r := withAdminContext(httptest.NewRequest(http.MethodPost, "/server/auth/regenerate-token", nil), admin)
		w := httptest.NewRecorder()
		AdminRegenerateAPITokenHandler(w, r)

		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
		}
	})

	t.Run("unauthorized", func(t *testing.T) {
		r := httptest.NewRequest(http.MethodPost, "/server/auth/regenerate-token", nil)
		w := httptest.NewRecorder()
		AdminRegenerateAPITokenHandler(w, r)

		if w.Code != http.StatusUnauthorized {
			t.Fatalf("status = %d, want 401", w.Code)
		}
	})
}
