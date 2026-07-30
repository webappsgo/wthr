package database

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

// openMemDB opens a fresh, uniquely-named in-memory SQLite database shared
// across the connection pool (file:name?mode=memory&cache=shared), matching
// the DSN pattern used throughout this codebase's tests and connection.go.
func openMemDB(t *testing.T, name string) *sql.DB {
	t.Helper()
	dsn := fmt.Sprintf("file:%s_%d?mode=memory&cache=shared", name, time.Now().UnixNano())
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		t.Fatalf("failed to open in-memory db: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	if err := db.Ping(); err != nil {
		t.Fatalf("failed to ping in-memory db: %v", err)
	}
	return db
}

// TestDefaultPoolConfig and TestDevPoolConfig assert the exact numeric
// contract from AI.md PART 10 (small deployment vs development tiers).
func TestDefaultPoolConfig(t *testing.T) {
	cfg := DefaultPoolConfig()
	if cfg.MaxOpen != 25 {
		t.Errorf("MaxOpen = %d, want 25", cfg.MaxOpen)
	}
	if cfg.MaxIdle != 5 {
		t.Errorf("MaxIdle = %d, want 5", cfg.MaxIdle)
	}
	if cfg.MaxLifetime != 5*time.Minute {
		t.Errorf("MaxLifetime = %v, want 5m", cfg.MaxLifetime)
	}
	if cfg.MaxIdleTime != 1*time.Minute {
		t.Errorf("MaxIdleTime = %v, want 1m", cfg.MaxIdleTime)
	}
}

func TestDevPoolConfig(t *testing.T) {
	cfg := DevPoolConfig()
	if cfg.MaxOpen != 5 {
		t.Errorf("MaxOpen = %d, want 5", cfg.MaxOpen)
	}
	if cfg.MaxIdle != 2 {
		t.Errorf("MaxIdle = %d, want 2", cfg.MaxIdle)
	}
	if cfg.MaxLifetime != 5*time.Minute {
		t.Errorf("MaxLifetime = %v, want 5m", cfg.MaxLifetime)
	}
	if cfg.MaxIdleTime != 1*time.Minute {
		t.Errorf("MaxIdleTime = %v, want 1m", cfg.MaxIdleTime)
	}
}

// TestApplyPoolConfig verifies pool settings are actually applied to the
// *sql.DB via db.Stats(), not merely accepted without effect.
func TestApplyPoolConfig(t *testing.T) {
	tests := []struct {
		name string
		cfg  PoolConfig
	}{
		{"default pool", DefaultPoolConfig()},
		{"dev pool", DevPoolConfig()},
		{"custom small pool", PoolConfig{MaxOpen: 3, MaxIdle: 1, MaxLifetime: time.Second, MaxIdleTime: time.Second}},
		{"zero pool disables limits", PoolConfig{MaxOpen: 0, MaxIdle: 0, MaxLifetime: 0, MaxIdleTime: 0}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db := openMemDB(t, "pool")
			ApplyPoolConfig(db, tt.cfg)

			stats := db.Stats()
			if stats.MaxOpenConnections != tt.cfg.MaxOpen {
				t.Errorf("MaxOpenConnections = %d, want %d", stats.MaxOpenConnections, tt.cfg.MaxOpen)
			}
		})
	}
}

// TestApplyPoolConfig_EnforcesMaxOpen verifies MaxOpenConns is a real cap:
// concurrently holding more connections than configured must block until one
// is released, rather than silently allowing unbounded connections.
func TestApplyPoolConfig_EnforcesMaxOpen(t *testing.T) {
	db := openMemDB(t, "poolcap")
	ApplyPoolConfig(db, PoolConfig{MaxOpen: 1, MaxIdle: 1, MaxLifetime: time.Minute, MaxIdleTime: time.Minute})

	conn1, err := db.Conn(context.Background())
	if err != nil {
		t.Fatalf("failed to acquire first conn: %v", err)
	}
	defer conn1.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	_, err = db.Conn(ctx)
	if err == nil {
		t.Error("expected second connection acquisition to block/fail when MaxOpen=1 and first conn held, got nil error")
	}
}

