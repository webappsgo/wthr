package handler

import (
	"context"
	"database/sql"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/webappsgo/wthr/src/backup"
	"github.com/webappsgo/wthr/src/database"
	"github.com/webappsgo/wthr/src/path"
	"github.com/webappsgo/wthr/src/server/model"
	"github.com/webappsgo/wthr/src/server/service"
)

// BackupFile represents a backup file information
type BackupFile struct {
	Filename string    `json:"filename"`
	Size     int64     `json:"size"`
	Created  time.Time `json:"created"`
}

// SaveWebSettings handles saving web configuration settings
func SaveWebSettings(c *gin.Context) {
	var settings map[string]interface{}
	if err := c.ShouldBindJSON(&settings); err != nil {
		BadRequest(c, "Invalid request data")
		return
	}

	// Get database from context
	db, exists := c.Get("db")
	if !exists {
		InternalError(c, "Database connection not available")
		return
	}

	settingsModel := &model.SettingsModel{DB: db.(*sql.DB)}

	// Save each setting to database
	for key, value := range settings {
		var err error
		switch v := value.(type) {
		case string:
			err = settingsModel.SetString(key, v)
		case float64:
			err = settingsModel.SetInt(key, int(v))
		case bool:
			err = settingsModel.SetBool(key, v)
		default:
			err = settingsModel.SetJSON(key, v)
		}

		if err != nil {
			InternalError(c, fmt.Sprintf("Failed to save setting %s: %v", key, err))
			return
		}
	}

	RespondSuccess(c, "Web settings saved successfully")
}

// SaveSecuritySettings handles saving security configuration
func SaveSecuritySettings(c *gin.Context) {
	var settings map[string]interface{}
	if err := c.ShouldBindJSON(&settings); err != nil {
		BadRequest(c, "Invalid request data")
		return
	}

	// Get database from context
	db, exists := c.Get("db")
	if !exists {
		InternalError(c, "Database connection not available")
		return
	}

	settingsModel := &model.SettingsModel{DB: db.(*sql.DB)}

	// Save each setting to database
	for key, value := range settings {
		var err error
		switch v := value.(type) {
		case string:
			err = settingsModel.SetString(key, v)
		case float64:
			err = settingsModel.SetInt(key, int(v))
		case bool:
			err = settingsModel.SetBool(key, v)
		default:
			err = settingsModel.SetJSON(key, v)
		}

		if err != nil {
			InternalError(c, fmt.Sprintf("Failed to save setting %s: %v", key, err))
			return
		}
	}

	RespondSuccess(c, "Security settings saved successfully")
}

// TestDatabaseConnection tests the database connection
func TestDatabaseConnection(c *gin.Context) {
	db, exists := c.Get("db")
	if !exists {
		InternalError(c, "Database connection not available")
		return
	}

	start := time.Now()
	if err := db.(*sql.DB).Ping(); err != nil {
		InternalError(c, fmt.Sprintf("Database connection failed: %v", err))
		return
	}
	latency := time.Since(start)

	RespondSuccess(c, "Database connection successful", map[string]interface{}{
		"latency": latency.String(),
		"status":  "connected",
	})
}

// OptimizeDatabase optimizes the database
func OptimizeDatabase(c *gin.Context) {
	db, exists := c.Get("db")
	if !exists {
		InternalError(c, "Database connection not available")
		return
	}

	sqlDB := db.(*sql.DB)

	// Run ANALYZE to update statistics
	if _, err := database.ExecContext(context.Background(), sqlDB, database.TimeoutBulk, "ANALYZE"); err != nil {
		InternalError(c, fmt.Sprintf("Failed to analyze database: %v", err))
		return
	}

	RespondSuccess(c, "Database optimized successfully", map[string]interface{}{
		"operation": "ANALYZE completed",
	})
}

// ClearCache clears the application cache
func ClearCache(c *gin.Context) {
	// Get cache manager from context
	cacheInterface, exists := c.Get("cache")
	if !exists {
		RespondSuccess(c, "Cache not configured (running without cache)")
		return
	}

	cache, ok := cacheInterface.(*service.CacheManager)
	if !ok || !cache.IsEnabled() {
		RespondSuccess(c, "Cache not enabled")
		return
	}

	// Flush all cache entries
	if err := cache.Flush(); err != nil {
		InternalError(c, fmt.Sprintf("Failed to clear cache: %v", err))
		return
	}

	RespondSuccess(c, "Cache cleared successfully")
}

