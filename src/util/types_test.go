package utils

import (
	"testing"
	"time"
)

// TestNow verifies Now() produces a string parseable as RFC3339 and close to
// the actual current time.
func TestNow(t *testing.T) {
	before := time.Now()
	got := Now()
	after := time.Now()

	parsed, err := time.Parse(time.RFC3339, got)
	if err != nil {
		t.Fatalf("Now() = %q, not valid RFC3339: %v", got, err)
	}

	if parsed.Before(before.Add(-time.Second)) || parsed.After(after.Add(time.Second)) {
		t.Errorf("Now() = %q (%v), want between %v and %v", got, parsed, before, after)
	}
}
