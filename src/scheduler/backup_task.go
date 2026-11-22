// Package scheduler - automated backup task per AI.md PART 19 (Built-in Tasks:
// backup_hourly) implementing the backup behavior defined in AI.md PART 22
// (Backup Files Created).
package scheduler

import (
	"fmt"
	"log"

	"github.com/webappsgo/wthr/src/backup"
	"github.com/webappsgo/wthr/src/config"
	"github.com/webappsgo/wthr/src/database"
	"github.com/webappsgo/wthr/src/util"
)

var appVersion = "devel"

// SetAppVersion supplies the build version used in scheduled backup manifests.
func SetAppVersion(version string) {
	if version != "" {
		appVersion = version
	}
}

func scheduledBackupPassword() (string, error) {
	cfg := config.GetGlobalConfig()
	if cfg == nil || !cfg.Server.Maintenance.Backup.Encryption.Enabled {
		return "", nil
	}
	if cfg.Server.Security.EncryptionKey == "" {
		return "", fmt.Errorf("backup encryption is enabled but the server encryption key is unavailable")
	}
	sealed, err := config.ReadSecret("server.backup.encryption_password")
	if err != nil {
		return "", fmt.Errorf("stored backup password could not be read")
	}
	if sealed == "" {
		return "", fmt.Errorf("backup encryption is enabled but no stored backup password is available")
	}
	password, _, err := util.DecryptAtRestGrace(cfg.Server.Security.EncryptionKey, sealed)
	if err != nil {
		return "", fmt.Errorf("stored backup password could not be decrypted")
	}
	return password, nil
}

// BackupHourlyTask performs automated hourly incremental backups per AI.md PART 19 (Built-in Tasks: backup_hourly)
// Schedule: @hourly (disabled by default)
// Creates: {projectname}-hourly.tar.gz[.enc] (single file, replaced each hour)
func BackupHourlyTask(configDir, dataDir string) func() error {
	return func() error {
		log.Println("INFO: Starting hourly backup...")
		password, err := scheduledBackupPassword()
		if err != nil {
			return err
		}

		// Create backup service
		svc := backup.New(configDir, dataDir)

		// Create hourly backup with specific filename
		// Per AI.md PART 22 (Backup Files Created): always 1 file, replaced each hour
		opts := backup.BackupOptions{
			ConfigDir:   configDir,
			DataDir:     dataDir,
			OutputPath:  "",
			Kind:        backup.KindHourlyIncremental,
			Password:    password,
			IncludeSSL:  false,
			IncludeData: false,
			CreatedBy:   "scheduler-hourly",
			AppVersion:  appVersion,
			Retention:   systemBackupRetention(),
		}

		backupPath, deleted, err := svc.CreateBackupArchive(opts)
		if err != nil {
			log.Printf("ERROR: Hourly backup failed: %v", err)
			return fmt.Errorf("hourly backup failed: %w", err)
		}

		logBackupRetentionAudit(database.GetServerDB(), "scheduler", "hourly", backupPath, deleted)

		log.Printf("OK: Hourly backup completed: %s", backupPath)
		return nil
	}
}
