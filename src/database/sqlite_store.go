package database

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"io"
	"log"
	"net/url"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/webappsgo/wthr/src/server/metric"
	_ "modernc.org/sqlite"
)

const sqliteStoreWriteTimeout = 10 * time.Second

var sqliteStoresByDB sync.Map

// Store owns separate SQLite handles and exposes one routed database facade.
// Callers use the facade for ordinary SQL while Store.Read and Store.Write
// provide explicit read-only and transactional access for specialized flows.
type Store struct {
	writeDB *sql.DB
	readDB  *sql.DB
	db      *sql.DB

	closeOnce sync.Once
	closeErr  error
	owned     bool
	sqlite    bool
}

// ReadSession holds a query-only connection checked out from a Store.
type ReadSession struct {
	conn *sql.Conn
}

// OpenSQLite opens one SQLite file with independent write and read pools.
func OpenSQLite(path string, readers int) (*Store, error) {
	if readers < 1 {
		readers = DefaultPoolConfig().MaxOpen
	}
	base, err := sqliteStoreDSN(path)
	if err != nil {
		return nil, err
	}
	common := url.Values{}
	common.Add("_pragma", "busy_timeout(10000)")
	common.Add("_pragma", "synchronous(NORMAL)")
	common.Add("_pragma", "foreign_keys(1)")
	writeValues := parseSQLiteStoreQuery(base)
	for key, values := range common {
		for _, value := range values {
			writeValues.Add(key, value)
		}
	}
	writeValues.Set("_txlock", "immediate")
	writeDSN := sqliteStoreURL(base, writeValues)
	readValues := parseSQLiteStoreQuery(base)
	for key, values := range common {
		for _, value := range values {
			readValues.Add(key, value)
		}
	}
	readValues.Add("_pragma", "query_only(1)")
	readDSN := sqliteStoreURL(base, readValues)

	writeDB, err := sql.Open("sqlite", writeDSN)
	if err != nil {
		return nil, fmt.Errorf("open SQLite write handle %q: %w", path, err)
	}
	writeDB.SetMaxOpenConns(1)
	writeDB.SetMaxIdleConns(1)
	writeDB.SetConnMaxLifetime(0)

	store := &Store{writeDB: writeDB, owned: true, sqlite: true}
	ctx, cancel := context.WithTimeout(context.Background(), TimeoutPing)
	defer cancel()
	var mode string
	if err := writeDB.QueryRowContext(ctx, "PRAGMA journal_mode=WAL").Scan(&mode); err != nil {
		_ = writeDB.Close()
		log.Printf("ERROR SQLite database %q refused: could not verify WAL journal mode: %v", path, err)
		return nil, fmt.Errorf("check SQLite journal mode for %q: %w", path, err)
	}
	if !strings.EqualFold(mode, "wal") {
		_ = writeDB.Close()
		log.Printf("ERROR SQLite database %q refused: journal_mode is %q, want wal", path, mode)
		return nil, fmt.Errorf("SQLite database %q requires WAL journal mode, got %q", path, mode)
	}

	readDB, err := sql.Open("sqlite", readDSN)
	if err != nil {
		_ = writeDB.Close()
		return nil, fmt.Errorf("open SQLite read handle %q: %w", path, err)
	}
	readDB.SetMaxOpenConns(readers)
	readDB.SetMaxIdleConns(min(readers, DefaultPoolConfig().MaxIdle))
	readDB.SetConnMaxLifetime(DefaultPoolConfig().MaxLifetime)
	readDB.SetConnMaxIdleTime(DefaultPoolConfig().MaxIdleTime)
	store.readDB = readDB

	if err := PingWithTimeout(writeDB); err != nil {
		_ = readDB.Close()
		_ = writeDB.Close()
		return nil, fmt.Errorf("ping SQLite write handle %q: %w", path, err)
	}
	if err := PingWithTimeout(readDB); err != nil {
		_ = readDB.Close()
		_ = writeDB.Close()
		return nil, fmt.Errorf("ping SQLite read handle %q: %w", path, err)
	}

	store.db = sql.OpenDB(sqliteStoreConnector{store: store})
	sqliteStoresByDB.Store(store.db, store)
	return store, nil
}

