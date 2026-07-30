// Tests for maintenance_backup.go per AI.md PART 25 (Maintenance) / PART 29 (Testing).
//
// MaintenanceBackupCommand only prompts for a password via term.ReadPassword
// (which needs a real terminal fd, not fakeable via os.Stdin reassignment)
// when no --password flag is supplied; every test here passes --password to
// stay off that path. MaintenanceRestoreCommand's password prompt is only
// reached for a ".enc" file with no --password, so non-.enc fixtures (or an
// explicit --password) are used to stay off that path too; the
// bufio.NewReader(os.Stdin) confirmation prompt IS faked via withStdin.
package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestMaintenanceRestoreCommand covers: no backup path argument, a backup
// file that does not exist, declining the overwrite confirmation, and
// accepting it against a file that is not a valid backup archive (real
// backup.Restore failure path, no network/system calls involved).
func TestMaintenanceRestoreCommand(t *testing.T) {
	t.Run("no_args_errors", func(t *testing.T) {
		err := MaintenanceRestoreCommand(nil)
		if err == nil {
			t.Fatal("MaintenanceRestoreCommand(nil) = nil, want error")
		}
		if !strings.Contains(err.Error(), "restore requires a backup file path") {
			t.Errorf("error = %q, want substring %q", err.Error(), "restore requires a backup file path")
		}
	})

	t.Run("missing_file_errors", func(t *testing.T) {
		missing := filepath.Join(t.TempDir(), "nope.tar.gz")
		err := MaintenanceRestoreCommand([]string{missing})
		if err == nil {
			t.Fatal("MaintenanceRestoreCommand() on missing file = nil, want error")
		}
		if !strings.Contains(err.Error(), "backup file not found") {
			t.Errorf("error = %q, want substring %q", err.Error(), "backup file not found")
		}
	})

	t.Run("declined_confirmation_cancels", func(t *testing.T) {
		dir := t.TempDir()
		backupFile := filepath.Join(dir, "wthr_backup.tar.gz")
		if err := os.WriteFile(backupFile, []byte("not-a-real-archive"), 0644); err != nil {
			t.Fatalf("setup: %v", err)
		}
		t.Setenv("CONFIG_DIR", filepath.Join(dir, "config"))
		t.Setenv("DATA_DIR", filepath.Join(dir, "data"))

		var err error
		out := captureStdout(t, func() {
			withStdin(t, "no\n", func() {
				err = MaintenanceRestoreCommand([]string{backupFile})
			})
		})
		if err != nil {
			t.Fatalf("MaintenanceRestoreCommand() error = %v, want nil for declined restore", err)
		}
		if !strings.Contains(out, "Restore cancelled") {
			t.Errorf("output = %q, want it to contain %q", out, "Restore cancelled")
		}
	})

	t.Run("accepted_confirmation_on_invalid_archive_errors", func(t *testing.T) {
		dir := t.TempDir()
		backupFile := filepath.Join(dir, "wthr_backup.tar.gz")
		if err := os.WriteFile(backupFile, []byte("not-a-real-archive"), 0644); err != nil {
			t.Fatalf("setup: %v", err)
		}
		t.Setenv("CONFIG_DIR", filepath.Join(dir, "config"))
		t.Setenv("DATA_DIR", filepath.Join(dir, "data"))

		var err error
		captureStdout(t, func() {
			withStdin(t, "yes\n", func() {
				err = MaintenanceRestoreCommand([]string{backupFile})
			})
		})
		if err == nil {
			t.Fatal("MaintenanceRestoreCommand() on a non-archive file = nil, want error")
		}
		if !strings.Contains(err.Error(), "restore failed") {
			t.Errorf("error = %q, want substring %q", err.Error(), "restore failed")
		}
	})

	t.Run("explicit_password_flag_skips_terminal_prompt", func(t *testing.T) {
		dir := t.TempDir()
		// .enc extension would normally trigger the term.ReadPassword prompt
		// unless --password is supplied.
		backupFile := filepath.Join(dir, "wthr_backup.tar.gz.enc")
		if err := os.WriteFile(backupFile, []byte("not-a-real-archive"), 0644); err != nil {
			t.Fatalf("setup: %v", err)
		}
		t.Setenv("CONFIG_DIR", filepath.Join(dir, "config"))
		t.Setenv("DATA_DIR", filepath.Join(dir, "data"))

		var err error
		out := captureStdout(t, func() {
			withStdin(t, "no\n", func() {
				err = MaintenanceRestoreCommand([]string{backupFile, "--password", "secret"})
			})
		})
		if err != nil {
			t.Fatalf("MaintenanceRestoreCommand() error = %v, want nil for declined restore", err)
		}
		if !strings.Contains(out, "Restore cancelled") {
			t.Errorf("output = %q, want it to contain %q", out, "Restore cancelled")
		}
	})
}

