package database

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

// DualDB holds connections to both server and users databases per TEMPLATE.md PART 31
type DualDB struct {
	// Server database (server.db) - admin credentials, config, scheduler, etc.
	Server *sql.DB

	// Users database (users.db) - user accounts, tokens, sessions, etc.
	Users *sql.DB
}

// InitDualDB initializes both server and users databases
// AI.md: Database paths must be {data_dir}/db/server.db and {data_dir}/db/users.db
func InitDualDB(dataDir string) (*DualDB, error) {
	if dataDir == "" {
		dataDir = "./data"
	}

	// AI.md PART 24: Databases go in {data_dir}/db/ subdirectory
	dbDir := filepath.Join(dataDir, "db")
	if err := os.MkdirAll(dbDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create database directory: %w", err)
	}

	serverDBPath := filepath.Join(dbDir, "server.db")
	usersDBPath := filepath.Join(dbDir, "users.db")

	log.Printf("Initializing dual databases:")
	log.Printf("  Server DB: %s", serverDBPath)
	log.Printf("  Users DB:  %s", usersDBPath)

	// Initialize server database
	serverDB, err := initServerDB(serverDBPath)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize server database: %w", err)
	}

	// Initialize users database
	usersDB, err := initUsersDB(usersDBPath)
	if err != nil {
		serverDB.Close()
		return nil, fmt.Errorf("failed to initialize users database: %w", err)
	}

	return &DualDB{
		Server: serverDB,
		Users:  usersDB,
	}, nil
}

// initServerDB initializes the server database with server schema
func initServerDB(path string) (*sql.DB, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("failed to open server database: %w", err)
	}

	// Set connection parameters
	db.SetConnMaxLifetime(time.Minute * 3)
	db.SetMaxOpenConns(10)
	db.SetMaxIdleConns(5)

	// Test connection
	if err := PingWithTimeout(db); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to connect to server database: %w", err)
	}

	// Enable SQLite optimizations
	// AI.md PART 10: schema/DDL statements use the Migration timeout tier (5m)
	if _, err := ExecContext(context.Background(), db, TimeoutMigration, "PRAGMA foreign_keys = ON"); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to enable foreign keys: %w", err)
	}
	if _, err := ExecContext(context.Background(), db, TimeoutMigration, "PRAGMA journal_mode = WAL"); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to enable WAL mode: %w", err)
	}

	// Create server schema
	if _, err := ExecContext(context.Background(), db, TimeoutMigration, ServerSchema); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to create server schema: %w", err)
	}

	// Check and initialize schema version
	var currentVersion int
	err = QueryRowContext(context.Background(), db, TimeoutSimpleSelect, "SELECT COALESCE(MAX(version), 0) FROM schema_version").Scan(&currentVersion)
	if err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to check schema version: %w", err)
	}

	if currentVersion == 0 {
		// New database - insert schema version
		if _, err := ExecContext(context.Background(), db, TimeoutMigration, "INSERT INTO schema_version (version) VALUES (?)", ServerSchemaVersion); err != nil {
			db.Close()
			return nil, fmt.Errorf("failed to insert schema version: %w", err)
		}
		log.Printf("Server database initialized with schema version %d", ServerSchemaVersion)
	}

	return db, nil
}

// initUsersDB initializes the users database with users schema
func initUsersDB(path string) (*sql.DB, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("failed to open users database: %w", err)
	}

	// Set connection parameters
	db.SetConnMaxLifetime(time.Minute * 3)
	db.SetMaxOpenConns(10)
	db.SetMaxIdleConns(5)

	// Test connection
	if err := PingWithTimeout(db); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to connect to users database: %w", err)
	}

	// Enable SQLite optimizations
	// AI.md PART 10: schema/DDL statements use the Migration timeout tier (5m)
	if _, err := ExecContext(context.Background(), db, TimeoutMigration, "PRAGMA foreign_keys = ON"); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to enable foreign keys: %w", err)
	}
	if _, err := ExecContext(context.Background(), db, TimeoutMigration, "PRAGMA journal_mode = WAL"); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to enable WAL mode: %w", err)
	}

	// Create users schema
	if _, err := ExecContext(context.Background(), db, TimeoutMigration, UsersSchema); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to create users schema: %w", err)
	}

	// Check and initialize schema version
	var currentVersion int
	err = QueryRowContext(context.Background(), db, TimeoutSimpleSelect, "SELECT COALESCE(MAX(version), 0) FROM schema_version").Scan(&currentVersion)
	if err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to check schema version: %w", err)
	}

	if currentVersion == 0 {
		// New database - insert schema version
		if _, err := ExecContext(context.Background(), db, TimeoutMigration, "INSERT INTO schema_version (version) VALUES (?)", UsersSchemaVersion); err != nil {
			db.Close()
			return nil, fmt.Errorf("failed to insert schema version: %w", err)
		}
		log.Printf("Users database initialized with schema version %d", UsersSchemaVersion)
	} else if currentVersion < UsersSchemaVersion {
		// Existing database - apply idempotent migrations
		if err := migrateUsersDB(db, currentVersion); err != nil {
			db.Close()
			return nil, fmt.Errorf("failed to migrate users database: %w", err)
		}
		if _, err := ExecContext(context.Background(), db, TimeoutMigration, "INSERT INTO schema_version (version) VALUES (?)", UsersSchemaVersion); err != nil {
			db.Close()
			return nil, fmt.Errorf("failed to update users schema version: %w", err)
		}
		log.Printf("Users database migrated from version %d to %d", currentVersion, UsersSchemaVersion)
	}

	return db, nil
}

