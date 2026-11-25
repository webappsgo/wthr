package handler

import (
	"context"
	"database/sql"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/webappsgo/wthr/src/database"
	"github.com/webappsgo/wthr/src/server/middleware"
	"github.com/webappsgo/wthr/src/server/model"
)

func seedModerationUsers(t *testing.T, db *sql.DB) {
	t.Helper()
	_, err := db.Exec(`INSERT INTO users (username, email, password_hash, display_name, is_active, is_banned) VALUES
		('alice', 'alice@example.test', 'hash', 'Alice Alpha', 1, 0),
		('bob', 'bob@example.test', 'hash', 'Bob Beta', 0, 0),
		('carol', 'carol@example.test', 'hash', 'Carol Gamma', 1, 1)`)
	if err != nil {
		t.Fatal(err)
	}
}

func moderationRouteRequest(method, target, id string) *http.Request {
	r := httptest.NewRequest(method, target, nil)
	if id != "" {
		rctx := chi.NewRouteContext()
		rctx.URLParams.Add("id", id)
		r = r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, rctx))
	}
	return r
}

func TestShowAdminModerationPageFiltersUsersServerSide(t *testing.T) {
	_, _, usersDB := newAdminTestHandler(t)
	seedModerationUsers(t, usersDB)
	previous := middleware.RenderHTML
	var rows []*model.User
	middleware.RenderHTML = func(w http.ResponseWriter, _ *http.Request, status int, page string, data map[string]interface{}) {
		w.WriteHeader(status)
		if page != "admin/admin_moderation.tmpl" {
			t.Errorf("template = %q", page)
		}
		rows, _ = data["users"].([]*model.User)
	}
	t.Cleanup(func() { middleware.RenderHTML = previous })

	ShowAdminModerationPage(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/server/administration/moderation/users?q=ALPHA", nil))
	if len(rows) != 1 || rows[0].Username != "alice" {
		t.Fatalf("search results = %#v", rows)
	}
	ShowAdminModerationPage(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/server/administration/moderation/users?status=active", nil))
	if len(rows) != 1 || rows[0].Username != "alice" {
		t.Fatalf("active results = %#v", rows)
	}
	ShowAdminModerationPage(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/server/administration/moderation/users?status=disabled", nil))
	if len(rows) != 2 || rows[0].Username == rows[1].Username || (rows[0].Username != "carol" && rows[1].Username != "carol") || (rows[0].Username != "bob" && rows[1].Username != "bob") {
		t.Fatalf("disabled results = %#v", rows)
	}
}

func TestAdminModerationUserActionUpdatesAndRejectsUnknownActions(t *testing.T) {
	_, _, usersDB := newAdminTestHandler(t)
	seedModerationUsers(t, usersDB)
	for _, test := range []struct {
		action string
		active bool
		status int
	}{
		{action: "disable", active: false, status: http.StatusSeeOther},
		{action: "enable", active: true, status: http.StatusSeeOther},
		{action: "revoke_sessions", active: true, status: http.StatusSeeOther},
		{action: "unknown", active: true, status: http.StatusSeeOther},
	} {
		values := url.Values{"action": {test.action}}
		r := adminFormRequest("/server/administration/moderation/users/1", values)
		rctx := chi.NewRouteContext()
		rctx.URLParams.Add("id", "1")
		r = r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, rctx))
		w := httptest.NewRecorder()
		AdminModerationUserAction(w, r)
		if w.Code != test.status {
			t.Fatalf("action %q status = %d", test.action, w.Code)
		}
		user, err := (&model.UserModel{Store: storeForDB(usersDB)}).GetByID(1)
		if err != nil {
			t.Fatal(err)
		}
		if user.IsActive != test.active {
			t.Fatalf("action %q active state = %t, want %t", test.action, user.IsActive, test.active)
		}
	}
}

func TestShowAdminModerationUserPageHandlesInvalidAndValidUsers(t *testing.T) {
	_, _, usersDB := newAdminTestHandler(t)
	seedModerationUsers(t, usersDB)
	previous := middleware.RenderHTML
	middleware.RenderHTML = func(w http.ResponseWriter, _ *http.Request, status int, page string, data map[string]interface{}) {
		w.WriteHeader(status)
		if page != "admin/admin_user_detail.tmpl" {
			t.Errorf("template = %q", page)
		}
		if user, ok := data["user"].(*model.User); !ok || user.ID != 1 {
			t.Errorf("rendered user = %#v", data["user"])
		}
	}
	t.Cleanup(func() { middleware.RenderHTML = previous })
	for _, id := range []string{"bad", "99"} {
		w := httptest.NewRecorder()
		ShowAdminModerationUserPage(w, moderationRouteRequest(http.MethodGet, "/users/"+id, id))
		if w.Code != http.StatusNotFound {
			t.Errorf("invalid user %q status = %d", id, w.Code)
		}
	}
	w := httptest.NewRecorder()
	ShowAdminModerationUserPage(w, moderationRouteRequest(http.MethodGet, "/users/1", "1"))
	if w.Code != http.StatusOK {
		t.Fatalf("valid user status = %d", w.Code)
	}
}

func TestAdminModerationImpersonationRequiresValidRequest(t *testing.T) {
	newAdminTestHandler(t)
	r := moderationRouteRequest(http.MethodPost, "/users/1/impersonate", "1")
	w := httptest.NewRecorder()
	AdminModerationImpersonateUser(w, r)
	if w.Code != http.StatusForbidden {
		t.Fatalf("missing CSRF status = %d", w.Code)
	}
}

func TestImpersonationSessionHelpersAndAudit(t *testing.T) {
	for _, test := range []struct {
		name  string
		value interface{}
		want  int64
		valid bool
	}{
		{name: "positive int", value: int(4), want: 4, valid: true},
		{name: "positive int64", value: int64(5), want: 5, valid: true},
		{name: "integral float", value: float64(6), want: 6, valid: true},
		{name: "fractional float", value: float64(6.5), want: 6},
		{name: "zero", value: 0},
		{name: "unsupported", value: "7"},
	} {
		got, valid := numericSessionValue(test.value)
		if got != test.want || valid != test.valid {
			t.Errorf("numericSessionValue(%v) = %d, %t; want %d, %t", test.value, got, valid, test.want, test.valid)
		}
	}
	if isAdminImpersonationSession(nil) || isAdminImpersonationSession(&model.Session{}) || isAdminImpersonationSession(&model.Session{Data: map[string]interface{}{"admin_impersonation": false}}) {
		t.Fatal("unmarked session was considered an admin impersonation")
	}
	if !isAdminImpersonationSession(&model.Session{Data: map[string]interface{}{"admin_impersonation": true}}) {
		t.Fatal("marked session was not recognized as an admin impersonation")
	}

	newAdminTestHandler(t)
	r := httptest.NewRequest(http.MethodPost, "/server/administration/moderation/users/1/impersonate", nil)
	if err := writeImpersonationAudit(r, 11, 22, "start", "success"); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := database.GetServerDB().QueryRow(`SELECT COUNT(*) FROM audit_log WHERE actor_id = '11' AND resource_id = '22' AND action = 'admin.user.impersonate'`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("impersonation audit count = %d", count)
	}
}
