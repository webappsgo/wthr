// Package database provides database connection and query helpers
// AI.md PART 10: All database queries MUST have timeouts
package database

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"math/rand"
	"reflect"
	"runtime"
	"strings"
	"time"

	"github.com/webappsgo/wthr/src/server/metric"
)

// Query timeout constants per AI.md PART 10
const (
	TimeoutSimpleSelect  = 5 * time.Second
	TimeoutComplexSelect = 15 * time.Second
	TimeoutWrite         = 10 * time.Second
	TimeoutBulk          = 60 * time.Second
	TimeoutMigration     = 5 * time.Minute
	TimeoutReport        = 2 * time.Minute
	TimeoutTransaction   = 30 * time.Second
	TimeoutPing          = 5 * time.Second
)

// PoolConfig holds connection pool settings per AI.md PART 10
type PoolConfig struct {
	MaxOpen     int
	MaxIdle     int
	MaxLifetime time.Duration
	MaxIdleTime time.Duration
}

// DefaultPoolConfig returns pool settings for small deployment
// AI.md PART 10: Small (1-2 nodes) = 25 open, 5 idle
func DefaultPoolConfig() PoolConfig {
	return PoolConfig{
		MaxOpen:     25,
		MaxIdle:     5,
		MaxLifetime: 5 * time.Minute,
		MaxIdleTime: 1 * time.Minute,
	}
}

// DevPoolConfig returns pool settings for development
// AI.md PART 10: Development = 5 open, 2 idle
func DevPoolConfig() PoolConfig {
	return PoolConfig{
		MaxOpen:     5,
		MaxIdle:     2,
		MaxLifetime: 5 * time.Minute,
		MaxIdleTime: 1 * time.Minute,
	}
}

// ApplyPoolConfig applies pool configuration to database
func ApplyPoolConfig(db *sql.DB, cfg PoolConfig) {
	db.SetMaxOpenConns(cfg.MaxOpen)
	db.SetMaxIdleConns(cfg.MaxIdle)
	db.SetConnMaxLifetime(cfg.MaxLifetime)
	db.SetConnMaxIdleTime(cfg.MaxIdleTime)
}

// QueryRowContext executes a query with timeout and returns a single row.
// The row is scanned lazily by the caller (Row.Scan), so cancel must NOT run
// before this function returns (that would cancel the query before Scan
// executes) — release the context once the timeout itself elapses instead.
func QueryRowContext(ctx context.Context, db *sql.DB, timeout time.Duration, query string, args ...interface{}) *sql.Row {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	time.AfterFunc(timeout, cancel)
	return db.QueryRowContext(ctx, query, args...)
}

// QueryContext executes a query with timeout and returns rows.
// The rows are iterated lazily by the caller (Rows.Next/Scan) after this
// function returns, so cancel must NOT run on return (that would cancel the
// query before iteration completes, same hazard as QueryRowContext) —
// release the context once the timeout itself elapses instead.
func QueryContext(ctx context.Context, db *sql.DB, timeout time.Duration, query string, args ...interface{}) (*sql.Rows, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		cancel()
		return nil, err
	}
	time.AfterFunc(timeout, cancel)
	return rows, nil
}

// ExecContext executes a statement with timeout
func ExecContext(ctx context.Context, db *sql.DB, timeout time.Duration, query string, args ...interface{}) (sql.Result, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	if !isSQLiteDB(db) {
		return db.ExecContext(ctx, query, args...)
	}

	started := time.Now()
	result, err := execSQLiteWithRetry(ctx, db, query, args...)
	recordSQLiteWriteWait(started)
	return result, err
}

const sqliteWriteMaxRetries = 8

func execSQLiteWithRetry(ctx context.Context, db *sql.DB, query string, args ...interface{}) (sql.Result, error) {
	for attempt := 0; ; attempt++ {
		result, err := db.ExecContext(ctx, query, args...)
		if !isSQLiteBusyError(err) || attempt >= sqliteWriteMaxRetries {
			return result, err
		}
		metric.RecordDBBusy()
		if err := waitSQLiteRetry(ctx, attempt); err != nil {
			return nil, err
		}
	}
}

