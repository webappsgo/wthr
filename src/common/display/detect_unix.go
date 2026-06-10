//go:build !windows

package display

import (
	"os"
	"runtime"
)

// detectPlatformDisplay detects the display server on Linux, BSD, and macOS.
func (e *DisplayEnv) detectPlatformDisplay() {
	// Wayland takes precedence over X11 on Linux/BSD.
	if os.Getenv("WAYLAND_DISPLAY") != "" {
		e.HasDisplay = true
		e.DisplayType = "wayland"
		return
	}

	// X11 fallback.
	if os.Getenv("DISPLAY") != "" {
		e.HasDisplay = true
		e.DisplayType = "x11"
		return
	}

	// macOS: native display is always available unless running over SSH or as a daemon.
	if runtime.GOOS == "darwin" && !e.IsSSH {
		// $__CFBundleIdentifier is set in Aqua sessions; absence means a daemon context.
		if os.Getenv("__CFBundleIdentifier") != "" {
			e.HasDisplay = true
			e.DisplayType = "macos"
			return
		}
	}

	e.HasDisplay = false
	e.DisplayType = "none"
}
