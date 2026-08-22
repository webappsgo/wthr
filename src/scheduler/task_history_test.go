package scheduler

import (
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/webappsgo/wthr/src/database"
)

func TestInitTaskHistoryTable(t *testing.T) {
	newSchedulerTestDBs(t)
	s := NewScheduler(nil)

	if err := s.InitTaskHistoryTable(); err != nil {
		t.Fatalf("InitTaskHistoryTable() error: %v", err)
	}
	// Idempotent: calling twice must not error.
	if err := s.InitTaskHistoryTable(); err != nil {
		t.Fatalf("second InitTaskHistoryTable() error: %v", err)
	}
}

// TestTaskHistoryTableComesFromServerSchema proves the table and its indexes
// are declared in database.ServerSchema rather than created at scheduler start.
// newSchedulerTestDBs applies nothing but the real schema constants, so this
// query only succeeds if the DDL really moved; against the old code, where the
// DDL lived in InitTaskHistoryTable, the table would be absent here.
func TestTaskHistoryTableComesFromServerSchema(t *testing.T) {
	serverDB, _ := newSchedulerTestDBs(t)

	var name string
	if err := serverDB.QueryRow(
		"SELECT name FROM sqlite_master WHERE type = 'table' AND name = ?", "server_scheduler_history",
	).Scan(&name); err != nil {
		t.Fatalf("server_scheduler_history not created by database.ServerSchema: %v", err)
	}

	for _, index := range []string{"idx_server_scheduler_history_name", "idx_server_scheduler_history_start"} {
		var found string
		if err := serverDB.QueryRow(
			"SELECT name FROM sqlite_master WHERE type = 'index' AND name = ?", index,
		).Scan(&found); err != nil {
			t.Errorf("index %s not created by database.ServerSchema: %v", index, err)
		}
	}

	// The column set has to be byte-for-byte what the runtime DDL created, or
	// an existing database (where CREATE TABLE IF NOT EXISTS is a no-op) would
	// disagree with a fresh one.
	rows, err := serverDB.Query("SELECT name FROM pragma_table_info('server_scheduler_history') ORDER BY cid")
	if err != nil {
		t.Fatalf("read table info: %v", err)
	}
	defer rows.Close()

	var columns []string
	for rows.Next() {
		var column string
		if err := rows.Scan(&column); err != nil {
			t.Fatalf("scan column: %v", err)
		}
		columns = append(columns, column)
	}

	want := []string{"id", "task_name", "start_time", "end_time", "duration_ms", "status", "error", "created_at"}
	if len(columns) != len(want) {
		t.Fatalf("columns = %v, want %v", columns, want)
	}
	for i := range want {
		if columns[i] != want[i] {
			t.Errorf("column %d = %q, want %q", i, columns[i], want[i])
		}
	}
}

// TestInitTaskHistoryTableReportsMissingSchema pins the remaining job of
// InitTaskHistoryTable: it no longer creates anything, so a server database
// that never had database.ServerSchema applied must fail loudly at startup
// instead of silently gaining a table at scheduler start.
func TestInitTaskHistoryTableReportsMissingSchema(t *testing.T) {
	newSchedulerTestDBs(t)
	s := NewScheduler(nil)

	if _, err := database.GetServerDB().Exec("DROP TABLE server_scheduler_history"); err != nil {
		t.Fatalf("drop table: %v", err)
	}

	if err := s.InitTaskHistoryTable(); err == nil {
		t.Error("InitTaskHistoryTable() = nil, want an error when the table is missing")
	}
}

