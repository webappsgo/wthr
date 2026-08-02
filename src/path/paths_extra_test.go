package paths

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestInitializeAndGetInstance verifies Initialize populates the global
// singleton exactly once and GetInstance returns the same instance,
// creating an entry for every documented subdirectory.
func TestInitializeAndGetInstance(t *testing.T) {
	resetPathsSingleton(t)
	setTempDirEnv(t)

	if err := Initialize("wthr"); err != nil {
		t.Fatalf("Initialize() returned error: %v", err)
	}

	inst := GetInstance()
	if inst == nil {
		t.Fatal("GetInstance() returned nil after Initialize()")
	}
	if inst.AppName != "wthr" {
		t.Errorf("AppName = %q, want %q", inst.AppName, "wthr")
	}

	// A second GetInstance() call must return the exact same instance
	// (Initialize is only-once, per the sync.Once guard).
	if second := GetInstance(); second != inst {
		t.Error("GetInstance() returned a different instance on second call")
	}
}

// TestGlobalDirGetters verifies every Get*Dir() convenience function reads
// from the initialized singleton instance.
func TestGlobalDirGetters(t *testing.T) {
	resetPathsSingleton(t)
	setTempDirEnv(t)
	Initialize("wthr")

	inst := GetInstance()

	cases := []struct {
		name string
		got  string
		want string
	}{
		{"GetDataDir", GetDataDir(), inst.DataDir},
		{"GetConfigDir", GetConfigDir(), inst.ConfigDir},
		{"GetLogDir", GetLogDir(), inst.LogDir},
		{"GetCacheDir", GetCacheDir(), inst.CacheDir},
		{"GetTempDir", GetTempDir(), inst.TempDir},
		{"GetSSLDir", GetSSLDir(), inst.SSLDir},
		{"GetTorDir", GetTorDir(), inst.TorDir},
		{"GetGeoIPDir", GetGeoIPDir(), inst.GeoIPDir},
		{"GetBackupDir", GetBackupDir(), inst.BackupDir},
		{"GetBlocklistDir", GetBlocklistDir(), inst.BlocklistDir},
	}

	for _, tc := range cases {
		if tc.got != tc.want {
			t.Errorf("%s() = %q, want %q", tc.name, tc.got, tc.want)
		}
	}

	if IsPrivileged() != inst.IsPrivileged {
		t.Errorf("IsPrivileged() = %v, want %v", IsPrivileged(), inst.IsPrivileged)
	}
}

// TestGetConfigFilePath verifies it resolves to server.yml under ConfigDir
// when no such file exists yet on disk (first-run case).
func TestGetConfigFilePath(t *testing.T) {
	resetPathsSingleton(t)
	setTempDirEnv(t)
	Initialize("wthr")

	got := GetConfigFilePath()
	want := filepath.Join(GetInstance().ConfigDir, "server.yml")
	if got != want {
		t.Errorf("GetConfigFilePath() = %q, want %q", got, want)
	}
}

// TestInitializeSubdirectories verifies the derived SSL/Tor/GeoIP/Backup/
// Blocklist subdirectory paths are built under the expected parent dirs.
func TestInitializeSubdirectories(t *testing.T) {
	p := &Paths{
		ConfigDir: "/tmp/example/config",
		DataDir:   "/tmp/example/data",
	}
	p.initializeSubdirectories()

	if want := filepath.Join(p.ConfigDir, "ssl"); p.SSLDir != want {
		t.Errorf("SSLDir = %q, want %q", p.SSLDir, want)
	}
	if want := filepath.Join(p.ConfigDir, "security", "geoip"); p.GeoIPDir != want {
		t.Errorf("GeoIPDir = %q, want %q", p.GeoIPDir, want)
	}
	if want := filepath.Join(p.ConfigDir, "security", "blocklists"); p.BlocklistDir != want {
		t.Errorf("BlocklistDir = %q, want %q", p.BlocklistDir, want)
	}
	if want := filepath.Join(p.DataDir, "tor"); p.TorDir != want {
		t.Errorf("TorDir = %q, want %q", p.TorDir, want)
	}
	if want := filepath.Join(p.DataDir, "backups"); p.BackupDir != want {
		t.Errorf("BackupDir = %q, want %q", p.BackupDir, want)
	}
	if p.GOOS == "" {
		t.Error("expected GOOS to be populated by initializeSubdirectories")
	}
}

