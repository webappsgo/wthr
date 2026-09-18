// Tests for update.go per AI.md PART 23 (Update) / PART 29 (Testing).
//
// fetchReleases/downloadFile/fetchChecksum are exercised against a local
// httptest.Server rather than the real GitHub API, per testing-rules.md
// ("NEVER write tests that depend on external services -> mock or skip").
// checkForUpdates/performUpdate against the real GitHub API are NOT covered
// here for the same reason; performUpdate's branch-validation error path
// (which returns before any network call) IS covered via UpdateCommand.
package cli

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// helloWorldSHA256 is the SHA-256 digest of the literal bytes "hello world",
// used as the known-good hash in the checksum tests below.
const helloWorldSHA256 = "b94d27b9934d3e08a52e52d7da7dabfac484efe37a5380ee9088f7ace2efcde9"

// TestUpdateCommand_Dispatch covers the command-routing error paths that
// never reach the network: no args, unknown command, missing branch value,
// and an invalid branch name (performUpdate validates the branch before
// making any HTTP request).
func TestUpdateCommand_Dispatch(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		wantErr string
	}{
		{"no_args", []string{}, "no update command specified"},
		{"unknown_command", []string{"bogus"}, "unknown update command: bogus"},
		{"branch_missing_value", []string{"branch"}, "branch requires a value"},
		{"branch_invalid_value", []string{"branch", "sideways"}, "invalid branch: sideways"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := UpdateCommand(tt.args)
			if err == nil {
				t.Fatalf("UpdateCommand(%v) = nil, want error containing %q", tt.args, tt.wantErr)
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("UpdateCommand(%v) error = %q, want substring %q", tt.args, err.Error(), tt.wantErr)
			}
		})
	}
}

// TestMatchesBranch covers the cumulative channel rule from PART 23: stable
// sees only stable, beta sees beta + stable, daily sees everything.
func TestMatchesBranch(t *testing.T) {
	stable := GitHubRelease{TagName: "v1.2.3"}
	beta := GitHubRelease{TagName: "v1.3.0-beta", Prerelease: true}
	daily := GitHubRelease{TagName: "daily", Prerelease: true}
	prerelease := GitHubRelease{TagName: "v2.0.0-rc1", Prerelease: true}

	tests := []struct {
		name    string
		release GitHubRelease
		branch  string
		want    bool
	}{
		{"stable_channel_takes_stable", stable, "stable", true},
		{"stable_channel_rejects_beta", beta, "stable", false},
		{"stable_channel_rejects_daily", daily, "stable", false},
		{"beta_channel_takes_beta", beta, "beta", true},
		{"beta_channel_takes_stable", stable, "beta", true},
		{"beta_channel_rejects_daily", daily, "beta", false},
		{"daily_channel_takes_daily", daily, "daily", true},
		{"daily_channel_takes_beta", beta, "daily", true},
		{"daily_channel_takes_stable", stable, "daily", true},
		{"prerelease_is_not_stable", prerelease, "stable", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := MatchesBranch(tt.release, tt.branch); got != tt.want {
				t.Errorf("MatchesBranch(%q, %q) = %v, want %v", tt.release.TagName, tt.branch, got, tt.want)
			}
		})
	}
}

// TestSelectRelease covers channel selection, the already-current
// short-circuit, and the daily channel's BuildEpoch comparison.
func TestSelectRelease(t *testing.T) {
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	stableOld := GitHubRelease{TagName: "v1.0.0", PublishedAt: base}
	stableNew := GitHubRelease{TagName: "v1.1.0", PublishedAt: base.Add(48 * time.Hour)}
	betaNew := GitHubRelease{TagName: "v1.2.0-beta", Prerelease: true, PublishedAt: base.Add(72 * time.Hour)}
	releases := []GitHubRelease{stableOld, stableNew, betaNew}

	t.Run("stable_channel_picks_newest_stable", func(t *testing.T) {
		got := SelectRelease(releases, "stable", "v1.0.0")
		if got == nil || got.TagName != "v1.1.0" {
			t.Fatalf("SelectRelease(stable) = %v, want v1.1.0", got)
		}
	})

	t.Run("beta_channel_picks_newest_beta", func(t *testing.T) {
		got := SelectRelease(releases, "beta", "v1.0.0")
		if got == nil || got.TagName != "v1.2.0-beta" {
			t.Fatalf("SelectRelease(beta) = %v, want v1.2.0-beta", got)
		}
	})

	t.Run("already_current_returns_nil", func(t *testing.T) {
		if got := SelectRelease(releases, "stable", "v1.1.0"); got != nil {
			t.Errorf("SelectRelease() on current version = %v, want nil", got)
		}
	})

	t.Run("no_matching_channel_returns_nil", func(t *testing.T) {
		if got := SelectRelease([]GitHubRelease{betaNew}, "stable", "v1.0.0"); got != nil {
			t.Errorf("SelectRelease() with no stable release = %v, want nil", got)
		}
	})

	t.Run("daily_compares_against_build_epoch", func(t *testing.T) {
		published := base.Add(96 * time.Hour)
		daily := []GitHubRelease{{TagName: "daily", Prerelease: true, PublishedAt: published}}

		saved := BuildEpoch
		defer func() { BuildEpoch = saved }()

		BuildEpoch = fmt.Sprintf("%d", published.Add(-time.Hour).Unix())
		if got := SelectRelease(daily, "daily", "v1.0.0"); got == nil {
			t.Error("SelectRelease(daily) with older build = nil, want the daily release")
		}

		BuildEpoch = fmt.Sprintf("%d", published.Add(time.Hour).Unix())
		if got := SelectRelease(daily, "daily", "v1.0.0"); got != nil {
			t.Errorf("SelectRelease(daily) with newer build = %v, want nil", got)
		}
	})
}