// NewStore wraps a non-SQLite database handle with explicit read and write
// accessors. The store does not take ownership of db; its caller still closes
// the handle. SQLite files must be opened with OpenSQLite instead.
func NewStore(db *sql.DB) *Store {
	if db == nil {
		return nil
	}
	if store, ok := sqliteStoresByDB.Load(db); ok {
		return store.(*Store)
	}
	return &Store{writeDB: db, readDB: db, db: db}
}

func sqliteStoreDSN(path string) (string, error) {
	if strings.TrimSpace(path) == "" {
		return "", errors.New("SQLite database path is empty")
	}
	if path == ":memory:" {
		return "", errors.New("SQLite store requires a local database file; in-memory databases cannot provide WAL mode")
	}
	parsed, err := url.Parse(path)
	if err != nil {
		return "", fmt.Errorf("parse SQLite database path: %w", err)
	}
	if parsed.Scheme == "file" {
		if parsed.Fragment != "" {
			return "", errors.New("SQLite database path must not include a fragment")
		}
		if strings.EqualFold(parsed.Query().Get("mode"), "memory") {
			return "", errors.New("SQLite store requires a local database file; in-memory databases cannot provide WAL mode")
		}
		return parsed.String(), nil
	}
	absolutePath, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("resolve SQLite database path: %w", err)
	}
	return (&url.URL{Scheme: "file", Path: absolutePath}).String(), nil
}

func parseSQLiteStoreQuery(databaseURL string) url.Values {
	parsed, err := url.Parse(databaseURL)
	if err != nil {
		return make(url.Values)
	}
	return parsed.Query()
}

func sqliteStoreURL(databaseURL string, query url.Values) string {
	parsed, err := url.Parse(databaseURL)
	if err != nil {
		return databaseURL
	}
	parsed.RawQuery = query.Encode()
	return parsed.String()
}

// DB returns the routed facade used by existing SQL model interfaces. It
// exposes neither underlying handle: SELECT statements use readDB and every
// other statement uses writeDB.
func (s *Store) DB() *sql.DB {
	if s == nil {
		return nil
	}
	return s.db
}

// IsSQLite reports whether this Store owns the two local SQLite pools.
func (s *Store) IsSQLite() bool {
	return s != nil && s.sqlite
}

// Dialect reports the configured database dialect for health and diagnostics.
func (s *Store) Dialect() string {
	if s == nil {
		return ""
	}
	if s.sqlite {
		return "sqlite"
	}
	return RemoteDatabaseDialect(s.writeDB)
}

// Stats returns the reader pool statistics for health and diagnostics pages.
func (s *Store) Stats() sql.DBStats {
	if s == nil || s.readDB == nil {
		return sql.DBStats{}
	}
	return s.readDB.Stats()
}

// HealthCheck probes both the reader pool and an immediate write transaction.
func (s *Store) HealthCheck() (status string, latencyMs int64, err error) {
	started := time.Now()
	if err = s.Probe(context.Background()); err != nil {
		return "disconnected", time.Since(started).Milliseconds(), err
	}
	return "connected", time.Since(started).Milliseconds(), nil
}

// QueryContext runs a read-only statement through the store's reader pool.
func (s *Store) QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error) {
	if s == nil || s.readDB == nil {
		return nil, errors.New("database read store is unavailable")
	}
	if s.sqlite && s.db != nil {
		return s.db.QueryContext(ctx, query, args...)
	}
	return s.readDB.QueryContext(ctx, query, args...)
}

// Query runs a read statement with a background context through the reader pool.
func (s *Store) Query(query string, args ...any) (*sql.Rows, error) {
	return s.QueryContext(context.Background(), query, args...)
}

// QueryRowContext runs a read-only statement through the store's reader pool.
func (s *Store) QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row {
	if s == nil || s.readDB == nil {
		return nil
	}
	if s.sqlite && s.db != nil {
		return s.db.QueryRowContext(ctx, query, args...)
	}
	return s.readDB.QueryRowContext(ctx, query, args...)
}

// QueryRow runs a read statement with a background context through the reader pool.
func (s *Store) QueryRow(query string, args ...any) *sql.Row {
	return s.QueryRowContext(context.Background(), query, args...)
}

// ExecContext runs one write statement through an immediate Store.Write
// transaction. Multi-statement read-then-write flows must use Write directly.
func (s *Store) ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error) {
	var result sql.Result
	err := s.Write(ctx, func(tx *sql.Tx) error {
		var err error
		result, err = tx.ExecContext(ctx, query, args...)
		return err
	})
	return result, err
}

