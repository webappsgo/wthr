package handler

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/webappsgo/wthr/src/common/dbtime"
	"github.com/webappsgo/wthr/src/config"
	"github.com/webappsgo/wthr/src/database"
	"github.com/webappsgo/wthr/src/server/middleware"
	models "github.com/webappsgo/wthr/src/server/model"
)

// newAuthTestHandler wires an AuthHandler against fresh in-memory
// server.db/users.db instances and primes the database.GetServerDB/
// GetUsersDB globals HandleLogin/HandleRegister read directly (in addition
// to the h.DB field, which only backs the users.db-scoped model calls).
func newAuthTestHandler(t *testing.T) (*AuthHandler, *sql.DB, *sql.DB) {
	t.Helper()
	serverDB := newTestServerDB(t)
	usersDB := newTestUsersDB(t)
	setGlobalTestDualDB(t, serverDB, usersDB)
	return &AuthHandler{DB: usersDB}, serverDB, usersDB
}

// seedAdmin inserts an active, passkey-less super admin directly into
// server_admin_credentials with a real Argon2id hash so VerifyCredentials
// exercises the genuine password-verification code path.
func seedAdmin(t *testing.T, db *sql.DB, username, email, password string) int64 {
	t.Helper()
	hash, err := models.HashPassword(password)
	if err != nil {
		t.Fatalf("hash admin password: %v", err)
	}
	res, err := db.Exec(`
		INSERT INTO server_admin_credentials (username, email, password_hash, is_super_admin, is_active, created_at, updated_at)
		VALUES (?, ?, ?, 1, 1, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
	`, username, email, hash)
	if err != nil {
		t.Fatalf("seed admin: %v", err)
	}
	id, _ := res.LastInsertId()
	return id
}

// seedAuthUser inserts an active user account directly into user_accounts
// with a real Argon2id hash, mirroring UserModel.Create's column set.
func seedAuthUser(t *testing.T, db *sql.DB, username, email, password string) int64 {
	t.Helper()
	hash, err := models.HashPassword(password)
	if err != nil {
		t.Fatalf("hash user password: %v", err)
	}
	res, err := db.Exec(`
		INSERT INTO user_accounts (username, email, password_hash, role, email_verified, is_active, is_banned, two_factor_enabled, created_at, updated_at)
		VALUES (?, ?, ?, 'user', 1, 1, 0, 0, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
	`, username, email, hash)
	if err != nil {
		t.Fatalf("seed user: %v", err)
	}
	id, _ := res.LastInsertId()
	return id
}

func withJSONAccept(c *gin.Context) {
	c.Request.Header.Set("Accept", "application/json")
	c.Request.Header.Set("Content-Type", "application/json")
}

// TestHandleLogin_Admin covers the admin-credentials-checked-first branch of
// HandleLogin, both the success path (session issued, cookie set) and the
// wrong-password error path.
func TestHandleLogin_Admin(t *testing.T) {
	t.Run("valid admin credentials issue admin_session cookie", func(t *testing.T) {
		h, _, _ := newAuthTestHandler(t)
		config.SetGlobalConfig(&config.AppConfig{})
		t.Cleanup(func() { config.SetGlobalConfig(nil) })
		seedAdmin(t, database.GetServerDB(), "root-admin", "root@example.com", "correct-horse-battery")

		c, w := newTestContextJSON(t, http.MethodPost, "/server/auth/login", map[string]string{
			"identifier": "root-admin",
			"password":   "correct-horse-battery",
		})
		withJSONAccept(c)
		h.HandleLogin(c)

		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
		}
		var body map[string]interface{}
		if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
			t.Fatalf("decode response: %v", err)
		}
		if body["type"] != "admin" {
			t.Errorf("type = %v, want admin", body["type"])
		}
		found := false
		for _, ck := range w.Result().Cookies() {
			if ck.Name == "admin_session" && ck.Value != "" {
				found = true
			}
		}
		if !found {
			t.Errorf("expected admin_session cookie to be set; got cookies=%v", w.Result().Cookies())
		}
	})

	t.Run("wrong admin password falls through and is rejected as invalid credentials", func(t *testing.T) {
		h, _, _ := newAuthTestHandler(t)
		config.SetGlobalConfig(&config.AppConfig{})
		t.Cleanup(func() { config.SetGlobalConfig(nil) })
		seedAdmin(t, database.GetServerDB(), "root-admin", "root@example.com", "correct-horse-battery")

		c, w := newTestContextJSON(t, http.MethodPost, "/server/auth/login", map[string]string{
			"identifier": "root-admin",
			"password":   "totally-wrong",
		})
		withJSONAccept(c)
		h.HandleLogin(c)

		// Admin password check fails, and there is no matching user account
		// either, so the handler should fall through to the generic
		// "Invalid credentials" user-auth error rather than ever succeeding.
		if w.Code != http.StatusUnauthorized {
			t.Fatalf("status = %d, want 401; body=%s", w.Code, w.Body.String())
		}
	})
}

