package database

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/webappsgo/wthr/src/config"
	_ "modernc.org/sqlite"
)

// DualDB holds connections to both server and users databases per TEMPLATE.md PART 31
type DualDB struct {
	// Server database (server.db) - admin sessions, config, scheduler, audit log, etc.
	Server *sql.DB

	// Users database (users.db) - Server Admins, user accounts, tokens, sessions, passkeys, etc.
	Users *sql.DB

	serverStore *Store
	usersStore  *Store
}

// InitDualDB initializes both server and users databases
// AI.md: Database paths must be {data_dir}/db/server.db and {data_dir}/db/users.db
func InitDualDB(dataDir string) (*DualDB, error) {
	if strings.TrimSpace(dataDir) == "" {
		dataDir = "./data"
	}
	if err := seedDatabaseEnvironmentOnFirstRun(dataDir); err != nil {
		return nil, err
	}
	driverValue, _, driverErr := config.ReadSetting("server.database.driver")
	urlValue, _, urlErr := config.ReadSetting("server.database.url")
	if driverErr != nil || urlErr != nil {
		return nil, fmt.Errorf("read configured database connection: %w", errors.Join(driverErr, urlErr))
	}
	driver := strings.ToLower(settingText(driverValue))
	connectionURL := settingText(urlValue)
	if driver != "" && driver != "file" && driver != "sqlite" || remoteURLScheme(connectionURL) {
		return initRemoteDualDB(driver, connectionURL, dataDir)
	}
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
	serverStore, err := initServerDB(serverDBPath)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize server database: %w", err)
	}
	serverDB := serverStore.DB()

	// Initialize users database
	usersStore, err := initUsersDB(usersDBPath)
	if err != nil {
		_ = serverStore.Close()
		return nil, fmt.Errorf("failed to initialize users database: %w", err)
	}
	usersDB := usersStore.DB()

	// Server Admin data moved from server.db to users.db (AI.md PART 12)
	if err := migrateLegacyAdminTables(serverDB, usersDB); err != nil {
		_ = serverStore.Close()
		_ = usersStore.Close()
		return nil, fmt.Errorf("failed to carry legacy admin data forward: %w", err)
	}

	return &DualDB{
		Server:      serverDB,
		Users:       usersDB,
		serverStore: serverStore,
		usersStore:  usersStore,
	}, nil
}

func seedDatabaseEnvironmentOnFirstRun(dataDir string) error {
	localPath := filepath.Join(dataDir, "db", "server.db")
	if _, err := os.Stat(localPath); err == nil {
		if envURL := strings.TrimSpace(os.Getenv("DATABASE_URL")); envURL != "" {
			storedURL, exists, readErr := config.ReadSetting("server.database.url")
			if readErr != nil {
				return readErr
			}
			if exists && settingText(storedURL) != envURL {
				log.Printf("WARNING: DATABASE_URL differs from the saved database connection and is ignored")
			}
		}
		return nil
	} else if !os.IsNotExist(err) {
		return err
	}
	savedURL, savedURLExists, err := config.ReadSetting("server.database.url")
	if err != nil {
		return err
	}
	savedDriverValue, savedDriverExists, err := config.ReadSetting("server.database.driver")
	if err != nil {
		return err
	}
	savedDriver := strings.ToLower(settingText(savedDriverValue))
	if savedURLExists {
		if envURL := strings.TrimSpace(os.Getenv("DATABASE_URL")); envURL != "" && settingText(savedURL) != envURL {
			log.Printf("WARNING: DATABASE_URL differs from the saved database connection and is ignored")
		}
		return nil
	}
	if savedDriverExists && savedDriver != "" && savedDriver != "file" && savedDriver != "sqlite" {
		if os.Getenv("DATABASE_URL") != "" || os.Getenv("DATABASE_DRIVER") != "" {
			log.Printf("WARNING: database connection environment variables are ignored because the connection is already configured")
		}
		return nil
	}
	if envURL := strings.TrimSpace(os.Getenv("DATABASE_URL")); envURL != "" {
		parsed, err := ParseConnectionString(envURL)
		if err != nil {
			return err
		}
		if parsed.Type != "sqlite" && parsed.Type != "postgres" && parsed.Type != "mysql" {
			return fmt.Errorf("DATABASE_URL uses an unsupported database driver")
		}
		if err := config.WriteSetting("server.database.url", envURL); err != nil {
			return fmt.Errorf("save first-run database URL: %w", err)
		}
		if err := config.WriteSetting("server.database.driver", parsed.Type); err != nil {
			return fmt.Errorf("save first-run database driver: %w", err)
		}
		return nil
	}
	if driver := strings.ToLower(strings.TrimSpace(os.Getenv("DATABASE_DRIVER"))); driver != "" {
		aliases := map[string]string{"sqlite2": "sqlite", "sqlite3": "sqlite", "pgsql": "postgres", "postgresql": "postgres", "mariadb": "mysql"}
		if canonical, ok := aliases[driver]; ok {
			driver = canonical
		}
		switch driver {
		case "sqlite", "postgres", "mysql":
			return config.WriteSetting("server.database.driver", driver)
		default:
			return fmt.Errorf("DATABASE_DRIVER uses an unsupported database driver")
		}
	}
	return nil
}

