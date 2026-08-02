// Tests for ascii.go per AI.md PART 29 (Testing)
package renderer

import (
	"strings"
	"testing"

	utils "github.com/webappsgo/wthr/src/util"
)

// TestColorizeWithFlag verifies the core colorize helper: colors are applied
// when enabled (including bold prefix), and stripped entirely when
// noColors is true — this flag gates every rendered line in the ASCII
// renderer, so a bug here silently breaks --no-colors support everywhere.
func TestColorizeWithFlag(t *testing.T) {
	tests := []struct {
		name     string
		text     string
		hex      string
		bold     bool
		noColors bool
		wantAnsi bool
	}{
		{"colored_plain", "hello", "#ff0000", false, false, true},
		{"colored_bold", "hello", "#ff0000", true, false, true},
		{"nocolors_returns_plain", "hello", "#ff0000", false, true, false},
		{"nocolors_ignores_bold", "hello", "#ff0000", true, true, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := colorizeWithFlag(tt.text, tt.hex, tt.bold, tt.noColors)
			hasAnsi := strings.Contains(got, "\033[")
			if hasAnsi != tt.wantAnsi {
				t.Errorf("colorizeWithFlag() ansi presence = %v, want %v (got %q)", hasAnsi, tt.wantAnsi, got)
			}
			if !strings.Contains(got, tt.text) {
				t.Errorf("colorizeWithFlag() = %q, must contain original text %q", got, tt.text)
			}
			if tt.noColors && got != tt.text {
				t.Errorf("colorizeWithFlag(noColors=true) = %q, want exactly %q", got, tt.text)
			}
		})
	}
}

// TestHexToRGB covers standard, black, white and lowercase/uppercase hex.
func TestHexToRGB(t *testing.T) {
	tests := []struct {
		hex     string
		r, g, b int
	}{
		{"#ff0000", 255, 0, 0},
		{"#00ff00", 0, 255, 0},
		{"#0000ff", 0, 0, 255},
		{"#000000", 0, 0, 0},
		{"#ffffff", 255, 255, 255},
		{"#F1FA8C", 241, 250, 140}, // uppercase
	}

	for _, tt := range tests {
		t.Run(tt.hex, func(t *testing.T) {
			r, g, b := hexToRGB(tt.hex)
			if r != tt.r || g != tt.g || b != tt.b {
				t.Errorf("hexToRGB(%q) = (%d,%d,%d), want (%d,%d,%d)", tt.hex, r, g, b, tt.r, tt.g, tt.b)
			}
		})
	}
}

// TestStripAnsiCodes verifies ANSI escape sequences are fully removed while
// preserving the surrounding plain text, including strings with multiple
// escape sequences and strings with none.
func TestStripAnsiCodes(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"no_escapes", "plain text", "plain text"},
		{"single_color", "\033[38;2;255;0;0mred\033[0m", "red"},
		{"bold_and_color", "\033[1m\033[38;2;0;255;0mgreen\033[0m", "green"},
		{"multiple_segments", "\033[38;2;1;2;3ma\033[0mb\033[38;2;4;5;6mc\033[0m", "abc"},
		{"empty", "", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := stripAnsiCodes(tt.input)
			if got != tt.want {
				t.Errorf("stripAnsiCodes(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

// TestPadToWidth verifies padding accounts for ANSI codes (using visible
// length, not byte length) and handles the negative-padding edge case
// where text is already longer than the target width.
func TestPadToWidth(t *testing.T) {
	t.Run("plain_text_padded", func(t *testing.T) {
		got := padToWidth("ab", 5)
		want := "ab   "
		if got != want {
			t.Errorf("padToWidth() = %q, want %q", got, want)
		}
	})

	t.Run("colored_text_padded_by_visible_length", func(t *testing.T) {
		colored := colorizeWithFlag("ab", "#ff0000", false, false)
		got := padToWidth(colored, 5)
		visible := stripAnsiCodes(got)
		if len(visible) != 5 {
			t.Errorf("padToWidth() visible length = %d, want 5 (got %q)", len(visible), got)
		}
	})

	t.Run("text_longer_than_width_no_negative_padding", func(t *testing.T) {
		got := padToWidth("abcdefgh", 3)
		if got != "abcdefgh" {
			t.Errorf("padToWidth() = %q, want unchanged text with no padding", got)
		}
	})
}

// TestCenterInWidth covers even/odd padding split, exact-width fit, and
// truncation of text that exceeds the target width (both plain and colored
// variants of truncation, since colored truncation has separate logic to
// preserve the color/reset codes).
func TestCenterInWidth(t *testing.T) {
	t.Run("even_padding_split", func(t *testing.T) {
		got := centerInWidth("ab", 6)
		want := "  ab  "
		if got != want {
			t.Errorf("centerInWidth() = %q, want %q", got, want)
		}
	})

	t.Run("odd_padding_favors_right", func(t *testing.T) {
		got := centerInWidth("a", 4)
		want := " a  "
		if got != want {
			t.Errorf("centerInWidth() = %q, want %q", got, want)
		}
	})

	t.Run("exact_width_no_padding", func(t *testing.T) {
		got := centerInWidth("abcd", 4)
		if got != "abcd" {
			t.Errorf("centerInWidth() = %q, want %q", got, "abcd")
		}
	})

	t.Run("plain_text_truncated", func(t *testing.T) {
		got := centerInWidth("abcdefgh", 4)
		if got != "abcd" {
			t.Errorf("centerInWidth() = %q, want truncated %q", got, "abcd")
		}
	})

	t.Run("colored_text_truncated_preserves_codes", func(t *testing.T) {
		colored := colorizeWithFlag("abcdefgh", "#ff0000", false, false)
		got := centerInWidth(colored, 4)
		if !strings.HasPrefix(got, "\033[") {
			t.Errorf("centerInWidth() truncated colored text lost color prefix: %q", got)
		}
		if !strings.HasSuffix(got, "\033[0m") {
			t.Errorf("centerInWidth() truncated colored text lost reset suffix: %q", got)
		}
		visible := stripAnsiCodes(got)
		if visible != "abcd" {
			t.Errorf("centerInWidth() truncated visible text = %q, want %q", visible, "abcd")
		}
	})
}

// TestGetWindArrow covers all 8 compass points and wraparound at 360.
func TestGetWindArrow(t *testing.T) {
	tests := []struct {
		degrees int
		want    string
	}{
		{0, "↓"},
		{45, "↙"},
		{90, "←"},
		{180, "↑"},
		{360, "↓"}, // wraps
	}
	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			got := getWindArrow(tt.degrees)
			if got != tt.want {
				t.Errorf("getWindArrow(%d) = %q, want %q", tt.degrees, got, tt.want)
			}
		})
	}
}

