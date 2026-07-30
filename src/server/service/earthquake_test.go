package service

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/patrickmn/go-cache"
)

// newTestEarthquakeService builds an EarthquakeService whose usgsAPIURL points
// at the given httptest server base, mirroring newTestWeatherService's pattern
// of overriding the base URL field directly rather than hitting the real API.
func newTestEarthquakeService(baseURL string) *EarthquakeService {
	return &EarthquakeService{
		client:     &http.Client{Timeout: 5 * time.Second},
		cache:      cache.New(1*time.Minute, 2*time.Minute),
		usgsAPIURL: baseURL,
	}
}

const usgsFixtureJSON = `{
  "type": "FeatureCollection",
  "metadata": {
    "generated": 1700000000000,
    "url": "https://example.com/feed",
    "title": "USGS Test Feed",
    "status": 200,
    "api": "1.10.3",
    "count": 2
  },
  "features": [
    {
      "type": "Feature",
      "properties": {
        "mag": 4.5,
        "place": "10km NE of Somewhere",
        "time": 1700000001000,
        "updated": 1700000002000,
        "tz": null,
        "url": "https://example.com/eq1",
        "detail": "https://example.com/eq1.geojson",
        "felt": 12,
        "cdi": 3.2,
        "mmi": 4.1,
        "alert": null,
        "status": "reviewed",
        "tsunami": 0,
        "sig": 300,
        "net": "us",
        "code": "abc123",
        "ids": ",us1,",
        "sources": ",us,",
        "types": ",origin,",
        "nst": 20,
        "dmin": 0.5,
        "rms": 0.6,
        "gap": 30,
        "magType": "mb",
        "type": "earthquake",
        "title": "M 4.5 - 10km NE of Somewhere"
      },
      "geometry": {
        "type": "Point",
        "coordinates": [-122.0, 37.0, 10.5]
      },
      "id": "us1"
    },
    {
      "type": "Feature",
      "properties": {
        "mag": 2.1,
        "place": "5km SW of Elsewhere",
        "time": 1700000010000,
        "updated": 1700000011000,
        "tz": null,
        "url": "https://example.com/eq2",
        "detail": "https://example.com/eq2.geojson",
        "felt": null,
        "cdi": null,
        "mmi": null,
        "alert": null,
        "status": "automatic",
        "tsunami": 0,
        "sig": 90,
        "net": "ci",
        "code": "def456",
        "ids": ",ci1,",
        "sources": ",ci,",
        "types": ",origin,",
        "nst": null,
        "dmin": null,
        "rms": 0.3,
        "gap": null,
        "magType": "ml",
        "type": "earthquake",
        "title": "M 2.1 - 5km SW of Elsewhere"
      },
      "geometry": {
        "type": "Point",
        "coordinates": [-122.5, 37.5, 3.2]
      },
      "id": "ci1"
    }
  ],
  "bbox": [-122.5, 37.0, 3.2, -122.0, 37.5, 10.5]
}`

// TestEarthquake_GetEarthquakes_HappyPath covers a successful fetch/parse,
// verifying that USGS GeoJSON fields are correctly mapped into Earthquake
// structs (including the [lon, lat, depth] geometry ordering) and that
// Metadata is copied through.
func TestEarthquake_GetEarthquakes_HappyPath(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/all_day.geojson" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(usgsFixtureJSON))
	}))
	defer server.Close()

	es := newTestEarthquakeService(server.URL)
	got, err := es.GetEarthquakes("all_day")
	if err != nil {
		t.Fatalf("GetEarthquakes() error = %v", err)
	}
	if len(got.Earthquakes) != 2 {
		t.Fatalf("len(Earthquakes) = %d, want 2", len(got.Earthquakes))
	}

	eq := got.Earthquakes[0]
	if eq.ID != "us1" {
		t.Errorf("ID = %q, want us1", eq.ID)
	}
	if eq.Magnitude != 4.5 {
		t.Errorf("Magnitude = %v, want 4.5", eq.Magnitude)
	}
	if eq.Longitude != -122.0 || eq.Latitude != 37.0 || eq.Depth != 10.5 {
		t.Errorf("coords = (%v,%v,%v), want (-122.0,37.0,10.5)", eq.Longitude, eq.Latitude, eq.Depth)
	}
	if eq.Felt == nil || *eq.Felt != 12 {
		t.Errorf("Felt = %v, want 12", eq.Felt)
	}
	if got.Metadata.Title != "USGS Test Feed" || got.Metadata.Count != 2 {
		t.Errorf("Metadata = %+v, unexpected", got.Metadata)
	}

	eq2 := got.Earthquakes[1]
	if eq2.Felt != nil {
		t.Errorf("Felt = %v, want nil for missing felt report", eq2.Felt)
	}
}