// TestIsReleaseEligible covers the defer window that gates the scheduled
// update_check task: zero/negative defer always passes, a release younger
// than the window is held back, an older one is eligible.
func TestIsReleaseEligible(t *testing.T) {
	now := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)

	tests := []struct {
		name        string
		publishedAt time.Time
		deferDays   int
		want        bool
	}{
		{"zero_defer_always_eligible", now, 0, true},
		{"negative_defer_always_eligible", now, -5, true},
		{"younger_than_window_held", now.Add(-24 * time.Hour), 7, false},
		{"exactly_at_window_eligible", now.Add(-7 * 24 * time.Hour), 7, true},
		{"older_than_window_eligible", now.Add(-30 * 24 * time.Hour), 7, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsReleaseEligible(tt.publishedAt, tt.deferDays, now); got != tt.want {
				t.Errorf("IsReleaseEligible(%v, %d) = %v, want %v", tt.publishedAt, tt.deferDays, got, tt.want)
			}
		})
	}
}

// TestBuildEpoch covers the three link-time states: unset, valid, and
// non-numeric — the latter two both fall back to 0 rather than failing.
func TestBuildEpoch(t *testing.T) {
	saved := BuildEpoch
	defer func() { BuildEpoch = saved }()

	tests := []struct {
		name  string
		value string
		want  int64
	}{
		{"unset_is_zero", "", 0},
		{"valid_epoch", "1767225600", 1767225600},
		{"non_numeric_is_zero", "not-a-number", 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			BuildEpoch = tt.value
			if got := buildEpoch(); got != tt.want {
				t.Errorf("buildEpoch() with %q = %d, want %d", tt.value, got, tt.want)
			}
		})
	}
}

// TestGetBinaryName verifies the asset name matches the release naming
// scheme for the running platform, including the Windows .exe suffix.
func TestGetBinaryName(t *testing.T) {
	got := getBinaryName()

	want := "wthr-" + runtime.GOOS + "-" + runtime.GOARCH
	if runtime.GOOS == "windows" {
		want += ".exe"
	}
	if got != want {
		t.Errorf("getBinaryName() = %q, want %q", got, want)
	}
}

// TestFetchReleases covers a well-formed 200 response, the 404 "no releases
// yet" case that must not be an error, a non-200 status, and malformed JSON.
func TestFetchReleases(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `[{"tag_name":"v1.2.3","name":"Release 1.2.3"}]`)
		}))
		defer srv.Close()

		releases, err := fetchReleases(srv.URL)
		if err != nil {
			t.Fatalf("fetchReleases() error = %v", err)
		}
		if len(releases) != 1 || releases[0].TagName != "v1.2.3" {
			t.Errorf("fetchReleases() = %v, want one release tagged v1.2.3", releases)
		}
	})

	t.Run("404_means_no_releases", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNotFound)
		}))
		defer srv.Close()

		releases, err := fetchReleases(srv.URL)
		if err != nil {
			t.Fatalf("fetchReleases() on 404 error = %v, want nil", err)
		}
		if releases != nil {
			t.Errorf("fetchReleases() on 404 = %v, want nil", releases)
		}
	})

	t.Run("non_200_status_errors", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		}))
		defer srv.Close()

		if _, err := fetchReleases(srv.URL); err == nil {
			t.Error("fetchReleases() on 500 = nil, want error")
		}
	})

	t.Run("malformed_json_errors", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			fmt.Fprint(w, "not json")
		}))
		defer srv.Close()

		if _, err := fetchReleases(srv.URL); err == nil {
			t.Error("fetchReleases() on malformed JSON = nil, want error")
		}
	})
}

// TestDownloadFile covers a successful download to a temp path and a
// non-200 upstream status.
func TestDownloadFile(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			fmt.Fprint(w, "payload-bytes")
		}))
		defer srv.Close()

		dst := filepath.Join(t.TempDir(), "downloaded.bin")
		if err := downloadFile(dst, srv.URL); err != nil {
			t.Fatalf("downloadFile() error = %v", err)
		}

		got, err := os.ReadFile(dst)
		if err != nil {
			t.Fatalf("downloaded file missing: %v", err)
		}
		if string(got) != "payload-bytes" {
			t.Errorf("downloaded content = %q, want %q", got, "payload-bytes")
		}
	})

	t.Run("non_200_status_errors", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		}))
		defer srv.Close()

		dst := filepath.Join(t.TempDir(), "downloaded.bin")
		if err := downloadFile(dst, srv.URL); err == nil {
			t.Error("downloadFile() on 500 = nil, want error")
		}
	})
}

