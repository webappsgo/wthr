// Package backup implements backup and restore functionality per AI.md PART 22
// (Backup & Restore Command).
package backup

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"hash"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/crypto/argon2"
	_ "modernc.org/sqlite"
)

// manifestEntryName is the archive member holding the backup manifest
const manifestEntryName = "manifest.json"

// Manifest represents backup metadata per AI.md PART 22 (Backup Format)
type Manifest struct {
	Version          string    `json:"version"`
	CreatedAt        time.Time `json:"created_at"`
	CreatedBy        string    `json:"created_by"`
	AppVersion       string    `json:"app_version"`
	Contents         []string  `json:"contents"`
	Encrypted        bool      `json:"encrypted"`
	EncryptionMethod string    `json:"encryption_method,omitempty"`
	Checksum         string    `json:"checksum"`
}

// Backup kind values recognized by BackupOptions.Kind, controlling which
// filename format Create auto-generates when OutputPath is empty, per AI.md
// PART 22's "Backup Files Created" table (lines 36453-36471):
//   - KindManual (default/""): {project_name}_backup_YYYY-MM-DD_HHMMSS.tar.gz[.enc],
//     one new file per call, counted under max_backups. Used by the CLI/API
//     "backup [filename]" command and any caller that doesn't set Kind.
//   - KindDailyFull: {project_name}_backup_YYYY-MM-DD.tar.gz[.enc], date-only
//     (no time), one per calendar day, counted under max_backups. Used by the
//     scheduled backup_daily task (02:00).
//   - KindDailyIncremental: {project_name}-daily.tar.gz[.enc], a single fixed
//     filename replaced in place every run - never counted by the retention
//     sweep's count-based tiers.
//   - KindHourlyIncremental: {project_name}-hourly.tar.gz[.enc], same
//     replaced-in-place behavior as KindDailyIncremental, on an hourly cadence.
const (
	KindManual            = ""
	KindDailyFull         = "daily_full"
	KindDailyIncremental  = "daily_incremental"
	KindHourlyIncremental = "hourly_incremental"
)

// BackupOptions configures backup creation per AI.md PART 22
type BackupOptions struct {
	ConfigDir   string
	DataDir     string
	OutputPath  string
	Password    string
	IncludeSSL  bool
	IncludeData bool
	CreatedBy   string
	AppVersion  string
	// Kind selects the auto-generated filename format when OutputPath is
	// empty - see the Kind* constants above. Ignored when OutputPath is set
	// explicitly.
	Kind string
	// Retention controls the tiered pruning sweep per AI.md PART 22
	// (Backup Retention). Nil uses DefaultRetention().
	Retention *RetentionConfig
}

// BackupService handles backup operations per AI.md PART 22
type BackupService struct {
	configDir string
	dataDir   string
}

// New creates a new BackupService
func New(configDir, dataDir string) *BackupService {
	return &BackupService{
		configDir: configDir,
		dataDir:   dataDir,
	}
}

