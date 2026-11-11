//go:build windows

package cli

import (
	"fmt"
	"os"
	"os/exec"

	"golang.org/x/sys/windows"
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

	if oldPathPtr, err := windows.UTF16PtrFromString(oldPath); err == nil {
		windows.MoveFileEx(oldPathPtr, nil, windows.MOVEFILE_DELAY_UNTIL_REBOOT)
	}

	return nil
}

func restartSelf() error {
	executable, err := os.Executable()
	if err != nil {
		return fmt.Errorf("failed to locate updated executable: %w", err)
	}
	command := exec.Command(executable, os.Args[1:]...)
	command.Stdin = os.Stdin
	command.Stdout = os.Stdout
	command.Stderr = os.Stderr
	return command.Start()
}