// TestFetchChecksum covers parsing the sha256.txt asset: the plain
// "hash  filename" form, the binary-mode "hash  *filename" form, a name with
// no entry, and a 404 checksum asset.
func TestFetchChecksum(t *testing.T) {
	body := "0000000000000000000000000000000000000000000000000000000000000000  other.bin\n" +
		helloWorldSHA256 + "  wthr-linux-amd64\n" +
		helloWorldSHA256 + "  *wthr-windows-amd64.exe\n"

	newServer := func(t *testing.T) *httptest.Server {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			fmt.Fprint(w, body)
		}))
		t.Cleanup(srv.Close)
		return srv
	}

	t.Run("plain_entry", func(t *testing.T) {
		got, err := fetchChecksum(newServer(t).URL, "wthr-linux-amd64")
		if err != nil {
			t.Fatalf("fetchChecksum() error = %v", err)
		}
		if got != helloWorldSHA256 {
			t.Errorf("fetchChecksum() = %q, want %q", got, helloWorldSHA256)
		}
	})

	t.Run("binary_mode_entry", func(t *testing.T) {
		got, err := fetchChecksum(newServer(t).URL, "wthr-windows-amd64.exe")
		if err != nil {
			t.Fatalf("fetchChecksum() error = %v", err)
		}
		if got != helloWorldSHA256 {
			t.Errorf("fetchChecksum() = %q, want %q", got, helloWorldSHA256)
		}
	})

	t.Run("missing_entry_errors", func(t *testing.T) {
		if _, err := fetchChecksum(newServer(t).URL, "wthr-plan9-mips"); err == nil {
			t.Error("fetchChecksum() for absent asset = nil, want error")
		}
	})

	t.Run("checksum_endpoint_404_errors", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNotFound)
		}))
		defer srv.Close()

		if _, err := fetchChecksum(srv.URL, "wthr-linux-amd64"); err == nil {
			t.Error("fetchChecksum() on 404 = nil, want error")
		}
	})
}

// TestVerifyChecksum covers a matching hash, a case-insensitive match, a
// mismatch, and a missing file.
func TestVerifyChecksum(t *testing.T) {
	writeArtifact := func(t *testing.T) string {
		path := filepath.Join(t.TempDir(), "artifact.bin")
		if err := os.WriteFile(path, []byte("hello world"), 0644); err != nil {
			t.Fatalf("setup: %v", err)
		}
		return path
	}

	t.Run("matching_checksum_ok", func(t *testing.T) {
		if err := verifyChecksum(writeArtifact(t), helloWorldSHA256); err != nil {
			t.Errorf("verifyChecksum() error = %v, want nil", err)
		}
	})

	t.Run("uppercase_checksum_ok", func(t *testing.T) {
		if err := verifyChecksum(writeArtifact(t), strings.ToUpper(helloWorldSHA256)); err != nil {
			t.Errorf("verifyChecksum() with uppercase hash error = %v, want nil", err)
		}
	})

	t.Run("mismatched_checksum_errors", func(t *testing.T) {
		bad := "0000000000000000000000000000000000000000000000000000000000000000"
		if err := verifyChecksum(writeArtifact(t), bad); err == nil {
			t.Error("verifyChecksum() with mismatched hash = nil, want error")
		}
	})

	t.Run("missing_file_errors", func(t *testing.T) {
		if err := verifyChecksum(filepath.Join(t.TempDir(), "nope.bin"), helloWorldSHA256); err == nil {
			t.Error("verifyChecksum() on missing file = nil, want error")
		}
	})
}

// TestReplaceBinary verifies the atomic swap installs the new content over
// the target path and preserves the original permission bits.
func TestReplaceBinary(t *testing.T) {
	dir := t.TempDir()
	current := filepath.Join(dir, "wthr")
	staged := filepath.Join(dir, "wthr-new")

	if err := os.WriteFile(current, []byte("old"), 0755); err != nil {
		t.Fatalf("setup: %v", err)
	}
	if err := os.WriteFile(staged, []byte("new"), 0600); err != nil {
		t.Fatalf("setup: %v", err)
	}

	if err := replaceBinary(current, staged); err != nil {
		t.Fatalf("replaceBinary() error = %v", err)
	}

	got, err := os.ReadFile(current)
	if err != nil {
		t.Fatalf("current binary missing after swap: %v", err)
	}
	if string(got) != "new" {
		t.Errorf("current binary content = %q, want %q", got, "new")
	}

	if runtime.GOOS != "windows" {
		info, err := os.Stat(current)
		if err != nil {
			t.Fatalf("stat current: %v", err)
		}
		if info.Mode().Perm() != 0755 {
			t.Errorf("current binary mode = %v, want %v", info.Mode().Perm(), os.FileMode(0755))
		}
	}
}
