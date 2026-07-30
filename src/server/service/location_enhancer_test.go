package service

import (
	"encoding/json"
	"math"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

// fakeUpstreamTransport rewrites every outgoing request to target the given
// httptest server instead of the hardcoded GitHub raw-content host, so the
// LocationEnhancer's loadCountriesData/loadCitiesData/Reload paths can be
// exercised without any real network access. The original request path is
// preserved so the test server can distinguish which dataset was requested.
type fakeUpstreamTransport struct {
	target *url.URL
}

func (f *fakeUpstreamTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	req.URL.Scheme = f.target.Scheme
	req.URL.Host = f.target.Host
	req.Host = f.target.Host
	return http.DefaultTransport.RoundTrip(req)
}

// newTestLocationEnhancer builds a LocationEnhancer directly (bypassing
// NewLocationEnhancer, which spawns a background goroutine that makes real
// network calls) so tests can control the client and seed data explicitly.
func newTestLocationEnhancer() *LocationEnhancer {
	return &LocationEnhancer{
		client:        &http.Client{},
		countriesData: []Country{},
		citiesData:    []City{},
	}
}

// TestLocationEnhancer_EnhanceLocationData covers country lookup, timezone
// fallback, nearest-city state/population enrichment (including the 100km
// distance cutoff), and the boundary case of no matching data at all.
func TestLocationEnhancer_EnhanceLocationData(t *testing.T) {
	le := newTestLocationEnhancer()
	le.countriesData = []Country{
		{Name: "United States", CountryCode: "US", Capital: "Washington D.C.", Timezones: []string{"America/New_York"}, Population: 331000000},
	}
	le.citiesData = []City{
		{ID: 1, Name: "Springfield", Country: "US", State: "Illinois", Population: 114000,
			Coord: struct {
				Lat float64 `json:"lat"`
				Lon float64 `json:"lon"`
			}{Lat: 39.7817, Lon: -89.6501}},
		// Far away same-name city; must not match because it's outside the 100km cutoff.
		{ID: 2, Name: "Springfield", Country: "US", State: "Massachusetts", Population: 999999999,
			Coord: struct {
				Lat float64 `json:"lat"`
				Lon float64 `json:"lon"`
			}{Lat: 42.1015, Lon: -72.5898}},
	}

	tests := []struct {
		name         string
		result       *GeocodeResult
		wantCountry  string
		wantCapital  string
		wantTimezone string
		wantAdmin1   string
		wantPop      int
	}{
		{
			name: "matches country and nearest city within range",
			result: &GeocodeResult{
				Latitude: 39.78, Longitude: -89.65, Name: "Springfield", CountryCode: "us",
			},
			wantCountry:  "United States",
			wantCapital:  "Washington D.C.",
			wantTimezone: "America/New_York",
			wantAdmin1:   "Illinois",
			wantPop:      114000,
		},
		{
			name: "existing timezone is preserved, not overwritten",
			result: &GeocodeResult{
				Latitude: 39.78, Longitude: -89.65, Name: "Springfield", CountryCode: "US", Timezone: "Custom/Zone",
			},
			wantCountry:  "United States",
			wantCapital:  "Washington D.C.",
			wantTimezone: "Custom/Zone",
			wantAdmin1:   "Illinois",
			wantPop:      114000,
		},
		{
			name: "unknown country code leaves country data untouched",
			result: &GeocodeResult{
				Latitude: 0, Longitude: 0, Name: "Nowhere", CountryCode: "ZZ", Country: "OriginalCountry",
			},
			wantCountry: "OriginalCountry",
		},
		{
			name: "no city match found (too far away)",
			result: &GeocodeResult{
				Latitude: -33.87, Longitude: 151.21, Name: "Springfield", CountryCode: "US",
			},
			wantCountry:  "United States",
			wantCapital:  "Washington D.C.",
			wantTimezone: "America/New_York",
			wantAdmin1:   "",
			wantPop:      0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := le.EnhanceLocationData(tt.result)
			if err != nil {
				t.Fatalf("EnhanceLocationData() unexpected error: %v", err)
			}
			if got.Country != tt.wantCountry {
				t.Errorf("Country = %q, want %q", got.Country, tt.wantCountry)
			}
			if got.Capital != tt.wantCapital {
				t.Errorf("Capital = %q, want %q", got.Capital, tt.wantCapital)
			}
			if got.Timezone != tt.wantTimezone {
				t.Errorf("Timezone = %q, want %q", got.Timezone, tt.wantTimezone)
			}
			if got.Admin1 != tt.wantAdmin1 {
				t.Errorf("Admin1 = %q, want %q", got.Admin1, tt.wantAdmin1)
			}
			if got.Population != tt.wantPop {
				t.Errorf("Population = %d, want %d", got.Population, tt.wantPop)
			}
			if got.FullName == "" {
				t.Errorf("FullName should not be empty")
			}
			if got.ShortName == "" {
				t.Errorf("ShortName should not be empty")
			}
		})
	}
}

