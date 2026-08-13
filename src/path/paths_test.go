// Tests for paths.go per AI.md PART 4 (Directory paths) / PART 29 (Testing)
package path

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

// TestGetDefaultPaths_CurrentOS verifies GetDefaultPaths dispatches to a
// platform-specific implementation and returns non-empty, namespaced paths
// for the OS actually running the test (avoids hardcoding a single OS).
func TestGetDefaultPaths_CurrentOS(t *testing.T) {
	p := GetDefaultPaths("wthr")
	if p == nil {
		t.Fatal("GetDefaultPaths returned nil")
	}
	if p.AppName != "wthr" {
		t.Errorf("AppName = %q, want %q", p.AppName, "wthr")
	}
	if p.DataDir == "" || p.ConfigDir == "" || p.LogDir == "" || p.CacheDir == "" || p.TempDir == "" {
		t.Errorf("expected all base dirs populated, got %+v", p)
	}
	if !strings.Contains(p.DataDir, "wthr") {
		t.Errorf("DataDir %q does not contain app name", p.DataDir)
	}
}

// TestGetLinuxPaths_PrivilegeSplit is the core regression target: root vs
// non-root must produce structurally different (system vs XDG) paths, and
// the organization namespace "webappsgo/wthr" must appear in every path.
func TestGetLinuxPaths_PrivilegeSplit(t *testing.T) {
	p := getLinuxPaths("wthr")
	if p.GOOS != "linux" {
		t.Errorf("GOOS = %q, want linux", p.GOOS)
	}

	orgNamespace := filepath.Join("webappsgo", "wthr")

	if os.Geteuid() == 0 {
		if p.IsPrivileged != true {
			t.Error("expected IsPrivileged=true when running as root")
		}
		if !strings.HasPrefix(p.DataDir, "/var/lib") {
			t.Errorf("root DataDir = %q, want prefix /var/lib", p.DataDir)
		}
		if !strings.HasPrefix(p.ConfigDir, "/etc") {
			t.Errorf("root ConfigDir = %q, want prefix /etc", p.ConfigDir)
		}
	} else {
		if p.IsPrivileged != false {
			t.Error("expected IsPrivileged=false when running as non-root")
		}
		if strings.HasPrefix(p.DataDir, "/var/lib") {
			t.Errorf("non-root DataDir = %q must not be a system path", p.DataDir)
		}
	}

	if !strings.Contains(p.DataDir, orgNamespace) {
		t.Errorf("DataDir %q missing org namespace %q", p.DataDir, orgNamespace)
	}
}

// TestGetLinuxPaths_XDGOverride verifies XDG_* environment variables are
// honored for non-root users, and that clearing them falls back to
// $HOME-relative defaults. Skipped when running as root since root paths
// ignore XDG entirely.
func TestGetLinuxPaths_XDGOverride(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("XDG override only applies to non-root paths")
	}

	origData := os.Getenv("XDG_DATA_HOME")
	origConfig := os.Getenv("XDG_CONFIG_HOME")
	origCache := os.Getenv("XDG_CACHE_HOME")
	t.Cleanup(func() {
		os.Setenv("XDG_DATA_HOME", origData)
		os.Setenv("XDG_CONFIG_HOME", origConfig)
		os.Setenv("XDG_CACHE_HOME", origCache)
	})

	os.Setenv("XDG_DATA_HOME", "/custom/data")
	os.Setenv("XDG_CONFIG_HOME", "/custom/config")
	os.Setenv("XDG_CACHE_HOME", "/custom/cache")

	p := getLinuxPaths("wthr")

	wantData := filepath.Join("/custom/data", "webappsgo", "wthr")
	if p.DataDir != wantData {
		t.Errorf("DataDir = %q, want %q", p.DataDir, wantData)
	}
	wantConfig := filepath.Join("/custom/config", "webappsgo", "wthr")
	if p.ConfigDir != wantConfig {
		t.Errorf("ConfigDir = %q, want %q", p.ConfigDir, wantConfig)
	}
	wantCache := filepath.Join("/custom/cache", "webappsgo", "wthr")
	if p.CacheDir != wantCache {
		t.Errorf("CacheDir = %q, want %q", p.CacheDir, wantCache)
	}

	// Clearing XDG vars must fall back to $HOME-relative defaults, not
	// leave stale/empty paths.
	os.Setenv("XDG_DATA_HOME", "")
	os.Setenv("XDG_CONFIG_HOME", "")
	os.Setenv("XDG_CACHE_HOME", "")
	home, _ := os.UserHomeDir()
	p2 := getLinuxPaths("wthr")
	wantDataFallback := filepath.Join(home, ".local", "share", "webappsgo", "wthr")
	if p2.DataDir != wantDataFallback {
		t.Errorf("fallback DataDir = %q, want %q", p2.DataDir, wantDataFallback)
	}
}

