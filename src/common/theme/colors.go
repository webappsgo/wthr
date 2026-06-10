// Package theme provides the unified color palette per AI.md PART 16.
// All binaries (server, CLI, agent) use these palettes for consistent output.
package theme

// ThemePalette holds the resolved color tokens for one theme mode.
// Hex strings are 6-digit sRGB values without transparency (e.g. "#1a1b26").
type ThemePalette struct {
	Background string `json:"background"`
	Foreground string `json:"foreground"`
	Primary    string `json:"primary"`
	Secondary  string `json:"secondary"`
	Accent     string `json:"accent"`
	Success    string `json:"success"`
	Warning    string `json:"warning"`
	Error      string `json:"error"`
	Info       string `json:"info"`
	Surface    string `json:"surface"`
	SurfaceAlt string `json:"surface_alt"`
	Border     string `json:"border"`
	Muted      string `json:"muted"`
}

// ThemePaletteDark is the default dark palette per AI.md PART 16.
var ThemePaletteDark = ThemePalette{
	Background: "#1a1b26",
	Foreground: "#c0caf5",
	Primary:    "#7aa2f7",
	Secondary:  "#9ece6a",
	Accent:     "#bb9af7",
	Success:    "#9ece6a",
	Warning:    "#e0af68",
	Error:      "#f7768e",
	Info:       "#7dcfff",
	Surface:    "#24283b",
	SurfaceAlt: "#1f2335",
	Border:     "#414868",
	Muted:      "#565f89",
}

// ThemePaletteLight is the light palette per AI.md PART 16.
var ThemePaletteLight = ThemePalette{
	Background: "#ffffff",
	Foreground: "#1a1b26",
	Primary:    "#2e7de9",
	Secondary:  "#587539",
	Accent:     "#7847bd",
	Success:    "#587539",
	Warning:    "#8c6c3e",
	Error:      "#c64343",
	Info:       "#007197",
	Surface:    "#f5f5f5",
	SurfaceAlt: "#e9e9ec",
	Border:     "#c0caf5",
	Muted:      "#6172b0",
}

// GetThemePalette returns the palette for the given theme mode string.
// Accepts "dark", "light", or "auto" (which falls back to dark since
// server-side system-theme detection is not yet available).
func GetThemePalette(themeMode string) ThemePalette {
	switch themeMode {
	case "light":
		return ThemePaletteLight
	case "auto":
		if IsSystemDarkTheme() {
			return ThemePaletteDark
		}
		return ThemePaletteLight
	default:
		return ThemePaletteDark
	}
}

// IsSystemDarkTheme attempts to detect whether the OS prefers a dark theme.
// Returns true when dark mode is detected or detection is not possible.
func IsSystemDarkTheme() bool {
	// Platform-specific detection is implemented in detect_*.go files.
	// Default to dark when detection cannot determine a preference.
	return detectSystemDarkTheme()
}