func TestRecordAndQueryTaskRun(t *testing.T) {
	newSchedulerTestDBs(t)
	s := NewScheduler(nil)
	if err := s.InitTaskHistoryTable(); err != nil {
		t.Fatalf("InitTaskHistoryTable() error: %v", err)
	}

	t.Run("successful run is recorded with success status and no error text", func(t *testing.T) {
		start := time.Now()
		end := start.Add(250 * time.Millisecond)
		if err := s.RecordTaskRun("job-x", start, end, nil); err != nil {
			t.Fatalf("RecordTaskRun() error: %v", err)
		}

		last, err := s.GetLastTaskRun("job-x")
		if err != nil {
			t.Fatalf("GetLastTaskRun() error: %v", err)
		}
		if last == nil {
			t.Fatal("GetLastTaskRun() returned nil for a recorded run")
		}
		if last.Status != "success" {
			t.Errorf("Status = %q, want success", last.Status)
		}
		if last.Error != "" {
			t.Errorf("Error = %q, want empty", last.Error)
		}
		if last.Duration != 250 {
			t.Errorf("Duration = %d, want 250", last.Duration)
		}
	})

	t.Run("failed run is recorded with error status and message", func(t *testing.T) {
		start := time.Now()
		end := start.Add(50 * time.Millisecond)
		if err := s.RecordTaskRun("job-y", start, end, errFakeTaskFailure); err != nil {
			t.Fatalf("RecordTaskRun() error: %v", err)
		}

		last, err := s.GetLastTaskRun("job-y")
		if err != nil {
			t.Fatalf("GetLastTaskRun() error: %v", err)
		}
		if last == nil {
			t.Fatal("GetLastTaskRun() returned nil for a recorded run")
		}
		if last.Status != "error" {
			t.Errorf("Status = %q, want error", last.Status)
		}
		if last.Error != errFakeTaskFailure.Error() {
			t.Errorf("Error = %q, want %q", last.Error, errFakeTaskFailure.Error())
		}
	})
}

func TestGetLastTaskRun_NoRowsReturnsNilNil(t *testing.T) {
	newSchedulerTestDBs(t)
	s := NewScheduler(nil)
	if err := s.InitTaskHistoryTable(); err != nil {
		t.Fatalf("InitTaskHistoryTable() error: %v", err)
	}

	last, err := s.GetLastTaskRun("never-ran")
	if err != nil {
		t.Fatalf("GetLastTaskRun() error: %v, want nil error on no rows", err)
	}
	if last != nil {
		t.Errorf("GetLastTaskRun() = %+v, want nil", last)
	}
}

func TestGetTaskHistory(t *testing.T) {
	newSchedulerTestDBs(t)
	s := NewScheduler(nil)
	if err := s.InitTaskHistoryTable(); err != nil {
		t.Fatalf("InitTaskHistoryTable() error: %v", err)
	}

	t.Run("empty history returns an empty slice, not an error", func(t *testing.T) {
		history, err := s.GetTaskHistory("nothing-here", 10)
		if err != nil {
			t.Fatalf("GetTaskHistory() error: %v", err)
		}
		if len(history) != 0 {
			t.Errorf("len(history) = %d, want 0", len(history))
		}
	})

	t.Run("single entry", func(t *testing.T) {
		start := time.Now()
		if err := s.RecordTaskRun("single-job", start, start.Add(10*time.Millisecond), nil); err != nil {
			t.Fatalf("RecordTaskRun() error: %v", err)
		}
		history, err := s.GetTaskHistory("single-job", 10)
		if err != nil {
			t.Fatalf("GetTaskHistory() error: %v", err)
		}
		if len(history) != 1 {
			t.Fatalf("len(history) = %d, want 1", len(history))
		}
	})

	t.Run("many entries are ordered most-recent-first", func(t *testing.T) {
		base := time.Now().Add(-1 * time.Hour)
		for i := 0; i < 5; i++ {
			start := base.Add(time.Duration(i) * time.Minute)
			if err := s.RecordTaskRun("many-job", start, start.Add(time.Second), nil); err != nil {
				t.Fatalf("RecordTaskRun() error: %v", err)
			}
		}
		history, err := s.GetTaskHistory("many-job", 10)
		if err != nil {
			t.Fatalf("GetTaskHistory() error: %v", err)
		}
		if len(history) != 5 {
			t.Fatalf("len(history) = %d, want 5", len(history))
		}
		for i := 0; i < len(history)-1; i++ {
			if history[i].StartTime.Before(history[i+1].StartTime) {
				t.Errorf("history not ordered DESC by start_time at index %d", i)
			}
		}
	})

	t.Run("limit boundary: zero and negative default to 50", func(t *testing.T) {
		for i := 0; i < 3; i++ {
			start := time.Now().Add(time.Duration(i) * time.Second)
			if err := s.RecordTaskRun("limit-job", start, start.Add(time.Millisecond), nil); err != nil {
				t.Fatalf("RecordTaskRun() error: %v", err)
			}
		}
		historyZero, err := s.GetTaskHistory("limit-job", 0)
		if err != nil {
			t.Fatalf("GetTaskHistory(limit=0) error: %v", err)
		}
		historyNeg, err := s.GetTaskHistory("limit-job", -5)
		if err != nil {
			t.Fatalf("GetTaskHistory(limit=-5) error: %v", err)
		}
		if len(historyZero) != 3 || len(historyNeg) != 3 {
			t.Errorf("expected all 3 rows returned under default limit=50, got zero=%d neg=%d", len(historyZero), len(historyNeg))
		}
	})

	t.Run("limit caps the number of rows returned", func(t *testing.T) {
		for i := 0; i < 5; i++ {
			start := time.Now().Add(time.Duration(i) * time.Second)
			if err := s.RecordTaskRun("capped-job", start, start.Add(time.Millisecond), nil); err != nil {
				t.Fatalf("RecordTaskRun() error: %v", err)
			}
		}
		history, err := s.GetTaskHistory("capped-job", 2)
		if err != nil {
			t.Fatalf("GetTaskHistory(limit=2) error: %v", err)
		}
		if len(history) != 2 {
			t.Errorf("len(history) = %d, want 2", len(history))
		}
	})
}

