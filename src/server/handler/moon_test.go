package handler

import (
	"strings"
	"testing"
	"time"
)

// TestNewMoonHandler verifies the constructor wires the weather/location
// dependencies as passed, and always constructs a non-nil moonService
// regardless of what was passed for the other two.
func TestNewMoonHandler(t *testing.T) {
	h := NewMoonHandler(nil, nil)
	if h == nil {
		t.Fatal("expected non-nil handler")
	}
	if h.weatherService != nil {
		t.Errorf("expected weatherService to be nil (as passed), got %v", h.weatherService)
	}
	if h.locationEnhancer != nil {
		t.Errorf("expected locationEnhancer to be nil (as passed), got %v", h.locationEnhancer)
	}
	if h.moonService == nil {
		t.Error("expected moonService to always be constructed, got nil")
	}
}

// TestMoonHandler_CalculateSunTimes_NormalLatitude verifies a mid-latitude
// location on an equinox-ish date produces real sunrise/sunset/day-length
// values (no polar-night/midnight-sun note).
func TestMoonHandler_CalculateSunTimes_NormalLatitude(t *testing.T) {
	h := &MoonHandler{}
	date := time.Date(2026, 3, 20, 0, 0, 0, 0, time.UTC)

	got := h.calculateSunTimes(40.0, -74.0, date)

	if got["sunrise"] == nil {
		t.Error("expected non-nil sunrise for mid-latitude location")
	}
	if got["sunset"] == nil {
		t.Error("expected non-nil sunset for mid-latitude location")
	}
	dayLength, ok := got["dayLength"].(string)
	if !ok || !strings.Contains(dayLength, "h") {
		t.Errorf("expected formatted dayLength string, got: %v", got["dayLength"])
	}
	if _, hasNote := got["note"]; hasNote {
		t.Errorf("did not expect a polar note for a mid-latitude location, got: %v", got["note"])
	}
}

// TestMoonHandler_CalculateSunTimes_PolarNight verifies a high northern
// latitude during northern-hemisphere winter reports polar night (no
// sunrise/sunset).
func TestMoonHandler_CalculateSunTimes_PolarNight(t *testing.T) {
	h := &MoonHandler{}
	date := time.Date(2026, 12, 21, 0, 0, 0, 0, time.UTC)

	got := h.calculateSunTimes(89.9, 0.0, date)

	if got["sunrise"] != nil {
		t.Errorf("expected nil sunrise during polar night, got: %v", got["sunrise"])
	}
	if got["sunset"] != nil {
		t.Errorf("expected nil sunset during polar night, got: %v", got["sunset"])
	}
	note, _ := got["note"].(string)
	if !strings.Contains(note, "Polar night") {
		t.Errorf("expected polar night note, got: %v", got["note"])
	}
}

// TestMoonHandler_CalculateSunTimes_MidnightSun verifies a high northern
// latitude during northern-hemisphere summer reports midnight sun (24h day).
func TestMoonHandler_CalculateSunTimes_MidnightSun(t *testing.T) {
	h := &MoonHandler{}
	date := time.Date(2026, 6, 21, 0, 0, 0, 0, time.UTC)

	got := h.calculateSunTimes(89.9, 0.0, date)

	if got["sunrise"] != nil {
		t.Errorf("expected nil sunrise during midnight sun, got: %v", got["sunrise"])
	}
	if got["sunset"] != nil {
		t.Errorf("expected nil sunset during midnight sun, got: %v", got["sunset"])
	}
	note, _ := got["note"].(string)
	if !strings.Contains(note, "Midnight sun") {
		t.Errorf("expected midnight sun note, got: %v", got["note"])
	}
	if got["dayLength"] != "24h 0m" {
		t.Errorf("dayLength = %v, want 24h 0m", got["dayLength"])
	}
}

// TestFormatTime covers the normal case plus min-value clamping below 0
// and above 59.
func TestFormatTime(t *testing.T) {
	cases := []struct {
		hour, min int
		want      string
	}{
		{6, 30, "06:30"},
		{6, -5, "06:00"},
		{6, 90, "06:59"},
		{0, 0, "00:00"},
	}
	for _, tc := range cases {
		if got := formatTime(tc.hour, tc.min); got != tc.want {
			t.Errorf("formatTime(%d, %d) = %q, want %q", tc.hour, tc.min, got, tc.want)
		}
	}
}

// TestFormatDuration covers the normal case plus negative-minute clamping.
func TestFormatDuration(t *testing.T) {
	cases := []struct {
		hours, mins int
		want        string
	}{
		{12, 30, "12h 30m"},
		{0, 0, "0h 0m"},
		{5, -10, "5h 0m"},
	}
	for _, tc := range cases {
		if got := formatDuration(tc.hours, tc.mins); got != tc.want {
			t.Errorf("formatDuration(%d, %d) = %q, want %q", tc.hours, tc.mins, got, tc.want)
		}
	}
}
