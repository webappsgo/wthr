package models

import (
	"database/sql"
	"fmt"
	"sync/atomic"
	"testing"

	_ "modernc.org/sqlite"

	"github.com/webappsgo/wthr/src/database"
)

// modelDBCounter guarantees a unique in-memory SQLite DSN per test, even
// when tests run in parallel, so the shared-cache in-memory databases never
// collide with each other or with notification_test.go's own counter.
var modelDBCounter int64

// newModelUsersDB opens a fresh in-memory SQLite database with the real
// production UsersSchema applied.
func newModelUsersDB(t *testing.T) *sql.DB {
	t.Helper()
	n := atomic.AddInt64(&modelDBCounter, 1)
	dsn := fmt.Sprintf("file:model_users_%d?mode=memory&cache=shared", n)
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

// newModelServerDB opens a fresh in-memory SQLite database with the real
// production ServerSchema applied.
func newModelServerDB(t *testing.T) *sql.DB {
	t.Helper()
	n := atomic.AddInt64(&modelDBCounter, 1)
	dsn := fmt.Sprintf("file:model_server_%d?mode=memory&cache=shared", n)
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		t.Fatalf("open server db: %v", err)
	}
	if _, err := db.Exec(database.ServerSchema); err != nil {
		db.Close()
		t.Fatalf("apply ServerSchema: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

// setModelGlobalDualDB wires database.SetGlobalDualDB for models that read
// from the package-level DB accessors (database.GetUsersDB / GetServerDB)
// instead of an injected field, and restores nil afterward so tests don't
// leak global state into each other.
func setModelGlobalDualDB(t *testing.T, serverDB, usersDB *sql.DB) {
	t.Helper()
	database.SetGlobalDualDB(&database.DualDB{Server: serverDB, Users: usersDB})
	t.Cleanup(func() { database.SetGlobalDualDB(nil) })
}

// insertTestUser inserts a minimal user_accounts row directly (bypassing
// UserModel.Create) for tests that only need a user to exist as a foreign
// key target. Returns the new row ID.
func insertTestUser(t *testing.T, db *sql.DB, username, email string) int64 {
	t.Helper()
	res, err := db.Exec(`
		INSERT INTO user_accounts (username, email, password_hash)
		VALUES (?, ?, 'x')
	`, username, email)
	if err != nil {
		t.Fatalf("insert test user: %v", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		t.Fatalf("get inserted user id: %v", err)
	}
	return id
}
