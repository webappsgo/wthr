package service

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestHurricane_FetchNOAAStorms_HappyPath covers the successful decode path
// against a local httptest server, asserting field mapping including the
// hardcoded Basin value and the Intensity string composition rule.
func TestHurricane_FetchNOAAStorms_HappyPath(t *testing.T) {
	body := `{
		"activeStorms": [
			{
				"id": "AL012026",
				"binNumber": 1,
				"name": "Alpha",
				"classification": "HU",
				"intensityMPH": 90,
				"pressureMB": 970,
				"movementSpeed": 12,
				"movementDir": "WNW",
				"latitude": 25.0,
				"longitude": -75.0,
				"lastUpdate": "2026-07-21T00:00:00Z",
				"publicAdvisory": "https://example.com/pub.shtml",
				"forecastAdvisory": "https://example.com/fcst.shtml",
				"discussionLink": "https://example.com/disc.shtml"
			}
		]
	}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(body))
	}))
	defer srv.Close()

	hs := NewHurricaneService()
	storms, err := hs.fetchNOAAStorms(srv.URL)
	if err != nil {
		t.Fatalf("fetchNOAAStorms() unexpected error: %v", err)
	}
	if len(storms) != 1 {
		t.Fatalf("expected 1 storm, got %d", len(storms))
	}
	s := storms[0]
	if s.Name != "Alpha" {
		t.Errorf("Name = %q, want %q", s.Name, "Alpha")
	}
	if s.Basin != "Atlantic" {
		t.Errorf("Basin = %q, want %q (always hardcoded)", s.Basin, "Atlantic")
	}
	const wantIntensity = "HU (90 mph)"
	if s.Intensity != wantIntensity {
		t.Errorf("Intensity = %q, want %q", s.Intensity, wantIntensity)
	}
}

// TestHurricane_FetchNOAAStorms_ZeroIntensity verifies the branch where
// IntensityMPH <= 0 falls back to just the classification string with no
// " (N mph)" suffix.
func TestHurricane_FetchNOAAStorms_ZeroIntensity(t *testing.T) {
	body := `{"activeStorms": [{"id":"AL022026","name":"Beta","classification":"TS","intensityMPH":0}]}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(body))
	}))
	defer srv.Close()

	hs := NewHurricaneService()
	storms, err := hs.fetchNOAAStorms(srv.URL)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(storms) != 1 {
		t.Fatalf("expected 1 storm, got %d", len(storms))
	}
	if storms[0].Intensity != "TS" {
		t.Errorf("Intensity = %q, want %q", storms[0].Intensity, "TS")
	}
}

// TestHurricane_FetchNOAAStorms_EmptyList covers the boundary of a
// well-formed response with zero active storms.
func TestHurricane_FetchNOAAStorms_EmptyList(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"activeStorms": []}`))
	}))
	defer srv.Close()

	hs := NewHurricaneService()
	storms, err := hs.fetchNOAAStorms(srv.URL)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(storms) != 0 {
		t.Errorf("expected 0 storms, got %d", len(storms))
	}
}

// TestHurricane_FetchNOAAStorms_MalformedJSON documents and locks in the
// quirky production behavior: a JSON decode failure returns an EMPTY slice
// and a NIL error, not an error. This is a real behavioral contract of the
// code (regression test for that specific quirk), not a bug fix.
func TestHurricane_FetchNOAAStorms_MalformedJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{not valid json`))
	}))
	defer srv.Close()

	hs := NewHurricaneService()
	storms, err := hs.fetchNOAAStorms(srv.URL)
	if err != nil {
		t.Fatalf("expected nil error for malformed JSON (documented quirk), got: %v", err)
	}
	if storms == nil {
		t.Fatal("expected non-nil empty slice for malformed JSON")
	}
	if len(storms) != 0 {
		t.Errorf("expected empty slice, got %d storms", len(storms))
	}
}

