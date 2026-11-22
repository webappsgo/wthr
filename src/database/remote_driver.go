package database

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"sync"

	mysql "github.com/go-sql-driver/mysql"
	pgxstdlib "github.com/jackc/pgx/v5/stdlib"
)

type rewritingDriver struct {
	base    driver.Driver
	dialect string
}

type rewritingConn struct {
	base    driver.Conn
	dialect string
}

var registerRemoteDrivers sync.Once
var remoteDBHandles sync.Map
var remoteTablesOnce sync.Once
var remoteTablePrefixes map[string]string
var remoteAutoIDTables map[string]struct{}

func openRemoteSQLDriver(driverName, dsn string) (*sql.DB, error) {
	registerRemoteDrivers.Do(func() {
		sql.Register("wthr-pgx", &rewritingDriver{base: pgxstdlib.GetDefaultDriver(), dialect: "postgres"})
		sql.Register("wthr-mysql", &rewritingDriver{base: &mysql.MySQLDriver{}, dialect: "mysql"})
	})
	registered := "wthr-pgx"
	if driverName == "mysql" || driverName == "mariadb" {
		registered = "wthr-mysql"
	}
	db, err := sql.Open(registered, dsn)
	if err == nil {
		remoteDBHandles.Store(db, driverName)
	}
	return db, err
}

func IsRemoteDatabase(db *sql.DB) bool {
	if db == nil {
		return false
	}
	_, ok := remoteDBHandles.Load(db)
	return ok
}

func RemoteDatabaseDialect(db *sql.DB) string {
	if db == nil {
		return ""
	}
	value, _ := remoteDBHandles.Load(db)
	dialect, _ := value.(string)
	return dialect
}

func (d *rewritingDriver) Open(name string) (driver.Conn, error) {
	conn, err := d.base.Open(name)
	if err != nil {
		return nil, err
	}
	return &rewritingConn{base: conn, dialect: d.dialect}, nil
}

func (c *rewritingConn) Prepare(query string) (driver.Stmt, error) {
	statement, err := c.base.Prepare(rewriteRemoteQuery(query, c.dialect))
	if err != nil {
		return nil, err
	}
	return statement, nil
}

func (c *rewritingConn) Close() error { return c.base.Close() }
func (c *rewritingConn) Begin() (driver.Tx, error) {
	if conn, ok := c.base.(driver.ConnBeginTx); ok {
		return conn.BeginTx(context.Background(), driver.TxOptions{})
	}
	return nil, errors.New("underlying database driver does not support transactions")
}

func (c *rewritingConn) PrepareContext(ctx context.Context, query string) (driver.Stmt, error) {
	query = rewriteRemoteQuery(query, c.dialect)
	if preparer, ok := c.base.(driver.ConnPrepareContext); ok {
		return preparer.PrepareContext(ctx, query)
	}
	return c.base.Prepare(query)
}

func (c *rewritingConn) BeginTx(ctx context.Context, opts driver.TxOptions) (driver.Tx, error) {
	if conn, ok := c.base.(driver.ConnBeginTx); ok {
		return conn.BeginTx(ctx, opts)
	}
	return nil, errors.New("underlying database driver does not support transactions")
}

func (c *rewritingConn) ExecContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Result, error) {
	query = rewriteRemoteQuery(query, c.dialect)
	if c.dialect == "postgres" && postgresInsertNeedsID(query) {
		if queryer, ok := c.base.(driver.QueryerContext); ok {
			rows, err := queryer.QueryContext(ctx, query+" RETURNING id", args)
			if err == nil {
				defer func() { _ = rows.Close() }()
				values := make([]driver.Value, len(rows.Columns()))
				if err := rows.Next(values); err == nil {
					id, ok := values[0].(int64)
					if !ok {
						id, _ = strconv.ParseInt(fmt.Sprint(values[0]), 10, 64)
					}
					return remoteInsertResult{lastID: id, rows: 1}, nil
				}
			}
		}
	}
	if conn, ok := c.base.(driver.ExecerContext); ok {
		return conn.ExecContext(ctx, query, args)
	}
	return nil, driver.ErrSkip
}

