package handler

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/webappsgo/wthr/src/server/middleware"
	models "github.com/webappsgo/wthr/src/server/model"
)

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

		c, w := newTestContext(http.MethodGet, "/api/v1/server/admin/users")
		h.ListUsers(c)

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

		c, w := newTestContext(http.MethodGet, "/api/v1/server/admin/users")
		h.ListUsers(c)

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
		c, w := newTestContextJSON(t, http.MethodPost, "/api/v1/server/admin/users", body)
		h.CreateUser(c)

		if w.Code != http.StatusCreated {
			t.Fatalf("status = %d, want 201; body=%s", w.Code, w.Body.String())
		}
	})

	t.Run("missing required fields returns 400", func(t *testing.T) {
		h, _, _ := newAdminTestHandler(t)
		c, w := newTestContextJSON(t, http.MethodPost, "/api/v1/server/admin/users", map[string]interface{}{})
		h.CreateUser(c)

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
		c, w := newTestContextJSON(t, http.MethodPost, "/api/v1/server/admin/users", body)
		h.CreateUser(c)

		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400; body=%s", w.Code, w.Body.String())
		}
	})
}

// setAdminCurrentUser mirrors setCurrentUser (locations_test.go) using the
// same middleware.UserContextKey convention AdminHandler.UpdateUser/DeleteUser
// read via middleware.GetCurrentUser.
func setAdminCurrentUser(c *gin.Context, id int64) {
	c.Set(middleware.UserContextKey, &models.User{ID: id})
}

// TestAdminHandler_UpdateUser_SelfEditForbidden documents the intended
// "cannot edit your own account" guard.
func TestAdminHandler_UpdateUser_SelfEditForbidden(t *testing.T) {
	h, _, _ := newAdminTestHandler(t)
	body := map[string]interface{}{"username": "newname", "email": "new@example.com", "role": "user"}
	c, w := newTestContextJSON(t, http.MethodPut, "/api/v1/server/admin/users/5", body)
	c.Params = gin.Params{{Key: "id", Value: "5"}}
	setAdminCurrentUser(c, 5)

	h.UpdateUser(c)

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
	c, w := newTestContextJSON(t, http.MethodPut, "/api/v1/server/admin/users/5", body)
	c.Params = gin.Params{{Key: "id", Value: "5"}}
	// Deliberately do NOT set a current user in the gin context.

	func() {
		defer func() {
			if r := recover(); r != nil {
				t.Errorf("BUG admin.go UpdateUser: handler panicked instead of returning 401 when no current user is set: %v", r)
			}
		}()
		h.UpdateUser(c)
	}()

	if w.Code != 0 && w.Code != http.StatusUnauthorized {
		t.Errorf("BUG admin.go UpdateUser: status = %d, want 401 Unauthorized when unauthenticated", w.Code)
	}
}

// TestAdminHandler_DeleteUser_SelfDeleteForbidden documents the intended
// "cannot delete your own account" guard.
func TestAdminHandler_DeleteUser_SelfDeleteForbidden(t *testing.T) {
	h, _, _ := newAdminTestHandler(t)
	c, w := newTestContext(http.MethodDelete, "/api/v1/server/admin/users/7")
	c.Params = gin.Params{{Key: "id", Value: "7"}}
	setAdminCurrentUser(c, 7)

	h.DeleteUser(c)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body=%s", w.Code, w.Body.String())
	}
}

// TestAdminHandler_DeleteUser_InvalidID covers the non-numeric id edge case.
func TestAdminHandler_DeleteUser_InvalidID(t *testing.T) {
	h, _, _ := newAdminTestHandler(t)
	c, w := newTestContext(http.MethodDelete, "/api/v1/server/admin/users/abc")
	c.Params = gin.Params{{Key: "id", Value: "abc"}}

	h.DeleteUser(c)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body=%s", w.Code, w.Body.String())
	}
}

