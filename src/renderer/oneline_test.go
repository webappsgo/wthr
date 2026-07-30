// Tests for oneline.go per AI.md PART 29 (Testing)
package renderer

import (
	"strings"
	"testing"

	utils "github.com/webappsgo/wthr/src/util"
)

func sampleCurrent() utils.CurrentData {
	return utils.CurrentData{
		Temperature:   11.4,
		FeelsLike:     9.2,
		Humidity:      65,
		WindSpeed:     4.2,
		WindDirection: 0,
		Condition:     "Partly cloudy",
		Icon:          "🌦",
		WeatherCode:   2,
		Precipitation: 0.2,
	}
}

func sampleLocation() utils.LocationData {
	return utils.LocationData{
		Name:        "London",
		ShortName:   "London, GB",
		CountryCode: "GB",
	}
}

// TestRenderOneLine verifies the summary line contains the key data points
// (name, rounded temperature, condition) and terminates with the expected
// double newline used by status-bar consumers.
func TestRenderOneLine(t *testing.T) {
	r := NewOneLineRenderer()
	out := r.RenderOneLine(sampleLocation(), sampleCurrent(), "metric")

	if !strings.Contains(out, "London") {
		t.Errorf("output missing location name: %q", out)
	}
	if !strings.Contains(out, "11°C") {
		t.Errorf("output missing rounded temperature: %q", out)
	}
	if !strings.Contains(out, "Partly cloudy") {
		t.Errorf("output missing condition: %q", out)
	}
	if !strings.HasSuffix(out, "\n\n") {
		t.Errorf("output must end with double newline, got %q", out)
	}
}

// TestRenderFormat1_NoColors verifies the noColors path emits plain text
// with no ANSI escape sequences, and formats temperature with an explicit
// sign per the documented "+11°C" format.
func TestRenderFormat1_NoColors(t *testing.T) {
	r := NewOneLineRenderer()
	current := sampleCurrent()

	tests := []struct {
		name  string
		units string
		want  string
	}{
		{"metric_rounds_up", "metric", "+11°C"},
		{"imperial", "imperial", "+11°F"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out := r.RenderFormat1(current, tt.units, true)
			if strings.Contains(out, "\033[") {
				t.Errorf("noColors=true but output contains ANSI escape: %q", out)
			}
			if !strings.Contains(out, tt.want) {
				t.Errorf("output = %q, want substring %q", out, tt.want)
			}
		})
	}
}

// TestRenderFormat1_WithColors verifies the color path wraps the temperature
// in an ANSI escape sequence (regression guard for the noColors branch).
func TestRenderFormat1_WithColors(t *testing.T) {
	r := NewOneLineRenderer()
	out := r.RenderFormat1(sampleCurrent(), "metric", false)
	if !strings.Contains(out, "\033[") {
		t.Errorf("noColors=false expected ANSI escape codes, got %q", out)
	}
}

// TestRenderFormat3_ShortLocationFallback verifies that when ShortName is
// empty, getShortLocationName falls back to "Name, CC" and defaults the
// country code to "XX" when that is also missing — both are easy to break
// silently since they only trigger on partial data.
func TestRenderFormat3_ShortLocationFallback(t *testing.T) {
	r := NewOneLineRenderer()
	current := sampleCurrent()

	tests := []struct {
		name     string
		location utils.LocationData
		want     string
	}{
		{
			name:     "uses_short_name_when_present",
			location: utils.LocationData{Name: "Paris", ShortName: "Paris, FR"},
			want:     "Paris, FR",
		},
		{
			name:     "falls_back_to_name_and_country_code",
			location: utils.LocationData{Name: "Berlin", CountryCode: "DE"},
			want:     "Berlin, DE",
		},
		{
			name:     "defaults_country_code_to_XX",
			location: utils.LocationData{Name: "Nowhere"},
			want:     "Nowhere, XX",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out := r.RenderFormat3(tt.location, current, "metric", true)
			if !strings.Contains(out, tt.want) {
				t.Errorf("output = %q, want substring %q", out, tt.want)
			}
		})
	}
}

// TestGetWindDirection covers every compass boundary, including wraparound
// at 360 degrees (must map back to N, not panic on out-of-range index).
func TestGetWindDirection(t *testing.T) {
	r := NewOneLineRenderer()

	tests := []struct {
		degrees int
		want    string
	}{
		{0, "N"},
		{90, "E"},
		{180, "S"},
		{270, "W"},
		{360, "N"},   // wraparound
		{359, "N"},   // rounds up to 360 -> wraps to N
		{11, "N"},    // rounds down within N sector
		{23, "NNE"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			got := r.getWindDirection(tt.degrees)
			if got != tt.want {
				t.Errorf("getWindDirection(%d) = %q, want %q", tt.degrees, got, tt.want)
			}
		})
	}
}

// TestRenderFormat2And4_Smoke exercises the remaining two format renderers
// to ensure they produce non-empty output containing the icon and don't
// panic, covering both the colored and non-colored branches.
func TestRenderFormat2And4_Smoke(t *testing.T) {
	r := NewOneLineRenderer()
	current := sampleCurrent()
	location := sampleLocation()

	for _, noColors := range []bool{true, false} {
		out2 := r.RenderFormat2(current, "metric", noColors)
		if !strings.Contains(out2, current.Icon) {
			t.Errorf("RenderFormat2(noColors=%v) missing icon: %q", noColors, out2)
		}

		out4 := r.RenderFormat4(location, current, "metric", noColors)
		if !strings.Contains(out4, location.ShortName) {
			t.Errorf("RenderFormat4(noColors=%v) missing location: %q", noColors, out4)
		}
	}
}
