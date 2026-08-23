package handler

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/webappsgo/wthr/src/common/dbtime"
	"github.com/webappsgo/wthr/src/server/middleware"
	models "github.com/webappsgo/wthr/src/server/model"
	"github.com/webappsgo/wthr/src/server/reqctx"
)

// newTestContext builds a bare net/http request/recorder pair with no body,
// for handlers invoked without a JSON payload.
func newTestContext(method, target string) (*http.Request, *httptest.ResponseRecorder) {
	w := httptest.NewRecorder()
	r := httptest.NewRequest(method, target, nil)
	return r, w
}

// setURLParam injects a chi URL parameter into r's context, replacing the
// prior router's Params-slice-based param injection this file used before
// the chi migration.
func setURLParam(r *http.Request, key, value string) *http.Request {
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add(key, value)
	return r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, rctx))
}

// withReqCtxValue stores an arbitrary value under key on r's context, for
// tests that need to simulate both a well-typed and a mistyped "admin_id"
// (see TestAdminHandler_ShowSettingsPage_RedirectBranches).
func withReqCtxValue(r *http.Request, key string, value interface{}) *http.Request {
	return r.WithContext(reqctx.Set(r.Context(), key, value))
}

// newAdminTestHandler wires an AdminHandler against fresh in-memory
// server/users databases (real ServerSchema/UsersSchema) and wires the
// global dual DB, since most model methods used by AdminHandler read from
// database.GetServerDB()/GetUsersDB() rather than the injected field.
func newAdminTestHandler(t *testing.T) (*AdminHandler, *sql.DB, *sql.DB) {
	t.Helper()
	serverDB := newTestServerDB(t)
	usersDB := newTestUsersDB(t)
	setGlobalTestDualDB(t, serverDB, usersDB)
	// AdminHandler.DB backs UserModel/TokenModel/SettingsModel struct fields;
	// in production main.go wires this to dualDB.Users (see src/main.go:923).
	return &AdminHandler{DB: usersDB}, serverDB, usersDB
}

// TestAdminHandler_ListUsers covers the success path (users present) and
// the DB-error path (backing table dropped out from under the handler).
func TestAdminHandler_ListUsers(t *testing.T) {
	t.Run("success returns seeded users", func(t *testing.T) {
		h, _, usersDB := newAdminTestHandler(t)
		if _, err := (&models.UserModel{DB: usersDB}).Create("alice", "alice@example.com", "password123", "user"); err != nil {
			t.Fatalf("seed user: %v", err)
		}

		r, w := newTestContext(http.MethodGet, "/api/v1/server/admin/config/users")
		h.ListUsers(w, r)

		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
		}
		var users []map[string]interface{}
		if err := json.Unmarshal(w.Body.Bytes(), &users); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if len(users) != 1 || users[0]["username"] != "alice" {
			t.Fatalf("unexpected users payload: %+v", users)
		}
	})

	t.Run("db error returns 500", func(t *testing.T) {
		h, _, usersDB := newAdminTestHandler(t)
		if _, err := usersDB.Exec("DROP TABLE user_accounts"); err != nil {
			t.Fatalf("drop table: %v", err)
		}

		r, w := newTestContext(http.MethodGet, "/api/v1/server/admin/config/users")
		h.ListUsers(w, r)

		if w.Code != http.StatusInternalServerError {
			t.Fatalf("status = %d, want 500; body=%s", w.Code, w.Body.String())
		}
	})
}

