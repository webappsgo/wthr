package handler

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/webappsgo/wthr/src/common/dbtime"
	"github.com/webappsgo/wthr/src/server/middleware"
	models "github.com/webappsgo/wthr/src/server/model"
	"github.com/webappsgo/wthr/src/server/reqctx"
)

// newUSTestRequest builds a request/recorder pair for the user settings
// handler tests, kept local to avoid depending on unexported helpers owned
// by another file beyond handler_helpers_test.go.
func newUSTestRequest(t *testing.T, method, target string, body interface{}) (*http.Request, *httptest.ResponseRecorder) {
	t.Helper()
	w := httptest.NewRecorder()

	var raw []byte
	switch v := body.(type) {
	case nil:
		raw = nil
	case string:
		raw = []byte(v)
	case []byte:
		raw = v
	default:
		var err error
		raw, err = json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal request body: %v", err)
		}
	}

	r := httptest.NewRequest(method, target, bytes.NewReader(raw))
	r.Header.Set("Content-Type", "application/json")
	return r, w
}

// withUSCurrentUser attaches an authenticated user to the request context,
// the same key middleware.AuthMiddleware sets on a real request.
func withUSCurrentUser(r *http.Request, id int64) *http.Request {
	return r.WithContext(reqctx.Set(r.Context(), middleware.UserContextKey, &models.User{ID: id}))
}

// withUSURLParam attaches a chi route param to the request context.
func withUSURLParam(r *http.Request, key, value string) *http.Request {
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add(key, value)
	return r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, rctx))
}

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
	t.Run("unauthenticated returns 401 with canonical error shape", func(t *testing.T) {
		db := newTestUsersDB(t)
		h := NewUserSettingsHandler(db)
		r, w := newUSTestRequest(t, http.MethodGet, "/api/v1/users/settings", "")

		h.GetSettings(w, r)

		if w.Code != http.StatusUnauthorized {
			t.Fatalf("status = %d, want 401; body=%s", w.Code, w.Body.String())
		}
		var got map[string]interface{}
		if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
			t.Fatalf("unmarshal body: %v", err)
		}
		if got["error"] != "UNAUTHORIZED" {
			t.Errorf(`error = %q, want "UNAUTHORIZED"`, got["error"])
		}
		if got["message"] != "Not authenticated" {
			t.Errorf(`message = %q, want "Not authenticated"`, got["message"])
		}
	})

	t.Run("authenticated user with no user_accounts row gets 500, not a panic", func(t *testing.T) {
		db := newTestUsersDB(t)
		h := NewUserSettingsHandler(db)
		r, w := newUSTestRequest(t, http.MethodGet, "/api/v1/users/settings", "")
		r = withUSCurrentUser(r, 999)

		h.GetSettings(w, r)

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

		r, w := newUSTestRequest(t, http.MethodGet, "/api/v1/users/settings", "")
		r = withUSCurrentUser(r, 1)

		h.GetSettings(w, r)

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
		r2, w2 := newUSTestRequest(t, http.MethodGet, "/api/v1/users/settings", "")
		r2 = withUSCurrentUser(r2, 1)
		h.GetSettings(w2, r2)
		if w2.Code != http.StatusOK {
			t.Errorf("second call status = %d, want 200; body=%s", w2.Code, w2.Body.String())
		}
	})
}