// TestLocationEnhancer_FindNearestCity covers the happy path (nearest of
// several), the boundary of a single city, and the error path of empty
// cities data.
func TestLocationEnhancer_FindNearestCity(t *testing.T) {
	t.Run("no cities loaded errors", func(t *testing.T) {
		le := newTestLocationEnhancer()
		if _, err := le.FindNearestCity(40.0, -90.0); err == nil {
			t.Errorf("FindNearestCity() expected error with empty cities data, got nil")
		}
	})

	t.Run("finds nearest among multiple candidates", func(t *testing.T) {
		le := newTestLocationEnhancer()
		le.countriesData = []Country{
			{Name: "United States", CountryCode: "US", Capital: "Washington D.C.", Timezones: []string{"America/New_York"}},
		}
		le.citiesData = []City{
			{ID: 1, Name: "Near", Country: "US", State: "IL", Coord: struct {
				Lat float64 `json:"lat"`
				Lon float64 `json:"lon"`
			}{Lat: 40.01, Lon: -90.01}},
			{ID: 2, Name: "Far", Country: "US", State: "CA", Coord: struct {
				Lat float64 `json:"lat"`
				Lon float64 `json:"lon"`
			}{Lat: 34.0, Lon: -118.0}},
		}

		got, err := le.FindNearestCity(40.0, -90.0)
		if err != nil {
			t.Fatalf("FindNearestCity() unexpected error: %v", err)
		}
		if got.Name != "Near" {
			t.Errorf("FindNearestCity() Name = %q, want %q", got.Name, "Near")
		}
		if got.Country != "United States" {
			t.Errorf("FindNearestCity() Country = %q, want %q", got.Country, "United States")
		}
	})

	t.Run("single city is always nearest", func(t *testing.T) {
		le := newTestLocationEnhancer()
		le.citiesData = []City{
			{ID: 1, Name: "Solo", Country: "FR", Coord: struct {
				Lat float64 `json:"lat"`
				Lon float64 `json:"lon"`
			}{Lat: 48.85, Lon: 2.35}},
		}

		got, err := le.FindNearestCity(0, 0)
		if err != nil {
			t.Fatalf("FindNearestCity() unexpected error: %v", err)
		}
		if got.Name != "Solo" {
			t.Errorf("FindNearestCity() Name = %q, want %q", got.Name, "Solo")
		}
	})
}

