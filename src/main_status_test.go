package main

import (
	"database/sql"
	"fmt"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	_ "modernc.org/sqlite"

	"github.com/webappsgo/wthr/src/database"
)

// newStatusTestServerDB opens a fresh in-memory SQLite database standing in
// for server.db. showServerStatus only ever runs "SELECT 1" against it via
// HealthCheck, so no schema needs to be applied.
func newStatusTestServerDB(t *testing.T) *sql.DB {
	t.Helper()
	dsn := fmt.Sprintf("file:main_status_server_%d?mode=memory&cache=shared", time.Now().UnixNano())
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		t.Fatalf("open server db: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

// newStatusTestUsersDB opens a fresh in-memory SQLite database with the real
// UsersSchema applied, mirroring server/handler/handler_helpers_test.go's
// newTestUsersDB. showServerStatus queries user_accounts, user_tokens, and
// user_saved_locations directly through database.GetUsersDB(), so those
// tables must exist.
func newStatusTestUsersDB(t *testing.T) *sql.DB {
	t.Helper()
	dsn := fmt.Sprintf("file:main_status_users_%d?mode=memory&cache=shared", time.Now().UnixNano())
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		t.Fatalf("open users db: %v", err)
	}
	if _, err := db.Exec(database.UsersSchema); err != nil {
		db.Close()
		t.Fatalf("apply UsersSchema: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

// wireStatusTestDualDB registers serverDB/usersDB as the package-level
// database.DualDB that showServerStatus reads via database.GetUsersDB(),
// and restores the previous global state afterward so tests don't leak
// state into each other (same convention as
// server/handler/handler_helpers_test.go's setGlobalTestDualDB).
func wireStatusTestDualDB(t *testing.T, serverDB, usersDB *sql.DB) {
	t.Helper()
	database.SetGlobalDualDB(&database.DualDB{Server: serverDB, Users: usersDB})
	t.Cleanup(func() { database.SetGlobalDualDB(nil) })
}

// captureStatusStdout redirects os.Stdout to a pipe for the duration of fn
// and returns everything written to it, following the convention already
// established in common/banner/banner_test.go.
func captureStatusStdout(t *testing.T, fn func()) string {
	t.Helper()

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe() error: %v", err)
	}
	orig := os.Stdout
	os.Stdout = w
	defer func() { os.Stdout = orig }()

	fn()

	if err := w.Close(); err != nil {
		t.Fatalf("pipe close error: %v", err)
	}
	out, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("pipe read error: %v", err)
	}
	return string(out)
}

// TestShowServerStatus_Healthy verifies the health-check happy path: a live
// server DB connection makes HealthCheck's "SELECT 1" succeed, so the
// function must report healthy (return true) and print "connected" status.
func TestShowServerStatus_Healthy(t *testing.T) {
	serverDB := newStatusTestServerDB(t)
	usersDB := newStatusTestUsersDB(t)
	wireStatusTestDualDB(t, serverDB, usersDB)

	db := &database.DB{DB: serverDB}

	var got bool
	out := captureStatusStdout(t, func() {
		got = showServerStatus(db, "/tmp/wthr-status-test/server.db", false)
	})

	if !got {
		t.Fatalf("showServerStatus() = false, want true for a healthy in-memory DB")
	}
	if !strings.Contains(out, "OK: Healthy") {
		t.Errorf("output missing healthy status banner, got:\n%s", out)
	}
	if !strings.Contains(out, "Status:         connected") {
		t.Errorf("output missing connected DB status, got:\n%s", out)
	}
	if !strings.Contains(out, "First Run:      false") {
		t.Errorf("output did not reflect isFirstRun=false, got:\n%s", out)
	}
}

// TestShowServerStatus_UnhealthyOnClosedDB verifies the health-check failure
// path: closing the server DB before the call makes HealthCheck's
// "SELECT 1" fail, so the function must report unhealthy (return false) and
// print the disconnected status rather than crash.
func TestShowServerStatus_UnhealthyOnClosedDB(t *testing.T) {
	serverDB := newStatusTestServerDB(t)
	usersDB := newStatusTestUsersDB(t)
	wireStatusTestDualDB(t, serverDB, usersDB)

	db := &database.DB{DB: serverDB}
	if err := serverDB.Close(); err != nil {
		t.Fatalf("close server db: %v", err)
	}

	var got bool
	out := captureStatusStdout(t, func() {
		got = showServerStatus(db, "/tmp/wthr-status-test/server.db", true)
	})

	if got {
		t.Fatalf("showServerStatus() = true, want false when the DB connection is closed")
	}
	if !strings.Contains(out, "Unhealthy (Database Error)") {
		t.Errorf("output missing unhealthy status banner, got:\n%s", out)
	}
	if !strings.Contains(out, "First Run:      true") {
		t.Errorf("output did not reflect isFirstRun=true, got:\n%s", out)
	}
}

// TestShowServerStatus_CountsReflectRows verifies the three COUNT(*) queries
// against the users DB actually surface real row counts in the printed
// output, not just static zeros.
func TestShowServerStatus_CountsReflectRows(t *testing.T) {
	serverDB := newStatusTestServerDB(t)
	usersDB := newStatusTestUsersDB(t)
	wireStatusTestDualDB(t, serverDB, usersDB)

	if _, err := usersDB.Exec(
		`INSERT INTO user_accounts (username, email, password_hash) VALUES (?, ?, ?)`,
		"statususer", "status@example.test", "argon2id$fakehash",
	); err != nil {
		t.Fatalf("insert user_accounts row: %v", err)
	}

	db := &database.DB{DB: serverDB}

	out := captureStatusStdout(t, func() {
		showServerStatus(db, "/tmp/wthr-status-test/server.db", false)
	})

	if !strings.Contains(out, "Users:          1") {
		t.Errorf("output did not reflect inserted user row, got:\n%s", out)
	}
	if !strings.Contains(out, "Locations:      0") {
		t.Errorf("output did not reflect zero saved locations, got:\n%s", out)
	}
	if !strings.Contains(out, "Active Tokens:  0") {
		t.Errorf("output did not reflect zero active tokens, got:\n%s", out)
	}
}

// TestShowServerStatus_ReverseProxyAddressMode verifies the REVERSE_PROXY
// env var switches the displayed listen address from the dual-stack default
// to the documented reverse-proxy loopback address.
func TestShowServerStatus_ReverseProxyAddressMode(t *testing.T) {
	serverDB := newStatusTestServerDB(t)
	usersDB := newStatusTestUsersDB(t)
	wireStatusTestDualDB(t, serverDB, usersDB)

	t.Setenv("LISTEN", "")
	t.Setenv("SERVER_ADDRESS", "")
	t.Setenv("REVERSE_PROXY", "true")
	t.Setenv("PORT", "4242")

	db := &database.DB{DB: serverDB}

	out := captureStatusStdout(t, func() {
		showServerStatus(db, "/tmp/wthr-status-test/server.db", false)
	})

	if !strings.Contains(out, "Listen Address: 127.0.0.1:4242 (reverse proxy mode)") {
		t.Errorf("output missing reverse-proxy listen address line, got:\n%s", out)
	}
}

// TestShowServerStatus_DefaultAddressMode verifies that with no LISTEN,
// SERVER_ADDRESS, or REVERSE_PROXY env vars set, the function falls back to
// the dual-stack "all interfaces" default rather than a reverse-proxy or
// custom address.
func TestShowServerStatus_DefaultAddressMode(t *testing.T) {
	serverDB := newStatusTestServerDB(t)
	usersDB := newStatusTestUsersDB(t)
	wireStatusTestDualDB(t, serverDB, usersDB)

	t.Setenv("LISTEN", "")
	t.Setenv("SERVER_ADDRESS", "")
	t.Setenv("REVERSE_PROXY", "")
	t.Setenv("PORT", "")

	db := &database.DB{DB: serverDB}

	out := captureStatusStdout(t, func() {
		showServerStatus(db, "/tmp/wthr-status-test/server.db", false)
	})

	if !strings.Contains(out, "Listen Address: :::3000 (all interfaces)") {
		t.Errorf("output missing default all-interfaces listen address line, got:\n%s", out)
	}
}

// TestShowServerStatus_ExplicitListenAddress verifies an explicit LISTEN
// value is used verbatim, with no addressMode annotation appended.
func TestShowServerStatus_ExplicitListenAddress(t *testing.T) {
	serverDB := newStatusTestServerDB(t)
	usersDB := newStatusTestUsersDB(t)
	wireStatusTestDualDB(t, serverDB, usersDB)

	t.Setenv("LISTEN", "192.0.2.10")
	t.Setenv("SERVER_ADDRESS", "")
	t.Setenv("REVERSE_PROXY", "true")
	t.Setenv("PORT", "9000")

	db := &database.DB{DB: serverDB}

	out := captureStatusStdout(t, func() {
		showServerStatus(db, "/tmp/wthr-status-test/server.db", false)
	})

	// An explicit LISTEN value must win over REVERSE_PROXY and be printed
	// with no addressMode suffix at all.
	if !strings.Contains(out, "Listen Address: 192.0.2.10:9000\n") {
		t.Errorf("output missing explicit listen address line, got:\n%s", out)
	}
}

// TestShowServerStatus_EnvironmentModeFallback verifies the legacy
// ENVIRONMENT env var is used when MODE is unset, and that MODE takes
// priority when both are set.
func TestShowServerStatus_EnvironmentModeFallback(t *testing.T) {
	tests := []struct {
		name        string
		mode        string
		environment string
		want        string
	}{
		{"MODE set directly", "staging", "", "staging"},
		{"ENVIRONMENT legacy fallback", "", "legacy-mode", "legacy-mode"},
		{"MODE takes priority over ENVIRONMENT", "staging", "legacy-mode", "staging"},
		{"neither set defaults to production", "", "", "production"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			serverDB := newStatusTestServerDB(t)
			usersDB := newStatusTestUsersDB(t)
			wireStatusTestDualDB(t, serverDB, usersDB)

			t.Setenv("MODE", tt.mode)
			t.Setenv("ENVIRONMENT", tt.environment)

			db := &database.DB{DB: serverDB}

			out := captureStatusStdout(t, func() {
				showServerStatus(db, "/tmp/wthr-status-test/server.db", false)
			})

			if !strings.Contains(out, "Environment:    "+tt.want) {
				t.Errorf("output missing expected environment line %q, got:\n%s", tt.want, out)
			}
		})
	}
}