func TestGetTaskStats(t *testing.T) {
	newSchedulerTestDBs(t)
	s := NewScheduler(nil)
	if err := s.InitTaskHistoryTable(); err != nil {
		t.Fatalf("InitTaskHistoryTable() error: %v", err)
	}

	// BUG (not fixed, per task constraints): GetTaskStats (task_history.go) scans
	// SQL SUM(CASE ...) directly into plain int success/errors return values.
	// SUM() over zero matching rows evaluates to SQL NULL, and *int cannot
	// receive NULL, so this call fails with a Scan error instead of returning
	// zeroed stats. This test documents the actual (buggy) behavior.
	t.Run("no history yields a NULL-scan error instead of zeroed stats", func(t *testing.T) {
		_, _, _, err := s.GetTaskStats("no-history")
		if err == nil {
			t.Fatal("GetTaskStats() error = nil, want a NULL-scan error given the current SUM()-over-zero-rows bug - if this now succeeds, the bug has been fixed upstream and this test should be reverted to asserting zeroed stats with no error")
		}
	})

	t.Run("mixed success/error runs are counted correctly", func(t *testing.T) {
		start := time.Now()
		if err := s.RecordTaskRun("stats-job", start, start.Add(time.Second), nil); err != nil {
			t.Fatalf("RecordTaskRun() error: %v", err)
		}
		if err := s.RecordTaskRun("stats-job", start, start.Add(time.Second), nil); err != nil {
			t.Fatalf("RecordTaskRun() error: %v", err)
		}
		if err := s.RecordTaskRun("stats-job", start, start.Add(time.Second), errFakeTaskFailure); err != nil {
			t.Fatalf("RecordTaskRun() error: %v", err)
		}

		total, success, errs, err := s.GetTaskStats("stats-job")
		if err != nil {
			t.Fatalf("GetTaskStats() error: %v", err)
		}
		if total != 3 {
			t.Errorf("total = %d, want 3", total)
		}
		if success != 2 {
			t.Errorf("success = %d, want 2", success)
		}
		if errs != 1 {
			t.Errorf("errors = %d, want 1", errs)
		}
	})
}

func TestGetAllTaskInfo(t *testing.T) {
	newSchedulerTestDBs(t)
	s := NewScheduler(nil)
	if err := s.InitTaskHistoryTable(); err != nil {
		t.Fatalf("InitTaskHistoryTable() error: %v", err)
	}

	if err := s.AddTask("info-job", "@daily", func() error { return nil }); err != nil {
		t.Fatalf("AddTask() error: %v", err)
	}
	start := time.Now()
	if err := s.RecordTaskRun("info-job", start, start.Add(time.Second), nil); err != nil {
		t.Fatalf("RecordTaskRun() error: %v", err)
	}

	infos, err := s.GetAllTaskInfo()
	if err != nil {
		t.Fatalf("GetAllTaskInfo() error: %v", err)
	}
	if len(infos) != 1 {
		t.Fatalf("len(infos) = %d, want 1", len(infos))
	}
	info := infos[0]
	if info.Name != "info-job" {
		t.Errorf("Name = %q, want info-job", info.Name)
	}
	if !info.Enabled {
		t.Error("Enabled = false, want true")
	}
	if info.NextRun == nil {
		t.Error("NextRun = nil, want a computed next-run time for an enabled task")
	}
	if info.LastRun == nil {
		t.Error("LastRun = nil, want the recorded run")
	}
	if info.RunCount != 1 {
		t.Errorf("RunCount = %d, want 1", info.RunCount)
	}
}