func TestWithTimeout(t *testing.T) {
	ctx, cancel := WithTimeout(50 * time.Millisecond)
	defer cancel()

	select {
	case <-ctx.Done():
		t.Fatal("context deadline exceeded before intended timeout")
	default:
	}

	deadline, ok := ctx.Deadline()
	if !ok {
		t.Fatal("expected context to have a deadline")
	}
	if time.Until(deadline) > 50*time.Millisecond {
		t.Errorf("deadline too far in future: %v", time.Until(deadline))
	}

	<-ctx.Done()
	if !errors.Is(ctx.Err(), context.DeadlineExceeded) {
		t.Errorf("ctx.Err() = %v, want DeadlineExceeded", ctx.Err())
	}
}

func TestHandleQueryError(t *testing.T) {
	tests := []struct {
		name    string
		in      error
		wantNil bool
		wantSub string
	}{
		{"nil error passthrough", nil, true, ""},
		{"deadline exceeded", context.DeadlineExceeded, false, "TIMEOUT"},
		{"no rows", sql.ErrNoRows, false, "NOT_FOUND"},
		{"canceled", context.Canceled, false, "CANCELED"},
		{"generic wrapped error", fmt.Errorf("boom"), false, "DATABASE_ERROR"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := HandleQueryError(tt.in)
			if tt.wantNil {
				if got != nil {
					t.Errorf("HandleQueryError(nil) = %v, want nil", got)
				}
				return
			}
			if got == nil {
				t.Fatalf("HandleQueryError(%v) = nil, want error containing %q", tt.in, tt.wantSub)
			}
			if !contains(got.Error(), tt.wantSub) {
				t.Errorf("HandleQueryError(%v) = %q, want substring %q", tt.in, got.Error(), tt.wantSub)
			}
		})
	}
}

func contains(s, sub string) bool {
	return len(sub) == 0 || (len(s) >= len(sub) && (func() bool {
		for i := 0; i+len(sub) <= len(s); i++ {
			if s[i:i+len(sub)] == sub {
				return true
			}
		}
		return false
	})())
}

func TestPingWithTimeout(t *testing.T) {
	t.Run("succeeds on open db", func(t *testing.T) {
		db := openMemDB(t, "ping")
		if err := PingWithTimeout(db); err != nil {
			t.Errorf("PingWithTimeout() = %v, want nil", err)
		}
	})

	t.Run("fails on closed db", func(t *testing.T) {
		db := openMemDB(t, "pingclosed")
		db.Close()
		if err := PingWithTimeout(db); err == nil {
			t.Error("PingWithTimeout() on closed db = nil, want error")
		}
	})
}

func TestQueryRowContext(t *testing.T) {
	db := openMemDB(t, "qrctx")
	if _, err := db.Exec("CREATE TABLE t (id INTEGER PRIMARY KEY, val TEXT)"); err != nil {
		t.Fatalf("setup: %v", err)
	}
	if _, err := db.Exec("INSERT INTO t (id, val) VALUES (1, 'hello')"); err != nil {
		t.Fatalf("setup: %v", err)
	}

	var val string
	err := QueryRowContext(context.Background(), db, TimeoutSimpleSelect, "SELECT val FROM t WHERE id = ?", 1).Scan(&val)
	if err != nil {
		t.Fatalf("QueryRowContext: %v", err)
	}
	if val != "hello" {
		t.Errorf("val = %q, want %q", val, "hello")
	}

	// Row for a nonexistent id must surface sql.ErrNoRows, not swallow it.
	err = QueryRowContext(context.Background(), db, TimeoutSimpleSelect, "SELECT val FROM t WHERE id = ?", 999).Scan(&val)
	if !errors.Is(err, sql.ErrNoRows) {
		t.Errorf("err = %v, want sql.ErrNoRows", err)
	}
}

func TestQueryRowContext_TimeoutExpires(t *testing.T) {
	db := openMemDB(t, "qrctxtimeout")
	// An already-expired parent context must cause the derived context to be done immediately.
	ctx, cancel := context.WithTimeout(context.Background(), -1*time.Second)
	defer cancel()

	var val int
	err := QueryRowContext(ctx, db, TimeoutSimpleSelect, "SELECT 1").Scan(&val)
	if err == nil {
		t.Error("expected error from already-expired context, got nil")
	}
}

