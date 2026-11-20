// Package paths provides OS-specific directory path detection per TEMPLATE.md PART 3
// Supports: Linux, macOS, BSD (FreeBSD/OpenBSD/NetBSD), Windows
// Handles both privileged (root/service) and unprivileged (user) scenarios
package path

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"

	"github.com/webappsgo/wthr/src/privilege"
	"github.com/webappsgo/wthr/src/util"
)

var (
	// defaultPaths is the global paths instance
	defaultPaths *Paths
	// initOnce ensures paths are initialized only once
	initOnce sync.Once
)

// Paths holds OS-specific directory paths
type Paths struct {
	DataDir   string
	ConfigDir string
	LogDir    string
	CacheDir  string
	TempDir   string

	// Additional subdirectories
	SSLDir       string
	TorDir       string
	GeoIPDir     string
	BackupDir    string
	BlocklistDir string

	// Runtime info
	IsPrivileged bool
	GOOS         string
	AppName      string
}

// GetDefaultPaths returns the AI.md PART 4 directories for this OS and the mode decided at
// process start. The per-OS tables live in util.DefaultDirectories, the one place they are
// defined.
func GetDefaultPaths(appName string) *Paths {
	home, _ := os.UserHomeDir()
	systemMode := util.IsSystemMode()
	dirs := util.DefaultDirectories(runtime.GOOS, systemMode, home, os.Getenv, privilege.InternalOrg, appName)

	return &Paths{
		DataDir:      dirs.Data,
		ConfigDir:    dirs.Config,
		LogDir:       dirs.Log,
		CacheDir:     dirs.Cache,
		TempDir:      filepath.Join(os.TempDir(), privilege.InternalOrg, appName),
		IsPrivileged: systemMode,
		GOOS:         runtime.GOOS,
		AppName:      appName,
	}
}

// EnsureDirectories creates all directories if they don't exist
// AI.md PART 7: Permissions - root: 0755, user: 0700
func (p *Paths) EnsureDirectories() error {
	dirs := []string{
		p.DataDir,
		p.ConfigDir,
		p.LogDir,
		p.CacheDir,
		p.TempDir,
	}

	for _, dir := range dirs {
		if err := EnsureDir(dir, p.IsPrivileged); err != nil {
			return err
		}
	}

	return nil
}

// EnsureDir creates directory with proper permissions if it doesn't exist
// AI.md PART 7: Permissions - root: 0755, user: 0700
func EnsureDir(path string, isRoot bool) error {
	perm := os.FileMode(0700)
	if isRoot {
		perm = 0755
	}

	if err := os.MkdirAll(path, perm); err != nil {
		return fmt.Errorf("create directory %s: %w", path, err)
	}

	// Enforce permissions even if dir already existed
	if err := os.Chmod(path, perm); err != nil {
		return fmt.Errorf("chmod directory %s: %w", path, err)
	}

	// Enforce ownership (current user) - skip on Windows
	if runtime.GOOS != "windows" {
		uid := os.Getuid()
		gid := os.Getgid()
		if err := os.Chown(path, uid, gid); err != nil {
			// Non-fatal if we don't own the directory (e.g., system dirs)
			// Only log, don't fail
		}
	}

	// Verify writable
	testFile := filepath.Join(path, ".write-test")
	if err := os.WriteFile(testFile, []byte{}, 0600); err != nil {
		return fmt.Errorf("directory %s is not writable: %w", path, err)
	}
	os.Remove(testFile)

	return nil
}

// EnsureFile creates/updates a file with correct permissions
// AI.md PART 7: Permissions - root: 0644, user: 0600
func EnsureFile(path string, content []byte, isRoot bool) error {
	perm := os.FileMode(0600)
	if isRoot {
		perm = 0644
	}

	// Ensure parent directory exists
	dir := filepath.Dir(path)
	if err := EnsureDir(dir, isRoot); err != nil {
		return fmt.Errorf("create parent directory: %w", err)
	}

	// Write file with correct permissions
	if err := os.WriteFile(path, content, perm); err != nil {
		return fmt.Errorf("write file %s: %w", path, err)
	}

	// Enforce permissions (in case file existed with wrong perms)
	if err := os.Chmod(path, perm); err != nil {
		return fmt.Errorf("chmod file %s: %w", path, err)
	}

	// Enforce ownership (current user) - skip on Windows
	if runtime.GOOS != "windows" {
		uid := os.Getuid()
		gid := os.Getgid()
		if err := os.Chown(path, uid, gid); err != nil {
			// Non-fatal if we don't own the file
		}
	}

	return nil
}

