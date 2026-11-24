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

func TestVersionAccessors(t *testing.T) {
	version, commitID, buildDate, buildEpoch, officialSite := Version, CommitID, BuildDate, BuildEpoch, OfficialSite
	Version, CommitID, BuildDate, BuildEpoch, OfficialSite = "1.2.3", "abc1234", "2026-01-02T03:04:05Z", "1767323045", "https://example.test"
	t.Cleanup(func() {
		Version, CommitID, BuildDate, BuildEpoch, OfficialSite = version, commitID, buildDate, buildEpoch, officialSite
	})

	if got := GetVersion(); got != Version {
		t.Fatalf("GetVersion() = %q, want %q", got, Version)
	}
	info := GetBuildInfo()
	want := map[string]string{
		"version": Version, "build_date": BuildDate, "build_epoch": BuildEpoch,
		"commit_id": CommitID, "official_site": OfficialSite,
	}
	for key, value := range want {
		if info[key] != value {
			t.Errorf("GetBuildInfo()[%q] = %q, want %q", key, info[key], value)
		}
	}
	if got, want := GetVersionString(), "v1.2.3 (built: 2026-01-02T03:04:05Z, commit: abc1234)"; got != want {
		t.Errorf("GetVersionString() = %q, want %q", got, want)
	}
}