func waitSQLiteRetry(ctx context.Context, attempt int) error {
	delay := 25 * time.Millisecond * time.Duration(1<<attempt)
	jitter := time.Duration(rand.Int63n(int64(delay/2) + 1))
	timer := time.NewTimer(delay + jitter)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func recordSQLiteWriteWait(started time.Time) {
	wait := time.Since(started)
	metric.RecordDBWriteWait(wait)
	if wait > time.Second {
		log.Printf("WARN db.busy wait_ms=%d caller=%s", wait.Milliseconds(), databaseCaller())
	}
}

func isSQLiteDB(db *sql.DB) bool {
	if db == nil {
		return false
	}
	t := reflect.TypeOf(db.Driver())
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	return t.PkgPath() == "modernc.org/sqlite"
}

func isSQLiteBusyError(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToUpper(err.Error())
	return strings.Contains(message, "SQLITE_BUSY") || strings.Contains(message, "SQLITE_LOCKED") ||
		strings.Contains(message, "DATABASE IS BUSY") || strings.Contains(message, "DATABASE IS LOCKED") ||
		strings.Contains(message, "DATABASE TABLE IS LOCKED") || strings.Contains(message, "DATABASE SCHEMA IS LOCKED")
}

func databaseCaller() string {
	pc, _, _, ok := runtime.Caller(2)
	if !ok {
		return "unknown"
	}
	if caller := runtime.FuncForPC(pc); caller != nil {
		return caller.Name()
	}
	return "unknown"
}

// WithTimeout creates a context with the specified timeout
func WithTimeout(timeout time.Duration) (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), timeout)
}

// HandleQueryError returns appropriate error message based on error type
// AI.md PART 10: Proper error handling for timeouts
func HandleQueryError(err error) error {
	if err == nil {
		return nil
	}

	switch err {
	case context.DeadlineExceeded:
		return fmt.Errorf("TIMEOUT: Query timed out")
	case sql.ErrNoRows:
		return fmt.Errorf("NOT_FOUND: Resource not found")
	case context.Canceled:
		return fmt.Errorf("CANCELED: Request was canceled")
	default:
		return fmt.Errorf("DATABASE_ERROR: %w", err)
	}
}

// WithTransaction executes a function within a transaction with timeout
// AI.md PART 10: Transaction timeout = 30 seconds. On SQLite, the callback is
// invoked at most once because it may perform non-database side effects.
func WithTransaction(ctx context.Context, db *sql.DB, fn func(*sql.Tx) error) error {
	ctx, cancel := context.WithTimeout(ctx, TimeoutTransaction)
	defer cancel()
	if !isSQLiteDB(db) {
		return withSingleTransaction(ctx, db, fn)
	}
	started := time.Now()
	defer recordSQLiteWriteWait(started)

	for attempt := 0; ; attempt++ {
		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			if isSQLiteBusyError(err) && attempt < sqliteWriteMaxRetries {
				metric.RecordDBBusy()
				if retryErr := waitSQLiteRetry(ctx, attempt); retryErr != nil {
					return retryErr
				}
				continue
			}
			return fmt.Errorf("failed to begin transaction: %w", err)
		}

		if err := fn(tx); err != nil {
			_ = tx.Rollback()
			return err
		}

		// Commit errors are returned without re-running fn: a driver may have
		// committed before surfacing an error, so repeating the callback could
		// duplicate side effects.
		return tx.Commit()
	}
}

func withSingleTransaction(ctx context.Context, db *sql.DB, fn func(*sql.Tx) error) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	if err := fn(tx); err != nil {
		_ = tx.Rollback()
		return err
	}
	return tx.Commit()
}

// PingWithTimeout tests database connection with timeout
func PingWithTimeout(db *sql.DB) error {
	ctx, cancel := context.WithTimeout(context.Background(), TimeoutPing)
	defer cancel()
	return db.PingContext(ctx)
}