// TestAdminHandler_CreateUser covers success, missing-field validation, and
// invalid-username validation (per src/util/username.go rules).
func TestAdminHandler_CreateUser(t *testing.T) {
	t.Run("success creates user", func(t *testing.T) {
		h, _, _ := newAdminTestHandler(t)
		body := map[string]interface{}{
			"username": "bobsmith",
			"email":    "bob@example.com",
			"password": "password123",
			"role":     "user",
		}
		r, w := newTestContextJSON(t, http.MethodPost, "/api/v1/server/admin/config/users", body)
		h.CreateUser(w, r)

		if w.Code != http.StatusCreated {
			t.Fatalf("status = %d, want 201; body=%s", w.Code, w.Body.String())
		}
	})

	t.Run("missing required fields returns 400", func(t *testing.T) {
		h, _, _ := newAdminTestHandler(t)
		r, w := newTestContextJSON(t, http.MethodPost, "/api/v1/server/admin/config/users", map[string]interface{}{})
		h.CreateUser(w, r)

		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400; body=%s", w.Code, w.Body.String())
		}
	})

	t.Run("invalid username returns 400", func(t *testing.T) {
		h, _, _ := newAdminTestHandler(t)
		body := map[string]interface{}{
			"username": "AB", // too short and uppercase - fails ValidateUsername
			"email":    "ab@example.com",
			"password": "password123",
			"role":     "user",
		}
		r, w := newTestContextJSON(t, http.MethodPost, "/api/v1/server/admin/config/users", body)
		h.CreateUser(w, r)

		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400; body=%s", w.Code, w.Body.String())
		}
	})
}

// setAdminCurrentUser mirrors setCurrentUser (locations_test.go) using the
// same middleware.UserContextKey convention AdminHandler.UpdateUser/DeleteUser
// read via middleware.GetCurrentUser.
func setAdminCurrentUser(r *http.Request, id int64) *http.Request {
	return withReqCtxValue(r, middleware.UserContextKey, &models.User{ID: id})
}

// TestAdminHandler_UpdateUser_SelfEditForbidden documents the intended
// "cannot edit your own account" guard.
func TestAdminHandler_UpdateUser_SelfEditForbidden(t *testing.T) {
	h, _, _ := newAdminTestHandler(t)
	body := map[string]interface{}{"username": "newname", "email": "new@example.com", "role": "user"}
	r, w := newTestContextJSON(t, http.MethodPut, "/api/v1/server/admin/config/users/5", body)
	r = setURLParam(r, "id", "5")
	r = setAdminCurrentUser(r, 5)

	h.UpdateUser(w, r)

	if w.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403; body=%s", w.Code, w.Body.String())
	}
}

// TestAdminHandler_UpdateUser_NoCurrentUser_PanicsInsteadOf401 is a
// regression test documenting a genuine production bug in
// src/server/handler/admin.go (UpdateUser, around the
// "currentUser, _ := middleware.GetCurrentUser(c)" line): the `ok` bool is
// discarded, so when no "user" key exists in the gin context (e.g. request
// reaches this handler without auth middleware attaching one),
// currentUser is nil and "currentUser.ID" panics with a nil pointer
// dereference instead of returning 401 Unauthorized.
//
// Correct behavior: the handler should return 401 Unauthorized. This test
// recovers the panic so it doesn't crash the test binary and reports the
// mismatch as a failure.
func TestAdminHandler_UpdateUser_NoCurrentUser_PanicsInsteadOf401(t *testing.T) {
	h, _, _ := newAdminTestHandler(t)
	body := map[string]interface{}{"username": "newname", "email": "new@example.com", "role": "user"}
	r, w := newTestContextJSON(t, http.MethodPut, "/api/v1/server/admin/config/users/5", body)
	r = setURLParam(r, "id", "5")
	// Deliberately do NOT set a current user in the request context.

	func() {
		defer func() {
			if rec := recover(); rec != nil {
				t.Errorf("BUG admin.go UpdateUser: handler panicked instead of returning 401 when no current user is set: %v", rec)
			}
		}()
		h.UpdateUser(w, r)
	}()

	if w.Code != 0 && w.Code != http.StatusUnauthorized {
		t.Errorf("BUG admin.go UpdateUser: status = %d, want 401 Unauthorized when unauthenticated", w.Code)
	}
}

// TestAdminHandler_DeleteUser_SelfDeleteForbidden documents the intended
// "cannot delete your own account" guard.
func TestAdminHandler_DeleteUser_SelfDeleteForbidden(t *testing.T) {
	h, _, _ := newAdminTestHandler(t)
	r, w := newTestContext(http.MethodDelete, "/api/v1/server/admin/config/users/7")
	r = setURLParam(r, "id", "7")
	r = setAdminCurrentUser(r, 7)

	h.DeleteUser(w, r)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body=%s", w.Code, w.Body.String())
	}
}