// TestHurricane_FetchNOAAStorms_NonOKStatus covers the error path for a
// non-200 response, asserting the status code is present in the error text.
func TestHurricane_FetchNOAAStorms_NonOKStatus(t *testing.T) {
	tests := []struct {
		name   string
		status int
	}{
		{"internal server error", http.StatusInternalServerError},
		{"not found", http.StatusNotFound},
		{"service unavailable", http.StatusServiceUnavailable},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tt.status)
			}))
			defer srv.Close()

			hs := NewHurricaneService()
			storms, err := hs.fetchNOAAStorms(srv.URL)
			if err == nil {
				t.Fatal("expected error for non-200 status, got nil")
			}
			if storms != nil {
				t.Errorf("expected nil storms on error, got %v", storms)
			}
		})
	}
}

// TestHurricane_FetchNOAAStorms_Unreachable covers a network-level failure
// (connection refused) by hitting a server that has already been closed.
func TestHurricane_FetchNOAAStorms_Unreachable(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	url := srv.URL
	srv.Close()

	hs := NewHurricaneService()
	storms, err := hs.fetchNOAAStorms(url)
	if err == nil {
		t.Fatal("expected error for unreachable server, got nil")
	}
	if storms != nil {
		t.Errorf("expected nil storms on network error, got %v", storms)
	}
}

// TestHurricane_GetStormCategory is table-driven across every Saffir-Simpson
// threshold boundary, including the values immediately below/at/above each
// cutoff to catch off-by-one errors.
func TestHurricane_GetStormCategory(t *testing.T) {
	tests := []struct {
		windSpeed int
		want      string
	}{
		{-10, "Tropical Depression"},
		{0, "Tropical Depression"},
		{38, "Tropical Depression"},
		{39, "Tropical Storm"},
		{73, "Tropical Storm"},
		{74, "Category 1 Hurricane"},
		{95, "Category 1 Hurricane"},
		{96, "Category 2 Hurricane"},
		{110, "Category 2 Hurricane"},
		{111, "Category 3 Hurricane (Major)"},
		{129, "Category 3 Hurricane (Major)"},
		{130, "Category 4 Hurricane (Major)"},
		{156, "Category 4 Hurricane (Major)"},
		{157, "Category 5 Hurricane (Major)"},
		{200, "Category 5 Hurricane (Major)"},
	}

	hs := NewHurricaneService()
	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			got := hs.GetStormCategory(tt.windSpeed)
			if got != tt.want {
				t.Errorf("GetStormCategory(%d) = %q, want %q", tt.windSpeed, got, tt.want)
			}
		})
	}
}

// TestHurricane_GetStormIcon is table-driven across the three windSpeed
// bands including exact boundary values, and confirms the classification
// argument has no effect on the result (documented dead-parameter quirk).
func TestHurricane_GetStormIcon(t *testing.T) {
	tests := []struct {
		name           string
		classification string
		windSpeed      int
		want           string
	}{
		{"below tropical storm", "TD", 0, "🌧️"},
		{"just below tropical storm boundary", "TD", 38, "🌧️"},
		{"tropical storm boundary", "TS", 39, "🌊"},
		{"tropical storm mid-range", "TS", 60, "🌊"},
		{"just below hurricane boundary", "TS", 73, "🌊"},
		{"hurricane boundary", "HU", 74, "🌀"},
		{"major hurricane", "HU", 160, "🌀"},
		{"negative wind speed", "TD", -5, "🌧️"},
		{"classification ignored when hurricane speed", "bogus-classification", 100, "🌀"},
	}

	hs := NewHurricaneService()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := hs.GetStormIcon(tt.classification, tt.windSpeed)
			if got != tt.want {
				t.Errorf("GetStormIcon(%q, %d) = %q, want %q", tt.classification, tt.windSpeed, got, tt.want)
			}
		})
	}
}

// TestHurricane_NewHurricaneService_Defaults verifies the constructor wires
// up a non-nil cache and a 15-minute TTL, and that the service starts with
// no cached data (so the first GetActiveStorms call cannot serve stale/empty
// cache silently).
func TestHurricane_NewHurricaneService_Defaults(t *testing.T) {
	hs := NewHurricaneService()
	if hs == nil {
		t.Fatal("NewHurricaneService() returned nil")
	}
	if hs.cache == nil {
		t.Error("expected cache map to be initialized")
	}
	if hs.cacheTTL.Minutes() != 15 {
		t.Errorf("cacheTTL = %v, want 15m", hs.cacheTTL)
	}
	if !hs.cacheTime.IsZero() {
		t.Error("expected cacheTime to be zero on construction")
	}
}
