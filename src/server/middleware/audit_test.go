package middleware

import (
	"database/sql"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/webappsgo/wthr/src/database"
	_ "modernc.org/sqlite"
)

// openAuditTestDB opens an in-memory SQLite db seeded with the real
// ServerSchema, matching the pattern in src/database/dual_db_test.go
// (unique DSN per test via UnixNano to avoid cross-test collisions on the
// shared-cache in-memory DSN).
func openAuditTestDB(t *testing.T) *sql.DB {
	t.Helper()
	dsn := fmt.Sprintf("file:audit_%d?mode=memory&cache=shared", time.Now().UnixNano())
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if _, err := db.Exec(database.ServerSchema); err != nil {
		t.Fatalf("apply ServerSchema: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

// TestIsAdminRoute covers the route-matching helper directly. AI.md
// backend-rules.md requires "Audit log every admin action" - this helper
// gates which routes get logged at all.
func TestIsAdminRoute(t *testing.T) {
	tests := []struct {
		path string
		want bool
	}{
		{"/server/admin", true},
		{"/server/admin/users", true},
		{"/api/v1/server/admin/config", true},
		{"/users/settings", false},
		{"/api/v1/weather/london", false},
		{"/", false},
	}
	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			if got := isAdminRoute(tt.path); got != tt.want {
				t.Errorf("isAdminRoute(%q) = %v, want %v", tt.path, got, tt.want)
			}
		})
	}
}

// TestIsAdminRoute_ConfiguredAdminPathNotDefault verifies isAdminRoute
// derives the admin prefix from the configured admin.path (via
// config.LoadConfig()/cfg.GetAdminPath(), same pattern as auth.go) instead
// of hardcoding "/server/admin". This is a regression test for a real
// production bug that was fixed in audit.go: previously the prefix was
// hardcoded, so audit logging silently stopped matching admin routes
// whenever admin.path was customized away from the default "admin".
//
// isAdminRoute loads config via config.LoadConfig(), which reads
// {CONFIG_DIR}/server.yml (src/config/config.go findConfigFile). TestMain
// pins CONFIG_DIR to setupTestConfigDir for the whole test binary, so this
// test writes a server.yml there with a non-default admin_path, then
// restores the directory to its previous (no server.yml) state so it
// doesn't leak into other tests in this package.
func TestIsAdminRoute_ConfiguredAdminPathNotDefault(t *testing.T) {
	customAdminPath := "/server/backoffice" // admin.path: backoffice

	configFile := filepath.Join(setupTestConfigDir, "server.yml")
	if err := os.WriteFile(configFile, []byte("server:\n  admin_path: backoffice\n"), 0o600); err != nil {
		t.Fatalf("write server.yml fixture: %v", err)
	}
	t.Cleanup(func() { os.Remove(configFile) })

	if got := isAdminRoute(customAdminPath + "/users"); !got {
		t.Errorf("isAdminRoute(%q) = false, want true - audit.go should derive "+
			"the admin prefix from the configured admin.path (%q) instead of a "+
			"hardcoded \"/server/admin\"", customAdminPath+"/users", customAdminPath)
	}
}

// TestGetActionFromRequest covers the method-to-action mapping.
func TestGetActionFromRequest(t *testing.T) {
	tests := []struct {
		method string
		path   string
		want   string
	}{
		{"POST", "/server/admin/auth/login", "login"},
		{"POST", "/server/admin/auth/logout", "logout"},
		{"POST", "/server/admin/users", "create"},
		{"PUT", "/server/admin/users/1", "update"},
		{"PATCH", "/server/admin/users/1", "update"},
		{"DELETE", "/server/admin/users/1", "delete"},
		{"HEAD", "/server/admin/users/1", "HEAD"},
	}
	for _, tt := range tests {
		t.Run(tt.method+"_"+tt.path, func(t *testing.T) {
			if got := getActionFromRequest(tt.method, tt.path); got != tt.want {
				t.Errorf("getActionFromRequest(%q, %q) = %q, want %q", tt.method, tt.path, got, tt.want)
			}
		})
	}
}