// Exec runs one write statement with a background context in Store.Write.
func (s *Store) Exec(query string, args ...any) (sql.Result, error) {
	return s.ExecContext(context.Background(), query, args...)
}

// Read acquires a query-only connection from the read pool.
func (s *Store) Read(ctx context.Context) (*ReadSession, error) {
	if s == nil || s.readDB == nil {
		return nil, errors.New("SQLite read store is unavailable")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	conn, err := s.readDB.Conn(ctx)
	if err != nil {
		return nil, err
	}
	return &ReadSession{conn: conn}, nil
}

// Probe verifies the read-only pool and immediate-write transaction can each
// reach the database without leaving statements or transactions open.
func (s *Store) Probe(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}
	probeCtx, cancel := context.WithTimeout(ctx, TimeoutPing)
	defer cancel()
	reader, err := s.Read(probeCtx)
	if err != nil {
		return fmt.Errorf("read handle: %w", err)
	}
	rows, err := reader.QueryContext(probeCtx, "SELECT 1")
	if err != nil {
		_ = reader.Close()
		return fmt.Errorf("read SELECT 1: %w", err)
	}
	var readResult int
	if !rows.Next() {
		err = rows.Err()
		if err == nil {
			err = errors.New("read SELECT 1 returned no row")
		}
	} else if err = rows.Scan(&readResult); err == nil && readResult != 1 {
		err = fmt.Errorf("read SELECT 1 returned %d", readResult)
	}
	rowsCloseErr := rows.Close()
	readerCloseErr := reader.Close()
	if err != nil {
		return fmt.Errorf("read SELECT 1: %w", err)
	}
	if rowsCloseErr != nil {
		return fmt.Errorf("close read probe rows: %w", rowsCloseErr)
	}
	if readerCloseErr != nil {
		return fmt.Errorf("close read probe connection: %w", readerCloseErr)
	}
	return s.Write(probeCtx, func(tx *sql.Tx) error {
		var result int
		if err := tx.QueryRowContext(probeCtx, "SELECT 1").Scan(&result); err != nil {
			return fmt.Errorf("write SELECT 1: %w", err)
		}
		if result != 1 {
			return fmt.Errorf("write SELECT 1 returned %d", result)
		}
		return nil
	})
}

// QueryContext executes a read query using the acquired read-only connection.
func (r *ReadSession) QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error) {
	if r == nil || r.conn == nil {
		return nil, errors.New("SQLite read session is closed")
	}
	return r.conn.QueryContext(ctx, query, args...)
}

// QueryRowContext executes a read query on this read-only session.
func (r *ReadSession) QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row {
	if r == nil || r.conn == nil {
		return nil
	}
	return r.conn.QueryRowContext(ctx, query, args...)
}

// Close returns the read-only connection to its pool.
func (r *ReadSession) Close() error {
	if r == nil || r.conn == nil {
		return nil
	}
	return r.conn.Close()
}

