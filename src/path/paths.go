// Package paths provides OS-specific directory path detection per TEMPLATE.md PART 3
// Supports: Linux, macOS, BSD (FreeBSD/OpenBSD/NetBSD), Windows
// Handles both privileged (root/service) and unprivileged (user) scenarios
package paths

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
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

// GetDefaultPaths returns default directory paths for the current OS
func GetDefaultPaths(appName string) *Paths {
	switch runtime.GOOS {
	case "linux":
		return getLinuxPaths(appName)
	case "darwin":
		return getDarwinPaths(appName)
	case "windows":
		return getWindowsPaths(appName)
	case "freebsd", "openbsd", "netbsd":
		return getBSDPaths(appName)
	default:
		// Fallback to Linux paths
		return getLinuxPaths(appName)
	}
}

// getLinuxPaths returns Linux-specific paths (XDG Base Directory spec)
// Per AI.md PART 4: Paths must include organization namespace (apimgr/weather)
func getLinuxPaths(appName string) *Paths {
	homeDir, _ := os.UserHomeDir()

	// Organization-namespaced paths per AI.md specification
	orgNamespace := filepath.Join("apimgr", appName)

	// Check if running as root
	if os.Geteuid() == 0 {
		return &Paths{
			DataDir:   filepath.Join("/var/lib", orgNamespace),
			ConfigDir: filepath.Join("/etc", orgNamespace),
			LogDir:    filepath.Join("/var/log", orgNamespace),
			CacheDir:  filepath.Join("/var/cache", orgNamespace),
			TempDir:   filepath.Join("/tmp", appName),
			IsPrivileged: true,
			GOOS:      "linux",
			AppName:   appName,
		}
	}

	// User-level paths (XDG Base Directory)
	dataHome := os.Getenv("XDG_DATA_HOME")
	if dataHome == "" {
		dataHome = filepath.Join(homeDir, ".local", "share")
	}

	configHome := os.Getenv("XDG_CONFIG_HOME")
	if configHome == "" {
		configHome = filepath.Join(homeDir, ".config")
	}

	cacheHome := os.Getenv("XDG_CACHE_HOME")
	if cacheHome == "" {
		cacheHome = filepath.Join(homeDir, ".cache")
	}

	return &Paths{
		DataDir:   filepath.Join(dataHome, orgNamespace),
		ConfigDir: filepath.Join(configHome, orgNamespace),
		LogDir:    filepath.Join(dataHome, orgNamespace, "logs"),
		CacheDir:  filepath.Join(cacheHome, orgNamespace),
		TempDir:   filepath.Join(os.TempDir(), appName),
		IsPrivileged: false,
		GOOS:      "linux",
		AppName:   appName,
	}
}

// getDarwinPaths returns macOS-specific paths
// Per AI.md PART 4: macOS paths must include organization namespace
func getDarwinPaths(appName string) *Paths {
	homeDir, _ := os.UserHomeDir()

	// Organization-namespaced paths per AI.md specification
	orgNamespace := filepath.Join("apimgr", appName)

	// Check if running as root or system service
	if os.Geteuid() == 0 {
		return &Paths{
			DataDir:   filepath.Join("/Library", "Application Support", orgNamespace, "data"),
			ConfigDir: filepath.Join("/Library", "Application Support", orgNamespace),
			LogDir:    filepath.Join("/Library", "Logs", orgNamespace),
			CacheDir:  filepath.Join("/Library", "Caches", orgNamespace),
			TempDir:   filepath.Join(os.TempDir(), appName),
			IsPrivileged: true,
			GOOS:      "darwin",
			AppName:   appName,
		}
	}

	// User-level paths
	return &Paths{
		DataDir:   filepath.Join(homeDir, "Library", "Application Support", orgNamespace),
		ConfigDir: filepath.Join(homeDir, "Library", "Application Support", orgNamespace),
		LogDir:    filepath.Join(homeDir, "Library", "Logs", orgNamespace),
		CacheDir:  filepath.Join(homeDir, "Library", "Caches", orgNamespace),
		TempDir:   filepath.Join(os.TempDir(), appName),
		IsPrivileged: false,
		GOOS:      "darwin",
		AppName:   appName,
	}
}

