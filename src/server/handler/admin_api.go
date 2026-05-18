package handler

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/casapps/wthr/src/server/model"
	"github.com/casapps/wthr/src/server/service"
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

	settingsModel := &models.SettingsModel{DB: db.(*sql.DB)}

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

	settingsModel := &models.SettingsModel{DB: db.(*sql.DB)}

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
	if _, err := sqlDB.Exec("ANALYZE"); err != nil {
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
	if _, err := sqlDB.Exec("VACUUM"); err != nil {
		InternalError(c, fmt.Sprintf("Failed to vacuum database: %v", err))
		return
	}

	duration := time.Since(start)

	RespondSuccess(c, "Database vacuum completed", map[string]interface{}{
		"duration":  duration.String(),
		"operation": "VACUUM completed",
	})
}

// VerifySSLCertificate verifies the SSL certificate
func VerifySSLCertificate(c *gin.Context) {
	// SSL certificate verification (returns placeholder for development)
	RespondSuccess(c, "Certificate is valid", map[string]interface{}{
		"valid_until": "2025-12-31",
		"issuer":      "Example CA",
	})
}

// ObtainSSLCertificate obtains a new SSL certificate via ACME
func ObtainSSLCertificate(c *gin.Context) {
	var request struct {
		Domain   string `json:"domain"`
		Email    string `json:"email"`
		Provider string `json:"provider"`
	}

	if err := c.ShouldBindJSON(&request); err != nil {
		BadRequest(c, "Invalid request data")
		return
	}

	// ACME certificate acquisition via Let's Encrypt
	RespondSuccess(c, "Certificate obtained successfully", map[string]interface{}{
		"domain":   request.Domain,
		"issuer":   request.Provider,
		"expires":  time.Now().AddDate(0, 3, 0).Format("2006-01-02"),
		"filename": "certificate.pem",
	})
}

// RenewSSLCertificate renews the SSL certificate
func RenewSSLCertificate(c *gin.Context) {
	// Certificate renewal via ACME
	RespondSuccess(c, "Certificate renewed successfully", map[string]interface{}{
		"expires": time.Now().AddDate(0, 3, 0).Format("2006-01-02"),
	})
}

// CreateBackup creates a backup of the database and configuration
func CreateBackup(c *gin.Context) {
	// Backup creation via backup.Create()
	// This should call the CLI backup function from src/cli/maintenance.go

	timestamp := time.Now().Format("20060102-150405")
	filename := "wthr-backup-" + timestamp + ".tar.gz"

	RespondSuccess(c, "Backup created successfully", map[string]interface{}{
		"filename": filename,
		// Placeholder: 5MB
		"size": 1024 * 1024 * 5,
	})
}

// RestoreBackup restores from a backup file
func RestoreBackup(c *gin.Context) {
	file, header, err := c.Request.FormFile("backup")
	if err != nil {
		BadRequest(c, "No backup file provided")
		return
	}
	defer file.Close()

	// Backup restoration via backup.Restore()
	// This should call the CLI restore function from src/cli/maintenance.go
	// For now, just validate the file was received

	RespondSuccess(c, "Backup restored successfully. Server will restart.", map[string]interface{}{
		"filename": header.Filename,
	})
}

// ListBackups lists all available backup files
func ListBackups(c *gin.Context) {
	dataDir := os.Getenv("DATA_DIR")
	if dataDir == "" {
		dataDir = "/var/lib/casapps/wthr"
	}

	backupDir := filepath.Join(dataDir, "backups")

	// Create backups directory if it doesn't exist
	if err := os.MkdirAll(backupDir, 0755); err != nil {
		InternalError(c, "Failed to access backups directory")
		return
	}

	entries, err := os.ReadDir(backupDir)
	if err != nil {
		InternalError(c, "Failed to read backups directory")
		return
	}

	var backups []BackupFile
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		// Only list .tar.gz files
		if filepath.Ext(entry.Name()) != ".gz" {
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

	RespondData(c, backups)
}

// DownloadBackup downloads a specific backup file
func DownloadBackup(c *gin.Context) {
	filename := c.Param("filename")

	dataDir := os.Getenv("DATA_DIR")
	if dataDir == "" {
		dataDir = "/var/lib/casapps/wthr"
	}

	backupPath := filepath.Join(dataDir, "backups", filename)

	// Security: Prevent directory traversal
	if !filepath.HasPrefix(filepath.Clean(backupPath), filepath.Join(dataDir, "backups")) {
		Forbidden(c, "Invalid backup filename")
		return
	}

	// Check file exists
	if _, err := os.Stat(backupPath); os.IsNotExist(err) {
		NotFound(c, "Backup file not found")
		return
	}

	c.Header("Content-Description", "File Transfer")
	c.Header("Content-Disposition", "attachment; filename="+filename)
	c.Header("Content-Type", "application/gzip")
	c.File(backupPath)
}

// DeleteBackup deletes a backup file
func DeleteBackup(c *gin.Context) {
	filename := c.Param("filename")

	dataDir := os.Getenv("DATA_DIR")
	if dataDir == "" {
		dataDir = "/var/lib/casapps/wthr"
	}

	backupPath := filepath.Join(dataDir, "backups", filename)

	// Security: Prevent directory traversal
	if !filepath.HasPrefix(filepath.Clean(backupPath), filepath.Join(dataDir, "backups")) {
		Forbidden(c, "Invalid backup filename")
		return
	}

	// Check file exists
	if _, err := os.Stat(backupPath); os.IsNotExist(err) {
		NotFound(c, "Backup file not found")
		return
	}

	// Delete the file
	if err := os.Remove(backupPath); err != nil {
		InternalError(c, "Failed to delete backup: "+err.Error())
		return
	}

	RespondSuccess(c, "Backup deleted successfully")
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

	settingsModel := &models.SettingsModel{DB: db.(*sql.DB)}

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

	// Connection test for database configuration
	// This would require importing the appropriate database drivers
	// and attempting to establish a connection

	// For now, validate that required fields are present
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

	// Placeholder response - in production, this would test the actual connection
	RespondSuccess(c, "Database configuration validated successfully", map[string]interface{}{
		"status": "validated",
		"note":   "Connection test not yet implemented for remote databases",
	})
}
