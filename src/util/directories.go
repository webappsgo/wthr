package utils

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
)

// DirectoryPaths holds all application directory paths
type DirectoryPaths struct {
	// Configuration directory
	Config string
	// Data directory (database, user files)
	Data   string
	// Log directory
	Log    string
	// Cache directory
	Cache  string
}

// GetDirectoryPaths returns the appropriate directory paths based on privileges and OS
func GetDirectoryPaths() (*DirectoryPaths, error) {
	// TEMPLATE.md PART 3: Use {projectorg}/{projectname} pattern
	// This creates paths like /etc/apimgr/weather/ instead of /etc/weather/
	projectOrg := "apimgr"
	projectName := "weather"
	projectPath := filepath.Join(projectOrg, projectName)

	// Check if running as root/administrator
	hasRoot := isRoot()

	var paths DirectoryPaths

	switch runtime.GOOS {
	case "linux", "freebsd", "openbsd", "netbsd":
		if hasRoot {
			paths = DirectoryPaths{
				Config: filepath.Join("/etc", projectPath),
				Data:   filepath.Join("/var/lib", projectPath),
				Log:    filepath.Join("/var/log", projectPath),
				Cache:  filepath.Join("/var/cache", projectPath),
			}
		} else {
			homeDir, err := os.UserHomeDir()
			if err != nil {
				return nil, fmt.Errorf("failed to get home directory: %w", err)
			}
			paths = DirectoryPaths{
				Config: filepath.Join(homeDir, ".config", projectPath),
				Data:   filepath.Join(homeDir, ".local", "share", projectPath),
				Log:    filepath.Join(homeDir, ".local", "share", projectPath, "logs"),
				Cache:  filepath.Join(homeDir, ".cache", projectPath),
			}
		}

	// macOS
	case "darwin":
		if hasRoot {
			paths = DirectoryPaths{
				Config: filepath.Join("/Library/Application Support", projectPath),
				Data:   filepath.Join("/Library/Application Support", projectPath, "data"),
				Log:    filepath.Join("/Library/Logs", projectPath),
				Cache:  filepath.Join("/Library/Caches", projectPath),
			}
		} else {
			homeDir, err := os.UserHomeDir()
			if err != nil {
				return nil, fmt.Errorf("failed to get home directory: %w", err)
			}
			paths = DirectoryPaths{
				Config: filepath.Join(homeDir, "Library", "Application Support", projectPath),
				Data:   filepath.Join(homeDir, "Library", "Application Support", projectPath, "data"),
				Log:    filepath.Join(homeDir, "Library", "Logs", projectPath),
				Cache:  filepath.Join(homeDir, "Library", "Caches", projectPath),
			}
		}

	case "windows":
		appData := os.Getenv("APPDATA")
		if appData == "" {
			homeDir, err := os.UserHomeDir()
			if err != nil {
				return nil, fmt.Errorf("failed to get home directory: %w", err)
			}
			appData = filepath.Join(homeDir, "AppData", "Roaming")
		}

		localAppData := os.Getenv("LOCALAPPDATA")
		if localAppData == "" {
			homeDir, err := os.UserHomeDir()
			if err != nil {
				return nil, fmt.Errorf("failed to get home directory: %w", err)
			}
			localAppData = filepath.Join(homeDir, "AppData", "Local")
		}

		paths = DirectoryPaths{
			Config: filepath.Join(appData, projectPath),
			Data:   filepath.Join(localAppData, projectPath, "data"),
			Log:    filepath.Join(localAppData, projectPath, "logs"),
			Cache:  filepath.Join(localAppData, projectPath, "cache"),
		}

	default:
		return nil, fmt.Errorf("unsupported operating system: %s", runtime.GOOS)
	}

	return &paths, nil
}

