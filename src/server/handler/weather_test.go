package handler

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

// newWeatherTestContext builds a bare GET gin.Context/recorder pair, mirroring
// the newTestContext helper style already used in response_test.go, but kept
// local since this file must not redefine or depend on unexported helpers
// owned by another agent's file beyond what's in handler_helpers_test.go.
func newWeatherTestContext(target string) (*gin.Context, *httptest.ResponseRecorder) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, target, nil)
	return c, w
}

// HandleRoot/HandleLocation both gate on IsInitialized() before touching the
// (here nil) weatherService, so this is the only success/error pair
// reachable without a live WeatherService. IsInitialized/SetInitStatus are
// global mutable state shared by the whole handler package's test suite, so
// we always set an explicit value rather than relying on ambient state.
func TestWeatherHandler_HandleRoot_NotInitialized(t *testing.T) {
	t.Run("root returns loading page while services are not yet initialized", func(t *testing.T) {
		SetInitStatus(false, false, false)
		t.Cleanup(func() { SetInitStatus(false, false, false) })

		h := NewWeatherHandler(nil, nil)
		c, w := newWeatherTestContext("/")
		c.Request.Header.Set("Accept", "application/json")

		h.HandleRoot(c)

		if w.Code != http.StatusServiceUnavailable {
			t.Fatalf("status = %d, want %d; body=%s", w.Code, http.StatusServiceUnavailable, w.Body.String())
		}
		if !strings.Contains(w.Body.String(), "Initializing") {
			t.Errorf("body = %s, want it to mention Initializing", w.Body.String())
		}
	})

	t.Run("location route also gates on IsInitialized before touching weatherService", func(t *testing.T) {
		SetInitStatus(false, false, false)
		t.Cleanup(func() { SetInitStatus(false, false, false) })

		h := NewWeatherHandler(nil, nil)
		c, w := newWeatherTestContext("/London,GB")
		c.Request.Header.Set("Accept", "application/json")
		c.Params = gin.Params{{Key: "location", Value: "London,GB"}}

		h.HandleLocation(c)

		if w.Code != http.StatusServiceUnavailable {
			t.Fatalf("status = %d, want %d; body=%s", w.Code, http.StatusServiceUnavailable, w.Body.String())
		}
	})
}