// CreateBackupArchive creates a new backup per AI.md PART 22 (Backup & Restore)
// Follows complete backup workflow with verification and cleanup.
// The second return value lists backup filenames deleted by the tiered
// retention sweep (AI.md PART 22's "backup.retention_cleanup" audit event) -
// callers with DB access log it, callers without (the CLI) may discard it.
func (s *BackupService) CreateBackupArchive(opts BackupOptions) (string, []string, error) {
	// Set defaults
	if opts.ConfigDir == "" {
		opts.ConfigDir = s.configDir
	}
	if opts.DataDir == "" {
		opts.DataDir = s.dataDir
	}
	if opts.OutputPath == "" {
		ext := ".tar.gz"
		if opts.Password != "" {
			ext = ".tar.gz.enc"
		}
		// The directory is "backups" (plural) because that is what
		// path.Paths.BackupDir resolves to, and every reader - the admin API,
		// the CLI list and the retention sweep - looks there. This used to
		// write to "backup" (singular), so a backup created here was invisible
		// to every one of those readers and the retention sweep never saw it.
		var filename string
		switch opts.Kind {
		case KindDailyFull:
			// Date-only per AI.md PART 22's "Backup Files Created" table -
			// one full backup per calendar day, distinct from the
			// timestamped manual/CLI/API format.
			filename = fmt.Sprintf("wthr_backup_%s%s", time.Now().Format("2006-01-02"), ext)
		case KindDailyIncremental:
			// Fixed filename, replaced in place every run - AI.md PART 22:
			// "always exactly 1 file".
			filename = "wthr-daily" + ext
		case KindHourlyIncremental:
			filename = "wthr-hourly" + ext
		default:
			// Manual/CLI/API backups per AI.md PART 22 (Backup Files Created):
			// wthr_backup_YYYY-MM-DD_HHMMSS.tar.gz[.enc]
			filename = fmt.Sprintf("wthr_backup_%s%s", time.Now().Format("2006-01-02_150405"), ext)
		}
		opts.OutputPath = filepath.Join(opts.DataDir, "backups", filename)
	}

	// Ensure backup directory exists
	backupDir := filepath.Dir(opts.OutputPath)
	if err := os.MkdirAll(backupDir, 0700); err != nil {
		return "", nil, fmt.Errorf("failed to create backup directory: %w", err)
	}

	// Collect files to backup per AI.md PART 22 (Backup Contents)
	files, err := s.collectFiles(opts)
	if err != nil {
		return "", nil, fmt.Errorf("failed to collect files: %w", err)
	}

	// Create manifest per AI.md PART 22 (Backup Format)
	manifest := Manifest{
		Version:    "1.0.0",
		CreatedAt:  time.Now(),
		CreatedBy:  opts.CreatedBy,
		AppVersion: opts.AppVersion,
		Contents:   files,
		Encrypted:  opts.Password != "",
	}
	if manifest.Encrypted {
		// Per AI.md PART 22 (Encryption)
		manifest.EncryptionMethod = "AES-256-GCM"
	}

	// Create tar.gz archive in memory per AI.md PART 22 (Backup Format)
	// "Unencrypted archive never touches disk"
	// createArchive embeds the content checksum in the archived manifest itself
	archiveData, _, err := s.createArchive(opts.ConfigDir, opts.DataDir, files, manifest)
	if err != nil {
		return "", nil, fmt.Errorf("failed to create archive: %w", err)
	}

	// Encrypt if password provided per AI.md PART 22 (Encryption)
	var finalData []byte
	if opts.Password != "" {
		encrypted, err := s.encrypt(archiveData, opts.Password)
		if err != nil {
			return "", nil, fmt.Errorf("failed to encrypt backup: %w", err)
		}
		finalData = encrypted
	} else {
		finalData = archiveData
	}

	// Write to disk
	if err := os.WriteFile(opts.OutputPath, finalData, 0600); err != nil {
		return "", nil, fmt.Errorf("failed to write backup file: %w", err)
	}

	// Verify backup per AI.md PART 22 (Verification)
	// "Every backup is verified immediately after creation"
	if err := s.Verify(opts.OutputPath, opts.Password); err != nil {
		// Delete failed backup per AI.md PART 22 (Verification)
		os.Remove(opts.OutputPath)
		return "", nil, fmt.Errorf("backup verification failed: %w", err)
	}

	// Apply tiered retention per AI.md PART 22 (Backup Retention)
	// "Only delete old backups if new backup passes ALL verification checks"
	retention := DefaultRetention()
	if opts.Retention != nil {
		retention = *opts.Retention
	}
	totalBytes, _ := VolumeTotalBytes(backupDir)
	deleted, err := applyRetention(backupDir, retention, totalBytes)
	if err != nil {
		// Log but don't fail - backup itself succeeded
		log.Printf("WARNING: failed to apply backup retention: %v", err)
	}

	return opts.OutputPath, deleted, nil
}

// collectFiles identifies files to include in backup per AI.md PART 22 (Backup Contents)
func (s *BackupService) collectFiles(opts BackupOptions) ([]string, error) {
	var files []string

	// server.yml - Always included per AI.md PART 22 (Backup Contents)
	serverYML := filepath.Join(opts.ConfigDir, "server.yml")
	if _, err := os.Stat(serverYML); err == nil {
		files = append(files, "server.yml")
	}

	// server.db - Always included per AI.md PART 22 (Backup Contents)
	serverDB := filepath.Join(opts.DataDir, "db", "server.db")
	if _, err := os.Stat(serverDB); err == nil {
		files = append(files, "db/server.db")
	}

	// users.db - If exists per AI.md PART 22 (Backup Contents)
	usersDB := filepath.Join(opts.DataDir, "db", "users.db")
	if _, err := os.Stat(usersDB); err == nil {
		files = append(files, "db/users.db")
	}

	// Custom templates - If exists per AI.md PART 22 (Backup Contents)
	templatesDir := filepath.Join(opts.ConfigDir, "template")
	if _, err := os.Stat(templatesDir); err == nil {
		files = append(files, "template/")
	}

	// Custom themes - If exists per AI.md PART 22 Backup Contents
	themeDir := filepath.Join(opts.ConfigDir, "theme")
	if _, err := os.Stat(themeDir); err == nil {
		files = append(files, "theme/")
	}

	// SSL certificates - Optional per AI.md PART 22 (Backup Contents)
	if opts.IncludeSSL {
		sslDir := filepath.Join(opts.ConfigDir, "ssl")
		if _, err := os.Stat(sslDir); err == nil {
			files = append(files, "ssl/")
		}
	}

	// Data files - Optional per AI.md PART 22 (Backup Contents)
	if opts.IncludeData {
		files = append(files, "data/")
	}

	return files, nil
}

