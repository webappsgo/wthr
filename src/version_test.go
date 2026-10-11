package main

import (
	"testing"
	"time"
)

// TestParseBuildEpoch covers the unset, valid and invalid link-time states
func TestParseBuildEpoch(t *testing.T) {
	tests := []struct {
		in   string
		want int64
	}{
		{"0", 0},
		{"", 0},
		{"abc", 0},
		{"1765112713", 1765112713},
	}
	for _, tt := range tests {
		if got := parseBuildEpoch(tt.in); got != tt.want {
			t.Errorf("parseBuildEpoch(%q) = %d, want %d", tt.in, got, tt.want)
		}
	}
}

// TestBuildDateDerivedFromEpoch verifies init() formats BuildDate as RFC 3339 UTC
func TestBuildDateDerivedFromEpoch(t *testing.T) {
	savedEpoch, savedDate := BuildEpoch, BuildDate
	defer func() { BuildEpoch, BuildDate = savedEpoch, savedDate }()

	BuildEpoch = "1765112713"
	BuildDate = "N/A"
	deriveBuildDate()
	want := time.Unix(1765112713, 0).UTC().Format(time.RFC3339)
	if BuildDate != want {
		t.Errorf("BuildDate = %q, want %q", BuildDate, want)
	}
}
