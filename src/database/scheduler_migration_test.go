package database

import (
	"database/sql"
	"testing"
	"time"
)

func TestMigrateSchedulerSchemaPreservesStateAsUnixSeconds(t *testing.T) {
	db, err := sql.Open("sqlite", memDSN("scheduler_migration"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if _, err := db.Exec(`
		CREATE TABLE scheduler_tasks (
			task_id TEXT PRIMARY KEY, task_name TEXT NOT NULL, schedule TEXT NOT NULL,
			last_run DATETIME, last_status TEXT, last_error TEXT, next_run DATETIME,
			run_count INTEGER DEFAULT 0, fail_count INTEGER DEFAULT 0,
			enabled BOOLEAN DEFAULT 1, locked_by TEXT, locked_at DATETIME
		);
		CREATE INDEX idx_scheduler_enabled ON scheduler_tasks(enabled);
		CREATE INDEX idx_scheduler_next_run ON scheduler_tasks(next_run);
		CREATE INDEX idx_scheduler_locked ON scheduler_tasks(locked_by);
		CREATE TABLE scheduler_history (
			id INTEGER PRIMARY KEY AUTOINCREMENT, task_name TEXT NOT NULL,
			start_time DATETIME NOT NULL, end_time DATETIME,
			duration_ms INTEGER, status TEXT NOT NULL, error TEXT,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP
		);
		CREATE INDEX idx_server_scheduler_history_name ON scheduler_history(task_name);
		CREATE INDEX idx_server_scheduler_history_start ON scheduler_history(start_time DESC);
		INSERT INTO scheduler_tasks VALUES (
			'backup_daily', 'backup_daily', '0 2 * * *', '2026-11-21 02:00:00',
			'success', '', '2026-11-22 02:00:00', 4, 1, 1, 'node-a', '2026-11-21 02:01:00'
		);
		INSERT INTO scheduler_history (task_name, start_time, end_time, duration_ms, status, error, created_at)
		VALUES ('backup_daily', '2026-11-21 02:00:00', '2026-11-21 02:01:00', 60000, 'success', '', '2026-11-21 02:01:00');
		INSERT INTO scheduler_history (task_name, start_time, end_time, duration_ms, status, error, created_at)
		VALUES ('backup_daily', '2026-11-22 02:00:00', NULL, NULL, 'running', '', CURRENT_TIMESTAMP);
	`); err != nil {
		t.Fatalf("seed legacy scheduler schema: %v", err)
	}

	if err := migrateSchedulerSchema(db); err != nil {
		t.Fatalf("migrateSchedulerSchema: %v", err)
	}
	if err := migrateSchedulerSchema(db); err != nil {
		t.Fatalf("second migrateSchedulerSchema: %v", err)
	}

	var name, schedule, status, taskType, lockedBy string
	var lastRun, nextRun, lockedAt int64
	var runs, failures, enabled int
	if err := db.QueryRow(`SELECT name, schedule, last_run, last_status, next_run, run_count, fail_count, enabled, task_type, locked_by, locked_at FROM scheduler_tasks WHERE id='backup_daily'`).Scan(
		&name, &schedule, &lastRun, &status, &nextRun, &runs, &failures, &enabled, &taskType, &lockedBy, &lockedAt,
	); err != nil {
		t.Fatalf("read migrated scheduler task: %v", err)
	}
	if name != "backup_daily" || schedule != "0 2 * * *" || status != "success" || taskType != "global" || lockedBy != "node-a" || enabled != 1 || runs != 4 || failures != 1 {
		t.Errorf("migrated task fields = %q %q %q %q %q enabled=%d runs=%d failures=%d", name, schedule, status, taskType, lockedBy, enabled, runs, failures)
	}
	wantLast := time.Date(2026, 11, 21, 2, 0, 0, 0, time.UTC).Unix()
	wantNext := time.Date(2026, 11, 22, 2, 0, 0, 0, time.UTC).Unix()
	wantLock := time.Date(2026, 11, 21, 2, 1, 0, 0, time.UTC).Unix()
	if lastRun != wantLast || nextRun != wantNext || lockedAt != wantLock {
		t.Errorf("migrated task timestamps = %d, %d, %d; want %d, %d, %d", lastRun, nextRun, lockedAt, wantLast, wantNext, wantLock)
	}

	var historyTask string
	var started, finished int64
	if err := db.QueryRow(`SELECT task_id, started_at, finished_at FROM scheduler_history WHERE id=1`).Scan(&historyTask, &started, &finished); err != nil {
		t.Fatalf("read migrated scheduler history: %v", err)
	}
	if historyTask != "backup_daily" || started != wantLast || finished != wantLock {
		t.Errorf("migrated history = %q %d %d, want backup_daily %d %d", historyTask, started, finished, wantLast, wantLock)
	}
	var nullableFinished, nullableDuration sql.NullInt64
	if err := db.QueryRow(`SELECT finished_at, duration_ms FROM scheduler_history WHERE id=2`).Scan(&nullableFinished, &nullableDuration); err != nil {
		t.Fatalf("read nullable migrated scheduler history: %v", err)
	}
	if nullableFinished.Valid || nullableDuration.Valid {
		t.Errorf("migrated nullable history = finished %v duration %v, want both NULL", nullableFinished, nullableDuration)
	}
}

func TestMigrateCanonicalSchedulerTasksUpgradesLegacyHistory(t *testing.T) {
	db, err := sql.Open("sqlite", memDSN("scheduler_history_only_migration"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if _, err := db.Exec(schedulerTasksSchema); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`
		CREATE TABLE scheduler_history (
			id INTEGER PRIMARY KEY, task_name TEXT NOT NULL, start_time DATETIME NOT NULL,
			end_time DATETIME, duration_ms INTEGER, status TEXT NOT NULL,
			error TEXT, created_at DATETIME
		);
		INSERT INTO scheduler_history (id, task_name, start_time, end_time, duration_ms, status, error, created_at)
		VALUES (7, 'weather_refresh', '2026-11-22 01:00:00', '2026-11-22 01:01:00', 60000, 'success', '', CURRENT_TIMESTAMP);
	`); err != nil {
		t.Fatal(err)
	}

	if err := migrateSchedulerSchema(db); err != nil {
		t.Fatalf("migrateSchedulerSchema: %v", err)
	}
	var taskID string
	var startedAt, finishedAt int64
	var duration int
	if err := db.QueryRow(`SELECT task_id, started_at, finished_at, duration_ms FROM scheduler_history WHERE id = 7`).Scan(&taskID, &startedAt, &finishedAt, &duration); err != nil {
		t.Fatal(err)
	}
	wantStarted := time.Date(2026, 11, 22, 1, 0, 0, 0, time.UTC).Unix()
	if taskID != "weather_refresh" || startedAt != wantStarted || finishedAt != wantStarted+60 || duration != 60000 {
		t.Fatalf("migrated history = %q %d %d %d", taskID, startedAt, finishedAt, duration)
	}
}

func TestSchedulerUnixSeconds(t *testing.T) {
	want := time.Date(2026, 11, 22, 1, 0, 0, 0, time.UTC).Unix()
	tests := []struct {
		name  string
		value any
		want  int64
		ok    bool
	}{
		{name: "nil"},
		{name: "int64", value: int64(42), want: 42, ok: true},
		{name: "int", value: int(42), want: 42, ok: true},
		{name: "float", value: float64(42.9), want: 42, ok: true},
		{name: "time", value: time.Unix(want, 0), want: want, ok: true},
		{name: "bytes", value: []byte("42"), want: 42, ok: true},
		{name: "integer string", value: "42", want: 42, ok: true},
		{name: "timestamp string", value: "2026-11-22 01:00:00", want: want, ok: true},
		{name: "invalid string", value: "not a timestamp"},
		{name: "unsupported", value: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := schedulerUnixSeconds(tt.value)
			if got != tt.want || ok != tt.ok {
				t.Fatalf("schedulerUnixSeconds(%#v) = %d, %v; want %d, %v", tt.value, got, ok, tt.want, tt.ok)
			}
		})
	}
}