// createArchive creates tar.gz archive and returns data + checksum
// Per AI.md PART 22 (Backup Format)
func (s *BackupService) createArchive(configDir, dataDir string, files []string, manifest Manifest) ([]byte, string, error) {
	// Create in-memory buffer (unencrypted archive never touches disk per AI.md PART 22 (Backup Format))
	var buf []byte
	writer := &memoryWriter{data: buf}

	// Create gzip writer
	gzWriter := gzip.NewWriter(writer)
	defer gzWriter.Close()

	// Create tar writer
	tarWriter := tar.NewWriter(gzWriter)
	defer tarWriter.Close()

	// Hash every archived member as it is written so the manifest can carry a
	// checksum that Verify recomputes from the archive itself
	contentHasher := sha256.New()

	// Add each file/directory
	for _, file := range files {
		var sourcePath string
		if file == "server.yml" || strings.HasPrefix(file, "template/") ||
			strings.HasPrefix(file, "theme/") || strings.HasPrefix(file, "ssl/") {
			sourcePath = filepath.Join(configDir, file)
		} else {
			sourcePath = filepath.Join(dataDir, file)
		}

		// Skip if file doesn't exist
		if _, err := os.Stat(sourcePath); os.IsNotExist(err) {
			continue
		}

		if err := s.addToArchive(tarWriter, contentHasher, sourcePath, file); err != nil {
			return nil, "", fmt.Errorf("failed to add %s: %w", file, err)
		}
	}

	// The manifest is written last so it can carry the checksum of everything
	// that precedes it per AI.md PART 22 (Verification: checksum matches manifest)
	checksumStr := fmt.Sprintf("sha256:%s", hex.EncodeToString(contentHasher.Sum(nil)))
	manifest.Checksum = checksumStr

	manifestData, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return nil, "", fmt.Errorf("failed to marshal manifest: %w", err)
	}

	if err := tarWriter.WriteHeader(&tar.Header{
		Name:    manifestEntryName,
		Size:    int64(len(manifestData)),
		Mode:    0600,
		ModTime: time.Now(),
	}); err != nil {
		return nil, "", err
	}
	if _, err := tarWriter.Write(manifestData); err != nil {
		return nil, "", err
	}

	// Close writers to flush
	if err := tarWriter.Close(); err != nil {
		return nil, "", err
	}
	if err := gzWriter.Close(); err != nil {
		return nil, "", err
	}

	return writer.data, checksumStr, nil
}

// addToArchive adds file or directory to tar archive recursively, feeding every
// member path and byte into hasher so the manifest checksum covers the content
func (s *BackupService) addToArchive(tw *tar.Writer, hasher hash.Hash, sourcePath, archivePath string) error {
	info, err := os.Stat(sourcePath)
	if err != nil {
		return err
	}

	if info.IsDir() {
		return filepath.Walk(sourcePath, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}

			relPath, err := filepath.Rel(sourcePath, path)
			if err != nil {
				return err
			}
			tarPath := filepath.Join(archivePath, relPath)

			if info.IsDir() {
				hashArchiveMember(hasher, tarPath+"/")
				return tw.WriteHeader(&tar.Header{
					Name:     tarPath + "/",
					Mode:     int64(info.Mode()),
					ModTime:  info.ModTime(),
					Typeflag: tar.TypeDir,
				})
			}

			header := &tar.Header{
				Name:    tarPath,
				Size:    info.Size(),
				Mode:    int64(info.Mode()),
				ModTime: info.ModTime(),
			}

			if err := tw.WriteHeader(header); err != nil {
				return err
			}

			file, err := os.Open(path)
			if err != nil {
				return err
			}
			defer file.Close()

			hashArchiveMember(hasher, tarPath)
			_, err = io.Copy(io.MultiWriter(tw, hasher), file)
			return err
		})
	}

	// Single file
	file, err := os.Open(sourcePath)
	if err != nil {
		return err
	}
	defer file.Close()

	header := &tar.Header{
		Name:    archivePath,
		Size:    info.Size(),
		Mode:    int64(info.Mode()),
		ModTime: info.ModTime(),
	}

	if err := tw.WriteHeader(header); err != nil {
		return err
	}

	hashArchiveMember(hasher, archivePath)
	_, err = io.Copy(io.MultiWriter(tw, hasher), file)
	return err
}

