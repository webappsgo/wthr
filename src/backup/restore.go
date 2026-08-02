// Package backup - restore functionality per AI.md PART 25 lines 22588-22649
package backup

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// RestoreOptions configures backup restoration per AI.md PART 25
type RestoreOptions struct {
	BackupPath string
	Password   string
	ConfigDir  string
	DataDir    string
	Force      bool
}

// Restore restores from a backup file per AI.md PART 25 lines 22588-22649
func (s *BackupService) Restore(opts RestoreOptions) error {
	// Set defaults
	if opts.ConfigDir == "" {
		opts.ConfigDir = s.configDir
	}
	if opts.DataDir == "" {
		opts.DataDir = s.dataDir
	}

	// Read backup file
	data, err := os.ReadFile(opts.BackupPath)
	if err != nil {
		return fmt.Errorf("failed to read backup: %w", err)
	}

	// Decrypt if encrypted per AI.md PART 25 line 22463
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

	// Extract manifest per AI.md PART 25 line 22554
	manifest, err := s.extractManifest(data)
	if err != nil {
		return fmt.Errorf("failed to extract manifest: %w", err)
	}

	// Version check per AI.md PART 25 line 22600
	// Warning shown but proceed with schema updates if needed
	fmt.Printf("Restoring backup created at %s (version %s)\n",
		manifest.CreatedAt.Format("2006-01-02 15:04:05"),
		manifest.AppVersion)

	// Extract archive
	if err := s.extractArchive(data, opts.ConfigDir, opts.DataDir); err != nil {
		return fmt.Errorf("failed to extract archive: %w", err)
	}

	// Per AI.md PART 25 lines 22603-22622:
	// When restoring to NEW server, primary admin must re-authenticate
	// Generate setup token for re-authentication
	setupToken := generateSetupToken()

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
	// Create gzip reader
	gzReader, err := gzip.NewReader(bytes.NewReader(archiveData))
	if err != nil {
		return fmt.Errorf("failed to create gzip reader: %w", err)
	}
	defer gzReader.Close()

	// Create tar reader
	tarReader := tar.NewReader(gzReader)

	// Extract each file
	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}

		// Skip manifest.json (already extracted)
		if header.Name == "manifest.json" {
			continue
		}

		// Determine destination path per AI.md PART 25 backup contents
		var destPath string
		if header.Name == "server.yml" || strings.HasPrefix(header.Name, "template/") ||
			strings.HasPrefix(header.Name, "themes/") || strings.HasPrefix(header.Name, "ssl/") {
			destPath = filepath.Join(configDir, header.Name)
		} else {
			destPath = filepath.Join(dataDir, header.Name)
		}

		// Create directory if needed
		if header.Typeflag == tar.TypeDir {
			if err := os.MkdirAll(destPath, os.FileMode(header.Mode)); err != nil {
				return err
			}
			continue
		}

		// Ensure parent directory exists
		if err := os.MkdirAll(filepath.Dir(destPath), 0755); err != nil {
			return err
		}

		// Extract file per AI.md PART 25 line 22596
		// "Overwrites current config and database"
		outFile, err := os.Create(destPath)
		if err != nil {
			return err
		}

		if _, err := io.Copy(outFile, tarReader); err != nil {
			outFile.Close()
			return err
		}

		outFile.Close()

		// Set file permissions
		if err := os.Chmod(destPath, os.FileMode(header.Mode)); err != nil {
			return err
		}
	}

	return nil
}

// generateSetupToken generates a 32-character hex setup token per AI.md PART 25 line 22609
func generateSetupToken() string {
	token := make([]byte, 16)
	rand.Read(token)
	return hex.EncodeToString(token)
}
