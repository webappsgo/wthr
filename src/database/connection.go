package database

import (
	"database/sql"
	"fmt"
	"net"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	mysql "github.com/go-sql-driver/mysql"
	_ "github.com/jackc/pgx/v5/stdlib"
	_ "github.com/microsoft/go-mssqldb"
	_ "modernc.org/sqlite"
)

// DatabaseConfig holds database connection configuration
// AI.md PART 10: Connection pooling required
type DatabaseConfig struct {
	// sqlite, postgres, mysql, mssql, mongodb
	Type     string
	Host     string
	Port     int
	Database string
	Username string
	Password string
	// For PostgreSQL
	SSLMode string
	Options map[string]string
	// Connection pool settings per AI.md PART 10
	Pool PoolConfig
}

func initializeRemoteSchema(db *sql.DB, driver string) error {
	if err := migrateRemoteSchedulerSchema(db, driver); err != nil {
		return fmt.Errorf("migrate remote scheduler schema: %w", err)
	}
	for _, schema := range []struct{ name, prefix, ddl string }{
		{"server", "srv_", ServerSchema},
		{"users", "usr_", UsersSchema},
	} {
		tables := regexp.MustCompile(`(?i)CREATE\s+TABLE\s+IF\s+NOT\s+EXISTS\s+([a-zA-Z_][a-zA-Z0-9_]*)`).FindAllStringSubmatch(schema.ddl, -1)
		rewritten := schema.ddl
		for _, match := range tables {
			name := match[1]
			prefixed := schema.prefix + name
			identifier := regexp.MustCompile(`\b` + regexp.QuoteMeta(name) + `\b`)
			rewritten = identifier.ReplaceAllString(rewritten, prefixed)
		}
		rewritten = strings.ReplaceAll(rewritten, "INTEGER PRIMARY KEY AUTOINCREMENT", "BIGSERIAL PRIMARY KEY")
		rewritten = strings.ReplaceAll(rewritten, "DATETIME", "TIMESTAMP")
		rewritten = regexp.MustCompile(`(?i)BOOLEAN(\s+NOT NULL)?\s+DEFAULT\s+1\b`).ReplaceAllString(rewritten, "BOOLEAN${1} DEFAULT TRUE")
		rewritten = regexp.MustCompile(`(?i)BOOLEAN(\s+NOT NULL)?\s+DEFAULT\s+0\b`).ReplaceAllString(rewritten, "BOOLEAN${1} DEFAULT FALSE")
		if driver == "mysql" || driver == "mariadb" {
			rewritten = strings.ReplaceAll(rewritten, "BIGSERIAL PRIMARY KEY", "BIGINT PRIMARY KEY AUTO_INCREMENT")
			rewritten = rewriteMySQLSchemaTextTypes(rewritten)
		} else {
			rewritten = regexp.MustCompile(`(?i)CREATE\s+(UNIQUE\s+)?INDEX\s+IF\s+NOT\s+EXISTS\s+([a-zA-Z_][a-zA-Z0-9_]*)`).ReplaceAllString(rewritten, "CREATE $1INDEX IF NOT EXISTS "+schema.prefix+"$2")
		}
		for _, statement := range strings.Split(rewritten, ";") {
			statement = strings.TrimSpace(statement)
			if statement == "" || strings.HasPrefix(strings.ToUpper(statement), "--") && !strings.Contains(strings.ToUpper(statement), "CREATE TABLE") && !strings.Contains(strings.ToUpper(statement), "CREATE INDEX") {
				continue
			}
			if (driver == "mysql" || driver == "mariadb") && strings.Contains(strings.ToUpper(statement), "CREATE ") && strings.Contains(strings.ToUpper(statement), " INDEX ") {
				indexName := regexp.MustCompile(`(?i)CREATE\s+(?:UNIQUE\s+)?INDEX\s+IF\s+NOT\s+EXISTS\s+([a-zA-Z_][a-zA-Z0-9_]*)`).FindStringSubmatch(statement)
				if len(indexName) != 2 {
					continue
				}
				var found int
				if err := db.QueryRow(`SELECT COUNT(*) FROM information_schema.statistics WHERE table_schema = DATABASE() AND index_name = ?`, indexName[1]).Scan(&found); err != nil {
					return fmt.Errorf("check remote index %s: %w", indexName[1], err)
				}
				if found > 0 {
					continue
				}
				statement = regexp.MustCompile(`(?i)\s+IF\s+NOT\s+EXISTS`).ReplaceAllString(statement, "")
			}
			if _, err := db.Exec(statement); err != nil {
				return fmt.Errorf("create remote %s schema: %w", schema.name, err)
			}
		}
		if err := applyRemoteSchemaUpdates(db, driver, schema.name); err != nil {
			return err
		}
	}
	return nil
}