func remoteURLScheme(connectionURL string) bool {
	return strings.HasPrefix(connectionURL, "postgres://") || strings.HasPrefix(connectionURL, "postgresql://") || strings.HasPrefix(connectionURL, "mysql://") || strings.HasPrefix(connectionURL, "mariadb://")
}

// RemoteDatabaseConfigured reports whether server.yml or secrets.yml selects
// a shared PostgreSQL or MySQL database.
func RemoteDatabaseConfigured() bool {
	driverValue, _, _ := config.ReadSetting("server.database.driver")
	urlValue, _, _ := config.ReadSetting("server.database.url")
	driver := strings.ToLower(settingText(driverValue))
	return remoteURLScheme(settingText(urlValue)) || driver == "postgres" || driver == "postgresql" || driver == "pgsql" || driver == "mysql" || driver == "mariadb"
}

func initRemoteDualDB(driver, connectionURL, dataDir string) (*DualDB, error) {
	if strings.TrimSpace(dataDir) == "" {
		dataDir = "./data"
	}
	localServerPath := filepath.Join(dataDir, "db", "server.db")
	if _, err := os.Stat(localServerPath); err == nil {
		localStore, openErr := OpenSQLite(localServerPath, 1)
		if openErr != nil {
			return nil, fmt.Errorf("open local SQLite database for migration: %w", openErr)
		}
		if migrateErr := config.MigrateLegacyDatabaseSettings(localStore); migrateErr != nil {
			_ = localStore.Close()
			return nil, fmt.Errorf("migrate local settings before remote database startup: %w", migrateErr)
		}
		if closeErr := localStore.Close(); closeErr != nil {
			return nil, fmt.Errorf("close local SQLite database after settings migration: %w", closeErr)
		}
	}
	readString := func(key string) string {
		value, _, _ := config.ReadSetting(key)
		return settingText(value)
	}
	portValue, _, _ := config.ReadSetting("server.database.port")
	port, _ := strconv.Atoi(settingText(portValue))
	db, err := OpenConfigured(driver, connectionURL, readString("server.database.host"), port, readString("server.database.name"), readString("server.database.username"), readString("server.database.password"), readString("server.database.sslmode"))
	if err != nil {
		return nil, err
	}
	configurePool(db)
	if err := PingWithTimeout(db); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("connect to configured remote database: %w", err)
	}
	if err := initializeRemoteSchema(db, driver); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("initialize configured remote database schema: %w", err)
	}
	if err := migrateLocalSQLiteData(dataDir, db); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("migrate local SQLite data into remote database: %w", err)
	}
	if RemoteDatabaseDialect(db) == "postgres" {
		if err := synchronizePostgresSequences(db); err != nil {
			_ = db.Close()
			return nil, fmt.Errorf("synchronize PostgreSQL sequences after local data migration: %w", err)
		}
	}
	store := NewStore(db)
	return &DualDB{Server: db, Users: db, serverStore: store, usersStore: store}, nil
}