// TestHandleLogin_User covers the regular-user login branch: success,
// wrong password, and the 2FA-required gate.
func TestHandleLogin_User(t *testing.T) {
	t.Run("valid user credentials issue weather_session cookie", func(t *testing.T) {
		h, _, _ := newAuthTestHandler(t)
		config.SetGlobalConfig(&config.AppConfig{})
		t.Cleanup(func() { config.SetGlobalConfig(nil) })
		seedAuthUser(t, database.GetUsersDB(), "alice", "alice@example.com", "hunter2hunter2")

		c, w := newTestContextJSON(t, http.MethodPost, "/server/auth/login", map[string]string{
			"identifier": "alice",
			"password":   "hunter2hunter2",
		})
		withJSONAccept(c)
		h.HandleLogin(c)

		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
		}
		found := false
		for _, ck := range w.Result().Cookies() {
			if ck.Name == middleware.SessionCookieName && ck.Value != "" {
				found = true
			}
		}
		if !found {
			t.Errorf("expected %s cookie to be set; got cookies=%v", middleware.SessionCookieName, w.Result().Cookies())
		}
	})

	t.Run("unknown identifier returns generic invalid credentials", func(t *testing.T) {
		h, _, _ := newAuthTestHandler(t)
		config.SetGlobalConfig(&config.AppConfig{})
		t.Cleanup(func() { config.SetGlobalConfig(nil) })

		c, w := newTestContextJSON(t, http.MethodPost, "/server/auth/login", map[string]string{
			"identifier": "nobody",
			"password":   "whatever12345",
		})
		withJSONAccept(c)
		h.HandleLogin(c)

		if w.Code != http.StatusUnauthorized {
			t.Fatalf("status = %d, want 401; body=%s", w.Code, w.Body.String())
		}
	})

	t.Run("whitespace-padded password rejected before any DB lookup", func(t *testing.T) {
		h, _, _ := newAuthTestHandler(t)
		config.SetGlobalConfig(&config.AppConfig{})
		t.Cleanup(func() { config.SetGlobalConfig(nil) })

		c, w := newTestContextJSON(t, http.MethodPost, "/server/auth/login", map[string]string{
			"identifier": "alice",
			"password":   " leading-space",
		})
		withJSONAccept(c)
		h.HandleLogin(c)

		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400; body=%s", w.Code, w.Body.String())
		}
	})

	t.Run("missing identifier/password fails JSON bind with 400", func(t *testing.T) {
		h, _, _ := newAuthTestHandler(t)
		config.SetGlobalConfig(&config.AppConfig{})
		t.Cleanup(func() { config.SetGlobalConfig(nil) })

		c, w := newTestContextJSON(t, http.MethodPost, "/server/auth/login", map[string]string{})
		withJSONAccept(c)
		h.HandleLogin(c)

		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400; body=%s", w.Code, w.Body.String())
		}
	})

	t.Run("2FA enabled without code returns require_2fa flag, not a session", func(t *testing.T) {
		h, _, usersDB := newAuthTestHandler(t)
		config.SetGlobalConfig(&config.AppConfig{})
		t.Cleanup(func() { config.SetGlobalConfig(nil) })
		uid := seedAuthUser(t, usersDB, "bob", "bob@example.com", "hunter2hunter2")
		if _, err := usersDB.Exec(`UPDATE user_accounts SET two_factor_enabled = 1, two_factor_secret = 'JBSWY3DPEHPK3PXP' WHERE id = ?`, uid); err != nil {
			t.Fatalf("enable 2fa: %v", err)
		}

		c, w := newTestContextJSON(t, http.MethodPost, "/server/auth/login", map[string]string{
			"identifier": "bob",
			"password":   "hunter2hunter2",
		})
		withJSONAccept(c)
		h.HandleLogin(c)

		if w.Code != http.StatusUnauthorized {
			t.Fatalf("status = %d, want 401; body=%s", w.Code, w.Body.String())
		}
		var body map[string]interface{}
		json.Unmarshal(w.Body.Bytes(), &body)
		if body["require_2fa"] != true {
			t.Errorf("require_2fa = %v, want true; body=%s", body["require_2fa"], w.Body.String())
		}
		for _, ck := range w.Result().Cookies() {
			if ck.Name == middleware.SessionCookieName {
				t.Errorf("session cookie must not be issued before 2FA is verified")
			}
		}
	})

	t.Run("2FA enabled with wrong code is rejected", func(t *testing.T) {
		h, _, usersDB := newAuthTestHandler(t)
		config.SetGlobalConfig(&config.AppConfig{})
		t.Cleanup(func() { config.SetGlobalConfig(nil) })
		uid := seedAuthUser(t, usersDB, "carol", "carol@example.com", "hunter2hunter2")
		if _, err := usersDB.Exec(`UPDATE user_accounts SET two_factor_enabled = 1, two_factor_secret = 'JBSWY3DPEHPK3PXP' WHERE id = ?`, uid); err != nil {
			t.Fatalf("enable 2fa: %v", err)
		}

		c, w := newTestContextJSON(t, http.MethodPost, "/server/auth/login", map[string]string{
			"identifier":      "carol",
			"password":        "hunter2hunter2",
			"two_factor_code": "000000",
		})
		withJSONAccept(c)
		h.HandleLogin(c)

		if w.Code != http.StatusUnauthorized {
			t.Fatalf("status = %d, want 401; body=%s", w.Code, w.Body.String())
		}
	})
}

