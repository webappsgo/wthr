package handler

import (
	"testing"
	"time"
)

// TestParseHistoricalDate_Empty verifies that an empty date string resolves
// to "today" (month/day/year of time.Now()).
func TestParseHistoricalDate_Empty(t *testing.T) {
	now := time.Now()

	month, day, year, err := parseHistoricalDate("")
	if err != nil {
		t.Fatalf("expected no error for empty date, got %v", err)
	}
	if month != int(now.Month()) {
		t.Errorf("expected month %d, got %d", int(now.Month()), month)
	}
	if day != now.Day() {
		t.Errorf("expected day %d, got %d", now.Day(), day)
	}
	if year != now.Year() {
		t.Errorf("expected year %d, got %d", now.Year(), year)
	}
}

// TestParseHistoricalDate_WithYear covers every supported dated format.
func TestParseHistoricalDate_WithYear(t *testing.T) {
	cases := []struct {
		name  string
		input string
		month int
		day   int
		year  int
	}{
		{"ISO 8601", "2024-03-15", 3, 15, 2024},
		{"US slash", "03/15/2024", 3, 15, 2024},
		{"abbreviated month, comma year", "Mar 15, 2024", 3, 15, 2024},
		{"full month, comma year", "March 15, 2024", 3, 15, 2024},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			month, day, year, err := parseHistoricalDate(tc.input)
			if err != nil {
				t.Fatalf("parseHistoricalDate(%q) returned error: %v", tc.input, err)
			}
			if month != tc.month || day != tc.day || year != tc.year {
				t.Errorf("parseHistoricalDate(%q) = (%d, %d, %d), want (%d, %d, %d)",
					tc.input, month, day, year, tc.month, tc.day, tc.year)
			}
		})
	}
}

// TestParseHistoricalDate_YearlessCurrentYear covers formats that omit the
// year, which should default to the current year.
func TestParseHistoricalDate_YearlessCurrentYear(t *testing.T) {
	currentYear := time.Now().Year()

	cases := []struct {
		name  string
		input string
		month int
		day   int
	}{
		{"US slash no year", "03/15", 3, 15},
		{"abbreviated month no year", "Mar 15", 3, 15},
		{"full month no year", "March 15", 3, 15},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			month, day, year, err := parseHistoricalDate(tc.input)
			if err != nil {
				t.Fatalf("parseHistoricalDate(%q) returned error: %v", tc.input, err)
			}
			if month != tc.month || day != tc.day || year != currentYear {
				t.Errorf("parseHistoricalDate(%q) = (%d, %d, %d), want (%d, %d, %d)",
					tc.input, month, day, year, tc.month, tc.day, currentYear)
			}
		})
	}
}

// TestParseHistoricalDate_Invalid verifies unsupported formats return a
// descriptive error rather than a zero-value silent success.
func TestParseHistoricalDate_Invalid(t *testing.T) {
	invalidInputs := []string{
		"not-a-date",
		"2024/03/15",
		"15-03-2024",
		"tomorrow",
	}

	for _, input := range invalidInputs {
		t.Run(input, func(t *testing.T) {
			month, day, year, err := parseHistoricalDate(input)
			if err == nil {
				t.Fatalf("expected error for input %q, got month=%d day=%d year=%d",
					input, month, day, year)
			}
			if month != 0 || day != 0 || year != 0 {
				t.Errorf("expected zero-value result on error, got month=%d day=%d year=%d",
					month, day, year)
			}
		})
	}
}
