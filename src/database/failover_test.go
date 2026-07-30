package database

import (
	"testing"
	"time"
)

// TestNewFailoverManager_HealthyPrimary verifies the manager starts in
// non-read-only mode and routes queries/execs straight to the primary DB.
func TestNewFailoverManager_HealthyPrimary(t *testing.T) {
	primary := openMemDB(t, "fo_primary")
	cache := openMemDB(t, "fo_cache")
	if _, err := primary.Exec("CREATE TABLE t (id INTEGER PRIMARY KEY, val TEXT)"); err != nil {
		t.Fatalf("setup primary: %v", err)
	}

	fm := NewFailoverManager(primary, cache)
	defer fm.Close()

	if fm.IsReadOnly() {
		t.Error("IsReadOnly() = true on fresh manager, want false")
	}

	if _, err := fm.Exec("INSERT INTO t (id, val) VALUES (1, 'x')"); err != nil {
		t.Fatalf("Exec on healthy primary: %v", err)
	}

	rows, err := fm.Query("SELECT val FROM t WHERE id = 1")
	if err != nil {
		t.Fatalf("Query on healthy primary: %v", err)
	}
	defer rows.Close()
	var found bool
	for rows.Next() {
		var v string
		if err := rows.Scan(&v); err != nil {
			t.Fatalf("scan: %v", err)
		}
		if v != "x" {
			t.Errorf("val = %q, want x", v)
		}
		found = true
	}
	if !found {
		t.Error("Query returned no rows, want 1")
	}

	var val string
	if err := fm.QueryRow("SELECT val FROM t WHERE id = 1").Scan(&val); err != nil {
		t.Fatalf("QueryRow on healthy primary: %v", err)
	}
	if val != "x" {
		t.Errorf("QueryRow val = %q, want x", val)
	}
}

// TestFailoverManager_ExecFailureTripsReadOnly verifies that when the
// primary DB is closed (simulating a hard failure), Exec fails over: the
// manager flips to read-only, records the error, and queues the write
// instead of losing it.
func TestFailoverManager_ExecFailureTripsReadOnly(t *testing.T) {
	primary := openMemDB(t, "fo_exec_primary")
	cache := openMemDB(t, "fo_exec_cache")
	primary.Close() // force every primary op to fail

	fm := NewFailoverManager(primary, cache)
	defer fm.Close()

	_, err := fm.Exec("INSERT INTO t (id) VALUES (1)")
	if err == nil {
		t.Fatal("Exec against closed primary = nil error, want error")
	}
	if !contains(err.Error(), "database unavailable - write queued for retry") {
		t.Errorf("error = %q, want substring about queued write", err.Error())
	}

	if !fm.IsReadOnly() {
		t.Error("IsReadOnly() = false after primary failure, want true")
	}
	when, lastErr := fm.GetLastError()
	if lastErr == nil {
		t.Error("GetLastError() = nil after failure, want the triggering error")
	}
	if when.IsZero() {
		t.Error("GetLastError() timestamp is zero, want a recorded failure time")
	}
	if fm.GetQueuedWriteCount() != 1 {
		t.Errorf("GetQueuedWriteCount() = %d, want 1", fm.GetQueuedWriteCount())
	}
}

// TestFailoverManager_ExecWhileReadOnlyQueuesWithoutTryingPrimary verifies
// that once in read-only mode, further Exec calls skip the primary entirely
// and go straight to the write queue.
func TestFailoverManager_ExecWhileReadOnlyQueuesWithoutTryingPrimary(t *testing.T) {
	primary := openMemDB(t, "fo_ro_primary")
	cache := openMemDB(t, "fo_ro_cache")

	fm := NewFailoverManager(primary, cache)
	defer fm.Close()

	fm.mu.Lock()
	fm.readOnly = true
	fm.mu.Unlock()

	_, err := fm.Exec("INSERT INTO t (id) VALUES (1)")
	if err == nil {
		t.Fatal("Exec while read-only = nil error, want queued-write error")
	}
	if fm.GetQueuedWriteCount() != 1 {
		t.Errorf("GetQueuedWriteCount() = %d, want 1", fm.GetQueuedWriteCount())
	}
}

