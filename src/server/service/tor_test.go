package service

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/cretz/bine/tor"
)

// TestTor_NewTorService covers the constructor: directory creation, initial
// field state, and that it tolerates a nil DB (it only stores the pointer,
// never dereferences it during construction).
func TestTor_NewTorService(t *testing.T) {
	t.Run("creates data dir and default state", func(t *testing.T) {
		base := t.TempDir()
		ts := NewTorService(nil, base)
		if ts == nil {
			t.Fatal("NewTorService returned nil")
		}

		wantDir := filepath.Join(base, "tor")
		if ts.dataDir != wantDir {
			t.Errorf("dataDir = %q, want %q", ts.dataDir, wantDir)
		}
		if info, err := os.Stat(wantDir); err != nil || !info.IsDir() {
			t.Errorf("expected tor data dir to exist at %q: %v", wantDir, err)
		}
		if ts.isRunning {
			t.Error("isRunning should default to false")
		}
		if ts.healthStatus != "not_started" {
			t.Errorf("healthStatus = %q, want %q", ts.healthStatus, "not_started")
		}
		if !ts.monitorEnabled {
			t.Error("monitorEnabled should default to true")
		}
		if ts.monitorStop == nil {
			t.Error("monitorStop channel should be initialized")
		}
		if ts.ctx == nil || ts.cancel == nil {
			t.Error("ctx/cancel should be initialized")
		}
	})

	t.Run("nested data dir is created", func(t *testing.T) {
		base := filepath.Join(t.TempDir(), "a", "b", "c")
		ts := NewTorService(nil, base)
		if _, err := os.Stat(ts.dataDir); err != nil {
			t.Errorf("expected nested tor dir to be created: %v", err)
		}
	})
}

// TestTor_GetOnionAddress covers the plain getter, including the empty
// (never-started) boundary case.
func TestTor_GetOnionAddress(t *testing.T) {
	tests := []struct {
		name string
		addr string
	}{
		{"empty when never started", ""},
		{"returns stored address", "abcdefghijklmnop.onion"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ts := &TorService{onionAddress: tt.addr}
			if got := ts.GetOnionAddress(); got != tt.addr {
				t.Errorf("GetOnionAddress() = %q, want %q", got, tt.addr)
			}
		})
	}
}

// TestTor_IsRunning covers both boolean states of the running flag.
func TestTor_IsRunning(t *testing.T) {
	tests := []struct {
		name      string
		isRunning bool
	}{
		{"not running", false},
		{"running", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ts := &TorService{isRunning: tt.isRunning}
			if got := ts.IsRunning(); got != tt.isRunning {
				t.Errorf("IsRunning() = %v, want %v", got, tt.isRunning)
			}
		})
	}
}

// TestTor_GetStatus covers the disconnected and connected branches of the
// status map, including the fields that only appear when running.
func TestTor_GetStatus(t *testing.T) {
	t.Run("disconnected", func(t *testing.T) {
		ts := &TorService{
			isRunning:    false,
			healthStatus: "not_started",
			dataDir:      "/tmp/x",
		}
		status := ts.GetStatus()
		if status["enabled"] != false {
			t.Errorf("enabled = %v, want false", status["enabled"])
		}
		if status["status"] != "disconnected" {
			t.Errorf("status = %v, want disconnected", status["status"])
		}
		if _, ok := status["uptime"]; ok {
			t.Error("uptime should not be present when not running")
		}
	})

	t.Run("connected", func(t *testing.T) {
		start := time.Now().Add(-5 * time.Minute)
		ts := &TorService{
			isRunning:       true,
			onionAddress:    "abc.onion",
			dataDir:         "/tmp/x",
			healthStatus:    "healthy",
			restartCount:    2,
			startTime:       start,
			lastHealthCheck: time.Now(),
		}
		status := ts.GetStatus()
		if status["status"] != "connected" {
			t.Errorf("status = %v, want connected", status["status"])
		}
		if status["onion_address"] != "abc.onion" {
			t.Errorf("onion_address = %v, want abc.onion", status["onion_address"])
		}
		if status["restart_count"] != 2 {
			t.Errorf("restart_count = %v, want 2", status["restart_count"])
		}
		uptime, ok := status["uptime"].(string)
		if !ok || uptime == "" {
			t.Errorf("uptime = %v, want non-empty string", status["uptime"])
		}
		if _, ok := status["last_health_check"].(string); !ok {
			t.Error("last_health_check should be a formatted string when running")
		}
	})
}