// isGPSCoordinates is a pure parser with no service dependency, so it's
// tested directly across the boundary conditions the special-endpoint and
// invalid-path routing logic relies on.
func TestWeatherHandler_isGPSCoordinates(t *testing.T) {
	h := NewWeatherHandler(nil, nil)

	tests := []struct {
		name  string
		input string
		want  bool
	}{
		{"valid coordinates", "40.7128,-74.0060", true},
		{"valid coordinates with space", "40.7128, -74.0060", true},
		{"city name", "London,GB", false},
		{"single number only", "40.7128", false},
		{"empty string", "", false},
		{"three comma-separated parts", "1,2,3", false},
		{"non-numeric parts", "abc,def", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := h.isGPSCoordinates(tt.input); got != tt.want {
				t.Errorf("isGPSCoordinates(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

// isInvalidPath drives 404 filtering for scanner/bot noise (wp-*, .php,
// favicon.ico, etc.) while explicitly not rejecting GPS-coordinate paths
// that happen to contain a decimal point.
func TestWeatherHandler_isInvalidPath(t *testing.T) {
	h := NewWeatherHandler(nil, nil)

	tests := []struct {
		name  string
		input string
		want  bool
	}{
		{"empty path is valid", "", false},
		{"plain city is valid", "London,GB", false},
		{"wp scanner path is invalid", "wp-admin/setup.php", true},
		{"admin path is invalid", "admin/config", true},
		{"favicon is invalid", "favicon.ico", true},
		{"php extension is invalid", "index.php", true},
		{"json extension is invalid", "data.json", true},
		{"GPS coordinates are never invalid despite the decimal point", "40.7128,-74.0060", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := h.isInvalidPath(tt.input); got != tt.want {
				t.Errorf("isInvalidPath(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

// formatPopulation is a pure formatter; 0 is the documented empty-string
// sentinel for "unknown population" and is the most important boundary.
func TestFormatPopulation(t *testing.T) {
	tests := []struct {
		name string
		pop  int
		want string
	}{
		{"zero returns empty string", 0, ""},
		{"small number has no commas", 42, "42"},
		{"three digits has no commas", 999, "999"},
		{"four digits gets one comma", 1234, "1,234"},
		{"millions gets two commas", 8175133, "8,175,133"},
		{"negative number", -5, "-5"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := formatPopulation(tt.pop); got != tt.want {
				t.Errorf("formatPopulation(%d) = %q, want %q", tt.pop, got, tt.want)
			}
		})
	}
}

// handleSpecialEndpoints routes :help and :bash.function to plain-text
// handlers with zero external dependencies, so these run the real code path
// end to end rather than just checking the boolean return.
func TestWeatherHandler_handleSpecialEndpoints(t *testing.T) {
	h := NewWeatherHandler(nil, nil)

	t.Run(":help is handled and returns usage text", func(t *testing.T) {
		c, w := newWeatherTestContext("/:help")
		handled := h.handleSpecialEndpoints(c, ":help")
		if !handled {
			t.Fatalf("handleSpecialEndpoints(:help) = false, want true")
		}
		if w.Code != http.StatusOK {
			t.Errorf("status = %d, want 200", w.Code)
		}
		if !strings.Contains(w.Body.String(), "USAGE:") {
			t.Errorf("body missing USAGE section: %s", w.Body.String())
		}
	})

	t.Run(":bash.function is handled and returns a shell function", func(t *testing.T) {
		c, w := newWeatherTestContext("/:bash.function")
		handled := h.handleSpecialEndpoints(c, ":bash.function")
		if !handled {
			t.Fatalf("handleSpecialEndpoints(:bash.function) = false, want true")
		}
		if w.Code != http.StatusOK {
			t.Errorf("status = %d, want 200", w.Code)
		}
		if !strings.Contains(w.Body.String(), "wttr()") {
			t.Errorf("body missing wttr() function: %s", w.Body.String())
		}
	})

	t.Run("unrecognized path is not handled", func(t *testing.T) {
		c, _ := newWeatherTestContext("/London,GB")
		if h.handleSpecialEndpoints(c, "London,GB") {
			t.Errorf("handleSpecialEndpoints(London,GB) = true, want false")
		}
	})
}

// handleError branches on substrings of the underlying error message to
// choose an HTTP status and a tailored plain-text hint. This exercises every
// branch for the non-browser (console/curl) path, which has no template
// dependency.
func TestWeatherHandler_handleError(t *testing.T) {
	h := NewWeatherHandler(nil, nil)

	tests := []struct {
		name       string
		err        error
		wantStatus int
		wantSubstr string
	}{
		{"not found error maps to 404", errString("location not found"), http.StatusNotFound, "Location not found"},
		{"timeout error maps to 500 with timeout hint", errString("upstream timeout"), http.StatusInternalServerError, "Request timeout"},
		{"generic error maps to 500 with generic hint", errString("boom"), http.StatusInternalServerError, "Weather service error"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, w := newWeatherTestContext("/Nowhere")
			h.handleError(c, tt.err, "Nowhere", false)
			if w.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d; body=%s", w.Code, tt.wantStatus, w.Body.String())
			}
			if !strings.Contains(w.Body.String(), tt.wantSubstr) {
				t.Errorf("body = %q, want it to contain %q", w.Body.String(), tt.wantSubstr)
			}
		})
	}
}

// handleMoonRequest's non-browser branch renders plain text with no
// external dependencies; this is the only piece of moon handling reachable
// without a browser-detecting User-Agent and real templates.
func TestWeatherHandler_handleMoonRequest_NonBrowser(t *testing.T) {
	h := NewWeatherHandler(nil, nil)
	c, w := newWeatherTestContext("/moon")
	// No browser-like Accept/User-Agent header, so utils.IsBrowser(c) is false.

	h.handleMoonRequest(c, "moon")

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "Moon Phase Feature") {
		t.Errorf("body missing moon phase banner: %s", w.Body.String())
	}
}

// errString is a minimal error type so table-driven tests above can build
// arbitrary error messages without importing errors.New per case inline.
type errString string

func (e errString) Error() string { return string(e) }
