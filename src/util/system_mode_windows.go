//go:build windows

package util

import "golang.org/x/sys/windows/svc"

// detectSystemMode reports whether the process is elevated or runs as a Windows service. The
// Virtual Service Account is not an Administrator, so the service check is what selects the
// %ProgramData% directories for it.
func detectSystemMode() bool {
	if isElevatedProcess() {
		return true
	}

	isService, err := svc.IsWindowsService()

	return err == nil && isService
}
