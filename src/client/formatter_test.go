package main

import (
	"strings"
	"testing"
)

func TestNewFormatter(t *testing.T) {
	tests := []struct {
		format  string
		noColor bool
	}{
		{"json", false},
		{"table", false},
		{"plain", false},
		{"json", true},
	}

	for _, tt := range tests {
		formatter := NewFormatter(tt.format, tt.noColor)
		if formatter.Format != tt.format {
			t.Errorf("Expected format %s, got %s", tt.format, formatter.Format)
		}
		if formatter.NoColor != tt.noColor {
			t.Errorf("Expected NoColor %t, got %t", tt.noColor, formatter.NoColor)
		}
	}
}

func TestFormatJSON(t *testing.T) {
	formatter := NewFormatter("json", false)

	data := map[string]interface{}{
		"temperature": 72.5,
		"location":    "New York",
		"condition":   "Sunny",
	}

	result := formatter.formatJSON(data)

	// Verify it's valid JSON format
	if !strings.Contains(result, "temperature") {
		t.Error("Expected temperature field in JSON output")
	}
	if !strings.Contains(result, "location") {
		t.Error("Expected location field in JSON output")
	}
	if !strings.Contains(result, "condition") {
		t.Error("Expected condition field in JSON output")
	}
	if !strings.Contains(result, "72.5") {
		t.Error("Expected temperature value in JSON output")
	}
}

func TestFormatPlainWeather(t *testing.T) {
	formatter := NewFormatter("plain", false)

	data := map[string]interface{}{
		"location":    "New York, NY",
		"temperature": 72.5,
		"feels_like":  70.0,
		"condition":   "Sunny",
		"humidity":    60.0,
		"wind_speed":  10.5,
	}

	result := formatter.formatPlainWeather(data)

	// Verify all fields are present
	expectedStrings := []string{
		"Location: New York, NY",
		"Temperature: 72.5°F",
		"Feels Like: 70.0°F",
		"Condition: Sunny",
		"Humidity: 60%",
		"Wind Speed: 10.5 mph",
	}

	for _, expected := range expectedStrings {
		if !strings.Contains(result, expected) {
			t.Errorf("Expected output to contain: %s", expected)
		}
	}
}

func TestFormatTableWeather(t *testing.T) {
	formatter := NewFormatter("table", false)

	data := map[string]interface{}{
		"location":    "New York, NY",
		"temperature": 72.5,
		"feels_like":  70.0,
		"condition":   "Sunny",
		"humidity":    60.0,
		"wind_speed":  10.5,
	}

	result := formatter.formatTableWeather(data)

	// Verify table structure
	if !strings.Contains(result, "┌") || !strings.Contains(result, "┐") {
		t.Error("Expected table border characters")
	}
	if !strings.Contains(result, "New York, NY") {
		t.Error("Expected location in table")
	}
	if !strings.Contains(result, "72.5°F") {
		t.Error("Expected temperature in table")
	}
}

func TestFormatWeatherCurrent(t *testing.T) {
	data := map[string]interface{}{
		"location":    "Test City",
		"temperature": 75.0,
	}

	tests := []struct {
		format string
	}{
		{"json"},
		{"table"},
		{"plain"},
	}

	for _, tt := range tests {
		t.Run(tt.format, func(t *testing.T) {
			formatter := NewFormatter(tt.format, false)
			result := formatter.FormatWeatherCurrent(data)

			if result == "" {
				t.Error("Expected non-empty output")
			}

			// Verify format-specific characteristics
			switch tt.format {
			case "json":
				if !strings.Contains(result, "{") {
					t.Error("JSON output should contain braces")
				}
			case "table":
				if !strings.Contains(result, "┌") {
					t.Error("Table output should contain box drawing characters")
				}
			case "plain":
				if !strings.Contains(result, "Location:") {
					t.Error("Plain output should contain field labels")
				}
			}
		})
	}
}

func TestFormatForecast(t *testing.T) {
	data := map[string]interface{}{
		"location": "Test City",
		"forecast": []interface{}{
			map[string]interface{}{
				"date":      "2025-12-24",
				"high":      80.0,
				"low":       65.0,
				"condition": "Sunny",
			},
			map[string]interface{}{
				"date":      "2025-12-25",
				"high":      75.0,
				"low":       60.0,
				"condition": "Cloudy",
			},
		},
	}

	tests := []struct {
		format string
	}{
		{"json"},
		{"table"},
		{"plain"},
	}

	for _, tt := range tests {
		t.Run(tt.format, func(t *testing.T) {
			formatter := NewFormatter(tt.format, false)
			result := formatter.FormatForecast(data)

			if result == "" {
				t.Error("Expected non-empty output")
			}

			// Verify dates are present
			if !strings.Contains(result, "2025-12-24") {
				t.Error("Expected first forecast date in output")
			}
			if !strings.Contains(result, "2025-12-25") {
				t.Error("Expected second forecast date in output")
			}
		})
	}
}

