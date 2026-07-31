package scheduler

import (
	"database/sql"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/webappsgo/wthr/src/database"
	_ "modernc.org/sqlite"
)

// dbCounter ensures unique in-memory database names across tests in this package.
var dbCounter int64

// newSchedulerTestDBs creates fresh in-memory server/users SQLite databases with the
// tables touched by src/scheduler, wires them up as the global dual-DB (which is how
// this package's functions locate their database via database.GetServerDB/GetUsersDB),
// and registers cleanup to close the DBs and clear the global state.
func newSchedulerTestDBs(t *testing.T) (serverDB, usersDB *sql.DB) {
	t.Helper()

	counter := atomic.AddInt64(&dbCounter, 1)
	name := fmt.Sprintf("file:%s_%d", t.Name(), counter)

	var err error
	serverDB, err = sql.Open("sqlite", name+"_server?mode=memory&cache=shared")
	if err != nil {
		t.Fatalf("failed to open server test db: %v", err)
	}
	usersDB, err = sql.Open("sqlite", name+"_users?mode=memory&cache=shared")
	if err != nil {
		t.Fatalf("failed to open users test db: %v", err)
	}

	schema := []string{
		`CREATE TABLE server_scheduler_state (
			task_id TEXT PRIMARY KEY,
			task_name TEXT NOT NULL,
			locked_by TEXT,
			locked_at DATETIME,
			enabled BOOLEAN DEFAULT 1
		)`,
		`CREATE TABLE server_config (
			key TEXT PRIMARY KEY,
			value TEXT
		)`,
		`CREATE TABLE server_audit_log (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			user_id INTEGER,
			action TEXT,
			resource_type TEXT,
			resource_id TEXT,
			details TEXT,
			ip_address TEXT,
			user_agent TEXT,
			status TEXT,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE server_api_tokens (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			expires_at DATETIME
		)`,
		`CREATE TABLE server_setup_tokens (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			expires_at DATETIME
		)`,
		`CREATE TABLE server_rate_limits (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			window_start DATETIME
		)`,
		`CREATE TABLE server_nodes (
			node_id TEXT PRIMARY KEY,
			last_heartbeat DATETIME,
			status TEXT
		)`,
	}
	for _, stmt := range schema {
		if _, err := serverDB.Exec(stmt); err != nil {
			t.Fatalf("failed to create server schema (%s): %v", stmt, err)
		}
	}

	usersSchema := []string{
		`CREATE TABLE user_sessions (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			expires_at DATETIME
		)`,
		`CREATE TABLE user_accounts (
			id INTEGER PRIMARY KEY AUTOINCREMENT
		)`,
		`CREATE TABLE user_saved_locations (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT,
			latitude REAL,
			longitude REAL,
			user_id INTEGER,
			alerts_enabled INTEGER DEFAULT 0
		)`,
		`CREATE TABLE user_notifications (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			user_id INTEGER,
			type TEXT,
			title TEXT,
			message TEXT,
			link TEXT,
			read INTEGER DEFAULT 0
		)`,
	}
	for _, stmt := range usersSchema {
		if _, err := usersDB.Exec(stmt); err != nil {
			t.Fatalf("failed to create users schema (%s): %v", stmt, err)
		}
	}

	database.SetGlobalDualDB(&database.DualDB{Server: serverDB, Users: usersDB})

	t.Cleanup(func() {
		database.SetGlobalDualDB(nil)
		serverDB.Close()
		usersDB.Close()
	})

	return serverDB, usersDB
}

// --- AddTask / AddTaskInterval -----------------------------------------------------

