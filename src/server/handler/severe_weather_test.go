package handler

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/webappsgo/wthr/src/server/service"
)

func newSevereWeatherTestContext(target string) (*http.Request, *httptest.ResponseRecorder) {
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, target, nil)
	return req, w
}

// GetSevereWeatherData short-circuits with an error when the handler or its
// severe weather service is nil, before ever calling a live service.
func TestSevereWeatherHandler_GetSevereWeatherData_NilHandler(t *testing.T) {
	var h *SevereWeatherHandler
	data, err := h.GetSevereWeatherData("New York")
	if err == nil {
		t.Fatal("expected an error for nil handler")
	}
	if data != nil {
		t.Fatalf("expected nil data, got %v", data)
	}
}

func TestSevereWeatherHandler_GetSevereWeatherData_NilService(t *testing.T) {
	h := NewSevereWeatherHandler(nil, nil, nil)
	data, err := h.GetSevereWeatherData("New York")
	if err == nil {
		t.Fatal("expected an error for nil severe weather service")
	}
	if data != nil {
		t.Fatalf("expected nil data, got %v", data)
	}
}

// When a location is supplied but the weather service is nil, GetSevereWeatherData
// must fail before dereferencing the nil weatherService.
func TestSevereWeatherHandler_GetSevereWeatherData_NilWeatherService(t *testing.T) {
	h := NewSevereWeatherHandler(service.NewSevereWeatherService(), nil, nil)
	data, err := h.GetSevereWeatherData("Chicago")
	if err == nil {
		t.Fatal("expected an error for nil weather service when a location is given")
	}
	if data != nil {
		t.Fatalf("expected nil data, got %v", data)
	}
}

// HandleAlertByIDAPI validates the id path param before touching the (here
// nil) severeWeatherService.
func TestSevereWeatherHandler_HandleAlertByIDAPI_MissingID(t *testing.T) {
	h := NewSevereWeatherHandler(nil, nil, nil)
	r, w := newSevereWeatherTestContext("/api/v1/severe-weather/")

	h.HandleAlertByIDAPI(w, r)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d; body=%s", w.Code, http.StatusBadRequest, w.Body.String())
	}
}

func TestSevereWeatherHandler_renderConsoleOutput(t *testing.T) {
	h := NewSevereWeatherHandler(service.NewSevereWeatherService(), nil, nil)

	t.Run("no active alerts", func(t *testing.T) {
		data := &service.SevereWeatherData{}
		out := h.renderConsoleOutput(data, "")
		if !strings.Contains(out, "No active severe weather alerts") {
			t.Errorf("output = %q, want it to mention no active alerts", out)
		}
	})

	t.Run("with location and mixed alert types", func(t *testing.T) {
		data := &service.SevereWeatherData{
			Hurricanes: []service.Storm{
				{
					Name:           "Hurricane Test",
					Classification: "Hurricane",
					WindSpeed:      120,
					Pressure:       950,
					Latitude:       25.0,
					Longitude:      -80.0,
					DistanceMiles:  150,
					MovementSpeed:  10,
					MovementDir:    "NW",
				},
			},
			TornadoWarnings: []service.Alert{
				{ID: "t1", Event: "Tornado Warning", AreaDesc: "Test County", Severity: "Extreme", Urgency: "Immediate", Headline: "Take shelter now", Expires: "2026-01-15T18:00:00Z"},
			},
			SevereStorms: []service.Alert{
				{ID: "s1", Event: "Severe Thunderstorm Warning", AreaDesc: "Test County", Severity: "Severe", Urgency: "Expected"},
			},
			WinterStorms: []service.Alert{
				{ID: "w1", Event: "Winter Storm Warning", AreaDesc: "Test County", Severity: "Moderate", Urgency: "Expected"},
			},
			FloodWarnings: []service.Alert{
				{ID: "f1", Event: "Flood Warning", AreaDesc: "Test County", Severity: "Moderate", Urgency: "Expected"},
			},
			OtherAlerts: []service.Alert{
				{ID: "o1", Event: "Special Weather Statement", AreaDesc: "Test County", Severity: "Minor", Urgency: "Future"},
			},
		}
		out := h.renderConsoleOutput(data, "Testville")
		for _, want := range []string{
			"Location: Testville",
			"HURRICANES",
			"Hurricane Test",
			"TORNADO WARNINGS",
			"Tornado Warning",
			"SEVERE THUNDERSTORM WARNINGS",
			"WINTER STORM WARNINGS",
			"FLOOD WARNINGS",
			"OTHER ALERTS",
		} {
			if !strings.Contains(out, want) {
				t.Errorf("output missing %q", want)
			}
		}
	})
}

func TestSevereWeatherHandler_formatAlert(t *testing.T) {
	h := NewSevereWeatherHandler(service.NewSevereWeatherService(), nil, nil)

	t.Run("short headline is kept as-is", func(t *testing.T) {
		alert := service.Alert{
			Event:    "Tornado Warning",
			AreaDesc: "Test County",
			Severity: "Extreme",
			Urgency:  "Immediate",
			Headline: "Short headline",
			Expires:  "2026-01-15T18:00:00Z",
		}
		out := h.formatAlert(alert)
		if !strings.Contains(out, "Tornado Warning") {
			t.Errorf("output missing event: %q", out)
		}
		if !strings.Contains(out, "Short headline") {
			t.Errorf("output missing headline: %q", out)
		}
		if !strings.Contains(out, "Expires: 2026-01-15T18:00:00Z") {
			t.Errorf("output missing expires: %q", out)
		}
	})

	t.Run("long headline is truncated", func(t *testing.T) {
		longHeadline := strings.Repeat("a", 100)
		alert := service.Alert{
			Event:    "Severe Thunderstorm Warning",
			AreaDesc: "Test County",
			Severity: "Severe",
			Urgency:  "Expected",
			Headline: longHeadline,
		}
		out := h.formatAlert(alert)
		if strings.Contains(out, longHeadline) {
			t.Errorf("expected long headline to be truncated, got full headline in output")
		}
		if !strings.Contains(out, "...") {
			t.Errorf("expected truncation ellipsis in output: %q", out)
		}
	})

	t.Run("no expires omits the expires line", func(t *testing.T) {
		alert := service.Alert{
			Event:    "Flood Warning",
			AreaDesc: "Test County",
			Severity: "Moderate",
			Urgency:  "Expected",
		}
		out := h.formatAlert(alert)
		if strings.Contains(out, "Expires:") {
			t.Errorf("did not expect an Expires line when Expires is empty: %q", out)
		}
	})
}