// hashArchiveMember mixes a member path into the running content hash so a
// renamed or reordered entry changes the manifest checksum
func hashArchiveMember(hasher hash.Hash, name string) {
	_, _ = io.WriteString(hasher, name+"\n")
}

// memoryWriter implements io.Writer for in-memory buffer
type memoryWriter struct {
	data []byte
}

func (w *memoryWriter) Write(p []byte) (n int, err error) {
	w.data = append(w.data, p...)
	return len(p), nil
}

// encrypt encrypts data with AES-256-GCM per AI.md PART 22 (Encryption)
func (s *BackupService) encrypt(data []byte, password string) ([]byte, error) {
	// Generate salt for key derivation per AI.md PART 22 (Encryption)
	salt := make([]byte, 32)
	if _, err := rand.Read(salt); err != nil {
		return nil, err
	}

	// Derive key using Argon2id per AI.md PART 22 (Encryption)
	// Parameters: time=1, memory=64MB, threads=4, keyLen=32 (256 bits)
	key := argon2.IDKey([]byte(password), salt, 1, 64*1024, 4, 32)

	// Create AES-256-GCM cipher per AI.md PART 22 (Encryption)
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	// Generate nonce
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, err
	}

	// Encrypt data
	ciphertext := gcm.Seal(nil, nonce, data, nil)

	// Format: salt(32) + nonce(12) + ciphertext
	result := make([]byte, 0, len(salt)+len(nonce)+len(ciphertext))
	result = append(result, salt...)
	result = append(result, nonce...)
	result = append(result, ciphertext...)

	return result, nil
}

// Verify verifies backup integrity per AI.md PART 22 (Verification)
func (s *BackupService) Verify(backupPath, password string) error {
	// File exists check per AI.md PART 22 (Verification)
	info, err := os.Stat(backupPath)
	if err != nil {
		return fmt.Errorf("backup file does not exist: %w", err)
	}

	// Size > 0 check per AI.md PART 22 (Verification)
	if info.Size() == 0 {
		return fmt.Errorf("backup file is empty")
	}

	// Read file
	data, err := os.ReadFile(backupPath)
	if err != nil {
		return fmt.Errorf("failed to read backup: %w", err)
	}

	// Decrypt test if encrypted per AI.md PART 22 (Verification: decrypt test).
	// Encryption is keyed off the .enc extension alone: a password supplied for
	// a plaintext archive must not turn into a bogus decrypt failure.
	if filepath.Ext(backupPath) == ".enc" {
		if password == "" {
			return fmt.Errorf("backup is encrypted but no password provided")
		}

		plaintext, decErr := s.decrypt(data, password)
		if decErr != nil {
			return fmt.Errorf("decryption failed (wrong password?): %w", decErr)
		}
		data = plaintext
	}

	// Extract every member to a temp dir, recompute the content checksum and
	// parse the manifest per AI.md PART 22 (Verification)
	extractDir, err := verifyTempDir()
	if err != nil {
		return fmt.Errorf("failed to create verification directory: %w", err)
	}
	defer os.RemoveAll(extractDir)

	manifest, contentChecksum, err := extractArchive(data, extractDir)
	if err != nil {
		return err
	}

	if manifest == nil {
		return fmt.Errorf("backup manifest is missing")
	}

	if manifest.Checksum != "" && manifest.Checksum != contentChecksum {
		return fmt.Errorf("checksum mismatch: manifest %s, archive %s", manifest.Checksum, contentChecksum)
	}

	// Database integrity per AI.md PART 22 (Verification: database integrity)
	return verifyDatabases(extractDir)
}

// verifyTempDir creates the org/project scoped temp directory used to test
// extract a backup, never a bare /tmp path
func verifyTempDir() (string, error) {
	base := filepath.Join(os.TempDir(), "webappsgo")
	if err := os.MkdirAll(base, 0700); err != nil {
		return "", err
	}

	return os.MkdirTemp(base, "wthr-verify-*")
}