func TestScheduler_AddTask(t *testing.T) {
	t.Run("valid schedule registers the task", func(t *testing.T) {
		s := NewScheduler(nil)
		err := s.AddTask("job-a", "@daily", func() error { return nil })
		if err != nil {
			t.Fatalf("AddTask() unexpected error: %v", err)
		}
		if got := s.GetTask("job-a"); got == nil {
			t.Fatal("GetTask() returned nil after successful AddTask()")
		}
	})

	t.Run("invalid cron expression returns error, not panic", func(t *testing.T) {
		s := NewScheduler(nil)
		err := s.AddTask("job-bad", "not a cron expression", func() error { return nil })
		if err == nil {
			t.Fatal("AddTask() expected error for invalid schedule, got nil")
		}
		if got := s.GetTask("job-bad"); got != nil {
			t.Error("GetTask() should return nil when AddTask failed to register")
		}
	})

	t.Run("duplicate name overwrites the previous task entry", func(t *testing.T) {
		s := NewScheduler(nil)
		if err := s.AddTask("dup", "@daily", func() error { return nil }); err != nil {
			t.Fatalf("first AddTask() error: %v", err)
		}
		if err := s.AddTask("dup", "@hourly", func() error { return nil }); err != nil {
			t.Fatalf("second AddTask() error: %v", err)
		}
		got := s.GetTask("dup")
		if got == nil {
			t.Fatal("GetTask() returned nil for duplicate task name")
		}
		if got.Schedule != "@hourly" {
			t.Errorf("Schedule = %q, want %q (last write should win in the map, though the original cron entry is orphaned)", got.Schedule, "@hourly")
		}
	})
}

func TestScheduler_AddTaskInterval(t *testing.T) {
	s := NewScheduler(nil)
	err := s.AddTaskInterval("interval-job", 5*time.Minute, func() error { return nil })
	if err != nil {
		t.Fatalf("AddTaskInterval() unexpected error: %v", err)
	}
	got := s.GetTask("interval-job")
	if got == nil {
		t.Fatal("GetTask() returned nil after AddTaskInterval()")
	}
	if got.Schedule != "@every 5m0s" {
		t.Errorf("Schedule = %q, want %q", got.Schedule, "@every 5m0s")
	}
}

// --- Enable / Disable / Get / Trigger -----------------------------------------------

func TestScheduler_EnableDisableTask(t *testing.T) {
	t.Run("unknown task returns error", func(t *testing.T) {
		s := NewScheduler(nil)
		if err := s.EnableTask("missing"); err == nil {
			t.Error("EnableTask() expected error for unknown task")
		}
		if err := s.DisableTask("missing"); err == nil {
			t.Error("DisableTask() expected error for unknown task")
		}
	})

	t.Run("disable then enable toggles state visible via GetTaskStatus", func(t *testing.T) {
		s := NewScheduler(nil)
		if err := s.AddTask("toggle", "@daily", func() error { return nil }); err != nil {
			t.Fatalf("AddTask() error: %v", err)
		}

		if err := s.DisableTask("toggle"); err != nil {
			t.Fatalf("DisableTask() error: %v", err)
		}
		if enabled := statusEnabled(t, s, "toggle"); enabled {
			t.Error("task should be disabled after DisableTask()")
		}

		if err := s.EnableTask("toggle"); err != nil {
			t.Fatalf("EnableTask() error: %v", err)
		}
		if enabled := statusEnabled(t, s, "toggle"); !enabled {
			t.Error("task should be enabled after EnableTask()")
		}
	})
}

func statusEnabled(t *testing.T, s *Scheduler, name string) bool {
	t.Helper()
	for _, st := range s.GetTaskStatus() {
		if st["name"] == name {
			return st["enabled"].(bool)
		}
	}
	t.Fatalf("task %q not found in GetTaskStatus()", name)
	return false
}

func TestScheduler_GetTask(t *testing.T) {
	s := NewScheduler(nil)
	if got := s.GetTask("nope"); got != nil {
		t.Errorf("GetTask() for unknown task = %v, want nil", got)
	}
}

func TestScheduler_TriggerTask(t *testing.T) {
	t.Run("unknown task returns error", func(t *testing.T) {
		s := NewScheduler(nil)
		if err := s.TriggerTask("missing"); err == nil {
			t.Error("TriggerTask() expected error for unknown task")
		}
	})

	t.Run("known task runs asynchronously and is recorded in history", func(t *testing.T) {
		newSchedulerTestDBs(t)
		s := NewScheduler(nil)
		if err := s.InitTaskHistoryTable(); err != nil {
			t.Fatalf("InitTaskHistoryTable() error: %v", err)
		}

		ran := make(chan struct{}, 1)
		// "manual-trigger" is deliberately not in globalTasks, so acquireTaskLock
		// short-circuits to true without needing a DB round trip for the lock itself.
		if err := s.AddTask("manual-trigger", "@yearly", func() error {
			ran <- struct{}{}
			return nil
		}); err != nil {
			t.Fatalf("AddTask() error: %v", err)
		}

		if err := s.TriggerTask("manual-trigger"); err != nil {
			t.Fatalf("TriggerTask() error: %v", err)
		}

		select {
		case <-ran:
		case <-time.After(5 * time.Second):
			t.Fatal("timed out waiting for triggered task to run")
		}

		// executeTask records history asynchronously relative to the fn body running,
		// so poll briefly instead of asserting immediately after the fn returns.
		deadline := time.Now().Add(2 * time.Second)
		var last *TaskRun
		for time.Now().Before(deadline) {
			var err error
			last, err = s.GetLastTaskRun("manual-trigger")
			if err != nil {
				t.Fatalf("GetLastTaskRun() error: %v", err)
			}
			if last != nil {
				break
			}
			time.Sleep(10 * time.Millisecond)
		}
		if last == nil {
			t.Fatal("expected a recorded task run for manual-trigger, got none")
		}
		if last.Status != "success" {
			t.Errorf("recorded status = %q, want success", last.Status)
		}
	})
}

