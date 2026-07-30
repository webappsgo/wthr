// Tests for status.go per AI.md PART 6 / PART 29 (Testing).
package cli

import (
	"strings"
	"testing"
	"time"
)

// TestFormatDuration covers the days/hours/minutes/seconds boundaries: each
// branch of formatDuration is chosen by the first non-zero unit, so we
// assert on that boundary explicitly.
func TestFormatDuration(t *testing.T) {
	tests := []struct {
		name string
		d    time.Duration
		want string
	}{
		{"zero", 0, "0s"},
		{"seconds_only", 45 * time.Second, "45s"},
		{"minutes_only", 5 * time.Minute, "5m"},
		{"minutes_and_seconds_drops_seconds", 5*time.Minute + 30*time.Second, "5m"},
		{"hours_only", 3 * time.Hour, "3h 0m"},
		{"hours_and_minutes", 3*time.Hour + 15*time.Minute, "3h 15m"},
		{"days_only", 48 * time.Hour, "2d 0h 0m"},
		{"days_hours_minutes", 53*time.Hour + 30*time.Minute, "2d 5h 30m"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := formatDuration(tt.d); got != tt.want {
				t.Errorf("formatDuration(%v) = %q, want %q", tt.d, got, tt.want)
			}
		})
	}
}

// TestStatusCommand_Execute_Running covers a fully populated "running"
// status, including cluster and Tor sections both enabled.
func TestStatusCommand_Execute_Running(t *testing.T) {
	s := &StatusCommand{
		ServerRunning: true,
		Port:          8080,
		Mode:          "production",
		StartTime:     time.Now().Add(-90 * time.Minute),
		NodeID:        "node-1",
		ClusterMode:   true,
		ClusterStatus: "healthy",
		ClusterNodes:  3,
		DatabaseInfo:  "sqlite",
		TorEnabled:    true,
		TorConnected:  true,
		TorAddress:    "abc123.onion",
	}

	out := captureStdout(t, func() {
		if err := s.Execute(); err != nil {
			t.Fatalf("Execute() error = %v", err)
		}
	})

	for _, want := range []string{
		"Status:   Running", "Port:     8080", "Mode:     production",
		"Node:     node-1", "Mode:     cluster", "Status:   healthy",
		"Nodes:    3", "Database: sqlite", "Status:   Connected",
		"Address:  abc123.onion",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("Execute() output missing %q; got %q", want, out)
		}
	}
}

// TestStatusCommand_Execute_Stopped covers the minimal/default state:
// server not running, standalone node, cluster disabled, Tor disabled.
func TestStatusCommand_Execute_Stopped(t *testing.T) {
	s := &StatusCommand{}

	out := captureStdout(t, func() {
		if err := s.Execute(); err != nil {
			t.Fatalf("Execute() error = %v", err)
		}
	})

	for _, want := range []string{
		"Status:   Stopped", "Node:     standalone", "Mode:     disabled",
		"Status:   Disabled",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("Execute() output missing %q; got %q", want, out)
		}
	}
	if strings.Contains(out, "Uptime:") {
		t.Error("Execute() output contains Uptime for a server with zero StartTime")
	}
}

// TestStatusCommand_Execute_TorDisconnected covers the Tor-enabled-but-not-
// connected branch, distinct from both fully enabled and fully disabled.
func TestStatusCommand_Execute_TorDisconnected(t *testing.T) {
	s := &StatusCommand{TorEnabled: true, TorConnected: false}

	out := captureStdout(t, func() {
		if err := s.Execute(); err != nil {
			t.Fatalf("Execute() error = %v", err)
		}
	})

	if !strings.Contains(out, "Status:   Disconnected") {
		t.Errorf("Execute() output missing %q; got %q", "Status:   Disconnected", out)
	}
}

// TestShowStatusNotRunning covers the standalone helper used when the
// server process itself cannot be queried.
func TestShowStatusNotRunning(t *testing.T) {
	out := captureStdout(t, ShowStatusNotRunning)

	for _, want := range []string{
		"Server Status:  Stopped", "Node:           standalone",
		"Cluster:        disabled", "wthr --service start",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("ShowStatusNotRunning() output missing %q; got %q", want, out)
		}
	}
}
