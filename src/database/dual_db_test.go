package database

import (
	"database/sql"
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
// creation, and initial schema_version insertion.
func TestInitServerDB_NewDatabase(t *testing.T) {
	db, err := initServerDB(memDSN("initserver"))
	if err != nil {
		t.Fatalf("initServerDB: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	stats := db.Stats()
	if stats.MaxOpenConnections != 10 {
		t.Errorf("MaxOpenConnections = %d, want 10", stats.MaxOpenConnections)
	}

	var version int
	if err := db.QueryRow("SELECT COALESCE(MAX(version), 0) FROM schema_version").Scan(&version); err != nil {
		t.Fatalf("query schema_version: %v", err)
	}
	if version != ServerSchemaVersion {
		t.Errorf("schema_version = %d, want %d", version, ServerSchemaVersion)
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
	if stats.MaxOpenConnections != 10 {
		t.Errorf("MaxOpenConnections = %d, want 10", stats.MaxOpenConnections)
	}

	var version int
	if err := db.QueryRow("SELECT COALESCE(MAX(version), 0) FROM schema_version").Scan(&version); err != nil {
		t.Fatalf("query schema_version: %v", err)
	}
	if version != UsersSchemaVersion {
		t.Errorf("schema_version = %d, want %d", version, UsersSchemaVersion)
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

// TestMigrateUsersDB_V6AddsDataColumn seeds a pre-v6 user_sessions table
// (no data column) and verifies the migration adds it, and that running the
// same migration again is a no-op (idempotent, no duplicate-column error).
func TestMigrateUsersDB_V6AddsDataColumn(t *testing.T) {
	db := newSchemaDB(t, memDSN("migv6"))

	if _, err := db.Exec(`
		CREATE TABLE user_sessions (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			user_id INTEGER NOT NULL,
			token_hash TEXT NOT NULL
		)
	`); err != nil {
		t.Fatalf("create v5 user_sessions: %v", err)
	}

	if err := migrateUsersDB(db, 5); err != nil {
		t.Fatalf("migrateUsersDB(fromVersion=5): %v", err)
	}
	if _, err := db.Exec("SELECT data FROM user_sessions LIMIT 0"); err != nil {
		t.Errorf("user_sessions.data missing after v6 migration: %v", err)
	}

	// Running again from the same "fromVersion" must not error (idempotency).
	if err := migrateUsersDB(db, 5); err != nil {
		t.Errorf("migrateUsersDB run twice: %v", err)
	}
}

// TestMigrateUsersDB_V7RenamesSessionID exercises the v7 migration: old rows
// are deleted, session_id is renamed to token_hash, and the unique index is
// created. Running it twice must not error.
func TestMigrateUsersDB_V7RenamesSessionID(t *testing.T) {
	db := newSchemaDB(t, memDSN("migv7"))

	if _, err := db.Exec(`
		CREATE TABLE user_sessions (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			user_id INTEGER NOT NULL,
			session_id TEXT NOT NULL,
			data TEXT
		)
	`); err != nil {
		t.Fatalf("create v6 user_sessions: %v", err)
	}
	if _, err := db.Exec("CREATE INDEX idx_sessions_id ON user_sessions(session_id)"); err != nil {
		t.Fatalf("create old index: %v", err)
	}
	if _, err := db.Exec("INSERT INTO user_sessions (user_id, session_id) VALUES (1, 'rawtoken123')"); err != nil {
		t.Fatalf("seed old session row: %v", err)
	}

	if err := migrateUsersDB(db, 6); err != nil {
		t.Fatalf("migrateUsersDB(fromVersion=6): %v", err)
	}

	// Old raw-token rows must have been purged.
	var count int
	if err := db.QueryRow("SELECT COUNT(*) FROM user_sessions").Scan(&count); err != nil {
		t.Fatalf("count sessions: %v", err)
	}
	if count != 0 {
		t.Errorf("user_sessions count after v7 migration = %d, want 0 (old raw-token rows must be purged)", count)
	}
	if _, err := db.Exec("SELECT token_hash FROM user_sessions LIMIT 0"); err != nil {
		t.Errorf("token_hash column missing after v7 migration: %v", err)
	}

	// Running the migration again from the same fromVersion must not error.
	if err := migrateUsersDB(db, 6); err != nil {
		t.Errorf("migrateUsersDB run twice: %v", err)
	}
}

// TestMigrateUsersDB_FromZeroRunsBothSteps verifies that migrating from a
// version below both thresholds (e.g. 0) chains v6 and v7 without error
// against a schema that already has token_hash (simulating the base schema).
func TestMigrateUsersDB_FromZeroRunsBothSteps(t *testing.T) {
	db := newSchemaDB(t, memDSN("migzero"))

	if _, err := db.Exec(UsersSchema); err != nil {
		t.Fatalf("create base schema: %v", err)
	}

	if err := migrateUsersDB(db, 0); err != nil {
		t.Fatalf("migrateUsersDB(fromVersion=0) against already-current schema: %v", err)
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
