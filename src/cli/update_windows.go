//go:build windows

package cli

import (
	"fmt"
	"os"
	"syscall"
)

// replaceBinary replaces the running binary. Windows refuses to overwrite a
// file that is currently executing, so the running binary is renamed aside
// first and the leftover is scheduled for removal on the next reboot.
func replaceBinary(currentPath, newBinaryPath string) error {
	oldPath := currentPath + ".old"

	os.Remove(oldPath)

	if err := os.Rename(currentPath, oldPath); err != nil {
		return fmt.Errorf("failed to rename current binary: %w", err)
	}

	if err := os.Rename(newBinaryPath, currentPath); err != nil {
		os.Rename(oldPath, currentPath)
		return fmt.Errorf("failed to move new binary: %w", err)
	}

	if oldPathPtr, err := syscall.UTF16PtrFromString(oldPath); err == nil {
		syscall.MoveFileEx(oldPathPtr, nil, syscall.MOVEFILE_DELAY_UNTIL_REBOOT)
	}

	return nil
}