// TestTor_GetHealthStatus covers the health status snapshot map.
func TestTor_GetHealthStatus(t *testing.T) {
	ts := &TorService{
		healthStatus:   "healthy",
		isRunning:      true,
		restartCount:   3,
		monitorEnabled: true,
		startTime:      time.Now().Add(-1 * time.Minute),
	}
	got := ts.GetHealthStatus()
	if got["status"] != "healthy" {
		t.Errorf("status = %v, want healthy", got["status"])
	}
	if got["is_running"] != true {
		t.Errorf("is_running = %v, want true", got["is_running"])
	}
	if got["restart_count"] != 3 {
		t.Errorf("restart_count = %v, want 3", got["restart_count"])
	}
	if got["monitoring_active"] != true {
		t.Errorf("monitoring_active = %v, want true", got["monitoring_active"])
	}
	if uptime, ok := got["uptime"].(string); !ok || uptime == "" {
		t.Errorf("uptime = %v, want non-empty string", got["uptime"])
	}
}

// TestTor_Stop_NilTor covers the early-return path of Stop() when the
// service was never started, so it must not touch the monitor channel or
// panic on a nil onionService/tor.
func TestTor_Stop_NilTor(t *testing.T) {
	ts := &TorService{
		monitorEnabled: true,
		monitorStop:    make(chan struct{}),
		isRunning:      false,
	}
	if err := ts.Stop(); err != nil {
		t.Fatalf("Stop() with nil tor = %v, want nil", err)
	}
	// The monitor channel must remain open (Stop returned before closing it).
	select {
	case <-ts.monitorStop:
		t.Error("monitorStop was closed even though Stop() short-circuited")
	default:
	}

	// Idempotent: calling Stop() again is still a no-op, no panic.
	if err := ts.Stop(); err != nil {
		t.Fatalf("second Stop() call = %v, want nil", err)
	}
}

// TestTor_PerformHealthCheck covers the three branches of the health check:
// not running (no-op), running with nil dependencies (unhealthy), and
// running with live dependencies (healthy, timestamp refreshed).
func TestTor_PerformHealthCheck(t *testing.T) {
	tests := []struct {
		name             string
		isRunning        bool
		onionService     *tor.OnionService
		torInstance      *tor.Tor
		initialStatus    string
		wantStatus       string
		wantStatusChange bool
	}{
		{
			name:          "not running leaves status untouched",
			isRunning:     false,
			initialStatus: "healthy",
			wantStatus:    "healthy",
		},
		{
			name:          "running with nil deps is unhealthy",
			isRunning:     true,
			onionService:  nil,
			torInstance:   nil,
			initialStatus: "healthy",
			wantStatus:    "unhealthy",
		},
		{
			name:             "running with live deps is healthy",
			isRunning:        true,
			onionService:     &tor.OnionService{},
			torInstance:      &tor.Tor{},
			initialStatus:    "unhealthy",
			wantStatus:       "healthy",
			wantStatusChange: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ts := &TorService{
				isRunning:    tt.isRunning,
				onionService: tt.onionService,
				tor:          tt.torInstance,
				healthStatus: tt.initialStatus,
			}
			before := ts.lastHealthCheck
			ts.performHealthCheck()
			if ts.healthStatus != tt.wantStatus {
				t.Errorf("healthStatus = %q, want %q", ts.healthStatus, tt.wantStatus)
			}
			if tt.wantStatusChange && !ts.lastHealthCheck.After(before) {
				t.Error("lastHealthCheck should have been refreshed")
			}
		})
	}
}

// TestTor_EnableDisableMonitoring covers toggling monitoring, including
// idempotency (repeated enable/disable calls must not panic on a
// double-close of the stop channel or spawn duplicate goroutines).
func TestTor_EnableDisableMonitoring(t *testing.T) {
	ts := NewTorService(nil, t.TempDir())

	// Constructor already enables monitoring; disable it first.
	ts.DisableMonitoring()
	if ts.monitorEnabled {
		t.Fatal("expected monitoring disabled")
	}

	// Idempotent: disabling again must not panic (no double-close).
	ts.DisableMonitoring()
	if ts.monitorEnabled {
		t.Fatal("expected monitoring to remain disabled")
	}

	ts.EnableMonitoring()
	if !ts.monitorEnabled {
		t.Fatal("expected monitoring enabled")
	}

	// Idempotent: enabling again must not panic or replace the channel improperly.
	ch := ts.monitorStop
	ts.EnableMonitoring()
	if ts.monitorStop != ch {
		t.Error("re-enabling an already-enabled monitor should not replace the channel")
	}

	// Clean up: disable so the test does not leak state (no goroutine was
	// started since isRunning is false).
	ts.DisableMonitoring()
}

// TestTor_MonitorHealth_StopsOnSignal covers the monitor loop's shutdown
// path: closing monitorStop must cause monitorHealth to return promptly
// rather than waiting on its 30s ticker.
func TestTor_MonitorHealth_StopsOnSignal(t *testing.T) {
	ts := &TorService{
		monitorStop:  make(chan struct{}),
		healthStatus: "healthy",
	}
	close(ts.monitorStop)

	done := make(chan struct{})
	go func() {
		ts.monitorHealth()
		close(done)
	}()

	select {
	case <-done:
		// success
	case <-time.After(2 * time.Second):
		t.Fatal("monitorHealth did not return after monitorStop was closed")
	}
}