// extractArchive expands a gzip tar backup into destDir, returning the parsed
// manifest and the recomputed content checksum of every non-manifest member
func extractArchive(data []byte, destDir string) (*Manifest, string, error) {
	gzReader, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, "", fmt.Errorf("backup is not a valid gzip archive: %w", err)
	}
	defer gzReader.Close()

	tarReader := tar.NewReader(gzReader)
	contentHasher := sha256.New()

	var manifest *Manifest
	for {
		header, readErr := tarReader.Next()
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			return nil, "", fmt.Errorf("backup archive is corrupt: %w", readErr)
		}

		// Reject traversal and special members before touching the filesystem
		target, pathErr := safeExtractPath(destDir, header.Name)
		if pathErr != nil {
			return nil, "", pathErr
		}

		switch header.Typeflag {
		case tar.TypeDir:
			if header.Name != manifestEntryName {
				hashArchiveMember(contentHasher, header.Name)
			}
			if mkErr := os.MkdirAll(target, 0700); mkErr != nil {
				return nil, "", mkErr
			}
		case tar.TypeReg:
			if mkErr := os.MkdirAll(filepath.Dir(target), 0700); mkErr != nil {
				return nil, "", mkErr
			}

			out, createErr := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0600)
			if createErr != nil {
				return nil, "", createErr
			}

			var writer io.Writer = out
			if header.Name != manifestEntryName {
				hashArchiveMember(contentHasher, header.Name)
				writer = io.MultiWriter(out, contentHasher)
			}

			if _, copyErr := io.Copy(writer, tarReader); copyErr != nil {
				out.Close()
				return nil, "", fmt.Errorf("failed to extract %s: %w", header.Name, copyErr)
			}
			out.Close()

			if header.Name == manifestEntryName {
				manifestData, readManifestErr := os.ReadFile(target)
				if readManifestErr != nil {
					return nil, "", readManifestErr
				}

				parsed := &Manifest{}
				if jsonErr := json.Unmarshal(manifestData, parsed); jsonErr != nil {
					return nil, "", fmt.Errorf("manifest is not readable: %w", jsonErr)
				}
				manifest = parsed
			}
		default:
			return nil, "", fmt.Errorf("backup contains an unsupported entry type for %s", header.Name)
		}
	}

	return manifest, fmt.Sprintf("sha256:%s", hex.EncodeToString(contentHasher.Sum(nil))), nil
}

// safeExtractPath resolves an archive member against destDir and rejects any
// path that would escape it
func safeExtractPath(destDir, name string) (string, error) {
	if strings.Contains(name, "\x00") {
		return "", fmt.Errorf("backup contains an invalid entry name")
	}

	cleaned := filepath.Clean(filepath.Join(destDir, name))
	if cleaned != destDir && !strings.HasPrefix(cleaned, destDir+string(os.PathSeparator)) {
		return "", fmt.Errorf("backup entry %s escapes the extraction directory", name)
	}

	return cleaned, nil
}

// verifyDatabases runs a SQLite integrity check over every extracted database
// so a corrupt db can never be counted as a good backup
func verifyDatabases(root string) error {
	return filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() || filepath.Ext(path) != ".db" {
			return nil
		}

		db, openErr := sql.Open("sqlite", path)
		if openErr != nil {
			return fmt.Errorf("failed to open %s: %w", filepath.Base(path), openErr)
		}
		defer db.Close()

		var result string
		if queryErr := db.QueryRow("PRAGMA integrity_check").Scan(&result); queryErr != nil {
			return fmt.Errorf("integrity check failed for %s: %w", filepath.Base(path), queryErr)
		}
		if result != "ok" {
			return fmt.Errorf("integrity check failed for %s: %s", filepath.Base(path), result)
		}

		return nil
	})
}

// decrypt decrypts AES-256-GCM encrypted data
func (s *BackupService) decrypt(data []byte, password string) ([]byte, error) {
	if len(data) < 32+12 {
		return nil, fmt.Errorf("encrypted data too short")
	}

	// Extract salt, nonce, ciphertext
	salt := data[:32]
	nonce := data[32:44]
	ciphertext := data[44:]

	// Derive key using same parameters as encryption
	key := argon2.IDKey([]byte(password), salt, 1, 64*1024, 4, 32)

	// Create cipher
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	// Decrypt
	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, fmt.Errorf("decryption failed: %w", err)
	}

	return plaintext, nil
}
