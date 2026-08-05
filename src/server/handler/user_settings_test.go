package handler

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"strconv"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

// seedUserAccountRow inserts a minimal row into user_accounts so loadSettings
// (and anything that joins against it) has a row to find for the given id.
func seedUserAccountRow(t *testing.T, db *sql.DB, id int64, email, username string) {
	t.Helper()
	_, err := db.Exec(`
		INSERT INTO user_accounts (id, email, username, display_name, password_hash, bio, location, website, timezone, language, role, visibility, is_active, email_verified, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, id, email, username, username, "argon2id$dummy", "", "", "", "UTC", "en", "user", "public", true, true, time.Now(), time.Now())
	if err != nil {
		t.Fatalf("seed user_accounts: %v", err)
	}
}

func TestUserSettingsHandler_GetSettings(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("unauthenticated returns 401 with raw error shape", func(t *testing.T) {
		db := newTestUsersDB(t)
		h := NewUserSettingsHandler(db)
		c, w := newTestContextJSON(t, http.MethodGet, "/api/v1/users/settings", "")

		h.GetSettings(c)

		if w.Code != http.StatusUnauthorized {
			t.Fatalf("status = %d, want 401; body=%s", w.Code, w.Body.String())
		}
		var got map[string]string
		if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
			t.Fatalf("unmarshal body: %v", err)
		}
		if got["error"] != "Not authenticated" {
			t.Errorf(`error = %q, want "Not authenticated"`, got["error"])
		}
	})

	t.Run("authenticated user with no user_accounts row gets 500, not a panic", func(t *testing.T) {
		db := newTestUsersDB(t)
		h := NewUserSettingsHandler(db)
		c, w := newTestContextJSON(t, http.MethodGet, "/api/v1/users/settings", "")
		setCurrentUser(c, 999)

		h.GetSettings(c)

		if w.Code != http.StatusInternalServerError {
			t.Fatalf("status = %d, want 500; body=%s", w.Code, w.Body.String())
		}
	})

	// BUG: getOrCreatePreferences (user_settings.go:367-371) runs
	// `SELECT id, user_id, theme, ... FROM user_preferences WHERE user_id = ?`
	// but the real production user_preferences table (src/database/users_schema.go,
	// "CREATE TABLE IF NOT EXISTS user_preferences") has NO `id` column at all —
	// its primary key is `user_id` itself. Against the real schema this query
	// fails on every single call with "no such column: id", which loadSettings
	// propagates as an error, and GetSettings turns into an unconditional
	// 500 {"error":"Failed to get preferences"} for every authenticated user.
	// Repro: seed a user_accounts row, call GetSettings — expect 200, get 500.
	// This test encodes the CORRECT expected behavior (200 + populated
	// settings) and is expected to fail until getOrCreatePreferences' SELECT
	// (and its models.UserPreferences.Scan target list) drops the `id` column.
	t.Run("BUG: success returns full settings and auto-creates default preferences", func(t *testing.T) {
		db := newTestUsersDB(t)
		h := NewUserSettingsHandler(db)
		seedUserAccountRow(t, db, 1, "a@example.com", "alice")

		c, w := newTestContextJSON(t, http.MethodGet, "/api/v1/users/settings", "")
		setCurrentUser(c, 1)

		h.GetSettings(c)

		if w.Code != http.StatusOK {
			t.Errorf("status = %d, want 200; body=%s", w.Code, w.Body.String())
			return
		}
		var resp UserSettingsResponse
		if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
			t.Fatalf("unmarshal body: %v; body=%s", err, w.Body.String())
		}
		if resp.Account.DisplayName != "alice" {
			t.Errorf("Account.DisplayName = %q, want alice", resp.Account.DisplayName)
		}

		// A second call must reuse the auto-created preferences row rather than
		// erroring (idempotency of getOrCreatePreferences).
		c2, w2 := newTestContextJSON(t, http.MethodGet, "/api/v1/users/settings", "")
		setCurrentUser(c2, 1)
		h.GetSettings(c2)
		if w2.Code != http.StatusOK {
			t.Errorf("second call status = %d, want 200; body=%s", w2.Code, w2.Body.String())
		}
	})
}

func TestUserSettingsHandler_UpdateSettings(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("unauthenticated returns 401", func(t *testing.T) {
		db := newTestUsersDB(t)
		h := NewUserSettingsHandler(db)
		c, w := newTestContextJSON(t, http.MethodPost, "/api/v1/users/settings", "{}")

		h.UpdateSettings(c)

		if w.Code != http.StatusUnauthorized {
			t.Fatalf("status = %d, want 401; body=%s", w.Code, w.Body.String())
		}
	})

	t.Run("malformed JSON body returns 400", func(t *testing.T) {
		db := newTestUsersDB(t)
		h := NewUserSettingsHandler(db)
		seedUserAccountRow(t, db, 1, "b@example.com", "bob")
		c, w := newTestContextJSON(t, http.MethodPost, "/api/v1/users/settings", "{not json")
		setCurrentUser(c, 1)

		h.UpdateSettings(c)

		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400; body=%s", w.Code, w.Body.String())
		}
	})

	t.Run("bio over 500 characters returns validation 400", func(t *testing.T) {
		db := newTestUsersDB(t)
		h := NewUserSettingsHandler(db)
		seedUserAccountRow(t, db, 1, "c@example.com", "carol")

		longBio := make([]byte, 501)
		for i := range longBio {
			longBio[i] = 'x'
		}
		req := UpdateSettingsRequest{Account: &AccountSettings{Bio: string(longBio)}}
		c, w := newTestContextJSON(t, http.MethodPost, "/api/v1/users/settings", req)
		setCurrentUser(c, 1)

		h.UpdateSettings(c)

		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400; body=%s", w.Code, w.Body.String())
		}
	})

	t.Run("invalid theme returns validation 400", func(t *testing.T) {
		db := newTestUsersDB(t)
		h := NewUserSettingsHandler(db)
		seedUserAccountRow(t, db, 1, "d@example.com", "dave")

		req := UpdateSettingsRequest{Appearance: &AppearanceSettings{Theme: "rainbow"}}
		c, w := newTestContextJSON(t, http.MethodPost, "/api/v1/users/settings", req)
		setCurrentUser(c, 1)

		h.UpdateSettings(c)

		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400; body=%s", w.Code, w.Body.String())
		}
	})

	t.Run("success updates account settings and returns 200", func(t *testing.T) {
		db := newTestUsersDB(t)
		h := NewUserSettingsHandler(db)
		seedUserAccountRow(t, db, 1, "e@example.com", "erin")

		req := UpdateSettingsRequest{Account: &AccountSettings{Bio: "hello world"}}
		c, w := newTestContextJSON(t, http.MethodPost, "/api/v1/users/settings", req)
		setCurrentUser(c, 1)

		h.UpdateSettings(c)

		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
		}
	})
}

func TestUserSettingsHandler_Tokens(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("CreateToken unauthenticated returns 401", func(t *testing.T) {
		db := newTestUsersDB(t)
		h := NewUserSettingsHandler(db)
		c, w := newTestContextJSON(t, http.MethodPost, "/api/v1/users/tokens", "{}")

		h.CreateToken(c)

		if w.Code != http.StatusUnauthorized {
			t.Fatalf("status = %d, want 401; body=%s", w.Code, w.Body.String())
		}
	})

	t.Run("CreateToken success returns plaintext token once", func(t *testing.T) {
		db := newTestUsersDB(t)
		h := NewUserSettingsHandler(db)
		seedUserAccountRow(t, db, 1, "f@example.com", "frank")

		c, w := newTestContextJSON(t, http.MethodPost, "/api/v1/users/tokens", CreateTokenRequest{Name: "my token"})
		setCurrentUser(c, 1)

		h.CreateToken(c)

		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
		}
		var got map[string]string
		if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if got["token"] == "" {
			t.Errorf("expected non-empty token in response, got %v", got)
		}
	})

	t.Run("CreateToken enforces max 5 tokens per user", func(t *testing.T) {
		db := newTestUsersDB(t)
		h := NewUserSettingsHandler(db)
		seedUserAccountRow(t, db, 1, "g@example.com", "gina")

		for i := 0; i < 5; i++ {
			c, w := newTestContextJSON(t, http.MethodPost, "/api/v1/users/tokens", CreateTokenRequest{Name: "tok"})
			setCurrentUser(c, 1)
			h.CreateToken(c)
			if w.Code != http.StatusOK {
				t.Fatalf("seed token %d: status = %d, body=%s", i, w.Code, w.Body.String())
			}
		}

		c, w := newTestContextJSON(t, http.MethodPost, "/api/v1/users/tokens", CreateTokenRequest{Name: "sixth"})
		setCurrentUser(c, 1)
		h.CreateToken(c)

		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400 at token limit; body=%s", w.Code, w.Body.String())
		}
	})

	t.Run("RevokeToken not found returns 404", func(t *testing.T) {
		db := newTestUsersDB(t)
		h := NewUserSettingsHandler(db)
		seedUserAccountRow(t, db, 1, "h@example.com", "hank")

		c, w := newTestContextJSON(t, http.MethodDelete, "/api/v1/users/tokens/999", "")
		c.Params = gin.Params{{Key: "id", Value: "999"}}
		setCurrentUser(c, 1)

		h.RevokeToken(c)

		if w.Code != http.StatusNotFound {
			t.Fatalf("status = %d, want 404; body=%s", w.Code, w.Body.String())
		}
	})

	t.Run("ListTokens success returns empty list for a fresh user", func(t *testing.T) {
		db := newTestUsersDB(t)
		h := NewUserSettingsHandler(db)
		seedUserAccountRow(t, db, 1, "i@example.com", "ivan")

		c, w := newTestContextJSON(t, http.MethodGet, "/api/v1/users/tokens", "")
		setCurrentUser(c, 1)

		h.ListTokens(c)

		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
		}
	})
}

func TestUserSettingsHandler_Sessions(t *testing.T) {
	gin.SetMode(gin.TestMode)

	// Session model methods use database.GetUsersDB() internally rather than
	// an injected field, so these tests must also wire the package-level
	// global DB via setGlobalTestDualDB in addition to constructing the
	// handler with its own DB field.
	newSessionsHandler := func(t *testing.T) (*UserSettingsHandler, *sql.DB) {
		t.Helper()
		serverDB := newTestServerDB(t)
		usersDB := newTestUsersDB(t)
		setGlobalTestDualDB(t, serverDB, usersDB)
		return NewUserSettingsHandler(usersDB), usersDB
	}

	t.Run("ListSessions unauthenticated returns 401", func(t *testing.T) {
		db := newTestUsersDB(t)
		h := NewUserSettingsHandler(db)
		c, w := newTestContextJSON(t, http.MethodGet, "/api/v1/users/sessions", "")

		h.ListSessions(c)

		if w.Code != http.StatusUnauthorized {
			t.Fatalf("status = %d, want 401; body=%s", w.Code, w.Body.String())
		}
	})

	t.Run("ListSessions success with no sessions returns empty list", func(t *testing.T) {
		h, dbLike := newSessionsHandler(t)
		seedUserAccountRow(t, dbLike, 1, "j@example.com", "jill")

		c, w := newTestContextJSON(t, http.MethodGet, "/api/v1/users/sessions", "")
		setCurrentUser(c, 1)

		h.ListSessions(c)

		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
		}
	})

	t.Run("RevokeSession empty id returns 400", func(t *testing.T) {
		h, dbLike := newSessionsHandler(t)
		seedUserAccountRow(t, dbLike, 1, "k@example.com", "kim")

		c, w := newTestContextJSON(t, http.MethodDelete, "/api/v1/users/sessions/", "")
		c.Params = gin.Params{{Key: "id", Value: ""}}
		setCurrentUser(c, 1)

		h.RevokeSession(c)

		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400; body=%s", w.Code, w.Body.String())
		}
	})

	t.Run("RevokeSession non-numeric id returns 400", func(t *testing.T) {
		h, dbLike := newSessionsHandler(t)
		seedUserAccountRow(t, dbLike, 1, "l@example.com", "leo")

		c, w := newTestContextJSON(t, http.MethodDelete, "/api/v1/users/sessions/abc", "")
		c.Params = gin.Params{{Key: "id", Value: "abc"}}
		setCurrentUser(c, 1)

		h.RevokeSession(c)

		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400; body=%s", w.Code, w.Body.String())
		}
	})

	t.Run("RevokeSession unknown id returns 404", func(t *testing.T) {
		h, dbLike := newSessionsHandler(t)
		seedUserAccountRow(t, dbLike, 1, "m@example.com", "mia")

		c, w := newTestContextJSON(t, http.MethodDelete, "/api/v1/users/sessions/12345", "")
		c.Params = gin.Params{{Key: "id", Value: "12345"}}
		setCurrentUser(c, 1)

		h.RevokeSession(c)

		if w.Code != http.StatusNotFound {
			t.Fatalf("status = %d, want 404; body=%s", w.Code, w.Body.String())
		}
	})

	// BUG: RevokeSession's ownership lookup (user_settings.go:769) is
	// `SELECT user_id FROM user_sessions WHERE id = ? AND expires_at >
	// CURRENT_TIMESTAMP`. modernc.org/sqlite binds a Go time.Time parameter
	// as its RFC3339Nano text representation WITH a numeric zone offset
	// (e.g. "2026-07-21T05:23:45.484833776-04:00"), while SQLite's
	// CURRENT_TIMESTAMP is UTC "YYYY-MM-DD HH:MM:SS" with no "T", no
	// fractional seconds, and no offset. `>` on TEXT affinity columns is a
	// byte-wise string comparison, so expires_at set even an hour in the
	// future compares as NOT greater than CURRENT_TIMESTAMP purely because
	// of the differing string shape/offset — confirmed directly: for
	// expires_at="2026-07-21T05:23:45.484833776-04:00" and
	// CURRENT_TIMESTAMP="2026-07-21 08:23:45", `expires_at >
	// CURRENT_TIMESTAMP` evaluates to 0 (false) even though the moment in
	// time is genuinely ~1h in the future. Every "still valid" row is
	// therefore invisible to this WHERE clause, so RevokeSession always
	// 404s "Session not found" for a real, unexpired session, and the
	// intended 403 (wrong owner) branch is unreachable. The same pattern is
	// used for real session validity in src/server/model/user.go:862 and
	// admin session checks in src/server/model/admin.go:755,908 — this is a
	// systemic, not file-local, timestamp-comparison bug.
	// These two subtests encode the CORRECT expected behavior (403 for a
	// session owned by someone else, 200 for the owner's own session) and
	// are expected to fail until expires_at is stored/compared in a
	// consistent, sortable format (e.g. UTC with no offset, or compared via
	// a parsed-time query rather than raw string `>`).
	t.Run("BUG: RevokeSession belonging to another user returns 403", func(t *testing.T) {
		h, dbLike := newSessionsHandler(t)
		seedUserAccountRow(t, dbLike, 1, "n@example.com", "nate")
		seedUserAccountRow(t, dbLike, 2, "o@example.com", "olga")

		res, err := dbLike.Exec(`
			INSERT INTO user_sessions (user_id, token_hash, ip_address, user_agent, created_at, expires_at, last_used_at)
			VALUES (?, ?, ?, ?, ?, ?, ?)
		`, 2, "deadbeef", "127.0.0.1", "test-agent", time.Now(), time.Now().Add(time.Hour), time.Now())
		if err != nil {
			t.Fatalf("seed session: %v", err)
		}
		rowID, err := res.LastInsertId()
		if err != nil {
			t.Fatalf("last insert id: %v", err)
		}

		c, w := newTestContextJSON(t, http.MethodDelete, "/api/v1/users/sessions/1", "")
		c.Params = gin.Params{{Key: "id", Value: strconv.FormatInt(rowID, 10)}}
		setCurrentUser(c, 1)

		h.RevokeSession(c)

		if w.Code != http.StatusForbidden {
			t.Errorf("status = %d, want 403; body=%s", w.Code, w.Body.String())
		}
	})

	t.Run("BUG: RevokeSession success deletes owner's own session", func(t *testing.T) {
		h, dbLike := newSessionsHandler(t)
		seedUserAccountRow(t, dbLike, 1, "p@example.com", "paul")

		res, err := dbLike.Exec(`
			INSERT INTO user_sessions (user_id, token_hash, ip_address, user_agent, created_at, expires_at, last_used_at)
			VALUES (?, ?, ?, ?, ?, ?, ?)
		`, 1, "cafef00d", "127.0.0.1", "test-agent", time.Now(), time.Now().Add(time.Hour), time.Now())
		if err != nil {
			t.Fatalf("seed session: %v", err)
		}
		rowID, err := res.LastInsertId()
		if err != nil {
			t.Fatalf("last insert id: %v", err)
		}

		c, w := newTestContextJSON(t, http.MethodDelete, "/api/v1/users/sessions/1", "")
		c.Params = gin.Params{{Key: "id", Value: strconv.FormatInt(rowID, 10)}}
		setCurrentUser(c, 1)

		h.RevokeSession(c)

		if w.Code != http.StatusOK {
			t.Errorf("status = %d, want 200; body=%s", w.Code, w.Body.String())
		}
	})

	t.Run("RevokeAllSessions unauthenticated returns 401", func(t *testing.T) {
		db := newTestUsersDB(t)
		h := NewUserSettingsHandler(db)
		c, w := newTestContextJSON(t, http.MethodDelete, "/api/v1/users/sessions", "")

		h.RevokeAllSessions(c)

		if w.Code != http.StatusUnauthorized {
			t.Fatalf("status = %d, want 401; body=%s", w.Code, w.Body.String())
		}
	})

	t.Run("RevokeAllSessions success with no sessions is a no-op 200", func(t *testing.T) {
		h, dbLike := newSessionsHandler(t)
		seedUserAccountRow(t, dbLike, 1, "q@example.com", "quinn")

		c, w := newTestContextJSON(t, http.MethodDelete, "/api/v1/users/sessions", "")
		setCurrentUser(c, 1)

		h.RevokeAllSessions(c)

		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
		}
	})
}

// TestUserSettingsHandler_ShowNotificationSettings_Unauthenticated verifies
// an unauthenticated request (no user in the gin context) is redirected to
// the login page rather than attempting a template render.
func TestUserSettingsHandler_ShowNotificationSettings_Unauthenticated(t *testing.T) {
	h := &UserSettingsHandler{DB: newTestServerDB(t)}

	c, w := newAPITestContext("/users/settings/notifications")
	h.ShowNotificationSettings(c)

	if w.Code != http.StatusFound {
		t.Fatalf("expected status 302, got %d", w.Code)
	}
	if location := w.Header().Get("Location"); location != "/server/auth/login" {
		t.Errorf("expected redirect to /server/auth/login, got %q", location)
	}
}
