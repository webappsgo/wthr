// Package database provides database connection and query helpers
// AI.md PART 10: All database queries MUST have timeouts
package database

import (
	"context"
	"database/sql"
	"fmt"
	"time"
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

// QueryContext executes a query with timeout and returns rows
func QueryContext(ctx context.Context, db *sql.DB, timeout time.Duration, query string, args ...interface{}) (*sql.Rows, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	return db.QueryContext(ctx, query, args...)
}

// ExecContext executes a statement with timeout
func ExecContext(ctx context.Context, db *sql.DB, timeout time.Duration, query string, args ...interface{}) (sql.Result, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	return db.ExecContext(ctx, query, args...)
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

	switch {
	case err == context.DeadlineExceeded:
		return fmt.Errorf("TIMEOUT: Query timed out")
	case err == sql.ErrNoRows:
		return fmt.Errorf("NOT_FOUND: Resource not found")
	case err == context.Canceled:
		return fmt.Errorf("CANCELED: Request was canceled")
	default:
		return fmt.Errorf("DATABASE_ERROR: %w", err)
	}
}

// WithTransaction executes a function within a transaction with timeout
// AI.md PART 10: Transaction timeout = 30 seconds
func WithTransaction(ctx context.Context, db *sql.DB, fn func(*sql.Tx) error) error {
	ctx, cancel := context.WithTimeout(ctx, TimeoutTransaction)
	defer cancel()

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}

	if err := fn(tx); err != nil {
		tx.Rollback()
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