// TestEarthquake_GetEarthquakes_CacheHit ensures a second call for the same
// feedType does not re-hit the network (idempotent read, cache honored).
func TestEarthquake_GetEarthquakes_CacheHit(t *testing.T) {
	var hits int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(usgsFixtureJSON))
	}))
	defer server.Close()

	es := newTestEarthquakeService(server.URL)
	if _, err := es.GetEarthquakes("all_day"); err != nil {
		t.Fatalf("first GetEarthquakes() error = %v", err)
	}
	if _, err := es.GetEarthquakes("all_day"); err != nil {
		t.Fatalf("second GetEarthquakes() error = %v", err)
	}
	if hits != 1 {
		t.Errorf("server hits = %d, want 1 (second call should be served from cache)", hits)
	}

	// A different feedType is a different cache key and must still hit the network.
	if _, err := es.GetEarthquakes("all_week"); err != nil {
		t.Fatalf("GetEarthquakes(all_week) error = %v", err)
	}
	if hits != 2 {
		t.Errorf("server hits = %d, want 2 after distinct feedType", hits)
	}
}

// TestEarthquake_GetEarthquakes_Errors covers HTTP error status and malformed
// JSON boundary/error paths.
func TestEarthquake_GetEarthquakes_Errors(t *testing.T) {
	tests := []struct {
		name       string
		statusCode int
		body       string
	}{
		{"non-200 status", http.StatusInternalServerError, `{}`},
		{"malformed json", http.StatusOK, `{not valid json`},
		{"empty body", http.StatusOK, ``},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tt.statusCode)
				_, _ = w.Write([]byte(tt.body))
			}))
			defer server.Close()

			es := newTestEarthquakeService(server.URL)
			// Use a unique feed type per subtest so the shared cache never masks a real request.
			_, err := es.GetEarthquakes(fmt.Sprintf("feed_%s", tt.name))
			if err == nil {
				t.Fatal("expected error, got nil")
			}
		})
	}
}

