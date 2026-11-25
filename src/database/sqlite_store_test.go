package database

import (
	"context"
	"database/sql"
	"fmt"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func TestOpenSQLiteCreatesSeparateWALHandles(t *testing.T) {
	store, err := OpenSQLite(filepath.Join(t.TempDir(), "store.db"), 7)
	if err != nil {
		t.Fatalf("OpenSQLite: %v", err)
	}
	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Errorf("close store: %v", err)
		}
	})
	if store.writeDB == store.readDB {
		t.Fatal("write and read handles must be distinct")
	}
	if got := store.writeDB.Stats().MaxOpenConnections; got != 1 {
		t.Errorf("write max open connections = %d, want 1", got)
	}
	if got := store.readDB.Stats().MaxOpenConnections; got != 7 {
		t.Errorf("read max open connections = %d, want 7", got)
	}
	var journalMode string
	if err := store.writeDB.QueryRow("PRAGMA journal_mode").Scan(&journalMode); err != nil {
		t.Fatalf("read journal mode: %v", err)
	}
	if journalMode != "wal" {
		t.Errorf("journal mode = %q, want wal", journalMode)
	}
	var queryOnly int
	if err := store.readDB.QueryRow("PRAGMA query_only").Scan(&queryOnly); err != nil {
		t.Fatalf("read query_only: %v", err)
	}
	if queryOnly != 1 {
		t.Errorf("read query_only = %d, want 1", queryOnly)
	}
}

func TestSQLiteStoreRoutesReadsAndWrites(t *testing.T) {
	store, err := OpenSQLite(filepath.Join(t.TempDir(), "store.db"), 4)
	if err != nil {
		t.Fatalf("OpenSQLite: %v", err)
	}
	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Errorf("close store: %v", err)
		}
	})
	if err := store.Write(context.Background(), func(tx *sql.Tx) error {
		_, err := tx.ExecContext(context.Background(), "CREATE TABLE values_table (value INTEGER NOT NULL)")
		return err
	}); err != nil {
		t.Fatalf("create table through Store.Write: %v", err)
	}
	if _, err := store.ExecContext(context.Background(), "INSERT INTO values_table (value) VALUES (1)"); err != nil {
		t.Fatalf("insert through Store.ExecContext: %v", err)
	}
	reader, err := store.Read(context.Background())
	if err != nil {
		t.Fatalf("acquire reader: %v", err)
	}
	rows, err := reader.QueryContext(context.Background(), "SELECT value FROM values_table")
	if err != nil {
		_ = reader.Close()
		t.Fatalf("read through Store.Read: %v", err)
	}
	if !rows.Next() {
		t.Fatalf("read row: %v", rows.Err())
	}
	var value int
	if err := rows.Scan(&value); err != nil {
		t.Fatalf("scan value: %v", err)
	}
	if err := rows.Close(); err != nil {
		t.Fatalf("close rows: %v", err)
	}
	if err := reader.Close(); err != nil {
		t.Fatalf("close reader: %v", err)
	}
	if value != 1 {
		t.Errorf("stored value = %d, want 1", value)
	}
	reader, err = store.Read(context.Background())
	if err != nil {
		t.Fatalf("acquire second reader: %v", err)
	}
	defer func() { _ = reader.Close() }()
	if _, err := reader.QueryContext(context.Background(), "INSERT INTO values_table (value) VALUES (2)"); err == nil {
		t.Fatal("read handle accepted a write")
	}
}

func TestSQLiteStoreTreatsIntegrityCheckAsReadOnly(t *testing.T) {
	if !isSQLiteReadQuery("PRAGMA integrity_check") {
		t.Fatal("PRAGMA integrity_check is not routed through the query-only reader")
	}
}

func TestSQLiteStoreConcurrentReadersAndWriters(t *testing.T) {
	store, err := OpenSQLite(filepath.Join(t.TempDir(), "concurrent.db"), 25)
	if err != nil {
		t.Fatalf("OpenSQLite: %v", err)
	}
	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Errorf("close store: %v", err)
		}
	})
	if err := store.Write(context.Background(), func(tx *sql.Tx) error {
		_, err := tx.ExecContext(context.Background(), "CREATE TABLE writes (id INTEGER PRIMARY KEY, value TEXT NOT NULL)")
		return err
	}); err != nil {
		t.Fatalf("create table: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	errCh := make(chan error, 40)
	var workers sync.WaitGroup
	for worker := 0; worker < 20; worker++ {
		workers.Add(1)
		go func(worker int) {
			defer workers.Done()
			for ctx.Err() == nil {
				err := store.Write(ctx, func(tx *sql.Tx) error {
					_, err := tx.ExecContext(ctx, "INSERT INTO writes (value) VALUES (?)", fmt.Sprintf("writer-%d", worker))
					return err
				})
				if err != nil {
					if ctx.Err() == nil {
						errCh <- err
					}
					return
				}
			}
		}(worker)
	}
	for worker := 0; worker < 20; worker++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			for ctx.Err() == nil {
				reader, err := store.Read(ctx)
				if err != nil {
					if ctx.Err() == nil {
						errCh <- err
					}
					return
				}
				rows, err := reader.QueryContext(ctx, "SELECT COUNT(*) FROM writes")
				if err != nil {
					_ = reader.Close()
					if ctx.Err() == nil {
						errCh <- err
					}
					return
				}
				if rows.Next() {
					var count int
					err = rows.Scan(&count)
				} else if rows.Err() != nil {
					err = rows.Err()
				} else {
					err = fmt.Errorf("reader returned no count row")
				}
				if closeErr := rows.Close(); err == nil {
					err = closeErr
				}
				if closeErr := reader.Close(); err == nil {
					err = closeErr
				}
				if err != nil {
					if ctx.Err() == nil {
						errCh <- err
					}
					return
				}
			}
		}()
	}
	workers.Wait()
	close(errCh)
	for err := range errCh {
		t.Errorf("concurrent SQLite operation: %v", err)
	}
}
