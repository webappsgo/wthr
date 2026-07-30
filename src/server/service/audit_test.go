package service

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// readAuditLines reads and returns the raw lines of an audit log file.
func readAuditLines(t *testing.T, path string) []string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read audit log: %v", err)
	}
	trimmed := strings.TrimSpace(string(data))
	if trimmed == "" {
		return nil
	}
	return strings.Split(trimmed, "\n")
}

// TestAudit_NewAuditLogger covers happy path creation and the error path when
// the log directory cannot be created (a file exists where a directory is expected).
func TestAudit_NewAuditLogger(t *testing.T) {
	t.Run("creates log directory and file", func(t *testing.T) {
		dir := filepath.Join(t.TempDir(), "audit")
		logger, err := NewAuditLogger(dir)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		defer logger.Close()

		if logger.logFile != filepath.Join(dir, "audit.log") {
			t.Errorf("unexpected logFile path: %s", logger.logFile)
		}
		if _, err := os.Stat(logger.logFile); err != nil {
			t.Errorf("expected audit.log to exist: %v", err)
		}
	})

	t.Run("errors when log dir path is blocked by a file", func(t *testing.T) {
		base := t.TempDir()
		blocked := filepath.Join(base, "blocker")
		if err := os.WriteFile(blocked, []byte("x"), 0644); err != nil {
			t.Fatalf("failed to seed blocking file: %v", err)
		}

		// Attempt to create the log dir as a subdirectory of a file (not a directory).
		dir := filepath.Join(blocked, "audit")
		if _, err := NewAuditLogger(dir); err == nil {
			t.Error("expected error when log directory cannot be created, got nil")
		}
	})
}

// TestAudit_Log covers default field population, preservation of caller-supplied
// values, and the marshal error path.
func TestAudit_Log(t *testing.T) {
	t.Run("sets defaults for zero-value fields", func(t *testing.T) {
		logger, err := NewAuditLogger(t.TempDir())
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		defer logger.Close()

		event := AuditEvent{
			Event:    "test.event",
			Category: "test",
			Result:   "success",
		}
		if err := logger.Log(event); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		lines := readAuditLines(t, logger.logFile)
		if len(lines) != 1 {
			t.Fatalf("expected 1 line, got %d", len(lines))
		}

		var got AuditEvent
		if err := json.Unmarshal([]byte(lines[0]), &got); err != nil {
			t.Fatalf("failed to unmarshal logged event: %v", err)
		}
		if got.ID == "" {
			t.Error("expected ID to be generated")
		}
		if got.Time.IsZero() {
			t.Error("expected Time to be set")
		}
		if got.Severity != "info" {
			t.Errorf("expected default severity 'info' for success result, got %q", got.Severity)
		}
	})

	t.Run("sets warn severity by default on failure result", func(t *testing.T) {
		logger, err := NewAuditLogger(t.TempDir())
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		defer logger.Close()

		if err := logger.Log(AuditEvent{Event: "test.fail", Result: "failure"}); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		lines := readAuditLines(t, logger.logFile)
		var got AuditEvent
		if err := json.Unmarshal([]byte(lines[0]), &got); err != nil {
			t.Fatalf("failed to unmarshal: %v", err)
		}
		if got.Severity != "warn" {
			t.Errorf("expected severity 'warn', got %q", got.Severity)
		}
	})

	t.Run("preserves caller-supplied ID, Time and Severity", func(t *testing.T) {
		logger, err := NewAuditLogger(t.TempDir())
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		defer logger.Close()

		fixedTime := time.Date(2020, 1, 2, 3, 4, 5, 0, time.UTC)
		event := AuditEvent{
			ID:       "custom-id",
			Time:     fixedTime,
			Event:    "test.custom",
			Severity: "critical",
			Result:   "success",
		}
		if err := logger.Log(event); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		lines := readAuditLines(t, logger.logFile)
		var got AuditEvent
		if err := json.Unmarshal([]byte(lines[0]), &got); err != nil {
			t.Fatalf("failed to unmarshal: %v", err)
		}
		if got.ID != "custom-id" {
			t.Errorf("expected ID to be preserved, got %q", got.ID)
		}
		if !got.Time.Equal(fixedTime) {
			t.Errorf("expected Time to be preserved, got %v", got.Time)
		}
		if got.Severity != "critical" {
			t.Errorf("expected Severity to be preserved, got %q", got.Severity)
		}
	})

	t.Run("returns error when event cannot be marshaled", func(t *testing.T) {
		logger, err := NewAuditLogger(t.TempDir())
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		defer logger.Close()

		// A channel value is not JSON-marshalable, forcing json.Marshal to fail.
		event := AuditEvent{
			Event:   "test.badmarshal",
			Details: map[string]interface{}{"bad": make(chan int)},
		}
		if err := logger.Log(event); err == nil {
			t.Error("expected marshal error, got nil")
		}

		lines := readAuditLines(t, logger.logFile)
		if len(lines) != 0 {
			t.Errorf("expected no lines written on marshal failure, got %d", len(lines))
		}
	})

	t.Run("writing after Close returns an error", func(t *testing.T) {
		logger, err := NewAuditLogger(t.TempDir())
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if err := logger.Close(); err != nil {
			t.Fatalf("unexpected error closing: %v", err)
		}

		if err := logger.Log(AuditEvent{Event: "test.afterclose"}); err == nil {
			t.Error("expected error writing to closed audit log, got nil")
		}
	})

	t.Run("multiple sequential logs each get a distinct ID (idempotency of ID generation)", func(t *testing.T) {
		logger, err := NewAuditLogger(t.TempDir())
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		defer logger.Close()

		for i := 0; i < 2; i++ {
			if err := logger.Log(AuditEvent{Event: "test.dup", Result: "success"}); err != nil {
				t.Fatalf("unexpected error on iteration %d: %v", i, err)
			}
		}

		lines := readAuditLines(t, logger.logFile)
		if len(lines) != 2 {
			t.Fatalf("expected 2 lines, got %d", len(lines))
		}

		var first, second AuditEvent
		if err := json.Unmarshal([]byte(lines[0]), &first); err != nil {
			t.Fatalf("failed to unmarshal first: %v", err)
		}
		if err := json.Unmarshal([]byte(lines[1]), &second); err != nil {
			t.Fatalf("failed to unmarshal second: %v", err)
		}
		if first.ID == second.ID {
			t.Errorf("expected distinct IDs for two log calls, both were %q", first.ID)
		}
	})

	t.Run("concurrent logging does not corrupt the file", func(t *testing.T) {
		logger, err := NewAuditLogger(t.TempDir())
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		defer logger.Close()

		const n = 50
		var wg sync.WaitGroup
		wg.Add(n)
		for i := 0; i < n; i++ {
			go func(i int) {
				defer wg.Done()
				_ = logger.Log(AuditEvent{Event: "test.concurrent", Result: "success"})
			}(i)
		}
		wg.Wait()

		lines := readAuditLines(t, logger.logFile)
		if len(lines) != n {
			t.Fatalf("expected %d lines, got %d", n, len(lines))
		}
		for i, l := range lines {
			var evt AuditEvent
			if err := json.Unmarshal([]byte(l), &evt); err != nil {
				t.Errorf("line %d is not valid JSON (possible interleaved write): %v", i, err)
			}
		}
	})
}

