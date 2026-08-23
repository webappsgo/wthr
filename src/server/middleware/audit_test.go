package middleware

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/webappsgo/wthr/src/common/dbtime"
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

// TestGetResourceFromPath_ConfiguredAdminPathNotDefault is a regression test
// for the companion half of the hardcoded-prefix bug covered by
// TestIsAdminRoute_ConfiguredAdminPathNotDefault. isAdminRoute was fixed to
// resolve the configured admin.path, but getResourceFromPath still compared
// path[:20]/path[:13] against literal "/api/v1/server/admin" and
// "/server/admin". With a non-default admin.path those literals never matched,
// so nothing was stripped and every audit row recorded the resource as "server"
// - the same value for every admin action, making the audit trail useless for
// telling one action apart from another.
//
// Both helpers now share adminRoutePrefixes, so they cannot drift again. The
// config fixture follows the same write-then-remove pattern as the isAdminRoute
// test above, since config.LoadConfig reads CONFIG_DIR/server.yml.
func TestGetResourceFromPath_ConfiguredAdminPathNotDefault(t *testing.T) {
	configFile := filepath.Join(setupTestConfigDir, "server.yml")
	if err := os.WriteFile(configFile, []byte("server:\n  admin_path: backoffice\n"), 0o600); err != nil {
		t.Fatalf("write server.yml fixture: %v", err)
	}
	t.Cleanup(func() { os.Remove(configFile) })

	tests := []struct {
		path string
		want string
	}{
		{"/server/backoffice", "dashboard"},
		{"/server/backoffice/users", "users"},
		{"/server/backoffice/users/42", "users"},
		{"/api/v1/server/backoffice/config", "config"},
		{"/api/v1/server/backoffice", "dashboard"},
	}
	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			if got := getResourceFromPath(tt.path); got != tt.want {
				t.Errorf("getResourceFromPath(%q) = %q, want %q - the admin prefix must "+
					"come from the configured admin.path; a fixed-length strip of the "+
					"default prefix leaves the resource name mangled", tt.path, got, tt.want)
			}
		})
	}
}

// TestAuditLogger_SkipsNonAdminAndSafeMethods verifies the middleware is a
// pure no-op (no error surfaced, handler runs normally) for non-admin routes
// and for GET/OPTIONS on admin routes - it must not attempt to write for
// read-only traffic.
func TestAuditLogger_SkipsNonAdminAndSafeMethods(t *testing.T) {
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
			next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte("ok"))
			})
			handler := AuditLogger(db)(next)

			w := httptest.NewRecorder()
			req := httptest.NewRequest(tt.method, tt.path, nil)
			handler.ServeHTTP(w, req)

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
func TestAuditLogger_WritesRowForMutatingAdminRequest(t *testing.T) {
	db := openAuditTestDB(t)

	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"ok": true})
	})
	handler := AuditLogger(db)(next)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/server/admin/users", nil)
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201", w.Code)
	}

	var count int
	err := db.QueryRow("SELECT COUNT(*) FROM server_audit_log WHERE action = 'create'").Scan(&count)
	if err != nil {
		t.Fatalf("query server_audit_log: %v", err)
	}
	if count != 1 {
		t.Errorf("server_audit_log rows for this request = %d, want 1", count)
	}
}

// TestAuditLogger_ReportsFailedWrite verifies a failed audit insert is loud in
// the log rather than silent.
//
// The middleware must never fail the request over a broken audit table, but a
// missing or broken server_audit_log must never drop every admin action with
// no operator-visible signal at all. PART 11 makes recording security-relevant
// actions non-negotiable, which makes a failure to record one a security event
// in its own right.
func TestAuditLogger_ReportsFailedWrite(t *testing.T) {
	// A database with no schema at all, so the insert cannot succeed.
	db, err := sql.Open("sqlite", fmt.Sprintf("file:audit_noschema_%d?mode=memory&cache=shared", time.Now().UnixNano()))
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	var logged bytes.Buffer
	previousOutput := log.Writer()
	previousFlags := log.Flags()
	log.SetOutput(&logged)
	log.SetFlags(0)
	t.Cleanup(func() {
		log.SetOutput(previousOutput)
		log.SetFlags(previousFlags)
	})

	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"ok": true})
	})
	handler := AuditLogger(db)(next)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/server/admin/users", nil)
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Errorf("status = %d, want 201 - a failed audit write must not fail the request", w.Code)
	}
	if got := logged.String(); !strings.Contains(got, "audit:") {
		t.Errorf("log output = %q, want an \"audit:\" failure line - a dropped audit "+
			"record must be visible to the operator", got)
	}
}

