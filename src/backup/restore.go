// Package backup - restore functionality per AI.md PART 22 (Restore)
package backup

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/webappsgo/wthr/src/util"
)

// RestoreOptions configures backup restoration per AI.md PART 22
type RestoreOptions struct {
	BackupPath string
	Password   string
	ConfigDir  string
	DataDir    string
	Force      bool
	Limits     RestoreLimits
}

// RestoreLimits bounds archive input and extraction work before any restore
// destination is modified.
type RestoreLimits struct {
	MaxArchiveSize  int64
	MaxExpandedSize int64
	MaxFiles        int
	MaxFileSize     int64
	MaxRatio        int64
}

func defaultRestoreLimits() RestoreLimits {
	return RestoreLimits{MaxArchiveSize: 1 << 30, MaxExpandedSize: 4 << 30, MaxFiles: 10000, MaxFileSize: 512 << 20, MaxRatio: 100}
}

func (l RestoreLimits) withDefaults() RestoreLimits {
	d := defaultRestoreLimits()
	if l.MaxArchiveSize <= 0 {
		l.MaxArchiveSize = d.MaxArchiveSize
	}
	if l.MaxExpandedSize <= 0 {
		l.MaxExpandedSize = d.MaxExpandedSize
	}
	if l.MaxFiles <= 0 {
		l.MaxFiles = d.MaxFiles
	}
	if l.MaxFileSize <= 0 {
		l.MaxFileSize = d.MaxFileSize
	}
	if l.MaxRatio <= 0 {
		l.MaxRatio = d.MaxRatio
	}
	return l
}

func maxExpandedByRatio(archiveSize, ratio int64) int64 {
	maxInt64 := int64(^uint64(0) >> 1)
	if archiveSize <= 0 || ratio <= 0 {
		return 0
	}
	if archiveSize > maxInt64/ratio {
		return maxInt64
	}
	return archiveSize * ratio
}

// Restore restores from a backup file per AI.md PART 22 (Restore Verification,
// Restore Behavior). Verification runs first and a failure aborts the restore
// before anything on disk is touched.
func (s *BackupService) Restore(opts RestoreOptions) error {
	limits := opts.Limits.withDefaults()
	// Set defaults
	if opts.ConfigDir == "" {
		opts.ConfigDir = s.configDir
	}
	if opts.DataDir == "" {
		opts.DataDir = s.dataDir
	}

	// Read once, then verify and restore these exact bytes to avoid a
	// verification-to-use race if the archive path changes during restore.
	data, err := readBackupBounded(opts.BackupPath, limits.MaxArchiveSize)
	if err != nil {
		return fmt.Errorf("failed to read backup: %w", err)
	}
	// Per AI.md PART 22 Restore Verification: "Only proceed with restore if ALL
	// verification checks pass." Verify covers format, decryption, checksum,
	// manifest parsing, and database integrity before the destination is touched.
	if err := s.verifyDataWithLimits(opts.BackupPath, opts.Password, data, limits); err != nil {
		return fmt.Errorf("backup verification failed, restore aborted: %w", err)
	}

	// Decrypt if encrypted per AI.md PART 22 (Encryption)
	if filepath.Ext(opts.BackupPath) == ".enc" {
		if opts.Password == "" {
			return fmt.Errorf("backup is encrypted, password required")
		}
		decrypted, err := s.decrypt(data, opts.Password)
		if err != nil {
			return fmt.Errorf("decryption failed: %w", err)
		}
		data = decrypted
	}

	// Extract manifest per AI.md PART 22 (Verification)
	manifest, err := s.extractManifest(data)
	if err != nil {
		return fmt.Errorf("failed to extract manifest: %w", err)
	}

	// Version check per AI.md PART 22 (Restore Behavior)
	// Warning shown but proceed with schema updates if needed
	fmt.Printf("Restoring backup created at %s (version %s)\n",
		manifest.CreatedAt.Format("2006-01-02 15:04:05"),
		manifest.AppVersion)

	// Extract archive
	if err := s.extractArchiveWithLimits(data, opts.ConfigDir, opts.DataDir, limits); err != nil {
		return fmt.Errorf("failed to extract archive: %w", err)
	}

	// Per AI.md PART 22 lines 36635-36667:
	// When restoring to NEW server, primary admin must re-authenticate
	// through a one-time setup token. Reuse the same first-run mechanism
	// (util.GenerateSetupToken/SaveSetupToken) so the printed token actually
	// authenticates against SetupTokenRequired/ValidateSetupToken instead of
	// being decorative output nothing checks.
	setupToken, err := util.GenerateSetupToken()
	if err != nil {
		return fmt.Errorf("failed to generate setup token: %w", err)
	}
	if err := util.SaveSetupToken(opts.ConfigDir, setupToken); err != nil {
		return fmt.Errorf("failed to save setup token: %w", err)
	}

	fmt.Println("\n" + strings.Repeat("=", 70))
	fmt.Println("Restore completed. Primary admin re-authentication required.")
	fmt.Println(strings.Repeat("=", 70))
	fmt.Println()
	fmt.Printf("A new setup token has been generated:\n\n")
	fmt.Printf("  Setup Token: %s\n\n", setupToken)
	fmt.Printf("Go to: https://{fqdn}:{port}/admin\n\n")
	fmt.Println("Enter the setup token to verify you are the server administrator.")
	fmt.Println("Your existing password and settings will be preserved.")
	fmt.Println(strings.Repeat("=", 70))

	return nil
}

func readBackupBounded(path string, limit int64) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > limit {
		return nil, fmt.Errorf("backup archive exceeds maximum size of %d bytes", limit)
	}
	return data, nil
}

