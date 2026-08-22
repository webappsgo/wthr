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