// TestEarthquake_GetEarthquakes_EmptyFeatures covers the boundary case of a
// well-formed feed with zero features.
func TestEarthquake_GetEarthquakes_EmptyFeatures(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"type":"FeatureCollection","metadata":{"generated":1,"url":"u","title":"t","status":200,"api":"1","count":0},"features":[],"bbox":[]}`))
	}))
	defer server.Close()

	es := newTestEarthquakeService(server.URL)
	got, err := es.GetEarthquakes("all_hour")
	if err != nil {
		t.Fatalf("GetEarthquakes() error = %v", err)
	}
	if len(got.Earthquakes) != 0 {
		t.Errorf("len(Earthquakes) = %d, want 0", len(got.Earthquakes))
	}
}

// TestEarthquake_GetEarthquakesByLocation covers radius filtering and
// distance calculation, including the zero-radius boundary (nothing matches
// unless exactly at the origin) and a radius large enough to include both
// fixture earthquakes.
func TestEarthquake_GetEarthquakesByLocation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(usgsFixtureJSON))
	}))
	defer server.Close()

	tests := []struct {
		name      string
		lat, lon  float64
		radiusKm  float64
		feedType  string
		wantCount int
	}{
		// Origin far from both fixture quakes (roughly the South Pole),
		// so a zero radius must exclude everything.
		{"zero radius excludes all", -90, 0, 0, "loc_zero", 0},
		// Origin exactly at the first fixture quake (37.0, -122.0): a zero
		// radius still includes an earthquake at distance exactly zero.
		{"zero radius includes exact match", 37.0, -122.0, 0, "loc_exact", 1},
		{"huge radius includes all", 37.0, -122.0, 20000, "loc_huge", 2},
		{"tight radius includes only near quake", 37.0, -122.0, 60, "loc_tight", 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			es := newTestEarthquakeService(server.URL)
			got, err := es.GetEarthquakesByLocation(tt.lat, tt.lon, tt.radiusKm, tt.feedType)
			if err != nil {
				t.Fatalf("GetEarthquakesByLocation() error = %v", err)
			}
			if len(got.Earthquakes) != tt.wantCount {
				t.Errorf("len(Earthquakes) = %d, want %d", len(got.Earthquakes), tt.wantCount)
			}
			for _, eq := range got.Earthquakes {
				if eq.DistanceFmt == "" {
					t.Error("expected DistanceFmt to be populated for filtered earthquake")
				}
			}
		})
	}
}

// TestEarthquake_GetEarthquakesByLocation_PropagatesFetchError covers the
// error path where the underlying GetEarthquakes call fails.
func TestEarthquake_GetEarthquakesByLocation_PropagatesFetchError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()

	es := newTestEarthquakeService(server.URL)
	if _, err := es.GetEarthquakesByLocation(0, 0, 100, "loc_error"); err == nil {
		t.Fatal("expected error to propagate from GetEarthquakes")
	}
}

// TestEarthquake_FormatDistanceKm covers the three formatting boundaries:
// sub-kilometer (meters), sub-10km (one decimal), and >=10km (whole number).
func TestEarthquake_FormatDistanceKm(t *testing.T) {
	tests := []struct {
		name string
		km   float64
		want string
	}{
		{"zero", 0, "0 m"},
		{"sub-km in meters", 0.25, "250 m"},
		{"just under 1km", 0.999, "999 m"},
		{"exactly 1km", 1, "1.0 km"},
		{"mid range one decimal", 5.4, "5.4 km"},
		{"just under 10km", 9.999, "10.0 km"},
		{"exactly 10km", 10, "10 km"},
		{"large distance", 1234.5, "1234 km"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := formatDistanceKm(tt.km); got != tt.want {
				t.Errorf("formatDistanceKm(%v) = %q, want %q", tt.km, got, tt.want)
			}
		})
	}
}

// TestEarthquake_SortAndLimit_Magnitude covers descending magnitude sort.
func TestEarthquake_SortAndLimit_Magnitude(t *testing.T) {
	ec := &EarthquakeCollection{Earthquakes: []Earthquake{
		{ID: "a", Magnitude: 2.0},
		{ID: "b", Magnitude: 5.0},
		{ID: "c", Magnitude: 3.5},
	}}
	ec.SortAndLimit("magnitude", 0)
	want := []string{"b", "c", "a"}
	for i, id := range want {
		if ec.Earthquakes[i].ID != id {
			t.Errorf("position %d = %q, want %q", i, ec.Earthquakes[i].ID, id)
		}
	}
}

// TestEarthquake_SortAndLimit_Oldest covers ascending time sort.
func TestEarthquake_SortAndLimit_Oldest(t *testing.T) {
	base := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	ec := &EarthquakeCollection{Earthquakes: []Earthquake{
		{ID: "newest", Time: base.Add(2 * time.Hour)},
		{ID: "oldest", Time: base},
		{ID: "middle", Time: base.Add(1 * time.Hour)},
	}}
	ec.SortAndLimit("oldest", 0)
	want := []string{"oldest", "middle", "newest"}
	for i, id := range want {
		if ec.Earthquakes[i].ID != id {
			t.Errorf("position %d = %q, want %q", i, ec.Earthquakes[i].ID, id)
		}
	}
}

// TestEarthquake_SortAndLimit_Depth covers descending depth sort.
func TestEarthquake_SortAndLimit_Depth(t *testing.T) {
	ec := &EarthquakeCollection{Earthquakes: []Earthquake{
		{ID: "shallow", Depth: 1.0},
		{ID: "deep", Depth: 50.0},
		{ID: "mid", Depth: 10.0},
	}}
	ec.SortAndLimit("depth", 0)
	want := []string{"deep", "mid", "shallow"}
	for i, id := range want {
		if ec.Earthquakes[i].ID != id {
			t.Errorf("position %d = %q, want %q", i, ec.Earthquakes[i].ID, id)
		}
	}
}

// TestEarthquake_SortAndLimit_Closest covers ascending distance sort.
func TestEarthquake_SortAndLimit_Closest(t *testing.T) {
	ec := &EarthquakeCollection{Earthquakes: []Earthquake{
		{ID: "far", Distance: 100.0},
		{ID: "near", Distance: 1.0},
		{ID: "mid", Distance: 10.0},
	}}
	ec.SortAndLimit("closest", 0)
	want := []string{"near", "mid", "far"}
	for i, id := range want {
		if ec.Earthquakes[i].ID != id {
			t.Errorf("position %d = %q, want %q", i, ec.Earthquakes[i].ID, id)
		}
	}
}

// TestEarthquake_SortAndLimit_DefaultUnknown covers the default branch
// ("newest" or any unrecognized value): order must be left untouched.
func TestEarthquake_SortAndLimit_DefaultUnknown(t *testing.T) {
	tests := []string{"newest", "", "bogus-sort-mode"}
	for _, sortBy := range tests {
		t.Run(sortBy, func(t *testing.T) {
			ec := &EarthquakeCollection{Earthquakes: []Earthquake{
				{ID: "first"},
				{ID: "second"},
				{ID: "third"},
			}}
			ec.SortAndLimit(sortBy, 0)
			want := []string{"first", "second", "third"}
			for i, id := range want {
				if ec.Earthquakes[i].ID != id {
					t.Errorf("position %d = %q, want %q (order should be unchanged)", i, ec.Earthquakes[i].ID, id)
				}
			}
		})
	}
}

// TestEarthquake_SortAndLimit_Limit covers the limit boundary conditions:
// limit=0 is a no-op, limit greater than length is a no-op, and limit less
// than length truncates and updates Metadata.Count.
func TestEarthquake_SortAndLimit_Limit(t *testing.T) {
	newCollection := func() *EarthquakeCollection {
		return &EarthquakeCollection{
			Earthquakes: []Earthquake{{ID: "a"}, {ID: "b"}, {ID: "c"}},
			Metadata:    Metadata{Count: 3},
		}
	}

	t.Run("limit zero is no-op", func(t *testing.T) {
		ec := newCollection()
		ec.SortAndLimit("", 0)
		if len(ec.Earthquakes) != 3 {
			t.Errorf("len = %d, want 3", len(ec.Earthquakes))
		}
	})

	t.Run("negative limit is no-op", func(t *testing.T) {
		ec := newCollection()
		ec.SortAndLimit("", -1)
		if len(ec.Earthquakes) != 3 {
			t.Errorf("len = %d, want 3", len(ec.Earthquakes))
		}
	})

	t.Run("limit greater than length is no-op", func(t *testing.T) {
		ec := newCollection()
		ec.SortAndLimit("", 10)
		if len(ec.Earthquakes) != 3 {
			t.Errorf("len = %d, want 3", len(ec.Earthquakes))
		}
	})

	t.Run("limit less than length truncates and updates count", func(t *testing.T) {
		ec := newCollection()
		ec.SortAndLimit("", 2)
		if len(ec.Earthquakes) != 2 {
			t.Errorf("len = %d, want 2", len(ec.Earthquakes))
		}
		if ec.Metadata.Count != 2 {
			t.Errorf("Metadata.Count = %d, want 2", ec.Metadata.Count)
		}
	})

	t.Run("limit exactly at length is no-op", func(t *testing.T) {
		ec := newCollection()
		ec.SortAndLimit("", 3)
		if len(ec.Earthquakes) != 3 {
			t.Errorf("len = %d, want 3", len(ec.Earthquakes))
		}
		if ec.Metadata.Count != 3 {
			t.Errorf("Metadata.Count = %d, want unchanged 3", ec.Metadata.Count)
		}
	})

	t.Run("empty collection with positive limit is no-op", func(t *testing.T) {
		ec := &EarthquakeCollection{Earthquakes: []Earthquake{}}
		ec.SortAndLimit("magnitude", 5)
		if len(ec.Earthquakes) != 0 {
			t.Errorf("len = %d, want 0", len(ec.Earthquakes))
		}
	})
}