func rewriteMySQLSchemaTextTypes(ddl string) string {
	indexed := make(map[string]map[string]struct{})
	for _, match := range regexp.MustCompile(`(?i)CREATE\s+(?:UNIQUE\s+)?INDEX\s+IF\s+NOT\s+EXISTS\s+[a-zA-Z_][a-zA-Z0-9_]*\s+ON\s+([a-zA-Z_][a-zA-Z0-9_]*)\s*\(([^)]*)\)`).FindAllStringSubmatch(ddl, -1) {
		columns := indexed[strings.ToLower(match[1])]
		if columns == nil {
			columns = make(map[string]struct{})
			indexed[strings.ToLower(match[1])] = columns
		}
		for _, item := range strings.Split(match[2], ",") {
			name := strings.Fields(strings.TrimSpace(item))[0]
			columns[strings.ToLower(name)] = struct{}{}
		}
	}
	lines := strings.Split(ddl, "\n")
	table := ""
	tableStart := regexp.MustCompile(`(?i)^\s*CREATE\s+TABLE\s+IF\s+NOT\s+EXISTS\s+([a-zA-Z_][a-zA-Z0-9_]*)\s*\(`)
	textColumn := regexp.MustCompile(`(?i)^(\s*[a-zA-Z_][a-zA-Z0-9_]*\s+)TEXT\b`)
	for i, line := range lines {
		if match := tableStart.FindStringSubmatch(line); len(match) == 2 {
			table = strings.ToLower(match[1])
		}
		if table != "" {
			if match := textColumn.FindStringSubmatch(line); len(match) == 2 {
				columnName := strings.ToLower(strings.Fields(strings.TrimSpace(match[1]))[0])
				_, indexedColumn := indexed[table][columnName]
				if indexedColumn || regexp.MustCompile(`(?i)\b(?:UNIQUE|PRIMARY\s+KEY)\b`).MatchString(line) {
					lines[i] = textColumn.ReplaceAllString(line, `${1}VARCHAR(191)`)
				} else {
					lines[i] = textColumn.ReplaceAllString(line, `${1}LONGTEXT`)
					lines[i] = regexp.MustCompile(`(?i)DEFAULT\s+'([^']*)'`).ReplaceAllString(lines[i], "DEFAULT ('$1')")
				}
			}
			if strings.Contains(line, ");") {
				table = ""
			}
		}
	}
	return strings.Join(lines, "\n")
}

func applyRemoteSchemaUpdates(db *sql.DB, driver, schemaName string) error {
	updates := serverSchemaUpdates
	if schemaName == "users" {
		updates = usersSchemaUpdates
	}
	for _, update := range updates {
		query := rewriteRemoteTableNames(update)
		if driver == "mysql" || driver == "mariadb" {
			index := regexp.MustCompile(`(?i)^\s*CREATE\s+(?:UNIQUE\s+)?INDEX\s+IF\s+NOT\s+EXISTS\s+([a-zA-Z_][a-zA-Z0-9_]*)`).FindStringSubmatch(query)
			if len(index) == 2 {
				if schemaName == "server" && strings.EqualFold(index[1], "idx_join_tokens_hash") {
					if _, err := db.Exec(`ALTER TABLE srv_server_join_tokens MODIFY token_hash VARCHAR(64)`); err != nil {
						return fmt.Errorf("size remote join-token hash column: %w", err)
					}
				}
				var exists int
				if err := db.QueryRow(`SELECT COUNT(*) FROM information_schema.statistics WHERE table_schema = DATABASE() AND index_name = ?`, index[1]).Scan(&exists); err != nil {
					return fmt.Errorf("check %s schema index %s: %w", schemaName, index[1], err)
				}
				if exists != 0 {
					continue
				}
				query = regexp.MustCompile(`(?i)\s+IF\s+NOT\s+EXISTS`).ReplaceAllString(query, "")
			}
		}
		if _, err := db.Exec(query); err != nil && !isColumnExistsError(err) {
			return fmt.Errorf("update remote %s schema: %w", schemaName, err)
		}
	}
	return nil
}

