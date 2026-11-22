package service

import (
	"encoding/csv"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

// TestAirport_IsAirportCode covers happy path, boundary lengths, and
// malformed input for the airport-code shape check.
func TestAirport_IsAirportCode(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want bool
	}{
		{"valid 3-char ICAO-ish", "JFK", true},
		{"valid 4-char ICAO", "KJFK", true},
		{"valid lowercase", "kjfk", true},
		{"valid mixed case digits", "K1J2", true},
		{"empty string", "", false},
		{"too short 1 char", "A", false},
		{"too short 2 char", "AB", false},
		{"too long 5 char", "ABCDE", false},
		{"contains space", "AB C", false},
		{"contains hyphen", "AB-C", false},
		{"contains unicode", "AB✈", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsAirportCode(tt.in); got != tt.want {
				t.Errorf("IsAirportCode(%q) = %v, want %v", tt.in, got, tt.want)
			}
		})
	}
}

// TestAirport_LookupAirport_NotLoaded covers the error path when a lookup
// is attempted before the database has finished loading.
func TestAirport_LookupAirport_NotLoaded(t *testing.T) {
	as := &AirportService{
		airports:  make(map[string]*AirportData),
		iataIndex: make(map[string]*AirportData),
		loaded:    false,
	}

	if _, err := as.LookupAirport("JFK"); err == nil {
		t.Error("expected error when database not loaded, got nil")
	}
}

// TestAirport_LookupAirport covers happy-path lookups by ICAO and IATA
// code, case-insensitivity, and the not-found error path.
func TestAirport_LookupAirport(t *testing.T) {
	jfk := &AirportData{
		ICAO: "KJFK", IATA: "JFK", Name: "John F Kennedy Intl",
		City: "New York", State: "NY", Country: "United States", CountryCode: "US",
		Lat: 40.6413, Lon: -73.7781,
	}
	lhr := &AirportData{
		ICAO: "EGLL", IATA: "LHR", Name: "Heathrow",
		City: "London", State: "", Country: "United Kingdom", CountryCode: "GB",
		Lat: 51.4700, Lon: -0.4543,
	}

	as := &AirportService{
		airports: map[string]*AirportData{
			"KJFK": jfk,
			"EGLL": lhr,
		},
		iataIndex: map[string]*AirportData{
			"JFK": jfk,
			"LHR": lhr,
		},
		loaded: true,
	}

	tests := []struct {
		name        string
		code        string
		wantErr     bool
		wantLat     float64
		wantName    string
		wantCountry string
	}{
		{"ICAO exact case", "KJFK", false, 40.6413, "New York (John F Kennedy Intl)", "United States"},
		{"ICAO lowercase", "kjfk", false, 40.6413, "New York (John F Kennedy Intl)", "United States"},
		{"IATA code", "JFK", false, 40.6413, "New York (John F Kennedy Intl)", "United States"},
		{"IATA lowercase", "jfk", false, 40.6413, "New York (John F Kennedy Intl)", "United States"},
		{"second airport by IATA", "LHR", false, 51.4700, "London (Heathrow)", "United Kingdom"},
		{"unknown code", "ZZZ", true, 0, "", ""},
		{"empty code", "", true, 0, "", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			coords, err := as.LookupAirport(tt.code)
			if (err != nil) != tt.wantErr {
				t.Fatalf("LookupAirport(%q) error = %v, wantErr %v", tt.code, err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}
			if coords.Latitude != tt.wantLat {
				t.Errorf("Latitude = %v, want %v", coords.Latitude, tt.wantLat)
			}
			if coords.Name != tt.wantName {
				t.Errorf("Name = %q, want %q", coords.Name, tt.wantName)
			}
			if coords.Country != tt.wantCountry {
				t.Errorf("Country = %q, want %q", coords.Country, tt.wantCountry)
			}
		})
	}
}

