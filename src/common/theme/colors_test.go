package theme

import (
	"testing"
)

func TestGetThemePalette(t *testing.T) {
	tests := []struct {
		name      string
		themeMode string
		want      *ThemePalette // nil means "either dark or light is acceptable"
	}{
		{"dark", "dark", &ThemePaletteDark},
		{"light", "light", &ThemePaletteLight},
		{"empty defaults to dark", "", &ThemePaletteDark},
		{"unrecognized garbage defaults to dark", "solarized", &ThemePaletteDark},
		{"auto resolves to a valid palette", "auto", nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := GetThemePalette(tt.themeMode)

			if tt.want != nil {
				if got != *tt.want {
					t.Errorf("GetThemePalette(%q) = %+v, want %+v", tt.themeMode, got, *tt.want)
				}
				return
			}

			// "auto" depends on IsSystemDarkTheme(); accept either known palette.
			if got != ThemePaletteDark && got != ThemePaletteLight {
				t.Errorf("GetThemePalette(%q) = %+v, want either ThemePaletteDark or ThemePaletteLight", tt.themeMode, got)
			}
		})
	}
}

// TestThemePaletteFieldsPopulated guards against a future edit that adds a
// field to ThemePalette but forgets to populate it in one of the two
// built-in palettes, leaving a silent empty-string hex value.
func TestThemePaletteFieldsPopulated(t *testing.T) {
	tests := []struct {
		name    string
		palette ThemePalette
	}{
		{"dark", ThemePaletteDark},
		{"light", ThemePaletteLight},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := tt.palette
			fields := map[string]string{
				"Background": p.Background,
				"Foreground": p.Foreground,
				"Primary":    p.Primary,
				"Secondary":  p.Secondary,
				"Accent":     p.Accent,
				"Success":    p.Success,
				"Warning":    p.Warning,
				"Error":      p.Error,
				"Info":       p.Info,
				"Surface":    p.Surface,
				"SurfaceAlt": p.SurfaceAlt,
				"Border":     p.Border,
				"Muted":      p.Muted,
			}
			for name, val := range fields {
				if val == "" {
					t.Errorf("%s.%s is empty, want a populated hex color", tt.name, name)
				}
				if val != "" && val[0] != '#' {
					t.Errorf("%s.%s = %q, want a hex color starting with '#'", tt.name, name, val)
				}
			}
		})
	}
}

func TestGetTerminalPalette(t *testing.T) {
	tests := []struct {
		name      string
		themeMode string
		want      *TerminalPalette // nil means "either dark or light is acceptable"
	}{
		{"dark", "dark", &TerminalPaletteDark},
		{"light", "light", &TerminalPaletteLight},
		{"empty defaults via system detection", "", nil},
		{"unrecognized garbage defaults via system detection", "solarized", nil},
		{"auto resolves to a valid palette", "auto", nil},
		{"system resolves to a valid palette", "system", nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := GetTerminalPalette(tt.themeMode)

			if tt.want != nil {
				if got != *tt.want {
					t.Errorf("GetTerminalPalette(%q) = %+v, want %+v", tt.themeMode, got, *tt.want)
				}
				return
			}

			// depends on IsSystemDarkTheme(); accept either known palette.
			if got != TerminalPaletteDark && got != TerminalPaletteLight {
				t.Errorf("GetTerminalPalette(%q) = %+v, want either TerminalPaletteDark or TerminalPaletteLight", tt.themeMode, got)
			}
		})
	}
}

// TestIsSystemDarkTheme verifies the function runs without panicking and is
// idempotent (repeated calls in the same process return the same value,
// since neither gsettings nor defaults output should change mid-test-run).
func TestIsSystemDarkTheme(t *testing.T) {
	first := IsSystemDarkTheme()
	second := IsSystemDarkTheme()

	t.Logf("IsSystemDarkTheme() = %v", first)

	if first != second {
		t.Errorf("IsSystemDarkTheme() not idempotent: first=%v second=%v", first, second)
	}
}