func TestCleanupOldTaskHistory(t *testing.T) {
	newSchedulerTestDBs(t)
	s := NewScheduler(nil)
	if err := s.InitTaskHistoryTable(); err != nil {
		t.Fatalf("InitTaskHistoryTable() error: %v", err)
	}

	old := time.Now().AddDate(0, 0, -100)
	recent := time.Now().AddDate(0, 0, -10)
	if err := s.RecordTaskRun("cleanup-job", old, old.Add(time.Second), nil); err != nil {
		t.Fatalf("RecordTaskRun() error: %v", err)
	}
	if err := s.RecordTaskRun("cleanup-job", recent, recent.Add(time.Second), nil); err != nil {
		t.Fatalf("RecordTaskRun() error: %v", err)
	}

	t.Run("days<=0 defaults to 90 days retention", func(t *testing.T) {
		if err := s.CleanupOldTaskHistory(0); err != nil {
			t.Fatalf("CleanupOldTaskHistory(0) error: %v", err)
		}
		history, err := s.GetTaskHistory("cleanup-job", 10)
		if err != nil {
			t.Fatalf("GetTaskHistory() error: %v", err)
		}
		if len(history) != 1 {
			t.Fatalf("len(history) = %d, want 1 (only the 100-day-old row removed)", len(history))
		}
	})

	t.Run("explicit small retention removes the remaining row too", func(t *testing.T) {
		if err := s.CleanupOldTaskHistory(5); err != nil {
			t.Fatalf("CleanupOldTaskHistory(5) error: %v", err)
		}
		history, err := s.GetTaskHistory("cleanup-job", 10)
		if err != nil {
			t.Fatalf("GetTaskHistory() error: %v", err)
		}
		if len(history) != 0 {
			t.Errorf("len(history) = %d, want 0 after 5-day retention cleanup", len(history))
		}
	})
}

// errFakeTaskFailure is a stable sentinel error used to assert error-path recording.
var errFakeTaskFailure = errors.New("simulated task failure")

// historyOffsetLayout renders an instant with a NUMERIC offset and no zone
// name. Every layout carrying the MST element instead prints the fixed zone's
// abbreviation, and an abbreviation the time package refuses to parse would
// make the whole fixture unreadable - which would quietly turn these cases into
// another copy of the unparseable-value case rather than the mixed-layout case
// they exist to cover.
const historyOffsetLayout = "2006-01-02 15:04:05-07:00"

// historyZoneWest and historyZoneEast are fixed offsets far enough apart that
// the wall-clock text of two instants orders the opposite way from the instants
// themselves: the same moment reads eleven hours earlier in the west zone than
// in UTC and thirteen hours later in the east zone. Both names are short,
// uppercase and digit-free, so time.Parse accepts them wherever a layout does
// print a zone name.
var (
	historyZoneWest = time.FixedZone("WST", -11*60*60)
	historyZoneEast = time.FixedZone("EAST", 13*60*60)
)

// seedTaskHistoryRow inserts one server_scheduler_history row at an explicit
// id, writing start_time and end_time exactly as given so each case controls
// the stored layout and zone. The column list is the one RecordTaskRun writes;
// the table itself comes from database.ServerSchema via newSchedulerTestDBs, so
// no fixture here can drift from the production DDL.
func seedTaskHistoryRow(t *testing.T, db *sql.DB, id int64, taskName string, startTime, endTime interface{}) {
	t.Helper()

	if _, err := db.Exec(`
		INSERT INTO server_scheduler_history (id, task_name, start_time, end_time, duration_ms, status, error)
		VALUES (?, ?, ?, ?, 1000, 'success', '')
	`, id, taskName, startTime, endTime); err != nil {
		t.Fatalf("seed server_scheduler_history id=%d: %v", id, err)
	}
}