// TestAudit_LogSuccess verifies the convenience wrapper populates Actor,
// Result and Severity correctly.
func TestAudit_LogSuccess(t *testing.T) {
	logger, err := NewAuditLogger(t.TempDir())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer logger.Close()

	details := map[string]interface{}{"key": "value"}
	if err := logger.LogSuccess("user.login", "authentication", "user", "42", "1.2.3.4", details); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	lines := readAuditLines(t, logger.logFile)
	var got AuditEvent
	if err := json.Unmarshal([]byte(lines[0]), &got); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}
	if got.Result != "success" {
		t.Errorf("expected result 'success', got %q", got.Result)
	}
	if got.Actor.Type != "user" || got.Actor.ID != "42" || got.Actor.IP != "1.2.3.4" {
		t.Errorf("unexpected actor: %+v", got.Actor)
	}
	if got.Details["key"] != "value" {
		t.Errorf("expected details to be preserved, got %+v", got.Details)
	}
}

// TestAudit_LogFailure verifies error message injection into Details,
// including the boundary case of a nil details map.
func TestAudit_LogFailure(t *testing.T) {
	t.Run("nil details map is initialized and given the error", func(t *testing.T) {
		logger, err := NewAuditLogger(t.TempDir())
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		defer logger.Close()

		if err := logger.LogFailure("user.login.failed", "authentication", "user", "42", "1.2.3.4", "bad password", nil); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		lines := readAuditLines(t, logger.logFile)
		var got AuditEvent
		if err := json.Unmarshal([]byte(lines[0]), &got); err != nil {
			t.Fatalf("failed to unmarshal: %v", err)
		}
		if got.Result != "failure" {
			t.Errorf("expected result 'failure', got %q", got.Result)
		}
		if got.Details["error"] != "bad password" {
			t.Errorf("expected details.error to be set, got %+v", got.Details)
		}
	})

	t.Run("existing details map keeps its keys plus error", func(t *testing.T) {
		logger, err := NewAuditLogger(t.TempDir())
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		defer logger.Close()

		details := map[string]interface{}{"attempt": 3}
		if err := logger.LogFailure("user.login.failed", "authentication", "user", "42", "1.2.3.4", "locked out", details); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		lines := readAuditLines(t, logger.logFile)
		var got AuditEvent
		if err := json.Unmarshal([]byte(lines[0]), &got); err != nil {
			t.Fatalf("failed to unmarshal: %v", err)
		}
		if got.Details["error"] != "locked out" {
			t.Errorf("expected details.error to be set, got %+v", got.Details)
		}
		if got.Details["attempt"] != float64(3) {
			t.Errorf("expected original detail key preserved, got %+v", got.Details)
		}
	})
}

