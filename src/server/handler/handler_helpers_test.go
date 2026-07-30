package handler

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"testing"
	"time"

	_ "modernc.org/sqlite"

	"github.com/gin-gonic/gin"

	"github.com/webappsgo/wthr/src/database"
)

// newTestServerDB opens a fresh in-memory SQLite database with the real
// ServerSchema applied, mirroring the DSN-uniqueness convention used in
// src/database/connection_test.go so parallel tests never collide on a
// shared in-memory database name.
func newTestServerDB(t *testing.T) *sql.DB {
	t.Helper()
	dsn := fmt.Sprintf("file:handler_server_%d?mode=memory&cache=shared", time.Now().UnixNano())
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

// newTestUsersDB opens a fresh in-memory SQLite database with the real
// UsersSchema applied.
func newTestUsersDB(t *testing.T) *sql.DB {
	t.Helper()
	dsn := fmt.Sprintf("file:handler_users_%d?mode=memory&cache=shared", time.Now().UnixNano())
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

// newTestDatabaseDB wraps a fresh in-memory server-schema SQLite connection
// in database.DB, the type most handler.* functions expect.
func newTestDatabaseDB(t *testing.T) *database.DB {
	t.Helper()
	return &database.DB{DB: newTestServerDB(t)}
}

// setGlobalTestDualDB wires database.SetGlobalDualDB for handlers/models
// (e.g. UserModel) that read from the package-level DB accessors instead of
// an injected field, and restores the previous global state afterward so
// tests don't leak state into each other.
func setGlobalTestDualDB(t *testing.T, serverDB, usersDB *sql.DB) {
	t.Helper()
	database.SetGlobalDualDB(&database.DualDB{Server: serverDB, Users: usersDB})
	t.Cleanup(func() { database.SetGlobalDualDB(nil) })
}

// newTestContextJSON builds a gin test context with a JSON-encoded request
// body, for handlers that call c.ShouldBindJSON. Passing a raw string as
// body sends it verbatim (useful for malformed-JSON error-path tests);
// any other value is json.Marshal'd.
func newTestContextJSON(t *testing.T, method, target string, body interface{}) (*gin.Context, *httptest.ResponseRecorder) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	var raw []byte
	switch v := body.(type) {
	case string:
		raw = []byte(v)
	case []byte:
		raw = v
	default:
		var err error
		raw, err = json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal request body: %v", err)
		}
	}

	c.Request = httptest.NewRequest(method, target, bytes.NewReader(raw))
	c.Request.Header.Set("Content-Type", "application/json")
	return c, w
}
