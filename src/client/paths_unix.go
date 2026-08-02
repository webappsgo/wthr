//go:build !windows
// +build !windows

package main

import "os"

// setDirPermissions sets directory permissions on Unix systems
// Per AI.md PART 33 line 45061-45068: Unix directories use 0700 (user-only access)
func setDirPermissions(dir string) error {
	return os.Chmod(dir, 0700)
}