// TestAudit_Close verifies Close is idempotent-safe to call and closes the file.
func TestAudit_Close(t *testing.T) {
	logger, err := NewAuditLogger(t.TempDir())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if err := logger.Close(); err != nil {
		t.Fatalf("unexpected error on first close: %v", err)
	}

	// Second close on an already-closed *os.File returns an error from the OS,
	// but must not panic - this documents current behavior.
	_ = logger.Close()
}

// TestAudit_Rotate verifies rotation archives the current file with a
// timestamp suffix and continues logging to a fresh file afterward.
func TestAudit_Rotate(t *testing.T) {
	dir := t.TempDir()
	logger, err := NewAuditLogger(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer logger.Close()

	if err := logger.Log(AuditEvent{Event: "before.rotate", Result: "success"}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if err := logger.Rotate(); err != nil {
		t.Fatalf("unexpected error rotating: %v", err)
	}

	// The original log content should now live in an archive file.
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("failed to read dir: %v", err)
	}
	var archiveFound, freshFound bool
	for _, e := range entries {
		if e.Name() == "audit.log" {
			freshFound = true
		} else if strings.HasPrefix(e.Name(), "audit.log.") {
			archiveFound = true
		}
	}
	if !archiveFound {
		t.Error("expected an archived audit.log.<timestamp> file after rotate")
	}
	if !freshFound {
		t.Error("expected a fresh audit.log file after rotate")
	}

	// Logging after rotate should succeed and land in the new file.
	if err := logger.Log(AuditEvent{Event: "after.rotate", Result: "success"}); err != nil {
		t.Fatalf("unexpected error logging after rotate: %v", err)
	}
	lines := readAuditLines(t, logger.logFile)
	if len(lines) != 1 {
		t.Fatalf("expected 1 line in fresh log file, got %d", len(lines))
	}
	var got AuditEvent
	if err := json.Unmarshal([]byte(lines[0]), &got); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}
	if got.Event != "after.rotate" {
		t.Errorf("expected 'after.rotate' event in fresh log, got %q", got.Event)
	}
}

// TestAudit_Rotate_MissingFile covers the error path when the underlying
// log file has been removed out from under the logger before rotation.
func TestAudit_Rotate_MissingFile(t *testing.T) {
	dir := t.TempDir()
	logger, err := NewAuditLogger(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Close and remove the file so the rename in Rotate fails.
	logger.Close()
	if err := os.Remove(logger.logFile); err != nil {
		t.Fatalf("failed to remove log file: %v", err)
	}

	if err := logger.Rotate(); err == nil {
		t.Error("expected error rotating a missing log file, got nil")
	}
}

// TestAudit_GetEventTypes sanity-checks the static event catalog: it should be
// non-empty, contain no duplicates, and include a few representative entries
// from each documented category.
func TestAudit_GetEventTypes(t *testing.T) {
	types := GetEventTypes()

	if len(types) < 100 {
		t.Errorf("expected 100+ event types per spec, got %d", len(types))
	}

	seen := make(map[EventType]bool, len(types))
	for _, et := range types {
		if seen[et] {
			t.Errorf("duplicate event type in catalog: %s", et)
		}
		seen[et] = true
		if et == "" {
			t.Error("found empty event type in catalog")
		}
	}

	mustContain := []EventType{
		EventUserLogin,
		EventAdminUserDelete,
		EventSystemStartup,
		EventAPIWeatherFetch,
		EventSecurityBruteForce,
	}
	for _, want := range mustContain {
		if !seen[want] {
			t.Errorf("expected catalog to contain %s", want)
		}
	}
}
