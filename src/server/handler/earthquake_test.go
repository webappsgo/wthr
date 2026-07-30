package handler

import (
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/webappsgo/wthr/src/server/service"
)

func newEarthquakeTestContext(target string) (*gin.Context, *httptest.ResponseRecorder) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, target, nil)
	return c, w
}

// ListEarthquakes short-circuits with an error when the handler or its
// earthquake service is nil, so this is reachable without a live service.
func TestEarthquakeHandler_ListEarthquakes_NilService(t *testing.T) {
	h := NewEarthquakeHandler(nil, nil, nil)
	got, err := h.ListEarthquakes("all_day", nil, nil)
	if err == nil {
		t.Fatal("expected an error for nil earthquake service")
	}
	if got != nil {
		t.Fatalf("expected nil result, got %v", got)
	}
}

func TestEarthquakeHandler_ListEarthquakes_NilHandler(t *testing.T) {
	var h *EarthquakeHandler
	got, err := h.ListEarthquakes("all_day", nil, nil)
	if err == nil {
		t.Fatal("expected an error for nil handler")
	}
	if got != nil {
		t.Fatalf("expected nil result, got %v", got)
	}
}

// HandleEarthquakeByIDAPI validates the id path param before touching the
// (here nil) earthquakeService.
func TestEarthquakeHandler_HandleEarthquakeByIDAPI_MissingID(t *testing.T) {
	h := NewEarthquakeHandler(nil, nil, nil)
	c, w := newEarthquakeTestContext("/api/v1/earthquakes/")

	h.HandleEarthquakeByIDAPI(c)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d; body=%s", w.Code, http.StatusBadRequest, w.Body.String())
	}
}

// HandleEarthquakeDetail validates the id path param before touching the
// (here nil) earthquakeService.
func TestEarthquakeHandler_HandleEarthquakeDetail_MissingID(t *testing.T) {
	h := NewEarthquakeHandler(nil, nil, nil)
	c, w := newEarthquakeTestContext("/earthquake/detail/")

	h.HandleEarthquakeDetail(c)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d; body=%s", w.Code, http.StatusBadRequest, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "Earthquake ID required") {
		t.Errorf("body = %q, want it to mention the missing ID", w.Body.String())
	}
}

func TestHaversineDistanceCalc(t *testing.T) {
	tests := []struct {
		name                   string
		lat1, lon1, lat2, lon2 float64
		want                   float64
		tolerance              float64
	}{
		{"same point", 40.0, -74.0, 40.0, -74.0, 0, 0.001},
		// New York (40.7128, -74.0060) to Los Angeles (34.0522, -118.2437)
		// is approximately 3936 km great-circle distance.
		{"NY to LA", 40.7128, -74.0060, 34.0522, -118.2437, 3936, 20},
		// One degree of longitude at the equator is ~111.19 km.
		{"one degree longitude at equator", 0, 0, 0, 1, 111.19, 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := haversineDistanceCalc(tt.lat1, tt.lon1, tt.lat2, tt.lon2)
			if math.Abs(got-tt.want) > tt.tolerance {
				t.Errorf("haversineDistanceCalc(%v,%v,%v,%v) = %v, want ~%v (tolerance %v)",
					tt.lat1, tt.lon1, tt.lat2, tt.lon2, got, tt.want, tt.tolerance)
			}
		})
	}
}

func TestFormatEarthquakeDistance(t *testing.T) {
	tests := []struct {
		name string
		km   float64
		want string
	}{
		{"sub-kilometer in meters", 0.5, "500 m"},
		{"under ten km with decimal", 5.4, "5.4 km"},
		{"under hundred km rounded", 42.6, "43 km"},
		{"hundred plus km rounded", 3936.2, "3936 km"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatEarthquakeDistance(tt.km)
			if got != tt.want {
				t.Errorf("formatEarthquakeDistance(%v) = %q, want %q", tt.km, got, tt.want)
			}
		})
	}
}