func TestQueryContext(t *testing.T) {
	db := openMemDB(t, "qctx")
	if _, err := db.Exec("CREATE TABLE t (id INTEGER PRIMARY KEY)"); err != nil {
		t.Fatalf("setup: %v", err)
	}
	for i := 1; i <= 3; i++ {
		if _, err := db.Exec("INSERT INTO t (id) VALUES (?)", i); err != nil {
			t.Fatalf("setup: %v", err)
		}
	}

	rows, err := QueryContext(context.Background(), db, TimeoutSimpleSelect, "SELECT id FROM t ORDER BY id")
	if err != nil {
		t.Fatalf("QueryContext: %v", err)
	}
	defer rows.Close()

	var count int
	for rows.Next() {
		count++
	}
	if count != 3 {
		t.Errorf("row count = %d, want 3", count)
	}
}

func TestExecContext(t *testing.T) {
	db := openMemDB(t, "exctx")
	if _, err := db.Exec("CREATE TABLE t (id INTEGER PRIMARY KEY, val TEXT)"); err != nil {
		t.Fatalf("setup: %v", err)
	}

	res, err := ExecContext(context.Background(), db, TimeoutWrite, "INSERT INTO t (id, val) VALUES (1, 'x')")
	if err != nil {
		t.Fatalf("ExecContext: %v", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		t.Fatalf("RowsAffected: %v", err)
	}
	if n != 1 {
		t.Errorf("RowsAffected = %d, want 1", n)
	}

	// Invalid SQL must return an error, not panic or silently succeed.
	if _, err := ExecContext(context.Background(), db, TimeoutWrite, "NOT VALID SQL"); err == nil {
		t.Error("expected error for invalid SQL, got nil")
	}
}

func TestWithTransaction(t *testing.T) {
	t.Run("commits on success", func(t *testing.T) {
		db := openMemDB(t, "txcommit")
		if _, err := db.Exec("CREATE TABLE t (id INTEGER PRIMARY KEY)"); err != nil {
			t.Fatalf("setup: %v", err)
		}

		err := WithTransaction(context.Background(), db, func(tx *sql.Tx) error {
			_, err := tx.Exec("INSERT INTO t (id) VALUES (1)")
			return err
		})
		if err != nil {
			t.Fatalf("WithTransaction: %v", err)
		}

		var count int
		if err := db.QueryRow("SELECT COUNT(*) FROM t").Scan(&count); err != nil {
			t.Fatalf("verify: %v", err)
		}
		if count != 1 {
			t.Errorf("count = %d, want 1 (commit should have persisted the row)", count)
		}
	})

	t.Run("rolls back on function error", func(t *testing.T) {
		db := openMemDB(t, "txrollback")
		if _, err := db.Exec("CREATE TABLE t (id INTEGER PRIMARY KEY)"); err != nil {
			t.Fatalf("setup: %v", err)
		}

		wantErr := errors.New("intentional failure")
		err := WithTransaction(context.Background(), db, func(tx *sql.Tx) error {
			if _, err := tx.Exec("INSERT INTO t (id) VALUES (1)"); err != nil {
				return err
			}
			return wantErr
		})
		if !errors.Is(err, wantErr) {
			t.Fatalf("WithTransaction err = %v, want %v", err, wantErr)
		}

		var count int
		if err := db.QueryRow("SELECT COUNT(*) FROM t").Scan(&count); err != nil {
			t.Fatalf("verify: %v", err)
		}
		if count != 0 {
			t.Errorf("count = %d, want 0 (rollback should have discarded the row)", count)
		}
	})

	t.Run("returns error when begin fails on closed db", func(t *testing.T) {
		db := openMemDB(t, "txclosed")
		db.Close()

		err := WithTransaction(context.Background(), db, func(tx *sql.Tx) error {
			t.Error("transaction function should not run when BeginTx fails")
			return nil
		})
		if err == nil {
			t.Error("expected error from WithTransaction on closed db, got nil")
		}
	})
}