// VacuumDatabase performs database vacuum operation
func VacuumDatabase(c *gin.Context) {
	db, exists := c.Get("db")
	if !exists {
		InternalError(c, "Database connection not available")
		return
	}

	sqlDB := db.(*sql.DB)
	start := time.Now()

	// Run VACUUM to reclaim space
	if _, err := database.ExecContext(context.Background(), sqlDB, database.TimeoutBulk, "VACUUM"); err != nil {
		InternalError(c, fmt.Sprintf("Failed to vacuum database: %v", err))
		return
	}

	duration := time.Since(start)

	RespondSuccess(c, "Database vacuum completed", map[string]interface{}{
		"duration":  duration.String(),
		"operation": "VACUUM completed",
	})
}

// backupArchiveSuffixes are the only two archive shapes src/backup produces
// (AI.md PART 22): a gzipped tarball, or that same tarball encrypted with
// AES-256-GCM. Both list and download/delete key off this one predicate so a
// file that appears in the list is always addressable by the other endpoints.
var backupArchiveSuffixes = []string{".tar.gz", ".tar.gz.enc"}

// isBackupArchiveName reports whether a directory entry is a backup archive.
func isBackupArchiveName(name string) bool {
	for _, suffix := range backupArchiveSuffixes {
		if strings.HasSuffix(name, suffix) {
			return true
		}
	}
	return false
}

// adminBackupDir resolves the directory backups live in. DATA_DIR is honoured
// first so a container override (and a test) points these handlers at exactly
// the tree src/backup writes to; otherwise the canonical PART 4 resolver
// decides, which is what the CLI and the retention sweep also use.
func adminBackupDir() string {
	if dataDir := os.Getenv("DATA_DIR"); dataDir != "" {
		return filepath.Join(dataDir, "backups")
	}
	return path.GetBackupDir()
}

// adminBackupRoots returns the config and data directories a backup is taken
// from, resolved with the same env-first precedence as adminBackupDir.
func adminBackupRoots() (string, string) {
	configDir := os.Getenv("CONFIG_DIR")
	if configDir == "" {
		configDir = path.GetConfigDir()
	}
	dataDir := os.Getenv("DATA_DIR")
	if dataDir == "" {
		dataDir = path.GetDataDir()
	}
	return configDir, dataDir
}

// resolveBackupFile validates a client-supplied backup name and returns its
// absolute path. Only a bare file name is accepted: a name carrying any
// directory separator, a "." or ".." segment, or a null byte is rejected
// before it ever reaches the filesystem, so no request can address a file
// outside the backup directory (AI.md PART 5 path security).
func resolveBackupFile(name string) (string, error) {
	if name == "" {
		return "", fmt.Errorf("backup filename is required")
	}
	if strings.ContainsRune(name, 0) || name != filepath.Base(name) || name == "." || name == ".." {
		return "", fmt.Errorf("invalid backup filename")
	}
	if !isBackupArchiveName(name) {
		return "", fmt.Errorf("invalid backup filename")
	}
	return filepath.Join(adminBackupDir(), name), nil
}

// CreateBackup creates a backup of the database and configuration
func CreateBackup(c *gin.Context) {
	configDir, dataDir := adminBackupRoots()

	// The admin panel exposes the same optional contents the CLI does: an
	// encryption password (PART 22 makes it mandatory only under compliance
	// mode) and the two optional payloads.
	password := c.PostForm("password")
	includeSSL := c.PostForm("include_ssl") == "true"
	includeData := c.PostForm("include_data") == "true"

	svc := backup.New(configDir, dataDir)
	backupPath, err := svc.Create(backup.BackupOptions{
		ConfigDir:   configDir,
		DataDir:     dataDir,
		Password:    password,
		IncludeSSL:  includeSSL,
		IncludeData: includeData,
		CreatedBy:   AdminUsername(c),
		AppVersion:  Version,
	})
	if err != nil {
		InternalError(c, "Failed to create backup: "+err.Error())
		return
	}

	info, err := os.Stat(backupPath)
	if err != nil {
		InternalError(c, "Backup created but could not stat file: "+err.Error())
		return
	}

	RespondSuccess(c, "Backup created successfully", map[string]interface{}{
		"filename":  filepath.Base(backupPath),
		"size":      info.Size(),
		"created":   info.ModTime(),
		"encrypted": strings.HasSuffix(backupPath, ".enc"),
	})
}