// EnsurePIDFile creates PID file directory and validates path
// AI.md PART 7: PID file permissions - root: 0644, user: 0600
func EnsurePIDFile(path string, isRoot bool) error {
	dir := filepath.Dir(path)
	return EnsureDir(dir, isRoot)
}

// Override allows overriding paths via environment variables
func (p *Paths) Override() {
	if dataDir := os.Getenv("DATA_DIR"); dataDir != "" {
		p.DataDir = dataDir
	}
	if configDir := os.Getenv("CONFIG_DIR"); configDir != "" {
		p.ConfigDir = configDir
	}
	if logDir := os.Getenv("LOG_DIR"); logDir != "" {
		p.LogDir = logDir
	}
	if cacheDir := os.Getenv("CACHE_DIR"); cacheDir != "" {
		p.CacheDir = cacheDir
	}
	if tempDir := os.Getenv("TEMP_DIR"); tempDir != "" {
		p.TempDir = tempDir
	}
	// BACKUP_DIR overrides the derived {data_dir}/backups default set by
	// initializeSubdirectories, so it must be applied after that call -
	// see Initialize, which runs initializeSubdirectories() before Override().
	if backupDir := os.Getenv("BACKUP_DIR"); backupDir != "" {
		p.BackupDir = backupDir
	}
}

// initializeSubdirectories sets up additional directory paths
// AI.md PART 4: SSL and security directories go in ConfigDir (user-editable)
// Tor, backups, and runtime data go in DataDir (application-managed)
func (p *Paths) initializeSubdirectories() {
	// ConfigDir: User-editable configuration and security data
	p.SSLDir = filepath.Join(p.ConfigDir, "ssl")
	p.GeoIPDir = filepath.Join(p.DataDir, "security", "geoip")
	p.BlocklistDir = filepath.Join(p.DataDir, "security", "blocklists")

	// DataDir: Application-managed runtime data
	p.TorDir = filepath.Join(p.DataDir, "tor")
	home, _ := os.UserHomeDir()
	p.BackupDir = util.ResolveBackupDir(runtime.GOOS, util.IsSystemMode(), home, os.Getenv, util.IsDirWritable,
		privilege.InternalOrg, p.AppName, p.DataDir)
	p.GOOS = runtime.GOOS
}

// Initialize sets up the global paths instance
func Initialize(appName string) error {
	var initErr error
	initOnce.Do(func() {
		defaultPaths = GetDefaultPaths(appName)
		defaultPaths.AppName = appName
		defaultPaths.IsPrivileged = util.IsSystemMode()
		defaultPaths.initializeSubdirectories()
		defaultPaths.Override()

		// Create directories
		if err := defaultPaths.EnsureDirectories(); err != nil {
			initErr = fmt.Errorf("failed to create directories: %w", err)
			return
		}
	})
	return initErr
}

// GetInstance returns the global paths instance
func GetInstance() *Paths {
	if defaultPaths == nil {
		// Initialize with default app name if not already initialized
		_ = Initialize("wthr")
	}
	return defaultPaths
}

// Global helper functions for easy access

// GetDataDir returns the data directory path
func GetDataDir() string {
	return GetInstance().DataDir
}

// GetConfigDir returns the config directory path
func GetConfigDir() string {
	return GetInstance().ConfigDir
}

// GetLogDir returns the log directory path
func GetLogDir() string {
	return GetInstance().LogDir
}

// GetCacheDir returns the cache directory path
func GetCacheDir() string {
	return GetInstance().CacheDir
}

// GetTempDir returns the temp directory path
func GetTempDir() string {
	return GetInstance().TempDir
}

// GetSSLDir returns the SSL directory path
func GetSSLDir() string {
	return GetInstance().SSLDir
}

// GetTorDir returns the Tor directory path
func GetTorDir() string {
	return GetInstance().TorDir
}

// GetGeoIPDir returns the GeoIP directory path
func GetGeoIPDir() string {
	return GetInstance().GeoIPDir
}

