//go:build !windows

package cli

import (
	"fmt"
	"os"
)

// replaceBinary replaces the running binary. Unix allows renaming over a
// running executable: the old inode stays mapped for the current process and
// the new file takes effect on the next start, so the swap is atomic.
func replaceBinary(currentPath, newBinaryPath string) error {
	info, err := os.Stat(currentPath)
	if err != nil {
		return fmt.Errorf("failed to stat current binary: %w", err)
	}

	if err := os.Rename(newBinaryPath, currentPath); err != nil {
		return fmt.Errorf("failed to replace binary: %w", err)
	}

	if err := os.Chmod(currentPath, info.Mode()); err != nil {
		return fmt.Errorf("failed to restore permissions: %w", err)
	}

	return nil
}