func TestFormatAlerts(t *testing.T) {
	data := map[string]interface{}{
		"alerts": []interface{}{
			map[string]interface{}{
				"event":       "Tornado Warning",
				"severity":    "Extreme",
				"description": "A tornado has been spotted in your area. Take shelter immediately.",
			},
		},
	}

	formatter := NewFormatter("plain", false)
	result := formatter.FormatAlerts(data)

	if !strings.Contains(result, "Tornado Warning") {
		t.Error("Expected alert event in output")
	}
	if !strings.Contains(result, "Extreme") {
		t.Error("Expected severity in output")
	}
}

func TestFormatAlertsEmpty(t *testing.T) {
	data := map[string]interface{}{
		"alerts": []interface{}{},
	}

	formatter := NewFormatter("plain", false)
	result := formatter.FormatAlerts(data)

	if !strings.Contains(result, "No active weather alerts") {
		t.Error("Expected no alerts message")
	}
}

func TestFormatMoon(t *testing.T) {
	data := map[string]interface{}{
		"phase":        "Full Moon",
		"illumination": 0.99,
		"age":          14.5,
	}

	tests := []struct {
		format string
	}{
		{"json"},
		{"table"},
		{"plain"},
	}

	for _, tt := range tests {
		t.Run(tt.format, func(t *testing.T) {
			formatter := NewFormatter(tt.format, false)
			result := formatter.FormatMoon(data)

			if result == "" {
				t.Error("Expected non-empty output")
			}

			// Verify moon phase is present
			if !strings.Contains(result, "Full Moon") {
				t.Error("Expected moon phase in output")
			}
		})
	}
}

func TestGetStringValue(t *testing.T) {
	data := map[string]interface{}{
		"key1": "value1",
		"key2": 123,
	}

	// Test valid string
	result := getStringValue(data, "key1", "default")
	if result != "value1" {
		t.Errorf("Expected value1, got %s", result)
	}

	// Test non-string value (should return default)
	result = getStringValue(data, "key2", "default")
	if result != "default" {
		t.Errorf("Expected default, got %s", result)
	}

	// Test missing key (should return default)
	result = getStringValue(data, "key3", "default")
	if result != "default" {
		t.Errorf("Expected default, got %s", result)
	}
}

func TestGetFloatValue(t *testing.T) {
	data := map[string]interface{}{
		"key1": 123.45,
		"key2": "not a float",
	}

	// Test valid float
	result := getFloatValue(data, "key1", 0.0)
	if result != 123.45 {
		t.Errorf("Expected 123.45, got %f", result)
	}

	// Test non-float value (should return default)
	result = getFloatValue(data, "key2", 99.9)
	if result != 99.9 {
		t.Errorf("Expected 99.9, got %f", result)
	}

	// Test missing key (should return default)
	result = getFloatValue(data, "key3", 88.8)
	if result != 88.8 {
		t.Errorf("Expected 88.8, got %f", result)
	}
}

func TestWrapText(t *testing.T) {
	tests := []struct {
		text  string
		width int
		// expected number of lines
		expected int
	}{
		{"short", 10, 1},
		{"this is a longer text that should wrap", 10, 5},
		{"word", 20, 1},
		{"", 10, 0},
	}

	for _, tt := range tests {
		t.Run(tt.text, func(t *testing.T) {
			lines := wrapText(tt.text, tt.width)
			if len(lines) != tt.expected {
				t.Errorf("Expected %d lines, got %d", tt.expected, len(lines))
			}

			// Verify no line exceeds width
			for _, line := range lines {
				if len(line) > tt.width {
					t.Errorf("Line exceeds width %d: %s (length %d)", tt.width, line, len(line))
				}
			}
		})
	}
}

func TestFormatTableForecast(t *testing.T) {
	formatter := NewFormatter("table", false)

	data := map[string]interface{}{
		"location": "Test City",
		"forecast": []interface{}{
			map[string]interface{}{
				"date":      "2025-12-24",
				"high":      80.0,
				"low":       65.0,
				"condition": "Sunny",
			},
		},
	}

	result := formatter.formatTableForecast(data)

	// Verify table headers
	if !strings.Contains(result, "Date") {
		t.Error("Expected Date column header")
	}
	if !strings.Contains(result, "High") {
		t.Error("Expected High column header")
	}
	if !strings.Contains(result, "Low") {
		t.Error("Expected Low column header")
	}
	if !strings.Contains(result, "Condition") {
		t.Error("Expected Condition column header")
	}

	// Verify data
	if !strings.Contains(result, "2025-12-24") {
		t.Error("Expected date in table")
	}
	if !strings.Contains(result, "80.0°F") {
		t.Error("Expected high temperature in table")
	}
	if !strings.Contains(result, "65.0°F") {
		t.Error("Expected low temperature in table")
	}
}