// GetBackupDir returns the backup directory path
func GetBackupDir() string {
	return GetInstance().BackupDir
}

// GetBlocklistDir returns the blocklist directory path
func GetBlocklistDir() string {
	return GetInstance().BlocklistDir
}

// IsPrivileged returns whether the application is running with elevated privileges
func IsPrivileged() bool {
	return GetInstance().IsPrivileged
}

// ResolvePath resolves path variables like {data_dir}, {config_dir}, {log_dir}
// This is used extensively in server.yml configuration
// Example: "{data_dir}/server.db" -> "/var/lib/weather/server.db"
func ResolvePath(path string) string {
	if path == "" {
		return ""
	}

	instance := GetInstance()

	// Replace path variables
	replacements := map[string]string{
		"{data_dir}":      instance.DataDir,
		"{config_dir}":    instance.ConfigDir,
		"{log_dir}":       instance.LogDir,
		"{cache_dir}":     instance.CacheDir,
		"{temp_dir}":      instance.TempDir,
		"{ssl_dir}":       instance.SSLDir,
		"{tor_dir}":       instance.TorDir,
		"{geoip_dir}":     instance.GeoIPDir,
		"{backup_dir}":    instance.BackupDir,
		"{blocklist_dir}": instance.BlocklistDir,
	}

	result := path
	for placeholder, value := range replacements {
		result = strings.ReplaceAll(result, placeholder, value)
	}

	// Clean the path
	return filepath.Clean(result)
}

// ResolveConfigPath resolves a path and checks config directory first
// Used for finding config files like server.yml
func ResolveConfigPath(filename string) string {
	// If absolute path, use it directly
	if filepath.IsAbs(filename) {
		return filename
	}

	instance := GetInstance()

	// Try config directory first
	configPath := filepath.Join(instance.ConfigDir, filename)
	if _, err := os.Stat(configPath); err == nil {
		return configPath
	}

	// Try current directory
	if _, err := os.Stat(filename); err == nil {
		return filename
	}

	// Default to config directory (even if doesn't exist yet)
	return configPath
}

// GetConfigFilePath returns the full path to server.yml
func GetConfigFilePath() string {
	return ResolveConfigPath("server.yml")
}

// EnsureAllDirectories creates all standard directories including subdirectories
// AI.md PART 7: Permissions - root: 0755, user: 0700
func (p *Paths) EnsureAllDirectories() error {
	dirs := []string{
		p.DataDir,
		p.ConfigDir,
		p.LogDir,
		p.CacheDir,
		p.TempDir,
		p.SSLDir,
		p.TorDir,
		p.GeoIPDir,
		p.BackupDir,
		p.BlocklistDir,
		// SSL subdirectories
		filepath.Join(p.SSLDir, "letsencrypt"),
		filepath.Join(p.SSLDir, "local"),
		// Tor subdirectories
		filepath.Join(p.TorDir, "site"),
	}

	for _, dir := range dirs {
		if err := EnsureDir(dir, p.IsPrivileged); err != nil {
			return fmt.Errorf("failed to create directory %s: %w", dir, err)
		}
	}

	return nil
}

// PrintPaths prints all configured paths (useful for --status command)
func (p *Paths) PrintPaths() {
	fmt.Printf("Paths Configuration:\n")
	fmt.Printf("  OS:           %s\n", p.GOOS)
	fmt.Printf("  Privileged:   %t\n", p.IsPrivileged)
	fmt.Printf("  App Name:     %s\n", p.AppName)
	fmt.Printf("  Config:       %s\n", p.ConfigDir)
	fmt.Printf("  Data:         %s\n", p.DataDir)
	fmt.Printf("  Logs:         %s\n", p.LogDir)
	fmt.Printf("  Cache:        %s\n", p.CacheDir)
	fmt.Printf("  Temp:         %s\n", p.TempDir)
	fmt.Printf("  SSL:          %s\n", p.SSLDir)
	fmt.Printf("  Tor:          %s\n", p.TorDir)
	fmt.Printf("  GeoIP:        %s\n", p.GeoIPDir)
	fmt.Printf("  Backups:      %s\n", p.BackupDir)
	fmt.Printf("  Blocklists:   %s\n", p.BlocklistDir)
}
