package util

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"github.com/webappsgo/wthr/src/privilege"
)

// DirectoryPaths holds all application directory paths
type DirectoryPaths struct {
	// Configuration directory
	Config string
	// Data directory (database, user files)
	Data string
	// Log directory
	Log string
	// Cache directory
	Cache string
	// Backup directory (AI.md PART 8 GetBackupDir); {data_dir}/backup when empty
	Backup string
	// SQLite database directory; {data_dir}/db when empty
	Database string
}

// DefaultDirectories returns the platform directories of AI.md PART 4 for one organization
// and project name. systemMode selects the privileged table (root, Administrator or a
// service) over the user table; home and env are the user's home directory and an
// environment lookup. Backup and Database are left empty: they depend on the host.
func DefaultDirectories(goos string, systemMode bool, home string, env func(string) string, org, name string) DirectoryPaths {
	project := filepath.Join(org, name)

	switch goos {
	case "darwin":
		if systemMode {
			support := filepath.Join("/Library/Application Support", project)
			return DirectoryPaths{
				Config: support,
				Data:   filepath.Join(support, "data"),
				Cache:  filepath.Join("/Library/Caches", project),
				Log:    filepath.Join("/Library/Logs", project),
			}
		}
		support := filepath.Join(home, "Library", "Application Support", project)
		return DirectoryPaths{
			Config: support,
			Data:   support,
			Cache:  filepath.Join(home, "Library", "Caches", project),
			Log:    filepath.Join(home, "Library", "Logs", project),
		}
	case "windows":
		if systemMode {
			base := filepath.Join(windowsEnvDir(env, "ProgramData", `C:\ProgramData`), project)
			return DirectoryPaths{
				Config: base,
				Data:   filepath.Join(base, "data"),
				Cache:  filepath.Join(base, "cache"),
				Log:    filepath.Join(base, "logs"),
			}
		}
		roaming := windowsEnvDir(env, "AppData", filepath.Join(home, "AppData", "Roaming"))
		local := filepath.Join(windowsEnvDir(env, "LocalAppData", filepath.Join(home, "AppData", "Local")), project)
		return DirectoryPaths{
			Config: filepath.Join(roaming, project),
			Data:   local,
			Cache:  filepath.Join(local, "cache"),
			Log:    filepath.Join(local, "logs"),
		}
	case "freebsd", "openbsd", "netbsd":
		if systemMode {
			return DirectoryPaths{
				Config: filepath.Join("/usr/local/etc", project),
				Data:   filepath.Join("/var/db", project),
				Cache:  filepath.Join("/var/cache", project),
				Log:    filepath.Join("/var/log", project),
			}
		}
	default:
		if systemMode {
			return DirectoryPaths{
				Config: filepath.Join("/etc", project),
				Data:   filepath.Join("/var/lib", project),
				Cache:  filepath.Join("/var/cache", project),
				Log:    filepath.Join("/var/log", project),
			}
		}
	}

	// Linux and BSD user mode
	return DirectoryPaths{
		Config: filepath.Join(home, ".config", project),
		Data:   filepath.Join(home, ".local", "share", project),
		Cache:  filepath.Join(home, ".cache", project),
		Log:    filepath.Join(home, ".local", "log", project),
	}
}

// windowsEnvDir returns a Windows directory environment variable, or the fallback when unset.
func windowsEnvDir(env func(string) string, key, fallback string) string {
	if value := env(key); value != "" {
		return value
	}

	return fallback
}

// SystemBackupDir is the system-level backup directory of this platform (AI.md PART 8).
func SystemBackupDir(goos string, env func(string) string, org, name string) string {
	switch goos {
	case "darwin":
		return filepath.Join("/Library/Backups", org, name)
	case "windows":
		return filepath.Join(windowsEnvDir(env, "ProgramData", `C:\ProgramData`), "Backups", org, name)
	case "freebsd", "openbsd", "netbsd":
		return filepath.Join("/var/backups", org, name)
	default:
		return filepath.Join("/mnt/Backups", org, name)
	}
}

// userBackupDir is the user-level backup directory. It is for user mode only: after a
// privilege drop $HOME belongs to the service account (AI.md PART 8 GetBackupDir).
func userBackupDir(goos, home string, env func(string) string, org, name string) string {
	switch goos {
	case "darwin":
		return filepath.Join(home, "Library", "Backups", org, name)
	case "windows":
		return filepath.Join(windowsEnvDir(env, "LocalAppData", filepath.Join(home, "AppData", "Local")), "Backups", org, name)
	default:
		return filepath.Join(home, ".local", "share", "Backups", org, name)
	}
}