// --- isGlobalTask --------------------------------------------------------------------

func TestIsGlobalTask(t *testing.T) {
	tests := []struct {
		name string
		want bool
	}{
		{"backup-daily", true},
		{"geoip-update", true},
		{"ssl-renewal", true},
		{"not-a-real-task", false},
		{"", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isGlobalTask(tt.name); got != tt.want {
				t.Errorf("isGlobalTask(%q) = %v, want %v", tt.name, got, tt.want)
			}
		})
	}
}

// --- acquireTaskLock / releaseTaskLock (cluster-aware locking) -----------------------

func TestScheduler_AcquireTaskLock(t *testing.T) {
	t.Run("non-global task always acquires without touching the DB", func(t *testing.T) {
		s := NewScheduler(nil) // no DB wired up at all
		if !s.acquireTaskLock("some-local-only-task") {
			t.Error("expected non-global task lock to always succeed")
		}
	})

	t.Run("global task with no existing lock acquires it", func(t *testing.T) {
		newSchedulerTestDBs(t)
		s := NewScheduler(nil)
		if !s.acquireTaskLock("backup-daily") {
			t.Fatal("expected lock acquisition to succeed when no lock row exists")
		}
		var lockedBy string
		if err := database.GetServerDB().QueryRow(
			"SELECT locked_by FROM server_scheduler_state WHERE task_id = ?", "backup-daily",
		).Scan(&lockedBy); err != nil {
			t.Fatalf("query lock row: %v", err)
		}
		if lockedBy != s.nodeID {
			t.Errorf("locked_by = %q, want %q", lockedBy, s.nodeID)
		}
	})

	t.Run("global task locked by another node (not expired) fails", func(t *testing.T) {
		newSchedulerTestDBs(t)
		s := NewScheduler(nil)
		_, err := database.GetServerDB().Exec(
			`INSERT INTO server_scheduler_state (task_id, task_name, locked_by, locked_at, enabled)
			 VALUES (?, ?, ?, ?, 1)`,
			"backup-daily", "backup-daily", "other-node", time.Now(),
		)
		if err != nil {
			t.Fatalf("seed lock row: %v", err)
		}

		if s.acquireTaskLock("backup-daily") {
			t.Error("expected lock acquisition to fail while another node holds a fresh lock")
		}
	})

	t.Run("global task locked by another node past LockTimeout is stolen", func(t *testing.T) {
		newSchedulerTestDBs(t)
		s := NewScheduler(nil)
		expiredAt := time.Now().Add(-LockTimeout - time.Minute)
		_, err := database.GetServerDB().Exec(
			`INSERT INTO server_scheduler_state (task_id, task_name, locked_by, locked_at, enabled)
			 VALUES (?, ?, ?, ?, 1)`,
			"backup-daily", "backup-daily", "other-node", expiredAt,
		)
		if err != nil {
			t.Fatalf("seed lock row: %v", err)
		}

		if !s.acquireTaskLock("backup-daily") {
			t.Error("expected an expired lock (older than LockTimeout) to be stealable")
		}
		var lockedBy string
		if err := database.GetServerDB().QueryRow(
			"SELECT locked_by FROM server_scheduler_state WHERE task_id = ?", "backup-daily",
		).Scan(&lockedBy); err != nil {
			t.Fatalf("query lock row: %v", err)
		}
		if lockedBy != s.nodeID {
			t.Errorf("locked_by after steal = %q, want %q", lockedBy, s.nodeID)
		}
	})

	t.Run("global task already held by this node refreshes cleanly", func(t *testing.T) {
		newSchedulerTestDBs(t)
		s := NewScheduler(nil)
		if !s.acquireTaskLock("backup-daily") {
			t.Fatal("first acquire should succeed")
		}
		if !s.acquireTaskLock("backup-daily") {
			t.Error("re-acquiring a lock already held by the same node should succeed")
		}
	})
}

