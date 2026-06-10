//go:build windows

package theme

import (
	"golang.org/x/sys/windows/registry"
)

// detectSystemDarkTheme returns true when Windows dark mode is active.
// Reads AppsUseLightTheme from the Personalize registry key (0=dark, 1=light).
// Defaults to true (dark) when the registry is unavailable.
func detectSystemDarkTheme() bool {
	key, err := registry.OpenKey(registry.CURRENT_USER,
		`Software\Microsoft\Windows\CurrentVersion\Themes\Personalize`,
		registry.QUERY_VALUE,
	)
	if err != nil {
		return true
	}
	defer key.Close()

	val, _, err := key.GetIntegerValue("AppsUseLightTheme")
	if err != nil {
		return true
	}
	// 0 = dark apps, 1 = light apps.
	return val == 0
}
