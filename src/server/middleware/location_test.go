package middleware

import (
	"math"
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestSaveLocationCookies_RoundTripsThroughGetLocationFromCookies verifies
// that cookies set by SaveLocationCookies can be read back correctly by
// GetLocationFromCookies, simulating a real browser round trip (set on one
// response, sent back on the next request).
func TestSaveLocationCookies_RoundTripsThroughGetLocationFromCookies(t *testing.T) {
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/set", nil)
	SaveLocationCookies(w, req, 51.5074, -0.1278, "London")

	resp := w.Result()
	cookies := resp.Cookies()
	if len(cookies) != 4 {
		t.Fatalf("got %d cookies, want 4 (lat, lon, name, set_at)", len(cookies))
	}

	// Simulate the next request carrying those cookies back.
	req2 := httptest.NewRequest(http.MethodGet, "/get", nil)
	for _, ck := range cookies {
		req2.AddCookie(ck)
	}

	lat, lon, name, found := GetLocationFromCookies(req2)
	if !found {
		t.Fatal("GetLocationFromCookies found = false, want true")
	}
	if lat != 51.5074 {
		t.Errorf("lat = %v, want 51.5074", lat)
	}
	if lon != -0.1278 {
		t.Errorf("lon = %v, want -0.1278", lon)
	}
	if name != "London" {
		t.Errorf("name = %q, want %q", name, "London")
	}
}

func TestSaveLocationCookiesSkipsGPCRequest(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/weather", nil)
	req.Header.Set("Sec-GPC", "1")
	rec := httptest.NewRecorder()
	SaveLocationCookies(rec, req, 51.5074, -0.1278, "London")
	if cookies := rec.Result().Cookies(); len(cookies) != 0 {
		t.Fatalf("SaveLocationCookies() emitted optional cookies for GPC request: %+v", cookies)
	}
}

func TestSaveLocationCookiesRejectsInvalidCoordinates(t *testing.T) {
	tests := []struct {
		name      string
		latitude  float64
		longitude float64
	}{
		{name: "latitude below range", latitude: -90.1, longitude: 0},
		{name: "latitude above range", latitude: 90.1, longitude: 0},
		{name: "longitude below range", latitude: 0, longitude: -180.1},
		{name: "longitude above range", latitude: 0, longitude: 180.1},
		{name: "non-finite latitude", latitude: math.NaN(), longitude: 0},
		{name: "non-finite longitude", latitude: 0, longitude: math.Inf(1)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			SaveLocationCookies(rec, httptest.NewRequest(http.MethodGet, "/weather", nil), tt.latitude, tt.longitude, "Somewhere")
			if cookies := rec.Result().Cookies(); len(cookies) != 0 {
				t.Fatalf("SaveLocationCookies() emitted cookies for invalid coordinates: %+v", cookies)
			}
		})
	}
}

// TestGetLocationFromCookies_NotFoundWhenMissing verifies the zero-value,
// found=false contract when no location cookies are present at all.
func TestGetLocationFromCookies_NotFoundWhenMissing(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)

	lat, lon, name, found := GetLocationFromCookies(req)
	if found {
		t.Error("found = true with no cookies present, want false")
	}
	if lat != 0 || lon != 0 || name != "" {
		t.Errorf("got (%v, %v, %q), want zero values when not found", lat, lon, name)
	}
}

// TestGetLocationFromCookies_PartialCookiesNotFound verifies that a partial
// cookie set (e.g. lat/lon present but name missing, or a corrupted
// non-numeric value) is treated as not-found rather than silently
// succeeding with a zero-valued field.
func TestGetLocationFromCookies_PartialOrCorruptCookies(t *testing.T) {
	tests := []struct {
		name    string
		cookies map[string]string
	}{
		{
			name: "missing location name cookie",
			cookies: map[string]string{
				"user_lat": "51.5074",
				"user_lon": "-0.1278",
			},
		},
		{
			name: "non-numeric latitude",
			cookies: map[string]string{
				"user_lat":           "not-a-number",
				"user_lon":           "-0.1278",
				"user_location_name": "London",
			},
		},
		{
			name: "non-numeric longitude",
			cookies: map[string]string{
				"user_lat":           "51.5074",
				"user_lon":           "not-a-number",
				"user_location_name": "London",
			},
		},
		{
			name: "out-of-range latitude",
			cookies: map[string]string{
				"user_lat":           "90.1",
				"user_lon":           "-0.1278",
				"user_location_name": "London",
			},
		},
		{
			name: "out-of-range longitude",
			cookies: map[string]string{
				"user_lat":           "51.5074",
				"user_lon":           "180.1",
				"user_location_name": "London",
			},
		},
		{
			name: "non-finite coordinate",
			cookies: map[string]string{
				"user_lat":           "NaN",
				"user_lon":           "-0.1278",
				"user_location_name": "London",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			for k, v := range tt.cookies {
				req.AddCookie(&http.Cookie{Name: k, Value: v})
			}

			_, _, _, found := GetLocationFromCookies(req)
			if found {
				t.Error("found = true for partial/corrupt cookie set, want false")
			}
		})
	}
}

// TestClearLocationCookies_SetsExpiredCookies verifies ClearLocationCookies
// emits deletion cookies (MaxAge -1, empty value) for all four cookie names,
// rather than merely omitting them.
func TestClearLocationCookies_SetsExpiredCookies(t *testing.T) {
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/clear", nil)
	ClearLocationCookies(w, req)

	cookies := w.Result().Cookies()
	wantNames := map[string]bool{
		"user_lat": false, "user_lon": false,
		"user_location_name": false, "user_location_set_at": false,
	}
	for _, ck := range cookies {
		if _, ok := wantNames[ck.Name]; ok {
			wantNames[ck.Name] = true
			if ck.MaxAge >= 0 {
				t.Errorf("cookie %s MaxAge = %d, want negative (expired)", ck.Name, ck.MaxAge)
			}
			if ck.Value != "" {
				t.Errorf("cookie %s Value = %q, want empty on clear", ck.Name, ck.Value)
			}
		}
	}
	for name, seen := range wantNames {
		if !seen {
			t.Errorf("ClearLocationCookies did not set a clearing cookie for %s", name)
		}
	}
}