// RestoreBackup restores from a backup file. The archive is either uploaded in
// the "backup" multipart field or named by the "filename" field, in which case
// it is taken from the backup directory.
func RestoreBackup(c *gin.Context) {
	configDir, dataDir := adminBackupRoots()
	password := c.PostForm("password")

	backupPath := ""
	if name := c.PostForm("filename"); name != "" {
		resolved, err := resolveBackupFile(name)
		if err != nil {
			BadRequest(c, err.Error())
			return
		}
		if _, statErr := os.Stat(resolved); statErr != nil {
			NotFound(c, "Backup file not found")
			return
		}
		backupPath = resolved
	} else {
		file, header, err := c.Request.FormFile("backup")
		if err != nil {
			BadRequest(c, "No backup file provided")
			return
		}
		defer file.Close()

		if !isBackupArchiveName(filepath.Base(header.Filename)) {
			BadRequest(c, "Uploaded file is not a backup archive")
			return
		}

		// The upload is staged under the backup directory rather than the
		// system temp dir so a restore never crosses a filesystem boundary and
		// never leaves an archive outside the app's own tree.
		if err := os.MkdirAll(adminBackupDir(), 0o700); err != nil {
			InternalError(c, "Failed to access backups directory")
			return
		}
		suffix := ".tar.gz"
		if strings.HasSuffix(header.Filename, ".enc") {
			suffix = ".tar.gz.enc"
		}
		staged, err := os.CreateTemp(adminBackupDir(), "upload-*"+suffix)
		if err != nil {
			InternalError(c, "Failed to stage uploaded backup")
			return
		}
		stagedPath := staged.Name()
		// The staged copy is transient input to the restore, not a backup the
		// operator asked to keep, so it is removed on every exit path.
		defer os.Remove(stagedPath)
		if _, err := io.Copy(staged, file); err != nil {
			staged.Close()
			InternalError(c, "Failed to store uploaded backup")
			return
		}
		if err := staged.Close(); err != nil {
			InternalError(c, "Failed to store uploaded backup")
			return
		}
		backupPath = stagedPath
	}

	svc := backup.New(configDir, dataDir)
	if err := svc.Restore(backup.RestoreOptions{
		BackupPath: backupPath,
		Password:   password,
		ConfigDir:  configDir,
		DataDir:    dataDir,
		Force:      true,
	}); err != nil {
		InternalError(c, "Failed to restore backup: "+err.Error())
		return
	}

	RespondSuccess(c, "Backup restored successfully. Restart the server to load the restored configuration.", map[string]interface{}{
		"filename": filepath.Base(backupPath),
	})
}

// ListBackups lists all available backup files
func ListBackups(c *gin.Context) {
	backups, err := readBackupFiles()
	if err != nil {
		InternalError(c, "Failed to read backups directory")
		return
	}

	RespondData(c, backups)
}

