// Tests for the AI.md PART 19 resilience behaviors: panic isolation, the
// retry policy (max_retries 3 with 5m/10m/20m backoff) and startup catch-up of
// occurrences missed inside catch_up_window.
package scheduler

import (
	"errors"
	"testing"
	"time"
)

func TestRunTaskFn_RecoversPanic(t *testing.T) {
	task := &Task{
		Name: "panicking",
		Fn:   func() error { panic("boom") },
	}

	err := runTaskFn(task)
	if err == nil {
		t.Fatal("runTaskFn() = nil, want an error describing the panic")
	}
	if got := err.Error(); got != "task panicked: boom" {
		t.Errorf("runTaskFn() error = %q, want %q", got, "task panicked: boom")
	}
}

func TestRunTaskFn_PassesThroughResult(t *testing.T) {
	want := errors.New("task failed")
	task := &Task{Name: "failing", Fn: func() error { return want }}

	if err := runTaskFn(task); !errors.Is(err, want) {
		t.Errorf("runTaskFn() error = %v, want %v", err, want)
	}

	ok := &Task{Name: "ok", Fn: func() error { return nil }}
	if err := runTaskFn(ok); err != nil {
		t.Errorf("runTaskFn() error = %v, want nil", err)
	}
}

func TestApplyRetryPolicy_BackoffSequence(t *testing.T) {
	s := NewScheduler(nil)
	end := time.Date(2026, 1, 2, 3, 0, 0, 0, time.UTC)
	// A next run far in the future so every retry is the earlier time and wins.
	task := &Task{Name: "retrying", nextRun: end.Add(24 * time.Hour)}
	runErr := errors.New("failed")

	for attempt, delay := range retryDelays {
		// executeTask recomputes nextRun from the schedule before every
		// applyRetryPolicy call, so the fixture does the same.
		task.nextRun = end.Add(24 * time.Hour)
		s.applyRetryPolicy(task, end, runErr)

		if task.retryCount != attempt+1 {
			t.Fatalf("attempt %d: retryCount = %d, want %d", attempt+1, task.retryCount, attempt+1)
		}
		if want := end.Add(delay); !task.nextRun.Equal(want) {
			t.Fatalf("attempt %d: nextRun = %s, want %s", attempt+1, task.nextRun, want)
		}
	}

	// Retries are exhausted: the counter resets and the task falls back to its
	// own schedule rather than being pushed out again.
	beforeExhausted := task.nextRun
	s.applyRetryPolicy(task, end, runErr)
	if task.retryCount != 0 {
		t.Errorf("retryCount after exhaustion = %d, want 0", task.retryCount)
	}
	if !task.nextRun.Equal(beforeExhausted) {
		t.Errorf("nextRun after exhaustion = %s, want it unchanged at %s", task.nextRun, beforeExhausted)
	}
}

func TestApplyRetryPolicy_SuccessResetsCounter(t *testing.T) {
	s := NewScheduler(nil)
	end := time.Date(2026, 1, 2, 3, 0, 0, 0, time.UTC)
	scheduled := end.Add(6 * time.Hour)
	task := &Task{Name: "recovered", nextRun: scheduled, retryCount: 2}

	s.applyRetryPolicy(task, end, nil)

	if task.retryCount != 0 {
		t.Errorf("retryCount = %d, want 0 after a successful run", task.retryCount)
	}
	if !task.nextRun.Equal(scheduled) {
		t.Errorf("nextRun = %s, want the scheduled time %s", task.nextRun, scheduled)
	}
}

func TestApplyRetryPolicy_NeverDelaysBeyondSchedule(t *testing.T) {
	s := NewScheduler(nil)
	end := time.Date(2026, 1, 2, 3, 0, 0, 0, time.UTC)
	// The next scheduled occurrence is sooner than the 5m retry delay.
	scheduled := end.Add(time.Minute)
	task := &Task{Name: "frequent", nextRun: scheduled}

	s.applyRetryPolicy(task, end, errors.New("failed"))

	if !task.nextRun.Equal(scheduled) {
		t.Errorf("nextRun = %s, want the earlier scheduled time %s", task.nextRun, scheduled)
	}
}