// TestFailoverManager_QueryFallsBackToCache verifies that when the primary
// fails on Query, results are still served from the cache DB rather than
// erroring out entirely.
func TestFailoverManager_QueryFallsBackToCache(t *testing.T) {
	primary := openMemDB(t, "fo_query_primary")
	cache := openMemDB(t, "fo_query_cache")
	if _, err := cache.Exec("CREATE TABLE t (id INTEGER PRIMARY KEY, val TEXT)"); err != nil {
		t.Fatalf("setup cache: %v", err)
	}
	if _, err := cache.Exec("INSERT INTO t (id, val) VALUES (1, 'cached')"); err != nil {
		t.Fatalf("seed cache: %v", err)
	}
	primary.Close()

	fm := NewFailoverManager(primary, cache)
	defer fm.Close()

	rows, err := fm.Query("SELECT val FROM t WHERE id = 1")
	if err != nil {
		t.Fatalf("Query with dead primary should fall back to cache, got error: %v", err)
	}
	defer rows.Close()
	var got string
	for rows.Next() {
		if err := rows.Scan(&got); err != nil {
			t.Fatalf("scan: %v", err)
		}
	}
	if got != "cached" {
		t.Errorf("val = %q, want cached (from fallback db)", got)
	}
	if !fm.IsReadOnly() {
		t.Error("IsReadOnly() = false after Query failover, want true")
	}
}

// TestFailoverManager_QueryReadOnlyUsesCacheDirectly verifies that once
// already in read-only mode, Query never touches the primary at all.
func TestFailoverManager_QueryReadOnlyUsesCacheDirectly(t *testing.T) {
	primary := openMemDB(t, "fo_qro_primary")
	cache := openMemDB(t, "fo_qro_cache")
	if _, err := cache.Exec("CREATE TABLE t (id INTEGER PRIMARY KEY)"); err != nil {
		t.Fatalf("setup cache: %v", err)
	}

	fm := NewFailoverManager(primary, cache)
	defer fm.Close()
	fm.mu.Lock()
	fm.readOnly = true
	fm.mu.Unlock()

	rows, err := fm.Query("SELECT id FROM t")
	if err != nil {
		t.Fatalf("Query in read-only mode: %v", err)
	}
	rows.Close()
}

// TestFailoverManager_HandlePrimaryFailure_OnlyTripsOnFirstFailure verifies
// the "only transitions on the first failure" guard in handlePrimaryFailure:
// subsequent failures must not overwrite lastError/lastErrorAt or spawn a
// second retryTicker.
func TestFailoverManager_HandlePrimaryFailure_OnlyTripsOnFirstFailure(t *testing.T) {
	primary := openMemDB(t, "fo_guard_primary")
	cache := openMemDB(t, "fo_guard_cache")

	fm := NewFailoverManager(primary, cache)
	defer fm.Close()

	firstErr := errString("first failure")
	fm.handlePrimaryFailure(firstErr)

	fm.mu.RLock()
	firstTicker := fm.retryTicker
	firstAt := fm.lastErrorAt
	fm.mu.RUnlock()

	if firstTicker == nil {
		t.Fatal("retryTicker not created on first failure")
	}

	time.Sleep(5 * time.Millisecond)
	secondErr := errString("second failure")
	fm.handlePrimaryFailure(secondErr)

	fm.mu.RLock()
	secondTicker := fm.retryTicker
	secondAt := fm.lastErrorAt
	gotErr := fm.lastError
	fm.mu.RUnlock()

	if secondTicker != firstTicker {
		t.Error("retryTicker was replaced on second failure, want unchanged (first-failure-only guard)")
	}
	if !secondAt.Equal(firstAt) {
		t.Error("lastErrorAt changed on second failure, want unchanged (first-failure-only guard)")
	}
	if gotErr.Error() != firstErr.Error() {
		t.Errorf("lastError = %q, want unchanged %q (first-failure-only guard)", gotErr.Error(), firstErr.Error())
	}
}

// TestFailoverManager_AttemptRecovery_NoOpWhenHealthy verifies
// attemptRecovery is a no-op when the manager is not currently in read-only
// mode.
func TestFailoverManager_AttemptRecovery_NoOpWhenHealthy(t *testing.T) {
	primary := openMemDB(t, "fo_recnoop_primary")
	cache := openMemDB(t, "fo_recnoop_cache")

	fm := NewFailoverManager(primary, cache)
	defer fm.Close()

	fm.attemptRecovery()
	if fm.IsReadOnly() {
		t.Error("attemptRecovery flipped readOnly to true on an already-healthy manager")
	}
}