// Vacuum compacts a local SQLite database outside a transaction. It is intended
// only for a registered maintenance task, never for a request handler.
func (s *Store) Vacuum(ctx context.Context) error {
	if s == nil || !s.sqlite || s.writeDB == nil {
		return errors.New("VACUUM is available only for local SQLite databases")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	started := time.Now()
	defer recordSQLiteWriteWait(started)
	_, err := s.writeDB.ExecContext(ctx, "VACUUM")
	return err
}

// Write runs fn in one immediate write transaction and retries SQLite busy
// errors within the ten-second transaction deadline.
func (s *Store) Write(ctx context.Context, fn func(*sql.Tx) error) error {
	if s == nil || s.writeDB == nil {
		return errors.New("SQLite write store is unavailable")
	}
	if fn == nil {
		return errors.New("SQLite write callback is nil")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if s.sqlite {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, sqliteStoreWriteTimeout)
		defer cancel()
	}
	var err error
	for attempt := 0; attempt < 5; attempt++ {
		err = s.writeOnce(ctx, fn)
		if !isSQLiteBusyError(err) {
			return err
		}
		metric.RecordDBBusy()
		if attempt == 4 {
			break
		}
		if waitErr := waitSQLiteRetry(ctx, attempt); waitErr != nil {
			return waitErr
		}
	}
	return err
}

func (s *Store) writeOnce(ctx context.Context, fn func(*sql.Tx) error) error {
	started := time.Now()
	tx, err := s.writeDB.BeginTx(ctx, nil)
	recordSQLiteWriteWait(started)
	if err != nil {
		return err
	}
	if err := fn(tx); err != nil {
		_ = tx.Rollback()
		return err
	}
	return tx.Commit()
}

// Close releases the facade and reader pool before checkpointing WAL and
// closing the writer, so no pooled reader can keep the checkpoint busy.
func (s *Store) Close() error {
	if s == nil {
		return nil
	}
	s.closeOnce.Do(func() {
		if !s.owned {
			return
		}
		var errs []error
		if s.db != nil {
			if err := s.db.Close(); err != nil {
				errs = append(errs, fmt.Errorf("close SQLite facade: %w", err))
			}
		}
		if s.readDB != nil {
			if err := s.readDB.Close(); err != nil {
				errs = append(errs, fmt.Errorf("close SQLite read handle: %w", err))
			}
		}
		if s.writeDB != nil {
			ctx, cancel := context.WithTimeout(context.Background(), TimeoutPing)
			var busy, logFrames, checkpointed int
			err := s.writeDB.QueryRowContext(ctx, "PRAGMA wal_checkpoint(TRUNCATE)").Scan(&busy, &logFrames, &checkpointed)
			cancel()
			if err != nil {
				errs = append(errs, fmt.Errorf("checkpoint SQLite WAL: %w", err))
			} else if busy != 0 {
				errs = append(errs, fmt.Errorf("checkpoint SQLite WAL is busy: %d frames remain", logFrames-checkpointed))
			}
		}
		if s.writeDB != nil {
			if err := s.writeDB.Close(); err != nil {
				errs = append(errs, fmt.Errorf("close SQLite write handle: %w", err))
			}
		}
		s.closeErr = errors.Join(errs...)
		if s.db != nil && s.sqlite {
			sqliteStoresByDB.Delete(s.db)
		}
	})
	return s.closeErr
}

type sqliteStoreConnector struct {
	store *Store
}

func (c sqliteStoreConnector) Connect(context.Context) (driver.Conn, error) {
	return &sqliteStoreConn{store: c.store}, nil
}

func (c sqliteStoreConnector) Driver() driver.Driver {
	return sqliteStoreDriver{}
}

type sqliteStoreDriver struct{}

func (sqliteStoreDriver) Open(string) (driver.Conn, error) {
	return nil, errors.New("SQLite store driver must be opened with its connector")
}

type sqliteStoreConn struct {
	store    *Store
	tx       *sql.Tx
	txCancel context.CancelFunc
}

func (c *sqliteStoreConn) Prepare(query string) (driver.Stmt, error) {
	return &sqliteStoreStmt{conn: c, query: query}, nil
}

func (c *sqliteStoreConn) PrepareContext(_ context.Context, query string) (driver.Stmt, error) {
	return c.Prepare(query)
}

func (*sqliteStoreConn) Close() error {
	return nil
}

func (c *sqliteStoreConn) Begin() (driver.Tx, error) {
	return c.BeginTx(context.Background(), driver.TxOptions{})
}

func (c *sqliteStoreConn) BeginTx(ctx context.Context, _ driver.TxOptions) (driver.Tx, error) {
	if c.tx != nil {
		return nil, errors.New("SQLite store transaction is already active")
	}
	started := time.Now()
	tx, cancel, err := c.store.beginTransaction(ctx)
	recordSQLiteWriteWait(started)
	if err != nil {
		return nil, err
	}
	c.tx = tx
	c.txCancel = cancel
	return &sqliteStoreTx{conn: c, tx: tx, cancel: cancel}, nil
}

func (s *Store) beginTransaction(ctx context.Context) (*sql.Tx, context.CancelFunc, error) {
	deadlineCtx, cancel := context.WithTimeout(ctx, sqliteStoreWriteTimeout)
	var tx *sql.Tx
	var err error
	for attempt := 0; attempt < 5; attempt++ {
		tx, err = s.writeDB.BeginTx(deadlineCtx, nil)
		if !isSQLiteBusyError(err) {
			if err != nil {
				cancel()
				return nil, nil, err
			}
			return tx, cancel, nil
		}
		metric.RecordDBBusy()
		if attempt == 4 {
			break
		}
		if waitErr := waitSQLiteRetry(deadlineCtx, attempt); waitErr != nil {
			cancel()
			return nil, nil, waitErr
		}
	}
	cancel()
	return nil, nil, err
}

func (c *sqliteStoreConn) ExecContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Result, error) {
	values, err := sqliteNamedValues(args)
	if err != nil {
		return nil, err
	}
	if c.tx != nil {
		result, err := c.tx.ExecContext(ctx, query, values...)
		return sqliteDriverResult(result, err)
	}
	var result sql.Result
	err = c.store.Write(ctx, func(tx *sql.Tx) error {
		var execErr error
		result, execErr = tx.ExecContext(ctx, query, values...)
		return execErr
	})
	return sqliteDriverResult(result, err)
}