// TestAirport_LookupAirport_NoCity covers the name-building branch when the
// airport record has no city (falls back to the raw name).
func TestAirport_LookupAirport_NoCity(t *testing.T) {
	base := &AirportData{
		ICAO: "TEST", Name: "Test Field", City: "", Country: "Nowhere",
		Lat: 1, Lon: 1,
	}
	as := &AirportService{
		airports:  map[string]*AirportData{"TEST": base},
		iataIndex: make(map[string]*AirportData),
		loaded:    true,
	}

	coords, err := as.LookupAirport("TEST")
	if err != nil {
		t.Fatalf("LookupAirport() error = %v", err)
	}
	if coords.Name != "Test Field" {
		t.Errorf("Name = %q, want %q", coords.Name, "Test Field")
	}
}

// TestAirport_IsLoaded covers both loaded states directly.
func TestAirport_IsLoaded(t *testing.T) {
	tests := []struct {
		name   string
		loaded bool
	}{
		{"not loaded", false},
		{"loaded", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			as := &AirportService{loaded: tt.loaded}
			if got := as.IsLoaded(); got != tt.loaded {
				t.Errorf("IsLoaded() = %v, want %v", got, tt.loaded)
			}
		})
	}
}

func writeAirportFixture(t *testing.T, path string, airports []AirportData) {
	t.Helper()
	airports = withMinimumAirportRecords(airports)
	body, err := json.Marshal(airports)
	if err != nil {
		t.Fatalf("failed to marshal fixture: %v", err)
	}
	if err := os.WriteFile(path, body, 0644); err != nil {
		t.Fatalf("failed to write fixture: %v", err)
	}
}

func withMinimumAirportRecords(airports []AirportData) []AirportData {
	result := append([]AirportData(nil), airports...)
	validCount := 0
	for _, airport := range result {
		if validAirportCoordinates(airport.Lat, airport.Lon) && airport.Name != "" && (IsAirportCode(airport.ICAO) || IsAirportCode(airport.IATA)) {
			validCount++
		}
	}
	for validCount < minAirportRecords {
		index := len(result)
		result = append(result, AirportData{
			ICAO: "X" + leftPadAirportNumber(index%1000, 3), IATA: "Q" + leftPadAirportNumber(index%100, 2),
			Name: "Fixture Airport", Country: "United States", CountryCode: "US",
			Type: "small_airport", Lat: 10 + float64(index%70), Lon: 10 + float64(index%150),
		})
		validCount++
	}
	return result
}

func leftPadAirportNumber(value, width int) string {
	text := strconv.Itoa(value)
	for len(text) < width {
		text = "0" + text
	}
	return text
}

func writeAirportCSVFixture(t *testing.T, airports []AirportData) []byte {
	t.Helper()
	var output strings.Builder
	writer := csv.NewWriter(&output)
	if err := writer.Write([]string{"type", "name", "latitude_deg", "longitude_deg", "elevation_ft", "iso_country", "iso_region", "municipality", "icao_code", "iata_code", "scheduled_service"}); err != nil {
		t.Fatalf("write airport CSV header: %v", err)
	}
	for _, airport := range withMinimumAirportRecords(airports) {
		if airport.Type == "" {
			airport.Type = "small_airport"
		}
		if airport.CountryCode == "" {
			airport.CountryCode = "GB"
		}
		scheduled := "no"
		if airport.Scheduled {
			scheduled = "yes"
		}
		if err := writer.Write([]string{
			airport.Type, airport.Name, strconv.FormatFloat(airport.Lat, 'f', -1, 64), strconv.FormatFloat(airport.Lon, 'f', -1, 64),
			strconv.Itoa(airport.Elevation), airport.CountryCode, airport.CountryCode + "-" + airport.State,
			airport.City, airport.ICAO, airport.IATA, scheduled,
		}); err != nil {
			t.Fatalf("write airport CSV row: %v", err)
		}
	}
	writer.Flush()
	if err := writer.Error(); err != nil {
		t.Fatalf("flush airport CSV fixture: %v", err)
	}
	return []byte(output.String())
}