// IsDirWritable reports whether a directory can be written or created: its parent must
// exist and accept a new file.
func IsDirWritable(dir string) bool {
	parent := filepath.Dir(dir)
	info, err := os.Stat(parent)
	if err != nil || !info.IsDir() {
		return false
	}

	probe, err := os.CreateTemp(parent, ".write_test_")
	if err != nil {
		return false
	}
	name := probe.Name()
	probe.Close()
	_ = os.Remove(name)

	return true
}

// ResolveBackupDir picks the backup directory (AI.md PART 8 GetBackupDir): BACKUP_DIR, then
// the system backup directory when writable, then {data_dir}/backup in system mode (never a
// $HOME path) or the user backup directory in user mode.
func ResolveBackupDir(goos string, systemMode bool, home string, env func(string) string, writable func(string) bool, org, name, dataDir string) string {
	if value := env("BACKUP_DIR"); value != "" {
		return value
	}

	system := SystemBackupDir(goos, env, org, name)
	if writable(system) {
		return system
	}

	if systemMode {
		return filepath.Join(dataDir, "backup")
	}

	return userBackupDir(goos, home, env, org, name)
}

// ResolveDatabaseDir picks the SQLite directory: DATABASE_DIR, then /data/db/sqlite in a
// container, then {data_dir}/db.
func ResolveDatabaseDir(env func(string) string, inContainer bool, dataDir string) string {
	if value := env("DATABASE_DIR"); value != "" {
		return value
	}
	if inContainer {
		return "/data/db/sqlite"
	}

	return filepath.Join(dataDir, "db")
}

// GetDirectoryPaths returns the application directories for this host: the platform table
// of AI.md PART 4 for the mode decided at process start, then the CONFIG_DIR, DATA_DIR,
// CACHE_DIR, LOG_DIR, DATABASE_DIR and BACKUP_DIR overrides (the --config, --data, --cache,
// --log and --backup flags set the same variables).
func GetDirectoryPaths() (*DirectoryPaths, error) {
	home, err := os.UserHomeDir()
	if err != nil && !IsSystemMode() {
		return nil, fmt.Errorf("failed to get home directory: %w", err)
	}

	paths := DefaultDirectories(runtime.GOOS, IsSystemMode(), home, os.Getenv, privilege.InternalOrg, privilege.InternalName)

	for target, key := range map[*string]string{
		&paths.Config: "CONFIG_DIR",
		&paths.Data:   "DATA_DIR",
		&paths.Cache:  "CACHE_DIR",
		&paths.Log:    "LOG_DIR",
	} {
		if value := os.Getenv(key); value != "" {
			*target = value
		}
	}

	paths.Database = ResolveDatabaseDir(os.Getenv, IsContainer(), paths.Data)
	paths.Backup = ResolveBackupDir(runtime.GOOS, IsSystemMode(), home, os.Getenv, IsDirWritable,
		privilege.InternalOrg, privilege.InternalName, paths.Data)

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

// isRoot reports whether the directory mode decided at process start is the system mode.
func isRoot() bool {
	return IsSystemMode()
}

// GetDatabasePath returns the full path to the SQLite database file
func GetDatabasePath(paths *DirectoryPaths) string {
	return filepath.Join(GetDatabaseDir(paths), "server.db")
}

// GetDatabaseDir returns the directory containing database files
func GetDatabaseDir(paths *DirectoryPaths) string {
	if paths.Database != "" {
		return paths.Database
	}

	return filepath.Join(paths.Data, "db")
}

// GetBackupPath returns the full path to the backup directory
func GetBackupPath(paths *DirectoryPaths) string {
	if paths.Backup != "" {
		return paths.Backup
	}

	return filepath.Join(paths.Data, "backup")
}

// GetCertsPath returns the full path to the SSL directory, {config_dir}/ssl (AI.md PART 15)
func GetCertsPath(paths *DirectoryPaths) string {
	return filepath.Join(paths.Config, "ssl")
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
	return filepath.Join(paths.Cache, "wthr")
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
	tempBase := filepath.Join(os.TempDir(), "webappsgo", "wthr-test")

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
