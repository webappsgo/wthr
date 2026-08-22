// Tests for backup.go per AI.md PART 22 (Backup & Restore) / PART 29 (Testing)
package backup

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestEncryptDecryptRoundTrip verifies AES-256-GCM encrypt/decrypt returns
// the original plaintext exactly, and that a wrong password fails to decrypt.
func TestEncryptDecryptRoundTrip(t *testing.T) {
	svc := New("", "")

	tests := []struct {
		name     string
		data     []byte
		password string
	}{
		{"normal_data", []byte("hello world backup contents"), "correct-horse-battery-staple"},
		{"empty_data", []byte{}, "somepassword"},
		{"binary_data", []byte{0x00, 0xFF, 0x10, 0x20, 0x00, 0x01}, "binpass"},
		{"large_data", bytes.Repeat([]byte("A"), 1<<20), "largepass"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			encrypted, err := svc.encrypt(tt.data, tt.password)
			if err != nil {
				t.Fatalf("encrypt() error = %v", err)
			}

			decrypted, err := svc.decrypt(encrypted, tt.password)
			if err != nil {
				t.Fatalf("decrypt() error = %v", err)
			}

			if !bytes.Equal(decrypted, tt.data) {
				t.Errorf("decrypt() = %v, want %v", decrypted, tt.data)
			}
		})
	}
}

// TestDecryptWrongPassword verifies decrypting with the wrong password
// returns an error rather than silently producing garbage.
func TestDecryptWrongPassword(t *testing.T) {
	svc := New("", "")

	encrypted, err := svc.encrypt([]byte("secret contents"), "right-password")
	if err != nil {
		t.Fatalf("encrypt() error = %v", err)
	}

	_, err = svc.decrypt(encrypted, "wrong-password")
	if err == nil {
		t.Fatal("decrypt() with wrong password should have returned an error")
	}
}

// TestDecryptTooShort verifies decrypt() rejects data too short to contain
// a salt+nonce header instead of panicking or slicing out of range.
func TestDecryptTooShort(t *testing.T) {
	svc := New("", "")

	tests := []struct {
		name string
		data []byte
	}{
		{"empty", []byte{}},
		{"one_byte", []byte{0x01}},
		{"salt_only_no_nonce", make([]byte, 32)},
		{"salt_plus_partial_nonce", make([]byte, 40)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := svc.decrypt(tt.data, "anypassword")
			if err == nil {
				t.Fatalf("decrypt() with %d bytes should have returned an error", len(tt.data))
			}
		})
	}
}

// TestDecryptCorruptedCiphertext verifies tampering with ciphertext after
// encryption is detected via GCM authentication rather than returning
// corrupted plaintext.
func TestDecryptCorruptedCiphertext(t *testing.T) {
	svc := New("", "")

	encrypted, err := svc.encrypt([]byte("authentic data"), "password123")
	if err != nil {
		t.Fatalf("encrypt() error = %v", err)
	}

	// Flip a bit well into the ciphertext (after the salt+nonce header).
	corrupted := make([]byte, len(encrypted))
	copy(corrupted, encrypted)
	corrupted[len(corrupted)-1] ^= 0xFF

	if _, err := svc.decrypt(corrupted, "password123"); err == nil {
		t.Fatal("decrypt() of tampered ciphertext should have returned an error")
	}
}