// ParseConnectionString parses a database connection string and returns config
func ParseConnectionString(connString string) (*DatabaseConfig, error) {
	config := &DatabaseConfig{
		Options: make(map[string]string),
	}

	if strings.HasPrefix(connString, "sqlite://") || strings.HasPrefix(connString, "sqlite:") {
		config.Type = "sqlite"
		config.Database = strings.TrimPrefix(connString, "sqlite://")
		config.Database = strings.TrimPrefix(config.Database, "sqlite:")
		config.Database = strings.TrimPrefix(config.Database, "//")
		return config, nil
	}

	if strings.HasPrefix(connString, "postgres://") || strings.HasPrefix(connString, "postgresql://") || strings.HasPrefix(connString, "mysql://") || strings.HasPrefix(connString, "mariadb://") {
		parsed, err := url.Parse(connString)
		if err != nil || parsed.Hostname() == "" {
			return nil, fmt.Errorf("invalid database connection URL; use the URL form")
		}
		switch parsed.Scheme {
		case "postgres", "postgresql":
			config.Type = "postgres"
		case "mysql", "mariadb":
			config.Type = "mysql"
		}
		config.Host = parsed.Hostname()
		if parsed.Port() != "" {
			config.Port, _ = strconv.Atoi(parsed.Port())
		}
		config.Database = strings.TrimPrefix(parsed.Path, "/")
		if parsed.User != nil {
			config.Username = parsed.User.Username()
			config.Password, _ = parsed.User.Password()
		}
		for key, values := range parsed.Query() {
			if len(values) > 0 {
				config.Options[key] = values[len(values)-1]
			}
		}
		config.SSLMode = parsed.Query().Get("sslmode")
		return config, nil
	}

	if strings.HasPrefix(connString, "sqlserver://") || strings.HasPrefix(connString, "mssql://") {
		config.Type = "mssql"
		return nil, fmt.Errorf("MSSQL connection string format is not supported — set DB_TYPE, DB_HOST, DB_PORT, DB_NAME, DB_USER, DB_PASSWORD instead")
	}

	if strings.HasPrefix(connString, "mongodb://") || strings.HasPrefix(connString, "mongo://") {
		return nil, fmt.Errorf("MongoDB is not supported: weather service requires SQL database for relational queries")
	}

	// Assume raw SQLite path
	config.Type = "sqlite"
	config.Database = connString
	return config, nil
}

// OpenConfigured opens the configured PostgreSQL or MySQL database using its
// connection URL or individual connection fields.
func OpenConfigured(driver, connectionURL, host string, port int, name, username, password, sslmode string) (*sql.DB, error) {
	driver = strings.ToLower(strings.TrimSpace(driver))
	if strings.TrimSpace(connectionURL) != "" {
		parsed, err := ParseConnectionString(connectionURL)
		if err != nil {
			return nil, err
		}
		driver = parsed.Type
		if driver == "postgres" {
			db, err := openRemoteSQLDriver("postgres", connectionURL)
			if err != nil {
				return nil, fmt.Errorf("open PostgreSQL database: %w", err)
			}
			return db, nil
		}
		return openMySQLFromParts(parsed.Host, parsed.Port, parsed.Database, parsed.Username, parsed.Password, parsed.Options)
	}
	switch driver {
	case "postgres", "postgresql", "pgsql":
		if port == 0 {
			port = 5432
		}
		query := url.Values{}
		if sslmode != "" {
			query.Set("sslmode", sslmode)
		}
		u := &url.URL{Scheme: "postgres", Host: net.JoinHostPort(host, strconv.Itoa(port)), Path: "/" + name}
		if username != "" {
			u.User = url.UserPassword(username, password)
		}
		u.RawQuery = query.Encode()
		db, err := openRemoteSQLDriver("postgres", u.String())
		if err != nil {
			return nil, fmt.Errorf("open PostgreSQL database: %w", err)
		}
		return db, nil
	case "mysql", "mariadb":
		return openMySQLFromParts(host, port, name, username, password, map[string]string{"parseTime": "true"})
	default:
		return nil, fmt.Errorf("unsupported remote database driver %q", driver)
	}
}

func ColumnExists(db *sql.DB, table, column string) (bool, error) {
	if db == nil || !regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_]*$`).MatchString(table) || !regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_]*$`).MatchString(column) {
		return false, fmt.Errorf("invalid database table or column")
	}
	if IsRemoteDatabase(db) {
		table = remoteTableName(table)
		query := `SELECT COUNT(*) FROM information_schema.columns WHERE table_schema = current_schema() AND table_name = ? AND column_name = ?`
		if RemoteDatabaseDialect(db) == "mysql" || RemoteDatabaseDialect(db) == "mariadb" {
			query = `SELECT COUNT(*) FROM information_schema.columns WHERE table_schema = DATABASE() AND table_name = ? AND column_name = ?`
		}
		var count int
		if err := db.QueryRow(query, table, column).Scan(&count); err != nil {
			return false, err
		}
		return count > 0, nil
	}
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM pragma_table_info(?) WHERE name = ?`, table, column).Scan(&count); err != nil {
		return false, err
	}
	return count > 0, nil
}

func openMySQLFromParts(host string, port int, name, username, password string, options map[string]string) (*sql.DB, error) {
	if port == 0 {
		port = 3306
	}
	cfg := mysql.NewConfig()
	cfg.User, cfg.Passwd, cfg.Net, cfg.Addr, cfg.DBName, cfg.ParseTime = username, password, "tcp", net.JoinHostPort(host, strconv.Itoa(port)), name, true
	if cfg.Params == nil {
		cfg.Params = make(map[string]string)
	}
	for key, value := range options {
		cfg.Params[key] = value
	}
	db, err := openRemoteSQLDriver("mysql", cfg.FormatDSN())
	if err != nil {
		return nil, fmt.Errorf("open MySQL database: %w", err)
	}
	return db, nil
}
