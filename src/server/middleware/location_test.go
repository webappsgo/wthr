package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

// TestSaveLocationCookies_RoundTripsThroughGetLocationFromCookies verifies
// that cookies set by SaveLocationCookies can be read back correctly by
// GetLocationFromCookies, simulating a real browser round trip (set on one
// response, sent back on the next request).
func TestSaveLocationCookies_RoundTripsThroughGetLocationFromCookies(t *testing.T) {
	gin.SetMode(gin.TestMode)

	router := gin.New()
	router.GET("/set", func(c *gin.Context) {
		SaveLocationCookies(c, 51.5074, -0.1278, "London")
		c.String(http.StatusOK, "ok")
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/set", nil)
	router.ServeHTTP(w, req)

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
	c2, _ := gin.CreateTestContext(httptest.NewRecorder())
	c2.Request = req2

	lat, lon, name, found := GetLocationFromCookies(c2)
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

// TestGetLocationFromCookies_NotFoundWhenMissing verifies the zero-value,
// found=false contract when no location cookies are present at all.
func TestGetLocationFromCookies_NotFoundWhenMissing(t *testing.T) {
	gin.SetMode(gin.TestMode)

	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)

	lat, lon, name, found := GetLocationFromCookies(c)
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
	gin.SetMode(gin.TestMode)

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
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			for k, v := range tt.cookies {
				req.AddCookie(&http.Cookie{Name: k, Value: v})
			}
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = req

			_, _, _, found := GetLocationFromCookies(c)
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
	gin.SetMode(gin.TestMode)

	router := gin.New()
	router.GET("/clear", func(c *gin.Context) {
		ClearLocationCookies(c)
		c.String(http.StatusOK, "ok")
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/clear", nil)
	router.ServeHTTP(w, req)

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