// TestLocationEnhancer_BuildFullName covers name assembly with all parts
// present, duplicate/empty parts skipped, and the empty-location boundary.
func TestLocationEnhancer_BuildFullName(t *testing.T) {
	le := newTestLocationEnhancer()

	tests := []struct {
		name string
		loc  *EnhancedLocation
		want string
	}{
		{
			name: "all parts distinct",
			loc:  &EnhancedLocation{Name: "London", Admin2: "Greater London", Admin1: "England", Country: "United Kingdom"},
			want: "London, Greater London, England, United Kingdom",
		},
		{
			name: "admin2 equal to admin1 is skipped",
			loc:  &EnhancedLocation{Name: "City", Admin2: "State", Admin1: "State", Country: "Country"},
			want: "City, State, Country",
		},
		{
			name: "admin2 equal to name is skipped",
			loc:  &EnhancedLocation{Name: "City", Admin2: "City", Admin1: "State", Country: "Country"},
			want: "City, State, Country",
		},
		{
			name: "admin1 equal to name is skipped",
			loc:  &EnhancedLocation{Name: "City", Admin1: "City", Country: "Country"},
			want: "City, Country",
		},
		{
			name: "only name present",
			loc:  &EnhancedLocation{Name: "Solo"},
			want: "Solo",
		},
		{
			name: "fully empty location",
			loc:  &EnhancedLocation{},
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := le.buildFullName(tt.loc); got != tt.want {
				t.Errorf("buildFullName() = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestLocationEnhancer_BuildShortName covers US state abbreviation lookup
// (both full name and pre-abbreviated), Canadian province lookup, unknown
// country codes, and the empty-country-code inference fallback.
func TestLocationEnhancer_BuildShortName(t *testing.T) {
	le := newTestLocationEnhancer()

	tests := []struct {
		name string
		loc  *EnhancedLocation
		want string
	}{
		{
			name: "US with full state name",
			loc:  &EnhancedLocation{Name: "Chicago", CountryCode: "US", Admin1: "Illinois"},
			want: "Chicago, IL",
		},
		{
			name: "US with pre-abbreviated state",
			loc:  &EnhancedLocation{Name: "Chicago", CountryCode: "US", Admin1: "IL"},
			want: "Chicago, IL",
		},
		{
			name: "US with unrecognized state falls back to full name",
			loc:  &EnhancedLocation{Name: "Somewhere", CountryCode: "US", Admin1: "Not A State"},
			want: "Somewhere, Not A State",
		},
		{
			name: "US with no admin1 falls through to country code",
			loc:  &EnhancedLocation{Name: "Somewhere", CountryCode: "US"},
			want: "Somewhere, US",
		},
		{
			name: "CA with full province name",
			loc:  &EnhancedLocation{Name: "Toronto", CountryCode: "CA", Admin1: "Ontario"},
			want: "Toronto, ON",
		},
		{
			name: "CA with pre-abbreviated province",
			loc:  &EnhancedLocation{Name: "Toronto", CountryCode: "CA", Admin1: "ON"},
			want: "Toronto, ON",
		},
		{
			name: "CA with unrecognized province falls back to full name",
			loc:  &EnhancedLocation{Name: "Somewhere", CountryCode: "CA", Admin1: "Not A Province"},
			want: "Somewhere, Not A Province",
		},
		{
			name: "empty country code infers US state",
			loc:  &EnhancedLocation{Name: "Austin", Admin1: "Texas"},
			want: "Austin, TX, US",
		},
		{
			name: "empty country code infers CA province",
			loc:  &EnhancedLocation{Name: "Calgary", Admin1: "Alberta"},
			want: "Calgary, AB, CA",
		},
		{
			name: "empty country code and unrecognized admin1 falls back to city only",
			loc:  &EnhancedLocation{Name: "Mystery", Admin1: "Nowhere"},
			want: "Mystery",
		},
		{
			name: "empty country code and no admin1 falls back to city only",
			loc:  &EnhancedLocation{Name: "Mystery"},
			want: "Mystery",
		},
		{
			name: "other country code uses country code directly",
			loc:  &EnhancedLocation{Name: "Paris", CountryCode: "FR"},
			want: "Paris, FR",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := le.buildShortName(tt.loc); got != tt.want {
				t.Errorf("buildShortName() = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestLocationEnhancer_GetStateAbbreviation covers case-insensitive matching,
// whitespace trimming, and the not-found boundary.
func TestLocationEnhancer_GetStateAbbreviation(t *testing.T) {
	le := newTestLocationEnhancer()

	tests := []struct {
		input string
		want  string
	}{
		{"California", "CA"},
		{"california", "CA"},
		{"  California  ", "CA"},
		{"New York", "NY"},
		{"Not A State", ""},
		{"", ""},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			if got := le.getStateAbbreviation(tt.input); got != tt.want {
				t.Errorf("getStateAbbreviation(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

// TestLocationEnhancer_GetProvinceAbbreviation covers case-insensitive
// matching and the not-found boundary. Note: unlike getStateAbbreviation,
// this method does not trim whitespace before lookup.
func TestLocationEnhancer_GetProvinceAbbreviation(t *testing.T) {
	le := newTestLocationEnhancer()

	tests := []struct {
		input string
		want  string
	}{
		{"Ontario", "ON"},
		{"ontario", "ON"},
		{"British Columbia", "BC"},
		{"Not A Province", ""},
		{"", ""},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			if got := le.getProvinceAbbreviation(tt.input); got != tt.want {
				t.Errorf("getProvinceAbbreviation(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

// TestHaversineDistance covers zero distance (same point), a known real
// distance (NYC to LA, roughly 3936km), symmetry, and antipodal points.
func TestHaversineDistance(t *testing.T) {
	tests := []struct {
		name                   string
		lat1, lon1, lat2, lon2 float64
		want                   float64
		tolerance              float64
	}{
		{name: "same point is zero distance", lat1: 40.7128, lon1: -74.0060, lat2: 40.7128, lon2: -74.0060, want: 0, tolerance: 0.001},
		{name: "NYC to LA approx 3936km", lat1: 40.7128, lon1: -74.0060, lat2: 34.0522, lon2: -118.2437, want: 3936, tolerance: 20},
		{name: "antipodal points are half the earth's circumference", lat1: 0, lon1: 0, lat2: 0, lon2: 180, want: math.Pi * 6371.0, tolerance: 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := haversineDistance(tt.lat1, tt.lon1, tt.lat2, tt.lon2)
			if math.Abs(got-tt.want) > tt.tolerance {
				t.Errorf("haversineDistance() = %f, want ~%f (tolerance %f)", got, tt.want, tt.tolerance)
			}
		})
	}

	t.Run("symmetric", func(t *testing.T) {
		a := haversineDistance(10, 20, 30, 40)
		b := haversineDistance(30, 40, 10, 20)
		if math.Abs(a-b) > 0.0001 {
			t.Errorf("haversineDistance() not symmetric: %f vs %f", a, b)
		}
	})
}

// TestLocationEnhancer_EnhanceLocation covers the Coordinates wrapper,
// including the nil boundary case.
func TestLocationEnhancer_EnhanceLocation(t *testing.T) {
	le := newTestLocationEnhancer()
	le.countriesData = []Country{
		{Name: "United States", CountryCode: "US", Capital: "Washington D.C.", Timezones: []string{"America/New_York"}},
	}

	t.Run("nil coordinates returns nil", func(t *testing.T) {
		if got := le.EnhanceLocation(nil); got != nil {
			t.Errorf("EnhanceLocation(nil) = %+v, want nil", got)
		}
	})

	t.Run("valid coordinates are enhanced", func(t *testing.T) {
		coords := &Coordinates{
			Latitude: 39.78, Longitude: -89.65, Name: "Springfield", CountryCode: "US",
		}
		got := le.EnhanceLocation(coords)
		if got == nil {
			t.Fatalf("EnhanceLocation() returned nil, want a value")
		}
		if got.Country != "United States" {
			t.Errorf("EnhanceLocation() Country = %q, want %q", got.Country, "United States")
		}
		if got.FullName == "" {
			t.Errorf("EnhanceLocation() FullName should not be empty")
		}
	})
}

// TestLocationEnhancer_IsInitialized_SetOnInitComplete verifies initial
// state, concurrency-safe mutation via the mutex-protected fields, and that
// the stored callback can be invoked.
func TestLocationEnhancer_IsInitialized_SetOnInitComplete(t *testing.T) {
	le := newTestLocationEnhancer()

	if le.IsInitialized() {
		t.Errorf("IsInitialized() = true before any load, want false")
	}

	le.mu.Lock()
	le.initialized = true
	le.mu.Unlock()

	if !le.IsInitialized() {
		t.Errorf("IsInitialized() = false after setting true, want true")
	}

	var gotCountries, gotCities bool
	called := false
	le.SetOnInitComplete(func(countries, cities bool) {
		called = true
		gotCountries = countries
		gotCities = cities
	})

	le.mu.RLock()
	cb := le.onInitComplete
	le.mu.RUnlock()
	if cb == nil {
		t.Fatalf("SetOnInitComplete() did not store the callback")
	}
	cb(true, false)

	if !called {
		t.Errorf("callback was not invoked")
	}
	if !gotCountries || gotCities {
		t.Errorf("callback args = (%v, %v), want (true, false)", gotCountries, gotCities)
	}
}

// TestLocationEnhancer_GetCitiesData covers the empty and populated cases.
func TestLocationEnhancer_GetCitiesData(t *testing.T) {
	le := newTestLocationEnhancer()

	if got := le.GetCitiesData(); len(got) != 0 {
		t.Errorf("GetCitiesData() on empty enhancer = %d items, want 0", len(got))
	}

	le.citiesData = []City{{ID: 1, Name: "Test"}}
	got := le.GetCitiesData()
	if len(got) != 1 || got[0].Name != "Test" {
		t.Errorf("GetCitiesData() = %+v, want one city named Test", got)
	}
}

// TestLocationEnhancer_FindCityByID covers the not-initialized error path,
// found/not-found cases, and the state-vs-no-state name formatting branches.
func TestLocationEnhancer_FindCityByID(t *testing.T) {
	t.Run("not initialized errors", func(t *testing.T) {
		le := newTestLocationEnhancer()
		if _, err := le.FindCityByID(1); err == nil {
			t.Errorf("FindCityByID() expected error when not initialized, got nil")
		}
	})

	le := newTestLocationEnhancer()
	le.initialized = true
	le.countriesData = []Country{
		{Name: "United States", CountryCode: "US", Capital: "Washington D.C.", Timezones: []string{"America/New_York"}},
	}
	le.citiesData = []City{
		{ID: 1, Name: "Chicago", Country: "US", State: "Illinois", Population: 2700000},
		{ID: 2, Name: "Vatican City", Country: "VA"},
	}

	t.Run("found city with state formats full/short names with state", func(t *testing.T) {
		got, err := le.FindCityByID(1)
		if err != nil {
			t.Fatalf("FindCityByID() unexpected error: %v", err)
		}
		if got.FullName != "Chicago, Illinois, US" {
			t.Errorf("FullName = %q, want %q", got.FullName, "Chicago, Illinois, US")
		}
		if got.ShortName != "Chicago, Illinois" {
			t.Errorf("ShortName = %q, want %q", got.ShortName, "Chicago, Illinois")
		}
		if got.Capital != "Washington D.C." {
			t.Errorf("Capital = %q, want %q", got.Capital, "Washington D.C.")
		}
		if got.Timezone != "America/New_York" {
			t.Errorf("Timezone = %q, want %q", got.Timezone, "America/New_York")
		}
	})

	t.Run("found city without state formats names without state", func(t *testing.T) {
		got, err := le.FindCityByID(2)
		if err != nil {
			t.Fatalf("FindCityByID() unexpected error: %v", err)
		}
		if got.FullName != "Vatican City, VA" {
			t.Errorf("FullName = %q, want %q", got.FullName, "Vatican City, VA")
		}
		if got.ShortName != "Vatican City" {
			t.Errorf("ShortName = %q, want %q", got.ShortName, "Vatican City")
		}
		// No matching country data was seeded for "VA", so capital/timezone stay empty.
		if got.Capital != "" {
			t.Errorf("Capital = %q, want empty (no country match)", got.Capital)
		}
	})

	t.Run("city id not found errors", func(t *testing.T) {
		if _, err := le.FindCityByID(9999); err == nil {
			t.Errorf("FindCityByID() expected error for unknown id, got nil")
		}
	})
}

// TestLocationEnhancer_Reload exercises loadCountriesData/loadCitiesData
// through Reload() against a local httptest.Server (via a RoundTripper that
// redirects the hardcoded upstream host), covering both the success path and
// the upstream-failure error path. No real network access is made.
func TestLocationEnhancer_Reload(t *testing.T) {
	t.Run("successful reload populates countries and cities", func(t *testing.T) {
		countries := []Country{{Name: "Testland", CountryCode: "TL", Capital: "Test City", Timezones: []string{"UTC"}}}
		cities := []City{{ID: 1, Name: "Test City", Country: "TL"}}

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			if strings.Contains(r.URL.Path, "countries") {
				json.NewEncoder(w).Encode(countries)
				return
			}
			json.NewEncoder(w).Encode(cities)
		}))
		defer server.Close()

		target, err := url.Parse(server.URL)
		if err != nil {
			t.Fatalf("failed to parse test server URL: %v", err)
		}

		le := newTestLocationEnhancer()
		le.client = &http.Client{Transport: &fakeUpstreamTransport{target: target}}

		if err := le.Reload(); err != nil {
			t.Fatalf("Reload() unexpected error: %v", err)
		}

		if len(le.countriesData) != 1 || le.countriesData[0].CountryCode != "TL" {
			t.Errorf("countriesData after Reload = %+v, want one Testland entry", le.countriesData)
		}
		if len(le.citiesData) != 1 || le.citiesData[0].Name != "Test City" {
			t.Errorf("citiesData after Reload = %+v, want one Test City entry", le.citiesData)
		}
	})

	t.Run("upstream failure on countries returns wrapped error", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		}))
		defer server.Close()

		target, err := url.Parse(server.URL)
		if err != nil {
			t.Fatalf("failed to parse test server URL: %v", err)
		}

		le := newTestLocationEnhancer()
		le.client = &http.Client{Transport: &fakeUpstreamTransport{target: target}}

		err = le.Reload()
		if err == nil {
			t.Fatalf("Reload() expected error on upstream 500, got nil")
		}
		if !strings.Contains(err.Error(), "reload countries") {
			t.Errorf("Reload() error = %v, want it to mention 'reload countries'", err)
		}
	})

	t.Run("malformed JSON response returns wrapped error", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Write([]byte("not json"))
		}))
		defer server.Close()

		target, err := url.Parse(server.URL)
		if err != nil {
			t.Fatalf("failed to parse test server URL: %v", err)
		}

		le := newTestLocationEnhancer()
		le.client = &http.Client{Transport: &fakeUpstreamTransport{target: target}}

		if err := le.Reload(); err == nil {
			t.Errorf("Reload() expected error on malformed JSON, got nil")
		}
	})
}
