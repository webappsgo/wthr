package database

import (
	"testing"
)

// resetGlobalDualDB clears global state before/after each test so tests
// don't leak state into each other via the package-level singleton.
func resetGlobalDualDB(t *testing.T) {
	t.Helper()
	SetGlobalDualDB(nil)
	t.Cleanup(func() { SetGlobalDualDB(nil) })
}

func TestGlobalDualDB_NotSet(t *testing.T) {
	resetGlobalDualDB(t)

	if IsDualMode() {
		t.Error("IsDualMode() = true before SetGlobalDualDB, want false")
	}
	if GetGlobalDualDB() != nil {
		t.Error("GetGlobalDualDB() != nil before SetGlobalDualDB, want nil")
	}
	if GetServerDB() != nil {
		t.Error("GetServerDB() != nil before SetGlobalDualDB, want nil")
	}
	if GetUsersDB() != nil {
		t.Error("GetUsersDB() != nil before SetGlobalDualDB, want nil")
	}
}

func TestGlobalDualDB_SetAndGet(t *testing.T) {
	resetGlobalDualDB(t)

	serverDB := openMemDB(t, "global_server")
	usersDB := openMemDB(t, "global_users")
	ddb := &DualDB{Server: serverDB, Users: usersDB}

	SetGlobalDualDB(ddb)

	if !IsDualMode() {
		t.Error("IsDualMode() = false after SetGlobalDualDB, want true")
	}
	if got := GetGlobalDualDB(); got != ddb {
		t.Errorf("GetGlobalDualDB() = %p, want %p", got, ddb)
	}
	if got := GetServerDB(); got != serverDB {
		t.Errorf("GetServerDB() = %v, want the configured server *sql.DB", got)
	}
	if got := GetUsersDB(); got != usersDB {
		t.Errorf("GetUsersDB() = %v, want the configured users *sql.DB", got)
	}
}

func TestGlobalDualDB_ClearedAfterSetNil(t *testing.T) {
	resetGlobalDualDB(t)

	ddb := &DualDB{Server: openMemDB(t, "global_clear_server"), Users: openMemDB(t, "global_clear_users")}
	SetGlobalDualDB(ddb)
	if !IsDualMode() {
		t.Fatal("expected IsDualMode() true after setting")
	}

	SetGlobalDualDB(nil)

	if IsDualMode() {
		t.Error("IsDualMode() = true after clearing with nil, want false")
	}
	if got := GetServerDB(); got != nil {
		t.Errorf("GetServerDB() = %v after clearing, want nil", got)
	}
}

// TestGlobalDualDB_ConcurrentAccess exercises the RWMutex-guarded globals
// under concurrent readers/writers to catch data races (run with -race).
func TestGlobalDualDB_ConcurrentAccess(t *testing.T) {
	resetGlobalDualDB(t)

	ddb := &DualDB{Server: openMemDB(t, "global_race_server"), Users: openMemDB(t, "global_race_users")}

	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := 0; i < 100; i++ {
			SetGlobalDualDB(ddb)
			SetGlobalDualDB(nil)
		}
	}()

	for i := 0; i < 100; i++ {
		_ = IsDualMode()
		_ = GetGlobalDualDB()
		_ = GetServerDB()
		_ = GetUsersDB()
	}
	<-done
}