func (c *rewritingConn) QueryContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
	query = rewriteRemoteQuery(query, c.dialect)
	if conn, ok := c.base.(driver.QueryerContext); ok {
		return conn.QueryContext(ctx, query, args)
	}
	return nil, driver.ErrSkip
}

func (c *rewritingConn) Ping(ctx context.Context) error {
	if conn, ok := c.base.(driver.Pinger); ok {
		return conn.Ping(ctx)
	}
	return nil
}

func rewriteRemoteQuery(query, dialect string) string {
	query = rewriteRemoteTableNames(query)
	upper := strings.ToUpper(strings.TrimSpace(query))
	if dialect == "mysql" {
		query = rewriteMySQLConflict(query)
		if strings.HasPrefix(upper, "INSERT OR IGNORE INTO ") {
			query = "INSERT IGNORE INTO " + strings.TrimSpace(query[len("INSERT OR IGNORE INTO "):])
		}
		if strings.HasPrefix(upper, "INSERT OR REPLACE INTO ") {
			query = "REPLACE INTO " + strings.TrimSpace(query[len("INSERT OR REPLACE INTO "):])
		}
		return strings.ReplaceAll(query, "datetime('now')", "CURRENT_TIMESTAMP")
	}
	if strings.HasPrefix(upper, "INSERT OR IGNORE INTO ") {
		query = "INSERT INTO " + strings.TrimSpace(query[len("INSERT OR IGNORE INTO "):])
		if !strings.Contains(strings.ToUpper(query), "ON CONFLICT") {
			query += " ON CONFLICT DO NOTHING"
		}
	}
	query = strings.ReplaceAll(query, "datetime('now')", "CURRENT_TIMESTAMP")
	return replaceQuestionMarks(query)
}

func rewriteRemoteTableNames(query string) string {
	remoteTablesOnce.Do(func() {
		remoteTablePrefixes = make(map[string]string)
		remoteAutoIDTables = make(map[string]struct{})
		for _, schema := range []struct{ prefix, ddl string }{{"srv_", ServerSchema}, {"usr_", UsersSchema}} {
			for _, match := range regexp.MustCompile(`(?i)CREATE\s+TABLE\s+IF\s+NOT\s+EXISTS\s+([a-zA-Z_][a-zA-Z0-9_]*)`).FindAllStringSubmatch(schema.ddl, -1) {
				remoteTablePrefixes[strings.ToLower(match[1])] = schema.prefix + match[1]
				body := regexp.MustCompile(`(?is)CREATE\s+TABLE\s+IF\s+NOT\s+EXISTS\s+` + regexp.QuoteMeta(match[1]) + `\s*\((.*?)\n\)`).FindStringSubmatch(schema.ddl)
				if len(body) == 2 && regexp.MustCompile(`(?i)\bid\s+INTEGER\s+PRIMARY\s+KEY\s+AUTOINCREMENT\b`).MatchString(body[1]) {
					remoteAutoIDTables[strings.ToLower(match[1])] = struct{}{}
				}
			}
		}
		remoteTablePrefixes["app_secrets"] = "srv_app_secrets"
	})
	for name, prefixed := range remoteTablePrefixes {
		pattern := regexp.MustCompile(`(?i)(\b(?:FROM|JOIN|UPDATE|INTO|REFERENCES|TABLE|ON)\s+)` + regexp.QuoteMeta(name) + `\b`)
		query = pattern.ReplaceAllString(query, "${1}"+prefixed)
	}
	return query
}

func remoteTableName(name string) string {
	remoteTablesOnce.Do(func() { _ = rewriteRemoteTableNames("") })
	if rewritten, ok := remoteTablePrefixes[strings.ToLower(name)]; ok {
		return rewritten
	}
	return name
}

