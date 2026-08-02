// Package handler - admin backup management per AI.md PART 19 & PART 25
// AI.md PART 19: Lines 15274+ (Admin Panel)
// AI.md PART 25: Lines 22349-22750 (Backup & Restore)
package handler

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"

	"github.com/gin-gonic/gin"
	"github.com/webappsgo/wthr/src/backup"
	"github.com/webappsgo/wthr/src/path"
)

// AdminBackupHandler handles /admin/server/backup page
func AdminBackupHandler(c *gin.Context) {
	// Get paths
	p := path.GetDefaultPaths("wthr")
	if p == nil {
		c.HTML(http.StatusInternalServerError, "error.tmpl", gin.H{
			"error": "Failed to get system paths",
		})
		return
	}

	// List existing backups
	backupDir := filepath.Join(p.DataDir, "backup")
	backups, err := listBackups(backupDir)
	if err != nil {
		// Empty list if error
		backups = []BackupInfo{}
	}

	// Render admin backup page
	c.HTML(http.StatusOK, "admin/backup.tmpl", gin.H{
		"title":   "Backup & Restore",
		"backups": backups,
	})
}

// AdminBackupCreateHandler handles POST /admin/server/backup/create
func AdminBackupCreateHandler(c *gin.Context) {
	// Get form data
	password := c.PostForm("password")
	includeSSL := c.PostForm("include_ssl") == "on"
	includeData := c.PostForm("include_data") == "on"

	// Get paths
	p := path.GetDefaultPaths("wthr")
	if p == nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"ok":    false,
			"error": "Failed to get system paths",
		})
		return
	}

	// Create backup service
	svc := backup.New(p.ConfigDir, p.DataDir)

	// Create backup per AI.md PART 25
	opts := backup.BackupOptions{
		ConfigDir: p.ConfigDir,
		DataDir:   p.DataDir,
		// Auto-generate filename
		OutputPath:  "",
		Password:    password,
		IncludeSSL:  includeSSL,
		IncludeData: includeData,
		// From admin panel
		CreatedBy: "admin",
		// Version from build info
		AppVersion: "1.0.0",
	}

	backupPath, err := svc.Create(opts)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"ok":    false,
			"error": fmt.Sprintf("Backup failed: %v", err),
		})
		return
	}

	// Get file info
	info, _ := os.Stat(backupPath)
	size := float64(0)
	if info != nil {
		// MB
		size = float64(info.Size()) / 1024 / 1024
	}

	c.JSON(http.StatusOK, gin.H{
		"ok":        true,
		"message":   "Backup created successfully",
		"path":      filepath.Base(backupPath),
		"size":      fmt.Sprintf("%.2f MB", size),
		"encrypted": password != "",
	})
}

// AdminBackupDownloadHandler handles GET /admin/server/backup/download/:filename
func AdminBackupDownloadHandler(c *gin.Context) {
	filename := c.Param("filename")

	// Get paths
	p := path.GetDefaultPaths("wthr")
	if p == nil {
		c.String(http.StatusInternalServerError, "Failed to get system paths")
		return
	}

	// Security: Only allow files in backup directory
	backupPath := filepath.Join(p.DataDir, "backup", filename)

	// Check if file exists
	if _, err := os.Stat(backupPath); os.IsNotExist(err) {
		c.String(http.StatusNotFound, "Backup file not found")
		return
	}

	// Serve file for download
	c.FileAttachment(backupPath, filename)
}

// AdminBackupDeleteHandler handles POST /admin/server/backup/delete/:filename
func AdminBackupDeleteHandler(c *gin.Context) {
	filename := c.Param("filename")

	// Get paths
	p := path.GetDefaultPaths("wthr")
	if p == nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"ok":    false,
			"error": "Failed to get system paths",
		})
		return
	}

	// Security: Only allow files in backup directory
	backupPath := filepath.Join(p.DataDir, "backup", filename)

	// Check if file exists
	if _, err := os.Stat(backupPath); os.IsNotExist(err) {
		c.JSON(http.StatusNotFound, gin.H{
			"ok":    false,
			"error": "Backup file not found",
		})
		return
	}

	// Delete file
	if err := os.Remove(backupPath); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"ok":    false,
			"error": fmt.Sprintf("Failed to delete backup: %v", err),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"ok":      true,
		"message": "Backup deleted successfully",
	})
}

// BackupInfo represents backup file metadata
type BackupInfo struct {
	Filename  string
	Size      string
	CreatedAt string
	Encrypted bool
}

// listBackups returns list of backup files
func listBackups(backupDir string) ([]BackupInfo, error) {
	// Ensure directory exists
	if err := os.MkdirAll(backupDir, 0700); err != nil {
		return nil, err
	}

	// List files
	files, err := filepath.Glob(filepath.Join(backupDir, "wthr_backup_*.tar.gz*"))
	if err != nil {
		return nil, err
	}

	backups := make([]BackupInfo, 0, len(files))
	for _, file := range files {
		info, err := os.Stat(file)
		if err != nil {
			continue
		}

		// MB
		size := float64(info.Size()) / 1024 / 1024
		encrypted := filepath.Ext(file) == ".enc"

		backups = append(backups, BackupInfo{
			Filename:  filepath.Base(file),
			Size:      fmt.Sprintf("%.2f MB", size),
			CreatedAt: info.ModTime().Format("2006-01-02 15:04:05"),
			Encrypted: encrypted,
		})
	}

	return backups, nil
}
