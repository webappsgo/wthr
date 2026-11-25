package database

import (
	"database/sql"
	"sync"
)

// Global database instances for handler access
var (
	globalDualDB *DualDB
	globalMutex  sync.RWMutex
)

// SetGlobalDualDB sets the global dual database instance
func SetGlobalDualDB(ddb *DualDB) {
	globalMutex.Lock()
	defer globalMutex.Unlock()
	if ddb != nil {
		if ddb.serverStore == nil && ddb.Server != nil {
			ddb.serverStore = NewStore(ddb.Server)
		}
		if ddb.usersStore == nil && ddb.Users != nil {
			ddb.usersStore = NewStore(ddb.Users)
		}
	}
	globalDualDB = ddb
}

// GetGlobalDualDB returns the global dual database instance
func GetGlobalDualDB() *DualDB {
	globalMutex.RLock()
	defer globalMutex.RUnlock()
	return globalDualDB
}

// GetServerDB returns the server database connection (server.db in SQLite mode)
// Returns nil if not using dual database mode
func GetServerDB() *sql.DB {
	globalMutex.RLock()
	defer globalMutex.RUnlock()
	if globalDualDB != nil {
		return globalDualDB.Server
	}
	return nil
}

// GetUsersDB returns the users database connection (users.db in SQLite mode)
// Returns nil if not using dual database mode
func GetUsersDB() *sql.DB {
	globalMutex.RLock()
	defer globalMutex.RUnlock()
	if globalDualDB != nil {
		return globalDualDB.Users
	}
	return nil
}

// GetServerStore returns the server SQLite store when local SQLite is active.
func GetServerStore() *Store {
	globalMutex.RLock()
	defer globalMutex.RUnlock()
	if globalDualDB != nil {
		if globalDualDB.serverStore != nil {
			return globalDualDB.serverStore
		}
		return NewStore(globalDualDB.Server)
	}
	return nil
}

// GetUsersStore returns the users SQLite store when local SQLite is active.
func GetUsersStore() *Store {
	globalMutex.RLock()
	defer globalMutex.RUnlock()
	if globalDualDB != nil {
		if globalDualDB.usersStore != nil {
			return globalDualDB.usersStore
		}
		return NewStore(globalDualDB.Users)
	}
	return nil
}

// IsDualMode returns true if running in dual database mode (SQLite standalone)
func IsDualMode() bool {
	globalMutex.RLock()
	defer globalMutex.RUnlock()
	return globalDualDB != nil
}

// UsersDBOr returns the process-global users.db handle, or fallback when no
// dual database is wired (single-handle unit tests). Server Admin accounts live
// in users.db (AI.md PART 12) while admin sessions live in server.db, so code
// that was handed one handle uses this to reach the other side.
func UsersDBOr(fallback *sql.DB) *sql.DB {
	if users := GetUsersDB(); users != nil {
		return users
	}
	return fallback
}

// ServerDBOr returns the process-global server.db handle, or fallback when no
// dual database is wired (single-handle unit tests).
func ServerDBOr(fallback *sql.DB) *sql.DB {
	if server := GetServerDB(); server != nil {
		return server
	}
	return fallback
}

// UsersStoreOr returns the process-global users store, or fallback when no
// dual database is wired.
func UsersStoreOr(fallback *Store) *Store {
	if users := GetUsersStore(); users != nil {
		return users
	}
	return fallback
}

// ServerStoreOr returns the process-global server store, or fallback when no
// dual database is wired.
func ServerStoreOr(fallback *Store) *Store {
	if server := GetServerStore(); server != nil {
		return server
	}
	return fallback
}
