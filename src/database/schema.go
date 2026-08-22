package database

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/webappsgo/wthr/src/common/dbtime"

	_ "modernc.org/sqlite"
)

// DB represents the database connection
type DB struct {
	*sql.DB
}

// IsFirstRun checks if this is the first run (no server admins exist)
// Per AI.md: Server Admins are in server_admin_credentials table (server DB)
func (db *DB) IsFirstRun() (bool, error) {
	// Server admin credentials are in the server database, not users database
	serverDB := GetServerDB()
	if serverDB == nil {
		return false, fmt.Errorf("server database not initialized")
	}

	var count int
	err := QueryRowContext(context.Background(), serverDB, TimeoutSimpleSelect, "SELECT COUNT(*) FROM server_admin_credentials").Scan(&count)
	if err != nil {
		return false, err
	}
	return count == 0, nil
}

// HealthCheck returns database health status with latency
func (db *DB) HealthCheck() (status string, latencyMs int64, err error) {
	start := time.Now()

	// Simple query to check connection
	var result int
	err = QueryRowContext(context.Background(), db.DB, TimeoutSimpleSelect, "SELECT 1").Scan(&result)

	latencyMs = time.Since(start).Milliseconds()

	if err != nil {
		return "disconnected", latencyMs, err
	}

	return "connected", latencyMs, nil
}

// GetSessionCount returns the number of currently active sessions across both
// session tables the live schema actually creates: user_sessions in the users
// database (UsersSchema) and server_admin_sessions in the server database
// (ServerSchema). It reads those handles through GetUsersDB/GetServerDB rather
// than the receiver's own connection because a *DB may be bound to either one,
// and an active-session total that silently omitted half the sessions would be
// worse than no number at all.
//
// A missing database handle contributes zero rather than failing the whole
// count, so a single-database deployment still reports the sessions it does
// have.
func (db *DB) GetSessionCount() (int, error) {
	total := 0

	userCount, err := countActiveSessions(GetUsersDB(), "user_sessions")
	if err != nil {
		return 0, err
	}
	total += userCount

	adminCount, err := countActiveSessions(GetServerDB(), "server_admin_sessions")
	if err != nil {
		return 0, err
	}
	total += adminCount

	return total, nil
}

// countActiveSessions counts rows in table whose expires_at is still in the
// future. The comparison is done in Go rather than SQL: a SQL-side
// datetime(expires_at) > datetime('now') comparison yields NULL for any row
// whose stored text is not in the canonical layout, which makes the predicate
// silently never match, and a raw text comparison orders by wall clock instead
// of by instant. A value that cannot be parsed is counted as expired, so the
// total fails closed rather than inflating.
//
// table is always a compile-time literal supplied by this package, never user
// input, which is why it can be interpolated - a table name cannot be bound as
// a query parameter.
func countActiveSessions(db *sql.DB, table string) (int, error) {
	if db == nil {
		return 0, nil
	}

	rows, err := QueryContext(context.Background(), db, TimeoutSimpleSelect, "SELECT expires_at FROM "+table)
	if err != nil {
		return 0, err
	}
	defer rows.Close()

	now := time.Now()
	count := 0
	for rows.Next() {
		var expiresAt interface{}
		if err := rows.Scan(&expiresAt); err != nil {
			return 0, err
		}
		if dbtime.IsAfter(expiresAt, now) {
			count++
		}
	}
	if err := rows.Err(); err != nil {
		return 0, err
	}

	return count, nil
}