// TestOverride verifies environment variables take precedence over the
// struct's existing path values when set, and leave them untouched when
// unset.
func TestOverride(t *testing.T) {
	base := t.TempDir()
	envVars := map[string]string{
		"DATA_DIR":   filepath.Join(base, "override-data"),
		"CONFIG_DIR": filepath.Join(base, "override-config"),
	}
	for k, v := range envVars {
		old, had := os.LookupEnv(k)
		os.Setenv(k, v)
		t.Cleanup(func(k, old string, had bool) func() {
			return func() {
				if had {
					os.Setenv(k, old)
				} else {
					os.Unsetenv(k)
				}
			}
		}(k, old, had))
	}
	// Make sure LOG_DIR/CACHE_DIR/TEMP_DIR are unset so we can verify the
	// no-override path leaves the original value intact.
	for _, k := range []string{"LOG_DIR", "CACHE_DIR", "TEMP_DIR"} {
		old, had := os.LookupEnv(k)
		os.Unsetenv(k)
		t.Cleanup(func(k, old string, had bool) func() {
			return func() {
				if had {
					os.Setenv(k, old)
				}
			}
		}(k, old, had))
	}

	p := &Paths{
		DataDir:   "/original/data",
		ConfigDir: "/original/config",
		LogDir:    "/original/log",
		CacheDir:  "/original/cache",
		TempDir:   "/original/tmp",
	}
	p.Override()

	if p.DataDir != envVars["DATA_DIR"] {
		t.Errorf("DataDir = %q, want %q", p.DataDir, envVars["DATA_DIR"])
	}
	if p.ConfigDir != envVars["CONFIG_DIR"] {
		t.Errorf("ConfigDir = %q, want %q", p.ConfigDir, envVars["CONFIG_DIR"])
	}
	if p.LogDir != "/original/log" {
		t.Errorf("LogDir = %q, want unchanged %q", p.LogDir, "/original/log")
	}
	if p.CacheDir != "/original/cache" {
		t.Errorf("CacheDir = %q, want unchanged %q", p.CacheDir, "/original/cache")
	}
	if p.TempDir != "/original/tmp" {
		t.Errorf("TempDir = %q, want unchanged %q", p.TempDir, "/original/tmp")
	}
}

// TestEnsurePIDFile verifies it creates the PID file's parent directory.
func TestEnsurePIDFile(t *testing.T) {
	base := t.TempDir()
	pidPath := filepath.Join(base, "run", "wthr.pid")

	if err := EnsurePIDFile(pidPath, false); err != nil {
		t.Fatalf("EnsurePIDFile() returned error: %v", err)
	}

	info, err := os.Stat(filepath.Dir(pidPath))
	if err != nil {
		t.Fatalf("expected parent directory to exist: %v", err)
	}
	if !info.IsDir() {
		t.Error("expected parent path to be a directory")
	}
}

// TestEnsureAllDirectories verifies every top-level and subdirectory field
// gets created on disk, including the SSL/Tor nested subdirectories.
func TestEnsureAllDirectories(t *testing.T) {
	base := t.TempDir()
	p := &Paths{
		DataDir:      filepath.Join(base, "data"),
		ConfigDir:    filepath.Join(base, "config"),
		LogDir:       filepath.Join(base, "log"),
		CacheDir:     filepath.Join(base, "cache"),
		TempDir:      filepath.Join(base, "tmp"),
		IsPrivileged: false,
	}
	p.initializeSubdirectories()

	if err := p.EnsureAllDirectories(); err != nil {
		t.Fatalf("EnsureAllDirectories() returned error: %v", err)
	}

	checkDirs := []string{
		p.DataDir, p.ConfigDir, p.LogDir, p.CacheDir, p.TempDir,
		p.SSLDir, p.TorDir, p.GeoIPDir, p.BackupDir, p.BlocklistDir,
		filepath.Join(p.SSLDir, "letsencrypt"),
		filepath.Join(p.SSLDir, "local"),
		filepath.Join(p.TorDir, "site"),
	}
	for _, dir := range checkDirs {
		info, err := os.Stat(dir)
		if err != nil {
			t.Errorf("expected directory %q to exist: %v", dir, err)
			continue
		}
		if !info.IsDir() {
			t.Errorf("expected %q to be a directory", dir)
		}
	}
}

// TestPrintPaths verifies it writes every configured field to stdout
// without panicking.
func TestPrintPaths(t *testing.T) {
	p := &Paths{
		GOOS:         "linux",
		IsPrivileged: false,
		AppName:      "wthr",
		ConfigDir:    "/cfg",
		DataDir:      "/data",
		LogDir:       "/log",
		CacheDir:     "/cache",
		TempDir:      "/tmp",
		SSLDir:       "/cfg/ssl",
		TorDir:       "/data/tor",
		GeoIPDir:     "/cfg/security/geoip",
		BackupDir:    "/data/backups",
		BlocklistDir: "/cfg/security/blocklists",
	}

	stdout := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe() failed: %v", err)
	}
	os.Stdout = w
	p.PrintPaths()
	w.Close()
	os.Stdout = stdout

	var buf bytes.Buffer
	if _, err := io.Copy(&buf, r); err != nil {
		t.Fatalf("failed reading captured stdout: %v", err)
	}
	out := buf.String()

	for _, want := range []string{"linux", "wthr", "/cfg", "/data", "/log", "/cache"} {
		if !strings.Contains(out, want) {
			t.Errorf("PrintPaths() output missing %q, got:\n%s", want, out)
		}
	}
}
