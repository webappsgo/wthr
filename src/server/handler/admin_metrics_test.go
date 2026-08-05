package handler

import (
	"testing"
)

// TestNewMetricsHandler verifies the constructor returns a non-nil,
// zero-value handler (MetricsHandler carries no fields/dependencies).
func TestNewMetricsHandler(t *testing.T) {
	h := NewMetricsHandler()
	if h == nil {
		t.Fatal("expected non-nil handler")
	}
}
