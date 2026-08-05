package handler

import (
	"testing"
)

// TestNewLoggingHandler verifies the constructor wires the logsDir field
// as passed.
func TestNewLoggingHandler(t *testing.T) {
	h := NewLoggingHandler("/tmp/example/logs")
	if h == nil {
		t.Fatal("expected non-nil handler")
	}
	if h.logsDir != "/tmp/example/logs" {
		t.Errorf("logsDir = %q, want %q", h.logsDir, "/tmp/example/logs")
	}
}
