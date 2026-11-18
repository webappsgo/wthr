package util

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

const (
	// privateDirMode and privateFileMode apply to every private path in root AND user mode
	privateDirMode  os.FileMode = 0o700
	privateFileMode os.FileMode = 0o600
)

// privatePaths lists the private paths below the config and data directories (AI.md
// PART 8 "Directory Validation Rules"): each one, and everything below it, is owner-only.
func privatePaths(configDir, dataDir string) []string {
	return []string{
		filepath.Join(configDir, "server.yml"),
		filepath.Join(configDir, "yaml.d"),
		filepath.Join(configDir, "ssl"),
		filepath.Join(configDir, "tor"),
		filepath.Join(configDir, "i2p"),
		filepath.Join(dataDir, "security"),
		filepath.Join(dataDir, "tor"),
	}
}

// EnforcePrivatePaths sets directories to 0700 and files to 0600 on every private path
// that exists, with everything below it. A wrong mode is fixed, never reported as a
// config load failure; only a path that cannot be fixed stops startup, and the error
// names the path and never any content. It runs on every start, before the first read
// of server.yml and before any child process is spawned.
func EnforcePrivatePaths(configDir, dataDir string) error {
	for _, root := range privatePaths(configDir, dataDir) {
		if _, err := os.Lstat(root); err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return fmt.Errorf("private path %s: %w", root, err)
		}

		err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if entry.Type()&fs.ModeSymlink != 0 {
				return nil
			}
			mode := privateFileMode
			if entry.IsDir() {
				mode = privateDirMode
			}
			return os.Chmod(path, mode)
		})
		if err != nil {
			return fmt.Errorf("private path %s cannot be made private: %w", root, err)
		}
	}
	return nil
}