// getWindowsPaths returns Windows-specific paths
// Per AI.md PART 4: Windows paths must include organization namespace
func getWindowsPaths(appName string) *Paths {
	programData := os.Getenv("PROGRAMDATA")
	if programData == "" {
		programData = "C:\\ProgramData"
	}

	// Organization-namespaced paths per AI.md specification
	orgNamespace := filepath.Join("apimgr", appName)

	// Check if running as SYSTEM or Administrator (service mode)
	// In service mode, use ProgramData for all paths
	if os.Getenv("USERDOMAIN") == "NT AUTHORITY" || os.Getenv("USERNAME") == "SYSTEM" {
		return &Paths{
			DataDir:   filepath.Join(programData, orgNamespace, "data"),
			ConfigDir: filepath.Join(programData, orgNamespace),
			LogDir:    filepath.Join(programData, orgNamespace, "logs"),
			CacheDir:  filepath.Join(programData, orgNamespace, "cache"),
			TempDir:   filepath.Join(os.TempDir(), appName),
			IsPrivileged: true,
			GOOS:      "windows",
			AppName:   appName,
		}
	}

	// User-level paths
	appData := os.Getenv("APPDATA")
	if appData == "" {
		appData = filepath.Join(os.Getenv("USERPROFILE"), "AppData", "Roaming")
	}

	localAppData := os.Getenv("LOCALAPPDATA")
	if localAppData == "" {
		localAppData = filepath.Join(os.Getenv("USERPROFILE"), "AppData", "Local")
	}

	return &Paths{
		DataDir:   filepath.Join(localAppData, orgNamespace),
		ConfigDir: filepath.Join(appData, orgNamespace),
		LogDir:    filepath.Join(localAppData, orgNamespace, "logs"),
		CacheDir:  filepath.Join(localAppData, orgNamespace, "cache"),
		TempDir:   filepath.Join(os.TempDir(), appName),
		IsPrivileged: false,
		GOOS:      "windows",
		AppName:   appName,
	}
}

// getBSDPaths returns BSD-specific paths (similar to Linux)
// Per AI.md PART 4: BSD paths must include organization namespace
func getBSDPaths(appName string) *Paths {
	homeDir, _ := os.UserHomeDir()

	// Organization-namespaced paths per AI.md specification
	orgNamespace := filepath.Join("apimgr", appName)

	// Check if running as root
	if os.Geteuid() == 0 {
		return &Paths{
			DataDir:   filepath.Join("/var/db", orgNamespace),
			ConfigDir: filepath.Join("/usr/local/etc", orgNamespace),
			LogDir:    filepath.Join("/var/log", orgNamespace),
			CacheDir:  filepath.Join("/var/cache", orgNamespace),
			TempDir:   filepath.Join("/tmp", appName),
			IsPrivileged: true,
			GOOS:      runtime.GOOS,
			AppName:   appName,
		}
	}

	// User-level paths
	return &Paths{
		DataDir:   filepath.Join(homeDir, ".local", "share", orgNamespace),
		ConfigDir: filepath.Join(homeDir, ".config", orgNamespace),
		LogDir:    filepath.Join(homeDir, ".local", "share", orgNamespace, "logs"),
		CacheDir:  filepath.Join(homeDir, ".cache", orgNamespace),
		TempDir:   filepath.Join(os.TempDir(), appName),
		IsPrivileged: false,
		GOOS:      runtime.GOOS,
		AppName:   appName,
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
}

// initializeSubdirectories sets up additional directory paths
// AI.md PART 4: SSL and security directories go in ConfigDir (user-editable)
// Tor, backups, and runtime data go in DataDir (application-managed)
func (p *Paths) initializeSubdirectories() {
	// ConfigDir: User-editable configuration and security data
	p.SSLDir = filepath.Join(p.ConfigDir, "ssl")
	p.GeoIPDir = filepath.Join(p.ConfigDir, "security", "geoip")
	p.BlocklistDir = filepath.Join(p.ConfigDir, "security", "blocklists")

	// DataDir: Application-managed runtime data
	p.TorDir = filepath.Join(p.DataDir, "tor")
	p.BackupDir = filepath.Join(p.DataDir, "backups")
	p.GOOS = runtime.GOOS
}

// isPrivileged checks if running with elevated privileges
func isPrivileged() bool {
	switch runtime.GOOS {
	case "linux", "darwin", "freebsd", "openbsd", "netbsd":
		return os.Geteuid() == 0
	case "windows":
		// On Windows, check if running as SYSTEM or Administrator
		// This is a simplified check; proper Windows privilege check would use syscall
		return os.Getenv("USERDOMAIN") == "NT AUTHORITY" || os.Getenv("USERNAME") == "SYSTEM"
	default:
		return false
	}
}

// Initialize sets up the global paths instance
func Initialize(appName string) error {
	var initErr error
	initOnce.Do(func() {
		defaultPaths = GetDefaultPaths(appName)
		defaultPaths.AppName = appName
		defaultPaths.IsPrivileged = isPrivileged()
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
		_ = Initialize("weather")
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
