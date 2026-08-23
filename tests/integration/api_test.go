package integration_test

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/webappsgo/wthr/src/database"
	"github.com/webappsgo/wthr/src/server/handler"
	"github.com/webappsgo/wthr/src/server/service"
	_ "modernc.org/sqlite"
)

func setupIntegrationTest(t *testing.T) (chi.Router, *database.DB) {
	raw, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("Failed to initialize test database: %v", err)
	}
	db := &database.DB{DB: raw}

	r := chi.NewRouter()
	locationEnhancer := service.NewLocationEnhancer(db.DB)
	weatherService := service.NewWeatherService(locationEnhancer, nil)
	apiHandler := handler.NewAPIHandler(weatherService, locationEnhancer)

	// Setup API routes
	r.Route("/api/v1", func(api chi.Router) {
		api.Get("/weather", apiHandler.GetWeather)
		api.Get("/forecast", apiHandler.GetForecast)
		api.Get("/search", apiHandler.SearchLocations)
	})

	return r, db
}

func TestAPI_Weather_Coordinates(t *testing.T) {
	r, db := setupIntegrationTest(t)
	defer db.Close()

	tests := []struct {
		name       string
		lat        string
		lon        string
		wantStatus int
		liveNet    bool
	}{
		{"New York", "40.7128", "-74.0060", http.StatusOK, true},
		{"London", "51.5074", "-0.1278", http.StatusOK, true},
		{"Tokyo", "35.6762", "139.6503", http.StatusOK, true},
		{"Invalid latitude", "999", "0", http.StatusBadRequest, false},
		{"Missing longitude", "40.7128", "", http.StatusBadRequest, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.liveNet {
				// This case reaches the live api.open-meteo.com upstream,
				// which makes it non-deterministic for the Phase 1
				// toolchain gate (AI.md PART 29: Phase 1 must be provable
				// without a live network dependency). Equivalent coverage
				// lives in Phase 2 (tests/incus.sh's PUBLIC_API_ROUTES
				// `/api/v1/weather?lat=40.7128&lon=-74.0060` and
				// tests/docker.sh's matching check against the same
				// production route).
				t.Skip("live-network case covered in Phase 2 (tests/docker.sh, tests/incus.sh) per AI.md PART 29")
			}

			url := fmt.Sprintf("/api/v1/weather?lat=%s&lon=%s", tt.lat, tt.lon)
			req := httptest.NewRequest("GET", url, nil)
			w := httptest.NewRecorder()

			r.ServeHTTP(w, req)

			if w.Code != tt.wantStatus {
				t.Errorf("Expected status %d, got %d. Body: %s", tt.wantStatus, w.Code, w.Body.String())
			}

			if w.Code == http.StatusOK {
				var response map[string]interface{}
				if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
					t.Errorf("Failed to parse JSON response: %v", err)
				}

				// Check for required fields
				if _, ok := response["location"]; !ok {
					t.Error("Response missing 'location' field")
				}
				if _, ok := response["current"]; !ok {
					t.Error("Response missing 'current' field")
				}
			}
		})
	}
}

func TestAPI_Weather_CityID(t *testing.T) {
	r, db := setupIntegrationTest(t)
	defer db.Close()

	tests := []struct {
		name       string
		cityID     string
		wantStatus int
		liveNet    bool
	}{
		// Will be 404 until cities loaded
		{"Valid city ID", "5128581", http.StatusNotFound, true},
		{"Invalid city ID", "invalid", http.StatusBadRequest, false},
		{"Zero city ID", "0", http.StatusNotFound, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.liveNet {
				// This case reaches the live citylist download in the
				// location enhancer and, on a hit, the live
				// api.open-meteo.com upstream, which makes it
				// non-deterministic for the Phase 1 toolchain gate
				// (AI.md PART 29: Phase 1 must be provable without a live
				// network dependency). Equivalent coverage lives in Phase 2
				// (tests/incus.sh's PUBLIC_API_ROUTES
				// `/api/v1/weather?city_id=5128581` and tests/docker.sh's
				// matching check against the same production route).
				t.Skip("live-network case covered in Phase 2 (tests/docker.sh, tests/incus.sh) per AI.md PART 29")
			}

			url := fmt.Sprintf("/api/v1/weather?city_id=%s", tt.cityID)
			req := httptest.NewRequest("GET", url, nil)
			w := httptest.NewRecorder()

			r.ServeHTTP(w, req)

			if w.Code != tt.wantStatus {
				t.Logf("City ID test: %s - Status %d (expected %d)", tt.name, w.Code, tt.wantStatus)
			}
		})
	}
}

