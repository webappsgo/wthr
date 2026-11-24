package database

import (
	"context"
	"database/sql"
	"fmt"
	"strconv"
	"time"

	"github.com/webappsgo/wthr/src/common/dbtime"
	"github.com/webappsgo/wthr/src/config"
)

const schedulerTasksSchema = `CREATE TABLE scheduler_tasks (
	id TEXT PRIMARY KEY,
	name TEXT NOT NULL,
	task_type TEXT NOT NULL DEFAULT 'global',
	enabled INTEGER NOT NULL DEFAULT 1,
	schedule TEXT NOT NULL,
	last_run INTEGER,
	last_status TEXT,
	last_error TEXT,
	next_run INTEGER,
	run_count INTEGER DEFAULT 0,
	fail_count INTEGER DEFAULT 0,
	locked_by TEXT,
	locked_at INTEGER
)`

const schedulerHistorySchema = `CREATE TABLE scheduler_history (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	task_id TEXT NOT NULL,
	started_at INTEGER NOT NULL,
	finished_at INTEGER,
	status TEXT NOT NULL,
	error TEXT,
	duration_ms INTEGER
)`

// migrateSchedulerSchema upgrades the original text-timestamp scheduler tables
// before ServerSchema creates indexes against their canonical columns.
func migrateSchedulerSchema(db *sql.DB) error {
	ctx, cancel := context.WithTimeout(context.Background(), TimeoutMigration)
	defer cancel()
	if exists, err := tableExists(ctx, db, "scheduler_tasks"); err != nil || !exists {
		return err
	}
	taskColumns, err := tableColumns(ctx, db, "scheduler_tasks")
	if err != nil {
		return err
	}
	if _, canonical := taskColumns["id"]; canonical {
		return migrateSchedulerHistory(ctx, db)
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin scheduler schema migration: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	for _, index := range []string{
		"idx_scheduler_enabled", "idx_scheduler_next_run", "idx_scheduler_locked",
		"idx_server_scheduler_history_name", "idx_server_scheduler_history_start",
	} {
		if _, err := tx.ExecContext(ctx, "DROP INDEX IF EXISTS "+index); err != nil {
			return fmt.Errorf("drop old scheduler index %s: %w", index, err)
		}
	}
	if _, err := tx.ExecContext(ctx, "ALTER TABLE scheduler_tasks RENAME TO scheduler_tasks_legacy"); err != nil {
		return fmt.Errorf("rename old scheduler task table: %w", err)
	}
	if _, err := tx.ExecContext(ctx, schedulerTasksSchema); err != nil {
		return fmt.Errorf("create canonical scheduler task table: %w", err)
	}
	if err := copyLegacySchedulerTasks(ctx, tx); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, "DROP TABLE scheduler_tasks_legacy"); err != nil {
		return fmt.Errorf("remove old scheduler task table: %w", err)
	}
	if err := migrateSchedulerHistoryTx(ctx, tx); err != nil {
		return err
	}
	if err := createSchedulerIndexes(ctx, tx); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit scheduler schema migration: %w", err)
	}
	return nil
}

func migrateSchedulerHistory(ctx context.Context, db *sql.DB) error {
	exists, err := tableExists(ctx, db, "scheduler_history")
	if err != nil || !exists {
		return err
	}
	columns, err := tableColumns(ctx, db, "scheduler_history")
	if err != nil {
		return err
	}
	if _, canonical := columns["started_at"]; canonical {
		return nil
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin scheduler history migration: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, "DROP INDEX IF EXISTS idx_server_scheduler_history_name"); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, "DROP INDEX IF EXISTS idx_server_scheduler_history_start"); err != nil {
		return err
	}
	if err := migrateSchedulerHistoryTx(ctx, tx); err != nil {
		return err
	}
	if err := createSchedulerIndexes(ctx, tx); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit scheduler history migration: %w", err)
	}
	return nil
}