func synchronizePostgresSequences(db *sql.DB) error {
	remoteTablesOnce.Do(func() { _ = rewriteRemoteTableNames("") })
	for name := range remoteAutoIDTables {
		table := remoteTableName(name)
		var maxID int64
		if err := db.QueryRow("SELECT COALESCE(MAX(id), 0) FROM " + table).Scan(&maxID); err != nil {
			return fmt.Errorf("read maximum ID for %s: %w", table, err)
		}
		if maxID == 0 {
			continue
		}
		var sequence sql.NullString
		if err := db.QueryRow("SELECT pg_get_serial_sequence($1, 'id')", table).Scan(&sequence); err != nil {
			return fmt.Errorf("find sequence for %s: %w", table, err)
		}
		if !sequence.Valid || sequence.String == "" {
			continue
		}
		var lastValue sql.NullInt64
		if err := db.QueryRow(`
			SELECT ps.last_value
			FROM pg_sequences ps
			JOIN pg_class seq ON seq.relname = ps.sequencename
			JOIN pg_namespace ns ON ns.nspname = ps.schemaname AND ns.oid = seq.relnamespace
			WHERE seq.oid = $1::regclass`, sequence.String).Scan(&lastValue); err != nil {
			return fmt.Errorf("read current sequence value for %s: %w", table, err)
		}
		if lastValue.Valid && lastValue.Int64 >= maxID {
			continue
		}
		// Advance with nextval rather than setval: nextval is atomic across
		// server nodes, and cannot move a live sequence backward after deletes.
		for {
			var nextID int64
			if err := db.QueryRow("SELECT nextval($1::regclass)", sequence.String).Scan(&nextID); err != nil {
				return fmt.Errorf("advance sequence for %s: %w", table, err)
			}
			if nextID >= maxID {
				break
			}
		}
	}
	return nil
}

func migrateLocalSQLiteData(dataDir string, remote *sql.DB) error {
	for _, source := range []struct {
		file    string
		renames []legacyRename
		schemas []string
	}{
		{filepath.Join(dataDir, "db", "server.db"), serverLegacyRenames, []string{ServerSchema}},
		{filepath.Join(dataDir, "db", "users.db"), usersLegacyRenames, []string{UsersSchema}},
	} {
		if _, err := os.Stat(source.file); err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return err
		}
		localStore, err := OpenSQLite(source.file, 1)
		if err != nil {
			return err
		}
		local := localStore.DB()
		if err := renameLegacyTables(local, source.renames); err != nil {
			_ = localStore.Close()
			return err
		}
		var names []string
		for _, schema := range source.schemas {
			for _, match := range regexp.MustCompile(`(?i)CREATE\s+TABLE\s+IF\s+NOT\s+EXISTS\s+([a-zA-Z_][a-zA-Z0-9_]*)`).FindAllStringSubmatch(schema, -1) {
				if match[1] != "server_config" {
					names = append(names, match[1])
				}
			}
		}
		for _, table := range names {
			exists, err := sqliteTableExists(local, table)
			if err != nil {
				_ = localStore.Close()
				return err
			}
			if !exists {
				continue
			}
			columns, err := sqliteColumns(local, table)
			if err != nil {
				_ = localStore.Close()
				return err
			}
			if len(columns) == 0 {
				continue
			}
			rows, err := local.Query("SELECT * FROM " + table)
			if err != nil {
				_ = localStore.Close()
				return err
			}
			placeholders := strings.TrimSuffix(strings.Repeat("?,", len(columns)), ",")
			insert := fmt.Sprintf("INSERT INTO %s (%s) VALUES (%s) ON CONFLICT DO NOTHING", table, strings.Join(columns, ","), placeholders)
			for rows.Next() {
				values := make([]any, len(columns))
				dest := make([]any, len(columns))
				for i := range values {
					dest[i] = &values[i]
				}
				if err := rows.Scan(dest...); err != nil {
					_ = rows.Close()
					_ = localStore.Close()
					return err
				}
				if _, err := remote.Exec(insert, values...); err != nil {
					_ = rows.Close()
					_ = localStore.Close()
					return fmt.Errorf("copy %s row: %w", table, err)
				}
			}
			if err := rows.Err(); err != nil {
				_ = rows.Close()
				_ = localStore.Close()
				return err
			}
			if err := rows.Close(); err != nil {
				_ = localStore.Close()
				return err
			}
		}
		if err := localStore.Close(); err != nil {
			return err
		}
	}
	return nil
}

func settingText(value any) string {
	if value == nil {
		return ""
	}
	return strings.TrimSpace(fmt.Sprint(value))
}