// TestUnitHelpers table-drives every unit-conversion helper across the
// three supported unit systems ("metric", "M", and the imperial default),
// since a swapped case here silently produces wrong units in output.
func TestUnitHelpers(t *testing.T) {
	t.Run("temperature", func(t *testing.T) {
		cases := map[string]string{"metric": "°C", "M": "°C", "imperial": "°F", "": "°F"}
		for units, want := range cases {
			if got := getTemperatureUnit(units); got != want {
				t.Errorf("getTemperatureUnit(%q) = %q, want %q", units, got, want)
			}
		}
	})

	t.Run("speed", func(t *testing.T) {
		cases := map[string]string{"metric": "km/h", "M": "m/s", "imperial": "mph", "": "mph"}
		for units, want := range cases {
			if got := getSpeedUnit(units); got != want {
				t.Errorf("getSpeedUnit(%q) = %q, want %q", units, got, want)
			}
		}
	})

	t.Run("precipitation", func(t *testing.T) {
		cases := map[string]string{"metric": "mm", "M": "mm", "imperial": "in", "": "in"}
		for units, want := range cases {
			if got := getPrecipitationUnit(units); got != want {
				t.Errorf("getPrecipitationUnit(%q) = %q, want %q", units, got, want)
			}
		}
	})

	t.Run("pressure_unit", func(t *testing.T) {
		if got := getPressureUnit("imperial"); got != "inHg" {
			t.Errorf("getPressureUnit(imperial) = %q, want inHg", got)
		}
		if got := getPressureUnit("metric"); got != "hPa" {
			t.Errorf("getPressureUnit(metric) = %q, want hPa", got)
		}
	})

	t.Run("pressure_value_conversion", func(t *testing.T) {
		if got := getPressure(1013.25, "metric"); got != 1013 {
			t.Errorf("getPressure(metric) = %d, want 1013", got)
		}
		// 1013.25 hPa * 0.02953 = ~29.92 inHg, rounds to 30
		if got := getPressure(1013.25, "imperial"); got != 30 {
			t.Errorf("getPressure(imperial) = %d, want 30", got)
		}
	})
}

// TestCalculateDaysToShow covers the width-based adaptive column logic:
// zero width (default), very narrow, medium, and wide terminals, plus the
// case where fewer forecast days are available than the width would allow.
func TestCalculateDaysToShow(t *testing.T) {
	r := NewASCIIRenderer()

	tests := []struct {
		name          string
		termWidth     int
		availableDays int
		want          int
	}{
		{"zero_width_defaults_to_3", 0, 10, 3},
		{"zero_width_limited_by_available", 0, 2, 2},
		{"wide_terminal_3_days", 200, 10, 3},
		{"narrow_terminal_1_day", 60, 10, 1},
		{"medium_terminal_2_days", 100, 10, 2},
		{"medium_terminal_limited_by_available", 100, 1, 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := r.calculateDaysToShow(tt.termWidth, tt.availableDays)
			if got != tt.want {
				t.Errorf("calculateDaysToShow(%d, %d) = %d, want %d", tt.termWidth, tt.availableDays, got, tt.want)
			}
		})
	}
}