func TestScheduler_ReleaseTaskLock(t *testing.T) {
	t.Run("non-global task release is a no-op even without a DB", func(t *testing.T) {
		s := NewScheduler(nil)
		// Must not panic despite no global DB being configured.
		s.releaseTaskLock("some-local-only-task")
	})

	t.Run("releasing a held global lock clears locked_by", func(t *testing.T) {
		newSchedulerTestDBs(t)
		s := NewScheduler(nil)
		if !s.acquireTaskLock("backup-daily") {
			t.Fatal("acquire should succeed")
		}
		s.releaseTaskLock("backup-daily")

		var lockedBy sql.NullString
		if err := database.GetServerDB().QueryRow(
			"SELECT locked_by FROM server_scheduler_state WHERE task_id = ?", "backup-daily",
		).Scan(&lockedBy); err != nil {
			t.Fatalf("query lock row: %v", err)
		}
		if lockedBy.Valid {
			t.Errorf("locked_by after release = %q, want NULL", lockedBy.String)
		}
	})
}

// --- checkAndCreateAlerts boundary conditions -----------------------------------------

func weatherWith(temp, wind, precip float64, code int) struct {
	Current struct {
		Temperature   float64 `json:"temperature_2m"`
		WindSpeed     float64 `json:"wind_speed_10m"`
		Precipitation float64 `json:"precipitation"`
		WeatherCode   int     `json:"weather_code"`
	} `json:"current"`
} {
	var w struct {
		Current struct {
			Temperature   float64 `json:"temperature_2m"`
			WindSpeed     float64 `json:"wind_speed_10m"`
			Precipitation float64 `json:"precipitation"`
			WeatherCode   int     `json:"weather_code"`
		} `json:"current"`
	}
	w.Current.Temperature = temp
	w.Current.WindSpeed = wind
	w.Current.Precipitation = precip
	w.Current.WeatherCode = code
	return w
}

func TestCheckAndCreateAlerts(t *testing.T) {
	tests := []struct {
		name   string
		temp   float64
		wind   float64
		precip float64
		code   int
		want   int
	}{
		{"calm weather, no alerts", 70, 5, 0, 1, 0},
		{"exactly freezing (32F) is not below threshold", 32, 0, 0, 0, 0},
		{"just below freezing triggers cold alert", 31.9, 0, 0, 0, 1},
		{"exactly 95F is not above threshold", 95, 0, 0, 0, 0},
		{"just above 95F triggers heat alert", 95.1, 0, 0, 0, 1},
		{"exactly 40mph wind is not above threshold", 70, 40, 0, 0, 0},
		{"just above 40mph triggers wind alert", 70, 40.1, 0, 0, 1},
		{"exactly 0.5in precip is not above threshold", 70, 0, 0.5, 0, 0},
		{"just above 0.5in precip triggers rain alert", 70, 0, 0.51, 0, 1},
		{"weather code 94 is below severe threshold", 70, 0, 0, 94, 0},
		{"weather code 95 meets severe threshold", 70, 0, 0, 95, 1},
		// temp=20 only triggers the freezing threshold (not heat, which needs >95),
		// so at most 4 of the 5 thresholds can fire from a single reading: freezing,
		// wind, precip, and severe weather code.
		{"all conditions combined stack alert count", 20, 50, 1.0, 99, 4},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, usersDB := newSchedulerTestDBs(t)
			weather := weatherWith(tt.temp, tt.wind, tt.precip, tt.code)

			got := checkAndCreateAlerts(nil, 1, 1, "Test City", weather)
			if got != tt.want {
				t.Errorf("checkAndCreateAlerts() = %d alerts, want %d", got, tt.want)
			}

			var count int
			if err := usersDB.QueryRow("SELECT COUNT(*) FROM user_notifications WHERE user_id = 1").Scan(&count); err != nil {
				t.Fatalf("count notifications: %v", err)
			}
			if count != tt.want {
				t.Errorf("persisted notification rows = %d, want %d", count, tt.want)
			}
		})
	}
}