// extractManifest reads manifest.json from archive
func (s *BackupService) extractManifest(archiveData []byte) (*Manifest, error) {
	// Create gzip reader
	gzReader, err := gzip.NewReader(bytes.NewReader(archiveData))
	if err != nil {
		return nil, fmt.Errorf("failed to create gzip reader: %w", err)
	}
	defer gzReader.Close()

	// Create tar reader
	tarReader := tar.NewReader(gzReader)

	// Find manifest.json
	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}

		if header.Name == "manifest.json" {
			manifestData, err := io.ReadAll(tarReader)
			if err != nil {
				return nil, err
			}

			var manifest Manifest
			if err := json.Unmarshal(manifestData, &manifest); err != nil {
				return nil, err
			}

			return &manifest, nil
		}
	}

	return nil, fmt.Errorf("manifest.json not found in backup")
}

// extractArchive extracts all files from archive to destination directories
func (s *BackupService) extractArchive(archiveData []byte, configDir, dataDir string) error {
	return s.extractArchiveWithLimits(archiveData, configDir, dataDir, defaultRestoreLimits())
}

func (s *BackupService) extractArchiveWithLimits(archiveData []byte, configDir, dataDir string, limits RestoreLimits) error {
	if int64(len(archiveData)) > limits.MaxArchiveSize {
		return fmt.Errorf("backup archive exceeds maximum size")
	}
	// Create gzip reader
	gzReader, err := gzip.NewReader(bytes.NewReader(archiveData))
	if err != nil {
		return fmt.Errorf("failed to create gzip reader: %w", err)
	}
	defer gzReader.Close()

	// Create tar reader
	tarReader := tar.NewReader(gzReader)

	var expanded int64
	files := 0
	maxRatioBytes := maxExpandedByRatio(int64(len(archiveData)), limits.MaxRatio)
	// Extract each file
	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		files++
		if files > limits.MaxFiles {
			return fmt.Errorf("backup archive exceeds maximum file count of %d", limits.MaxFiles)
		}
		if header.Size < 0 || header.Size > limits.MaxFileSize {
			return fmt.Errorf("archive member %s exceeds maximum file size", header.Name)
		}
		if header.Size > limits.MaxExpandedSize-expanded || header.Size > maxRatioBytes-expanded {
			return fmt.Errorf("backup archive exceeds extraction limits")
		}
		expanded += header.Size

		// Reject symlinks and hard links: an archive member that resolves to a
		// link can redirect a later write outside the destination entirely.
		if header.Typeflag == tar.TypeSymlink || header.Typeflag == tar.TypeLink {
			return fmt.Errorf("backup archive contains a link entry (%s)", header.Name)
		}
		if header.Typeflag != tar.TypeDir && header.Typeflag != tar.TypeReg && header.Typeflag != tar.TypeRegA {
			return fmt.Errorf("backup archive contains a special entry (%s)", header.Name)
		}

		// Skip manifest.json (already extracted)
		if header.Name == "manifest.json" {
			continue
		}

		// Determine destination path per AI.md PART 22 backup contents
		var destPath string
		var baseDir string
		if header.Name == "server.yml" || strings.HasPrefix(header.Name, "template/") ||
			strings.HasPrefix(header.Name, "theme/") || strings.HasPrefix(header.Name, "ssl/") {
			baseDir = configDir
		} else {
			baseDir = dataDir
		}

		// Reject traversal. The prefix tests above run against the raw member
		// name, so "../../etc/template/x" matches "template/" only after
		// escaping — safeExtractPath is what actually confines the write.
		destPath, err = safeExtractPath(baseDir, header.Name)
		if err != nil {
			return err
		}
		if err := rejectRestoreSymlinkPath(baseDir, destPath); err != nil {
			return err
		}

		// Create directory if needed
		if header.Typeflag == tar.TypeDir {
			if err := os.MkdirAll(destPath, os.FileMode(header.Mode)); err != nil {
				return err
			}
			if err := rejectRestoreSymlinkPath(baseDir, destPath); err != nil {
				return err
			}
			continue
		}

		// Ensure parent directory exists
		if err := os.MkdirAll(filepath.Dir(destPath), 0755); err != nil {
			return err
		}
		if err := rejectRestoreSymlinkPath(baseDir, destPath); err != nil {
			return err
		}

		// Extract file per AI.md PART 22 (Restore Behavior)
		// "Overwrites current config and database"
		outFile, err := os.Create(destPath)
		if err != nil {
			return err
		}

		_, copyErr := io.Copy(outFile, tarReader)
		closeErr := outFile.Close()
		if copyErr != nil {
			return copyErr
		}
		if closeErr != nil {
			return fmt.Errorf("failed to close restored file: %w", closeErr)
		}

		// Set file permissions
		if err := os.Chmod(destPath, os.FileMode(header.Mode)); err != nil {
			return err
		}
	}

	return nil
}

func rejectRestoreSymlinkPath(baseDir, targetPath string) error {
	base, err := filepath.Abs(baseDir)
	if err != nil {
		return err
	}
	target, err := filepath.Abs(targetPath)
	if err != nil {
		return err
	}
	relative, err := filepath.Rel(base, target)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return fmt.Errorf("restore path escapes its destination directory")
	}
	current := base
	paths := append([]string{current}, strings.Split(relative, string(filepath.Separator))...)
	for _, path := range paths {
		if path != current {
			current = filepath.Join(current, path)
		}
		info, err := os.Lstat(current)
		if os.IsNotExist(err) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("inspect restore path %s: %w", current, err)
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("restore path contains a symbolic link: %s", current)
		}
	}
	return nil
}
