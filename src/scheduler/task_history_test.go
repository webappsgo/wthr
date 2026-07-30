package scheduler

import (
	"errors"
	"testing"
	"time"
)

func TestInitTaskHistoryTable(t *testing.T) {
	newSchedulerTestDBs(t)
	s := NewScheduler(nil)

	if err := s.InitTaskHistoryTable(); err != nil {
		t.Fatalf("InitTaskHistoryTable() error: %v", err)
	}
	// Idempotent: calling twice must not error (CREATE TABLE IF NOT EXISTS).
	if err := s.InitTaskHistoryTable(); err != nil {
		t.Fatalf("second InitTaskHistoryTable() error: %v", err)
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