// TestMaintenanceBackupCommand covers a full unencrypted-skip (explicit
// --password) happy path writing a real backup archive under a temp
// DATA_DIR, an explicit output-path positional argument, and the error path
// when the backup directory cannot be created.
func TestMaintenanceBackupCommand(t *testing.T) {
	t.Run("writes_encrypted_backup_under_data_dir", func(t *testing.T) {
		dir := t.TempDir()
		configDir := filepath.Join(dir, "config")
		dataDir := filepath.Join(dir, "data")
		if err := os.MkdirAll(configDir, 0755); err != nil {
			t.Fatalf("setup: %v", err)
		}
		if err := os.MkdirAll(dataDir, 0755); err != nil {
			t.Fatalf("setup: %v", err)
		}
		t.Setenv("CONFIG_DIR", configDir)
		t.Setenv("DATA_DIR", dataDir)

		var err error
		out := captureStdout(t, func() {
			err = MaintenanceBackupCommand([]string{"--password", "secret123", "--include-ssl", "--include-data"})
		})
		if err != nil {
			t.Fatalf("MaintenanceBackupCommand() error = %v", err)
		}
		if !strings.Contains(out, "Backup completed successfully") {
			t.Errorf("output = %q, want it to contain %q", out, "Backup completed successfully")
		}
		if !strings.Contains(out, "Backup is encrypted") {
			t.Errorf("output = %q, want it to mention encryption", out)
		}

		entries, err := os.ReadDir(filepath.Join(dataDir, "backup"))
		if err != nil {
			t.Fatalf("backup directory not created: %v", err)
		}
		if len(entries) != 1 {
			t.Fatalf("backup directory has %d entries, want 1", len(entries))
		}
		if !strings.HasSuffix(entries[0].Name(), ".tar.gz.enc") {
			t.Errorf("backup file name = %q, want .tar.gz.enc suffix", entries[0].Name())
		}
	})

	t.Run("explicit_output_path_argument_is_honored", func(t *testing.T) {
		dir := t.TempDir()
		t.Setenv("CONFIG_DIR", filepath.Join(dir, "config"))
		t.Setenv("DATA_DIR", filepath.Join(dir, "data"))

		outputPath := filepath.Join(dir, "custom-name.tar.gz.enc")
		var err error
		captureStdout(t, func() {
			err = MaintenanceBackupCommand([]string{outputPath, "--password", "secret123"})
		})
		if err != nil {
			t.Fatalf("MaintenanceBackupCommand() error = %v", err)
		}
		if _, statErr := os.Stat(outputPath); statErr != nil {
			t.Errorf("backup file not written at explicit path: %v", statErr)
		}
	})

	t.Run("undoable_backup_directory_errors", func(t *testing.T) {
		dir := t.TempDir()
		t.Setenv("CONFIG_DIR", filepath.Join(dir, "config"))
		t.Setenv("DATA_DIR", filepath.Join(dir, "data"))

		// blocker is a regular file; using it as a path component forces
		// os.MkdirAll to fail when backup.Create tries to create the
		// output file's parent directory.
		blocker := filepath.Join(dir, "blocker")
		if err := os.WriteFile(blocker, []byte("x"), 0644); err != nil {
			t.Fatalf("setup: %v", err)
		}
		outputPath := filepath.Join(blocker, "sub", "out.tar.gz.enc")

		var err error
		captureStdout(t, func() {
			err = MaintenanceBackupCommand([]string{outputPath, "--password", "secret123"})
		})
		if err == nil {
			t.Fatal("MaintenanceBackupCommand() with an uncreatable backup directory = nil, want error")
		}
		if !strings.Contains(err.Error(), "backup failed") {
			t.Errorf("error = %q, want substring %q", err.Error(), "backup failed")
		}
	})
}