func TestUserSettingsHandler_UpdateSettings(t *testing.T) {
	t.Run("unauthenticated returns 401", func(t *testing.T) {
		db := newTestUsersDB(t)
		h := NewUserSettingsHandler(db)
		r, w := newUSTestRequest(t, http.MethodPost, "/api/v1/users/settings", "{}")

		h.UpdateSettings(w, r)

		if w.Code != http.StatusUnauthorized {
			t.Fatalf("status = %d, want 401; body=%s", w.Code, w.Body.String())
		}
	})

	t.Run("malformed JSON body returns 400", func(t *testing.T) {
		db := newTestUsersDB(t)
		h := NewUserSettingsHandler(db)
		seedUserAccountRow(t, db, 1, "b@example.com", "bob")
		r, w := newUSTestRequest(t, http.MethodPost, "/api/v1/users/settings", "{not json")
		r = withUSCurrentUser(r, 1)

		h.UpdateSettings(w, r)

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
		r, w := newUSTestRequest(t, http.MethodPost, "/api/v1/users/settings", req)
		r = withUSCurrentUser(r, 1)

		h.UpdateSettings(w, r)

		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400; body=%s", w.Code, w.Body.String())
		}
	})

	t.Run("invalid theme returns validation 400", func(t *testing.T) {
		db := newTestUsersDB(t)
		h := NewUserSettingsHandler(db)
		seedUserAccountRow(t, db, 1, "d@example.com", "dave")

		req := UpdateSettingsRequest{Appearance: &AppearanceSettings{Theme: "rainbow"}}
		r, w := newUSTestRequest(t, http.MethodPost, "/api/v1/users/settings", req)
		r = withUSCurrentUser(r, 1)

		h.UpdateSettings(w, r)

		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400; body=%s", w.Code, w.Body.String())
		}
	})

	t.Run("success updates account settings and returns 200", func(t *testing.T) {
		db := newTestUsersDB(t)
		h := NewUserSettingsHandler(db)
		seedUserAccountRow(t, db, 1, "e@example.com", "erin")

		req := UpdateSettingsRequest{Account: &AccountSettings{Bio: "hello world"}}
		r, w := newUSTestRequest(t, http.MethodPost, "/api/v1/users/settings", req)
		r = withUSCurrentUser(r, 1)

		h.UpdateSettings(w, r)

		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
		}
	})
}

func TestUserSettingsHandler_Tokens(t *testing.T) {
	t.Run("CreateToken unauthenticated returns 401", func(t *testing.T) {
		db := newTestUsersDB(t)
		h := NewUserSettingsHandler(db)
		r, w := newUSTestRequest(t, http.MethodPost, "/api/v1/users/tokens", "{}")

		h.CreateToken(w, r)

		if w.Code != http.StatusUnauthorized {
			t.Fatalf("status = %d, want 401; body=%s", w.Code, w.Body.String())
		}
	})

	t.Run("CreateToken success returns plaintext token once", func(t *testing.T) {
		db := newTestUsersDB(t)
		h := NewUserSettingsHandler(db)
		seedUserAccountRow(t, db, 1, "f@example.com", "frank")

		r, w := newUSTestRequest(t, http.MethodPost, "/api/v1/users/tokens", CreateTokenRequest{Name: "my token"})
		r = withUSCurrentUser(r, 1)

		h.CreateToken(w, r)

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
			r, w := newUSTestRequest(t, http.MethodPost, "/api/v1/users/tokens", CreateTokenRequest{Name: "tok"})
			r = withUSCurrentUser(r, 1)
			h.CreateToken(w, r)
			if w.Code != http.StatusOK {
				t.Fatalf("seed token %d: status = %d, body=%s", i, w.Code, w.Body.String())
			}
		}

		r, w := newUSTestRequest(t, http.MethodPost, "/api/v1/users/tokens", CreateTokenRequest{Name: "sixth"})
		r = withUSCurrentUser(r, 1)
		h.CreateToken(w, r)

		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400 at token limit; body=%s", w.Code, w.Body.String())
		}
	})

	t.Run("RevokeToken not found returns 404", func(t *testing.T) {
		db := newTestUsersDB(t)
		h := NewUserSettingsHandler(db)
		seedUserAccountRow(t, db, 1, "h@example.com", "hank")

		r, w := newUSTestRequest(t, http.MethodDelete, "/api/v1/users/tokens/999", "")
		r = withUSURLParam(r, "id", "999")
		r = withUSCurrentUser(r, 1)

		h.RevokeToken(w, r)

		if w.Code != http.StatusNotFound {
			t.Fatalf("status = %d, want 404; body=%s", w.Code, w.Body.String())
		}
	})

	t.Run("ListTokens success returns empty list for a fresh user", func(t *testing.T) {
		db := newTestUsersDB(t)
		h := NewUserSettingsHandler(db)
		seedUserAccountRow(t, db, 1, "i@example.com", "ivan")

		r, w := newUSTestRequest(t, http.MethodGet, "/api/v1/users/tokens", "")
		r = withUSCurrentUser(r, 1)

		h.ListTokens(w, r)

		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
		}
	})
}

