//go:build windows

package display

import "os"

// detectPlatformDisplay detects the display on Windows.
// Windows always has a display except when running as a service (session 0).
func (e *DisplayEnv) detectPlatformDisplay() {
	// Remote Desktop sessions have a display.
	if sessionName := os.Getenv("SESSIONNAME"); sessionName != "" && sessionName != "Console" {
		e.HasDisplay = true
		e.DisplayType = "windows-rdp"
		return
	}

	// Normal interactive Windows session.
	e.HasDisplay = true
	e.DisplayType = "windows"
}