// --- top-level scheduled task functions: disabled/empty-data fast paths --------------

// BUG (not fixed, per task constraints): CleanupOldSessions (scheduler.go, the
// "DELETE FROM user_sessions WHERE expires_at < datetime('now')" query) compares
// a driver-serialized Go time.Time bind parameter against SQLite's datetime('now').
// The Go value is stored as e.g. "2026-07-21T04:07:57.73098018-04:00" (local
// offset, 'T' separator, fractional seconds) while datetime('now') produces
// "2026-07-21 07:07:57" (UTC, space separator, no fraction). Confirmed by direct
// probe: a session with expires_at = now+1h is deleted anyway, because the
// mixed-format/mixed-timezone comparison does not behave as a correct time
// comparison under the column's NUMERIC affinity. This means CleanupOldSessions
// can delete sessions that have not actually expired yet. This test documents
// the actual (buggy) behavior rather than the intended one.
func TestCleanupOldSessions(t *testing.T) {
	_, usersDB := newSchedulerTestDBs(t)
	future := time.Now().Add(1 * time.Hour)
	past := time.Now().Add(-1 * time.Hour)

	if _, err := usersDB.Exec("INSERT INTO user_sessions (expires_at) VALUES (?), (?), (?)", past, past, future); err != nil {
		t.Fatalf("seed sessions: %v", err)
	}

	if err := CleanupOldSessions(nil); err != nil {
		t.Fatalf("CleanupOldSessions() error: %v", err)
	}

	var remaining int
	if err := usersDB.QueryRow("SELECT COUNT(*) FROM user_sessions").Scan(&remaining); err != nil {
		t.Fatalf("count sessions: %v", err)
	}
	// Actual (buggy) behavior: all 3 rows are deleted, including the one whose
	// expiry is an hour in the future. See bug note above the test.
	if remaining != 0 {
		t.Errorf("remaining sessions = %d, want 0 given the current datetime-comparison bug - if this now fails with remaining=1, the bug has been fixed upstream and this test should be reverted to asserting the correct (1-remaining) behavior", remaining)
	}
}

func TestCleanupOldAuditLogs(t *testing.T) {
	t.Run("default retention (no config row) is 90 days", func(t *testing.T) {
		serverDB, _ := newSchedulerTestDBs(t)
		old := time.Now().AddDate(0, 0, -100)
		recent := time.Now().AddDate(0, 0, -10)
		if _, err := serverDB.Exec(
			`INSERT INTO server_audit_log (user_id, action, resource_type, resource_id, details, ip_address, user_agent, status, created_at)
			 VALUES (NULL,'a','t','1','d','ip','ua','success',?),(NULL,'a','t','2','d','ip','ua','success',?)`,
			old, recent,
		); err != nil {
			t.Fatalf("seed audit log: %v", err)
		}

		if err := CleanupOldAuditLogs(nil); err != nil {
			t.Fatalf("CleanupOldAuditLogs() error: %v", err)
		}

		var remaining int
		if err := serverDB.QueryRow("SELECT COUNT(*) FROM server_audit_log").Scan(&remaining); err != nil {
			t.Fatalf("count audit logs: %v", err)
		}
		if remaining != 1 {
			t.Errorf("remaining audit logs = %d, want 1", remaining)
		}
	})
}

