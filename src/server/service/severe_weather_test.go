package service

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func newTestSevereWeatherService() *SevereWeatherService {
	return NewSevereWeatherService()
}

func TestCalculateDistance(t *testing.T) {
	tests := []struct {
		name                   string
		lat1, lon1, lat2, lon2 float64
		wantMin, wantMax       float64
	}{
		{"same point", 40.0, -74.0, 40.0, -74.0, 0, 0},
		{"NYC to LA roughly", 40.7128, -74.0060, 34.0522, -118.2437, 2400, 2500},
		{"antipodal-ish", 0, 0, 0, 180, 12400, 12500},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := calculateDistance(tt.lat1, tt.lon1, tt.lat2, tt.lon2)
			if got < tt.wantMin || got > tt.wantMax {
				t.Errorf("calculateDistance() = %v, want between %v and %v", got, tt.wantMin, tt.wantMax)
			}
		})
	}
}

func TestSevereWeatherService_GetStormCategory(t *testing.T) {
	s := newTestSevereWeatherService()
	tests := []struct {
		windSpeed int
		want      string
	}{
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
	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			if got := s.GetStormCategory(tt.windSpeed); got != tt.want {
				t.Errorf("GetStormCategory(%d) = %q, want %q", tt.windSpeed, got, tt.want)
			}
		})
	}
}

func TestSevereWeatherService_GetStormIcon(t *testing.T) {
	s := newTestSevereWeatherService()
	tests := []struct {
		name           string
		classification string
		windSpeed      int
		want           string
	}{
		{"hurricane", "Hurricane", 100, "🌀"},
		{"boundary hurricane", "Hurricane", 74, "🌀"},
		{"tropical storm", "Tropical Storm", 50, "🌊"},
		{"boundary tropical storm", "Tropical Storm", 39, "🌊"},
		{"tropical depression", "Tropical Depression", 20, "🌧️"},
		{"unused classification param", "garbage", 100, "🌀"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := s.GetStormIcon(tt.classification, tt.windSpeed); got != tt.want {
				t.Errorf("GetStormIcon(%q,%d) = %q, want %q", tt.classification, tt.windSpeed, got, tt.want)
			}
		})
	}
}

func TestSevereWeatherService_GetAlertIcon(t *testing.T) {
	s := newTestSevereWeatherService()
	tests := []struct {
		name  string
		event string
		want  string
	}{
		{"tornado", "Tornado Warning", "🌪️"},
		{"severe thunderstorm", "Severe Thunderstorm Warning", "⛈️"},
		{"winter", "Winter Storm Warning", "❄️"},
		{"snow", "Heavy Snow Warning", "❄️"},
		{"blizzard", "Blizzard Warning", "❄️"},
		{"flood", "Flood Warning", "🌊"},
		{"hurricane", "Hurricane Warning", "🌀"},
		{"unmatched", "Air Quality Alert", "⚠️"},
		{"case insensitive", "TORNADO WATCH", "🌪️"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := s.GetAlertIcon(tt.event); got != tt.want {
				t.Errorf("GetAlertIcon(%q) = %q, want %q", tt.event, got, tt.want)
			}
		})
	}
}

func TestSevereWeatherService_getCountryFromCoordinates(t *testing.T) {
	s := newTestSevereWeatherService()
	tests := []struct {
		name     string
		lat, lon float64
		want     string
	}{
		{"zero coords default", 0, 0, "US"},
		{"continental US", 39.0, -100.0, "US"},
		{"alaska", 61.0, -150.0, "US"},
		{"hawaii", 20.0, -157.0, "US"},
		{"canada", 55.0, -100.0, "CA"},
		{"uk", 51.5, -0.1, "GB"},
		{"mexico", 20.0, -100.0, "MX"},
		{"australia", -25.0, 135.0, "AU"},
		{"japan", 35.0, 135.0, "JP"},
		{"unmapped ocean default", 0.1, 0.1, "US"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := s.getCountryFromCoordinates(tt.lat, tt.lon); got != tt.want {
				t.Errorf("getCountryFromCoordinates(%v,%v) = %q, want %q", tt.lat, tt.lon, got, tt.want)
			}
		})
	}
}