func TestUserSettingsHandler_Sessions(t *testing.T) {
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
		r, w := newUSTestRequest(t, http.MethodGet, "/api/v1/users/sessions", "")

		h.ListSessions(w, r)

		if w.Code != http.StatusUnauthorized {
			t.Fatalf("status = %d, want 401; body=%s", w.Code, w.Body.String())
		}
	})

	t.Run("ListSessions success with no sessions returns empty list", func(t *testing.T) {
		h, dbLike := newSessionsHandler(t)
		seedUserAccountRow(t, dbLike, 1, "j@example.com", "jill")

		r, w := newUSTestRequest(t, http.MethodGet, "/api/v1/users/sessions", "")
		r = withUSCurrentUser(r, 1)

		h.ListSessions(w, r)

		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
		}
	})

	t.Run("RevokeSession empty id returns 400", func(t *testing.T) {
		h, dbLike := newSessionsHandler(t)
		seedUserAccountRow(t, dbLike, 1, "k@example.com", "kim")

		r, w := newUSTestRequest(t, http.MethodDelete, "/api/v1/users/sessions/", "")
		r = withUSURLParam(r, "id", "")
		r = withUSCurrentUser(r, 1)

		h.RevokeSession(w, r)

		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400; body=%s", w.Code, w.Body.String())
		}
	})

	t.Run("RevokeSession non-numeric id returns 400", func(t *testing.T) {
		h, dbLike := newSessionsHandler(t)
		seedUserAccountRow(t, dbLike, 1, "l@example.com", "leo")

		r, w := newUSTestRequest(t, http.MethodDelete, "/api/v1/users/sessions/abc", "")
		r = withUSURLParam(r, "id", "abc")
		r = withUSCurrentUser(r, 1)

		h.RevokeSession(w, r)

		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400; body=%s", w.Code, w.Body.String())
		}
	})

	t.Run("RevokeSession unknown id returns 404", func(t *testing.T) {
		h, dbLike := newSessionsHandler(t)
		seedUserAccountRow(t, dbLike, 1, "m@example.com", "mia")

		r, w := newUSTestRequest(t, http.MethodDelete, "/api/v1/users/sessions/12345", "")
		r = withUSURLParam(r, "id", "12345")
		r = withUSCurrentUser(r, 1)

		h.RevokeSession(w, r)

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

		r, w := newUSTestRequest(t, http.MethodDelete, "/api/v1/users/sessions/1", "")
		r = withUSURLParam(r, "id", strconv.FormatInt(rowID, 10))
		r = withUSCurrentUser(r, 1)

		h.RevokeSession(w, r)

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

		r, w := newUSTestRequest(t, http.MethodDelete, "/api/v1/users/sessions/1", "")
		r = withUSURLParam(r, "id", strconv.FormatInt(rowID, 10))
		r = withUSCurrentUser(r, 1)

		h.RevokeSession(w, r)

		if w.Code != http.StatusOK {
			t.Errorf("status = %d, want 200; body=%s", w.Code, w.Body.String())
		}
	})

	t.Run("RevokeAllSessions unauthenticated returns 401", func(t *testing.T) {
		db := newTestUsersDB(t)
		h := NewUserSettingsHandler(db)
		r, w := newUSTestRequest(t, http.MethodDelete, "/api/v1/users/sessions", "")

		h.RevokeAllSessions(w, r)

		if w.Code != http.StatusUnauthorized {
			t.Fatalf("status = %d, want 401; body=%s", w.Code, w.Body.String())
		}
	})

	t.Run("RevokeAllSessions success with no sessions is a no-op 200", func(t *testing.T) {
		h, dbLike := newSessionsHandler(t)
		seedUserAccountRow(t, dbLike, 1, "q@example.com", "quinn")

		r, w := newUSTestRequest(t, http.MethodDelete, "/api/v1/users/sessions", "")
		r = withUSCurrentUser(r, 1)

		h.RevokeAllSessions(w, r)

		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
		}
	})
}

