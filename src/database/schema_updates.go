package database

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"
)

// AI.md PART 10 Connection Pooling: small-deployment defaults.
const (
	poolMaxOpen     = 25
	poolMaxIdle     = 5
	poolMaxLifetime = 5 * time.Minute
	poolMaxIdleTime = time.Minute
)

// configurePool applies the AI.md PART 10 pool settings to db. Every database
// connection must be pooled, including the idle-time bound.
func configurePool(db *sql.DB) {
	db.SetMaxOpenConns(poolMaxOpen)
	db.SetMaxIdleConns(poolMaxIdle)
	db.SetConnMaxLifetime(poolMaxLifetime)
	db.SetConnMaxIdleTime(poolMaxIdleTime)
}

// serverSchemaUpdates holds idempotent schema updates applied to server.db on
// every startup. AI.md PART 10 forbids migration files and version tracking:
// the base ServerSchema creates every table, and each statement here brings an
// older database forward without dropping or renaming anything.
var serverSchemaUpdates = []string{}

// usersSchemaUpdates holds idempotent schema updates applied to users.db on
// every startup. Columns added here also exist in the base UsersSchema, so on a
// fresh database every statement is a no-op that reports "duplicate column".
var usersSchemaUpdates = []string{
	// Session 2FA pending-state storage
	`ALTER TABLE user_sessions ADD COLUMN data TEXT`,

	// Hashed session tokens replaced the original plaintext session_id column.
	// The old column is left in place and unused per AI.md PART 10 (add new,
	// never drop), and rows written before the change are never readable
	// because lookups now match on the SHA-256 hash only.
	`ALTER TABLE user_sessions ADD COLUMN token_hash TEXT`,
	`CREATE UNIQUE INDEX IF NOT EXISTS idx_sessions_hash ON user_sessions(token_hash)`,

	// Invite metadata for PART 34 invite-only registration
	`ALTER TABLE user_invites ADD COLUMN username TEXT`,
	`ALTER TABLE user_invites ADD COLUMN role TEXT NOT NULL DEFAULT 'user'`,
}

// applySchemaUpdates runs every statement in updates against db, ignoring
// errors that only mean the column or index is already present. AI.md PART 10:
// schema/DDL statements use the Migration timeout tier.
func applySchemaUpdates(db *sql.DB, updates []string) error {
	ctx := context.Background()

	for _, stmt := range updates {
		if _, err := ExecContext(ctx, db, TimeoutMigration, stmt); err != nil {
			if isColumnExistsError(err) {
				continue
			}
			return fmt.Errorf("schema update %q: %w", stmt, err)
		}
	}

	return nil
}

// isColumnExistsError reports whether err is a driver complaint that the object
// a schema update adds is already present. SQLite, PostgreSQL and MySQL each
// word this differently, so every known phrasing is matched.
func isColumnExistsError(err error) bool {
	if err == nil {
		return false
	}

	msg := strings.ToLower(err.Error())

	return strings.Contains(msg, "duplicate column") ||
		strings.Contains(msg, "already exists") ||
		strings.Contains(msg, "duplicate key name")
}
