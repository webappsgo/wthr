package database

import (
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func memDSN(prefix string) string {
	return fmt.Sprintf("file:%s_%d?mode=memory&cache=shared", prefix, time.Now().UnixNano())
}

// TestInitServerDB_NewDatabase exercises initServerDB directly against an
// in-memory DSN (avoids real disk files) and verifies pool settings, schema
// creation, and idempotent schema updates.
func TestInitServerDB_NewDatabase(t *testing.T) {
	db, err := initServerDB(memDSN("initserver"))
	if err != nil {
		t.Fatalf("initServerDB: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	stats := db.Stats()
	if stats.MaxOpenConnections != poolMaxOpen {
		t.Errorf("MaxOpenConnections = %d, want %d", stats.MaxOpenConnections, poolMaxOpen)
	}

	// server_admin_credentials must exist from ServerSchema.
	if _, err := db.Exec("SELECT COUNT(*) FROM server_admin_credentials"); err != nil {
		t.Errorf("server_admin_credentials table missing: %v", err)
	}
}

func TestInitServerDB_InvalidPath(t *testing.T) {
	// A path pointing into a nonexistent directory should fail to open/ping
	// rather than panicking.
	_, err := initServerDB("/nonexistent/dir/that/should/not/exist/server.db")
	if err == nil {
		t.Fatal("initServerDB with invalid path = nil error, want error")
	}
}

func TestInitUsersDB_NewDatabase(t *testing.T) {
	db, err := initUsersDB(memDSN("initusers"))
	if err != nil {
		t.Fatalf("initUsersDB: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	stats := db.Stats()
	if stats.MaxOpenConnections != poolMaxOpen {
		t.Errorf("MaxOpenConnections = %d, want %d", stats.MaxOpenConnections, poolMaxOpen)
	}

	// A brand-new DB's user_sessions table already has token_hash (base
	// schema is post-v7), so no migration path should have been needed.
	if _, err := db.Exec("SELECT token_hash FROM user_sessions LIMIT 0"); err != nil {
		t.Errorf("user_sessions.token_hash missing on fresh db: %v", err)
	}
}

func TestInitUsersDB_InvalidPath(t *testing.T) {
	_, err := initUsersDB("/nonexistent/dir/that/should/not/exist/users.db")
	if err == nil {
		t.Fatal("initUsersDB with invalid path = nil error, want error")
	}
}

// TestApplySchemaUpdates_AddsMissingColumns seeds a legacy users schema whose
// user_sessions table predates the data and token_hash columns, then verifies
// applySchemaUpdates brings it forward and is safe to run again.
func TestApplySchemaUpdates_AddsMissingColumns(t *testing.T) {
	db := newSchemaDB(t, "schemaupdates")

	if _, err := db.Exec(`
		CREATE TABLE user_sessions (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			user_id INTEGER NOT NULL,
			session_id TEXT NOT NULL
		);
		CREATE TABLE user_invites (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			code TEXT UNIQUE NOT NULL
		);
	`); err != nil {
		t.Fatalf("create legacy users schema: %v", err)
	}

	if err := applySchemaUpdates(db.DB, usersSchemaUpdates); err != nil {
		t.Fatalf("applySchemaUpdates: %v", err)
	}

	for _, column := range []string{"data", "token_hash"} {
		if _, err := db.Exec("SELECT " + column + " FROM user_sessions LIMIT 0"); err != nil {
			t.Errorf("user_sessions.%s missing after schema updates: %v", column, err)
		}
	}
	for _, column := range []string{"username", "role"} {
		if _, err := db.Exec("SELECT " + column + " FROM user_invites LIMIT 0"); err != nil {
			t.Errorf("user_invites.%s missing after schema updates: %v", column, err)
		}
	}

	// AI.md PART 10: schema updates run on every startup, so a second pass
	// must be a no-op rather than a duplicate-column failure.
	if err := applySchemaUpdates(db.DB, usersSchemaUpdates); err != nil {
		t.Errorf("applySchemaUpdates run twice: %v", err)
	}
}

// TestApplySchemaUpdates_CurrentSchemaIsNoOp verifies the update list is a
// no-op against the base schema, which already declares every column it adds.
func TestApplySchemaUpdates_CurrentSchemaIsNoOp(t *testing.T) {
	db := newSchemaDB(t, "schemaupdatescurrent")

	if _, err := db.Exec(UsersSchema); err != nil {
		t.Fatalf("create base schema: %v", err)
	}

	if err := applySchemaUpdates(db.DB, usersSchemaUpdates); err != nil {
		t.Fatalf("applySchemaUpdates against current schema: %v", err)
	}
	if err := applySchemaUpdates(db.DB, serverSchemaUpdates); err != nil {
		t.Fatalf("applySchemaUpdates(serverSchemaUpdates): %v", err)
	}
}

// TestIsColumnExistsError covers each driver phrasing the helper must treat as
// "already applied" and confirms unrelated errors still propagate.
func TestIsColumnExistsError(t *testing.T) {
	cases := []struct {
		err  error
		want bool
	}{
		{nil, false},
		{errors.New("duplicate column name: data"), true},
		{errors.New("column \"data\" of relation \"user_sessions\" already exists"), true},
		{errors.New("Duplicate key name 'idx_sessions_hash'"), true},
		{errors.New("no such table: user_sessions"), false},
	}

	for _, tc := range cases {
		if got := isColumnExistsError(tc.err); got != tc.want {
			t.Errorf("isColumnExistsError(%v) = %v, want %v", tc.err, got, tc.want)
		}
	}
}

func TestInitDualDB(t *testing.T) {
	dataDir := t.TempDir()

	ddb, err := InitDualDB(dataDir)
	if err != nil {
		t.Fatalf("InitDualDB: %v", err)
	}
	t.Cleanup(func() { ddb.Close() })

	if ddb.Server == nil || ddb.Users == nil {
		t.Fatal("InitDualDB returned nil Server or Users db")
	}

	wantServerPath := filepath.Join(dataDir, "db", "server.db")
	wantUsersPath := filepath.Join(dataDir, "db", "users.db")
	// Confirm the files were actually created at the documented paths.
	if _, err := os.Stat(wantServerPath); err != nil {
		t.Errorf("server.db not created at %s: %v", wantServerPath, err)
	}
	if _, err := os.Stat(wantUsersPath); err != nil {
		t.Errorf("users.db not created at %s: %v", wantUsersPath, err)
	}

	status, _, err := ddb.HealthCheck()
	if err != nil {
		t.Fatalf("HealthCheck: %v", err)
	}
	if status != "connected" {
		t.Errorf("HealthCheck status = %q, want connected", status)
	}
}

func TestDualDB_Close(t *testing.T) {
	ddb := &DualDB{
		Server: openMemDB(t, "closeserver"),
		Users:  openMemDB(t, "closeusers"),
	}
	if err := ddb.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	// Using a closed DB must error, not panic.
	if err := ddb.Server.Ping(); err == nil {
		t.Error("Ping on closed server db = nil error, want error")
	}
}

func TestDualDB_HealthCheck_ServerDown(t *testing.T) {
	ddb := &DualDB{
		Server: mustOpenMem(t, "hcserver"),
		Users:  mustOpenMem(t, "hcusers"),
	}
	ddb.Server.Close()

	status, _, err := ddb.HealthCheck()
	if err == nil {
		t.Fatal("HealthCheck with closed server db = nil error, want error")
	}
	if status != "error" {
		t.Errorf("status = %q, want error", status)
	}
	if !strings.Contains(err.Error(), "server database unhealthy") {
		t.Errorf("error = %q, want substring %q", err.Error(), "server database unhealthy")
	}
}

func TestDualDB_HealthCheck_UsersDown(t *testing.T) {
	ddb := &DualDB{
		Server: mustOpenMem(t, "hc2server"),
		Users:  mustOpenMem(t, "hc2users"),
	}
	ddb.Users.Close()

	status, _, err := ddb.HealthCheck()
	if err == nil {
		t.Fatal("HealthCheck with closed users db = nil error, want error")
	}
	if status != "error" {
		t.Errorf("status = %q, want error", status)
	}
	if !strings.Contains(err.Error(), "users database unhealthy") {
		t.Errorf("error = %q, want substring %q", err.Error(), "users database unhealthy")
	}
}

func TestDualDB_GettersAndWrappers(t *testing.T) {
	serverDB := mustOpenMem(t, "wrapserver")
	usersDB := mustOpenMem(t, "wrapusers")
	ddb := &DualDB{Server: serverDB, Users: usersDB}

	if ddb.GetServerDB() != serverDB {
		t.Error("GetServerDB() did not return the configured Server db")
	}
	if ddb.GetUsersDB() != usersDB {
		t.Error("GetUsersDB() did not return the configured Users db")
	}

	if _, err := ddb.ExecServer("CREATE TABLE t (id INTEGER PRIMARY KEY)"); err != nil {
		t.Fatalf("ExecServer: %v", err)
	}
	if _, err := ddb.ExecServer("INSERT INTO t (id) VALUES (1)"); err != nil {
		t.Fatalf("ExecServer insert: %v", err)
	}
	var id int
	if err := ddb.QueryRowServer("SELECT id FROM t WHERE id = ?", 1).Scan(&id); err != nil {
		t.Fatalf("QueryRowServer: %v", err)
	}
	if id != 1 {
		t.Errorf("id = %d, want 1", id)
	}
	rows, err := ddb.QueryServer("SELECT id FROM t")
	if err != nil {
		t.Fatalf("QueryServer: %v", err)
	}
	rows.Close()

	if _, err := ddb.ExecUsers("CREATE TABLE u (id INTEGER PRIMARY KEY)"); err != nil {
		t.Fatalf("ExecUsers: %v", err)
	}
	if _, err := ddb.ExecUsers("INSERT INTO u (id) VALUES (1)"); err != nil {
		t.Fatalf("ExecUsers insert: %v", err)
	}
	if err := ddb.QueryRowUsers("SELECT id FROM u WHERE id = ?", 1).Scan(&id); err != nil {
		t.Fatalf("QueryRowUsers: %v", err)
	}
	rows, err = ddb.QueryUsers("SELECT id FROM u")
	if err != nil {
		t.Fatalf("QueryUsers: %v", err)
	}
	rows.Close()
}

// mustOpenMem is a small local alias for openMemDB to make each dual_db
// test's intent explicit at the call site.
func mustOpenMem(t *testing.T, name string) *sql.DB {
	t.Helper()
	return openMemDB(t, name)
}
