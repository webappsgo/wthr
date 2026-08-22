package scheduler

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// backup.Create() only does real local filesystem I/O (tar.gz archive creation,
// optional local encryption) with no network calls, so exercising it against a
// t.TempDir() is real local logic, not a mocked external service.
func TestBackupTask(t *testing.T) {
	configDir := t.TempDir()
	dataDir := t.TempDir()

	if err := os.WriteFile(filepath.Join(configDir, "server.yml"), []byte("mode: testing\n"), 0644); err != nil {
		t.Fatalf("seed server.yml: %v", err)
	}

	fn := BackupTask(configDir, dataDir)
	if err := fn(); err != nil {
		t.Fatalf("BackupTask()() error: %v", err)
	}

	if !anyTarGzUnder(t, dataDir) {
		t.Errorf("expected a .tar.gz backup file to be created somewhere under %s", dataDir)
	}
}

func TestBackupHourlyTask(t *testing.T) {
	configDir := t.TempDir()
	dataDir := t.TempDir()

	if err := os.WriteFile(filepath.Join(configDir, "server.yml"), []byte("mode: testing\n"), 0644); err != nil {
		t.Fatalf("seed server.yml: %v", err)
	}

	fn := BackupHourlyTask(configDir, dataDir)
	if err := fn(); err != nil {
		t.Fatalf("BackupHourlyTask()() error: %v", err)
	}

	if !anyTarGzUnder(t, dataDir) {
		t.Errorf("expected a .tar.gz backup file to be created somewhere under %s", dataDir)
	}
}

// anyTarGzUnder walks dataDir recursively - backup.Create() writes archives into
// a "backups/" subdirectory of dataDir rather than directly in dataDir, so a
// top-level os.ReadDir() is not sufficient.
func anyTarGzUnder(t *testing.T, dataDir string) bool {
	t.Helper()
	found := false
	if err := filepath.Walk(dataDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() && strings.Contains(info.Name(), ".tar.gz") {
			found = true
		}
		return nil
	}); err != nil {
		t.Fatalf("walk dataDir: %v", err)
	}
	return found
}

func TestRegisterBackupTask(t *testing.T) {
	s := NewScheduler(nil)

	t.Run("registers the backup_auto task regardless of the enabled flag", func(t *testing.T) {
		RegisterBackupTask(s, false)
		task := s.GetTask("backup_auto")
		if task == nil {
			t.Fatal("expected backup_auto task to be registered")
		}
		if task.Schedule != "0 1 * * *" {
			t.Errorf("Schedule = %q, want %q", task.Schedule, "0 1 * * *")
		}

		// BUG (not fixed, per task constraints): RegisterBackupTask's `enabled`
		// parameter only changes a log message; AddTask() unconditionally sets
		// enabled: true on any new Task, so the task is left enabled here even
		// though we passed enabled=false and the code comments claim it should
		// be "disabled by default". This assertion documents that actual (buggy)
		// behavior rather than the intended behavior.
		if !statusEnabled(t, s, "backup_auto") {
			t.Error("backup_auto unexpectedly reported as disabled - if this now fails, the enabled-flag bug in RegisterBackupTask has been fixed upstream")
		}
	})
}