// TestCreateArchiveRoundTrip verifies createArchive packs the manifest and
// requested files, and that extractArchive can read them back correctly.
func TestCreateArchiveRoundTrip(t *testing.T) {
	configDir := t.TempDir()
	dataDir := t.TempDir()

	if err := os.WriteFile(filepath.Join(configDir, "server.yml"), []byte("server: config"), 0600); err != nil {
		t.Fatalf("failed to seed server.yml: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(dataDir, "db"), 0700); err != nil {
		t.Fatalf("failed to seed db dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dataDir, "db", "server.db"), []byte("sqlite-bytes"), 0600); err != nil {
		t.Fatalf("failed to seed server.db: %v", err)
	}

	svc := New(configDir, dataDir)
	files := []string{"server.yml", "db/server.db"}
	manifest := Manifest{
		Version:    "1.0.0",
		CreatedAt:  time.Now(),
		AppVersion: "test",
		Contents:   files,
	}

	archiveData, checksum, err := svc.createArchive(configDir, dataDir, files, manifest)
	if err != nil {
		t.Fatalf("createArchive() error = %v", err)
	}
	if len(archiveData) == 0 {
		t.Fatal("createArchive() returned empty data")
	}
	if checksum == "" {
		t.Fatal("createArchive() returned empty checksum")
	}

	// Read back the manifest via the package's own extractManifest.
	gotManifest, err := svc.extractManifest(archiveData)
	if err != nil {
		t.Fatalf("extractManifest() error = %v", err)
	}
	if gotManifest.AppVersion != "test" {
		t.Errorf("extractManifest().AppVersion = %q, want %q", gotManifest.AppVersion, "test")
	}

	// Extract into fresh destination dirs and verify file contents round-trip.
	restoredConfig := t.TempDir()
	restoredData := t.TempDir()
	if err := svc.extractArchive(archiveData, restoredConfig, restoredData); err != nil {
		t.Fatalf("extractArchive() error = %v", err)
	}

	got, err := os.ReadFile(filepath.Join(restoredConfig, "server.yml"))
	if err != nil {
		t.Fatalf("failed to read restored server.yml: %v", err)
	}
	if string(got) != "server: config" {
		t.Errorf("restored server.yml = %q, want %q", got, "server: config")
	}

	got, err = os.ReadFile(filepath.Join(restoredData, "db", "server.db"))
	if err != nil {
		t.Fatalf("failed to read restored server.db: %v", err)
	}
	if string(got) != "sqlite-bytes" {
		t.Errorf("restored server.db = %q, want %q", got, "sqlite-bytes")
	}
}

// TestCreateArchiveDirectoryContents verifies directories (e.g. template/)
// are walked recursively and nested file contents survive the round trip.
func TestCreateArchiveDirectoryContents(t *testing.T) {
	configDir := t.TempDir()
	dataDir := t.TempDir()

	templateDir := filepath.Join(configDir, "template")
	if err := os.MkdirAll(filepath.Join(templateDir, "nested"), 0700); err != nil {
		t.Fatalf("failed to seed template dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(templateDir, "base.tmpl"), []byte("base template"), 0600); err != nil {
		t.Fatalf("failed to seed base.tmpl: %v", err)
	}
	if err := os.WriteFile(filepath.Join(templateDir, "nested", "child.tmpl"), []byte("child template"), 0600); err != nil {
		t.Fatalf("failed to seed child.tmpl: %v", err)
	}

	svc := New(configDir, dataDir)
	files := []string{"template/"}
	manifest := Manifest{Version: "1.0.0", CreatedAt: time.Now(), Contents: files}

	archiveData, _, err := svc.createArchive(configDir, dataDir, files, manifest)
	if err != nil {
		t.Fatalf("createArchive() error = %v", err)
	}

	restoredConfig := t.TempDir()
	restoredData := t.TempDir()
	if err := svc.extractArchive(archiveData, restoredConfig, restoredData); err != nil {
		t.Fatalf("extractArchive() error = %v", err)
	}

	got, err := os.ReadFile(filepath.Join(restoredConfig, "template", "nested", "child.tmpl"))
	if err != nil {
		t.Fatalf("failed to read restored nested template: %v", err)
	}
	if string(got) != "child template" {
		t.Errorf("restored child.tmpl = %q, want %q", got, "child template")
	}
}

// TestCreateArchiveSkipsMissingFiles verifies that files listed but absent
// from disk are silently skipped rather than causing an error.
func TestCreateArchiveSkipsMissingFiles(t *testing.T) {
	configDir := t.TempDir()
	dataDir := t.TempDir()

	svc := New(configDir, dataDir)
	files := []string{"server.yml", "db/server.db"} // neither exists on disk
	manifest := Manifest{Version: "1.0.0", CreatedAt: time.Now(), Contents: files}

	archiveData, _, err := svc.createArchive(configDir, dataDir, files, manifest)
	if err != nil {
		t.Fatalf("createArchive() with missing files should not error, got: %v", err)
	}

	// Only manifest.json should be present in the resulting tar.
	gzReader, err := gzip.NewReader(bytes.NewReader(archiveData))
	if err != nil {
		t.Fatalf("gzip.NewReader() error = %v", err)
	}
	defer gzReader.Close()
	tr := tar.NewReader(gzReader)

	var names []string
	for {
		hdr, err := tr.Next()
		if err != nil {
			break
		}
		names = append(names, hdr.Name)
	}
	if len(names) != 1 || names[0] != "manifest.json" {
		t.Errorf("archive entries = %v, want only [manifest.json]", names)
	}
}

// TestCollectFiles verifies collectFiles only includes files/dirs that
// actually exist, and honors the IncludeSSL/IncludeData opt-in flags.
func TestCollectFiles(t *testing.T) {
	tests := []struct {
		name  string
		setup func(configDir, dataDir string)
		opts  func(configDir, dataDir string) BackupOptions
		want  []string
	}{
		{
			name:  "nothing_present",
			setup: func(configDir, dataDir string) {},
			opts: func(configDir, dataDir string) BackupOptions {
				return BackupOptions{ConfigDir: configDir, DataDir: dataDir}
			},
			want: nil,
		},
		{
			name: "server_yml_and_db_present",
			setup: func(configDir, dataDir string) {
				os.WriteFile(filepath.Join(configDir, "server.yml"), []byte("x"), 0600)
				os.MkdirAll(filepath.Join(dataDir, "db"), 0700)
				os.WriteFile(filepath.Join(dataDir, "db", "server.db"), []byte("x"), 0600)
			},
			opts: func(configDir, dataDir string) BackupOptions {
				return BackupOptions{ConfigDir: configDir, DataDir: dataDir}
			},
			want: []string{"server.yml", "db/server.db"},
		},
		{
			name: "ssl_excluded_without_flag",
			setup: func(configDir, dataDir string) {
				os.MkdirAll(filepath.Join(configDir, "ssl"), 0700)
			},
			opts: func(configDir, dataDir string) BackupOptions {
				return BackupOptions{ConfigDir: configDir, DataDir: dataDir, IncludeSSL: false}
			},
			want: nil,
		},
		{
			name: "ssl_included_with_flag",
			setup: func(configDir, dataDir string) {
				os.MkdirAll(filepath.Join(configDir, "ssl"), 0700)
			},
			opts: func(configDir, dataDir string) BackupOptions {
				return BackupOptions{ConfigDir: configDir, DataDir: dataDir, IncludeSSL: true}
			},
			want: []string{"ssl/"},
		},
		{
			name:  "data_included_with_flag_even_if_absent",
			setup: func(configDir, dataDir string) {},
			opts: func(configDir, dataDir string) BackupOptions {
				return BackupOptions{ConfigDir: configDir, DataDir: dataDir, IncludeData: true}
			},
			want: []string{"data/"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			configDir := t.TempDir()
			dataDir := t.TempDir()
			tt.setup(configDir, dataDir)

			svc := New(configDir, dataDir)
			got, err := svc.collectFiles(tt.opts(configDir, dataDir))
			if err != nil {
				t.Fatalf("collectFiles() error = %v", err)
			}

			if len(got) != len(tt.want) {
				t.Fatalf("collectFiles() = %v, want %v", got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("collectFiles()[%d] = %q, want %q", i, got[i], tt.want[i])
				}
			}
		})
	}
}

// TestCleanupOldBackupsRetention verifies pruning behavior at boundary
// counts: under the limit (nothing pruned), exactly at the limit (nothing
// pruned), and one over the limit (oldest file pruned).
func TestCleanupOldBackupsRetention(t *testing.T) {
	makeBackups := func(t *testing.T, dir string, n int) []string {
		t.Helper()
		var names []string
		for i := 0; i < n; i++ {
			name := filepath.Join(dir, "wthr_backup_2024-01-0"+string(rune('1'+i))+"_000000.tar.gz")
			if err := os.WriteFile(name, []byte("data"), 0600); err != nil {
				t.Fatalf("failed to write fixture backup: %v", err)
			}
			// Ensure distinct, increasing modtimes so ordering is deterministic.
			modTime := time.Now().Add(time.Duration(i) * time.Minute)
			if err := os.Chtimes(name, modTime, modTime); err != nil {
				t.Fatalf("failed to set modtime: %v", err)
			}
			names = append(names, name)
		}
		return names
	}

	retention := RetentionConfig{MaxBackups: 4}

	t.Run("zero_backups", func(t *testing.T) {
		dir := t.TempDir()
		if _, err := applyRetention(dir, retention, 0); err != nil {
			t.Fatalf("applyRetention() error = %v", err)
		}
		remaining, _ := filepath.Glob(filepath.Join(dir, "wthr_backup_*.tar.gz*"))
		if len(remaining) != 0 {
			t.Errorf("expected 0 remaining backups, got %d", len(remaining))
		}
	})

	t.Run("exactly_at_limit_keeps_all", func(t *testing.T) {
		dir := t.TempDir()
		names := makeBackups(t, dir, 4)
		if _, err := applyRetention(dir, retention, 0); err != nil {
			t.Fatalf("applyRetention() error = %v", err)
		}
		remaining, _ := filepath.Glob(filepath.Join(dir, "wthr_backup_*.tar.gz*"))
		if len(remaining) != len(names) {
			t.Errorf("expected all %d backups kept, got %d remaining", len(names), len(remaining))
		}
	})

	t.Run("one_over_limit_prunes_oldest", func(t *testing.T) {
		dir := t.TempDir()
		names := makeBackups(t, dir, 5)
		if _, err := applyRetention(dir, retention, 0); err != nil {
			t.Fatalf("applyRetention() error = %v", err)
		}
		remaining, _ := filepath.Glob(filepath.Join(dir, "wthr_backup_*.tar.gz*"))
		if len(remaining) != 4 {
			t.Fatalf("expected 4 remaining backups, got %d: %v", len(remaining), remaining)
		}
		// The oldest fixture (index 0) must have been removed.
		oldest := names[0]
		if _, err := os.Stat(oldest); !os.IsNotExist(err) {
			t.Errorf("oldest backup %q should have been pruned", oldest)
		}
		// The newest fixture (index 4) must still exist.
		newest := names[4]
		if _, err := os.Stat(newest); err != nil {
			t.Errorf("newest backup %q should have been kept: %v", newest, err)
		}
	})
}

// TestBackupServiceCreateAndVerify exercises the full Create() workflow
// end-to-end: collects real files, archives, optionally encrypts, writes to
// disk, and self-verifies. Covers both the plaintext and encrypted paths.
func TestBackupServiceCreateAndVerify(t *testing.T) {
	tests := []struct {
		name     string
		password string
	}{
		{"plaintext_backup", ""},
		{"encrypted_backup", "s3cret-passw0rd"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			configDir := t.TempDir()
			dataDir := t.TempDir()

			if err := os.WriteFile(filepath.Join(configDir, "server.yml"), []byte("mode: production"), 0600); err != nil {
				t.Fatalf("failed to seed server.yml: %v", err)
			}

			svc := New(configDir, dataDir)
			outPath, _, err := svc.Create(BackupOptions{
				ConfigDir:  configDir,
				DataDir:    dataDir,
				Password:   tt.password,
				CreatedBy:  "tester",
				AppVersion: "0.0.1-test",
			})
			if err != nil {
				t.Fatalf("Create() error = %v", err)
			}

			if _, err := os.Stat(outPath); err != nil {
				t.Fatalf("backup file was not written: %v", err)
			}

			if err := svc.Verify(outPath, tt.password); err != nil {
				t.Fatalf("Verify() error = %v", err)
			}
		})
	}
}

// TestVerifyErrors covers Verify() error paths: missing file, empty file,
// encrypted file without a password, and wrong password.
func TestVerifyErrors(t *testing.T) {
	svc := New("", "")

	t.Run("missing_file", func(t *testing.T) {
		if err := svc.Verify(filepath.Join(t.TempDir(), "nope.tar.gz"), ""); err == nil {
			t.Fatal("Verify() on a missing file should return an error")
		}
	})

	t.Run("empty_file", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "empty.tar.gz")
		if err := os.WriteFile(path, nil, 0600); err != nil {
			t.Fatalf("failed to write empty fixture: %v", err)
		}
		if err := svc.Verify(path, ""); err == nil {
			t.Fatal("Verify() on an empty file should return an error")
		}
	})

	t.Run("encrypted_without_password", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "backup.tar.gz.enc")
		if err := os.WriteFile(path, []byte("some encrypted looking bytes"), 0600); err != nil {
			t.Fatalf("failed to write fixture: %v", err)
		}
		if err := svc.Verify(path, ""); err == nil {
			t.Fatal("Verify() on an .enc file without a password should return an error")
		}
	})

	t.Run("wrong_password", func(t *testing.T) {
		encrypted, err := svc.encrypt([]byte("plaintext payload"), "right")
		if err != nil {
			t.Fatalf("encrypt() error = %v", err)
		}
		path := filepath.Join(t.TempDir(), "backup.tar.gz.enc")
		if err := os.WriteFile(path, encrypted, 0600); err != nil {
			t.Fatalf("failed to write fixture: %v", err)
		}
		if err := svc.Verify(path, "wrong"); err == nil {
			t.Fatal("Verify() with the wrong password should return an error")
		}
	})
}

// TestManifestJSONRoundTrip is a small sanity check that Manifest survives
// JSON marshal/unmarshal, since createArchive/extractManifest depend on it.
func TestManifestJSONRoundTrip(t *testing.T) {
	m := Manifest{
		Version:          "1.0.0",
		CreatedAt:        time.Now().UTC().Truncate(time.Second),
		CreatedBy:        "admin",
		AppVersion:       "1.2.3",
		Contents:         []string{"server.yml", "db/server.db"},
		Encrypted:        true,
		EncryptionMethod: "AES-256-GCM",
		Checksum:         "sha256:abc123",
	}

	data, err := json.Marshal(m)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}

	var got Manifest
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}

	if !got.CreatedAt.Equal(m.CreatedAt) || got.Version != m.Version || got.Checksum != m.Checksum {
		t.Errorf("Manifest round trip = %+v, want %+v", got, m)
	}
}