func TestSevereWeatherService_filterStormsByDistance(t *testing.T) {
	s := newTestSevereWeatherService()
	storms := []Storm{
		{ID: "near", Latitude: 40.0, Longitude: -74.0},
		{ID: "far", Latitude: 0.0, Longitude: 0.0},
	}
	got := s.filterStormsByDistance(storms, 40.0, -74.0, 100)
	if len(got) != 1 || got[0].ID != "near" {
		t.Errorf("filterStormsByDistance() = %+v, want only 'near'", got)
	}
	if got[0].DistanceMiles != 0 {
		t.Errorf("DistanceMiles for same point = %v, want 0", got[0].DistanceMiles)
	}
}

func TestSevereWeatherService_filterAlertsByDistance(t *testing.T) {
	s := newTestSevereWeatherService()

	t.Run("point geometry within range", func(t *testing.T) {
		alerts := []Alert{
			{
				ID: "near",
				Geometry: map[string]interface{}{
					"type":        "Point",
					"coordinates": []interface{}{-74.0, 40.0},
				},
			},
		}
		got := s.filterAlertsByDistance(alerts, 40.0, -74.0, 100)
		if len(got) != 1 {
			t.Fatalf("filterAlertsByDistance() len = %d, want 1", len(got))
		}
	})

	t.Run("point geometry out of range excluded", func(t *testing.T) {
		alerts := []Alert{
			{
				ID: "far",
				Geometry: map[string]interface{}{
					"type":        "Point",
					"coordinates": []interface{}{0.0, 0.0},
				},
			},
		}
		got := s.filterAlertsByDistance(alerts, 40.0, -74.0, 10)
		if len(got) != 0 {
			t.Errorf("filterAlertsByDistance() len = %d, want 0", len(got))
		}
	})

	t.Run("nil geometry excluded", func(t *testing.T) {
		alerts := []Alert{{ID: "no-geometry"}}
		got := s.filterAlertsByDistance(alerts, 40.0, -74.0, 100)
		if len(got) != 0 {
			t.Errorf("filterAlertsByDistance() with nil geometry len = %d, want 0", len(got))
		}
	})

	t.Run("empty input", func(t *testing.T) {
		got := s.filterAlertsByDistance(nil, 40.0, -74.0, 100)
		if len(got) != 0 {
			t.Errorf("filterAlertsByDistance(nil) len = %d, want 0", len(got))
		}
	})
}

func TestIsAlertNearLocation(t *testing.T) {
	t.Run("unparseable geometry included by default", func(t *testing.T) {
		if !isAlertNearLocation("not-a-map", 40.0, -74.0, 10) {
			t.Error("expected true for unparseable geometry")
		}
	})

	t.Run("missing type included by default", func(t *testing.T) {
		geom := map[string]interface{}{"coordinates": []interface{}{-74.0, 40.0}}
		if !isAlertNearLocation(geom, 40.0, -74.0, 10) {
			t.Error("expected true when type missing")
		}
	})

	t.Run("point within range", func(t *testing.T) {
		geom := map[string]interface{}{
			"type":        "Point",
			"coordinates": []interface{}{-74.0, 40.0},
		}
		if !isAlertNearLocation(geom, 40.0, -74.0, 10) {
			t.Error("expected true for point at same location")
		}
	})

	t.Run("point outside range", func(t *testing.T) {
		geom := map[string]interface{}{
			"type":        "Point",
			"coordinates": []interface{}{0.0, 0.0},
		}
		if isAlertNearLocation(geom, 40.0, -74.0, 1) {
			t.Error("expected false for distant point")
		}
	})

	t.Run("unknown geometry type included by default", func(t *testing.T) {
		geom := map[string]interface{}{
			"type":        "LineString",
			"coordinates": []interface{}{},
		}
		if !isAlertNearLocation(geom, 40.0, -74.0, 10) {
			t.Error("expected true for unknown geometry type")
		}
	})

	t.Run("polygon with nearby point", func(t *testing.T) {
		geom := map[string]interface{}{
			"type": "Polygon",
			"coordinates": []interface{}{
				[]interface{}{
					[]interface{}{-74.0, 40.0},
					[]interface{}{-75.0, 41.0},
				},
			},
		}
		if !isAlertNearLocation(geom, 40.0, -74.0, 10) {
			t.Error("expected true for polygon containing nearby point")
		}
	})
}