// TestAuditLogger_WritesCanonicalTimestampText is a regression test for the
// mixed-layout bug in server_audit_log.timestamp.
//
// The column has three producers: this middleware, src/scheduler/scheduler.go
// and src/server/handler/admin_passkey.go. The latter two write canonical
// "YYYY-MM-DD HH:MM:SS" UTC text. AuditLogger used to bind a raw time.Time,
// which modernc.org/sqlite serializes with time.Time.String() - the
// host-LOCAL "2006-01-02 15:04:05.999999999 -0700 MST" form, plus a
// monotonic-clock suffix for values from time.Now(). The single column then
// held two incomparable encodings, which is why the GraphQL request-stats
// reader has to carry an oversized skew prefilter.
//
// The strict time.Parse below is what catches the old code: a
// time.Time.String() value never parses against dbtime.SQLTimestampLayout, in
// any host timezone (a UTC host still yields the "+0000 UTC" zone suffix), so
// this test fails against the raw-time.Time bind and passes only once the
// write goes through dbtime.FormatSQLTimestamp.
func TestAuditLogger_WritesCanonicalTimestampText(t *testing.T) {
	db := openAuditTestDB(t)

	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"ok": true})
	})
	handler := AuditLogger(db)(next)

	before := time.Now().UTC().Truncate(time.Second)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/server/admin/users", nil)
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201", w.Code)
	}

	after := time.Now().UTC()

	// Scanned untyped, and CAST to TEXT in SQL, so the driver hands back
	// exactly what is on disk. A plain "SELECT timestamp" would trigger
	// modernc.org/sqlite's decltype-driven DATETIME auto-parsing (the
	// declared column type is DATETIME) and hand back a time.Time no matter
	// which text layout was actually stored, hiding the bug this test exists
	// to catch. CAST(... AS TEXT) makes the result an expression with no
	// column decltype, so the driver returns the raw stored bytes.
	var stored interface{}
	if err := db.QueryRow("SELECT CAST(timestamp AS TEXT) FROM server_audit_log WHERE action = 'create'").Scan(&stored); err != nil {
		t.Fatalf("query server_audit_log.timestamp: %v", err)
	}

	var text string
	switch value := stored.(type) {
	case string:
		text = value
	case []byte:
		text = string(value)
	default:
		t.Fatalf("server_audit_log.timestamp scanned as %T (%v), want text - the "+
			"column must hold canonical text written by dbtime.FormatSQLTimestamp", stored, stored)
	}

	parsed, err := time.Parse(dbtime.SQLTimestampLayout, text)
	if err != nil {
		t.Fatalf("server_audit_log.timestamp = %q does not parse with "+
			"dbtime.SQLTimestampLayout (%q): %v - AuditLogger must bind "+
			"dbtime.FormatSQLTimestamp(now), not a raw time.Time (which "+
			"modernc.org/sqlite stores in the host-local time.Time.String() form)",
			text, dbtime.SQLTimestampLayout, err)
	}

	// Round-trip: the canonical text must denote the same absolute instant the
	// request happened at, so a UTC value was written rather than a local
	// wall-clock reading formatted without its offset.
	parsed = parsed.UTC()
	if parsed.Before(before) || parsed.After(after) {
		t.Errorf("server_audit_log.timestamp = %q parses to %s, want an instant "+
			"within [%s, %s] - the stored text must be UTC, not local wall-clock",
			text, parsed.Format(time.RFC3339), before.Format(time.RFC3339), after.Format(time.RFC3339))
	}

	// The shared reader path must agree with the strict parse above; any
	// disagreement means the value only survives via a legacy-tolerance layout.
	viaDBTime, ok := dbtime.ParseStoredTimestamp(stored)
	if !ok {
		t.Fatalf("dbtime.ParseStoredTimestamp(%q) reported false for a value this project just wrote", text)
	}
	if !viaDBTime.Equal(parsed) {
		t.Errorf("dbtime.ParseStoredTimestamp(%q) = %s, want %s - the stored value "+
			"must round-trip identically through both parse paths",
			text, viaDBTime.Format(time.RFC3339), parsed.Format(time.RFC3339))
	}
}