// TestAdminHandler_DeleteUser_NoCurrentUser_PanicsInsteadOf401 mirrors the
// UpdateUser regression above for DeleteUser (admin.go DeleteUser has the
// identical "currentUser, _ := middleware.GetCurrentUser(c)" bug).
func TestAdminHandler_DeleteUser_NoCurrentUser_PanicsInsteadOf401(t *testing.T) {
	h, _, _ := newAdminTestHandler(t)
	c, w := newTestContext(http.MethodDelete, "/api/v1/server/admin/users/7")
	c.Params = gin.Params{{Key: "id", Value: "7"}}
	// Deliberately do NOT set a current user in the gin context.

	func() {
		defer func() {
			if r := recover(); r != nil {
				t.Errorf("BUG admin.go DeleteUser: handler panicked instead of returning 401 when no current user is set: %v", r)
			}
		}()
		h.DeleteUser(c)
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

		c, w := newTestContextJSON(t, http.MethodPut, "/api/v1/server/admin/users/1/password", map[string]interface{}{"password": "newpassword123"})
		c.Params = gin.Params{{Key: "id", Value: itoa(int(user.ID))}}
		h.UpdateUserPassword(c)

		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
		}
	})

	t.Run("too short password returns 400", func(t *testing.T) {
		h, _, _ := newAdminTestHandler(t)
		c, w := newTestContextJSON(t, http.MethodPut, "/api/v1/server/admin/users/1/password", map[string]interface{}{"password": "short"})
		c.Params = gin.Params{{Key: "id", Value: "1"}}
		h.UpdateUserPassword(c)

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
		c, w := newTestContext(http.MethodGet, "/api/v1/server/admin/settings")
		h.ListSettings(c)
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
		c, w := newTestContext(http.MethodGet, "/api/v1/server/admin/settings/server.title")
		c.Params = gin.Params{{Key: "key", Value: "server.title"}}
		h.GetSetting(c)
		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
		}
	})

	t.Run("GetSetting not found returns 404", func(t *testing.T) {
		c, w := newTestContext(http.MethodGet, "/api/v1/server/admin/settings/does.not.exist")
		c.Params = gin.Params{{Key: "key", Value: "does.not.exist"}}
		h.GetSetting(c)
		if w.Code != http.StatusNotFound {
			t.Fatalf("status = %d, want 404; body=%s", w.Code, w.Body.String())
		}
	})

	t.Run("UpdateSetting success", func(t *testing.T) {
		c, w := newTestContextJSON(t, http.MethodPut, "/api/v1/server/admin/settings/server.title", map[string]interface{}{"value": "New Title"})
		c.Params = gin.Params{{Key: "key", Value: "server.title"}}
		h.UpdateSetting(c)
		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
		}
	})

	t.Run("UpdateSetting missing value returns 400", func(t *testing.T) {
		c, w := newTestContextJSON(t, http.MethodPut, "/api/v1/server/admin/settings/server.title", map[string]interface{}{})
		c.Params = gin.Params{{Key: "key", Value: "server.title"}}
		h.UpdateSetting(c)
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
		c, w := newTestContextJSON(t, http.MethodPost, "/api/v1/server/admin/tokens", map[string]interface{}{
			"user_id": int(user.ID),
			"name":    "test token",
		})
		h.GenerateToken(c)
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
		c, w := newTestContextJSON(t, http.MethodPost, "/api/v1/server/admin/tokens", map[string]interface{}{})
		h.GenerateToken(c)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400; body=%s", w.Code, w.Body.String())
		}
	})

	t.Run("ListTokens scoped by user_id success", func(t *testing.T) {
		c, w := newTestContext(http.MethodGet, "/api/v1/server/admin/tokens?user_id="+itoa(int(user.ID)))
		h.ListTokens(c)
		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
		}
	})

	t.Run("RevokeToken success", func(t *testing.T) {
		c, w := newTestContext(http.MethodDelete, "/api/v1/server/admin/tokens/"+itoa(tokenID))
		c.Params = gin.Params{{Key: "id", Value: itoa(tokenID)}}
		h.RevokeToken(c)
		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
		}
	})

	t.Run("RevokeToken invalid id returns 400", func(t *testing.T) {
		c, w := newTestContext(http.MethodDelete, "/api/v1/server/admin/tokens/xyz")
		c.Params = gin.Params{{Key: "id", Value: "xyz"}}
		h.RevokeToken(c)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400; body=%s", w.Code, w.Body.String())
		}
	})
}