// initServerDB initializes the server database with server schema
func initServerDB(path string) (*Store, error) {
	store, err := OpenSQLite(path, poolMaxOpen)
	if err != nil {
		return nil, fmt.Errorf("failed to open server database: %w", err)
	}
	db := store.DB()

	// AI.md PART 10: every connection uses the standard pool configuration
	configurePool(db)

	// Test connection
	if err := PingWithTimeout(db); err != nil {
		_ = store.Close()
		return nil, fmt.Errorf("failed to connect to server database: %w", err)
	}

	// Databases created before AI.md PART 12 naming keep their data under the
	// legacy table names; rename them before the base schema runs
	if err := renameLegacyTables(db, serverLegacyRenames); err != nil {
		_ = store.Close()
		return nil, fmt.Errorf("failed to rename legacy server tables: %w", err)
	}
	if err := migrateSchedulerSchema(db); err != nil {
		_ = store.Close()
		return nil, fmt.Errorf("failed to migrate scheduler schema: %w", err)
	}

	// Create server schema
	if _, err := ExecContext(context.Background(), db, TimeoutMigration, ServerSchema); err != nil {
		_ = store.Close()
		return nil, fmt.Errorf("failed to create server schema: %w", err)
	}

	if err := copyLegacyAdminSessions(db); err != nil {
		_ = store.Close()
		return nil, fmt.Errorf("failed to carry legacy admin sessions forward: %w", err)
	}

	if err := applySchemaUpdates(db, serverSchemaUpdates); err != nil {
		_ = store.Close()
		return nil, fmt.Errorf("failed to apply server schema updates: %w", err)
	}
	if err := migrateLegacyAdminSessions(db); err != nil {
		_ = store.Close()
		return nil, fmt.Errorf("failed to migrate legacy admin sessions: %w", err)
	}

	return store, nil
}

// initUsersDB initializes the users database with users schema
func initUsersDB(path string) (*Store, error) {
	store, err := OpenSQLite(path, poolMaxOpen)
	if err != nil {
		return nil, fmt.Errorf("failed to open users database: %w", err)
	}
	db := store.DB()

	// AI.md PART 10: every connection uses the standard pool configuration
	configurePool(db)

	// Test connection
	if err := PingWithTimeout(db); err != nil {
		_ = store.Close()
		return nil, fmt.Errorf("failed to connect to users database: %w", err)
	}

	// Databases created before AI.md PART 12 naming keep their data under the
	// legacy table names; rename them before the base schema runs
	if err := renameLegacyTables(db, usersLegacyRenames); err != nil {
		_ = store.Close()
		return nil, fmt.Errorf("failed to rename legacy users tables: %w", err)
	}

	// Create users schema
	if _, err := ExecContext(context.Background(), db, TimeoutMigration, UsersSchema); err != nil {
		_ = store.Close()
		return nil, fmt.Errorf("failed to create users schema: %w", err)
	}

	if err := applySchemaUpdates(db, usersSchemaUpdates); err != nil {
		_ = store.Close()
		return nil, fmt.Errorf("failed to apply users schema updates: %w", err)
	}

	return store, nil
}

// Close closes both database connections
func (ddb *DualDB) Close() error {
	var errs []string

	if ddb.serverStore != nil && ddb.serverStore.owned {
		if err := ddb.serverStore.Close(); err != nil {
			errs = append(errs, fmt.Sprintf("server db: %v", err))
		}
	} else if ddb.Server != nil {
		if err := ddb.Server.Close(); err != nil {
			errs = append(errs, fmt.Sprintf("server db: %v", err))
		}
	}

	if ddb.usersStore != nil && ddb.usersStore.owned && ddb.usersStore != ddb.serverStore {
		if err := ddb.usersStore.Close(); err != nil {
			errs = append(errs, fmt.Sprintf("users db: %v", err))
		}
	} else if (ddb.usersStore == nil || !ddb.usersStore.owned) && ddb.Users != nil && ddb.Users != ddb.Server {
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

// GetServerStore returns the store for server.db when SQLite is configured.
func (ddb *DualDB) GetServerStore() *Store {
	if ddb == nil {
		return nil
	}
	if ddb.serverStore != nil {
		return ddb.serverStore
	}
	return NewStore(ddb.Server)
}

// GetUsersStore returns the store for users.db when SQLite is configured.
func (ddb *DualDB) GetUsersStore() *Store {
	if ddb == nil {
		return nil
	}
	if ddb.usersStore != nil {
		return ddb.usersStore
	}
	return NewStore(ddb.Users)
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
