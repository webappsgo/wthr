package database

import (
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
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to connect to server database: %w", err)
	}

	// Enable SQLite optimizations
	if _, err := db.Exec("PRAGMA foreign_keys = ON"); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to enable foreign keys: %w", err)
	}
	if _, err := db.Exec("PRAGMA journal_mode = WAL"); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to enable WAL mode: %w", err)
	}

	// Create server schema
	if _, err := db.Exec(ServerSchema); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to create server schema: %w", err)
	}

	// Check and initialize schema version
	var currentVersion int
	err = db.QueryRow("SELECT COALESCE(MAX(version), 0) FROM schema_version").Scan(&currentVersion)
	if err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to check schema version: %w", err)
	}

	if currentVersion == 0 {
		// New database - insert schema version
		if _, err := db.Exec("INSERT INTO schema_version (version) VALUES (?)", ServerSchemaVersion); err != nil {
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
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to connect to users database: %w", err)
	}

	// Enable SQLite optimizations
	if _, err := db.Exec("PRAGMA foreign_keys = ON"); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to enable foreign keys: %w", err)
	}
	if _, err := db.Exec("PRAGMA journal_mode = WAL"); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to enable WAL mode: %w", err)
	}

	// Create users schema
	if _, err := db.Exec(UsersSchema); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to create users schema: %w", err)
	}

	// Check and initialize schema version
	var currentVersion int
	err = db.QueryRow("SELECT COALESCE(MAX(version), 0) FROM schema_version").Scan(&currentVersion)
	if err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to check schema version: %w", err)
	}

	if currentVersion == 0 {
		// New database - insert schema version
		if _, err := db.Exec("INSERT INTO schema_version (version) VALUES (?)", UsersSchemaVersion); err != nil {
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
		if _, err := db.Exec("INSERT INTO schema_version (version) VALUES (?)", UsersSchemaVersion); err != nil {
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
	// v6: add data column to user_sessions for 2FA pending state storage
	if fromVersion < 6 {
		// SQLite: ignore "duplicate column" error — idempotent
		if _, err := db.Exec("ALTER TABLE user_sessions ADD COLUMN data TEXT"); err != nil {
			if !strings.Contains(err.Error(), "duplicate column") {
				return fmt.Errorf("add user_sessions.data: %w", err)
			}
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
	if err := ddb.Server.Ping(); err != nil {
		return "error", 0, fmt.Errorf("server database unhealthy: %w", err)
	}

	// Check users database
	if err := ddb.Users.Ping(); err != nil {
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

// QueryServer executes a query on the server database
func (ddb *DualDB) QueryServer(query string, args ...interface{}) (*sql.Rows, error) {
	return ddb.Server.Query(query, args...)
}

// QueryRowServer executes a query on the server database and returns a single row
func (ddb *DualDB) QueryRowServer(query string, args ...interface{}) *sql.Row {
	return ddb.Server.QueryRow(query, args...)
}

// ExecServer executes a statement on the server database
func (ddb *DualDB) ExecServer(query string, args ...interface{}) (sql.Result, error) {
	return ddb.Server.Exec(query, args...)
}

// QueryUsers executes a query on the users database
func (ddb *DualDB) QueryUsers(query string, args ...interface{}) (*sql.Rows, error) {
	return ddb.Users.Query(query, args...)
}

// QueryRowUsers executes a query on the users database and returns a single row
func (ddb *DualDB) QueryRowUsers(query string, args ...interface{}) *sql.Row {
	return ddb.Users.QueryRow(query, args...)
}

// ExecUsers executes a statement on the users database
func (ddb *DualDB) ExecUsers(query string, args ...interface{}) (sql.Result, error) {
	return ddb.Users.Exec(query, args...)
}