func TestCheckPolygonDistance(t *testing.T) {
	t.Run("nearby point found", func(t *testing.T) {
		coords := []interface{}{
			[]interface{}{-74.0, 40.0},
		}
		if !checkPolygonDistance(coords, 40.0, -74.0, 10) {
			t.Error("expected true for nearby coordinates")
		}
	})

	t.Run("no nearby point", func(t *testing.T) {
		coords := []interface{}{
			[]interface{}{0.0, 0.0},
		}
		if checkPolygonDistance(coords, 40.0, -74.0, 1) {
			t.Error("expected false for distant coordinates")
		}
	})
}

func TestGetAlertDistance(t *testing.T) {
	t.Run("unparseable geometry returns zero", func(t *testing.T) {
		if got := getAlertDistance("bad", 40.0, -74.0); got != 0 {
			t.Errorf("getAlertDistance(bad geometry) = %v, want 0", got)
		}
	})

	t.Run("point geometry returns distance", func(t *testing.T) {
		geom := map[string]interface{}{
			"type":        "Point",
			"coordinates": []interface{}{-74.0, 40.0},
		}
		if got := getAlertDistance(geom, 40.0, -74.0); got != 0 {
			t.Errorf("getAlertDistance(same point) = %v, want 0", got)
		}
	})
}

func TestFindMinDistance(t *testing.T) {
	coords := []interface{}{
		[]interface{}{-74.0, 40.0},
		[]interface{}{0.0, 0.0},
	}
	minDist := 9999.0
	findMinDistance(coords, 40.0, -74.0, &minDist)
	if minDist != 0 {
		t.Errorf("findMinDistance() minDist = %v, want 0", minDist)
	}
}

func TestExtractAreaFromTitle(t *testing.T) {
	tests := []struct {
		name  string
		title string
		want  string
	}{
		{"for pattern", "Tornado Warning for Cook County", "Cook County"},
		{"in effect for pattern", "Flood Warning in effect for River Valley", "River Valley"},
		{"no match", "Just a title", "Multiple areas"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := extractAreaFromTitle(tt.title); got != tt.want {
				t.Errorf("extractAreaFromTitle(%q) = %q, want %q", tt.title, got, tt.want)
			}
		})
	}
}

func TestExtractUKArea(t *testing.T) {
	tests := []struct {
		name  string
		title string
		want  string
	}{
		{"england", "Yellow warning for England", "England"},
		{"scotland", "Amber warning for Scotland", "Scotland"},
		{"wales", "Warning for Wales", "Wales"},
		{"northern ireland", "Warning for Northern Ireland", "Northern Ireland"},
		{"default", "Warning for somewhere else", "United Kingdom"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := extractUKArea(tt.title); got != tt.want {
				t.Errorf("extractUKArea(%q) = %q, want %q", tt.title, got, tt.want)
			}
		})
	}
}

func TestExtractAustraliaArea(t *testing.T) {
	tests := []struct {
		name  string
		title string
		want  string
	}{
		{"abbreviation NSW", "Warning for NSW", "NSW"},
		{"abbreviation QLD", "Warning for QLD region", "QLD"},
		// The word "Warning" itself contains the substring "WA", and the
		// abbreviation-loop check runs before the full-name check and scans
		// the whole title (not word-boundaries), so any title containing
		// "Warning" resolves to "WA" before the full-name branch is ever
		// reached. These subtests avoid that word to actually exercise the
		// full-name branch; see the New South Wales case below for a title
		// where the bug is unavoidable.
		{"full name Queensland", "Alert for Queensland", "QLD"},
		{"full name Victoria", "Alert for Victoria", "VIC"},
		{"full name Tasmania", "Alert for Tasmania", "TAS"},
		{"full name South Australia", "Alert for South Australia", "SA"},
		{"full name Western Australia", "Alert for Western Australia", "WA"},
		{"full name Northern Territory", "Alert for Northern Territory", "NT"},
		// BUG: "New South Wales" contains "Wales", which contains the
		// substring "WA". Because the abbreviation loop runs first and
		// matches on substrings anywhere in the title, this always resolves
		// to "WA" instead of "NSW" — the "New South Wales" full-name branch
		// is unreachable dead code. Documenting actual behavior, not the
		// intended behavior.
		{"full name New South Wales resolves to WA due to substring bug", "Alert for New South Wales", "WA"},
		{"default", "Alert for nowhere", "Australia"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := extractAustraliaArea(tt.title); got != tt.want {
				t.Errorf("extractAustraliaArea(%q) = %q, want %q", tt.title, got, tt.want)
			}
		})
	}
}

