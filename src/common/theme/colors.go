// Package theme provides the unified color palette per AI.md PART 16.
// All binaries (server, CLI, agent) use these palettes for consistent output.
package theme

// ThemePalette holds the resolved color tokens for one theme mode.
// Hex strings are 6-digit sRGB values without transparency (e.g. "#282a36").
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
	Background: "#282a36",
	Foreground: "#f8f8f2",
	Primary:    "#bd93f9",
	Secondary:  "#50fa7b",
	Accent:     "#ff79c6",
	Success:    "#50fa7b",
	Warning:    "#ffb86c",
	Error:      "#ff5555",
	Info:       "#8be9fd",
	Surface:    "#2b2d3a",
	SurfaceAlt: "#21222c",
	Border:     "#44475a",
	// Muted is lightened from the stock Dracula comment color (#6272a4,
	// 3.03:1 on Background) to #8591b8 (4.57:1) so normal-size muted/
	// secondary text meets WCAG 2.1 AA's 4.5:1 minimum (AI.md PART 31).
	Muted: "#8591b8",
}

// ThemePaletteLight is the light palette per AI.md PART 16.
var ThemePaletteLight = ThemePalette{
	Background: "#ffffff",
	Foreground: "#1f2328",
	Primary:    "#0969da",
	Secondary:  "#1a7f37",
	Accent:     "#8250df",
	Success:    "#1a7f37",
	Warning:    "#9a6700",
	Error:      "#d1242f",
	Info:       "#0969da",
	Surface:    "#f6f8fa",
	SurfaceAlt: "#eff2f5",
	Border:     "#d1d9e0",
	Muted:      "#59636e",
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
