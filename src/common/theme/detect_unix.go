//go:build !windows

package theme

import (
	"os/exec"
	"strings"
)

// detectSystemDarkTheme returns true when the OS dark mode preference is detected.
// On Linux it checks GNOME's color-scheme setting; on macOS it checks AppleInterfaceStyle.
// Defaults to true (dark) when detection fails or is unavailable.
func detectSystemDarkTheme() bool {
	// GNOME (Linux/BSD)
	out, err := exec.Command("gsettings", "get", "org.gnome.desktop.interface", "color-scheme").Output()
	if err == nil {
		return strings.Contains(string(out), "dark")
	}

	// macOS
	out, err = exec.Command("defaults", "read", "-g", "AppleInterfaceStyle").Output()
	if err == nil {
		return strings.TrimSpace(string(out)) == "Dark"
	}

	// Cannot determine — default to dark.
	return true
}