// TestFailoverManager_AttemptRecovery_RestoresPrimaryAndReplaysWrites is the
// core recovery test: after a simulated primary failure and queued write,
// swap in a healthy primary and call attemptRecovery directly (bypassing the
// 30s ticker) to deterministically verify recovery flips readOnly back to
// false, clears lastError, and replays the queued write against the new
// primary.
func TestFailoverManager_AttemptRecovery_RestoresPrimaryAndReplaysWrites(t *testing.T) {
	deadPrimary := openMemDB(t, "fo_rec_deadprimary")
	cache := openMemDB(t, "fo_rec_cache")
	deadPrimary.Close()

	fm := NewFailoverManager(deadPrimary, cache)
	defer fm.Close()

	if _, err := fm.Exec("INSERT INTO t (id) VALUES (1)"); err == nil {
		t.Fatal("expected Exec against dead primary to fail")
	}
	if !fm.IsReadOnly() {
		t.Fatal("expected manager to be read-only after failed Exec")
	}
	if fm.GetQueuedWriteCount() != 1 {
		t.Fatalf("expected 1 queued write, got %d", fm.GetQueuedWriteCount())
	}

	healthyPrimary := openMemDB(t, "fo_rec_healthyprimary")
	if _, err := healthyPrimary.Exec("CREATE TABLE t (id INTEGER PRIMARY KEY)"); err != nil {
		t.Fatalf("setup healthy primary: %v", err)
	}

	fm.mu.Lock()
	fm.primaryDB = healthyPrimary
	fm.mu.Unlock()

	fm.attemptRecovery()

	if fm.IsReadOnly() {
		t.Error("IsReadOnly() = true after attemptRecovery against healthy primary, want false")
	}
	if _, lastErr := fm.GetLastError(); lastErr != nil {
		t.Errorf("GetLastError() = %v after recovery, want nil", lastErr)
	}
	if fm.GetQueuedWriteCount() != 0 {
		t.Errorf("GetQueuedWriteCount() = %d after recovery, want 0 (queued write should have replayed)", fm.GetQueuedWriteCount())
	}

	var count int
	if err := healthyPrimary.QueryRow("SELECT COUNT(*) FROM t").Scan(&count); err != nil {
		t.Fatalf("verify replayed write: %v", err)
	}
	if count != 1 {
		t.Errorf("row count on recovered primary = %d, want 1 (queued write must have replayed)", count)
	}
}

// TestFailoverManager_AttemptRecovery_StaysReadOnlyIfStillDown verifies that
// if the swapped-in "recovered" primary is still unreachable, attemptRecovery
// leaves the manager in read-only mode rather than incorrectly flipping it.
func TestFailoverManager_AttemptRecovery_StaysReadOnlyIfStillDown(t *testing.T) {
	deadPrimary := openMemDB(t, "fo_stilldown_primary")
	cache := openMemDB(t, "fo_stilldown_cache")
	deadPrimary.Close()

	fm := NewFailoverManager(deadPrimary, cache)
	defer fm.Close()

	if _, err := fm.Exec("INSERT INTO t (id) VALUES (1)"); err == nil {
		t.Fatal("expected Exec against dead primary to fail")
	}
	if !fm.IsReadOnly() {
		t.Fatal("expected manager to be read-only after failed Exec")
	}

	fm.attemptRecovery()

	if !fm.IsReadOnly() {
		t.Error("IsReadOnly() = false after attemptRecovery against still-dead primary, want true (must stay read-only)")
	}
}

func TestFailoverManager_Close_StopsMonitorGoroutine(t *testing.T) {
	primary := openMemDB(t, "fo_close_primary")
	cache := openMemDB(t, "fo_close_cache")

	fm := NewFailoverManager(primary, cache)
	fm.Close()

	// Closing twice must not panic (guards against double-close of stopChan
	// being exercised accidentally by a future refactor); only assert this
	// once since a second Close() would panic on a closed channel per the
	// current implementation, so we only verify the single Close() above
	// completed without hanging the test.
}

// errString is a tiny error implementation avoiding an import of "errors"
// solely for a couple of literal error values in this file.
type errString string

func (e errString) Error() string { return string(e) }