// TestAdminHandler_DeleteUser_InvalidID covers the non-numeric id edge case.
func TestAdminHandler_DeleteUser_InvalidID(t *testing.T) {
	h, _, _ := newAdminTestHandler(t)
	r, w := newTestContext(http.MethodDelete, "/api/v1/server/admin/config/users/abc")
	r = setURLParam(r, "id", "abc")

	h.DeleteUser(w, r)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body=%s", w.Code, w.Body.String())
	}
}

// TestAdminHandler_DeleteUser_NoCurrentUser_PanicsInsteadOf401 mirrors the
// UpdateUser regression above for DeleteUser (admin.go DeleteUser has the
// identical "currentUser, _ := middleware.GetCurrentUser(c)" bug).
func TestAdminHandler_DeleteUser_NoCurrentUser_PanicsInsteadOf401(t *testing.T) {
	h, _, _ := newAdminTestHandler(t)
	r, w := newTestContext(http.MethodDelete, "/api/v1/server/admin/config/users/7")
	r = setURLParam(r, "id", "7")
	// Deliberately do NOT set a current user in the request context.

	func() {
		defer func() {
			if rec := recover(); rec != nil {
				t.Errorf("BUG admin.go DeleteUser: handler panicked instead of returning 401 when no current user is set: %v", rec)
			}
		}()
		h.DeleteUser(w, r)
	}()

	if w.Code != 0 && w.Code != http.StatusUnauthorized {
		t.Errorf("BUG admin.go DeleteUser: status = %d, want 401 Unauthorized when unauthenticated", w.Code)
	}
}

// TestAdminHandler_UpdateUserPassword covers success and short-password
// validation.
func TestAdminHandler_UpdateUserPassword(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		h, _, usersDB := newAdminTestHandler(t)
		user, err := (&models.UserModel{DB: usersDB}).Create("carol", "carol@example.com", "password123", "user")
		if err != nil {
			t.Fatalf("seed user: %v", err)
		}

		r, w := newTestContextJSON(t, http.MethodPut, "/api/v1/server/admin/config/users/1/password", map[string]interface{}{"password": "newpassword123"})
		r = setURLParam(r, "id", itoa(int(user.ID)))
		h.UpdateUserPassword(w, r)

		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
		}
	})

	t.Run("too short password returns 400", func(t *testing.T) {
		h, _, _ := newAdminTestHandler(t)
		r, w := newTestContextJSON(t, http.MethodPut, "/api/v1/server/admin/config/users/1/password", map[string]interface{}{"password": "short"})
		r = setURLParam(r, "id", "1")
		h.UpdateUserPassword(w, r)

		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400; body=%s", w.Code, w.Body.String())
		}
	})
}

