package main

import "testing"

// TestGetVersion covers the trivial passthrough of the package-level Version var.
func TestGetVersion(t *testing.T) {
	orig := Version
	defer func() { Version = orig }()

	tests := []struct {
		name    string
		version string
	}{
		{"default dev value", "dev"},
		{"empty string", ""},
		{"semver value", "1.2.3"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			Version = tt.version
			if got := GetVersion(); got != tt.version {
				t.Errorf("GetVersion() = %q, want %q", got, tt.version)
			}
		})
	}
}

// TestGetBuildInfo verifies all four build-info fields are surfaced with the
// exact keys consumers rely on (version/build_date/commit_id/official_site).
func TestGetBuildInfo(t *testing.T) {
	origVersion, origBuildDate, origCommitID, origSite := Version, BuildDate, CommitID, OfficialSite
	defer func() {
		Version, BuildDate, CommitID, OfficialSite = origVersion, origBuildDate, origCommitID, origSite
	}()

	Version = "9.9.9"
	BuildDate = "2026-01-01T00:00:00Z"
	CommitID = "deadbeef"
	OfficialSite = "https://example.test"

	info := GetBuildInfo()

	want := map[string]string{
		"version":       "9.9.9",
		"build_date":    "2026-01-01T00:00:00Z",
		"commit_id":     "deadbeef",
		"official_site": "https://example.test",
	}

	if len(info) != len(want) {
		t.Fatalf("GetBuildInfo() returned %d keys, want %d: %#v", len(info), len(want), info)
	}
	for k, v := range want {
		if got, ok := info[k]; !ok {
			t.Errorf("GetBuildInfo() missing key %q", k)
		} else if got != v {
			t.Errorf("GetBuildInfo()[%q] = %q, want %q", k, got, v)
		}
	}
}

// TestGetBuildInfo_ZeroValues confirms empty-string build vars are not
// silently dropped from the returned map (boundary: zero-value strings).
func TestGetBuildInfo_ZeroValues(t *testing.T) {
	origVersion, origBuildDate, origCommitID, origSite := Version, BuildDate, CommitID, OfficialSite
	defer func() {
		Version, BuildDate, CommitID, OfficialSite = origVersion, origBuildDate, origCommitID, origSite
	}()

	Version, BuildDate, CommitID, OfficialSite = "", "", "", ""

	info := GetBuildInfo()
	for _, k := range []string{"version", "build_date", "commit_id", "official_site"} {
		if got, ok := info[k]; !ok {
			t.Errorf("GetBuildInfo() missing key %q for empty values", k)
		} else if got != "" {
			t.Errorf("GetBuildInfo()[%q] = %q, want empty string", k, got)
		}
	}
}

// TestGetVersionString covers the formatted-string composition, including
// the boundary case where every build var is at its unpopulated default.
func TestGetVersionString(t *testing.T) {
	origVersion, origBuildDate, origCommitID := Version, BuildDate, CommitID
	defer func() {
		Version, BuildDate, CommitID = origVersion, origBuildDate, origCommitID
	}()

	tests := []struct {
		name      string
		version   string
		buildDate string
		commitID  string
		want      string
	}{
		{
			name:      "populated values",
			version:   "1.2.3",
			buildDate: "2026-01-01T00:00:00Z",
			commitID:  "abc123",
			want:      "v1.2.3 (built: 2026-01-01T00:00:00Z, commit: abc123)",
		},
		{
			name:      "unpopulated defaults",
			version:   "dev",
			buildDate: "unknown",
			commitID:  "unknown",
			want:      "vdev (built: unknown, commit: unknown)",
		},
		{
			name:      "empty strings",
			version:   "",
			buildDate: "",
			commitID:  "",
			want:      "v (built: , commit: )",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			Version, BuildDate, CommitID = tt.version, tt.buildDate, tt.commitID
			if got := GetVersionString(); got != tt.want {
				t.Errorf("GetVersionString() = %q, want %q", got, tt.want)
			}
		})
	}
}