func migrateSchedulerHistoryTx(ctx context.Context, tx *sql.Tx) error {
	exists, err := txTableExists(ctx, tx, "scheduler_history")
	if err != nil {
		return err
	}
	if !exists {
		_, err := tx.ExecContext(ctx, schedulerHistorySchema)
		return err
	}
	columns, err := txTableColumns(ctx, tx, "scheduler_history")
	if err != nil {
		return err
	}
	if _, canonical := columns["started_at"]; canonical {
		return nil
	}
	if _, err := tx.ExecContext(ctx, "ALTER TABLE scheduler_history RENAME TO scheduler_history_legacy"); err != nil {
		return fmt.Errorf("rename old scheduler history table: %w", err)
	}
	if _, err := tx.ExecContext(ctx, schedulerHistorySchema); err != nil {
		return fmt.Errorf("create canonical scheduler history table: %w", err)
	}
	rows, err := tx.QueryContext(ctx, `SELECT id, task_name, start_time, end_time, duration_ms, status, error, created_at FROM scheduler_history_legacy`)
	if err != nil {
		return fmt.Errorf("read old scheduler history rows: %w", err)
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var id int64
		var duration sql.NullInt64
		var taskID, status string
		var started, finished, errText, createdAt any
		if err := rows.Scan(&id, &taskID, &started, &finished, &duration, &status, &errText, &createdAt); err != nil {
			return fmt.Errorf("scan old scheduler history row: %w", err)
		}
		startUnix, ok := schedulerUnixSeconds(started)
		if !ok {
			startUnix = 0
		}
		finishUnix, ok := schedulerUnixSeconds(finished)
		if !ok {
			finishUnix = startUnix
		}
		var durationValue any
		if duration.Valid {
			durationValue = duration.Int64
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO scheduler_history (id, task_id, started_at, finished_at, status, error, duration_ms) VALUES (?, ?, ?, ?, ?, ?, ?)`, id, taskID, startUnix, nullableUnix(finished, finishUnix), status, errText, durationValue); err != nil {
			return fmt.Errorf("copy scheduler history row: %w", err)
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate old scheduler history rows: %w", err)
	}
	if err := rows.Close(); err != nil {
		return fmt.Errorf("close old scheduler history rows: %w", err)
	}
	if _, err := tx.ExecContext(ctx, "DROP TABLE scheduler_history_legacy"); err != nil {
		return fmt.Errorf("remove old scheduler history table: %w", err)
	}
	return nil
}

func nullableUnix(value any, parsed int64) any {
	if value == nil {
		return nil
	}
	return parsed
}

func copyLegacySchedulerTasks(ctx context.Context, tx *sql.Tx) error {
	rows, err := tx.QueryContext(ctx, `SELECT task_id, task_name, schedule, last_run, last_status, last_error, next_run, run_count, fail_count, enabled, locked_by, locked_at FROM scheduler_tasks_legacy`)
	if err != nil {
		return fmt.Errorf("read old scheduler task rows: %w", err)
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var id, name, schedule string
		var lastRun, nextRun, lockedAt, lastStatus, lastError, lockedBy any
		var runCount, failCount int
		var enabled bool
		if err := rows.Scan(&id, &name, &schedule, &lastRun, &lastStatus, &lastError, &nextRun, &runCount, &failCount, &enabled, &lockedBy, &lockedAt); err != nil {
			return fmt.Errorf("scan old scheduler task row: %w", err)
		}
		lastUnix, lastOK := schedulerUnixSeconds(lastRun)
		nextUnix, nextOK := schedulerUnixSeconds(nextRun)
		lockedUnix, lockedOK := schedulerUnixSeconds(lockedAt)
		taskType := "local"
		if config.IsGlobalSchedulerTask(name) {
			taskType = "global"
		}
		var lastValue, nextValue, lockedValue any
		if lastOK {
			lastValue = lastUnix
		}
		if nextOK {
			nextValue = nextUnix
		}
		if lockedOK {
			lockedValue = lockedUnix
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO scheduler_tasks (id, name, schedule, last_run, last_status, last_error, next_run, run_count, fail_count, enabled, task_type, locked_by, locked_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, id, name, schedule, lastValue, lastStatus, lastError, nextValue, runCount, failCount, enabled, taskType, lockedBy, lockedValue); err != nil {
			return fmt.Errorf("copy scheduler task %s: %w", id, err)
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate old scheduler task rows: %w", err)
	}
	if err := rows.Close(); err != nil {
		return fmt.Errorf("close old scheduler task rows: %w", err)
	}
	return nil
}

func schedulerUnixSeconds(value any) (int64, bool) {
	switch stored := value.(type) {
	case nil:
		return 0, false
	case int64:
		return stored, true
	case int:
		return int64(stored), true
	case float64:
		return int64(stored), true
	case time.Time:
		return stored.UTC().Unix(), true
	case []byte:
		return schedulerUnixSeconds(string(stored))
	case string:
		if seconds, err := strconv.ParseInt(stored, 10, 64); err == nil {
			return seconds, true
		}
		if parsed, ok := dbtime.ParseStoredTimestamp(stored); ok {
			return parsed.Unix(), true
		}
	}
	return 0, false
}

func tableExists(ctx context.Context, db *sql.DB, name string) (bool, error) {
	var count int
	err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name=?`, name).Scan(&count)
	return count > 0, err
}

func tableColumns(ctx context.Context, db *sql.DB, name string) (map[string]bool, error) {
	return queryTableColumns(ctx, db, name)
}

func txTableExists(ctx context.Context, tx *sql.Tx, name string) (bool, error) {
	var count int
	err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name=?`, name).Scan(&count)
	return count > 0, err
}

func txTableColumns(ctx context.Context, tx *sql.Tx, name string) (map[string]bool, error) {
	return queryTableColumns(ctx, tx, name)
}

type queryer interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
}

func queryTableColumns(ctx context.Context, query queryer, name string) (map[string]bool, error) {
	rows, err := query.QueryContext(ctx, "PRAGMA table_info("+name+")")
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	columns := make(map[string]bool)
	for rows.Next() {
		var cid, notnull, primaryKey int
		var column, columnType string
		var defaultValue any
		if err := rows.Scan(&cid, &column, &columnType, &notnull, &defaultValue, &primaryKey); err != nil {
			return nil, err
		}
		columns[column] = true
	}
	return columns, rows.Err()
}

func createSchedulerIndexes(ctx context.Context, tx *sql.Tx) error {
	for _, statement := range []string{
		`CREATE INDEX IF NOT EXISTS idx_scheduler_enabled ON scheduler_tasks(enabled)`,
		`CREATE INDEX IF NOT EXISTS idx_scheduler_next_run ON scheduler_tasks(next_run)`,
		`CREATE INDEX IF NOT EXISTS idx_scheduler_locked ON scheduler_tasks(locked_by)`,
		`CREATE INDEX IF NOT EXISTS idx_server_scheduler_history_name ON scheduler_history(task_id)`,
		`CREATE INDEX IF NOT EXISTS idx_server_scheduler_history_start ON scheduler_history(started_at DESC)`,
	} {
		if _, err := tx.ExecContext(ctx, statement); err != nil {
			return err
		}
	}
	return nil
}
