// Tests for update.go per AI.md PART 23 (Update) / PART 29 (Testing).
//
// fetchRelease/downloadFile/verifyChecksum are exercised against a local
// httptest.Server rather than the real GitHub API, per testing-rules.md
// ("NEVER write tests that depend on external services -> mock or skip").
// checkForUpdates/performUpdate against the real GitHub API are NOT covered
// here for the same reason; performUpdate's branch-validation error path
// (which returns before any network call) IS covered via UpdateCommand.
package cli

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

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

// TestIsNewer covers the version-string comparison used to decide whether
// to advertise an update: newer, older, equal, and the "v" prefix stripping.
func TestIsNewer(t *testing.T) {
	tests := []struct {
		name string
		a    string
		b    string
		want bool
	}{
		{"newer_without_prefix", "2.0.0", "1.0.0", true},
		{"older_without_prefix", "1.0.0", "2.0.0", false},
		{"equal_versions", "1.0.0", "1.0.0", false},
		{"newer_with_v_prefix", "v2.0.0", "v1.0.0", true},
		{"mixed_prefix", "v2.0.0", "1.0.0", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isNewer(tt.a, tt.b); got != tt.want {
				t.Errorf("isNewer(%q, %q) = %v, want %v", tt.a, tt.b, got, tt.want)
			}
		})
	}
}

// TestCopyFile covers a normal copy (content + permission bits preserved)
// and the error path when the source file does not exist. All I/O happens
// under t.TempDir().
func TestCopyFile(t *testing.T) {
	t.Run("copies_content_and_permissions", func(t *testing.T) {
		dir := t.TempDir()
		src := filepath.Join(dir, "src.bin")
		dst := filepath.Join(dir, "dst.bin")

		if err := os.WriteFile(src, []byte("binary-payload"), 0755); err != nil {
			t.Fatalf("setup: %v", err)
		}

		if err := copyFile(src, dst); err != nil {
			t.Fatalf("copyFile() error = %v", err)
		}

		got, err := os.ReadFile(dst)
		if err != nil {
			t.Fatalf("dst not written: %v", err)
		}
		if string(got) != "binary-payload" {
			t.Errorf("dst content = %q, want %q", got, "binary-payload")
		}

		srcInfo, _ := os.Stat(src)
		dstInfo, err := os.Stat(dst)
		if err != nil {
			t.Fatalf("stat dst: %v", err)
		}
		if dstInfo.Mode() != srcInfo.Mode() {
			t.Errorf("dst mode = %v, want %v", dstInfo.Mode(), srcInfo.Mode())
		}
	})

	t.Run("missing_source_errors", func(t *testing.T) {
		dir := t.TempDir()
		if err := copyFile(filepath.Join(dir, "nope"), filepath.Join(dir, "dst")); err == nil {
			t.Error("copyFile() on missing source = nil, want error")
		}
	})
}

// TestFetchRelease covers a well-formed 200 response, a non-200 status, and
// malformed JSON, against a local httptest server.
func TestFetchRelease(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"tag_name":"v1.2.3","name":"Release 1.2.3"}`))
		}))
		defer srv.Close()

		release, err := fetchRelease(srv.URL)
		if err != nil {
			t.Fatalf("fetchRelease() error = %v", err)
		}
		if release.TagName != "v1.2.3" {
			t.Errorf("release.TagName = %q, want %q", release.TagName, "v1.2.3")
		}
	})

	t.Run("non_200_status_errors", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNotFound)
		}))
		defer srv.Close()

		if _, err := fetchRelease(srv.URL); err == nil {
			t.Error("fetchRelease() on 404 = nil, want error")
		}
	})

	t.Run("malformed_json_errors", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Write([]byte("not json"))
		}))
		defer srv.Close()

		if _, err := fetchRelease(srv.URL); err == nil {
			t.Error("fetchRelease() on malformed JSON = nil, want error")
		}
	})
}

// TestDownloadFile covers a successful download to a temp path and a
// non-200 upstream status.
func TestDownloadFile(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Write([]byte("payload-bytes"))
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

// TestVerifyChecksum covers a matching checksum, a mismatched checksum, the
// "hash  filename" checksum-file format, and a checksum endpoint that 404s.
func TestVerifyChecksum(t *testing.T) {
	t.Run("matching_checksum_ok", func(t *testing.T) {
		fileDir := t.TempDir()
		filePath := filepath.Join(fileDir, "artifact.bin")
		if err := os.WriteFile(filePath, []byte("hello world"), 0644); err != nil {
			t.Fatalf("setup: %v", err)
		}
		// sha256("hello world")
		const wantHash = "b94d27b9934d3e08a52e52d7da7dabfac484efe37a5380ee9088f7ace2efcde9"

		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Write([]byte(wantHash))
		}))
		defer srv.Close()

		if err := verifyChecksum(filePath, srv.URL); err != nil {
			t.Errorf("verifyChecksum() error = %v, want nil", err)
		}
	})

	t.Run("checksum_with_filename_suffix_ok", func(t *testing.T) {
		fileDir := t.TempDir()
		filePath := filepath.Join(fileDir, "artifact.bin")
		if err := os.WriteFile(filePath, []byte("hello world"), 0644); err != nil {
			t.Fatalf("setup: %v", err)
		}
		const wantHash = "b94d27b9934d3e08a52e52d7da7dabfac484efe37a5380ee9088f7ace2efcde9"

		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Write([]byte(wantHash + "  artifact.bin\n"))
		}))
		defer srv.Close()

		if err := verifyChecksum(filePath, srv.URL); err != nil {
			t.Errorf("verifyChecksum() error = %v, want nil", err)
		}
	})

	t.Run("mismatched_checksum_errors", func(t *testing.T) {
		fileDir := t.TempDir()
		filePath := filepath.Join(fileDir, "artifact.bin")
		if err := os.WriteFile(filePath, []byte("hello world"), 0644); err != nil {
			t.Fatalf("setup: %v", err)
		}

		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Write([]byte("0000000000000000000000000000000000000000000000000000000000000000"))
		}))
		defer srv.Close()

		if err := verifyChecksum(filePath, srv.URL); err == nil {
			t.Error("verifyChecksum() with mismatched hash = nil, want error")
		}
	})

	t.Run("checksum_endpoint_404_errors", func(t *testing.T) {
		fileDir := t.TempDir()
		filePath := filepath.Join(fileDir, "artifact.bin")
		if err := os.WriteFile(filePath, []byte("hello world"), 0644); err != nil {
			t.Fatalf("setup: %v", err)
		}

		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNotFound)
		}))
		defer srv.Close()

		if err := verifyChecksum(filePath, srv.URL); err == nil {
			t.Error("verifyChecksum() on 404 checksum file = nil, want error")
		}
	})
}