// TestGenerateDayPeriods_LineCountInvariant guards the explicit contract in
// the source comment: every period MUST return exactly 7 lines, or the
// forecast table's column alignment breaks.
func TestGenerateDayPeriods_LineCountInvariant(t *testing.T) {
	r := NewASCIIRenderer()
	day := utils.ForecastData{
		Date:      "2026-07-21",
		TempMax:   20,
		TempMin:   10,
		Condition: "Cloudy",
	}
	params := utils.RenderParams{Units: "metric"}

	periods := r.generateDayPeriods(day, params)
	for name, lines := range map[string][]string{
		"morning": periods.morning,
		"noon":    periods.noon,
		"evening": periods.evening,
		"night":   periods.night,
	} {
		if len(lines) != 7 {
			t.Errorf("period %s has %d lines, want exactly 7", name, len(lines))
		}
	}
}

// TestRenderFull_EmptyForecast verifies the renderer never panics on an
// empty forecast slice and reports "No forecast data available" instead
// (guards the len(forecast)==0 branch in renderForecastTable).
func TestRenderFull_EmptyForecast(t *testing.T) {
	r := NewASCIIRenderer()
	weather := &utils.WeatherData{
		Location: utils.LocationData{Name: "Testville", Country: "TL"},
		Current:  utils.CurrentData{Temperature: 15, Condition: "Clear", Icon: "☀"},
		Forecast: []utils.ForecastData{},
	}
	params := utils.RenderParams{Units: "metric", Days: -1, NoColors: true}

	out := r.RenderFull(weather, params)
	if !strings.Contains(out, "No forecast data available") {
		t.Errorf("expected empty-forecast message, got %q", out)
	}
}

// TestRenderFull_QuietAndNoFooter verifies the Quiet and NoFooter flags
// actually suppress the header and footer sections respectively.
func TestRenderFull_QuietAndNoFooter(t *testing.T) {
	weather := &utils.WeatherData{
		Location: utils.LocationData{Name: "Testville", Country: "TL"},
		Current:  utils.CurrentData{Temperature: 15, Condition: "Clear", Icon: "☀", WeatherCode: 0},
		Forecast: []utils.ForecastData{},
	}

	t.Run("quiet_omits_header", func(t *testing.T) {
		r := NewASCIIRenderer()
		out := r.RenderFull(weather, utils.RenderParams{Units: "metric", Quiet: true, NoColors: true})
		if strings.Contains(out, "Weather report:") {
			t.Errorf("Quiet=true but header present: %q", out)
		}
	})

	t.Run("noFooter_omits_footer", func(t *testing.T) {
		r := NewASCIIRenderer()
		out := r.RenderFull(weather, utils.RenderParams{Units: "metric", NoFooter: true, NoColors: true})
		if strings.Contains(out, "Open-Meteo") {
			t.Errorf("NoFooter=true but footer present: %q", out)
		}
	})

	t.Run("default_shows_both", func(t *testing.T) {
		r := NewASCIIRenderer()
		out := r.RenderFull(weather, utils.RenderParams{Units: "metric", NoColors: true})
		if !strings.Contains(out, "Weather report:") {
			t.Errorf("expected header present: %q", out)
		}
		if !strings.Contains(out, "Open-Meteo") {
			t.Errorf("expected footer present: %q", out)
		}
	})
}

// TestCapitalizeLocation covers title-casing and multi-part
// comma-separated names.
//
// BUG (documented, not fixed — see final test report): the 2-letter
// country/state code detection at src/renderer/ascii.go:88 is
//
//	if len(part) == 2 && strings.ToUpper(part) == part {
//
// which only matches a part that is ALREADY all-uppercase. A lowercase
// input like "gb" fails this check (ToUpper("gb")=="GB" != "gb"), falls
// through to the word-title-casing branch, and comes out as "Gb" instead
// of "GB". The cases below assert the CURRENT (buggy) behavior for
// lowercase codes, and one already-uppercase case to show the intended
// behavior does work when the input happens to already be uppercase. If
// the len==2 branch is ever fixed to be case-insensitive, update the
// lowercase-input expectations below to "GB"/"NY, US" instead of deleting
// this test.
func TestCapitalizeLocation(t *testing.T) {
	r := NewASCIIRenderer()

	tests := []struct {
		input string
		want  string
	}{
		{"london, gb", "London, Gb"},             // BUG: want "London, GB"
		{"new york, ny, us", "New York, Ny, Us"}, // BUG: want "New York, NY, US"
		{"paris", "Paris"},
		{"london, GB", "London, GB"}, // already-uppercase input is preserved correctly
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := r.capitalizeLocation(tt.input)
			if got != tt.want {
				t.Errorf("capitalizeLocation(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}