// TestRecordTaskRun_WritesCanonicalTimestamps proves the write side stores
// canonical UTC text rather than letting the driver render the bound time.Time
// with time.Time.String() in the host's local zone. Against the old code the
// stored bytes carried an offset and a zone name, so this parse fails.
func TestRecordTaskRun_WritesCanonicalTimestamps(t *testing.T) {
	serverDB, _ := newSchedulerTestDBs(t)
	s := NewScheduler(nil)
	if err := s.InitTaskHistoryTable(); err != nil {
		t.Fatalf("InitTaskHistoryTable() error: %v", err)
	}

	// A start instant deliberately carried in a far-from-UTC zone: the value
	// stored must describe the same instant in UTC regardless.
	start := time.Date(2026, 3, 14, 12, 0, 0, 0, time.UTC).In(historyZoneEast)
	end := start.Add(2 * time.Second)
	if err := s.RecordTaskRun("canonical-job", start, end, nil); err != nil {
		t.Fatalf("RecordTaskRun() error: %v", err)
	}

	// CAST keeps the driver from re-rendering the DATETIME column as a
	// time.Time on the way out, so the assertion sees the bytes actually stored.
	var storedStart, storedEnd string
	if err := serverDB.QueryRow(
		"SELECT CAST(start_time AS TEXT), CAST(end_time AS TEXT) FROM server_scheduler_history WHERE task_name = ?", "canonical-job",
	).Scan(&storedStart, &storedEnd); err != nil {
		t.Fatalf("query stored timestamps: %v", err)
	}

	if want := start.UTC().Format(sqlTimestampLayout); storedStart != want {
		t.Errorf("start_time = %q, want canonical %q", storedStart, want)
	}
	if want := end.UTC().Format(sqlTimestampLayout); storedEnd != want {
		t.Errorf("end_time = %q, want canonical %q", storedEnd, want)
	}
}

// TestTaskHistoryOrderingAcrossStoredLayouts is the regression for the reader
// side: history rows left by earlier builds hold local-zone text, so ordering
// them with the SQL "ORDER BY start_time DESC" compares wall clocks and leading
// characters instead of instants. The fixtures below are written so that text
// ordering and instant ordering disagree on every host: the true newest run
// renders as 01:00 in a -11:00 zone while a run ten hours older renders as
// 15:00 in a +13:00 zone. Against the old code GetLastTaskRun returns the older
// run and GetTaskHistory lists it first.
func TestTaskHistoryOrderingAcrossStoredLayouts(t *testing.T) {
	serverDB, _ := newSchedulerTestDBs(t)
	s := NewScheduler(nil)
	if err := s.InitTaskHistoryTable(); err != nil {
		t.Fatalf("InitTaskHistoryTable() error: %v", err)
	}

	const taskName = "mixed-layout-job"
	base := time.Date(2026, 3, 14, 12, 0, 0, 0, time.UTC)

	// True newest run, stored as "2026-03-14 01:00:00-11:00".
	newestStart := base
	seedTaskHistoryRow(t, serverDB, 1, taskName,
		newestStart.In(historyZoneWest).Format(historyOffsetLayout),
		newestStart.Add(30*time.Second).In(historyZoneWest).Format(historyOffsetLayout))

	// Ten hours older, stored as "2026-03-14 15:00:00+13:00" - text that sorts
	// after the row above even though the instant is earlier.
	olderStart := base.Add(-10 * time.Hour)
	seedTaskHistoryRow(t, serverDB, 2, taskName,
		olderStart.In(historyZoneEast).Format(historyOffsetLayout),
		olderStart.Add(30*time.Second).In(historyZoneEast).Format(historyOffsetLayout))

	// A value no layout can parse, inserted at the highest explicit id so an
	// implementation that fell back to insertion order would pick it as "last
	// run". It must sort to the end instead: a row this code cannot interpret
	// must never be reported as the most recent execution.
	seedTaskHistoryRow(t, serverDB, 3, taskName, "not-a-timestamp", "not-a-timestamp")

	// The oldest run of all, recorded through the current writer, so the table
	// really does hold canonical and legacy encodings side by side.
	oldestStart := base.Add(-20 * time.Hour)
	if err := s.RecordTaskRun(taskName, oldestStart, oldestStart.Add(30*time.Second), nil); err != nil {
		t.Fatalf("RecordTaskRun() error: %v", err)
	}

	last, err := s.GetLastTaskRun(taskName)
	if err != nil {
		t.Fatalf("GetLastTaskRun() error: %v", err)
	}
	if last == nil {
		t.Fatal("GetLastTaskRun() = nil, want the newest recorded run")
	}
	if last.ID != 1 {
		t.Errorf("GetLastTaskRun() id = %d, want 1 (the true most recent run)", last.ID)
	}
	if !last.StartTime.Equal(newestStart) {
		t.Errorf("GetLastTaskRun() start = %s, want %s", last.StartTime, newestStart)
	}
	if !last.EndTime.Equal(newestStart.Add(30 * time.Second)) {
		t.Errorf("GetLastTaskRun() end = %s, want %s", last.EndTime, newestStart.Add(30*time.Second))
	}

	history, err := s.GetTaskHistory(taskName, 10)
	if err != nil {
		t.Fatalf("GetTaskHistory() error: %v", err)
	}
	if len(history) != 4 {
		t.Fatalf("len(history) = %d, want 4", len(history))
	}
	if history[0].ID != 1 {
		t.Errorf("history[0].id = %d, want 1", history[0].ID)
	}
	if history[1].ID != 2 {
		t.Errorf("history[1].id = %d, want 2", history[1].ID)
	}
	if !history[2].StartTime.Equal(oldestStart) {
		t.Errorf("history[2].start = %s, want the canonically written %s", history[2].StartTime, oldestStart)
	}
	if history[3].ID != 3 {
		t.Errorf("history[3].id = %d, want 3 (the unparseable row sorts last)", history[3].ID)
	}
	if !history[3].StartTime.IsZero() {
		t.Errorf("history[3].start = %s, want the zero time for an unparseable value", history[3].StartTime)
	}
	for i := 0; i < len(history)-1; i++ {
		if history[i].StartTime.Before(history[i+1].StartTime) {
			t.Errorf("history not ordered by descending instant at index %d", i)
		}
	}

	// The limit has to be applied after the true ordering: applied in SQL on
	// top of a text ORDER BY it would keep the wrong row and drop the newest.
	limited, err := s.GetTaskHistory(taskName, 1)
	if err != nil {
		t.Fatalf("GetTaskHistory(limit=1) error: %v", err)
	}
	if len(limited) != 1 {
		t.Fatalf("len(limited) = %d, want 1", len(limited))
	}
	if limited[0].ID != 1 {
		t.Errorf("limited[0].id = %d, want 1 (the true most recent run)", limited[0].ID)
	}
}