// TestAirport_LoadAirports_FromCache covers loading entirely from an
// on-disk cache file (no network access), excluding invalid coordinates and
// building both ICAO and IATA indexes.
func TestAirport_LoadAirports_FromCache(t *testing.T) {
	cachePath := filepath.Join(t.TempDir(), "airports.json")
	writeAirportFixture(t, cachePath, []AirportData{
		{ICAO: "KJFK", IATA: "JFK", Name: "JFK", City: "New York", Country: "US", Lat: 40.64, Lon: -73.77},
		{ICAO: "ZZZZ", IATA: "", Name: "No Coords Field", City: "Nowhere", Country: "US", Lat: 0, Lon: 0},
	})

	as := &AirportService{
		airports:  make(map[string]*AirportData),
		iataIndex: make(map[string]*AirportData),
		dataURL:   "http://127.0.0.1:1/unreachable", // must never be hit: cache satisfies the load
		cachePath: cachePath,
	}

	as.loadAirports()

	if !as.IsLoaded() {
		t.Fatal("expected database to be loaded from cache")
	}
	if _, err := as.LookupAirport("KJFK"); err != nil {
		t.Errorf("expected KJFK to be indexed: %v", err)
	}
	if _, err := as.LookupAirport("JFK"); err != nil {
		t.Errorf("expected JFK (IATA) to be indexed: %v", err)
	}
	if _, err := as.LookupAirport("ZZZZ"); err == nil {
		t.Error("expected airport with zero coordinates to be excluded")
	}
}

// TestAirport_LoadAirports_FromHTTP covers the cache-miss path: the service
// downloads from dataURL (a local httptest server, never a real network
// endpoint) and persists the result to cachePath.
func TestAirport_LoadAirports_FromHTTP(t *testing.T) {
	fixture := []AirportData{{ICAO: "EGLL", IATA: "LHR", Name: "Heathrow", City: "London", Country: "United Kingdom", CountryCode: "GB", Lat: 51.47, Lon: -0.4543}}
	body := writeAirportCSVFixture(t, fixture)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/csv")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(body)
	}))
	defer server.Close()

	cachePath := filepath.Join(t.TempDir(), "airports.json") // does not exist yet: forces download
	as := &AirportService{
		airports:  make(map[string]*AirportData),
		iataIndex: make(map[string]*AirportData),
		dataURL:   server.URL,
		cachePath: cachePath,
	}

	as.loadAirports()

	if !as.IsLoaded() {
		t.Fatal("expected database to be loaded from HTTP source")
	}
	if _, err := as.LookupAirport("LHR"); err != nil {
		t.Errorf("expected LHR to be indexed after download: %v", err)
	}

	if _, err := os.Stat(cachePath); err != nil {
		t.Errorf("expected downloaded data to be cached to disk: %v", err)
	}
}

func TestParseOurAirportsCSV(t *testing.T) {
	body := writeAirportCSVFixture(t, []AirportData{{
		ICAO: "KJFK", IATA: "JFK", Name: "John F Kennedy International Airport", City: "New York",
		State: "NY", CountryCode: "US", Type: "large_airport", Scheduled: true,
		Elevation: 13, Lat: 40.63980103, Lon: -73.77890015,
	}})
	airports, err := parseOurAirportsCSV(body)
	if err != nil {
		t.Fatalf("parseOurAirportsCSV() error = %v", err)
	}
	if len(airports) < minAirportRecords {
		t.Fatalf("airport count = %d, want at least %d", len(airports), minAirportRecords)
	}
	var jfk *AirportData
	for i := range airports {
		if airports[i].ICAO == "KJFK" {
			jfk = &airports[i]
			break
		}
	}
	if jfk == nil {
		t.Fatal("JFK record was not parsed")
	}
	if jfk.IATA != "JFK" || jfk.City != "New York" || jfk.Country != "United States" || jfk.CountryCode != "US" || !jfk.Scheduled {
		t.Errorf("parsed JFK metadata = %+v", jfk)
	}
	if jfk.Lat != 40.63980103 || jfk.Lon != -73.77890015 {
		t.Errorf("parsed JFK coordinates = %.8f, %.8f", jfk.Lat, jfk.Lon)
	}
}

