//go:build !windows

package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"syscall"
)

func replaceClientBinary(currentPath, newPath string) error {
	info, err := os.Stat(currentPath)
	if err != nil {
		return err
	}
	if err := os.Chmod(newPath, info.Mode()); err != nil {
		return err
	}
	if err := os.Rename(newPath, currentPath); err == nil {
		return nil
	} else if !errors.Is(err, syscall.EXDEV) {
		return err
	}
	newFile, err := os.Open(newPath)
	if err != nil {
		return err
	}
	defer newFile.Close()
	staged, err := os.CreateTemp(filepath.Dir(currentPath), ".wthr-update-*")
	if err != nil {
		return err
	}
	stagedPath := staged.Name()
	defer os.Remove(stagedPath)
	if _, err := io.Copy(staged, newFile); err != nil {
		_ = staged.Close()
		return err
	}
	if err := staged.Sync(); err != nil {
		_ = staged.Close()
		return err
	}
	if err := staged.Chmod(info.Mode()); err != nil {
		_ = staged.Close()
		return err
	}
	if err := staged.Close(); err != nil {
		return fmt.Errorf("close staged update: %w", err)
	}
	if err := os.Rename(stagedPath, currentPath); err != nil {
		return err
	}
	if err := os.Remove(newPath); err != nil {
		return fmt.Errorf("remove downloaded update: %w", err)
	}
	return nil
}

func restartClient() error {
	executable, err := os.Executable()
	if err != nil {
		return err
	}
	environment := append(os.Environ(), "WTHR_CLI_UPDATE_REEXEC=1")
	return syscall.Exec(executable, os.Args, environment)
}