// TestHandleRegister covers the public-registration gate, success path, and
// duplicate-username error path.
func TestHandleRegister(t *testing.T) {
	t.Run("registration disabled returns 404", func(t *testing.T) {
		h, _, _ := newAuthTestHandler(t)
		config.SetGlobalConfig(&config.AppConfig{Users: config.UsersConfig{Enabled: true, Registration: config.RegistrationConfig{Mode: "invite"}}})
		t.Cleanup(func() { config.SetGlobalConfig(nil) })

		c, w := newTestContextJSON(t, http.MethodPost, "/server/auth/register", map[string]string{
			"username":         "newuser",
			"email":            "new@example.com",
			"password":         "supersecret1",
			"confirm_password": "supersecret1",
		})
		withJSONAccept(c)
		h.HandleRegister(c)

		if w.Code != http.StatusNotFound {
			t.Fatalf("status = %d, want 404; body=%s", w.Code, w.Body.String())
		}
	})

	t.Run("valid registration creates account and session", func(t *testing.T) {
		h, _, usersDB := newAuthTestHandler(t)
		config.SetGlobalConfig(&config.AppConfig{Users: config.UsersConfig{Enabled: true, Registration: config.RegistrationConfig{Mode: "open"}}})
		t.Cleanup(func() { config.SetGlobalConfig(nil) })

		c, w := newTestContextJSON(t, http.MethodPost, "/server/auth/register", map[string]string{
			"username":         "newuser",
			"email":            "new@example.com",
			"password":         "supersecret1",
			"confirm_password": "supersecret1",
		})
		withJSONAccept(c)
		h.HandleRegister(c)

		if w.Code != http.StatusCreated {
			t.Fatalf("status = %d, want 201; body=%s", w.Code, w.Body.String())
		}
		var count int
		if err := usersDB.QueryRow(`SELECT COUNT(*) FROM user_accounts WHERE username = 'newuser'`).Scan(&count); err != nil {
			t.Fatalf("query created user: %v", err)
		}
		if count != 1 {
			t.Errorf("expected exactly 1 user_accounts row for newuser, got %d", count)
		}
	})

	t.Run("duplicate username returns 400 with enumeration-safe message", func(t *testing.T) {
		h, _, usersDB := newAuthTestHandler(t)
		config.SetGlobalConfig(&config.AppConfig{Users: config.UsersConfig{Enabled: true, Registration: config.RegistrationConfig{Mode: "open"}}})
		t.Cleanup(func() { config.SetGlobalConfig(nil) })
		seedAuthUser(t, usersDB, "dupeuser", "dupe@example.com", "supersecret1")

		c, w := newTestContextJSON(t, http.MethodPost, "/server/auth/register", map[string]string{
			"username":         "dupeuser",
			"email":            "dupe2@example.com",
			"password":         "supersecret1",
			"confirm_password": "supersecret1",
		})
		withJSONAccept(c)
		h.HandleRegister(c)

		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400; body=%s", w.Code, w.Body.String())
		}
	})

	t.Run("mismatched confirm_password returns 400 before hitting DB", func(t *testing.T) {
		h, _, _ := newAuthTestHandler(t)
		config.SetGlobalConfig(&config.AppConfig{Users: config.UsersConfig{Enabled: true, Registration: config.RegistrationConfig{Mode: "open"}}})
		t.Cleanup(func() { config.SetGlobalConfig(nil) })

		c, w := newTestContextJSON(t, http.MethodPost, "/server/auth/register", map[string]string{
			"username":         "mismatch",
			"email":            "mismatch@example.com",
			"password":         "supersecret1",
			"confirm_password": "different1",
		})
		withJSONAccept(c)
		h.HandleRegister(c)

		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400; body=%s", w.Code, w.Body.String())
		}
	})
}