// CreateDirectories creates all required directories with appropriate permissions
// AI.md PART 7: Permissions - root: 0755, user: 0700
func CreateDirectories(paths *DirectoryPaths) error {
	dirs := []string{
		paths.Config,
		paths.Data,
		paths.Log,
		paths.Cache,
		// Config subdirectories
		GetCertsPath(paths),
		GetConfigDatabasesPath(paths),
		// Data subdirectories
		GetDatabaseDir(paths),
		GetBackupPath(paths),
		// Cache subdirectories
		GetWeatherCachePath(paths),
	}

	// Determine permissions based on privilege level
	// Root: 0755 (world-readable), User: 0700 (private)
	perm := os.FileMode(0700)
	if isRoot() {
		perm = 0755
	}

	for _, dir := range dirs {
		if err := os.MkdirAll(dir, perm); err != nil {
			return fmt.Errorf("failed to create directory %s: %w", dir, err)
		}
		// Enforce permissions even if directory already existed
		if err := os.Chmod(dir, perm); err != nil {
			return fmt.Errorf("failed to set permissions on %s: %w", dir, err)
		}
	}

	return nil
}

// isRoot checks if the current process is running with root/administrator privileges
func isRoot() bool {
	if runtime.GOOS == "windows" {
		// On Windows, check if we can write to system directories
		testPath := filepath.Join(os.Getenv("SystemRoot"), "test_admin_access.tmp")
		file, err := os.Create(testPath)
		if err != nil {
			return false
		}
		file.Close()
		os.Remove(testPath)
		return true
	}

	// On Unix-like systems, check if UID is 0
	return os.Geteuid() == 0
}

// GetDatabasePath returns the full path to the SQLite database file
func GetDatabasePath(paths *DirectoryPaths) string {
	return filepath.Join(paths.Data, "db", "weather.db")
}

// GetDatabaseDir returns the directory containing database files
func GetDatabaseDir(paths *DirectoryPaths) string {
	return filepath.Join(paths.Data, "db")
}

// GetBackupPath returns the full path to the backup directory
func GetBackupPath(paths *DirectoryPaths) string {
	return filepath.Join(paths.Data, "backups")
}

// GetCertsPath returns the full path to the SSL certificates directory
func GetCertsPath(paths *DirectoryPaths) string {
	return filepath.Join(paths.Config, "certs")
}

// GetConfigDatabasesPath returns the full path to config databases (GeoIP, JSON, etc.)
func GetConfigDatabasesPath(paths *DirectoryPaths) string {
	return filepath.Join(paths.Config, "databases")
}

// GetGeoIPPath returns the full path to the GeoIP database
func GetGeoIPPath(paths *DirectoryPaths) string {
	return filepath.Join(paths.Config, "databases", "geoip.mmdb")
}

// GetAirportDataPath returns the full path to the airport database JSON
func GetAirportDataPath(paths *DirectoryPaths) string {
	return filepath.Join(paths.Config, "databases", "airports.json")
}

// GetWeatherCachePath returns the full path to weather cache directory
func GetWeatherCachePath(paths *DirectoryPaths) string {
	return filepath.Join(paths.Cache, "weather")
}

// GetTempPath returns the full path to temporary files directory
// Use this only for truly temporary files (should be cleaned up immediately)
// For persistent cache, use Cache directory instead
func GetTempPath() string {
	return os.TempDir()
}

// GetTestDirectoryPaths returns directory paths for testing (uses temp directory)
// This should be used by all tests to avoid polluting system directories
func GetTestDirectoryPaths() (*DirectoryPaths, error) {
	tempBase := filepath.Join(os.TempDir(), "apimgr-weather-test")

	paths := &DirectoryPaths{
		Config: filepath.Join(tempBase, "config"),
		Data:   filepath.Join(tempBase, "data"),
		Log:    filepath.Join(tempBase, "logs"),
		Cache:  filepath.Join(tempBase, "cache"),
	}

	return paths, nil
}

// CleanupTestDirectories removes test directories
func CleanupTestDirectories(paths *DirectoryPaths) error {
	// Remove the parent temp directory
	tempBase := filepath.Dir(paths.Config)
	return os.RemoveAll(tempBase)
}