// TestGetAllTaskInfoUsesTrueLastRun pins the same ordering fix at the surface
// the admin panel reads: TaskInfo.LastRun comes from GetLastTaskRun, so a
// history holding a legacy local-zone row would otherwise report a stale run as
// the task's last execution.
func TestGetAllTaskInfoUsesTrueLastRun(t *testing.T) {
	serverDB, _ := newSchedulerTestDBs(t)
	s := NewScheduler(nil)
	if err := s.InitTaskHistoryTable(); err != nil {
		t.Fatalf("InitTaskHistoryTable() error: %v", err)
	}

	if err := s.AddTask("mixed-info-job", "@daily", func() error { return nil }); err != nil {
		t.Fatalf("AddTask() error: %v", err)
	}

	base := time.Date(2026, 3, 14, 12, 0, 0, 0, time.UTC)
	seedTaskHistoryRow(t, serverDB, 1, "mixed-info-job",
		base.In(historyZoneWest).Format(historyOffsetLayout),
		base.Add(30*time.Second).In(historyZoneWest).Format(historyOffsetLayout))
	seedTaskHistoryRow(t, serverDB, 2, "mixed-info-job",
		base.Add(-10*time.Hour).In(historyZoneEast).Format(historyOffsetLayout),
		base.Add(-10*time.Hour).Add(30*time.Second).In(historyZoneEast).Format(historyOffsetLayout))

	infos, err := s.GetAllTaskInfo()
	if err != nil {
		t.Fatalf("GetAllTaskInfo() error: %v", err)
	}
	if len(infos) != 1 {
		t.Fatalf("len(infos) = %d, want 1", len(infos))
	}
	if infos[0].LastRun == nil {
		t.Fatal("LastRun = nil, want the newest recorded run")
	}
	if infos[0].LastRun.ID != 1 {
		t.Errorf("LastRun.id = %d, want 1 (the true most recent run)", infos[0].LastRun.ID)
	}
}
