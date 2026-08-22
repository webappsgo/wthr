// Package backup implements backup and restore functionality per AI.md PART 25
// AI.md Reference: Lines 22349-22750
package backup

import (
	"archive/tar"
	"compress/gzip"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/crypto/argon2"
)

// Manifest represents backup metadata per AI.md PART 25 line 22390
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

// BackupOptions configures backup creation per AI.md PART 25
type BackupOptions struct {
	ConfigDir   string
	DataDir     string
	OutputPath  string
	Password    string
	IncludeSSL  bool
	IncludeData bool
	CreatedBy   string
	AppVersion  string
	// Retention controls the tiered pruning sweep per AI.md PART 22
	// (Backup Retention). Nil uses DefaultRetention().
	Retention *RetentionConfig
}

// BackupService handles backup operations per AI.md PART 25
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

// Create creates a new backup per AI.md PART 25 lines 22351-22542
// Follows complete backup workflow with verification and cleanup.
// The second return value lists backup filenames deleted by the tiered
// retention sweep (AI.md PART 22's "backup.retention_cleanup" audit event) -
// callers with DB access log it, callers without (the CLI) may discard it.
func (s *BackupService) Create(opts BackupOptions) (string, []string, error) {
	// Set defaults
	if opts.ConfigDir == "" {
		opts.ConfigDir = s.configDir
	}
	if opts.DataDir == "" {
		opts.DataDir = s.dataDir
	}
	if opts.OutputPath == "" {
		// Filename format per AI.md PART 22 line 22386: wthr_backup_YYYY-MM-DD_HHMMSS.tar.gz[.enc]
		timestamp := time.Now().Format("2006-01-02_150405")
		ext := ".tar.gz"
		if opts.Password != "" {
			ext = ".tar.gz.enc"
		}
		// The directory is "backups" (plural) because that is what
		// path.Paths.BackupDir resolves to, and every reader - the admin API,
		// the CLI list and the retention sweep - looks there. This used to
		// write to "backup" (singular), so a backup created here was invisible
		// to every one of those readers and the retention sweep never saw it.
		opts.OutputPath = filepath.Join(opts.DataDir, "backups", fmt.Sprintf("wthr_backup_%s%s", timestamp, ext))
	}

	// Ensure backup directory exists
	backupDir := filepath.Dir(opts.OutputPath)
	if err := os.MkdirAll(backupDir, 0700); err != nil {
		return "", nil, fmt.Errorf("failed to create backup directory: %w", err)
	}

	// Collect files to backup per AI.md PART 25 lines 22357-22367
	files, err := s.collectFiles(opts)
	if err != nil {
		return "", nil, fmt.Errorf("failed to collect files: %w", err)
	}

	// Create manifest per AI.md PART 25 lines 22390-22408
	manifest := Manifest{
		Version:    "1.0.0",
		CreatedAt:  time.Now(),
		CreatedBy:  opts.CreatedBy,
		AppVersion: opts.AppVersion,
		Contents:   files,
		Encrypted:  opts.Password != "",
	}
	if manifest.Encrypted {
		// Per AI.md PART 25 line 22416
		manifest.EncryptionMethod = "AES-256-GCM"
	}

	// Create tar.gz archive in memory per AI.md PART 25 line 22425
	// "Unencrypted archive never touches disk"
	archiveData, checksumStr, err := s.createArchive(opts.ConfigDir, opts.DataDir, files, manifest)
	if err != nil {
		return "", nil, fmt.Errorf("failed to create archive: %w", err)
	}

	// Update manifest with checksum
	manifest.Checksum = checksumStr

	// Encrypt if password provided per AI.md PART 25 lines 22410-22427
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

	// Verify backup per AI.md PART 25 lines 22544-22556
	// "Every backup is verified immediately after creation"
	if err := s.Verify(opts.OutputPath, opts.Password); err != nil {
		// Delete failed backup per AI.md PART 25 line 22539
		os.Remove(opts.OutputPath)
		return "", nil, fmt.Errorf("backup verification failed: %w", err)
	}

	// Apply tiered retention per AI.md PART 22 (Backup Retention)
	// "Only delete old backups if new backup passes ALL verification checks"
	retention := DefaultRetention()
	if opts.Retention != nil {
		retention = *opts.Retention
	}
	totalBytes, _ := volumeTotalBytes(backupDir)
	deleted, err := applyRetention(backupDir, retention, totalBytes)
	if err != nil {
		// Log but don't fail - backup itself succeeded
		log.Printf("WARNING: failed to apply backup retention: %v", err)
	}

	return opts.OutputPath, deleted, nil
}