func TestLatestOccurrenceBefore(t *testing.T) {
	sched, err := parseSchedule("@every 15m")
	if err != nil {
		t.Fatalf("parseSchedule() error = %v", err)
	}

	now := time.Date(2026, 1, 2, 12, 0, 0, 0, time.UTC)
	got, ok := latestOccurrenceBefore(sched, now, time.Hour)
	if !ok {
		t.Fatal("latestOccurrenceBefore() ok = false, want an occurrence inside the window")
	}
	if got.After(now) {
		t.Errorf("occurrence %s is after now %s", got, now)
	}
	if now.Sub(got) > 15*time.Minute {
		t.Errorf("occurrence %s is not the most recent one before %s", got, now)
	}
}

func TestLatestOccurrenceBefore_NoneInWindow(t *testing.T) {
	// A yearly schedule has no occurrence inside a one-minute window.
	sched, err := parseSchedule("0 0 1 1 *")
	if err != nil {
		t.Fatalf("parseSchedule() error = %v", err)
	}

	now := time.Date(2026, 6, 15, 12, 0, 0, 0, time.UTC)
	if _, ok := latestOccurrenceBefore(sched, now, time.Minute); ok {
		t.Error("latestOccurrenceBefore() ok = true, want false with no occurrence in the window")
	}
}

func TestSetCatchUpWindow(t *testing.T) {
	s := NewScheduler(nil)
	if s.catchUpWindow != defaultCatchUpWindow {
		t.Errorf("default catchUpWindow = %s, want %s", s.catchUpWindow, defaultCatchUpWindow)
	}

	s.SetCatchUpWindow(2 * time.Hour)
	if s.catchUpWindow != 2*time.Hour {
		t.Errorf("catchUpWindow = %s, want 2h", s.catchUpWindow)
	}

	// A negative window is clamped to zero, which disables catch-up.
	s.SetCatchUpWindow(-time.Hour)
	if s.catchUpWindow != 0 {
		t.Errorf("catchUpWindow = %s, want 0 for a negative window", s.catchUpWindow)
	}
}

// runMissedTasks must not replay anything when catch-up is disabled, even for a
// task whose schedule fired while the process was down.
func TestRunMissedTasks_DisabledWindowIsNoOp(t *testing.T) {
	serverDB, _ := newSchedulerTestDBs(t)

	s := NewScheduler(serverDB)
	s.SetCatchUpWindow(0)

	ran := make(chan struct{}, 1)
	if err := s.AddTask("catchup-noop", "@every 1m", func() error {
		ran <- struct{}{}
		return nil
	}); err != nil {
		t.Fatalf("AddTask() error = %v", err)
	}

	s.runMissedTasks()
	s.inFlight.Wait()

	select {
	case <-ran:
		t.Error("task ran during catch-up, want no replay with the window disabled")
	default:
	}
}

// A task with no history row has never run on this database, so its first run
// belongs on its own schedule and not in the startup catch-up queue.
func TestRunMissedTasks_NoHistorySkipsReplay(t *testing.T) {
	serverDB, _ := newSchedulerTestDBs(t)

	s := NewScheduler(serverDB)
	s.SetCatchUpWindow(time.Hour)

	ran := make(chan struct{}, 1)
	if err := s.AddTask("catchup-first-run", "@every 1m", func() error {
		ran <- struct{}{}
		return nil
	}); err != nil {
		t.Fatalf("AddTask() error = %v", err)
	}

	s.runMissedTasks()
	s.inFlight.Wait()

	select {
	case <-ran:
		t.Error("task ran during catch-up, want no replay for a task with no history")
	default:
	}
}
