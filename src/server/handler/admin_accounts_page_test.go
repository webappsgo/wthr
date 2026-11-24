package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/webappsgo/wthr/src/server/middleware"
	"github.com/webappsgo/wthr/src/server/reqctx"
)

func TestShowServerAdminsPageRendersOnlyCurrentAdmin(t *testing.T) {
	_, _, usersDB := newAdminTestHandler(t)
	result, err := usersDB.Exec(`INSERT INTO admins (username, email, password_hash, is_super_admin) VALUES ('alice', 'alice@example.test', 'hash', 1)`)
	if err != nil {
		t.Fatal(err)
	}
	id, err := result.LastInsertId()
	if err != nil {
		t.Fatal(err)
	}
	previous := middleware.RenderHTML
	middleware.RenderHTML = func(w http.ResponseWriter, _ *http.Request, status int, page string, data map[string]interface{}) {
		w.WriteHeader(status)
		current, _ := data["current_admin"].(struct{ Username, Email string })
		if page != "admin/admin_admins.tmpl" || data["admin_count"] != 1 || current.Username != "alice" || current.Email != "alice@example.test" {
			t.Errorf("admin page data = %#v, page=%q", data, page)
		}
	}
	t.Cleanup(func() { middleware.RenderHTML = previous })
	r := httptest.NewRequest(http.MethodGet, "/server/administration/admins", nil)
	r = r.WithContext(reqctx.SetValue(r.Context(), "admin_id", id))
	w := httptest.NewRecorder()
	ShowServerAdminsPage(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d", w.Code)
	}
}

func TestShowAdminDetailPageRestrictsLegacyNumericRoute(t *testing.T) {
	for _, test := range []struct {
		name      string
		id        string
		adminID   int64
		username  string
		wantCode  int
		wantRoute string
	}{
		{name: "different admin", id: "2", adminID: 1, username: "alice", wantCode: http.StatusNotFound},
		{name: "malformed id", id: "bad", adminID: 1, username: "alice", wantCode: http.StatusNotFound},
		{name: "missing username", id: "1", adminID: 1, wantCode: http.StatusNotFound},
		{name: "self redirects to profile", id: "1", adminID: 1, username: "alice", wantCode: http.StatusSeeOther, wantRoute: "/server/administration/alice/profile"},
	} {
		t.Run(test.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodGet, "/server/administration/admins/"+test.id, nil)
			rctx := chi.NewRouteContext()
			rctx.URLParams.Add("id", test.id)
			ctx := reqctx.SetValue(r.Context(), "admin_id", test.adminID)
			if test.username != "" {
				ctx = reqctx.SetValue(ctx, adminUsernameKey, test.username)
			}
			r = r.WithContext(context.WithValue(ctx, chi.RouteCtxKey, rctx))
			w := httptest.NewRecorder()
			ShowAdminDetailPage(w, r)
			if w.Code != test.wantCode {
				t.Fatalf("status = %d, want %d", w.Code, test.wantCode)
			}
			if test.wantRoute != "" && w.Header().Get("Location") != test.wantRoute {
				t.Fatalf("Location = %q, want %q", w.Header().Get("Location"), test.wantRoute)
			}
		})
	}
}