func TestTruncateString(t *testing.T) {
	tests := []struct {
		name   string
		s      string
		maxLen int
		want   string
	}{
		{"shorter than max is unchanged", "short", 10, "short"},
		{"exact length is unchanged", "exact", 5, "exact"},
		{"longer is truncated with ellipsis", "this is a long place name", 10, "this is..."},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := truncateString(tt.s, tt.maxLen)
			if got != tt.want {
				t.Errorf("truncateString(%q, %d) = %q, want %q", tt.s, tt.maxLen, got, tt.want)
			}
		})
	}
}

func newTestEarthquake() service.Earthquake {
	return service.Earthquake{
		ID:            "us7000abcd",
		Magnitude:     5.4,
		Place:         "10km SW of Testville, CA",
		Time:          time.Date(2026, 1, 15, 12, 30, 0, 0, time.UTC),
		Latitude:      34.05,
		Longitude:     -118.24,
		Depth:         10.5,
		Type:          "earthquake",
		URL:           "https://earthquake.usgs.gov/earthquakes/eventpage/us7000abcd",
		Tsunami:       0,
		Status:        "reviewed",
		MagnitudeType: "mb",
		Network:       "us",
		UpdatedTime:   time.Date(2026, 1, 15, 13, 0, 0, 0, time.UTC),
	}
}

// renderASCIIEarthquakes and renderASCIIEarthquakeDetail are pure string
// builders taking only service structs, so they're fully testable without
// a live earthquake service.
func TestEarthquakeHandler_renderASCIIEarthquakes(t *testing.T) {
	h := NewEarthquakeHandler(nil, nil, nil)

	t.Run("empty collection", func(t *testing.T) {
		collection := &service.EarthquakeCollection{
			Earthquakes: nil,
			Metadata:    service.Metadata{Title: "Test Feed", Count: 0, Generated: time.Now().UnixMilli()},
		}
		out := h.renderASCIIEarthquakes(collection, "", "all_day")
		if !strings.Contains(out, "No earthquakes found") {
			t.Errorf("output = %q, want it to mention no earthquakes found", out)
		}
	})

	t.Run("with earthquakes and location", func(t *testing.T) {
		eq := newTestEarthquake()
		eq.Tsunami = 1
		collection := &service.EarthquakeCollection{
			Earthquakes: []service.Earthquake{eq},
			Metadata:    service.Metadata{Title: "Test Feed", Count: 1, Generated: time.Now().UnixMilli()},
		}
		out := h.renderASCIIEarthquakes(collection, "Testville", "all_day")
		if !strings.Contains(out, "Testville") {
			t.Errorf("output missing location name: %q", out)
		}
		if !strings.Contains(out, "Total:") {
			t.Errorf("output missing total count: %q", out)
		}
		if !strings.Contains(out, "🌊") {
			t.Errorf("output missing tsunami marker: %q", out)
		}
	})

	t.Run("without location uses metadata title", func(t *testing.T) {
		eq := newTestEarthquake()
		collection := &service.EarthquakeCollection{
			Earthquakes: []service.Earthquake{eq},
			Metadata:    service.Metadata{Title: "M4.5+ Earthquakes, Past Day", Count: 1, Generated: time.Now().UnixMilli()},
		}
		out := h.renderASCIIEarthquakes(collection, "", "4.5_day")
		if !strings.Contains(out, "M4.5+ Earthquakes, Past Day") {
			t.Errorf("output missing metadata title: %q", out)
		}
	})
}

func TestEarthquakeHandler_renderASCIIEarthquakeDetail(t *testing.T) {
	h := NewEarthquakeHandler(nil, nil, nil)

	felt := 120
	cdi := 6.2
	mmi := 5.8
	eq := newTestEarthquake()
	eq.Tsunami = 1
	eq.Felt = &felt
	eq.CDI = &cdi
	eq.MMI = &mmi

	out := h.renderASCIIEarthquakeDetail(&eq)

	for _, want := range []string{
		"Earthquake Details",
		eq.Place,
		"TSUNAMI WARNING",
		eq.ID,
		eq.Network,
		"120 reports",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q; got %q", want, out)
		}
	}
}