func (c *sqliteStoreConn) QueryContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
	values, err := sqliteNamedValues(args)
	if err != nil {
		return nil, err
	}
	if c.tx != nil {
		rows, err := c.tx.QueryContext(ctx, query, values...)
		if err != nil {
			return nil, err
		}
		return &sqliteStoreRows{rows: rows}, nil
	}
	if isSQLiteReadQuery(query) {
		conn, err := c.store.readDB.Conn(ctx)
		if err != nil {
			return nil, err
		}
		rows, err := conn.QueryContext(ctx, query, values...)
		if err != nil {
			_ = conn.Close()
			return nil, err
		}
		return &sqliteStoreRows{rows: rows, conn: conn}, nil
	}
	tx, cancel, err := c.store.beginTransaction(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := tx.QueryContext(ctx, query, values...)
	if err != nil {
		_ = tx.Rollback()
		cancel()
		return nil, err
	}
	return &sqliteStoreRows{rows: rows, tx: tx, txCancel: cancel}, nil
}

func (c *sqliteStoreConn) Ping(ctx context.Context) error {
	return c.store.readDB.PingContext(ctx)
}

type sqliteStoreTx struct {
	conn   *sqliteStoreConn
	tx     *sql.Tx
	cancel context.CancelFunc
}

func (t *sqliteStoreTx) Commit() error {
	err := t.tx.Commit()
	t.conn.tx = nil
	t.conn.txCancel = nil
	t.cancel()
	return err
}

func (t *sqliteStoreTx) Rollback() error {
	err := t.tx.Rollback()
	t.conn.tx = nil
	t.conn.txCancel = nil
	t.cancel()
	return err
}

type sqliteStoreStmt struct {
	conn  *sqliteStoreConn
	query string
}

func (*sqliteStoreStmt) Close() error {
	return nil
}

func (*sqliteStoreStmt) NumInput() int {
	return -1
}

func (s *sqliteStoreStmt) Exec(args []driver.Value) (driver.Result, error) {
	return s.ExecContext(context.Background(), sqliteValuesToNamed(args))
}

func (s *sqliteStoreStmt) ExecContext(ctx context.Context, args []driver.NamedValue) (driver.Result, error) {
	return s.conn.ExecContext(ctx, s.query, args)
}

func (s *sqliteStoreStmt) Query(args []driver.Value) (driver.Rows, error) {
	return s.QueryContext(context.Background(), sqliteValuesToNamed(args))
}

func (s *sqliteStoreStmt) QueryContext(ctx context.Context, args []driver.NamedValue) (driver.Rows, error) {
	return s.conn.QueryContext(ctx, s.query, args)
}

type sqliteStoreRows struct {
	rows     *sql.Rows
	conn     *sql.Conn
	tx       *sql.Tx
	txCancel context.CancelFunc
	close    sync.Once
	err      error
}

func (r *sqliteStoreRows) Columns() []string {
	columns, err := r.rows.Columns()
	if err != nil {
		r.err = err
		return nil
	}
	return columns
}

func (r *sqliteStoreRows) Close() error {
	r.close.Do(func() {
		if err := r.rows.Close(); err != nil {
			r.err = err
		}
		if r.tx != nil {
			if r.err != nil {
				r.err = errors.Join(r.err, r.tx.Rollback())
			} else {
				r.err = r.tx.Commit()
			}
			r.txCancel()
		}
		if r.conn != nil {
			r.err = errors.Join(r.err, r.conn.Close())
		}
	})
	return r.err
}

