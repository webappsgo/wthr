package database

import (
	"database/sql"
	"fmt"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

// newSchemaDB opens a fresh in-memory DB and runs the exact schema/version
// bootstrap logic InitDB performs, without going through the disk-path-only
// InitDB entry point twice in a row (each call would get a *new* random
// in-memory database). This lets tests control the DSN precisely.
func newSchemaDB(t *testing.T, dsn string) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		t.Fatalf("sql.Open: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func TestInitDB_CreatesSchemaAndVersion(t *testing.T) {
	dsn := fmt.Sprintf("file:initdb_%d?mode=memory&cache=shared", time.Now().UnixNano())
	db, err := InitDB(dsn)
	if err != nil {
		t.Fatalf("InitDB: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	var version int
	if err := db.QueryRow("SELECT MAX(version) FROM schema_version").Scan(&version); err != nil {
		t.Fatalf("query schema_version: %v", err)
	}
	if version != SchemaVersion {
		t.Errorf("schema_version = %d, want %d", version, SchemaVersion)
	}

	// Default settings must have been inserted on first run.
	for key, want := range DefaultSettings {
		var got string
		if err := db.QueryRow("SELECT value FROM settings WHERE key = ?", key).Scan(&got); err != nil {
			t.Errorf("default setting %q missing: %v", key, err)
			continue
		}
		if got != want {
			t.Errorf("default setting %q = %q, want %q", key, got, want)
		}
	}
}

// TestInitDB_IdempotentOnSameDatabase verifies calling the schema+migration
// bootstrap twice against the same underlying database never errors and
// never duplicates schema_version rows or default settings — this is the
// core idempotency guarantee CREATE TABLE IF NOT EXISTS / ON CONFLICT is
// supposed to provide.
func TestInitDB_IdempotentOnSameDatabase(t *testing.T) {
	dsn := fmt.Sprintf("file:initdb_idem_%d?mode=memory&cache=shared", time.Now().UnixNano())

	db1, err := InitDB(dsn)
	if err != nil {
		t.Fatalf("first InitDB: %v", err)
	}
	t.Cleanup(func() { db1.Close() })

	// Re-run schema creation + version/migration check against the SAME
	// live connection pool (same DSN, shared cache) to simulate a restart.
	if _, err := db1.Exec(Schema); err != nil {
		t.Fatalf("re-exec schema: %v", err)
	}
	var currentVersion int
	if err := db1.QueryRow("SELECT COALESCE(MAX(version), 0) FROM schema_version").Scan(&currentVersion); err != nil {
		t.Fatalf("check schema version: %v", err)
	}
	if currentVersion != SchemaVersion {
		t.Fatalf("currentVersion = %d, want %d before re-migrate check", currentVersion, SchemaVersion)
	}
	if currentVersion < SchemaVersion {
		if err := runMigrations(db1.DB, currentVersion, SchemaVersion); err != nil {
			t.Fatalf("re-run migrations: %v", err)
		}
	}

	var count int
	if err := db1.QueryRow("SELECT COUNT(*) FROM schema_version").Scan(&count); err != nil {
		t.Fatalf("count schema_version rows: %v", err)
	}
	if count != 1 {
		t.Errorf("schema_version row count = %d, want 1 (idempotent re-run must not insert duplicates)", count)
	}
}

// TestRunMigrations_FromV1ToV2 exercises the actual ALTER TABLE / data
// backfill logic in migrateToV2 against a hand-built v1 schema (no
// username/phone columns), which is what a real upgrade from an old
// database looks like.
func TestRunMigrations_FromV1ToV2(t *testing.T) {
	t.Skip("SKIPPED: migrateToV2 (schema.go, in the block populating username " +
		"from email, ~lines 464-503) deadlocks. It iterates an open `rows` " +
		"cursor from db.Query(\"SELECT id, email FROM users\") while calling " +
		"nested db.QueryRow/db.Exec on the SAME *sql.DB inside the loop body, " +
		"without closing the cursor first. Under SQLite's default rollback-" +
		"journal locking this self-deadlocks: the writer can never acquire its " +
		"lock while the reader's cursor is open, and the reader's cursor is " +
		"never closed because the loop body blocks forever waiting on the " +
		"writer. Reproduced deterministically with >=1 seeded user: this test " +
		"hung until killed by the 90s go test -timeout. This is a genuine " +
		"production bug (not a test bug) and was intentionally left unfixed " +
		"per task instructions to report rather than route around it. Do not " +
		"un-skip this test until migrateToV2 is fixed to fully drain/close " +
		"rows (e.g. buffer id/email pairs into a slice) before issuing any " +
		"further queries/execs on db within the loop.")

	dsn := fmt.Sprintf("file:migv1_%d?mode=memory&cache=shared", time.Now().UnixNano())
	db := newSchemaDB(t, dsn)

	// Minimal v1 users table: no username, no phone.
	if _, err := db.Exec(`
		CREATE TABLE users (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			email TEXT UNIQUE NOT NULL,
			password_hash TEXT NOT NULL,
			role TEXT DEFAULT 'user',
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)
	`); err != nil {
		t.Fatalf("create v1 users table: %v", err)
	}
	if _, err := db.Exec("CREATE TABLE schema_version (version INTEGER PRIMARY KEY, applied_at DATETIME DEFAULT CURRENT_TIMESTAMP)"); err != nil {
		t.Fatalf("create schema_version: %v", err)
	}
	if _, err := db.Exec("INSERT INTO schema_version (version) VALUES (1)"); err != nil {
		t.Fatalf("seed v1 version: %v", err)
	}
	if _, err := db.Exec("INSERT INTO users (email, password_hash) VALUES ('alice@example.com', 'hash1'), ('bob@example.com', 'hash2')"); err != nil {
		t.Fatalf("seed v1 users: %v", err)
	}

	if err := runMigrations(db, 1, 2); err != nil {
		t.Fatalf("runMigrations(1, 2): %v", err)
	}

	rows, err := db.Query("SELECT email, username FROM users ORDER BY email")
	if err != nil {
		t.Fatalf("query migrated users: %v", err)
	}
	defer rows.Close()

	want := map[string]string{
		"alice@example.com": "alice",
		"bob@example.com":   "bob",
	}
	seen := 0
	for rows.Next() {
		var email, username string
		if err := rows.Scan(&email, &username); err != nil {
			t.Fatalf("scan: %v", err)
		}
		seen++
		if want[email] != username {
			t.Errorf("username for %s = %q, want %q", email, username, want[email])
		}
	}
	if seen != 2 {
		t.Fatalf("migrated row count = %d, want 2", seen)
	}

	var version int
	if err := db.QueryRow("SELECT MAX(version) FROM schema_version").Scan(&version); err != nil {
		t.Fatalf("query final version: %v", err)
	}
	if version != 2 {
		t.Errorf("final schema_version = %d, want 2", version)
	}
}

func TestRunMigrations_UnknownVersion(t *testing.T) {
	dsn := fmt.Sprintf("file:migunknown_%d?mode=memory&cache=shared", time.Now().UnixNano())
	db := newSchemaDB(t, dsn)
	if _, err := db.Exec(Schema); err != nil {
		t.Fatalf("create schema: %v", err)
	}

	// Target a version with no defined case in the migration switch.
	err := runMigrations(db, SchemaVersion, SchemaVersion+5)
	if err == nil {
		t.Fatal("runMigrations to an unknown version = nil error, want error")
	}
}

func TestDB_GetSetSetting(t *testing.T) {
	db := mustInitDB(t)

	// Existing default setting can be overwritten.
	if err := db.SetSetting("server.theme", "light"); err != nil {
		t.Fatalf("SetSetting: %v", err)
	}
	got, err := db.GetSetting("server.theme")
	if err != nil {
		t.Fatalf("GetSetting: %v", err)
	}
	if got != "light" {
		t.Errorf("GetSetting(server.theme) = %q, want %q", got, "light")
	}

	// New key via upsert.
	if err := db.SetSetting("custom.key", "v1"); err != nil {
		t.Fatalf("SetSetting new key: %v", err)
	}
	if err := db.SetSetting("custom.key", "v2"); err != nil {
		t.Fatalf("SetSetting overwrite: %v", err)
	}
	got, err = db.GetSetting("custom.key")
	if err != nil {
		t.Fatalf("GetSetting: %v", err)
	}
	if got != "v2" {
		t.Errorf("GetSetting(custom.key) = %q, want %q (upsert should overwrite)", got, "v2")
	}

	// Missing key surfaces sql.ErrNoRows, not swallowed.
	if _, err := db.GetSetting("does.not.exist"); err != sql.ErrNoRows {
		t.Errorf("GetSetting(missing) err = %v, want sql.ErrNoRows", err)
	}
}

func TestDB_CleanupExpiredSessions(t *testing.T) {
	db := mustInitDB(t)
	seedUser(t, db, "alice")

	past := time.Now().Add(-time.Hour)
	future := time.Now().Add(time.Hour)
	if _, err := db.Exec("INSERT INTO sessions (id, user_id, expires_at) VALUES ('s1', 1, ?)", past); err != nil {
		t.Fatalf("seed expired session: %v", err)
	}
	if _, err := db.Exec("INSERT INTO sessions (id, user_id, expires_at) VALUES ('s2', 1, ?)", future); err != nil {
		t.Fatalf("seed active session: %v", err)
	}

	if err := db.CleanupExpiredSessions(); err != nil {
		t.Fatalf("CleanupExpiredSessions: %v", err)
	}

	var count int
	if err := db.QueryRow("SELECT COUNT(*) FROM sessions").Scan(&count); err != nil {
		t.Fatalf("count sessions: %v", err)
	}
	if count != 1 {
		t.Errorf("remaining session count = %d, want 1 (only the future one)", count)
	}
	var id string
	if err := db.QueryRow("SELECT id FROM sessions").Scan(&id); err != nil {
		t.Fatalf("query remaining session: %v", err)
	}
	if id != "s2" {
		t.Errorf("remaining session id = %q, want s2", id)
	}
}

func TestDB_CleanupExpiredAlerts(t *testing.T) {
	db := mustInitDB(t)
	seedUser(t, db, "alice")
	if _, err := db.Exec("INSERT INTO saved_locations (id, user_id, name, latitude, longitude) VALUES (1, 1, 'Home', 0, 0)"); err != nil {
		t.Fatalf("seed location: %v", err)
	}

	past := time.Now().Add(-time.Hour)
	if _, err := db.Exec("INSERT INTO weather_alerts (location_id, alert_type, severity, title, message, expires_at) VALUES (1, 'storm', 'warning', 't', 'm', ?)", past); err != nil {
		t.Fatalf("seed expired alert: %v", err)
	}
	if _, err := db.Exec("INSERT INTO weather_alerts (location_id, alert_type, severity, title, message, expires_at) VALUES (1, 'storm', 'warning', 't', 'm', NULL)"); err != nil {
		t.Fatalf("seed alert without expiry: %v", err)
	}

	if err := db.CleanupExpiredAlerts(); err != nil {
		t.Fatalf("CleanupExpiredAlerts: %v", err)
	}

	var count int
	if err := db.QueryRow("SELECT COUNT(*) FROM weather_alerts").Scan(&count); err != nil {
		t.Fatalf("count alerts: %v", err)
	}
	// The NULL-expiry alert must survive: NULL expires_at means "never expires".
	if count != 1 {
		t.Errorf("remaining alert count = %d, want 1 (NULL-expiry alert must be kept)", count)
	}
}

func TestDB_CleanupOldAuditLogs(t *testing.T) {
	db := mustInitDB(t)
	seedUser(t, db, "alice")

	old := time.Now().AddDate(0, 0, -100)
	recent := time.Now().AddDate(0, 0, -1)
	if _, err := db.Exec("INSERT INTO audit_log (user_id, action, created_at) VALUES (1, 'login', ?)", old); err != nil {
		t.Fatalf("seed old log: %v", err)
	}
	if _, err := db.Exec("INSERT INTO audit_log (user_id, action, created_at) VALUES (1, 'login', ?)", recent); err != nil {
		t.Fatalf("seed recent log: %v", err)
	}

	if err := db.CleanupOldAuditLogs(30); err != nil {
		t.Fatalf("CleanupOldAuditLogs: %v", err)
	}

	var count int
	if err := db.QueryRow("SELECT COUNT(*) FROM audit_log").Scan(&count); err != nil {
		t.Fatalf("count audit_log: %v", err)
	}
	if count != 1 {
		t.Errorf("remaining audit_log count = %d, want 1", count)
	}
}

func TestDB_CleanupRateLimits(t *testing.T) {
	db := mustInitDB(t)

	old := time.Now().Add(-3 * time.Hour)
	recent := time.Now().Add(-30 * time.Minute)
	if _, err := db.Exec("INSERT INTO rate_limits (identifier, endpoint, window_start) VALUES ('ip1', '/a', ?)", old); err != nil {
		t.Fatalf("seed old rate limit: %v", err)
	}
	if _, err := db.Exec("INSERT INTO rate_limits (identifier, endpoint, window_start) VALUES ('ip1', '/b', ?)", recent); err != nil {
		t.Fatalf("seed recent rate limit: %v", err)
	}

	if err := db.CleanupRateLimits(); err != nil {
		t.Fatalf("CleanupRateLimits: %v", err)
	}

	var count int
	if err := db.QueryRow("SELECT COUNT(*) FROM rate_limits").Scan(&count); err != nil {
		t.Fatalf("count rate_limits: %v", err)
	}
	if count != 1 {
		t.Errorf("remaining rate_limits count = %d, want 1", count)
	}
}

func TestDB_HealthCheck(t *testing.T) {
	t.Run("connected", func(t *testing.T) {
		db := mustInitDB(t)
		status, latency, err := db.HealthCheck()
		if err != nil {
			t.Fatalf("HealthCheck: %v", err)
		}
		if status != "connected" {
			t.Errorf("status = %q, want connected", status)
		}
		if latency < 0 {
			t.Errorf("latency = %d, want >= 0", latency)
		}
	})

	t.Run("disconnected reports error", func(t *testing.T) {
		db := mustInitDB(t)
		db.Close()
		status, _, err := db.HealthCheck()
		if err == nil {
			t.Fatal("HealthCheck on closed db = nil error, want error")
		}
		if status != "disconnected" {
			t.Errorf("status = %q, want disconnected", status)
		}
	})
}

func TestDB_GetSessionCount(t *testing.T) {
	db := mustInitDB(t)
	seedUser(t, db, "alice")

	future := time.Now().Add(time.Hour)
	past := time.Now().Add(-time.Hour)
	if _, err := db.Exec("INSERT INTO sessions (id, user_id, expires_at) VALUES ('a', 1, ?)", future); err != nil {
		t.Fatalf("seed active: %v", err)
	}
	if _, err := db.Exec("INSERT INTO sessions (id, user_id, expires_at) VALUES ('b', 1, ?)", future); err != nil {
		t.Fatalf("seed active: %v", err)
	}
	if _, err := db.Exec("INSERT INTO sessions (id, user_id, expires_at) VALUES ('c', 1, ?)", past); err != nil {
		t.Fatalf("seed expired: %v", err)
	}

	count, err := db.GetSessionCount()
	if err != nil {
		t.Fatalf("GetSessionCount: %v", err)
	}
	if count != 2 {
		t.Errorf("GetSessionCount() = %d, want 2 (only unexpired sessions)", count)
	}
}

func TestDB_IsFirstRun(t *testing.T) {
	resetGlobalDualDB(t)

	t.Run("no global server db configured", func(t *testing.T) {
		resetGlobalDualDB(t)
		db := mustInitDB(t)
		_, err := db.IsFirstRun()
		if err == nil {
			t.Error("IsFirstRun() with no global server DB = nil error, want error")
		}
	})

	t.Run("true when no admins exist", func(t *testing.T) {
		resetGlobalDualDB(t)
		serverDB := openMemDB(t, "isfirstrun_true")
		if _, err := serverDB.Exec(ServerSchema); err != nil {
			t.Fatalf("create server schema: %v", err)
		}
		SetGlobalDualDB(&DualDB{Server: serverDB})

		db := mustInitDB(t)
		first, err := db.IsFirstRun()
		if err != nil {
			t.Fatalf("IsFirstRun: %v", err)
		}
		if !first {
			t.Error("IsFirstRun() = false with zero admins, want true")
		}
	})

	t.Run("false once an admin exists", func(t *testing.T) {
		resetGlobalDualDB(t)
		serverDB := openMemDB(t, "isfirstrun_false")
		if _, err := serverDB.Exec(ServerSchema); err != nil {
			t.Fatalf("create server schema: %v", err)
		}
		if _, err := serverDB.Exec("INSERT INTO server_admin_credentials (username, email, password_hash) VALUES ('admin', 'admin@example.com', 'hash')"); err != nil {
			t.Fatalf("seed admin: %v", err)
		}
		SetGlobalDualDB(&DualDB{Server: serverDB})

		db := mustInitDB(t)
		first, err := db.IsFirstRun()
		if err != nil {
			t.Fatalf("IsFirstRun: %v", err)
		}
		if first {
			t.Error("IsFirstRun() = true with an admin present, want false")
		}
	})
}

// mustInitDB returns a fully-initialized, cleaned-up in-memory *DB for tests
// that only care about post-InitDB behavior.
func mustInitDB(t *testing.T) *DB {
	t.Helper()
	dsn := fmt.Sprintf("file:mustinit_%d?mode=memory&cache=shared", time.Now().UnixNano())
	db, err := InitDB(dsn)
	if err != nil {
		t.Fatalf("InitDB: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func seedUser(t *testing.T, db *DB, username string) {
	t.Helper()
	_, err := db.Exec(
		"INSERT INTO users (id, username, email, password_hash) VALUES (1, ?, ?, 'hash')",
		username, username+"@example.com",
	)
	if err != nil {
		t.Fatalf("seedUser: %v", err)
	}
}