// TestHandleLogout covers the authenticated (session deleted + cookie
// cleared) and unauthenticated (cookie still cleared, no panic) paths.
func TestHandleLogout(t *testing.T) {
	t.Run("clears session cookie even with no active session", func(t *testing.T) {
		h, _, _ := newAuthTestHandler(t)
		c, w := newTestContext(http.MethodPost, "/server/auth/logout")
		c.Request.Header.Set("Accept", "application/json")
		h.HandleLogout(c)

		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
		}
		cleared := false
		for _, ck := range w.Result().Cookies() {
			if ck.Name == middleware.SessionCookieName && ck.MaxAge < 0 {
				cleared = true
			}
		}
		if !cleared {
			t.Errorf("expected %s cookie to be cleared (MaxAge<0); got %v", middleware.SessionCookieName, w.Result().Cookies())
		}
	})

	t.Run("deletes session row when an active session is present", func(t *testing.T) {
		h, _, usersDB := newAuthTestHandler(t)
		uid := seedAuthUser(t, usersDB, "logoutuser", "logout@example.com", "supersecret1")
		sessionModel := &models.SessionModel{DB: usersDB}
		session, err := sessionModel.Create(uid, 3600)
		if err != nil {
			t.Fatalf("create session: %v", err)
		}

		c, w := newTestContext(http.MethodPost, "/server/auth/logout")
		c.Request.Header.Set("Accept", "application/json")
		c.Set(middleware.SessionContextKey, session)
		h.HandleLogout(c)

		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
		}
		if _, err := sessionModel.GetByID(session.ID); err == nil {
			t.Errorf("expected session to be deleted from DB after logout")
		}
	})
}

// TestGetCurrentUser covers the unauthenticated 401 and authenticated 200
// paths of GetCurrentUser.
func TestGetCurrentUser(t *testing.T) {
	t.Run("unauthenticated returns 401", func(t *testing.T) {
		h, _, _ := newAuthTestHandler(t)
		c, w := newTestContext(http.MethodGet, "/api/v1/users")
		h.GetCurrentUser(c)

		if w.Code != http.StatusUnauthorized {
			t.Fatalf("status = %d, want 401; body=%s", w.Code, w.Body.String())
		}
	})

	t.Run("authenticated user gets profile payload", func(t *testing.T) {
		h, _, usersDB := newAuthTestHandler(t)
		uid := seedAuthUser(t, usersDB, "profileuser", "profile@example.com", "supersecret1")

		c, w := newTestContext(http.MethodGet, "/api/v1/users")
		setCurrentUser(c, uid)
		h.GetCurrentUser(c)

		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
		}
		var body map[string]interface{}
		if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
			t.Fatalf("decode response: %v", err)
		}
	})
}