func postgresInsertNeedsID(query string) bool {
	if !strings.HasPrefix(strings.ToUpper(strings.TrimSpace(query)), "INSERT INTO ") || strings.Contains(strings.ToUpper(query), " RETURNING ") {
		return false
	}
	match := regexp.MustCompile(`(?i)^\s*INSERT\s+INTO\s+([a-zA-Z_][a-zA-Z0-9_]*)`).FindStringSubmatch(query)
	if len(match) != 2 {
		return false
	}
	remoteTablesOnce.Do(func() { _ = rewriteRemoteTableNames("") })
	_, exists := remoteAutoIDTables[strings.ToLower(strings.TrimPrefix(match[1], "srv_"))]
	if !exists {
		_, exists = remoteAutoIDTables[strings.ToLower(strings.TrimPrefix(match[1], "usr_"))]
	}
	return exists
}

type remoteInsertResult struct{ lastID, rows int64 }

func (r remoteInsertResult) LastInsertId() (int64, error) { return r.lastID, nil }
func (r remoteInsertResult) RowsAffected() (int64, error) { return r.rows, nil }

func replaceQuestionMarks(query string) string {
	var out strings.Builder
	index := 0
	quote := byte(0)
	for i := 0; i < len(query); i++ {
		char := query[i]
		if quote != 0 {
			out.WriteByte(char)
			if char == '\\' && i+1 < len(query) {
				i++
				out.WriteByte(query[i])
				continue
			}
			if char == quote {
				if i+1 < len(query) && query[i+1] == quote {
					i++
					out.WriteByte(query[i])
				} else {
					quote = 0
				}
			}
			continue
		}
		if char == '-' && i+1 < len(query) && query[i+1] == '-' {
			for i < len(query) && query[i] != '\n' {
				out.WriteByte(query[i])
				i++
			}
			if i < len(query) {
				out.WriteByte(query[i])
			}
			continue
		}
		if char == '/' && i+1 < len(query) && query[i+1] == '*' {
			for i < len(query) {
				out.WriteByte(query[i])
				if query[i] == '*' && i+1 < len(query) && query[i+1] == '/' {
					i++
					out.WriteByte(query[i])
					break
				}
				i++
			}
			continue
		}
		if char == '\'' || char == '"' || char == '`' {
			quote = char
			out.WriteByte(char)
			continue
		}
		if char == '?' {
			index++
			fmt.Fprintf(&out, "$%d", index)
			continue
		}
		out.WriteByte(char)
	}
	return out.String()
}

func rewriteMySQLConflict(query string) string {
	conflict := regexp.MustCompile(`(?is)\s+ON\s+CONFLICT\s*\([^)]*\)\s+DO\s+NOTHING\s*$`)
	if conflict.MatchString(query) {
		query = conflict.ReplaceAllString(query, "")
		query = regexp.MustCompile(`(?i)^\s*INSERT\s+INTO\s+`).ReplaceAllString(query, "INSERT IGNORE INTO ")
		return query
	}
	conflict = regexp.MustCompile(`(?is)\s+ON\s+CONFLICT\s+DO\s+NOTHING\s*$`)
	if conflict.MatchString(query) {
		query = conflict.ReplaceAllString(query, "")
		query = regexp.MustCompile(`(?i)^\s*INSERT\s+INTO\s+`).ReplaceAllString(query, "INSERT IGNORE INTO ")
		return query
	}
	update := regexp.MustCompile(`(?is)\s+ON\s+CONFLICT\s*\([^)]*\)\s+DO\s+UPDATE\s+SET\s+(.+)$`)
	if match := update.FindStringSubmatch(query); len(match) == 2 {
		assignments := regexp.MustCompile(`excluded\.([a-zA-Z_][a-zA-Z0-9_]*)`).ReplaceAllString(match[1], "VALUES($1)")
		return update.ReplaceAllString(query, " ON DUPLICATE KEY UPDATE "+assignments)
	}
	return query
}
