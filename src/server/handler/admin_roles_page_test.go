package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/webappsgo/wthr/src/config"
	"github.com/webappsgo/wthr/src/server/middleware"
)

func roleFormRequest(target string, values url.Values, role string) *http.Request {
	r := adminFormRequest(target, values)
	if role != "" {
		ctx := chi.NewRouteContext()
		ctx.URLParams.Add("name", role)
		r = r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, ctx))
	}
	return r
}

func TestAdminRoleFormLifecycle(t *testing.T) {
	newAdminTestHandler(t)
	write := func(role string, values url.Values, handler http.HandlerFunc) {
		t.Helper()
		w := httptest.NewRecorder()
		handler(w, roleFormRequest("/server/administration/roles", values, role))
		assertAdminFormRedirect(t, w, "/server/administration/roles")
	}
	read := func() (map[string]interface{}, bool) {
		t.Helper()
		value, exists, err := config.ReadSetting("server.users.roles")
		if err != nil {
			t.Fatal(err)
		}
		settings, _ := value.(map[string]interface{})
		return settings, exists
	}

	write("", url.Values{"name": {" Editor "}, "permissions": {"read", "write"}}, CreateAdminRoleForm)
	settings, exists := read()
	if !exists || !containsRolePermission(stringSlice(settings["available"]), "editor") {
		t.Fatalf("custom role was not created: %#v", settings)
	}
	write("editor", url.Values{"permissions": {"moderate"}}, UpdateAdminRolePermissionsForm)
	settings, _ = read()
	permissions, _ := settings["permissions"].(map[string]interface{})
	if got := stringSlice(permissions["editor"]); len(got) != 1 || got[0] != "moderate" {
		t.Fatalf("updated permissions = %#v", got)
	}
	write("", url.Values{"default_role": {"editor"}}, SetDefaultAdminRoleForm)
	settings, _ = read()
	if settings["default"] != "editor" {
		t.Fatalf("default role = %#v, want editor", settings["default"])
	}
	write("", url.Values{"default_role": {"user"}}, SetDefaultAdminRoleForm)
	write("editor", url.Values{"confirm_delete": {"yes"}}, DeleteAdminRoleForm)
	settings, _ = read()
	if containsRolePermission(stringSlice(settings["available"]), "editor") {
		t.Fatalf("custom role remained after deletion: %#v", settings)
	}
}

func TestAdminRoleFormsRejectInvalidAndProtectedRoles(t *testing.T) {
	newAdminTestHandler(t)
	if err := config.WriteSetting("server.users.roles", map[string]interface{}{
		"available": []string{"admin", "user", "reviewer"}, "default": "user",
		"permissions": map[string]interface{}{"reviewer": []string{"read"}},
	}); err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name    string
		role    string
		values  url.Values
		handler http.HandlerFunc
	}{
		{"invalid name", "", url.Values{"name": {"Bad Role"}}, CreateAdminRoleForm},
		{"duplicate role", "", url.Values{"name": {"user"}}, CreateAdminRoleForm},
		{"invalid permission", "", url.Values{"name": {"editor"}, "permissions": {"root"}}, CreateAdminRoleForm},
		{"protected permissions", "admin", url.Values{"permissions": {"read"}}, UpdateAdminRolePermissionsForm},
		{"unknown default", "", url.Values{"default_role": {"missing"}}, SetDefaultAdminRoleForm},
		{"unconfirmed delete", "reviewer", url.Values{}, DeleteAdminRoleForm},
		{"default role delete", "user", url.Values{"confirm_delete": {"yes"}}, DeleteAdminRoleForm},
	} {
		t.Run(test.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			test.handler(w, roleFormRequest("/server/administration/roles", test.values, test.role))
			assertAdminFormRedirect(t, w, "/server/administration/roles")
		})
	}
}

func TestShowAdminRolesPageRendersConfiguredRows(t *testing.T) {
	_, _, usersDB := newAdminTestHandler(t)
	if err := config.WriteSetting("server.users.roles", map[string]interface{}{
		"available": []string{"admin", "user", "reviewer"}, "default": "reviewer",
		"permissions": map[string]interface{}{"reviewer": []string{"read", "moderate"}},
	}); err != nil {
		t.Fatal(err)
	}
	previous := middleware.RenderHTML
	middleware.RenderHTML = func(w http.ResponseWriter, _ *http.Request, status int, page string, data map[string]interface{}) {
		w.WriteHeader(status)
		if page != "admin/admin_roles.tmpl" {
			t.Errorf("template = %q", page)
		}
		roles, _ := data["roles"].([]adminRoleRow)
		if len(roles) != 3 || roles[2].Name != "reviewer" || !roles[2].IsDefault || !roles[2].CanRead || !roles[2].CanModerate {
			t.Errorf("role rows = %#v", roles)
		}
	}
	t.Cleanup(func() { middleware.RenderHTML = previous })
	if _, err := usersDB.Exec(`INSERT INTO users (username, email, password_hash, role, created_at, updated_at) VALUES ('alice', 'alice@example.test', 'hash', 'reviewer', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`); err != nil {
		t.Fatal(err)
	}
	w := httptest.NewRecorder()
	ShowAdminRolesPage(w, httptest.NewRequest(http.MethodGet, "/server/administration/roles", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
}

func TestStringSliceFiltersAndCopiesValues(t *testing.T) {
	input := []string{"read", "write"}
	got := stringSlice(input)
	got[0] = "changed"
	if input[0] != "read" {
		t.Fatal("stringSlice returned the original []string backing array")
	}
	filtered := stringSlice([]interface{}{"read", 12, "moderate"})
	if strings.Join(filtered, ",") != "read,moderate" {
		t.Fatalf("filtered values = %#v", filtered)
	}
	if stringSlice(12) != nil {
		t.Fatal("stringSlice accepted an unsupported type")
	}
}
