package service

import (
	"strings"
	"testing"
)

func TestZipcode_IsZipcode(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want bool
	}{
		{"valid ZIP", "90210", true},
		{"leading zero", "00501", true},
		{"all zeros have valid shape", "00000", true},
		{"empty", "", false},
		{"too short", "1234", false},
		{"too long", "123456", false},
		{"letters", "9021A", false},
		{"ZIP plus four", "90210-1234", false},
		{"space", "9021 ", false},
		{"negative number", "-1234", false},
		{"non-ASCII digits", "１２３４５", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsZipcode(tt.in); got != tt.want {
				t.Errorf("IsZipcode(%q) = %v, want %v", tt.in, got, tt.want)
			}
		})
	}
}

func TestZipcode_ParseZipcode(t *testing.T) {
	for _, tt := range []struct {
		input   string
		want    int
		wantErr bool
	}{
		{"90210", 90210, false},
		{"00501", 501, false},
		{"00000", 0, false},
		{"1234", 0, true},
		{"12a45", 0, true},
		{"-1234", 0, true},
	} {
		got, err := ParseZipcode(tt.input)
		if (err != nil) != tt.wantErr {
			t.Fatalf("ParseZipcode(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
		}
		if !tt.wantErr && got != tt.want {
			t.Errorf("ParseZipcode(%q) = %d, want %d", tt.input, got, tt.want)
		}
	}
}

func TestParseGeoNamesUSZipcodes(t *testing.T) {
	input := strings.Join([]string{
		"US\t00501\tHoltsville\tNew York\tNY\tSuffolk\t103\t\t\t40.8154\t-73.0451\t1",
		"US\t00501\tFPO AA\t\t\t\t\t\t\t40.9\t-73.1\t4",
		"US\t90210\tBeverly Hills\tCalifornia\tCA\tLos Angeles\t037\t\t\t34.0901\t-118.4065\t1",
		"US\t02108\tBoston\tMassachusetts\tMA\tSuffolk\t025\t\t\t42.3576\t-71.0677\t",
		"US\t99999\tNowhere\tUnknown\tZZ\tVoid\t000\t\t\t91\t0\t1",
		"CA\tH0H0H0\tNorth Pole\tNunavut\tNU\t\t\t\t\t\t\t",
		"not a valid record",
	}, "\n")
	got, err := parseGeoNamesUSZipcodes(strings.NewReader(input))
	if err != nil {
		t.Fatalf("parseGeoNamesUSZipcodes() error = %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("parsed ZIP count = %d, want 3", len(got))
	}
	if got[501].ZipCode != "00501" {
		t.Errorf("leading-zero ZIP = %q, want 00501", got[501].ZipCode)
	}
	if got[501].City != "FPO AA" || got[501].Accuracy != 4 {
		t.Errorf("duplicate selection = %+v, want GeoNames place-coordinate FPO AA row", got[501])
	}
	if got[90210].Latitude != 34.0901 || got[90210].Longitude != -118.4065 || got[90210].State != "CA" || got[90210].County != "Los Angeles" {
		t.Errorf("90210 data = %+v, want the supplied state, county, and coordinates", got[90210])
	}
	if got[2108].City != "Boston" || got[2108].Accuracy != 0 {
		t.Errorf("02108 data = %+v, want Boston with unknown accuracy retained as 0", got[2108])
	}
}

func TestParseGeoNamesUSZipcodesRejectsEmptyData(t *testing.T) {
	if _, err := parseGeoNamesUSZipcodes(strings.NewReader("US\tbad\trow")); err == nil {
		t.Fatal("expected an error when no valid ZIP-code row exists")
	}
}

func TestZipcode_LookupZipcode(t *testing.T) {
	zs := &ZipcodeService{
		zipcodes: map[int]*ZipcodeData{
			501:   {State: "NY", City: "Holtsville", County: "Suffolk", ZipCode: "00501", Latitude: 40.8154, Longitude: -73.0451},
			90210: {State: "CA", City: "Beverly Hills", County: "Los Angeles", ZipCode: "90210", Latitude: 34.0901, Longitude: -118.4065},
		},
		loaded: true,
	}
	for _, tt := range []struct {
		zip       int
		wantCity  string
		wantShort string
		wantErr   bool
	}{
		{501, "Holtsville", "Holtsville, NY", false},
		{90210, "Beverly Hills", "Beverly Hills, CA", false},
		{12345, "", "", true},
		{-1, "", "", true},
		{100000, "", "", true},
	} {
		coords, err := zs.LookupZipcode(tt.zip)
		if (err != nil) != tt.wantErr {
			t.Fatalf("LookupZipcode(%d) error = %v, wantErr %v", tt.zip, err, tt.wantErr)
		}
		if tt.wantErr {
			continue
		}
		if coords.Name != tt.wantCity || coords.ShortName != tt.wantShort {
			t.Errorf("LookupZipcode(%d) = %+v, want city %q and short name %q", tt.zip, coords, tt.wantCity, tt.wantShort)
		}
		if coords.CountryCode != "US" {
			t.Errorf("CountryCode = %q, want US", coords.CountryCode)
		}
	}
}

func TestZipcode_LookupZipcodeNotLoaded(t *testing.T) {
	zs := &ZipcodeService{zipcodes: make(map[int]*ZipcodeData)}
	if _, err := zs.LookupZipcode(90210); err == nil {
		t.Fatal("expected error before data loads")
	}
}

func TestZipcode_IsLoaded(t *testing.T) {
	zs := &ZipcodeService{loaded: true}
	if !zs.IsLoaded() {
		t.Fatal("IsLoaded() = false, want true")
	}
}
