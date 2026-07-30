package utils

import "testing"

// TestParseCoordinates covers valid coordinates, boundary values, malformed
// input (wrong part count, non-numeric), and out-of-range lat/lon.
func TestParseCoordinates(t *testing.T) {
	tests := []struct {
		name    string
		coords  string
		wantLat float64
		wantLon float64
		wantErr bool
	}{
		{"valid_simple", "40.7128,-74.0060", 40.7128, -74.0060, false},
		{"valid_with_spaces", " 40.7128 , -74.0060 ", 40.7128, -74.0060, false},
		{"zero", "0,0", 0, 0, false},
		{"lat_max_boundary", "90,0", 90, 0, false},
		{"lat_min_boundary", "-90,0", -90, 0, false},
		{"lon_max_boundary", "0,180", 0, 180, false},
		{"lon_min_boundary", "0,-180", 0, -180, false},
		{"empty", "", 0, 0, true},
		{"missing_comma", "40.7128", 0, 0, true},
		{"too_many_parts", "1,2,3", 0, 0, true},
		{"non_numeric_lat", "abc,-74.0060", 0, 0, true},
		{"non_numeric_lon", "40.7128,abc", 0, 0, true},
		{"lat_too_high", "90.1,0", 0, 0, true},
		{"lat_too_low", "-90.1,0", 0, 0, true},
		{"lon_too_high", "0,180.1", 0, 0, true},
		{"lon_too_low", "0,-180.1", 0, 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lat, lon, err := ParseCoordinates(tt.coords)
			if tt.wantErr {
				if err == nil {
					t.Errorf("ParseCoordinates(%q) err = nil, want error", tt.coords)
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseCoordinates(%q) unexpected error: %v", tt.coords, err)
			}
			if lat != tt.wantLat || lon != tt.wantLon {
				t.Errorf("ParseCoordinates(%q) = (%v, %v), want (%v, %v)", tt.coords, lat, lon, tt.wantLat, tt.wantLon)
			}
		})
	}
}