// TestAdminHandler_ListSettings_GetSetting_UpdateSetting exercises the
// server_config-backed settings endpoints end to end, since they correctly
// match the real ServerSchema (no schema-mismatch bugs found here).
func TestAdminHandler_ListSettings_GetSetting_UpdateSetting(t *testing.T) {
	h, serverDB, _ := newAdminTestHandler(t)
	if _, err := serverDB.Exec(`INSERT INTO server_config (key, value, type, description) VALUES (?, ?, ?, ?)`,
		"server.title", "Weather", "string", "site title"); err != nil {
		t.Fatalf("seed setting: %v", err)
	}

	t.Run("ListSettings success", func(t *testing.T) {
		r, w := newTestContext(http.MethodGet, "/api/v1/server/admin/config/settings")
		h.ListSettings(w, r)
		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
		}
		var settings []map[string]interface{}
		if err := json.Unmarshal(w.Body.Bytes(), &settings); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if len(settings) != 1 || settings[0]["key"] != "server.title" {
			t.Fatalf("unexpected settings payload: %+v", settings)
		}
	})

	t.Run("GetSetting found", func(t *testing.T) {
		r, w := newTestContext(http.MethodGet, "/api/v1/server/admin/config/settings/server.title")
		r = setURLParam(r, "key", "server.title")
		h.GetSetting(w, r)
		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
		}
	})

	t.Run("GetSetting not found returns 404", func(t *testing.T) {
		r, w := newTestContext(http.MethodGet, "/api/v1/server/admin/config/settings/does.not.exist")
		r = setURLParam(r, "key", "does.not.exist")
		h.GetSetting(w, r)
		if w.Code != http.StatusNotFound {
			t.Fatalf("status = %d, want 404; body=%s", w.Code, w.Body.String())
		}
	})

	t.Run("UpdateSetting success", func(t *testing.T) {
		r, w := newTestContextJSON(t, http.MethodPut, "/api/v1/server/admin/config/settings/server.title", map[string]interface{}{"value": "New Title"})
		r = setURLParam(r, "key", "server.title")
		h.UpdateSetting(w, r)
		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
		}
	})

	t.Run("UpdateSetting missing value returns 400", func(t *testing.T) {
		r, w := newTestContextJSON(t, http.MethodPut, "/api/v1/server/admin/config/settings/server.title", map[string]interface{}{})
		r = setURLParam(r, "key", "server.title")
		h.UpdateSetting(w, r)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400; body=%s", w.Code, w.Body.String())
		}
	})
}

// TestAdminHandler_GenerateToken_RevokeToken covers the user_tokens-backed
// token lifecycle (matches real UsersSchema).
func TestAdminHandler_GenerateToken_RevokeToken(t *testing.T) {
	h, _, usersDB := newAdminTestHandler(t)
	user, err := (&models.UserModel{DB: usersDB}).Create("dave", "dave@example.com", "password123", "user")
	if err != nil {
		t.Fatalf("seed user: %v", err)
	}

	var tokenID int
	t.Run("GenerateToken success", func(t *testing.T) {
		r, w := newTestContextJSON(t, http.MethodPost, "/api/v1/server/admin/config/security/tokens", map[string]interface{}{
			"user_id": int(user.ID),
			"name":    "test token",
		})
		h.GenerateToken(w, r)
		if w.Code != http.StatusCreated {
			t.Fatalf("status = %d, want 201; body=%s", w.Code, w.Body.String())
		}
		var resp map[string]interface{}
		if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		tok, ok := resp["token"].(map[string]interface{})
		if !ok {
			t.Fatalf("expected token object in response, got %+v", resp)
		}
		idFloat, ok := tok["id"].(float64)
		if !ok {
			t.Fatalf("expected numeric token id, got %+v", tok["id"])
		}
		tokenID = int(idFloat)
	})

	t.Run("GenerateToken missing fields returns 400", func(t *testing.T) {
		r, w := newTestContextJSON(t, http.MethodPost, "/api/v1/server/admin/config/security/tokens", map[string]interface{}{})
		h.GenerateToken(w, r)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400; body=%s", w.Code, w.Body.String())
		}
	})

	t.Run("ListTokens scoped by user_id success", func(t *testing.T) {
		r, w := newTestContext(http.MethodGet, "/api/v1/server/admin/config/security/tokens?user_id="+itoa(int(user.ID)))
		h.ListTokens(w, r)
		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
		}
	})

	t.Run("RevokeToken success", func(t *testing.T) {
		r, w := newTestContext(http.MethodDelete, "/api/v1/server/admin/config/security/tokens/"+itoa(tokenID))
		r = setURLParam(r, "id", itoa(tokenID))
		h.RevokeToken(w, r)
		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
		}
	})

	t.Run("RevokeToken invalid id returns 400", func(t *testing.T) {
		r, w := newTestContext(http.MethodDelete, "/api/v1/server/admin/config/security/tokens/xyz")
		r = setURLParam(r, "id", "xyz")
		h.RevokeToken(w, r)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400; body=%s", w.Code, w.Body.String())
		}
	})
}

