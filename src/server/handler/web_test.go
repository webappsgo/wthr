package handler

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

// newWebTestContext builds a bare GET gin.Context/recorder pair, matching
// the newWeatherTestContext convention already established in
// weather_test.go for this package's not-initialized guard-clause tests.
func newWebTestContext(target string) (*gin.Context, *httptest.ResponseRecorder) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, target, nil)
	return c, w
}

// ServeWebInterface and ServeMoonInterface both gate on IsInitialized()
// before touching the (here nil) weatherService/locationEnhancer, so the
// not-initialized loading page is the only reachable branch without a live
// WeatherService — the same constraint documented in weather_test.go.
// ServeExamplesPage always calls c.HTML directly with no guard, so its
// success path is blocked by the lack of a wired HTMLRender in this unit
// test environment and is intentionally left untested (see final report).
func TestWebHandlerServeWebInterface_NotInitialized(t *testing.T) {
	SetInitStatus(false, false, false)
	t.Cleanup(func() { SetInitStatus(false, false, false) })

	h := NewWebHandler(nil, nil)
	c, w := newWebTestContext("/")
	c.Request.Header.Set("Accept", "application/json")

	h.ServeWebInterface(c)

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d; body=%s", w.Code, http.StatusServiceUnavailable, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "Initializing") {
		t.Errorf("body = %s, want it to mention Initializing", w.Body.String())
	}
}

func TestWebHandlerServeMoonInterface_NotInitialized(t *testing.T) {
	SetInitStatus(false, false, false)
	t.Cleanup(func() { SetInitStatus(false, false, false) })

	h := NewWebHandler(nil, nil)
	c, w := newWebTestContext("/moon")
	c.Request.Header.Set("Accept", "application/json")

	h.ServeMoonInterface(c)

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d; body=%s", w.Code, http.StatusServiceUnavailable, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "Initializing") {
		t.Errorf("body = %s, want it to mention Initializing", w.Body.String())
	}
}

// TestCalculateSunTimesForWeb exercises the pure solar-time computation at a
// handful of representative latitudes, including the polar day/night
// clamping branches (cosHa > 1 and cosHa < -1) that are otherwise
// unreachable from any handler-level test.
func TestCalculateSunTimesForWeb(t *testing.T) {
	fixedDate := time.Date(2024, time.June, 21, 0, 0, 0, 0, time.UTC)

	t.Run("mid-latitude location returns formatted sun times", func(t *testing.T) {
		got := calculateSunTimesForWeb(40.7128, -74.0060, fixedDate)

		for _, key := range []string{"SunriseFormatted", "SunsetFormatted", "SolarNoonFormatted", "DayLengthFormatted"} {
			v, ok := got[key].(string)
			if !ok || v == "" {
				t.Errorf("expected non-empty string for %s, got %v", key, got[key])
			}
		}
	})

	t.Run("north pole in summer returns midnight sun", func(t *testing.T) {
		got := calculateSunTimesForWeb(89.9, 0, fixedDate)

		if got["SunriseFormatted"] != "Midnight sun" {
			t.Errorf("SunriseFormatted = %v, want %q", got["SunriseFormatted"], "Midnight sun")
		}
		if got["DayLengthFormatted"] != "24h 0m" {
			t.Errorf("DayLengthFormatted = %v, want %q", got["DayLengthFormatted"], "24h 0m")
		}
	})

	t.Run("south pole in northern summer returns no sunrise", func(t *testing.T) {
		got := calculateSunTimesForWeb(-89.9, 0, fixedDate)

		if got["SunriseFormatted"] != "No sunrise" {
			t.Errorf("SunriseFormatted = %v, want %q", got["SunriseFormatted"], "No sunrise")
		}
		if got["DayLengthFormatted"] != "0h 0m" {
			t.Errorf("DayLengthFormatted = %v, want %q", got["DayLengthFormatted"], "0h 0m")
		}
	})

	t.Run("leap year day-of-year branch is exercised", func(t *testing.T) {
		leapDate := time.Date(2024, time.December, 31, 0, 0, 0, 0, time.UTC)
		got := calculateSunTimesForWeb(51.5074, -0.1278, leapDate)

		if _, ok := got["SunriseFormatted"].(string); !ok {
			t.Errorf("expected SunriseFormatted to be a string, got %v", got["SunriseFormatted"])
		}
	})
}

func TestFormatTimeToAMPM(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"midnight", "0:00", "12:00 AM"},
		{"noon", "12:00", "12:00 PM"},
		{"morning", "9:05", "9:05 AM"},
		{"evening", "21:30", "9:30 PM"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := formatTimeToAMPM(tt.input); got != tt.want {
				t.Errorf("formatTimeToAMPM(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestFormatHourMinToAMPM(t *testing.T) {
	tests := []struct {
		name string
		hour int
		min  int
		want string
	}{
		{"midnight hour zero", 0, 0, "12:00 AM"},
		{"noon hour twelve", 12, 0, "12:00 PM"},
		{"afternoon", 15, 45, "3:45 PM"},
		{"minute below range clamps to zero", 5, -1, "5:00 AM"},
		{"minute above range clamps to fifty-nine", 5, 100, "5:59 AM"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := formatHourMinToAMPM(tt.hour, tt.min); got != tt.want {
				t.Errorf("formatHourMinToAMPM(%d, %d) = %q, want %q", tt.hour, tt.min, got, tt.want)
			}
		})
	}
}

func TestFormatDistance(t *testing.T) {
	if got := formatDistance(384400.4); got != "384400 km" {
		t.Errorf("formatDistance(384400.4) = %q, want %q", got, "384400 km")
	}
}

// TestMathHelperWrappers exercises the thin math.* wrapper functions used by
// calculateSunTimesForWeb, since table-driven coverage of the caller alone
// does not guarantee every wrapper is invoked with a distinguishing input.
func TestMathHelperWrappers(t *testing.T) {
	if got := sinVal(0); got != 0 {
		t.Errorf("sinVal(0) = %v, want 0", got)
	}
	if got := cosVal(0); got != 1 {
		t.Errorf("cosVal(0) = %v, want 1", got)
	}
	if got := tanVal(0); got != 0 {
		t.Errorf("tanVal(0) = %v, want 0", got)
	}
	if got := acosVal(1); got != 0 {
		t.Errorf("acosVal(1) = %v, want 0", got)
	}
}
