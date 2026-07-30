package service

import (
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/webappsgo/wthr/src/config"
)

// TestConfigWatcher_NewConfigWatcher_Success covers the happy path
// construction: a valid path and a non-nil reload callback produce a
// usable watcher with no error.
func TestConfigWatcher_NewConfigWatcher_Success(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "server.yml")
	if err := os.WriteFile(cfgPath, []byte("server:\n  port: 8080\n"), 0o644); err != nil {
		t.Fatalf("write temp config: %v", err)
	}

	cw, err := NewConfigWatcher(cfgPath, func(*config.AppConfig) error { return nil })
	if err != nil {
		t.Fatalf("NewConfigWatcher() unexpected error: %v", err)
	}
	if cw == nil {
		t.Fatal("NewConfigWatcher() returned nil watcher")
	}
	if cw.configPath != cfgPath {
		t.Errorf("configPath = %q, want %q", cw.configPath, cfgPath)
	}
	if err := cw.Stop(); err != nil {
		t.Errorf("Stop() unexpected error: %v", err)
	}
}

// TestConfigWatcher_NewConfigWatcher_NilReloadFunc covers the boundary of a
// nil reload callback: construction itself must not fail (the nil check, if
// any, only matters once a reload is actually triggered).
func TestConfigWatcher_NewConfigWatcher_NilReloadFunc(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "server.yml")

	cw, err := NewConfigWatcher(cfgPath, nil)
	if err != nil {
		t.Fatalf("NewConfigWatcher() unexpected error: %v", err)
	}
	if cw.reloadFunc != nil {
		t.Error("expected reloadFunc to remain nil")
	}
	if err := cw.Stop(); err != nil {
		t.Errorf("Stop() unexpected error: %v", err)
	}
}

// TestConfigWatcher_Start_ValidDir covers the happy path for Start(): the
// parent directory exists, so watcher.Add must succeed.
func TestConfigWatcher_Start_ValidDir(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "server.yml")
	if err := os.WriteFile(cfgPath, []byte("server:\n  port: 8080\n"), 0o644); err != nil {
		t.Fatalf("write temp config: %v", err)
	}

	cw, err := NewConfigWatcher(cfgPath, func(*config.AppConfig) error { return nil })
	if err != nil {
		t.Fatalf("NewConfigWatcher() unexpected error: %v", err)
	}

	if err := cw.Start(); err != nil {
		t.Fatalf("Start() unexpected error: %v", err)
	}
	if err := cw.Stop(); err != nil {
		t.Errorf("Stop() unexpected error: %v", err)
	}
}

// TestConfigWatcher_Start_MissingDir covers the error path: the parent
// directory of configPath does not exist, so fsnotify's watcher.Add must
// fail and Start() must propagate that error.
func TestConfigWatcher_Start_MissingDir(t *testing.T) {
	cfgPath := filepath.Join(t.TempDir(), "does-not-exist-subdir", "server.yml")

	cw, err := NewConfigWatcher(cfgPath, func(*config.AppConfig) error { return nil })
	if err != nil {
		t.Fatalf("NewConfigWatcher() unexpected error: %v", err)
	}
	defer cw.Stop()

	if err := cw.Start(); err == nil {
		t.Fatal("expected Start() to fail for a nonexistent config directory, got nil")
	}
}

// TestConfigWatcher_UnrelatedFileWrite_NoTrigger covers the filter logic in
// the watch loop: writes to OTHER files in the same directory must never
// invoke the reload callback, even after waiting past the debounce window.
func TestConfigWatcher_UnrelatedFileWrite_NoTrigger(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "server.yml")
	otherPath := filepath.Join(dir, "unrelated.txt")
	if err := os.WriteFile(cfgPath, []byte("server:\n  port: 8080\n"), 0o644); err != nil {
		t.Fatalf("write temp config: %v", err)
	}

	var calls int32
	cw, err := NewConfigWatcher(cfgPath, func(*config.AppConfig) error {
		atomic.AddInt32(&calls, 1)
		return nil
	})
	if err != nil {
		t.Fatalf("NewConfigWatcher() unexpected error: %v", err)
	}
	if err := cw.Start(); err != nil {
		t.Fatalf("Start() unexpected error: %v", err)
	}
	defer cw.Stop()

	if err := os.WriteFile(otherPath, []byte("noise"), 0o644); err != nil {
		t.Fatalf("write unrelated file: %v", err)
	}

	// Wait past the 500ms debounce window with margin.
	time.Sleep(800 * time.Millisecond)

	if got := atomic.LoadInt32(&calls); got != 0 {
		t.Errorf("reloadFunc called %d times for an unrelated file write, want 0", got)
	}
}

// TestConfigWatcher_ConfigFileWrite_DoesNotPanic exercises the full write
// -> debounce -> reload path by writing to the actual watched file. It does
// NOT assert on whether reloadFunc was ultimately invoked, because the
// production reload path calls the real config.LoadConfig() (no injectable
// seam - it does not accept the test's temp path), whose success depends on
// paths outside this test's control. This test instead asserts the watcher
// goroutine survives a real file-change event without panicking or
// deadlocking, and that Stop() still cleanly terminates it afterward.
func TestConfigWatcher_ConfigFileWrite_DoesNotPanic(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "server.yml")
	if err := os.WriteFile(cfgPath, []byte("server:\n  port: 8080\n"), 0o644); err != nil {
		t.Fatalf("write temp config: %v", err)
	}

	var mu sync.Mutex
	var lastCfg *config.AppConfig
	cw, err := NewConfigWatcher(cfgPath, func(c *config.AppConfig) error {
		mu.Lock()
		lastCfg = c
		mu.Unlock()
		return nil
	})
	if err != nil {
		t.Fatalf("NewConfigWatcher() unexpected error: %v", err)
	}
	if err := cw.Start(); err != nil {
		t.Fatalf("Start() unexpected error: %v", err)
	}

	if err := os.WriteFile(cfgPath, []byte("server:\n  port: 9090\n"), 0o644); err != nil {
		t.Fatalf("rewrite config file: %v", err)
	}

	// Wait past the debounce window with margin for the reload attempt to run.
	time.Sleep(800 * time.Millisecond)

	if err := cw.Stop(); err != nil {
		t.Errorf("Stop() unexpected error: %v", err)
	}

	// No assertion on lastCfg contents: whether the real config.LoadConfig()
	// succeeded on this host is environment-dependent. Reading it here only
	// under the mutex proves no data race occurred, satisfying -race.
	mu.Lock()
	_ = lastCfg
	mu.Unlock()
}