func (r *sqliteStoreRows) Next(dest []driver.Value) error {
	if !r.rows.Next() {
		if err := r.rows.Err(); err != nil {
			r.err = err
			return err
		}
		return io.EOF
	}
	values := make([]any, len(dest))
	for index := range values {
		values[index] = new(any)
	}
	if err := r.rows.Scan(values...); err != nil {
		r.err = err
		return err
	}
	for index, value := range values {
		scanned := *(value.(*any))
		converted, err := driver.DefaultParameterConverter.ConvertValue(scanned)
		if err != nil {
			r.err = err
			return err
		}
		dest[index] = converted
	}
	return nil
}

func sqliteDriverResult(result sql.Result, err error) (driver.Result, error) {
	if err != nil {
		return nil, err
	}
	lastID, err := result.LastInsertId()
	if err != nil {
		return nil, err
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return nil, err
	}
	return sqliteStoreResult{lastID: lastID, rowsAffected: rowsAffected}, nil
}

type sqliteStoreResult struct {
	lastID       int64
	rowsAffected int64
}

func (r sqliteStoreResult) LastInsertId() (int64, error) {
	return r.lastID, nil
}

func (r sqliteStoreResult) RowsAffected() (int64, error) {
	return r.rowsAffected, nil
}

func sqliteNamedValues(args []driver.NamedValue) ([]any, error) {
	values := make([]any, len(args))
	for index, arg := range args {
		if arg.Name != "" {
			values[index] = sql.Named(arg.Name, arg.Value)
		} else {
			values[index] = arg.Value
		}
	}
	return values, nil
}

func sqliteValuesToNamed(values []driver.Value) []driver.NamedValue {
	args := make([]driver.NamedValue, len(values))
	for index, value := range values {
		args[index] = driver.NamedValue{Ordinal: index + 1, Value: value}
	}
	return args
}

func isSQLiteReadQuery(query string) bool {
	query = strings.TrimSpace(query)
	for strings.HasPrefix(query, "--") {
		if newline := strings.IndexByte(query, '\n'); newline >= 0 {
			query = strings.TrimSpace(query[newline+1:])
		} else {
			return false
		}
	}
	if len(query) >= 2 && query[:2] == "/*" {
		if end := strings.Index(query[2:], "*/"); end >= 0 {
			query = strings.TrimSpace(query[end+4:])
		} else {
			return false
		}
	}
	upper := strings.ToUpper(query)
	if strings.HasPrefix(upper, "SELECT") || strings.HasPrefix(upper, "VALUES") || strings.HasPrefix(upper, "EXPLAIN") {
		return true
	}
	if strings.HasPrefix(upper, "PRAGMA") {
		pragma := strings.TrimSpace(strings.TrimPrefix(upper, "PRAGMA"))
		if pragma == "" {
			return false
		}
		nameEnd := strings.IndexAny(pragma, " (=;")
		if nameEnd >= 0 {
			pragma = pragma[:nameEnd]
		}
		name := strings.ToLower(pragma)
		switch name {
		case "application_id", "collation_list", "compile_options", "database_list", "foreign_key_check", "foreign_key_list", "function_list", "index_info", "index_list", "index_xinfo", "integrity_check", "module_list", "page_count", "page_size", "table_info", "table_list", "table_xinfo", "user_version":
			return !strings.Contains(query, "=")
		}
	}
	return false
}

func init() {
	var _ driver.Connector = sqliteStoreConnector{}
	var _ driver.Driver = sqliteStoreDriver{}
	var _ driver.Conn = (*sqliteStoreConn)(nil)
	var _ driver.ConnBeginTx = (*sqliteStoreConn)(nil)
	var _ driver.ExecerContext = (*sqliteStoreConn)(nil)
	var _ driver.QueryerContext = (*sqliteStoreConn)(nil)
	var _ driver.Pinger = (*sqliteStoreConn)(nil)
	var _ driver.Stmt = (*sqliteStoreStmt)(nil)
	var _ driver.StmtExecContext = (*sqliteStoreStmt)(nil)
	var _ driver.StmtQueryContext = (*sqliteStoreStmt)(nil)
	var _ driver.Tx = (*sqliteStoreTx)(nil)
	var _ driver.Rows = (*sqliteStoreRows)(nil)
	var _ driver.Result = driver.RowsAffected(0)
}