// TestUserSettingsHandler_ShowNotificationSettings_Unauthenticated verifies
// an unauthenticated request (no user in the request context) is redirected
// to the login page rather than attempting a template render.
func TestUserSettingsHandler_ShowNotificationSettings_Unauthenticated(t *testing.T) {
	h := &UserSettingsHandler{DB: newTestServerDB(t)}

	r, w := newUSTestRequest(t, http.MethodGet, "/users/settings/notifications", nil)
	h.ShowNotificationSettings(w, r)

	if w.Code != http.StatusFound {
		t.Fatalf("expected status 302, got %d", w.Code)
	}
	if location := w.Header().Get("Location"); location != "/server/auth/login" {
		t.Errorf("expected redirect to /server/auth/login, got %q", location)
	}
}

// TestGetUserTokens_LegacyTimestampLayoutsSurvive is the regression test for
// the token listing. getUserTokens used to scan created_at/expires_at/
// last_used_at straight into time.Time and sql.NullTime and to `continue` on a
// scan error, so any row whose timestamp was written in the local-zone
// time.Time.String() layout the SQLite driver produces for a bound time.Time -
// a layout the driver's own scanner does not accept - vanished from the user's
// own token list with no error anywhere. The timestamps are now scanned untyped
// and parsed with dbtime, so the row is returned either way and only an
// unparseable timestamp is dropped, not the token.
func TestGetUserTokens_LegacyTimestampLayoutsSurvive(t *testing.T) {
	db := newTestUsersDB(t)
	seedUserAccountRow(t, db, 1, "tokenzone@example.com", "tokenzone")
	h := NewUserSettingsHandler(db)

	now := time.Now().Truncate(time.Second)
	rows := []struct {
		name      string
		createdAt string
		wantTime  time.Time
	}{
		{
			name:      "canonical-utc",
			createdAt: dbtime.FormatSQLTimestamp(now),
			wantTime:  now.UTC(),
		},
		{
			name:      "legacy-west",
			createdAt: now.In(handlerZoneWest).Format(handlerLocalLayout),
			wantTime:  now.UTC(),
		},
		{
			name:      "legacy-east",
			createdAt: now.In(handlerZoneEast).Format(handlerLocalLayout),
			wantTime:  now.UTC(),
		},
		{
			name:      "unparseable",
			createdAt: "not-a-timestamp",
			wantTime:  time.Time{},
		},
	}

	for i, row := range rows {
		_, err := db.Exec(`
			INSERT INTO user_tokens (user_id, token_hash, token_prefix, name, scopes, created_at, expires_at, last_used_at)
			VALUES (?, ?, 'usr_', ?, 'read', ?, ?, ?)
		`, 1, strconv.Itoa(i)+"-hash", row.name, row.createdAt, row.createdAt, row.createdAt)
		if err != nil {
			t.Fatalf("seed token row %q: %v", row.name, err)
		}
	}

	got, err := h.getUserTokens(1)
	if err != nil {
		t.Fatalf("getUserTokens: %v", err)
	}
	if len(got) != len(rows) {
		t.Fatalf("returned %d tokens, want %d - a legacy timestamp layout is silently dropping rows", len(got), len(rows))
	}

	byName := make(map[string]UserToken, len(got))
	for _, token := range got {
		byName[token.Name] = token
	}
	for _, row := range rows {
		token, ok := byName[row.name]
		if !ok {
			t.Fatalf("token %q missing from the listing", row.name)
		}
		if !token.CreatedAt.Equal(row.wantTime) {
			t.Errorf("token %q created_at = %v, want %v", row.name, token.CreatedAt, row.wantTime)
		}
		if row.wantTime.IsZero() {
			if token.ExpiresAt != nil || token.LastUsedAt != nil {
				t.Errorf("token %q: unparseable expires_at/last_used_at should stay nil, got %v/%v", row.name, token.ExpiresAt, token.LastUsedAt)
			}
			continue
		}
		if token.ExpiresAt == nil || !token.ExpiresAt.Equal(row.wantTime) {
			t.Errorf("token %q expires_at = %v, want %v", row.name, token.ExpiresAt, row.wantTime)
		}
		if token.LastUsedAt == nil || !token.LastUsedAt.Equal(row.wantTime) {
			t.Errorf("token %q last_used_at = %v, want %v", row.name, token.LastUsedAt, row.wantTime)
		}
	}
}