func TestAPI_Weather_Nearest(t *testing.T) {
	// This whole test reaches the live citylist download in the location
	// enhancer and the live api.open-meteo.com upstream on either branch
	// (nearest-city hit or coordinate fallback), which makes it
	// non-deterministic for the Phase 1 toolchain gate (AI.md PART 29:
	// Phase 1 must be provable without a live network dependency).
	// Equivalent coverage lives in Phase 2 (tests/incus.sh's
	// PUBLIC_API_ROUTES `/api/v1/weather?lat=40.7128&lon=-74.0060&nearest=true`
	// and tests/docker.sh's matching check against the same production route).
	t.Skip("live-network case covered in Phase 2 (tests/docker.sh, tests/incus.sh) per AI.md PART 29")

	r, db := setupIntegrationTest(t)
	defer db.Close()

	url := "/api/v1/weather?lat=40.7128&lon=-74.0060&nearest=true"
	req := httptest.NewRequest("GET", url, nil)
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	// Will work even without cities loaded (falls back to coordinates)
	if w.Code != http.StatusOK {
		t.Logf("Nearest city test returned status %d (fallback expected)", w.Code)
	}
}

func TestAPI_Forecast(t *testing.T) {
	r, db := setupIntegrationTest(t)
	defer db.Close()

	tests := []struct {
		name       string
		query      string
		wantStatus int
		liveNet    bool
	}{
		{"Default days", "lat=40.7128&lon=-74.0060", http.StatusOK, true},
		{"7 days", "lat=40.7128&lon=-74.0060&days=7", http.StatusOK, true},
		{"1 day", "lat=40.7128&lon=-74.0060&days=1", http.StatusOK, true},
		// Should default
		{"Invalid days", "lat=40.7128&lon=-74.0060&days=invalid", http.StatusOK, true},
		// Should cap
		{"Too many days", "lat=40.7128&lon=-74.0060&days=999", http.StatusOK, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.liveNet {
				// This case reaches the live api.open-meteo.com upstream,
				// which makes it non-deterministic for the Phase 1
				// toolchain gate (AI.md PART 29: Phase 1 must be provable
				// without a live network dependency). Equivalent coverage
				// lives in Phase 2 (tests/incus.sh's PUBLIC_API_ROUTES
				// `/api/v1/weather/forecast?lat=40.7128&lon=-74.0060&days=7`
				// and tests/docker.sh's matching check against the same
				// production route).
				t.Skip("live-network case covered in Phase 2 (tests/docker.sh, tests/incus.sh) per AI.md PART 29")
			}

			url := fmt.Sprintf("/api/v1/forecast?%s", tt.query)
			req := httptest.NewRequest("GET", url, nil)
			w := httptest.NewRecorder()

			r.ServeHTTP(w, req)

			if w.Code != tt.wantStatus {
				t.Errorf("Expected status %d, got %d. Body: %s", tt.wantStatus, w.Code, w.Body.String())
			}

			if w.Code == http.StatusOK {
				var response map[string]interface{}
				if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
					t.Errorf("Failed to parse JSON response: %v", err)
				}

				// Check for forecast object with days array
				if forecastObj, ok := response["forecast"].(map[string]interface{}); ok {
					if days, ok := forecastObj["days"].([]interface{}); ok {
						if len(days) == 0 {
							t.Error("Forecast days array is empty")
						}
					} else {
						t.Error("Forecast object missing 'days' array")
					}
				} else {
					t.Error("Response missing 'forecast' object")
				}
			}
		})
	}
}

func TestAPI_Search(t *testing.T) {
	r, db := setupIntegrationTest(t)
	defer db.Close()

	tests := []struct {
		name       string
		query      string
		wantStatus int
		liveNet    bool
	}{
		{"Valid search", "q=London", http.StatusOK, true},
		{"Empty query", "q=", http.StatusBadRequest, false},
		{"No query param", "", http.StatusBadRequest, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.liveNet {
				// This case reaches the live geocoding-api.open-meteo.com
				// upstream, which makes it non-deterministic for the
				// Phase 1 toolchain gate (AI.md PART 29: Phase 1 must be
				// provable without a live network dependency). Equivalent
				// coverage lives in Phase 2 (tests/incus.sh's
				// PUBLIC_API_ROUTES `/api/v1/locations/search?q=London`
				// and tests/docker.sh's matching check against the same
				// production route).
				t.Skip("live-network case covered in Phase 2 (tests/docker.sh, tests/incus.sh) per AI.md PART 29")
			}

			url := fmt.Sprintf("/api/v1/search?%s", tt.query)
			req := httptest.NewRequest("GET", url, nil)
			w := httptest.NewRecorder()

			r.ServeHTTP(w, req)

			if w.Code != tt.wantStatus {
				t.Errorf("Expected status %d, got %d. Body: %s", tt.wantStatus, w.Code, w.Body.String())
			}
		})
	}
}
