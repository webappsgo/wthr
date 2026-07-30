package service

import "testing"

// TestLocationFallbacks_ExactMatch covers direct (case/whitespace-normalized)
// lookups against the built-in fallback table.
func TestLocationFallbacks_ExactMatch(t *testing.T) {
	tests := []struct {
		name         string
		input        string
		wantName     string
		wantLat      float64
		wantLon      float64
		wantCC       string
	}{
		{"lowercase exact", "new york", "New York", 40.7128, -74.0060, "US"},
		{"uppercase", "LONDON", "London", 51.5074, -0.1278, "GB"},
		{"mixed case", "ToKyO", "Tokyo", 35.6762, 139.6503, "JP"},
		{"leading/trailing whitespace", "  paris  ", "Paris", 48.8566, 2.3522, "FR"},
		{"multi-word city", "san francisco", "San Francisco", 37.7749, -122.4194, "US"},
		{"multi-word city with spacing", "  los angeles ", "Los Angeles", 34.0522, -118.2437, "US"},
		{"negative latitude city", "sydney", "Sydney", -33.8688, 151.2093, "AU"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := CreateFallbackLocation(tt.input)
			if got == nil {
				t.Fatalf("CreateFallbackLocation(%q) = nil, want match", tt.input)
			}
			if got.Name != tt.wantName {
				t.Errorf("Name = %q, want %q", got.Name, tt.wantName)
			}
			if got.Latitude != tt.wantLat {
				t.Errorf("Latitude = %v, want %v", got.Latitude, tt.wantLat)
			}
			if got.Longitude != tt.wantLon {
				t.Errorf("Longitude = %v, want %v", got.Longitude, tt.wantLon)
			}
			if got.CountryCode != tt.wantCC {
				t.Errorf("CountryCode = %q, want %q", got.CountryCode, tt.wantCC)
			}
		})
	}
}

// TestLocationFallbacks_PartialMatch covers the substring fallback path used
// when no exact key matches but the input contains (or is contained by) a
// known city name.
func TestLocationFallbacks_PartialMatch(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		wantName string
	}{
		{"input contains key", "new york city", "New York"},
		{"input contains key with country", "london, uk", "London"},
		{"input is substring of key (short input)", "moscow", "Moscow"},
		{"key contained inside longer descriptive text", "greater tokyo area", "Tokyo"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := CreateFallbackLocation(tt.input)
			if got == nil {
				t.Fatalf("CreateFallbackLocation(%q) = nil, want match", tt.input)
			}
			if got.Name != tt.wantName {
				t.Errorf("Name = %q, want %q", got.Name, tt.wantName)
			}
		})
	}
}

// TestLocationFallbacks_NoMatch covers inputs that must resolve to nil:
// empty, whitespace-only, and genuinely unknown locations.
func TestLocationFallbacks_NoMatch(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{"empty string", ""},
		{"whitespace only", "   "},
		{"unknown city", "atlantis"},
		{"random text", "xyzzy plugh qux"},
		{"unknown short string not substring of any key", "zz"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := CreateFallbackLocation(tt.input)
			if got != nil {
				t.Errorf("CreateFallbackLocation(%q) = %+v, want nil", tt.input, got)
			}
		})
	}
}

// TestLocationFallbacks_FullDataIntegrity spot-checks that every returned
// Coordinates struct is fully populated (no zero-value fields that would
// indicate a copy/paste error in the fallback table).
func TestLocationFallbacks_FullDataIntegrity(t *testing.T) {
	cities := []string{
		"new york", "london", "paris", "tokyo", "sydney", "los angeles",
		"chicago", "toronto", "berlin", "moscow", "beijing", "shanghai",
		"mumbai", "dubai", "singapore", "san francisco", "boston",
		"seattle", "miami", "dallas",
	}

	for _, city := range cities {
		t.Run(city, func(t *testing.T) {
			got := CreateFallbackLocation(city)
			if got == nil {
				t.Fatalf("CreateFallbackLocation(%q) = nil, want match", city)
			}
			if got.Name == "" {
				t.Error("Name is empty")
			}
			if got.Country == "" {
				t.Error("Country is empty")
			}
			if got.CountryCode == "" {
				t.Error("CountryCode is empty")
			}
			if got.Timezone == "" {
				t.Error("Timezone is empty")
			}
			if got.FullName == "" {
				t.Error("FullName is empty")
			}
			if got.ShortName == "" {
				t.Error("ShortName is empty")
			}
			if got.Population <= 0 {
				t.Error("Population must be positive")
			}
			if got.Latitude < -90 || got.Latitude > 90 {
				t.Errorf("Latitude out of range: %v", got.Latitude)
			}
			if got.Longitude < -180 || got.Longitude > 180 {
				t.Errorf("Longitude out of range: %v", got.Longitude)
			}
		})
	}
}

// TestLocationFallbacks_Idempotent verifies repeated calls with the same
// input return equal results (the map is rebuilt on every call, so this
// also guards against nondeterministic map iteration order affecting the
// partial-match branch).
func TestLocationFallbacks_Idempotent(t *testing.T) {
	inputs := []string{"new york", "greater tokyo area", "unknown-city"}

	for _, input := range inputs {
		t.Run(input, func(t *testing.T) {
			first := CreateFallbackLocation(input)
			for i := 0; i < 20; i++ {
				got := CreateFallbackLocation(input)
				if (first == nil) != (got == nil) {
					t.Fatalf("iteration %d: nilness changed: first=%v got=%v", i, first, got)
				}
				if first != nil && got.Name != first.Name {
					t.Fatalf("iteration %d: Name changed from %q to %q", i, first.Name, got.Name)
				}
			}
		})
	}
}