// TestAdminHandler_ListTokens_AdminWideView_ReturnsTokensAcrossUsers covers
// the branch used when no ?user_id= query param is given: the admin-wide
// token view reads user_tokens LEFT JOIN user_accounts out of users.db, both
// of which are real tables in database.UsersSchema. Rows for every user must
// come back, joined to the owning account's email.
func TestAdminHandler_ListTokens_AdminWideView_ReturnsTokensAcrossUsers(t *testing.T) {
	h, _, usersDB := newAdminTestHandler(t)

	userModel := &models.UserModel{DB: usersDB}
	frank, err := userModel.Create("frank", "frank@example.com", "password123", "user")
	if err != nil {
		t.Fatalf("seed user frank: %v", err)
	}
	grace, err := userModel.Create("grace", "grace@example.com", "password123", "user")
	if err != nil {
		t.Fatalf("seed user grace: %v", err)
	}

	// user_tokens requires user_id, token_hash and token_prefix (all NOT NULL
	// in UsersSchema); created_at defaults to CURRENT_TIMESTAMP.
	for _, seed := range []struct {
		userID int64
		hash   string
		name   string
	}{
		{frank.ID, "hash-frank", "frank token"},
		{grace.ID, "hash-grace", "grace token"},
	} {
		if _, err := usersDB.Exec(`INSERT INTO user_tokens (user_id, token_hash, token_prefix, name) VALUES (?, ?, 'usr_', ?)`,
			seed.userID, seed.hash, seed.name); err != nil {
			t.Fatalf("seed token for user %d: %v", seed.userID, err)
		}
	}

	r, w := newTestContext(http.MethodGet, "/api/v1/server/admin/config/security/tokens")
	h.ListTokens(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
	}
	var tokens []map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &tokens); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(tokens) != 2 {
		t.Fatalf("len(tokens) = %d, want 2; body=%s", len(tokens), w.Body.String())
	}

	emailsByName := map[string]string{}
	for _, tok := range tokens {
		name, _ := tok["name"].(string)
		email, _ := tok["user_email"].(string)
		emailsByName[name] = email
		if _, ok := tok["created_at"]; !ok {
			t.Errorf("token %q has no created_at in the payload; body=%s", name, w.Body.String())
		}
	}
	if emailsByName["frank token"] != "frank@example.com" {
		t.Errorf("frank token email = %q, want frank@example.com", emailsByName["frank token"])
	}
	if emailsByName["grace token"] != "grace@example.com" {
		t.Errorf("grace token email = %q, want grace@example.com", emailsByName["grace token"])
	}
}

// TestAdminHandler_ListAuditLogs_QueriesNonexistentColumns is a regression
// test documenting a genuine production bug in
// src/server/handler/admin.go ListAuditLogs: the query selects
// a.user_id, a.resource, a.created_at from server_audit_log, but the real
// ServerSchema's server_audit_log table (src/database/server_schema.go) has
// no such columns - the actual columns are actor_id, resource_type,
// resource_id, and timestamp. This endpoint always returns 500 against the
// real schema instead of listing audit log entries.
func TestAdminHandler_ListAuditLogs_QueriesNonexistentColumns(t *testing.T) {
	h, _, _ := newAdminTestHandler(t)
	r, w := newTestContext(http.MethodGet, "/api/v1/server/admin/config/logs/audit-logs")
	h.ListAuditLogs(w, r)

	if w.Code != http.StatusOK {
		t.Errorf("BUG admin.go ListAuditLogs: status = %d, want 200 (query references nonexistent server_audit_log columns user_id/resource/created_at); body=%s", w.Code, w.Body.String())
	}
}

// TestAdminHandler_ClearAuditLogs_QueriesNonexistentColumn documents the
// same root-cause bug as ListAuditLogs applied to ClearAuditLogs: it
// DELETEs "WHERE created_at < ?" but server_audit_log's real time column is
// "timestamp", so this always 500s instead of clearing old entries.
func TestAdminHandler_ClearAuditLogs_QueriesNonexistentColumn(t *testing.T) {
	h, _, _ := newAdminTestHandler(t)
	r, w := newTestContext(http.MethodDelete, "/api/v1/server/admin/config/logs/audit-logs")
	h.ClearAuditLogs(w, r)

	if w.Code != http.StatusOK {
		t.Errorf("BUG admin.go ClearAuditLogs: status = %d, want 200 (DELETE references nonexistent server_audit_log.created_at column, real column is 'timestamp'); body=%s", w.Code, w.Body.String())
	}
}

