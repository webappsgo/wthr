// Package scheduler - automated backup task per AI.md PART 27
// AI.md Reference: Lines 24088-24250, specifically line 24182-24188
package scheduler

import (
	"fmt"
	"log"

	"github.com/webappsgo/wthr/src/backup"
	"github.com/webappsgo/wthr/src/database"
)

// BackupHourlyTask performs automated hourly incremental backups per AI.md PART 19 line 27050
// Schedule: @hourly (disabled by default)
// Creates: {projectname}-hourly.tar.gz[.enc] (single file, replaced each hour)
func BackupHourlyTask(configDir, dataDir string) func() error {
	return func() error {
		log.Println("INFO: Starting hourly backup...")

		// Create backup service
		svc := backup.New(configDir, dataDir)

		// Create hourly backup with specific filename
		// Per AI.md PART 19 line 27053-27054: Always 1 file (replaced each hour)
		opts := backup.BackupOptions{
			ConfigDir:   configDir,
			DataDir:     dataDir,
			OutputPath:  "",
			Kind:        backup.KindHourlyIncremental,
			Password:    "",
			IncludeSSL:  false,
			IncludeData: false,
			CreatedBy:   "scheduler-hourly",
			AppVersion:  "1.0.0",
			Retention:   systemBackupRetention(),
		}

		backupPath, deleted, err := svc.Create(opts)
		if err != nil {
			log.Printf("ERROR: Hourly backup failed: %v", err)
			return fmt.Errorf("hourly backup failed: %w", err)
		}

		logBackupRetentionAudit(database.GetServerDB(), "scheduler", "hourly", backupPath, deleted)

		log.Printf("OK: Hourly backup completed: %s", backupPath)
		return nil
	}
}
