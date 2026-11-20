//go:build !windows

package main

// runAsWindowsService reports whether the process runs under the Windows Service Control
// Manager. It never does on this platform, so the server starts directly.
func runAsWindowsService(run func()) bool {
	return false
}