// TestAdminHandler_GetLogsStats_CountsRecentRowsByInstant covers GetLogsStats
// against the real server_audit_log columns (status/timestamp). The
// recent_24h figure used to be produced by
// "WHERE timestamp >= datetime('now', '-1 day')", which returns NULL for a row
// stored in the driver's local-zone time.Time.String() layout: the zone-skewed
// row seeded below was therefore never counted, and the row whose skewed text
// reads as recent but whose true instant is four days old would be counted by
// any text comparison. Both expectations fail against that implementation, on
// any host timezone.
func TestAdminHandler_GetLogsStats_CountsRecentRowsByInstant(t *testing.T) {
	h, serverDB, _ := newAdminTestHandler(t)
	now := time.Now()

	seeds := []struct {
		ulid      string
		status    string
		timestamp string
	}{
		// Canonical UTC text, minutes old: recent.
		{"ulid-utc-recent", "success", dbtime.FormatSQLTimestamp(now.Add(-time.Minute))},
		// Canonical UTC text, well outside the window: not recent.
		{"ulid-utc-old", "error", dbtime.FormatSQLTimestamp(now.Add(-48 * time.Hour))},
		// Truly minutes old, but its wall-clock text reads eleven hours in the
		// past: still recent.
		{"ulid-west-recent", "success", now.Add(-time.Minute).In(handlerZoneWest).Format(handlerLocalLayout)},
		// Truly four days old, but its wall-clock text reads thirteen hours
		// later than the instant: still not recent.
		{"ulid-east-old", "error", now.Add(-96 * time.Hour).In(handlerZoneEast).Format(handlerLocalLayout)},
		// Uninterpretable: never counted as recent.
		{"ulid-unparseable", "success", "not-a-timestamp"},
	}
	for _, seed := range seeds {
		if _, err := serverDB.Exec(`INSERT INTO server_audit_log (ulid, timestamp, actor_type, actor_id, action, status)
			VALUES (?, ?, 'admin', 1, 'test.action', ?)`, seed.ulid, seed.timestamp, seed.status); err != nil {
			t.Fatalf("seed audit log %s: %v", seed.ulid, err)
		}
	}

	r, w := newTestContext(http.MethodGet, "/api/v1/server/admin/config/logs/audit-logs/stats")
	h.GetLogsStats(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
	}
	var resp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if total, _ := resp["total"].(float64); total != 5 {
		t.Fatalf("total = %v, want 5", resp["total"])
	}
	if errors, _ := resp["errors"].(float64); errors != 2 {
		t.Errorf("errors = %v, want 2", resp["errors"])
	}
	if success, _ := resp["success"].(float64); success != 3 {
		t.Errorf("success = %v, want 3", resp["success"])
	}
	if recent, _ := resp["recent_24h"].(float64); recent != 2 {
		t.Errorf("recent_24h = %v, want 2 (the canonical and the zone-skewed row from minutes ago)", resp["recent_24h"])
	}
}

// TestAdminHandler_GetTasksStats_Success covers the scheduler stats
// endpoint, whose queries correctly reference real server_scheduler_state
// columns (enabled, last_status).
func TestAdminHandler_GetTasksStats_Success(t *testing.T) {
	h, serverDB, _ := newAdminTestHandler(t)
	if _, err := serverDB.Exec(`INSERT INTO server_scheduler_state (task_id, task_name, schedule, enabled, last_status) VALUES ('t1', 'rotate-logs', 'daily', 1, 'ok')`); err != nil {
		t.Fatalf("seed scheduler state: %v", err)
	}

	r, w := newTestContext(http.MethodGet, "/api/v1/server/admin/config/scheduler/stats")
	h.GetTasksStats(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
	}
	var resp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if total, _ := resp["total"].(float64); total != 1 {
		t.Fatalf("total = %v, want 1", resp["total"])
	}
}

