package scheduler

import (
	"database/sql"
	"fmt"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/webappsgo/wthr/src/database"
	"github.com/webappsgo/wthr/src/util"
	_ "modernc.org/sqlite"
)

// dbCounter ensures unique in-memory database names across tests in this package.
var dbCounter int64

// testUserID is the id of the account newSchedulerTestDBs seeds into
// user_accounts. Every users-database table this package writes to
// (user_saved_locations, user_notifications, user_sessions, user_tokens) carries
// a foreign key to user_accounts(id), so the parent row has to exist before any
// test can insert a child row.
const testUserID = 1

// newSchedulerTestDBs creates fresh in-memory server/users SQLite databases, applies
// the real database.ServerSchema and database.UsersSchema so tests run against the
// same DDL the server creates at startup (column set, NOT NULL, CHECK constraints
// and foreign keys included), wires them up as the global dual-DB (which is how this
// package's functions locate their database via database.GetServerDB/GetUsersDB),
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

	if _, err := serverDB.Exec(database.ServerSchema); err != nil {
		t.Fatalf("failed to apply database.ServerSchema: %v", err)
	}
	if _, err := usersDB.Exec(database.UsersSchema); err != nil {
		t.Fatalf("failed to apply database.UsersSchema: %v", err)
	}

	// user_accounts requires a unique username, a unique email and a password
	// hash; every value here is a placeholder, only the id matters to callers.
	if _, err := usersDB.Exec(`
		INSERT INTO user_accounts (id, username, email, password_hash)
		VALUES (?, 'scheduler-fixture', 'scheduler-fixture@example.invalid', 'x')
	`, testUserID); err != nil {
		t.Fatalf("failed to seed user_accounts: %v", err)
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

// --- Start / run / runDueTasks / Stop (ticker-driven main loop) ----------------------

func TestScheduler_StartStop(t *testing.T) {
	t.Run("start fires a due task through the real ticker loop, stop drains cleanly", func(t *testing.T) {
		newSchedulerTestDBs(t)
		s := NewScheduler(nil)
		if err := s.InitTaskHistoryTable(); err != nil {
			t.Fatalf("InitTaskHistoryTable() error: %v", err)
		}

		ran := make(chan struct{}, 1)
		// "@every 1s" matches the scheduler's own tickInterval, so the first
		// real tick after Start() should already be due.
		if err := s.AddTask("tick-task", "@every 1s", func() error {
			select {
			case ran <- struct{}{}:
			default:
			}
			return nil
		}); err != nil {
			t.Fatalf("AddTask() error: %v", err)
		}

		s.Start()

		select {
		case <-ran:
		case <-time.After(5 * time.Second):
			s.Stop()
			t.Fatal("timed out waiting for the ticker-driven task to fire")
		}

		// Disable the task immediately so a second real tick (the schedule
		// interval matches tickInterval) cannot launch another in-flight
		// executeTask goroutine while we drain the first one below.
		if err := s.DisableTask("tick-task"); err != nil {
			t.Fatalf("DisableTask() error: %v", err)
		}

		// Stop() only waits for the dispatch-loop goroutine, not for any
		// go s.executeTask(task) goroutines runDueTasks already launched
		// (documented, intentional graceful-shutdown behavior). Poll until
		// executeTask's own DB writes (logTaskExecution/RecordTaskRun) have
		// actually completed before this subtest's DB fixture is torn down -
		// otherwise that goroutine can outlive the fixture and later panic
		// on a nil *sql.DB once an unrelated, later test's cleanup runs.
		deadline := time.Now().Add(2 * time.Second)
		for {
			run, err := s.GetLastTaskRun("tick-task")
			if err != nil {
				t.Fatalf("GetLastTaskRun() error: %v", err)
			}
			if run != nil {
				break
			}
			if time.Now().After(deadline) {
				t.Fatal("timed out waiting for the ticker-driven task's history write to land")
			}
			time.Sleep(5 * time.Millisecond)
		}

		s.mu.RLock()
		running := s.running
		s.mu.RUnlock()
		if !running {
			t.Error("scheduler should report running while its loop goroutine is active")
		}

		s.Stop()

		s.mu.RLock()
		running = s.running
		s.mu.RUnlock()
		if running {
			t.Error("scheduler should report not running after Stop()")
		}
	})

	t.Run("calling start twice does not replace the running ticker/loop", func(t *testing.T) {
		s := NewScheduler(nil)
		s.Start()
		defer s.Stop()

		s.mu.RLock()
		firstTicker := s.ticker
		s.mu.RUnlock()

		s.Start()

		s.mu.RLock()
		secondTicker := s.ticker
		s.mu.RUnlock()

		if firstTicker != secondTicker {
			t.Error("second Start() call should be a no-op, not swap out the running ticker")
		}
	})

	t.Run("stop before any start is a safe no-op", func(t *testing.T) {
		s := NewScheduler(nil)
		s.Stop()
	})

	t.Run("stop after start then a second stop is a safe no-op", func(t *testing.T) {
		s := NewScheduler(nil)
		s.Start()
		s.Stop()
		// Must not panic or block on a second close of already-closed channels.
		s.Stop()
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

// testLockSchedule is the cron expression the lock tests hand to
// acquireTaskLock. server_scheduler_state.schedule is NOT NULL, so an insert
// that omits it fails outright, and a caller that substituted a placeholder
// would leave the row describing a schedule the task does not have.
const testLockSchedule = "0 2 * * *"

func TestScheduler_AcquireTaskLock(t *testing.T) {
	t.Run("non-global task always acquires without touching the DB", func(t *testing.T) {
		s := NewScheduler(nil) // no DB wired up at all
		if !s.acquireTaskLock("some-local-only-task", testLockSchedule) {
			t.Error("expected non-global task lock to always succeed")
		}
	})

	t.Run("global task with no existing lock acquires it", func(t *testing.T) {
		newSchedulerTestDBs(t)
		s := NewScheduler(nil)
		if !s.acquireTaskLock("backup-daily", testLockSchedule) {
			t.Fatal("expected lock acquisition to succeed when no lock row exists")
		}
		var lockedBy, schedule string
		if err := database.GetServerDB().QueryRow(
			"SELECT locked_by, schedule FROM server_scheduler_state WHERE task_id = ?", "backup-daily",
		).Scan(&lockedBy, &schedule); err != nil {
			t.Fatalf("query lock row: %v", err)
		}
		if lockedBy != s.nodeID {
			t.Errorf("locked_by = %q, want %q", lockedBy, s.nodeID)
		}
		// The insert is the row's first writer, so the task's real cron
		// expression has to be stored here and not left for a later update.
		if schedule != testLockSchedule {
			t.Errorf("schedule = %q, want %q", schedule, testLockSchedule)
		}
	})

	t.Run("global task locked by another node (not expired) fails", func(t *testing.T) {
		newSchedulerTestDBs(t)
		s := NewScheduler(nil)
		_, err := database.GetServerDB().Exec(
			`INSERT INTO server_scheduler_state (task_id, task_name, schedule, locked_by, locked_at, enabled)
			 VALUES (?, ?, ?, ?, ?, 1)`,
			"backup-daily", "backup-daily", testLockSchedule, "other-node", time.Now(),
		)
		if err != nil {
			t.Fatalf("seed lock row: %v", err)
		}

		if s.acquireTaskLock("backup-daily", testLockSchedule) {
			t.Error("expected lock acquisition to fail while another node holds a fresh lock")
		}
	})

	t.Run("global task locked by another node past LockTimeout is stolen", func(t *testing.T) {
		newSchedulerTestDBs(t)
		s := NewScheduler(nil)
		expiredAt := time.Now().Add(-LockTimeout - time.Minute)
		_, err := database.GetServerDB().Exec(
			`INSERT INTO server_scheduler_state (task_id, task_name, schedule, locked_by, locked_at, enabled)
			 VALUES (?, ?, ?, ?, ?, 1)`,
			"backup-daily", "backup-daily", testLockSchedule, "other-node", expiredAt,
		)
		if err != nil {
			t.Fatalf("seed lock row: %v", err)
		}

		if !s.acquireTaskLock("backup-daily", testLockSchedule) {
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
		if !s.acquireTaskLock("backup-daily", testLockSchedule) {
			t.Fatal("first acquire should succeed")
		}
		if !s.acquireTaskLock("backup-daily", "*/30 * * * *") {
			t.Error("re-acquiring a lock already held by the same node should succeed")
		}
		// A re-acquire carries the task's current schedule, so a task whose cron
		// expression changed between runs updates the stored row rather than
		// leaving the old expression behind.
		var schedule string
		if err := database.GetServerDB().QueryRow(
			"SELECT schedule FROM server_scheduler_state WHERE task_id = ?", "backup-daily",
		).Scan(&schedule); err != nil {
			t.Fatalf("query lock row: %v", err)
		}
		if schedule != "*/30 * * * *" {
			t.Errorf("schedule after re-acquire = %q, want %q", schedule, "*/30 * * * *")
		}
	})
}

// seedTaskLock writes a lock row for taskName held by holder with lockedAt
// stored exactly as given, so a test can put any on-disk layout in the column.
func seedTaskLock(t *testing.T, taskName, holder string, lockedAt interface{}) {
	t.Helper()

	if _, err := database.GetServerDB().Exec(
		`INSERT INTO server_scheduler_state (task_id, task_name, schedule, locked_by, locked_at, enabled)
		 VALUES (?, ?, ?, ?, ?, 1)`,
		taskName, taskName, testLockSchedule, holder, lockedAt,
	); err != nil {
		t.Fatalf("seed lock row: %v", err)
	}
}

// lockHolder returns the current locked_by value for taskName.
func lockHolder(t *testing.T, taskName string) sql.NullString {
	t.Helper()

	var holder sql.NullString
	if err := database.GetServerDB().QueryRow(
		"SELECT locked_by FROM server_scheduler_state WHERE task_id = ?", taskName,
	).Scan(&holder); err != nil {
		t.Fatalf("query lock row: %v", err)
	}

	return holder
}

// TestScheduler_AcquireTaskLock_MixedTimestampLayouts pins the staleness test to
// the absolute instant a lock was taken rather than to how its text sorts.
//
// The previous implementation compared "locked_at < ?" in SQL against a bound
// time.Time, so the driver's local-zone rendering was compared byte-for-byte
// against whatever layout happened to be on disk. Every case below is written
// with an explicit offset so wall-clock text ordering contradicts true instant
// ordering on any host timezone, plus a value no layout can parse.
func TestScheduler_AcquireTaskLock_MixedTimestampLayouts(t *testing.T) {
	tests := []struct {
		name     string
		lockedAt interface{}
		steal    bool
	}{
		{
			// Two hours old, so genuinely stale. Rendered in +13:00 its text
			// begins "T"-separated and sorts AFTER any space-separated bound
			// value, which is why the old SQL comparison never expired it -
			// this case fails against the old code on every host.
			name:     "stale lock whose +13:00 RFC3339 text sorts newer is stolen",
			lockedAt: time.Now().Add(-2 * time.Hour).In(tokenZoneEast).Format(time.RFC3339Nano),
			steal:    true,
		},
		{
			// Taken seconds ago, but its -11:00 wall clock reads eleven hours
			// behind UTC, which a text comparison mistakes for an expired lock
			// and steals out from under the node still running the task.
			name:     "fresh lock whose -11:00 text reads eleven hours old is kept",
			lockedAt: time.Now().In(tokenZoneWest).Format("2006-01-02 15:04:05.999999999 -0700"),
			steal:    false,
		},
		{
			// Canonical UTC text, two hours old: the layout this code now
			// writes, and the one case the old comparison got right on a
			// UTC host. Behaviour must be unchanged.
			name:     "stale canonical UTC text is stolen",
			lockedAt: time.Now().UTC().Add(-2 * time.Hour).Format(sqlTimestampLayout),
			steal:    true,
		},
		{
			name:     "fresh canonical UTC text is kept",
			lockedAt: time.Now().UTC().Format(sqlTimestampLayout),
			steal:    false,
		},
		{
			// Fail-safe direction: a value the project cannot interpret is
			// treated as a HELD lock. Guessing "stale" would run a global task
			// on two nodes at once; guessing "held" only skips a tick, and the
			// holder's own release clears the row.
			name:     "unparseable locked_at is treated as held, not stale",
			lockedAt: "not-a-timestamp",
			steal:    false,
		},
		{
			// NULL locked_at with a live holder is equally uninterpretable.
			name:     "NULL locked_at with a foreign holder is treated as held",
			lockedAt: nil,
			steal:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			newSchedulerTestDBs(t)
			s := NewScheduler(nil)
			seedTaskLock(t, "backup-daily", "other-node", tt.lockedAt)

			if got := s.acquireTaskLock("backup-daily", testLockSchedule); got != tt.steal {
				t.Fatalf("acquireTaskLock() = %v, want %v", got, tt.steal)
			}

			holder := lockHolder(t, "backup-daily")
			want := "other-node"
			if tt.steal {
				want = s.nodeID
			}
			if holder.String != want {
				t.Errorf("locked_by = %q, want %q", holder.String, want)
			}
		})
	}
}

// TestScheduler_AcquireTaskLock_WritesCanonicalTimestamp proves the lock writes
// canonical UTC text, so the column never accumulates a second encoding that a
// later reader would have to guess at.
func TestScheduler_AcquireTaskLock_WritesCanonicalTimestamp(t *testing.T) {
	newSchedulerTestDBs(t)
	s := NewScheduler(nil)

	if !s.acquireTaskLock("backup-daily", testLockSchedule) {
		t.Fatal("acquire should succeed on a fresh row")
	}

	// CAST keeps the driver from re-rendering the column as a time.Time on the
	// way out, so the assertion sees the bytes actually stored.
	var stored string
	if err := database.GetServerDB().QueryRow(
		"SELECT CAST(locked_at AS TEXT) FROM server_scheduler_state WHERE task_id = ?", "backup-daily",
	).Scan(&stored); err != nil {
		t.Fatalf("query locked_at: %v", err)
	}

	if _, err := time.Parse(sqlTimestampLayout, stored); err != nil {
		t.Errorf("locked_at = %q, want canonical %q text", stored, sqlTimestampLayout)
	}

	// The refresh path has to keep the same encoding.
	if !s.acquireTaskLock("backup-daily", testLockSchedule) {
		t.Fatal("refresh of our own lock should succeed")
	}
	if err := database.GetServerDB().QueryRow(
		"SELECT CAST(locked_at AS TEXT) FROM server_scheduler_state WHERE task_id = ?", "backup-daily",
	).Scan(&stored); err != nil {
		t.Fatalf("query refreshed locked_at: %v", err)
	}
	if _, err := time.Parse(sqlTimestampLayout, stored); err != nil {
		t.Errorf("refreshed locked_at = %q, want canonical %q text", stored, sqlTimestampLayout)
	}
}

// TestScheduler_AcquireTaskLock_StealIsConditional covers the compare-and-swap
// the single UPSERT used to provide implicitly: a node that decided to steal an
// expired lock must lose if another node claims the row first.
func TestScheduler_AcquireTaskLock_StealIsConditional(t *testing.T) {
	newSchedulerTestDBs(t)
	// nodeID is the hostname, so both instances would otherwise be the same
	// node and no steal could be observed.
	loser := NewScheduler(nil)
	loser.nodeID = "node-loser"
	winner := NewScheduler(nil)
	winner.nodeID = "node-winner"

	seedTaskLock(t, "backup-daily", "other-node", time.Now().UTC().Add(-2*time.Hour).Format(sqlTimestampLayout))

	if !winner.acquireTaskLock("backup-daily", testLockSchedule) {
		t.Fatal("the first node to steal an expired lock should win it")
	}

	// The freshly written lock is not expired, so the second node backs off
	// instead of overwriting a holder that is already running the task.
	if loser.acquireTaskLock("backup-daily", testLockSchedule) {
		t.Error("a second node must not steal a lock another node just took")
	}
	if holder := lockHolder(t, "backup-daily"); holder.String != winner.nodeID {
		t.Errorf("locked_by = %q, want %q", holder.String, winner.nodeID)
	}
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
		if !s.acquireTaskLock("backup-daily", testLockSchedule) {
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

			got := checkAndCreateAlerts(testUserID, 7, "Test City", weather)
			if got != tt.want {
				t.Errorf("checkAndCreateAlerts() = %d alerts, want %d", got, tt.want)
			}

			var count int
			if err := usersDB.QueryRow("SELECT COUNT(*) FROM user_notifications WHERE user_id = ?", testUserID).Scan(&count); err != nil {
				t.Fatalf("count notifications: %v", err)
			}
			if count != tt.want {
				t.Errorf("persisted notification rows = %d, want %d", count, tt.want)
			}
		})
	}
}

// TestCheckAndCreateAlerts_RowShape pins the shape of the rows the alert path
// writes against the real user_notifications DDL: a ULID text primary key, a
// type inside the column's CHECK list, a display value inside its own CHECK
// list, and the deep link carried in action_json (the table has no link
// column). Any of these written the old way is rejected by SQLite outright.
func TestCheckAndCreateAlerts_RowShape(t *testing.T) {
	_, usersDB := newSchedulerTestDBs(t)

	// 20F with 50mph wind, 1in of rain and weather code 99 fires the freezing,
	// wind, precipitation and severe-weather alerts, covering all three severities.
	if got := checkAndCreateAlerts(testUserID, 7, "Test City", weatherWith(20, 50, 1.0, 99)); got != 4 {
		t.Fatalf("checkAndCreateAlerts() = %d alerts, want 4", got)
	}

	rows, err := usersDB.Query(
		"SELECT id, type, display, action_json FROM user_notifications WHERE user_id = ? ORDER BY title",
		testUserID,
	)
	if err != nil {
		t.Fatalf("read notifications: %v", err)
	}
	defer rows.Close()

	allowedTypes := map[string]bool{"success": true, "info": true, "warning": true, "error": true, "security": true}

	seen := 0
	for rows.Next() {
		var id, notifType, display string
		var actionJSON sql.NullString
		if err := rows.Scan(&id, &notifType, &display, &actionJSON); err != nil {
			t.Fatalf("scan notification: %v", err)
		}
		seen++

		// ULIDs are 26 characters of Crockford base32; an INTEGER autoincrement
		// key would scan as "1", "2", ...
		if len(id) != 26 {
			t.Errorf("notification id = %q (len %d), want a 26-character ULID", id, len(id))
		}
		if !allowedTypes[notifType] {
			t.Errorf("notification type = %q, want one of the CHECK-permitted values", notifType)
		}
		if display != "toast" {
			t.Errorf("notification display = %q, want %q", display, "toast")
		}
		if !actionJSON.Valid || !strings.Contains(actionJSON.String, "/dashboard?location=7") {
			t.Errorf("notification action_json = %q, want it to carry the /dashboard?location=7 link", actionJSON.String)
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate notifications: %v", err)
	}
	if seen != 4 {
		t.Errorf("read back %d notification rows, want 4", seen)
	}
}

// --- top-level scheduled task functions: disabled/empty-data fast paths --------------

// expiryFixture is one seeded timestamp row: an explicit id, the value written
// into the DATETIME column, and whether the row is expected to survive cleanup.
type expiryFixture struct {
	id       int64
	name     string
	value    interface{}
	survives bool
}

// timestampFixtures returns the shared set of expiry rows every cleanup test
// seeds: Go time.Time values in the local zone (which the SQLite driver stores
// as time.Time.String(), i.e. local wall-clock text), canonical UTC text values
// (what CURRENT_TIMESTAMP and the explicitly-formatting writers produce), values
// in fixed zones far from local time, and rows either side of the cutoff by only
// a few seconds. Every value is derived from base, so a caller whose cleanup
// cutoff is not "now" can shift the whole set by passing a shifted base.
func timestampFixtures(base time.Time) []expiryFixture {
	jst := time.FixedZone("JST", 9*60*60)
	hst := time.FixedZone("HST", -10*60*60)

	return []expiryFixture{
		{id: 1, name: "local time.Time one hour in the past", value: base.Add(-1 * time.Hour), survives: false},
		{id: 2, name: "local time.Time one hour in the future", value: base.Add(1 * time.Hour), survives: true},
		{id: 3, name: "local time.Time one second in the past", value: base.Add(-1 * time.Second), survives: false},
		{id: 4, name: "local time.Time thirty seconds in the future", value: base.Add(30 * time.Second), survives: true},
		{id: 5, name: "canonical UTC text in the past", value: base.UTC().Add(-2 * time.Hour).Format(sqlTimestampLayout), survives: false},
		{id: 6, name: "canonical UTC text in the future", value: base.UTC().Add(2 * time.Hour).Format(sqlTimestampLayout), survives: true},
		{id: 7, name: "future instant stored in a +09:00 zone", value: base.Add(3 * time.Hour).In(jst), survives: true},
		{id: 8, name: "past instant stored in a -10:00 zone", value: base.Add(-3 * time.Hour).In(hst), survives: false},
	}
}

// seedColumn describes one extra column a real-schema table requires beyond its
// id and its timestamp: the column name and a function producing that row's
// value from the fixture id, so NOT NULL and UNIQUE constraints are satisfied
// per row rather than shared across rows.
type seedColumn struct {
	name  string
	value func(id int64) interface{}
}

// seedTimestampRows inserts the fixtures into table at their explicit ids,
// supplying any additional required columns the real schema declares.
func seedTimestampRows(t *testing.T, db *sql.DB, table, column string, fixtures []expiryFixture, extra ...seedColumn) {
	t.Helper()

	columns := []string{"id", column}
	for _, col := range extra {
		columns = append(columns, col.name)
	}
	placeholders := strings.TrimSuffix(strings.Repeat("?,", len(columns)), ",")
	query := fmt.Sprintf("INSERT INTO %s (%s) VALUES (%s)", table, strings.Join(columns, ", "), placeholders)

	for _, fixture := range fixtures {
		args := []interface{}{fixture.id, fixture.value}
		for _, col := range extra {
			args = append(args, col.value(fixture.id))
		}
		if _, err := db.Exec(query, args...); err != nil {
			t.Fatalf("seed %s id=%d (%s): %v", table, fixture.id, fixture.name, err)
		}
	}
}

// assertSurvivors checks that exactly the fixtures marked survives (plus any
// extra ids the caller lists) are still present in table.
func assertSurvivors(t *testing.T, db *sql.DB, table string, fixtures []expiryFixture, extraIDs ...int64) {
	t.Helper()

	present := map[int64]bool{}
	rows, err := db.Query(fmt.Sprintf("SELECT id FROM %s", table))
	if err != nil {
		t.Fatalf("read %s: %v", table, err)
	}
	defer rows.Close()
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			t.Fatalf("scan %s id: %v", table, err)
		}
		present[id] = true
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate %s: %v", table, err)
	}

	for _, fixture := range fixtures {
		if present[fixture.id] != fixture.survives {
			t.Errorf("%s id=%d (%s): present = %v, want %v", table, fixture.id, fixture.name, present[fixture.id], fixture.survives)
		}
	}
	for _, id := range extraIDs {
		if !present[id] {
			t.Errorf("%s id=%d: row was deleted, want it kept", table, id)
		}
	}
}

// CleanupOldSessions must delete a session only when its expiry is genuinely in
// the past. The expires_at column has more than one producer (a bound Go
// time.Time written in the local zone, and canonical UTC text), so the fixtures
// below cover both layouts, both sides of the cutoff, a near-boundary pair, and
// instants stored in zones far from local time.
func TestCleanupOldSessions(t *testing.T) {
	_, usersDB := newSchedulerTestDBs(t)
	fixtures := timestampFixtures(time.Now())
	// user_sessions.user_id and user_sessions.token_hash are NOT NULL in the real
	// schema, and token_hash is UNIQUE, so each row needs its own hash value.
	seedTimestampRows(t, usersDB, "user_sessions", "expires_at", fixtures,
		seedColumn{name: "user_id", value: func(int64) interface{} { return testUserID }},
		seedColumn{name: "token_hash", value: func(id int64) interface{} { return fmt.Sprintf("session-hash-%d", id) }},
	)

	if err := CleanupOldSessions(nil); err != nil {
		t.Fatalf("CleanupOldSessions() error: %v", err)
	}

	assertSurvivors(t, usersDB, "user_sessions", fixtures)
}

// CleanupOldAuditLogs prunes server_audit_log against its timestamp column (the
// table has no created_at), and the age test has to compare absolute instants:
// the rows below are seeded in fixed zones far from UTC so their wall-clock text
// orders the opposite way from the instants they represent.
func TestCleanupOldAuditLogs(t *testing.T) {
	t.Run("default retention (no config row) is 90 days", func(t *testing.T) {
		serverDB, _ := newSchedulerTestDBs(t)

		// The retained window is 90 days. Rows 1 and 2 sit well either side of it;
		// rows 3 and 4 sit one hour either side and are rendered in zones far
		// enough from UTC that their wall-clock text orders the opposite way from
		// the instants they represent, so a text comparison keeps the expired row
		// and deletes the live one no matter what timezone the host runs in.
		cutoff := time.Now().AddDate(0, 0, -90)
		fixtures := []expiryFixture{
			{id: 1, name: "100 days old", value: time.Now().AddDate(0, 0, -100), survives: false},
			{id: 2, name: "10 days old", value: time.Now().AddDate(0, 0, -10), survives: true},
			{id: 3, name: "one hour past the window, +13:00 text reads twelve hours newer", value: cutoff.Add(-1 * time.Hour).In(tokenZoneEast), survives: false},
			{id: 4, name: "one hour inside the window, -11:00 text reads ten hours older", value: cutoff.Add(1 * time.Hour).In(tokenZoneWest), survives: true},
		}

		// ulid is UNIQUE NOT NULL and action is NOT NULL in the real schema; the
		// column set here is the one both this package and
		// src/server/middleware/audit.go write.
		seedTimestampRows(t, serverDB, "server_audit_log", "timestamp", fixtures,
			seedColumn{name: "ulid", value: func(id int64) interface{} { return fmt.Sprintf("audit-ulid-%d", id) }},
			seedColumn{name: "actor_type", value: func(int64) interface{} { return "scheduler" }},
			seedColumn{name: "action", value: func(id int64) interface{} { return fmt.Sprintf("task-%d", id) }},
		)

		if err := CleanupOldAuditLogs(nil); err != nil {
			t.Fatalf("CleanupOldAuditLogs() error: %v", err)
		}

		assertSurvivors(t, serverDB, "server_audit_log", fixtures)
	})

	t.Run("configured retention (scheduler.cleanup_audit_logs_days) overrides the default", func(t *testing.T) {
		serverDB, _ := newSchedulerTestDBs(t)

		// A 5-day retention window: a row 10 days old must be pruned even
		// though it is well inside the 90-day default, proving the function
		// actually reads 'scheduler.cleanup_audit_logs_days' (the key the
		// admin panel writes) rather than silently falling back to 90.
		if _, err := serverDB.Exec(
			"INSERT INTO server_config (key, value) VALUES ('scheduler.cleanup_audit_logs_days','5')",
		); err != nil {
			t.Fatalf("failed to seed retention config: %v", err)
		}

		fixtures := []expiryFixture{
			{id: 1, name: "10 days old, outside 5-day window", value: time.Now().AddDate(0, 0, -10), survives: false},
			{id: 2, name: "1 day old, inside 5-day window", value: time.Now().AddDate(0, 0, -1), survives: true},
		}

		seedTimestampRows(t, serverDB, "server_audit_log", "timestamp", fixtures,
			seedColumn{name: "ulid", value: func(id int64) interface{} { return fmt.Sprintf("audit-ulid-cfg-%d", id) }},
			seedColumn{name: "actor_type", value: func(int64) interface{} { return "scheduler" }},
			seedColumn{name: "action", value: func(id int64) interface{} { return fmt.Sprintf("task-cfg-%d", id) }},
		)

		if err := CleanupOldAuditLogs(nil); err != nil {
			t.Fatalf("CleanupOldAuditLogs() error: %v", err)
		}

		assertSurvivors(t, serverDB, "server_audit_log", fixtures)
	})
}

// tokenZoneWest and tokenZoneEast are fixed offsets far enough from UTC that
// wall-clock text ordering and true instant ordering disagree on any host
// timezone: an instant one hour in the PAST rendered in tokenZoneEast reads
// twelve hours later than the UTC cutoff, and an instant one hour in the FUTURE
// rendered in tokenZoneWest reads ten hours earlier than it. A comparison that
// looked at stored text instead of absolute instants would therefore keep the
// expired token and delete the live one.
//
// The names matter as much as the offsets. A fixture bound as a time.Time is
// stored by the driver through time.Time.String(), which prints the zone's
// name, and time.Parse's MST element accepts only a three-to-five letter
// all-uppercase abbreviation (five letters must end in T). A name carrying
// digits or a sixth letter makes the whole stored value unparseable, which
// silently demotes these fixtures to another copy of the unparseable-value case
// instead of the instant-comparison case they exist to cover.
var (
	tokenZoneWest = time.FixedZone("WST", -11*60*60)
	tokenZoneEast = time.FixedZone("EAST", 13*60*60)
)

// seedUserToken inserts one row into the real user_tokens table. token_hash and
// token_prefix are NOT NULL in database.UsersSchema, so every row needs its own
// unique hash; the value written into expires_at is passed through verbatim so
// each case controls the exact stored layout and zone.
func seedUserToken(t *testing.T, db *sql.DB, id int64, expiresAt interface{}) {
	t.Helper()

	if _, err := db.Exec(
		"INSERT INTO user_tokens (id, user_id, token_hash, token_prefix, name, expires_at) VALUES (?, ?, ?, 'usr_1234', 'test token', ?)",
		id, testUserID, fmt.Sprintf("hash-%d", id), expiresAt,
	); err != nil {
		t.Fatalf("seed user_tokens id=%d: %v", id, err)
	}
}

// CleanupExpiredTokens must prune user_tokens - the project's only API-token
// table, which lives in the users database - removing only tokens whose expiry
// has actually passed, across every stored timestamp layout and zone, and must
// never touch a token with a NULL expiry.
func TestCleanupExpiredTokens(t *testing.T) {
	_, usersDB := newSchedulerTestDBs(t)

	base := time.Now()
	fixtures := []expiryFixture{
		{id: 1, name: "past instant whose +13:00 text reads as future", value: base.Add(-1 * time.Hour).In(tokenZoneEast), survives: false},
		{id: 2, name: "future instant whose -11:00 text reads as past", value: base.Add(1 * time.Hour).In(tokenZoneWest), survives: true},
		{id: 3, name: "canonical UTC text in the past", value: base.UTC().Add(-2 * time.Hour).Format(sqlTimestampLayout), survives: false},
		{id: 4, name: "canonical UTC text in the future", value: base.UTC().Add(2 * time.Hour).Format(sqlTimestampLayout), survives: true},
		{id: 5, name: "past instant one second before the cutoff", value: base.Add(-1 * time.Second), survives: false},
		{id: 6, name: "never-expiring token (NULL expiry)", value: nil, survives: true},
	}
	for _, fixture := range fixtures {
		seedUserToken(t, usersDB, fixture.id, fixture.value)
	}

	if err := CleanupExpiredTokens(nil); err != nil {
		t.Fatalf("CleanupExpiredTokens() error: %v", err)
	}

	assertSurvivors(t, usersDB, "user_tokens", fixtures)
}

// CleanupRateLimitCounters prunes windows that started more than an hour ago, so
// the fixture set is shifted an hour back: rows the shared fixtures treat as
// "expired" are then older than the one-hour cutoff, and the rest are inside it.
func TestCleanupRateLimitCounters(t *testing.T) {
	serverDB, _ := newSchedulerTestDBs(t)
	fixtures := timestampFixtures(time.Now().Add(-1 * time.Hour))
	// identifier and endpoint are NOT NULL in the real schema and form a UNIQUE
	// key together with window_start, so each row gets its own identifier.
	seedTimestampRows(t, serverDB, "server_rate_limits", "window_start", fixtures,
		seedColumn{name: "identifier", value: func(id int64) interface{} { return fmt.Sprintf("10.0.0.%d", id) }},
		seedColumn{name: "endpoint", value: func(int64) interface{} { return "/api/v1/weather" }},
	)

	if err := CleanupRateLimitCounters(nil); err != nil {
		t.Fatalf("CleanupRateLimitCounters() error: %v", err)
	}

	assertSurvivors(t, serverDB, "server_rate_limits", fixtures)
}

// parseStoredTimestamp underpins every cleanup comparison, so each layout the
// project can actually write must resolve to the same absolute instant, and an
// unrecognised or NULL value must report failure so the row is left alone.
func TestParseStoredTimestamp(t *testing.T) {
	instant := time.Date(2026, 3, 14, 15, 9, 26, 0, time.UTC)
	local := instant.In(time.FixedZone("IST", 5*60*60+30*60))

	tests := []struct {
		name  string
		value interface{}
		want  time.Time
		ok    bool
	}{
		{name: "driver-returned time.Time", value: local, want: instant, ok: true},
		{name: "time.Time.String() layout", value: local.Format("2006-01-02 15:04:05.999999999 -0700 MST"), want: instant, ok: true},
		{name: "time.Time.String() with monotonic reading", value: local.Format("2006-01-02 15:04:05.999999999 -0700 MST") + " m=+0.000000001", want: instant, ok: true},
		{name: "RFC3339 with offset", value: local.Format(time.RFC3339Nano), want: instant, ok: true},
		{name: "canonical UTC text", value: instant.Format(sqlTimestampLayout), want: instant, ok: true},
		{name: "canonical UTC text as bytes", value: []byte(instant.Format(sqlTimestampLayout)), want: instant, ok: true},
		{name: "UTC text with Z suffix", value: instant.Format(sqlTimestampLayout) + "Z", want: instant, ok: true},
		{name: "date only", value: "2026-03-14", want: time.Date(2026, 3, 14, 0, 0, 0, 0, time.UTC), ok: true},
		{name: "nil", value: nil, ok: false},
		{name: "empty string", value: "   ", ok: false},
		{name: "unparseable text", value: "not-a-timestamp", ok: false},
		{name: "unsupported type", value: 12345, ok: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := parseStoredTimestamp(tt.value)
			if ok != tt.ok {
				t.Fatalf("parseStoredTimestamp(%v) ok = %v, want %v", tt.value, ok, tt.ok)
			}
			if !tt.ok {
				return
			}
			if !got.Equal(tt.want) {
				t.Errorf("parseStoredTimestamp(%v) = %s, want %s", tt.value, got, tt.want)
			}
		})
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

		// server_nodes.hostname is NOT NULL and this upsert is the row's only
		// creator, so it has to supply the node's real name - resolved the same
		// way the rest of the app names itself, via util.GetFQDN.
		var hostname string
		if err := serverDB.QueryRow("SELECT hostname FROM server_nodes WHERE node_id = 'node-1'").Scan(&hostname); err != nil {
			t.Fatalf("query hostname: %v", err)
		}
		if hostname == "" {
			t.Error("hostname is empty, want the node's resolved host name")
		}
		if want := util.GetFQDN(); hostname != want {
			t.Errorf("hostname = %q, want %q", hostname, want)
		}

		// last_heartbeat is bound from Go as canonical UTC text rather than
		// emitted by SQLite's datetime(), so it must parse with the shared
		// timestamp parser and land on the current instant. Readers compare it
		// against a Go-side UTC cutoff, so a value in any other layout or zone
		// would make a node look stale.
		var storedHeartbeat interface{}
		if err := serverDB.QueryRow("SELECT last_heartbeat FROM server_nodes WHERE node_id = 'node-1'").Scan(&storedHeartbeat); err != nil {
			t.Fatalf("query heartbeat: %v", err)
		}
		parsed, ok := parseStoredTimestamp(storedHeartbeat)
		if !ok {
			t.Fatalf("last_heartbeat %v did not parse", storedHeartbeat)
		}
		if drift := time.Since(parsed); drift < -time.Minute || drift > time.Minute {
			t.Errorf("last_heartbeat is %s away from now, want within a minute", drift)
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
	if err := CheckWeatherAlerts(); err != nil {
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

// --- logTaskExecution ------------------------------------------------------------------

func TestSchedulerLogTaskExecution(t *testing.T) {
	t.Run("audit disabled by default: no row written", func(t *testing.T) {
		serverDB, _ := newSchedulerTestDBs(t)
		s := NewScheduler(nil)

		s.logTaskExecution("task-a", 10*time.Millisecond, nil)

		var count int
		if err := serverDB.QueryRow("SELECT COUNT(*) FROM server_audit_log").Scan(&count); err != nil {
			t.Fatalf("count audit log: %v", err)
		}
		if count != 0 {
			t.Errorf("audit log rows = %d, want 0 when audit.enabled is unset", count)
		}
	})

	t.Run("audit.enabled=false explicitly still skips logging", func(t *testing.T) {
		serverDB, _ := newSchedulerTestDBs(t)
		if _, err := serverDB.Exec("INSERT INTO server_config (key, value) VALUES ('audit.enabled','false')"); err != nil {
			t.Fatalf("seed config: %v", err)
		}
		s := NewScheduler(nil)

		s.logTaskExecution("task-a2", 10*time.Millisecond, nil)

		var count int
		if err := serverDB.QueryRow("SELECT COUNT(*) FROM server_audit_log").Scan(&count); err != nil {
			t.Fatalf("count audit log: %v", err)
		}
		if count != 0 {
			t.Errorf("audit log rows = %d, want 0 when audit.enabled=false", count)
		}
	})

	t.Run("audit enabled records a successful run", func(t *testing.T) {
		serverDB, _ := newSchedulerTestDBs(t)
		if _, err := serverDB.Exec("INSERT INTO server_config (key, value) VALUES ('audit.enabled','true')"); err != nil {
			t.Fatalf("seed config: %v", err)
		}
		s := NewScheduler(nil)

		s.logTaskExecution("task-b", 25*time.Millisecond, nil)

		// The actor is described by actor_type/actor_id (there is no user_id
		// column) and ulid is UNIQUE NOT NULL, exactly as
		// src/server/middleware/audit.go writes it.
		var status, details, auditULID, actorType, actorID string
		var storedTimestamp interface{}
		if err := serverDB.QueryRow(
			"SELECT status, details, ulid, actor_type, actor_id, timestamp FROM server_audit_log WHERE action = 'task-b' AND resource_id = 'task-b'",
		).Scan(&status, &details, &auditULID, &actorType, &actorID, &storedTimestamp); err != nil {
			t.Fatalf("query audit log row: %v", err)
		}
		if status != "success" {
			t.Errorf("status = %q, want success", status)
		}
		if !strings.Contains(details, "Completed in") {
			t.Errorf("details = %q, want it to contain %q", details, "Completed in")
		}
		// ULIDs are 26 characters of Crockford base32.
		if len(auditULID) != 26 {
			t.Errorf("ulid = %q (len %d), want a 26-character ULID", auditULID, len(auditULID))
		}
		if actorType != "scheduler" {
			t.Errorf("actor_type = %q, want %q", actorType, "scheduler")
		}
		if actorID != s.nodeID {
			t.Errorf("actor_id = %q, want the scheduler node id %q", actorID, s.nodeID)
		}
		// timestamp is bound as canonical UTC text so CleanupOldAuditLogs can
		// compare it against a Go-side cutoff.
		parsed, ok := parseStoredTimestamp(storedTimestamp)
		if !ok {
			t.Fatalf("timestamp %v did not parse", storedTimestamp)
		}
		if drift := time.Since(parsed); drift < -time.Minute || drift > time.Minute {
			t.Errorf("timestamp is %s away from now, want within a minute", drift)
		}
	})

	t.Run("audit enabled records a failed run with the error message", func(t *testing.T) {
		serverDB, _ := newSchedulerTestDBs(t)
		if _, err := serverDB.Exec("INSERT INTO server_config (key, value) VALUES ('audit.enabled','true')"); err != nil {
			t.Fatalf("seed config: %v", err)
		}
		s := NewScheduler(nil)

		s.logTaskExecution("task-c", 5*time.Millisecond, fmt.Errorf("boom"))

		var status, details string
		if err := serverDB.QueryRow(
			"SELECT status, details FROM server_audit_log WHERE action = 'task-c' AND resource_id = 'task-c'",
		).Scan(&status, &details); err != nil {
			t.Fatalf("query audit log row: %v", err)
		}
		if status != "error" {
			t.Errorf("status = %q, want error", status)
		}
		if !strings.Contains(details, "Failed: boom") {
			t.Errorf("details = %q, want it to contain %q", details, "Failed: boom")
		}
	})
}

// TestParseNVDPublished pins the conversion that replaced storing the NVD
// API's own "published" string verbatim in a DATETIME column.
func TestParseNVDPublished(t *testing.T) {
	want := time.Date(2026, 3, 14, 15, 4, 5, 0, time.UTC)

	t.Run("accepted layouts resolve to the same UTC instant", func(t *testing.T) {
		cases := map[string]string{
			"documented millisecond form": "2026-03-14T15:04:05.000",
			"no fractional seconds":       "2026-03-14T15:04:05",
			"rfc3339 utc":                 "2026-03-14T15:04:05Z",
			"surrounding whitespace":      "  2026-03-14T15:04:05.000  ",
		}

		for name, input := range cases {
			parsed, err := parseNVDPublished(input)
			if err != nil {
				t.Errorf("%s: unexpected error: %v", name, err)
				continue
			}
			if !parsed.Equal(want) {
				t.Errorf("%s: parsed = %s, want %s", name, parsed, want)
			}
			if parsed.Location() != time.UTC {
				t.Errorf("%s: location = %s, want UTC", name, parsed.Location())
			}
		}
	})

	t.Run("offset form is normalized to utc", func(t *testing.T) {
		parsed, err := parseNVDPublished("2026-03-15T04:04:05+13:00")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !parsed.Equal(want) {
			t.Errorf("parsed = %s, want %s", parsed, want)
		}
	})

	t.Run("unreadable values error rather than guess", func(t *testing.T) {
		for _, input := range []string{"", "   ", "not-a-timestamp", "2026-03-14 15:04:05"} {
			if parsed, err := parseNVDPublished(input); err == nil {
				t.Errorf("input %q: expected an error, got %s", input, parsed)
			}
		}
	})
}
