package handler

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// newAPITestRequest builds a bare GET request/recorder pair, mirroring
// the pattern used in weather_test.go, kept local to avoid depending on
// unexported helpers owned by another file beyond handler_helpers_test.go.
func newAPITestRequest(target string) (*http.Request, *httptest.ResponseRecorder) {
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, target, nil)
	return r, w
}

// GetWeather validates city_id / lat / lon before ever touching the (here
// nil) weatherService or locationEnhancer, so all of these error branches
// are reachable without a live service and without any network calls.
func TestAPIHandler_GetWeather_ValidationErrors(t *testing.T) {
	h := NewAPIHandler(nil, nil)

	tests := []struct {
		name       string
		query      string
		wantStatus int
	}{
		{"invalid city_id format", "?city_id=abc", http.StatusBadRequest},
		{"lat without lon", "?lat=10", http.StatusBadRequest},
		{"lon without lat", "?lon=10", http.StatusBadRequest},
		{"invalid lat format", "?lat=abc&lon=10", http.StatusBadRequest},
		{"invalid lon format", "?lat=10&lon=abc", http.StatusBadRequest},
		{"lat out of range low", "?lat=-91&lon=10", http.StatusBadRequest},
		{"lat out of range high", "?lat=91&lon=10", http.StatusBadRequest},
		{"lon out of range low", "?lat=10&lon=-181", http.StatusBadRequest},
		{"lon out of range high", "?lat=10&lon=181", http.StatusBadRequest},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r, w := newAPITestRequest("/api/v1/weather" + tt.query)
			h.GetWeather(w, r)
			if w.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d; body=%s", w.Code, tt.wantStatus, w.Body.String())
			}
		})
	}
}

func TestAPIHandler_GetForecast_ValidationErrors(t *testing.T) {
	h := NewAPIHandler(nil, nil)

	tests := []struct {
		name       string
		query      string
		wantStatus int
	}{
		{"lat without lon", "?lat=10", http.StatusBadRequest},
		{"lon without lat", "?lon=10", http.StatusBadRequest},
		{"invalid lat format", "?lat=abc&lon=10", http.StatusBadRequest},
		{"invalid lon format", "?lat=10&lon=abc", http.StatusBadRequest},
		{"lat out of range", "?lat=-91&lon=10", http.StatusBadRequest},
		{"lon out of range", "?lat=10&lon=-181", http.StatusBadRequest},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r, w := newAPITestRequest("/api/v1/forecast" + tt.query)
			h.GetForecast(w, r)
			if w.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d; body=%s", w.Code, tt.wantStatus, w.Body.String())
			}
		})
	}
}

func TestAPIHandler_SearchLocations_MissingQuery(t *testing.T) {
	h := NewAPIHandler(nil, nil)
	r, w := newAPITestRequest("/api/v1/search")
	h.SearchLocations(w, r)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d; body=%s", w.Code, http.StatusBadRequest, w.Body.String())
	}
}

// GetIP has no service dependency at all: it only reads headers off the
// request, so it's fully exercisable with a nil-service handler.
func TestAPIHandler_GetIP(t *testing.T) {
	h := NewAPIHandler(nil, nil)
	r, w := newAPITestRequest("/api/v1/ip")
	r.Header.Set("X-Forwarded-For", "203.0.113.5")
	r.Header.Set("X-Real-IP", "203.0.113.6")

	h.GetIP(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", w.Code, http.StatusOK, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "203.0.113.5") {
		t.Errorf("body = %s, want it to contain the forwarded-for header value", w.Body.String())
	}
}

// GetDocsJSON is a pure builder with no service dependency.
func TestAPIHandler_GetDocsJSON(t *testing.T) {
	h := NewAPIHandler(nil, nil)
	r, w := newAPITestRequest("/api/v1/docs")

	h.GetDocsJSON(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", w.Code, http.StatusOK, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "Weather API") {
		t.Errorf("body = %s, want it to describe the Weather API", w.Body.String())
	}
}

// GetHistoricalWeather validates lat/lon and the date/years parameters
// before calling the weather service, so those branches are reachable with
// a nil-service handler; the "no location provided at all" branch and date
// parsing errors are pure logic as well.
func TestAPIHandler_GetHistoricalWeather_ValidationErrors(t *testing.T) {
	h := NewAPIHandler(nil, nil)

	tests := []struct {
		name       string
		query      string
		wantStatus int
	}{
		{"no location or coordinates", "", http.StatusBadRequest},
		{"lat without lon", "?lat=10", http.StatusBadRequest},
		{"invalid lat format", "?lat=abc&lon=10", http.StatusBadRequest},
		{"invalid lon format", "?lat=10&lon=abc", http.StatusBadRequest},
		{"lat out of range", "?lat=-91&lon=10", http.StatusBadRequest},
		{"lon out of range", "?lat=10&lon=-181", http.StatusBadRequest},
		{"invalid date format", "?lat=10&lon=10&date=notadate", http.StatusBadRequest},
		{"years too low", "?lat=10&lon=10&years=0", http.StatusBadRequest},
		{"years too high", "?lat=10&lon=10&years=101", http.StatusBadRequest},
		{"invalid years format", "?lat=10&lon=10&years=abc", http.StatusBadRequest},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r, w := newAPITestRequest("/api/v1/history" + tt.query)
			h.GetHistoricalWeather(w, r)
			if w.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d; body=%s", w.Code, tt.wantStatus, w.Body.String())
			}
		})
	}
}

// parseHistoricalDateAPI is a pure function supporting three date formats
// plus a "no date supplied -> today" default.
func TestParseHistoricalDateAPI(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantMonth int
		wantDay   int
		wantYear  int
		wantErr   bool
	}{
		{"empty defaults to today", "", 0, 0, 0, false},
		{"ISO format", "2024-03-15", 3, 15, 2024, false},
		{"US format with year", "03/15/2024", 3, 15, 2024, false},
		{"US format without year", "03/15", 3, 15, 0, false},
		{"unsupported format", "15-03-2024", 0, 0, 0, true},
		{"garbage", "not-a-date", 0, 0, 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			month, day, year, err := parseHistoricalDateAPI(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected an error, got month=%d day=%d year=%d", month, day, year)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tt.input == "" {
				// "today" is environment-dependent; just assert it parsed
				// to some plausible calendar date rather than the zero value.
				if month < 1 || month > 12 || day < 1 || day > 31 || year < 1 {
					t.Errorf("today defaults out of range: month=%d day=%d year=%d", month, day, year)
				}
				return
			}
			if month != tt.wantMonth || day != tt.wantDay {
				t.Errorf("month=%d day=%d, want month=%d day=%d", month, day, tt.wantMonth, tt.wantDay)
			}
			if tt.wantYear != 0 && year != tt.wantYear {
				t.Errorf("year=%d, want %d", year, tt.wantYear)
			}
		})
	}
}
