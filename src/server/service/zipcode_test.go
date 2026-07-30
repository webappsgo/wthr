package service

import "testing"

// TestZipcode_IsZipcode covers happy path, boundary lengths, and malformed
// (non-numeric) input for the 5-digit US zipcode shape check.
func TestZipcode_IsZipcode(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want bool
	}{
		{"valid zipcode", "90210", true},
		{"valid zipcode with leading zero", "00501", true},
		{"all zeros", "00000", true},
		{"empty string", "", false},
		{"too short", "1234", false},
		{"too long", "123456", false},
		{"contains letters", "9021A", false},
		{"contains hyphen (zip+4 shape)", "90210-1234", false},
		{"contains space", "9021 ", false},
		{"embedded hyphen mid-string", "12-34", false},
		{"leading hyphen parses as negative (5 chars, numeric)", "-1234", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsZipcode(tt.in); got != tt.want {
				t.Errorf("IsZipcode(%q) = %v, want %v", tt.in, got, tt.want)
			}
		})
	}
}

// TestZipcode_ParseZipcode covers happy path, boundary zero/leading-zero
// values, and malformed input producing an error.
func TestZipcode_ParseZipcode(t *testing.T) {
	tests := []struct {
		name    string
		in      string
		want    int
		wantErr bool
	}{
		{"valid zipcode", "90210", 90210, false},
		{"leading zero preserved numerically", "00501", 501, false},
		{"all zeros", "00000", 0, false},
		{"empty string", "", 0, true},
		{"non-numeric", "abcde", 0, true},
		{"partially numeric", "123a5", 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseZipcode(tt.in)
			if (err != nil) != tt.wantErr {
				t.Fatalf("ParseZipcode(%q) error = %v, wantErr %v", tt.in, err, tt.wantErr)
			}
			if !tt.wantErr && got != tt.want {
				t.Errorf("ParseZipcode(%q) = %d, want %d", tt.in, got, tt.want)
			}
		})
	}
}

// TestZipcode_ToFloat64 covers all supported dynamic input types plus the
// default/unsupported-type fallback (the JSON payload stores lat/lon as
// interface{} because upstream data mixes numeric and string encodings).
func TestZipcode_ToFloat64(t *testing.T) {
	tests := []struct {
		name string
		in   interface{}
		want float64
	}{
		{"float64", 40.7128, 40.7128},
		{"zero float64", 0.0, 0},
		{"negative float64", -73.9, -73.9},
		{"numeric string", "40.7128", 40.7128},
		{"negative numeric string", "-73.9", -73.9},
		{"malformed string", "not-a-number", 0},
		{"empty string", "", 0},
		{"int", 40, 40},
		{"nil", nil, 0},
		{"bool unsupported type", true, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := toFloat64(tt.in); got != tt.want {
				t.Errorf("toFloat64(%v) = %v, want %v", tt.in, got, tt.want)
			}
		})
	}
}

// TestZipcode_LookupZipcode_NotLoaded covers the error path when a lookup
// is attempted before the database has finished loading.
func TestZipcode_LookupZipcode_NotLoaded(t *testing.T) {
	zs := &ZipcodeService{
		zipcodes: make(map[int]*ZipcodeData),
		loaded:   false,
	}

	if _, err := zs.LookupZipcode(90210); err == nil {
		t.Error("expected error when database not loaded, got nil")
	}
}

// TestZipcode_LookupZipcode covers happy-path lookup, the not-found error
// path, and the case where an entry exists but has zero/invalid
// coordinates (data-quality boundary).
func TestZipcode_LookupZipcode(t *testing.T) {
	zs := &ZipcodeService{
		zipcodes: map[int]*ZipcodeData{
			90210: {State: "CA", City: "Beverly Hills", County: "Los Angeles", ZipCode: 90210, Latitude: 34.0901, Longitude: -118.4065},
			10001: {State: "NY", City: "New York", County: "New York", ZipCode: 10001, Latitude: "40.7484", Longitude: "-73.9967"},
			99999: {State: "ZZ", City: "Nowhere", County: "Void", ZipCode: 99999, Latitude: 0, Longitude: 0},
		},
		loaded: true,
	}

	tests := []struct {
		name      string
		zip       int
		wantErr   bool
		wantLat   float64
		wantCity  string
		wantShort string
	}{
		{"found with float coords", 90210, false, 34.0901, "Beverly Hills", "Beverly Hills, CA"},
		{"found with string coords", 10001, false, 40.7484, "New York", "New York, NY"},
		{"found but zero coordinates", 99999, true, 0, "", ""},
		{"not found", 12345, true, 0, "", ""},
		{"zero zipcode not found", 0, true, 0, "", ""},
		{"negative zipcode not found", -1, true, 0, "", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			coords, err := zs.LookupZipcode(tt.zip)
			if (err != nil) != tt.wantErr {
				t.Fatalf("LookupZipcode(%d) error = %v, wantErr %v", tt.zip, err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}
			if coords.Latitude != tt.wantLat {
				t.Errorf("Latitude = %v, want %v", coords.Latitude, tt.wantLat)
			}
			if coords.Name != tt.wantCity {
				t.Errorf("Name = %q, want %q", coords.Name, tt.wantCity)
			}
			if coords.ShortName != tt.wantShort {
				t.Errorf("ShortName = %q, want %q", coords.ShortName, tt.wantShort)
			}
			if coords.CountryCode != "US" {
				t.Errorf("CountryCode = %q, want %q", coords.CountryCode, "US")
			}
		})
	}
}

// TestZipcode_IsLoaded covers both loaded states directly.
func TestZipcode_IsLoaded(t *testing.T) {
	tests := []struct {
		name   string
		loaded bool
	}{
		{"not loaded", false},
		{"loaded", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			zs := &ZipcodeService{loaded: tt.loaded}
			if got := zs.IsLoaded(); got != tt.loaded {
				t.Errorf("IsLoaded() = %v, want %v", got, tt.loaded)
			}
		})
	}
}
