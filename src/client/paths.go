package main

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
)

// Path functions per AI.md PART 33 line 45113-45175
// Platform-specific directory paths for CLI configuration

const (
	projectOrg  = "webappsgo"
	projectName = "wthr"
)

// CLIConfigDir returns the CLI config directory
// Per AI.md PART 33: ~/.config/webappsgo/wthr/ (Unix) or %APPDATA%\webappsgo\weather\ (Windows)
func CLIConfigDir() string {
	if runtime.GOOS == "windows" {
		return filepath.Join(os.Getenv("APPDATA"), projectOrg, projectName)
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", projectOrg, projectName)
}

// CLIDataDir returns the CLI data directory
// Per AI.md PART 33: ~/.local/share/webappsgo/wthr/ (Unix) or %LOCALAPPDATA%\webappsgo\weather\data\ (Windows)
func CLIDataDir() string {
	if runtime.GOOS == "windows" {
		return filepath.Join(os.Getenv("LOCALAPPDATA"), projectOrg, projectName, "data")
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".local", "share", projectOrg, projectName)
}

// CLICacheDir returns the CLI cache directory
// Per AI.md PART 33: ~/.cache/webappsgo/wthr/ (Unix) or %LOCALAPPDATA%\webappsgo\weather\cache\ (Windows)
func CLICacheDir() string {
	if runtime.GOOS == "windows" {
		return filepath.Join(os.Getenv("LOCALAPPDATA"), projectOrg, projectName, "cache")
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".cache", projectOrg, projectName)
}

// CLILogDir returns the CLI log directory
// Per AI.md PART 33: ~/.local/log/webappsgo/wthr/ (Unix) or %LOCALAPPDATA%\webappsgo\weather\log\ (Windows)
func CLILogDir() string {
	if runtime.GOOS == "windows" {
		return filepath.Join(os.Getenv("LOCALAPPDATA"), projectOrg, projectName, "log")
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".local", "log", projectOrg, projectName)
}

// CLIConfigFile returns the CLI config file path
func CLIConfigFile() string {
	return filepath.Join(CLIConfigDir(), "cli.yml")
}

// CLILogFile returns the CLI log file path
func CLILogFile() string {
	return filepath.Join(CLILogDir(), "cli.log")
}

// CLITokenFile returns the CLI token file path
func CLITokenFile() string {
	return filepath.Join(CLIConfigDir(), "token")
}

// EnsureDirs creates all CLI directories with correct permissions
// Per AI.md PART 33 line 45006-45059: Called on every startup before any file operations
func EnsureDirs() error {
	dirs := []string{
		CLIConfigDir(),
		CLIDataDir(),
		CLICacheDir(),
		CLILogDir(),
	}

	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0700); err != nil {
			return NewConfigError(fmt.Sprintf("failed to create directory %s: %v", dir, err))
		}
		// Set permissions (platform-specific)
		if err := setDirPermissions(dir); err != nil {
			return NewConfigError(fmt.Sprintf("failed to set permissions on %s: %v", dir, err))
		}
	}
	return nil
}

// EnsureFile creates parent dirs and sets permissions before writing
// Per AI.md PART 33 line 45095-45103
func EnsureFile(path string) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("create parent dir: %w", err)
	}
	return nil
}
