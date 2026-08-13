package util

import (
	"os"
	"path/filepath"
	"testing"
)

// TestGetTestDirectoryPaths verifies the test-only path builder returns
// paths rooted under os.TempDir(), never touching real system directories.
func TestGetTestDirectoryPaths(t *testing.T) {
	paths, err := GetTestDirectoryPaths()
	if err != nil {
		t.Fatalf("GetTestDirectoryPaths: %v", err)
	}
	tempBase := filepath.Join(os.TempDir(), "webappsgo", "wthr-test")
	tests := []struct {
		name string
		got  string
		want string
	}{
		{"Config", paths.Config, filepath.Join(tempBase, "config")},
		{"Data", paths.Data, filepath.Join(tempBase, "data")},
		{"Log", paths.Log, filepath.Join(tempBase, "logs")},
		{"Cache", paths.Cache, filepath.Join(tempBase, "cache")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.got != tt.want {
				t.Errorf("%s = %q, want %q", tt.name, tt.got, tt.want)
			}
		})
	}
}

// TestCreateDirectoriesAndCleanup verifies CreateDirectories creates all
// required directories (including subdirectories) with the expected
// permission bit, and CleanupTestDirectories removes them afterward.
func TestCreateDirectoriesAndCleanup(t *testing.T) {
	// Use a paths struct rooted in t.TempDir() rather than
	// GetTestDirectoryPaths() so cleanup never touches a shared /tmp path.
	base := t.TempDir()
	paths := &DirectoryPaths{
		Config: filepath.Join(base, "config"),
		Data:   filepath.Join(base, "data"),
		Log:    filepath.Join(base, "logs"),
		Cache:  filepath.Join(base, "cache"),
	}

	if err := CreateDirectories(paths); err != nil {
		t.Fatalf("CreateDirectories: %v", err)
	}

	dirs := []string{
		paths.Config,
		paths.Data,
		paths.Log,
		paths.Cache,
		GetCertsPath(paths),
		GetConfigDatabasesPath(paths),
		GetDatabaseDir(paths),
		GetBackupPath(paths),
		GetWeatherCachePath(paths),
	}
	wantPerm := os.FileMode(0700)
	if isRoot() {
		wantPerm = 0755
	}
	for _, dir := range dirs {
		info, err := os.Stat(dir)
		if err != nil {
			t.Fatalf("Stat(%s): %v", dir, err)
		}
		if !info.IsDir() {
			t.Errorf("%s is not a directory", dir)
		}
		if info.Mode().Perm() != wantPerm {
			t.Errorf("%s perm = %v, want %v", dir, info.Mode().Perm(), wantPerm)
		}
	}

	if err := CleanupTestDirectories(paths); err != nil {
		t.Fatalf("CleanupTestDirectories: %v", err)
	}
	if _, err := os.Stat(base); !os.IsNotExist(err) {
		t.Errorf("base dir still exists after cleanup: err = %v", err)
	}
}

// TestPathHelpers verifies each Get*Path helper joins onto the correct
// parent directory with the expected suffix.
func TestPathHelpers(t *testing.T) {
	paths := &DirectoryPaths{
		Config: "/config",
		Data:   "/data",
		Log:    "/log",
		Cache:  "/cache",
	}

	tests := []struct {
		name string
		got  string
		want string
	}{
		{"GetDatabasePath", GetDatabasePath(paths), filepath.Join("/data", "db", "server.db")},
		{"GetDatabaseDir", GetDatabaseDir(paths), filepath.Join("/data", "db")},
		{"GetBackupPath", GetBackupPath(paths), filepath.Join("/data", "backups")},
		{"GetCertsPath", GetCertsPath(paths), filepath.Join("/config", "certs")},
		{"GetConfigDatabasesPath", GetConfigDatabasesPath(paths), filepath.Join("/config", "databases")},
		{"GetGeoIPPath", GetGeoIPPath(paths), filepath.Join("/config", "databases", "geoip.mmdb")},
		{"GetAirportDataPath", GetAirportDataPath(paths), filepath.Join("/config", "databases", "airports.json")},
		{"GetWeatherCachePath", GetWeatherCachePath(paths), filepath.Join("/cache", "wthr")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.got != tt.want {
				t.Errorf("%s = %q, want %q", tt.name, tt.got, tt.want)
			}
		})
	}
}

// TestGetTempPath verifies it returns os.TempDir() directly.
func TestGetTempPath(t *testing.T) {
	if got := GetTempPath(); got != os.TempDir() {
		t.Errorf("GetTempPath() = %q, want %q", got, os.TempDir())
	}
}

// TestGetDirectoryPaths_ReturnsRootedPaths is a smoke test: on the current
// (Linux) platform, GetDirectoryPaths must succeed and return non-empty,
// absolute paths for all four fields, whether running as root or not.
func TestGetDirectoryPaths_ReturnsRootedPaths(t *testing.T) {
	paths, err := GetDirectoryPaths()
	if err != nil {
		t.Fatalf("GetDirectoryPaths: %v", err)
	}
	for name, p := range map[string]string{
		"Config": paths.Config,
		"Data":   paths.Data,
		"Log":    paths.Log,
		"Cache":  paths.Cache,
	} {
		if p == "" {
			t.Errorf("%s is empty", name)
		}
		if !filepath.IsAbs(p) {
			t.Errorf("%s = %q, want absolute path", name, p)
		}
	}
}