// TestAdminHandler_GetSystemStats_Success exercises the dual-DB (server +
// users) system stats aggregation.
func TestAdminHandler_GetSystemStats_Success(t *testing.T) {
	h, _, usersDB := newAdminTestHandler(t)
	if _, err := (&models.UserModel{DB: usersDB}).Create("erin", "erin@example.com", "password123", "user"); err != nil {
		t.Fatalf("seed user: %v", err)
	}

	r, w := newTestContext(http.MethodGet, "/api/v1/server/admin/config/stats")
	h.GetSystemStats(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
	}
}

// TestAdminHandler_GetScheduledTasks_AlwaysEmpty_TableNameMismatch is a
// regression test documenting a genuine production bug in
// src/server/handler/admin.go GetScheduledTasks: it gates on
// "SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND
// name='scheduled_tasks'" to decide whether to seed/query scheduler state,
// but the real table used everywhere else in this file is
// "server_scheduler_state", not "scheduled_tasks". Since a table literally
// named "scheduled_tasks" never exists, this check always evaluates to
// tableExists == 0 and the handler always returns an empty array - even
// when server_scheduler_state has rows (seeded by the internal scheduler
// per PART 19). Admins can never see real scheduled task status through
// this endpoint.
func TestAdminHandler_GetScheduledTasks_AlwaysEmpty_TableNameMismatch(t *testing.T) {
	h, serverDB, _ := newAdminTestHandler(t)
	if _, err := serverDB.Exec(`INSERT INTO server_scheduler_state (task_id, task_name, schedule, enabled) VALUES ('t1', 'rotate-logs', 'daily', 1)`); err != nil {
		t.Fatalf("seed scheduler state: %v", err)
	}

	r, w := newTestContext(http.MethodGet, "/api/v1/server/admin/config/scheduler")
	h.GetScheduledTasks(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
	}
	var tasks []map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &tasks); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(tasks) == 0 {
		t.Errorf("BUG admin.go GetScheduledTasks: returned empty task list despite a seeded server_scheduler_state row (handler checks for a nonexistent 'scheduled_tasks' table instead of 'server_scheduler_state', so it always short-circuits to an empty response)")
	}
}

// TestAdminHandler_ShowSettingsPage_RedirectBranches covers the three
// pre-render redirect/error paths (missing admin_id, wrong-typed admin_id,
// admin lookup failure). The success path renders an HTML template and is
// intentionally not exercised here (see coverage gaps in final report).
func TestAdminHandler_ShowSettingsPage_RedirectBranches(t *testing.T) {
	t.Run("missing admin_id redirects", func(t *testing.T) {
		h, _, _ := newAdminTestHandler(t)
		r, w := newTestContext(http.MethodGet, "/server/admin/config/settings")
		h.ShowSettingsPage(w, r)
		if w.Code != http.StatusFound {
			t.Fatalf("status = %d, want 302; body=%s", w.Code, w.Body.String())
		}
	})

	t.Run("non-int admin_id redirects", func(t *testing.T) {
		h, _, _ := newAdminTestHandler(t)
		r, w := newTestContext(http.MethodGet, "/server/admin/config/settings")
		r = withReqCtxValue(r, "admin_id", "not-an-int")
		h.ShowSettingsPage(w, r)
		if w.Code != http.StatusFound {
			t.Fatalf("status = %d, want 302; body=%s", w.Code, w.Body.String())
		}
	})

	t.Run("admin not found redirects", func(t *testing.T) {
		h, _, _ := newAdminTestHandler(t)
		r, w := newTestContext(http.MethodGet, "/server/admin/config/settings")
		r = withReqCtxValue(r, "admin_id", 999)
		h.ShowSettingsPage(w, r)
		if w.Code != http.StatusFound {
			t.Fatalf("status = %d, want 302; body=%s", w.Code, w.Body.String())
		}
	})
}