// readBackupFiles returns every backup archive in the backup directory, newest
// first. A missing directory is an empty list, not an error - no backup has
// been taken yet.
func readBackupFiles() ([]BackupFile, error) {
	entries, err := os.ReadDir(adminBackupDir())
	if err != nil {
		if os.IsNotExist(err) {
			return []BackupFile{}, nil
		}
		return nil, err
	}

	backups := []BackupFile{}
	for _, entry := range entries {
		if entry.IsDir() || !isBackupArchiveName(entry.Name()) {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		backups = append(backups, BackupFile{
			Filename: entry.Name(),
			Size:     info.Size(),
			Created:  info.ModTime(),
		})
	}

	sort.Slice(backups, func(i, j int) bool {
		return backups[i].Created.After(backups[j].Created)
	})
	return backups, nil
}

// BackupStats reports the backup counters the admin backup page shows: how many
// archives exist, how much disk they use, when the newest one was taken and
// when the scheduler will take the next one.
func BackupStats(c *gin.Context) {
	backups, err := readBackupFiles()
	if err != nil {
		InternalError(c, "Failed to read backups directory")
		return
	}

	var totalSize int64
	for _, item := range backups {
		totalSize += item.Size
	}

	stats := map[string]interface{}{
		"count":      len(backups),
		"total_size": totalSize,
		"directory":  adminBackupDir(),
	}
	if len(backups) > 0 {
		stats["last_backup"] = backups[0].Created
		stats["last_filename"] = backups[0].Filename
	}
	if nextRun := nextScheduledBackup(); nextRun != "" {
		stats["next_backup"] = nextRun
	}

	RespondData(c, stats)
}

// nextScheduledBackup returns the stored next_run of the soonest enabled backup
// task. The scheduler owns that column, so an unreachable database or an
// unregistered task simply yields no value rather than an error - the counter
// is informational.
func nextScheduledBackup() string {
	db := database.GetServerDB()
	if db == nil {
		return ""
	}
	var nextRun sql.NullString
	err := database.QueryRowContext(context.Background(), db, database.TimeoutSimpleSelect,
		"SELECT next_run FROM server_scheduler_state WHERE task_id LIKE 'backup%' AND enabled = 1 AND next_run IS NOT NULL ORDER BY next_run ASC LIMIT 1").Scan(&nextRun)
	if err != nil || !nextRun.Valid {
		return ""
	}
	return nextRun.String
}

// DownloadBackup downloads a specific backup file
func DownloadBackup(c *gin.Context) {
	backupPath, err := resolveBackupFile(c.Param("filename"))
	if err != nil {
		BadRequest(c, err.Error())
		return
	}

	if _, err := os.Stat(backupPath); err != nil {
		NotFound(c, "Backup file not found")
		return
	}

	c.Header("Content-Description", "File Transfer")
	c.Header("Content-Disposition", "attachment; filename=\""+filepath.Base(backupPath)+"\"")
	c.Header("Content-Type", "application/gzip")
	c.File(backupPath)
}

// DeleteBackup deletes a backup file
func DeleteBackup(c *gin.Context) {
	backupPath, err := resolveBackupFile(c.Param("filename"))
	if err != nil {
		BadRequest(c, err.Error())
		return
	}

	if _, err := os.Stat(backupPath); err != nil {
		NotFound(c, "Backup file not found")
		return
	}

	if err := os.Remove(backupPath); err != nil {
		InternalError(c, "Failed to delete backup: "+err.Error())
		return
	}

	RespondSuccess(c, "Backup deleted successfully")
}

// GetBackupSchedule returns the stored automated-backup settings.
func GetBackupSchedule(c *gin.Context) {
	settings, err := adminSettingsModel(c)
	if err != nil {
		InternalError(c, err.Error())
		return
	}

	RespondData(c, map[string]interface{}{
		"enabled":   settings.GetBool("backup.enabled", true),
		"interval":  settings.GetInt("backup.interval", 6),
		"retention": settings.GetInt("backup.retention", 30),
	})
}

// SaveBackupSchedule persists the automated-backup settings the scheduler reads
// on its next run.
func SaveBackupSchedule(c *gin.Context) {
	var payload struct {
		Enabled   *bool `json:"enabled" form:"enabled"`
		Interval  *int  `json:"interval" form:"interval"`
		Retention *int  `json:"retention" form:"retention"`
	}
	if err := c.ShouldBind(&payload); err != nil {
		BadRequest(c, "Invalid request data")
		return
	}

	settings, err := adminSettingsModel(c)
	if err != nil {
		InternalError(c, err.Error())
		return
	}

	if payload.Enabled != nil {
		if err := settings.SetBool("backup.enabled", *payload.Enabled); err != nil {
			InternalError(c, "Failed to save backup settings: "+err.Error())
			return
		}
	}
	// An interval of zero hours would make the scheduler run continuously and a
	// negative retention would delete every archive, so both are rejected
	// rather than stored.
	if payload.Interval != nil {
		if *payload.Interval < 1 || *payload.Interval > 168 {
			BadRequest(c, "Backup interval must be between 1 and 168 hours")
			return
		}
		if err := settings.SetInt("backup.interval", *payload.Interval); err != nil {
			InternalError(c, "Failed to save backup settings: "+err.Error())
			return
		}
	}
	if payload.Retention != nil {
		if *payload.Retention < 1 || *payload.Retention > 3650 {
			BadRequest(c, "Backup retention must be between 1 and 3650 days")
			return
		}
		if err := settings.SetInt("backup.retention", *payload.Retention); err != nil {
			InternalError(c, "Failed to save backup settings: "+err.Error())
			return
		}
	}

	RespondSuccess(c, "Backup settings saved successfully")
}

// adminSettingsModel resolves the settings model from the request-scoped
// database handle the admin API middleware installs.
func adminSettingsModel(c *gin.Context) (*model.SettingsModel, error) {
	db, exists := c.Get("db")
	if !exists {
		return nil, fmt.Errorf("database connection not available")
	}
	handle, ok := db.(*sql.DB)
	if !ok || handle == nil {
		return nil, fmt.Errorf("database connection not available")
	}
	return &model.SettingsModel{DB: handle}, nil
}

// SaveDatabaseSettings handles saving database configuration
func SaveDatabaseSettings(c *gin.Context) {
	var settings map[string]interface{}
	if err := c.ShouldBindJSON(&settings); err != nil {
		BadRequest(c, "Invalid request data")
		return
	}

	// Get database from context
	db, exists := c.Get("db")
	if !exists {
		InternalError(c, "Database connection not available")
		return
	}

	settingsModel := &model.SettingsModel{DB: db.(*sql.DB)}

	// Validate driver value
	if driver, ok := settings["database.driver"].(string); ok {
		validDrivers := []string{"file", "sqlite", "postgres", "mysql", "mariadb", "mssql", "mongodb"}
		isValid := false
		for _, valid := range validDrivers {
			if driver == valid {
				isValid = true
				break
			}
		}
		if !isValid {
			BadRequest(c, "Invalid database driver")
			return
		}

		// Validate port if provided
		if port, ok := settings["database.port"]; ok {
			var portNum int
			switch v := port.(type) {
			case float64:
				portNum = int(v)
			case string:
				fmt.Sscanf(v, "%d", &portNum)
			}
			if portNum < 1 || portNum > 65535 {
				BadRequest(c, "Port must be between 1 and 65535")
				return
			}
		}

		// For remote databases, validate required fields
		if driver != "file" && driver != "sqlite" {
			requiredFields := []string{"database.host", "database.port", "database.name"}
			for _, field := range requiredFields {
				if val, ok := settings[field]; !ok || val == "" {
					BadRequest(c, fmt.Sprintf("Field %s is required for remote databases", field))
					return
				}
			}
		}
	}

	// Save each setting to database
	for key, value := range settings {
		var err error
		switch v := value.(type) {
		case string:
			err = settingsModel.SetString(key, v)
		case float64:
			err = settingsModel.SetInt(key, int(v))
		case bool:
			err = settingsModel.SetBool(key, v)
		default:
			err = settingsModel.SetJSON(key, v)
		}

		if err != nil {
			InternalError(c, fmt.Sprintf("Failed to save setting %s: %v", key, err))
			return
		}
	}

	RespondSuccess(c, "Database settings saved successfully. Restart the server for changes to take effect.")
}

// TestDatabaseConfigConnection tests a database configuration without saving it
func TestDatabaseConfigConnection(c *gin.Context) {
	var config struct {
		Driver   string `json:"driver"`
		Host     string `json:"host"`
		Port     int    `json:"port"`
		Name     string `json:"name"`
		Username string `json:"username"`
		Password string `json:"password"`
		SSLMode  string `json:"sslmode"`
	}

	if err := c.ShouldBindJSON(&config); err != nil {
		BadRequest(c, "Invalid request data")
		return
	}

	// For file and sqlite, just return success
	if config.Driver == "file" || config.Driver == "sqlite" {
		RespondSuccess(c, "Local database configuration is valid", map[string]interface{}{
			"status": "valid",
		})
		return
	}

	if config.Host == "" {
		BadRequest(c, "Host is required for remote databases")
		return
	}

	if config.Port < 1 || config.Port > 65535 {
		BadRequest(c, "Port must be between 1 and 65535")
		return
	}

	if config.Name == "" {
		BadRequest(c, "Database name is required")
		return
	}

	RespondSuccess(c, "Database configuration validated successfully", map[string]interface{}{
		"status": "validated",
	})
}