// TestAdminHandler_ListTokens_AdminWideView_QueriesNonexistentTables is a
// regression test documenting a genuine production bug in
// src/server/handler/admin.go ListTokens: the branch used when no
// ?user_id= query param is given (the admin-wide token view) queries
// "FROM tokens t LEFT JOIN users u ..." against database.GetServerDB().
// Neither a "tokens" table nor a "users" table exists in the real
// ServerSchema (src/database/server_schema.go) - those only existed in the
// old, unused legacy schema (src/database/schema.go). In production this
// endpoint always returns 500 instead of an (empty or populated) token
// list.
func TestAdminHandler_ListTokens_AdminWideView_QueriesNonexistentTables(t *testing.T) {
	h, _, _ := newAdminTestHandler(t)
	c, w := newTestContext(http.MethodGet, "/api/v1/server/admin/tokens")
	h.ListTokens(c)

	if w.Code != http.StatusOK {
		t.Errorf("BUG admin.go ListTokens: status = %d, want 200 (admin-wide token view queries nonexistent 'tokens'/'users' tables against the real ServerSchema); body=%s", w.Code, w.Body.String())
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
	c, w := newTestContext(http.MethodGet, "/api/v1/server/admin/audit-logs")
	h.ListAuditLogs(c)

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
	c, w := newTestContext(http.MethodDelete, "/api/v1/server/admin/audit-logs")
	h.ClearAuditLogs(c)

	if w.Code != http.StatusOK {
		t.Errorf("BUG admin.go ClearAuditLogs: status = %d, want 200 (DELETE references nonexistent server_audit_log.created_at column, real column is 'timestamp'); body=%s", w.Code, w.Body.String())
	}
}

// TestAdminHandler_GetLogsStats_SwallowsColumnMismatchErrors documents a
// second class of bug in src/server/handler/admin.go GetLogsStats: three of
// its four QueryRow(...).Scan(...) calls reference nonexistent
// server_audit_log columns ("success", "created_at" - real columns are
// "status" and "timestamp"), but the returned *error* from Scan is
// discarded (".Scan(&errorLogs)" with no err check), so instead of
// surfacing a 500 the handler silently returns 200 with errors/success/
// recent_24h always 0, regardless of actual audit log contents. This is a
// "DB error swallowed, returns 200 with wrong data" bug.
func TestAdminHandler_GetLogsStats_SwallowsColumnMismatchErrors(t *testing.T) {
	h, serverDB, _ := newAdminTestHandler(t)
	// Seed 2 real audit log rows so recent_24h should be > 0 if the query
	// worked against the real schema.
	for i := 0; i < 2; i++ {
		if _, err := serverDB.Exec(`INSERT INTO server_audit_log (ulid, timestamp, actor_type, actor_id, action, status)
			VALUES (?, CURRENT_TIMESTAMP, 'admin', 1, 'test.action', 'success')`, "ulid-"+itoa(i)); err != nil {
			t.Fatalf("seed audit log: %v", err)
		}
	}

	c, w := newTestContext(http.MethodGet, "/api/v1/server/admin/audit-logs/stats")
	h.GetLogsStats(c)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
	}
	var resp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if total, _ := resp["total"].(float64); total != 2 {
		t.Fatalf("total = %v, want 2", resp["total"])
	}
	if recent, _ := resp["recent_24h"].(float64); recent == 0 {
		t.Errorf("BUG admin.go GetLogsStats: recent_24h = 0, want > 0 for rows inserted moments ago (query references nonexistent column server_audit_log.created_at, real column is 'timestamp', and the Scan error is silently discarded)")
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

	c, w := newTestContext(http.MethodGet, "/api/v1/server/admin/tasks/stats")
	h.GetTasksStats(c)

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

	c, w := newTestContext(http.MethodGet, "/api/v1/server/admin/stats")
	h.GetSystemStats(c)

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

	c, w := newTestContext(http.MethodGet, "/api/v1/server/admin/tasks")
	h.GetScheduledTasks(c)

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
		c, w := newTestContext(http.MethodGet, "/server/admin/settings")
		h.ShowSettingsPage(c)
		if w.Code != http.StatusFound {
			t.Fatalf("status = %d, want 302; body=%s", w.Code, w.Body.String())
		}
	})

	t.Run("non-int admin_id redirects", func(t *testing.T) {
		h, _, _ := newAdminTestHandler(t)
		c, w := newTestContext(http.MethodGet, "/server/admin/settings")
		c.Set("admin_id", "not-an-int")
		h.ShowSettingsPage(c)
		if w.Code != http.StatusFound {
			t.Fatalf("status = %d, want 302; body=%s", w.Code, w.Body.String())
		}
	})

	t.Run("admin not found redirects", func(t *testing.T) {
		h, _, _ := newAdminTestHandler(t)
		c, w := newTestContext(http.MethodGet, "/server/admin/settings")
		c.Set("admin_id", 999)
		h.ShowSettingsPage(c)
		if w.Code != http.StatusFound {
			t.Fatalf("status = %d, want 302; body=%s", w.Code, w.Body.String())
		}
	})
}