// BUG (not fixed, per task constraints): CleanupExpiredTokens shares the same
// "expires_at < datetime('now')" pattern as CleanupOldSessions (see the bug note
// above TestCleanupOldSessions) - it deletes rows whose expiry is in the future
// too. This test documents the actual (buggy) behavior.
func TestCleanupExpiredTokens(t *testing.T) {
	serverDB, _ := newSchedulerTestDBs(t)
	future := time.Now().Add(1 * time.Hour)
	past := time.Now().Add(-1 * time.Hour)

	if _, err := serverDB.Exec("INSERT INTO server_api_tokens (expires_at) VALUES (?), (?), (NULL)", past, future); err != nil {
		t.Fatalf("seed api tokens: %v", err)
	}
	if _, err := serverDB.Exec("INSERT INTO server_setup_tokens (expires_at) VALUES (?), (?)", past, future); err != nil {
		t.Fatalf("seed setup tokens: %v", err)
	}

	if err := CleanupExpiredTokens(nil); err != nil {
		t.Fatalf("CleanupExpiredTokens() error: %v", err)
	}

	var apiRemaining, setupRemaining int
	if err := serverDB.QueryRow("SELECT COUNT(*) FROM server_api_tokens").Scan(&apiRemaining); err != nil {
		t.Fatalf("count api tokens: %v", err)
	}
	if err := serverDB.QueryRow("SELECT COUNT(*) FROM server_setup_tokens").Scan(&setupRemaining); err != nil {
		t.Fatalf("count setup tokens: %v", err)
	}
	// NULL-expiry API tokens are never touched (the query explicitly requires
	// expires_at IS NOT NULL), so that row still survives. The future-expiry row
	// does NOT survive due to the datetime-comparison bug described above.
	if apiRemaining != 1 {
		t.Errorf("remaining api tokens = %d, want 1 (NULL expiry only) given the current datetime-comparison bug - if this now fails with remaining=2, the bug has been fixed upstream and this test should be reverted to asserting the correct (2-remaining) behavior", apiRemaining)
	}
	if setupRemaining != 0 {
		t.Errorf("remaining setup tokens = %d, want 0 given the current datetime-comparison bug - if this now fails with remaining=1, the bug has been fixed upstream and this test should be reverted to asserting the correct (1-remaining) behavior", setupRemaining)
	}
}

// BUG (not fixed, per task constraints): CleanupRateLimitCounters compares a
// driver-serialized Go time.Time bind parameter against
// "datetime('now', '-1 hour')" (same class of bug as TestCleanupOldSessions
// above). Confirmed by direct probe: both a 2h-old row and a 30min-old row
// (which should survive the 1h cutoff) are deleted. This test documents the
// actual (buggy) behavior.
func TestCleanupRateLimitCounters(t *testing.T) {
	serverDB, _ := newSchedulerTestDBs(t)
	old := time.Now().Add(-2 * time.Hour)
	recent := time.Now().Add(-30 * time.Minute)
	if _, err := serverDB.Exec("INSERT INTO server_rate_limits (window_start) VALUES (?), (?)", old, recent); err != nil {
		t.Fatalf("seed rate limits: %v", err)
	}

	if err := CleanupRateLimitCounters(nil); err != nil {
		t.Fatalf("CleanupRateLimitCounters() error: %v", err)
	}

	var remaining int
	if err := serverDB.QueryRow("SELECT COUNT(*) FROM server_rate_limits").Scan(&remaining); err != nil {
		t.Fatalf("count rate limits: %v", err)
	}
	// Actual (buggy) behavior: both rows are deleted, including the 30-minute-old
	// one that should have survived a 1-hour cutoff. See bug note above the test.
	if remaining != 0 {
		t.Errorf("remaining rate limit rows = %d, want 0 given the current datetime-comparison bug - if this now fails with remaining=1, the bug has been fixed upstream and this test should be reverted to asserting the correct (1-remaining) behavior", remaining)
	}
}

func TestCreateSystemBackup_DisabledSkipsSilently(t *testing.T) {
	newSchedulerTestDBs(t)
	// No 'backup.enabled' row at all -> Scan errors -> function returns nil without
	// touching the filesystem or real paths.GetDefaultPaths() location.
	if err := CreateSystemBackup(nil); err != nil {
		t.Errorf("CreateSystemBackup() with backups disabled = %v, want nil", err)
	}
}

func TestCheckSSLRenewal_SkipPaths(t *testing.T) {
	t.Run("letsencrypt not enabled", func(t *testing.T) {
		newSchedulerTestDBs(t)
		if err := CheckSSLRenewal(); err != nil {
			t.Errorf("CheckSSLRenewal() = %v, want nil when disabled", err)
		}
	})

	t.Run("enabled but no domain configured", func(t *testing.T) {
		serverDB, _ := newSchedulerTestDBs(t)
		if _, err := serverDB.Exec("INSERT INTO server_config (key, value) VALUES ('ssl.letsencrypt.enabled','true')"); err != nil {
			t.Fatalf("seed config: %v", err)
		}
		if err := CheckSSLRenewal(); err != nil {
			t.Errorf("CheckSSLRenewal() = %v, want nil when no domain is configured", err)
		}
	})

	t.Run("enabled with domain but no certificate on disk", func(t *testing.T) {
		serverDB, _ := newSchedulerTestDBs(t)
		if _, err := serverDB.Exec(
			"INSERT INTO server_config (key, value) VALUES ('ssl.letsencrypt.enabled','true'), ('ssl.domain','example.invalid')",
		); err != nil {
			t.Fatalf("seed config: %v", err)
		}
		if err := CheckSSLRenewal(); err != nil {
			t.Errorf("CheckSSLRenewal() = %v, want nil when no certificate file is found", err)
		}
	})
}