// TestEnsureDir_CreatesAndVerifiesWritable exercises real directory
// creation on a temp dir: permission bits, idempotency (create twice),
// and the internal writability probe.
func TestEnsureDir_CreatesAndVerifiesWritable(t *testing.T) {
	base := t.TempDir()

	tests := []struct {
		name   string
		isRoot bool
		want   os.FileMode
	}{
		{"non_root_perm_0700", false, 0700},
		{"root_perm_0755", true, 0755},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := filepath.Join(base, tt.name)
			if err := EnsureDir(dir, tt.isRoot); err != nil {
				t.Fatalf("EnsureDir() error = %v", err)
			}

			info, err := os.Stat(dir)
			if err != nil {
				t.Fatalf("directory not created: %v", err)
			}
			if !info.IsDir() {
				t.Fatal("path exists but is not a directory")
			}
			if info.Mode().Perm() != tt.want {
				t.Errorf("perm = %o, want %o", info.Mode().Perm(), tt.want)
			}

			// Idempotent: calling again on an existing dir must not error.
			if err := EnsureDir(dir, tt.isRoot); err != nil {
				t.Fatalf("EnsureDir() second call error = %v", err)
			}

			// The .write-test probe file must be cleaned up, not left behind.
			if _, err := os.Stat(filepath.Join(dir, ".write-test")); !os.IsNotExist(err) {
				t.Errorf(".write-test probe file was not removed")
			}
		})
	}
}

// TestEnsureFile_WritesWithPermsAndCreatesParent verifies EnsureFile writes
// content, sets exact permission bits, and creates missing parent
// directories per AI.md PART 7.
func TestEnsureFile_WritesWithPermsAndCreatesParent(t *testing.T) {
	base := t.TempDir()
	path := filepath.Join(base, "nested", "sub", "file.txt")
	content := []byte("hello world")

	if err := EnsureFile(path, content, false); err != nil {
		t.Fatalf("EnsureFile() error = %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("file not created: %v", err)
	}
	if string(data) != string(content) {
		t.Errorf("content = %q, want %q", data, content)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat error: %v", err)
	}
	if info.Mode().Perm() != 0600 {
		t.Errorf("perm = %o, want 0600", info.Mode().Perm())
	}

	// Overwrite with root perms - must update mode even though file exists.
	if err := EnsureFile(path, []byte("v2"), true); err != nil {
		t.Fatalf("EnsureFile() overwrite error = %v", err)
	}
	info2, _ := os.Stat(path)
	if info2.Mode().Perm() != 0644 {
		t.Errorf("perm after overwrite = %o, want 0644", info2.Mode().Perm())
	}
	data2, _ := os.ReadFile(path)
	if string(data2) != "v2" {
		t.Errorf("content after overwrite = %q, want %q", data2, "v2")
	}
}

// TestResolvePath covers placeholder substitution, multiple placeholders in
// one string, unknown/no placeholders, and the empty-string short-circuit.
func TestResolvePath(t *testing.T) {
	// Reset global state so this test controls the instance deterministically.
	resetPathsSingleton(t)
	setTempDirEnv(t)
	Initialize("wthr")

	tests := []struct {
		name  string
		input string
	}{
		{"empty_string", ""},
		{"no_placeholders", "/absolute/literal/path"},
		{"single_placeholder", "{data_dir}/server.db"},
		{"multiple_placeholders", "{config_dir}/ssl:{data_dir}/backups"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ResolvePath(tt.input)
			if tt.input == "" {
				if got != "" {
					t.Errorf("ResolvePath(\"\") = %q, want empty", got)
				}
				return
			}
			if strings.Contains(got, "{") || strings.Contains(got, "}") {
				t.Errorf("ResolvePath(%q) = %q, unresolved placeholder remains", tt.input, got)
			}
		})
	}
}

