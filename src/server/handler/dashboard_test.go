package handler

import (
	"net/http"
	"testing"
)

// TestDashboardHandler_ShowDashboard_Unauthenticated verifies an
// unauthenticated request (no user in the gin context) is redirected to
// the login page rather than attempting a template render.
func TestDashboardHandler_ShowDashboard_Unauthenticated(t *testing.T) {
	h := &DashboardHandler{DB: newTestServerDB(t)}

	c, w := newAPITestContext("/dashboard")
	h.ShowDashboard(c)

	if w.Code != http.StatusFound {
		t.Fatalf("expected status 302, got %d", w.Code)
	}
	if location := w.Header().Get("Location"); location != "/server/auth/login" {
		t.Errorf("expected redirect to /server/auth/login, got %q", location)
	}
}

// TestDashboardHandler_ShowAdminPanel_MissingAdminID verifies a request
// with no "admin_id" set in the gin context is redirected to the admin
// login page rather than attempting a template render.
func TestDashboardHandler_ShowAdminPanel_MissingAdminID(t *testing.T) {
	h := &DashboardHandler{DB: newTestServerDB(t)}

	c, w := newAPITestContext("/server/admin")
	h.ShowAdminPanel(c)

	if w.Code != http.StatusFound {
		t.Fatalf("expected status 302, got %d", w.Code)
	}
	if location := w.Header().Get("Location"); location != "/server/admin" {
		t.Errorf("expected redirect to /server/admin, got %q", location)
	}
}

// TestDashboardHandler_ShowAdminPanel_WrongAdminIDType verifies a
// non-int "admin_id" context value is treated the same as missing and
// redirects to the admin login page.
func TestDashboardHandler_ShowAdminPanel_WrongAdminIDType(t *testing.T) {
	h := &DashboardHandler{DB: newTestServerDB(t)}

	c, w := newAPITestContext("/server/admin")
	c.Set("admin_id", "not-an-int")
	h.ShowAdminPanel(c)

	if w.Code != http.StatusFound {
		t.Fatalf("expected status 302, got %d", w.Code)
	}
	if location := w.Header().Get("Location"); location != "/server/admin" {
		t.Errorf("expected redirect to /server/admin, got %q", location)
	}
}

// TestDashboardHandler_ShowAdminPanel_AdminNotFound verifies a valid-typed
// "admin_id" that doesn't exist in the admins table redirects to the
// admin login page instead of erroring.
func TestDashboardHandler_ShowAdminPanel_AdminNotFound(t *testing.T) {
	serverDB := newTestServerDB(t)
	setGlobalTestDualDB(t, serverDB, newTestServerDB(t))

	h := &DashboardHandler{DB: serverDB}

	c, w := newAPITestContext("/server/admin")
	c.Set("admin_id", 999999)
	h.ShowAdminPanel(c)

	if w.Code != http.StatusFound {
		t.Fatalf("expected status 302, got %d", w.Code)
	}
	if location := w.Header().Get("Location"); location != "/server/admin" {
		t.Errorf("expected redirect to /server/admin, got %q", location)
	}
}