func TestMapJapaneseWarningType(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"heavy rain", "大雨", "Heavy Rain Warning"},
		{"tsunami", "津波", "Tsunami Warning"},
		{"typhoon", "台風", "Typhoon Warning"},
		{"unknown falls back", "未知", "未知 Warning"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := mapJapaneseWarningType(tt.input); got != tt.want {
				t.Errorf("mapJapaneseWarningType(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestMapJapaneseSeverity(t *testing.T) {
	tests := []struct {
		name        string
		warningType string
		status      string
		want        string
	}{
		{"special warning status", "大雨", "特別警報発表", "Extreme"},
		{"warning status", "大雨", "警報発表", "Severe"},
		{"advisory status", "大雨", "注意報発表", "Moderate"},
		{"tsunami type overrides plain status", "津波", "", "Extreme"},
		{"earthquake type overrides plain status", "地震", "", "Extreme"},
		{"typhoon type", "台風", "", "Severe"},
		{"default moderate", "大雨", "", "Moderate"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := mapJapaneseSeverity(tt.warningType, tt.status); got != tt.want {
				t.Errorf("mapJapaneseSeverity(%q,%q) = %q, want %q", tt.warningType, tt.status, got, tt.want)
			}
		})
	}
}

func TestMapSpanishEventType(t *testing.T) {
	// Only unambiguous inputs are asserted here: "tormenta" is a substring of
	// "tormenta tropical" and Go map iteration order is randomized, so testing
	// "tormenta tropical" would be flaky against the production implementation.
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"lluvia", "Alerta por lluvia intensa", "Heavy Rain Warning"},
		{"huracan", "Alerta por huracán", "Hurricane Warning"},
		{"case insensitive", "ALERTA POR NEVADA", "Snow Warning"},
		{"unmapped falls back", "algo raro", "algo raro Warning"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := mapSpanishEventType(tt.input); got != tt.want {
				t.Errorf("mapSpanishEventType(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestSevereWeatherService_fetchNOAAStorms(t *testing.T) {
	t.Run("successful response", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{
				"activeStorms": [
					{
						"id": "AL012024",
						"name": "Test Storm",
						"classification": "HU",
						"intensityMPH": 85,
						"pressureMB": 970,
						"latitude": 25.5,
						"longitude": -80.0,
						"lastUpdate": "2024-08-01T00:00:00Z"
					}
				]
			}`))
		}))
		defer server.Close()

		s := newTestSevereWeatherService()
		storms, err := s.fetchNOAAStorms(server.URL)
		if err != nil {
			t.Fatalf("fetchNOAAStorms returned error: %v", err)
		}
		if len(storms) != 1 {
			t.Fatalf("len(storms) = %d, want 1", len(storms))
		}
		if storms[0].Intensity != "HU (85 mph)" {
			t.Errorf("Intensity = %q, want 'HU (85 mph)'", storms[0].Intensity)
		}
		if storms[0].Basin != "Atlantic" {
			t.Errorf("Basin = %q, want Atlantic", storms[0].Basin)
		}
	})

	t.Run("no active storms empty array", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"activeStorms": []}`))
		}))
		defer server.Close()

		s := newTestSevereWeatherService()
		storms, err := s.fetchNOAAStorms(server.URL)
		if err != nil {
			t.Fatalf("fetchNOAAStorms returned error: %v", err)
		}
		if len(storms) != 0 {
			t.Errorf("len(storms) = %d, want 0", len(storms))
		}
	})

	t.Run("non-200 status returns error", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		}))
		defer server.Close()

		s := newTestSevereWeatherService()
		_, err := s.fetchNOAAStorms(server.URL)
		if err == nil {
			t.Fatal("expected error for non-200 status")
		}
	})

	t.Run("malformed json treated as no storms", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte(`not json`))
		}))
		defer server.Close()

		s := newTestSevereWeatherService()
		storms, err := s.fetchNOAAStorms(server.URL)
		if err != nil {
			t.Fatalf("fetchNOAAStorms returned error for malformed json: %v", err)
		}
		if len(storms) != 0 {
			t.Errorf("len(storms) = %d, want 0 for malformed json", len(storms))
		}
	})
}