// collectFiles identifies files to include in backup per AI.md PART 25 lines 22357-22367
func (s *BackupService) collectFiles(opts BackupOptions) ([]string, error) {
	var files []string

	// server.yml - Always included per AI.md PART 25 line 22361
	serverYML := filepath.Join(opts.ConfigDir, "server.yml")
	if _, err := os.Stat(serverYML); err == nil {
		files = append(files, "server.yml")
	}

	// server.db - Always included per AI.md PART 25 line 22362
	serverDB := filepath.Join(opts.DataDir, "db", "server.db")
	if _, err := os.Stat(serverDB); err == nil {
		files = append(files, "db/server.db")
	}

	// users.db - If exists per AI.md PART 25 line 22363
	usersDB := filepath.Join(opts.DataDir, "db", "users.db")
	if _, err := os.Stat(usersDB); err == nil {
		files = append(files, "db/users.db")
	}

	// Custom templates - If exists per AI.md PART 25 line 22364
	templatesDir := filepath.Join(opts.ConfigDir, "template")
	if _, err := os.Stat(templatesDir); err == nil {
		files = append(files, "template/")
	}

	// Custom themes - If exists per AI.md PART 25 line 22365
	themesDir := filepath.Join(opts.ConfigDir, "themes")
	if _, err := os.Stat(themesDir); err == nil {
		files = append(files, "themes/")
	}

	// SSL certificates - Optional per AI.md PART 25 line 22366
	if opts.IncludeSSL {
		sslDir := filepath.Join(opts.ConfigDir, "ssl")
		if _, err := os.Stat(sslDir); err == nil {
			files = append(files, "ssl/")
		}
	}

	// Data files - Optional per AI.md PART 25 line 22367
	if opts.IncludeData {
		files = append(files, "data/")
	}

	return files, nil
}

// createArchive creates tar.gz archive and returns data + checksum
// Per AI.md PART 25 lines 22383-22408
func (s *BackupService) createArchive(configDir, dataDir string, files []string, manifest Manifest) ([]byte, string, error) {
	// Create in-memory buffer (unencrypted archive never touches disk per AI.md PART 25 line 22425)
	var buf []byte
	writer := &memoryWriter{data: buf}

	// Create gzip writer
	gzWriter := gzip.NewWriter(writer)
	defer gzWriter.Close()

	// Create tar writer
	tarWriter := tar.NewWriter(gzWriter)
	defer tarWriter.Close()

	// Add manifest.json first
	manifestData, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return nil, "", fmt.Errorf("failed to marshal manifest: %w", err)
	}

	if err := tarWriter.WriteHeader(&tar.Header{
		Name:    "manifest.json",
		Size:    int64(len(manifestData)),
		Mode:    0600,
		ModTime: time.Now(),
	}); err != nil {
		return nil, "", err
	}
	if _, err := tarWriter.Write(manifestData); err != nil {
		return nil, "", err
	}

	// Add each file/directory
	for _, file := range files {
		var sourcePath string
		if file == "server.yml" || strings.HasPrefix(file, "template/") ||
			strings.HasPrefix(file, "themes/") || strings.HasPrefix(file, "ssl/") {
			sourcePath = filepath.Join(configDir, file)
		} else {
			sourcePath = filepath.Join(dataDir, file)
		}

		// Skip if file doesn't exist
		if _, err := os.Stat(sourcePath); os.IsNotExist(err) {
			continue
		}

		if err := s.addToArchive(tarWriter, sourcePath, file); err != nil {
			return nil, "", fmt.Errorf("failed to add %s: %w", file, err)
		}
	}

	// Close writers to flush
	if err := tarWriter.Close(); err != nil {
		return nil, "", err
	}
	if err := gzWriter.Close(); err != nil {
		return nil, "", err
	}

	// Calculate checksum per AI.md PART 25 line 22406
	checksum := sha256.Sum256(writer.data)
	checksumStr := fmt.Sprintf("sha256:%s", hex.EncodeToString(checksum[:]))

	return writer.data, checksumStr, nil
}

// addToArchive adds file or directory to tar archive recursively
func (s *BackupService) addToArchive(tw *tar.Writer, sourcePath, archivePath string) error {
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

			_, err = io.Copy(tw, file)
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

	_, err = io.Copy(tw, file)
	return err
}

// memoryWriter implements io.Writer for in-memory buffer
type memoryWriter struct {
	data []byte
}

func (w *memoryWriter) Write(p []byte) (n int, err error) {
	w.data = append(w.data, p...)
	return len(p), nil
}

// encrypt encrypts data with AES-256-GCM per AI.md PART 25 lines 22410-22427
func (s *BackupService) encrypt(data []byte, password string) ([]byte, error) {
	// Generate salt for key derivation per AI.md PART 25 line 22417
	salt := make([]byte, 32)
	if _, err := rand.Read(salt); err != nil {
		return nil, err
	}

	// Derive key using Argon2id per AI.md PART 25 line 22417
	// Parameters: time=1, memory=64MB, threads=4, keyLen=32 (256 bits)
	key := argon2.IDKey([]byte(password), salt, 1, 64*1024, 4, 32)

	// Create AES-256-GCM cipher per AI.md PART 25 line 22416
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

// Verify verifies backup integrity per AI.md PART 25 lines 22544-22556
func (s *BackupService) Verify(backupPath, password string) error {
	// File exists check per AI.md PART 25 line 22550
	info, err := os.Stat(backupPath)
	if err != nil {
		return fmt.Errorf("backup file does not exist: %w", err)
	}

	// Size > 0 check per AI.md PART 25 line 22551
	if info.Size() == 0 {
		return fmt.Errorf("backup file is empty")
	}

	// Read file
	data, err := os.ReadFile(backupPath)
	if err != nil {
		return fmt.Errorf("failed to read backup: %w", err)
	}

	// Decrypt test if encrypted per AI.md PART 25 line 22553
	if filepath.Ext(backupPath) == ".enc" || password != "" {
		if password == "" {
			return fmt.Errorf("backup is encrypted but no password provided")
		}

		_, err := s.decrypt(data, password)
		if err != nil {
			return fmt.Errorf("decryption failed (wrong password?): %w", err)
		}
	}

	return nil
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