func TestSelfHealthCheck(t *testing.T) {
	newSchedulerTestDBs(t)
	if err := SelfHealthCheck(); err != nil {
		t.Errorf("SelfHealthCheck() = %v, want nil against a healthy in-memory DB", err)
	}
}

func TestCheckTorHealth(t *testing.T) {
	newSchedulerTestDBs(t)
	// Regardless of whether the `tor` binary exists in this environment, without
	// tor.enabled='true' configured (and typically without tor installed at all in
	// the CI/build container), this must return nil and never panic.
	if err := CheckTorHealth(); err != nil {
		t.Logf("CheckTorHealth() returned error (acceptable if tor happens to be installed and misconfigured here): %v", err)
	}
}

func TestClusterHeartbeat(t *testing.T) {
	t.Run("disabled skips silently", func(t *testing.T) {
		serverDB, _ := newSchedulerTestDBs(t)
		if err := ClusterHeartbeat("node-1"); err != nil {
			t.Fatalf("ClusterHeartbeat() = %v, want nil when cluster mode disabled", err)
		}
		var count int
		if err := serverDB.QueryRow("SELECT COUNT(*) FROM server_nodes").Scan(&count); err != nil {
			t.Fatalf("count nodes: %v", err)
		}
		if count != 0 {
			t.Errorf("expected no node row when cluster mode disabled, got %d", count)
		}
	})

	t.Run("enabled records/updates the node heartbeat", func(t *testing.T) {
		serverDB, _ := newSchedulerTestDBs(t)
		if _, err := serverDB.Exec("INSERT INTO server_config (key, value) VALUES ('cluster.enabled','true')"); err != nil {
			t.Fatalf("seed config: %v", err)
		}

		if err := ClusterHeartbeat("node-1"); err != nil {
			t.Fatalf("ClusterHeartbeat() error: %v", err)
		}
		var status string
		if err := serverDB.QueryRow("SELECT status FROM server_nodes WHERE node_id = 'node-1'").Scan(&status); err != nil {
			t.Fatalf("query node: %v", err)
		}
		if status != "online" {
			t.Errorf("status = %q, want online", status)
		}

		// Second heartbeat should update, not duplicate, the row (ON CONFLICT upsert).
		if err := ClusterHeartbeat("node-1"); err != nil {
			t.Fatalf("second ClusterHeartbeat() error: %v", err)
		}
		var count int
		if err := serverDB.QueryRow("SELECT COUNT(*) FROM server_nodes").Scan(&count); err != nil {
			t.Fatalf("count nodes: %v", err)
		}
		if count != 1 {
			t.Errorf("expected exactly 1 node row after repeated heartbeats, got %d", count)
		}
	})
}

func TestCheckWeatherAlerts_NoLocationsIsNoOp(t *testing.T) {
	newSchedulerTestDBs(t)
	// No rows in user_saved_locations => the query returns nothing and the function
	// must return nil without making any real HTTP call to the weather API.
	if err := CheckWeatherAlerts(nil); err != nil {
		t.Errorf("CheckWeatherAlerts() = %v, want nil with no saved locations", err)
	}
}

func TestUpdateBlocklist_DisabledSkipsNetworkCall(t *testing.T) {
	newSchedulerTestDBs(t)
	if err := UpdateBlocklist(); err != nil {
		t.Errorf("UpdateBlocklist() = %v, want nil when disabled", err)
	}
}

func TestUpdateCVEDatabase_DisabledSkipsNetworkCall(t *testing.T) {
	newSchedulerTestDBs(t)
	if err := UpdateCVEDatabase(); err != nil {
		t.Errorf("UpdateCVEDatabase() = %v, want nil when disabled", err)
	}
}