// migrateUsersDB applies idempotent schema migrations for the users database.
// Each migration is safe to run on databases that already have the column.
func migrateUsersDB(db *sql.DB, fromVersion int) error {
	// AI.md PART 10: schema/DDL statements use the Migration timeout tier (5m)
	ctx := context.Background()

	// v6: add data column to user_sessions for 2FA pending state storage
	if fromVersion < 6 {
		// SQLite: ignore "duplicate column" error — idempotent
		if _, err := ExecContext(ctx, db, TimeoutMigration, "ALTER TABLE user_sessions ADD COLUMN data TEXT"); err != nil {
			if !strings.Contains(err.Error(), "duplicate column") {
				return fmt.Errorf("add user_sessions.data: %w", err)
			}
		}
	}

	// v7: rename session_id to token_hash + clear old unhashed sessions.
	// Sessions stored before this migration contain raw tokens and cannot be
	// retroactively hashed, so we delete them — existing users must log in
	// again. This is acceptable for a security fix (raw→hashed storage).
	if fromVersion < 7 {
		// Invalidate all old sessions first so no raw-token rows remain.
		if _, err := ExecContext(ctx, db, TimeoutMigration, "DELETE FROM user_sessions"); err != nil {
			return fmt.Errorf("clear user_sessions for v7 migration: %w", err)
		}
		// RENAME COLUMN requires SQLite ≥3.25 (2018); modernc.org/sqlite always satisfies this.
		if _, err := ExecContext(ctx, db, TimeoutMigration, "ALTER TABLE user_sessions RENAME COLUMN session_id TO token_hash"); err != nil {
			// If the column is already named token_hash (already migrated or new DB), ignore.
			if !strings.Contains(err.Error(), "no such column") && !strings.Contains(err.Error(), "duplicate column") {
				return fmt.Errorf("rename user_sessions.session_id to token_hash: %w", err)
			}
		}
		// Add a unique index on the new column name if it doesn't already exist.
		if _, err := ExecContext(ctx, db, TimeoutMigration, "CREATE UNIQUE INDEX IF NOT EXISTS idx_sessions_hash ON user_sessions(token_hash)"); err != nil {
			return fmt.Errorf("create idx_sessions_hash: %w", err)
		}
		// Drop the old index if it was named for session_id.
		if _, err := ExecContext(ctx, db, TimeoutMigration, "DROP INDEX IF EXISTS idx_sessions_id"); err != nil {
			return fmt.Errorf("drop idx_sessions_id: %w", err)
		}
	}
	return nil
}

// Close closes both database connections
func (ddb *DualDB) Close() error {
	var errs []string

	if ddb.Server != nil {
		if err := ddb.Server.Close(); err != nil {
			errs = append(errs, fmt.Sprintf("server db: %v", err))
		}
	}

	if ddb.Users != nil {
		if err := ddb.Users.Close(); err != nil {
			errs = append(errs, fmt.Sprintf("users db: %v", err))
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("failed to close databases: %s", strings.Join(errs, "; "))
	}

	return nil
}

// HealthCheck checks health of both databases
func (ddb *DualDB) HealthCheck() (string, int64, error) {
	start := time.Now()

	// Check server database
	if err := PingWithTimeout(ddb.Server); err != nil {
		return "error", 0, fmt.Errorf("server database unhealthy: %w", err)
	}

	// Check users database
	if err := PingWithTimeout(ddb.Users); err != nil {
		return "error", 0, fmt.Errorf("users database unhealthy: %w", err)
	}

	latency := time.Since(start).Milliseconds()
	return "connected", latency, nil
}

// GetServerDB returns the server database connection
func (ddb *DualDB) GetServerDB() *sql.DB {
	return ddb.Server
}

// GetUsersDB returns the users database connection
func (ddb *DualDB) GetUsersDB() *sql.DB {
	return ddb.Users
}

// QueryServer executes a query on the server database.
// AI.md PART 10: callers pass arbitrary SELECTs (including JOINs), so the
// Complex Select timeout tier (15s) is used here.
func (ddb *DualDB) QueryServer(query string, args ...interface{}) (*sql.Rows, error) {
	return QueryContext(context.Background(), ddb.Server, TimeoutComplexSelect, query, args...)
}

// QueryRowServer executes a query on the server database and returns a single
// row. AI.md PART 10: Simple Select timeout tier (5s).
func (ddb *DualDB) QueryRowServer(query string, args ...interface{}) *sql.Row {
	return QueryRowContext(context.Background(), ddb.Server, TimeoutSimpleSelect, query, args...)
}

// ExecServer executes a statement on the server database.
// AI.md PART 10: Write timeout tier (10s).
func (ddb *DualDB) ExecServer(query string, args ...interface{}) (sql.Result, error) {
	return ExecContext(context.Background(), ddb.Server, TimeoutWrite, query, args...)
}

// QueryUsers executes a query on the users database.
// AI.md PART 10: Complex Select timeout tier (15s).
func (ddb *DualDB) QueryUsers(query string, args ...interface{}) (*sql.Rows, error) {
	return QueryContext(context.Background(), ddb.Users, TimeoutComplexSelect, query, args...)
}

// QueryRowUsers executes a query on the users database and returns a single
// row. AI.md PART 10: Simple Select timeout tier (5s).
func (ddb *DualDB) QueryRowUsers(query string, args ...interface{}) *sql.Row {
	return QueryRowContext(context.Background(), ddb.Users, TimeoutSimpleSelect, query, args...)
}

// ExecUsers executes a statement on the users database.
// AI.md PART 10: Write timeout tier (10s).
func (ddb *DualDB) ExecUsers(query string, args ...interface{}) (sql.Result, error) {
	return ExecContext(context.Background(), ddb.Users, TimeoutWrite, query, args...)
}
