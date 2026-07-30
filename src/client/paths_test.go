package main

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestCLIConfigDir(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix-specific path test")
	}

	origHome := os.Getenv("HOME")
	defer os.Setenv("HOME", origHome)

	home := t.TempDir()
	os.Setenv("HOME", home)

	got := CLIConfigDir()
	want := filepath.Join(home, ".config", projectOrg, projectName)
	if got != want {
		t.Errorf("CLIConfigDir() = %s, want %s", got, want)
	}
}

func TestCLIDataCacheLogDirs(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix-specific path test")
	}

	origHome := os.Getenv("HOME")
	defer os.Setenv("HOME", origHome)

	home := t.TempDir()
	os.Setenv("HOME", home)

	tests := []struct {
		name string
		fn   func() string
		want string
	}{
		{"data", CLIDataDir, filepath.Join(home, ".local", "share", projectOrg, projectName)},
		{"cache", CLICacheDir, filepath.Join(home, ".cache", projectOrg, projectName)},
		{"log", CLILogDir, filepath.Join(home, ".local", "log", projectOrg, projectName)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.fn(); got != tt.want {
				t.Errorf("%s() = %s, want %s", tt.name, got, tt.want)
			}
		})
	}
}

func TestCLIFilePaths(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix-specific path test")
	}

	origHome := os.Getenv("HOME")
	defer os.Setenv("HOME", origHome)

	home := t.TempDir()
	os.Setenv("HOME", home)

	tests := []struct {
		name string
		fn   func() string
		base string
	}{
		{"config file", CLIConfigFile, "cli.yml"},
		{"log file", CLILogFile, "cli.log"},
		{"token file", CLITokenFile, "token"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.fn()
			if filepath.Base(got) != tt.base {
				t.Errorf("%s base = %s, want %s", tt.name, filepath.Base(got), tt.base)
			}
			if !filepath.IsAbs(got) {
				t.Errorf("%s is not absolute: %s", tt.name, got)
			}
		})
	}
}

func TestEnsureDirs(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix permission semantics differ on Windows")
	}

	origHome := os.Getenv("HOME")
	defer os.Setenv("HOME", origHome)

	home := t.TempDir()
	os.Setenv("HOME", home)

	if err := EnsureDirs(); err != nil {
		t.Fatalf("EnsureDirs() failed: %v", err)
	}

	dirs := []string{CLIConfigDir(), CLIDataDir(), CLICacheDir(), CLILogDir()}
	for _, dir := range dirs {
		info, err := os.Stat(dir)
		if err != nil {
			t.Fatalf("expected dir %s to exist: %v", dir, err)
		}
		if !info.IsDir() {
			t.Errorf("expected %s to be a directory", dir)
		}
		if perm := info.Mode().Perm(); perm != 0700 {
			t.Errorf("expected %s to have perm 0700, got %o", dir, perm)
		}
	}

	// Idempotency: calling EnsureDirs again on already-existing dirs must not error.
	if err := EnsureDirs(); err != nil {
		t.Fatalf("EnsureDirs() second call failed: %v", err)
	}
}

func TestEnsureFile(t *testing.T) {
	tempDir := t.TempDir()
	target := filepath.Join(tempDir, "nested", "sub", "file.txt")

	if err := EnsureFile(target); err != nil {
		t.Fatalf("EnsureFile() failed: %v", err)
	}

	info, err := os.Stat(filepath.Dir(target))
	if err != nil {
		t.Fatalf("expected parent dir to exist: %v", err)
	}
	if !info.IsDir() {
		t.Errorf("expected parent to be a directory")
	}

	// The file itself is not created by EnsureFile - only parent dirs.
	if _, err := os.Stat(target); !os.IsNotExist(err) {
		t.Errorf("expected file to not exist yet, got err=%v", err)
	}
}