// TestUpdateProfile covers the unauthenticated 401, malformed-body 400, and
// success 200 paths, plus the nil-request edge case on the free function.
func TestUpdateProfile(t *testing.T) {
	t.Run("unauthenticated returns 401", func(t *testing.T) {
		h, _, _ := newAuthTestHandler(t)
		c, w := newTestContextJSON(t, http.MethodPut, "/api/v1/users", map[string]string{"display_name": "x"})
		h.UpdateProfile(c)

		if w.Code != http.StatusUnauthorized {
			t.Fatalf("status = %d, want 401; body=%s", w.Code, w.Body.String())
		}
	})

	t.Run("malformed JSON body returns 400", func(t *testing.T) {
		h, _, usersDB := newAuthTestHandler(t)
		uid := seedAuthUser(t, usersDB, "malformeduser", "malformed@example.com", "supersecret1")

		c, w := newTestContextJSON(t, http.MethodPut, "/api/v1/users", "{not json")
		setCurrentUser(c, uid)
		h.UpdateProfile(c)

		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400; body=%s", w.Code, w.Body.String())
		}
	})

	t.Run("valid update persists display name and phone", func(t *testing.T) {
		h, _, usersDB := newAuthTestHandler(t)
		uid := seedAuthUser(t, usersDB, "updateuser", "update@example.com", "supersecret1")

		c, w := newTestContextJSON(t, http.MethodPut, "/api/v1/users", map[string]string{
			"display_name": "New Name",
			"phone":        "555-1234",
		})
		setCurrentUser(c, uid)
		h.UpdateProfile(c)

		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
		}
		var displayName, phone string
		if err := usersDB.QueryRow(`SELECT display_name, phone FROM user_accounts WHERE id = ?`, uid).Scan(&displayName, &phone); err != nil {
			t.Fatalf("query updated user: %v", err)
		}
		if displayName != "New Name" || phone != "555-1234" {
			t.Errorf("got display_name=%q phone=%q, want New Name/555-1234", displayName, phone)
		}
	})

	t.Run("nil request to UpdateCurrentUserProfile returns an error instead of panicking", func(t *testing.T) {
		_, _, usersDB := newAuthTestHandler(t)
		if err := UpdateCurrentUserProfile(usersDB, 1, nil); err == nil {
			t.Errorf("expected error for nil request, got nil")
		}
	})
}

// TestLoadCurrentUserProfile_UnknownUser covers the error path where the
// backing user row does not exist (e.g. deleted between session issuance
// and this call).
func TestLoadCurrentUserProfile_UnknownUser(t *testing.T) {
	_, _, usersDB := newAuthTestHandler(t)
	if _, err := LoadCurrentUserProfile(usersDB, 999999); err == nil {
		t.Errorf("expected error looking up a non-existent user ID, got nil")
	}
}

// TestShowLoginPage_RedirectsWhenAlreadyAuthenticated covers ShowLoginPage's
// early-return branch for already-authenticated users, without needing an
// HTML renderer wired up (the redirect happens before any template
// rendering, so it is safe to call without a configured gin template set).
func TestShowLoginPage_RedirectsWhenAlreadyAuthenticated(t *testing.T) {
	h, _, usersDB := newAuthTestHandler(t)
	config.SetGlobalConfig(&config.AppConfig{})
	t.Cleanup(func() { config.SetGlobalConfig(nil) })
	uid := seedAuthUser(t, usersDB, "alreadyin", "alreadyin@example.com", "supersecret1")

	c, w := newTestContext(http.MethodGet, "/server/auth/login")
	setCurrentUser(c, uid)
	h.ShowLoginPage(c)

	if w.Code != http.StatusFound {
		t.Fatalf("status = %d, want 302; body=%s", w.Code, w.Body.String())
	}
	if loc := w.Header().Get("Location"); loc == "" {
		t.Errorf("expected a non-empty Location redirect header")
	}
}

// TestShowRegisterPage_NotFoundWhenRegistrationDisabled covers the 404 gate
// (routed via the JSON-error branch since we request application/json, so
// it is safe to call without a template set).
func TestShowRegisterPage_NotFoundWhenRegistrationDisabled(t *testing.T) {
	h, _, _ := newAuthTestHandler(t)
	config.SetGlobalConfig(&config.AppConfig{Users: config.UsersConfig{Enabled: true, Registration: config.RegistrationConfig{Mode: "invite"}}})
	t.Cleanup(func() { config.SetGlobalConfig(nil) })

	c, w := newTestContext(http.MethodGet, "/server/auth/register")
	c.Request.Header.Set("Accept", "application/json")
	h.ShowRegisterPage(c)

	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404; body=%s", w.Code, w.Body.String())
	}
}

