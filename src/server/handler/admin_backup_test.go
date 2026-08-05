package handler

import (
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/gin-gonic/gin"
)

// TestListBackups_EmptyDir verifies an empty (or freshly created) backup
// directory returns an empty, non-nil slice with no error.
func TestListBackups_EmptyDir(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "backup")

	backups, err := listBackups(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if backups == nil {
		t.Error("expected non-nil empty slice")
	}
	if len(backups) != 0 {
		t.Errorf("expected 0 backups, got %d", len(backups))
	}
	if _, statErr := os.Stat(dir); statErr != nil {
		t.Errorf("expected backup dir to be created: %v", statErr)
	}
}

// TestListBackups_MatchingAndNonMatchingFiles verifies only files matching
// the wthr_backup_*.tar.gz* glob are returned, plain and .enc suffixed, and
// unrelated files are excluded.
func TestListBackups_MatchingAndNonMatchingFiles(t *testing.T) {
	dir := t.TempDir()

	plain := filepath.Join(dir, "wthr_backup_20260101-120000.tar.gz")
	encrypted := filepath.Join(dir, "wthr_backup_20260102-120000.tar.gz.enc")
	unrelated := filepath.Join(dir, "notes.txt")

	for _, f := range []string{plain, encrypted, unrelated} {
		if err := os.WriteFile(f, []byte("data"), 0600); err != nil {
			t.Fatalf("failed to write fixture file %s: %v", f, err)
		}
	}

	backups, err := listBackups(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(backups) != 2 {
		t.Fatalf("expected 2 backups, got %d: %+v", len(backups), backups)
	}

	var sawPlain, sawEncrypted bool
	for _, b := range backups {
		switch b.Filename {
		case filepath.Base(plain):
			sawPlain = true
			if b.Encrypted {
				t.Error("plain backup should not be marked encrypted")
			}
		case filepath.Base(encrypted):
			sawEncrypted = true
			if !b.Encrypted {
				t.Error(".enc backup should be marked encrypted")
			}
		default:
			t.Errorf("unexpected backup file in results: %s", b.Filename)
		}
		if b.Size == "" {
			t.Error("expected non-empty formatted size")
		}
		if b.CreatedAt == "" {
			t.Error("expected non-empty formatted CreatedAt")
		}
	}
	if !sawPlain || !sawEncrypted {
		t.Errorf("expected both plain and encrypted backups, sawPlain=%v sawEncrypted=%v", sawPlain, sawEncrypted)
	}
}

// TestAdminBackupDownloadHandler_NotFound verifies a nonexistent filename
// returns 404 without attempting to serve a file.
func TestAdminBackupDownloadHandler_NotFound(t *testing.T) {
	c, w := newAPITestContext("/server/admin/config/backup/download/does-not-exist.tar.gz")
	c.Params = []gin.Param{{Key: "filename", Value: "does-not-exist.tar.gz"}}

	AdminBackupDownloadHandler(c)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected status 404, got %d: %s", w.Code, w.Body.String())
	}
}

// TestAdminBackupDeleteHandler_NotFound verifies a nonexistent filename
// returns 404 without attempting to delete anything.
func TestAdminBackupDeleteHandler_NotFound(t *testing.T) {
	c, w := newAPITestContext("/server/admin/config/backup/delete/does-not-exist.tar.gz")
	c.Params = []gin.Param{{Key: "filename", Value: "does-not-exist.tar.gz"}}

	AdminBackupDeleteHandler(c)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected status 404, got %d: %s", w.Code, w.Body.String())
	}
}