// TestResolveConfigPath verifies absolute paths pass through unchanged and
// that a filename resolves to the config dir when no file exists on disk
// yet (the common first-run case).
func TestResolveConfigPath(t *testing.T) {
	resetPathsSingleton(t)
	setTempDirEnv(t)
	Initialize("wthr")

	t.Run("absolute_path_passthrough", func(t *testing.T) {
		abs := "/some/absolute/server.yml"
		if got := ResolveConfigPath(abs); got != abs {
			t.Errorf("ResolveConfigPath(%q) = %q, want unchanged", abs, got)
		}
	})

	t.Run("relative_defaults_to_config_dir", func(t *testing.T) {
		got := ResolveConfigPath("nonexistent-server.yml")
		want := filepath.Join(GetInstance().ConfigDir, "nonexistent-server.yml")
		if got != want {
			t.Errorf("ResolveConfigPath() = %q, want %q", got, want)
		}
	})
}

// TestIsPrivileged verifies the reported privilege level matches the
// process's actual euid, without hardcoding an assumption about the test
// runner's privilege level.
func TestIsPrivileged(t *testing.T) {
	got := isPrivileged()
	want := os.Geteuid() == 0
	if got != want {
		t.Errorf("isPrivileged() = %v, want %v (euid=%d)", got, want, os.Geteuid())
	}
}

// setTempDirEnv points every path override env var at a fresh temp dir so
// Initialize() never touches real system or user directories during tests,
// per testing-rules.md (test output must never land in real/project paths).
func setTempDirEnv(t *testing.T) {
	t.Helper()
	base := t.TempDir()
	vars := map[string]string{
		"DATA_DIR":   filepath.Join(base, "data"),
		"CONFIG_DIR": filepath.Join(base, "config"),
		"LOG_DIR":    filepath.Join(base, "log"),
		"CACHE_DIR":  filepath.Join(base, "cache"),
		"TEMP_DIR":   filepath.Join(base, "tmp"),
	}
	for k, v := range vars {
		old, had := os.LookupEnv(k)
		os.Setenv(k, v)
		t.Cleanup(func() {
			if had {
				os.Setenv(k, old)
			} else {
				os.Unsetenv(k)
			}
		})
	}
}

// resetPathsSingleton clears the package-level sync.Once/global state so
// each test can exercise Initialize() fresh. It restores the prior globals
// via t.Cleanup so tests don't leak state into one another.
func resetPathsSingleton(t *testing.T) {
	t.Helper()
	// initOnce is a plain sync.Once (not a pointer), so its value cannot be
	// saved and restored without copying the lock (go vet correctly flags
	// that). A used sync.Once has no meaningful "restore" semantics anyway -
	// resetting to a fresh zero Once on both entry and cleanup achieves the
	// same practical isolation between tests.
	prevPaths := defaultPaths
	defaultPaths = nil
	initOnce = sync.Once{}
	t.Cleanup(func() {
		defaultPaths = prevPaths
		initOnce = sync.Once{}
	})
}