// TestGetResourceFromPath covers resource-name extraction for both the web
// and API-prefixed admin paths, plus the dashboard-root fallback.
func TestGetResourceFromPath(t *testing.T) {
	tests := []struct {
		path string
		want string
	}{
		{"/server/admin", "dashboard"},
		{"/server/admin/users", "users"},
		{"/server/admin/users/42", "users"},
		{"/api/v1/server/admin/config", "config"},
		{"/api/v1/server/admin", "dashboard"},
	}
	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			if got := getResourceFromPath(tt.path); got != tt.want {
				t.Errorf("getResourceFromPath(%q) = %q, want %q", tt.path, got, tt.want)
			}
		})
	}
}

// TestAuditLogger_SkipsNonAdminAndSafeMethods verifies the middleware is a
// pure no-op (no error surfaced, handler runs normally) for non-admin routes
// and for GET/OPTIONS on admin routes - it must not attempt to write for
// read-only traffic.
func TestAuditLogger_SkipsNonAdminAndSafeMethods(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := openAuditTestDB(t)

	tests := []struct {
		name   string
		method string
		path   string
	}{
		{"non-admin route", http.MethodPost, "/api/v1/weather/london"},
		{"admin route but GET", http.MethodGet, "/server/admin/users"},
		{"admin route but OPTIONS", http.MethodOptions, "/server/admin/users"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			router := gin.New()
			router.Use(AuditLogger(db))
			router.Handle(tt.method, tt.path, func(c *gin.Context) { c.String(http.StatusOK, "ok") })

			w := httptest.NewRecorder()
			req := httptest.NewRequest(tt.method, tt.path, nil)
			router.ServeHTTP(w, req)

			if w.Code != http.StatusOK {
				t.Errorf("status = %d, want 200", w.Code)
			}
			if h := w.Result().Header.Get("X-Test-Errors"); h != "" {
				t.Errorf("unexpected error header: %s", h)
			}
		})
	}
}

// TestAuditLogger_WritesRowForMutatingAdminRequest is the core behavioral
// test: a mutating request to an admin route should result in a row being
// written to the real audit-log table defined by database.ServerSchema
// (server_audit_log).
//
// This test encodes CORRECT expected behavior. It is expected to FAIL
// against the current implementation because audit.go:47 issues
// `INSERT INTO audit_log (...)` - a table that does not exist anywhere in
// database.ServerSchema (the real table is `server_audit_log` with an
// entirely different column set: ulid, timestamp, actor_type, actor_id,
// resource_type, resource_id, details, status, error - none of which match
// audit.go's user_id/resource/created_at/success columns). Every admin
// audit-log write therefore fails with "no such table: audit_log" in
// production, and AuditLogger swallows the error via c.Error(err) without
// aborting the request, so this is currently silent: admin actions are
// never actually audited despite backend-rules.md's non-negotiable
// requirement to "Audit log every admin action".
func TestAuditLogger_WritesRowForMutatingAdminRequest(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := openAuditTestDB(t)

	router := gin.New()
	router.Use(AuditLogger(db))
	router.POST("/server/admin/users", func(c *gin.Context) {
		c.JSON(http.StatusCreated, gin.H{"ok": true})
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/server/admin/users", nil)
	router.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201", w.Code)
	}

	var count int
	err := db.QueryRow("SELECT COUNT(*) FROM server_audit_log WHERE action = 'create'").Scan(&count)
	if err != nil {
		t.Fatalf("query server_audit_log: %v", err)
	}
	if count != 1 {
		t.Errorf("server_audit_log rows for this request = %d, want 1 - "+
			"AuditLogger (audit.go:46-49) inserts into a table named 'audit_log' "+
			"that does not exist in database.ServerSchema; the real table is "+
			"'server_audit_log' with different columns, so this write silently "+
			"fails in production and no audit trail is ever recorded for admin actions", count)
	}
}