func TestParseOurAirportsCSVRejectsBadSchema(t *testing.T) {
	if _, err := parseOurAirportsCSV([]byte("name,latitude_deg,longitude_deg\nJFK,40,-73\n")); err == nil {
		t.Fatal("expected an error when required source columns are absent")
	}
}

// TestAirport_LoadAirports_MalformedCSV covers the parse-error path when the
// response is missing the required OurAirports columns.
func TestAirport_LoadAirports_MalformedCSV(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("not,csv,with,required,columns\n1,2,3,4,5\n"))
	}))
	defer server.Close()

	as := &AirportService{
		airports:  make(map[string]*AirportData),
		iataIndex: make(map[string]*AirportData),
		dataURL:   server.URL,
		cachePath: filepath.Join(t.TempDir(), "airports.json"),
	}

	as.loadAirports()

	if as.IsLoaded() {
		t.Error("expected database to remain unloaded after malformed CSV response")
	}
}

// TestAirport_LoadAirports_HTTPUnreachable covers the network-error path
// when the download fails outright (connection refused).
func TestAirport_LoadAirports_HTTPUnreachable(t *testing.T) {
	as := &AirportService{
		airports:  make(map[string]*AirportData),
		iataIndex: make(map[string]*AirportData),
		dataURL:   "http://127.0.0.1:1/unreachable",
		cachePath: filepath.Join(t.TempDir(), "airports.json"),
	}

	as.loadAirports()

	if as.IsLoaded() {
		t.Error("expected database to remain unloaded when download is unreachable")
	}
}

// TestAirport_Reload covers reload success (from a fixture cache) and
// reload failure (network unreachable, no cache) surfacing as an error.
func TestAirport_Reload(t *testing.T) {
	t.Run("failed refresh keeps validated cache available", func(t *testing.T) {
		cachePath := filepath.Join(t.TempDir(), "airports.json")
		writeAirportFixture(t, cachePath, []AirportData{
			{ICAO: "KJFK", IATA: "JFK", Name: "JFK", City: "New York", Country: "US", Lat: 40.64, Lon: -73.77},
		})

		as := &AirportService{
			airports:  make(map[string]*AirportData),
			iataIndex: make(map[string]*AirportData),
			dataURL:   "http://127.0.0.1:1/unreachable",
			cachePath: cachePath,
			loaded:    true, // simulate a previously-loaded service being reloaded
		}

		if err := as.Reload(); err == nil {
			t.Fatal("Reload() error = nil, want the upstream failure reported")
		}
		if !as.IsLoaded() {
			t.Error("expected IsLoaded() = true after successful reload")
		}
	})

	t.Run("failed reload surfaces error", func(t *testing.T) {
		as := &AirportService{
			airports:  make(map[string]*AirportData),
			iataIndex: make(map[string]*AirportData),
			dataURL:   "http://127.0.0.1:1/unreachable",
			cachePath: filepath.Join(t.TempDir(), "airports.json"),
		}

		if err := as.Reload(); err == nil {
			t.Error("expected error when reload cannot load any data, got nil")
		}
	})
}

// TestAirport_NewAirportService_Defaults covers the default dataURL/
// cachePath assignment without ever touching the network: the cache file
// is pre-populated so the background loader's cache-hit branch is taken.
func TestAirport_NewAirportService_Defaults(t *testing.T) {
	cachePath := filepath.Join(t.TempDir(), "airports.json")
	writeAirportFixture(t, cachePath, []AirportData{
		{ICAO: "KJFK", IATA: "JFK", Name: "JFK", City: "New York", Country: "US", Lat: 40.64, Lon: -73.77},
	})

	as := NewAirportService("", cachePath)

	if as.dataURL == "" {
		t.Error("expected a non-empty default dataURL")
	}
	if as.cachePath != cachePath {
		t.Errorf("cachePath = %q, want %q", as.cachePath, cachePath)
	}

	deadline := time.Now().Add(5 * time.Second)
	for !as.IsLoaded() && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if !as.IsLoaded() {
		t.Fatal("timed out waiting for background load from cache")
	}
	if _, err := as.LookupAirport("JFK"); err != nil {
		t.Errorf("expected JFK to be indexed: %v", err)
	}
}