// handlerZoneWest and handlerZoneEast are fixed, deliberately non-local zones
// used by the timestamp regression tests in this package. A timestamp written
// in handlerZoneWest reads 11 hours behind UTC as wall-clock text, and one
// written in handlerZoneEast reads 13 hours ahead, so text ordering
// contradicts true instant ordering by a wider margin than any real host
// offset. A test built on them fails on every host timezone if a text
// comparison is ever reintroduced. Both names are short, all-uppercase and
// digit-free so the "MST" element of handlerLocalLayout can read them back;
// a name with digits or more than six letters would leave every fixture below
// unparseable and quietly exercise the fail-closed path instead.
var (
	handlerZoneWest = time.FixedZone("WST", -11*60*60)
	handlerZoneEast = time.FixedZone("EAT", 13*60*60)
)

// handlerLocalLayout is the layout modernc.org/sqlite produces when a bound
// time.Time is serialized through time.Time.String(). Rows in this layout
// coexist on disk with the canonical UTC text CURRENT_TIMESTAMP emits.
const handlerLocalLayout = "2006-01-02 15:04:05.999999999 -0700 MST"

// TestShowLoginPage_AdminSessionExpiryComparedAsInstant is the regression test
// for the admin-session validity check in ShowLoginPage. The old query filtered
// with "AND datetime(expires_at) > datetime('now')", which returns NULL for any
// row not stored in canonical UTC text: a live session written in a non-UTC
// layout never matched (silently logged out), while an already-expired row
// whose wall-clock text reads in the future would match under any text
// comparison. Both zone-skewed cases below fail against that implementation.
func TestShowLoginPage_AdminSessionExpiryComparedAsInstant(t *testing.T) {
	now := time.Now()

	cases := []struct {
		name         string
		sessionID    string
		expiresAt    string
		wantRedirect bool
	}{
		{
			name:         "live session in canonical UTC text redirects",
			sessionID:    "sess-live-utc",
			expiresAt:    dbtime.FormatSQLTimestamp(now.Add(time.Hour)),
			wantRedirect: true,
		},
		{
			name:         "expired session in canonical UTC text does not redirect",
			sessionID:    "sess-expired-utc",
			expiresAt:    dbtime.FormatSQLTimestamp(now.Add(-time.Hour)),
			wantRedirect: false,
		},
		{
			name:         "live session whose zone-skewed text reads as past still redirects",
			sessionID:    "sess-live-west",
			expiresAt:    now.Add(time.Hour).In(handlerZoneWest).Format(handlerLocalLayout),
			wantRedirect: true,
		},
		{
			name:         "expired session whose zone-skewed text reads as future does not redirect",
			sessionID:    "sess-expired-east",
			expiresAt:    now.Add(-time.Hour).In(handlerZoneEast).Format(handlerLocalLayout),
			wantRedirect: false,
		},
		{
			name:         "unparseable expiry fails closed and does not redirect",
			sessionID:    "sess-unparseable",
			expiresAt:    "not-a-timestamp",
			wantRedirect: false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h, serverDB, _ := newAuthTestHandler(t)
			config.SetGlobalConfig(&config.AppConfig{})
			t.Cleanup(func() { config.SetGlobalConfig(nil) })

			adminID := seedAdmin(t, serverDB, "sessionadmin", "sessionadmin@example.com", "supersecret1")
			if _, err := serverDB.Exec(`
				INSERT INTO server_admin_sessions (id, admin_id, expires_at)
				VALUES (?, ?, ?)
			`, tc.sessionID, adminID, tc.expiresAt); err != nil {
				t.Fatalf("seed admin session: %v", err)
			}

			c, w := newTestContext(http.MethodGet, "/server/auth/login")
			// A JSON Accept header keeps the non-redirect branch out of the HTML
			// renderer, which no test in this package has a template set for.
			c.Request.Header.Set("Accept", "application/json")
			c.Request.AddCookie(&http.Cookie{Name: "admin_session", Value: tc.sessionID})
			h.ShowLoginPage(c)

			redirected := w.Code == http.StatusFound
			if redirected != tc.wantRedirect {
				t.Fatalf("redirected = %v (status %d), want %v; body=%s", redirected, w.Code, tc.wantRedirect, w.Body.String())
			}
			if tc.wantRedirect && w.Header().Get("Location") == "" {
				t.Errorf("expected a non-empty Location redirect header")
			}
		})
	}
}
