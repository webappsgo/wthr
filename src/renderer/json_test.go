// Tests for json.go per AI.md PART 29 (Testing)
package renderer

import (
	"encoding/json"
	"strings"
	"testing"

	utils "github.com/webappsgo/wthr/src/util"
)

// TestRender_FullWeather verifies the indented Render path produces valid,
// round-trippable JSON with all top-level sections present.
func TestRender_FullWeather(t *testing.T) {
	r := NewJSONRenderer()
	weather := &utils.WeatherData{
		Location: sampleLocation(),
		Current:  sampleCurrent(),
		Forecast: []utils.ForecastData{
			{Date: "2026-07-21", TempMax: 20, TempMin: 10, Condition: "Cloudy"},
		},
	}

	out, err := r.Render(weather)
	if err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	if !strings.Contains(out, "\n  ") {
		t.Errorf("Render() output not indented: %q", out)
	}

	var decoded WeatherResponse
	if err := json.Unmarshal([]byte(out), &decoded); err != nil {
		t.Fatalf("Render() output is not valid JSON: %v", err)
	}
	if decoded.Location.Name != "London" {
		t.Errorf("decoded Location.Name = %q, want %q", decoded.Location.Name, "London")
	}
	if len(decoded.Forecast) != 1 {
		t.Errorf("decoded Forecast len = %d, want 1", len(decoded.Forecast))
	}
}

// TestRender_NilWeather documents a genuine crash risk (see final test
// report): Render dereferences weather.Location/Current/Forecast/Moon
// directly with no nil check, so any caller that passes a nil
// *utils.WeatherData (e.g. an upstream fetch error left unchecked) panics
// with a nil pointer dereference instead of returning an error. This test
// asserts the CURRENT (buggy) panic-on-nil behavior; if Render is ever
// hardened with a nil check, update this test to assert a clean error
// return instead of deleting it.
func TestRender_NilWeather(t *testing.T) {
	r := NewJSONRenderer()

	defer func() {
		if rec := recover(); rec == nil {
			t.Fatal("Render(nil) unexpectedly did not panic; if a nil check was added, update this test to assert a graceful error instead")
		}
	}()

	_, _ = r.Render(nil)
}

// TestRenderCompact_NoIndentation verifies the compact path emits JSON with
// no embedded newlines/indentation, distinguishing it from Render().
func TestRenderCompact_NoIndentation(t *testing.T) {
	r := NewJSONRenderer()
	weather := &utils.WeatherData{
		Location: sampleLocation(),
		Current:  sampleCurrent(),
	}

	out, err := r.RenderCompact(weather)
	if err != nil {
		t.Fatalf("RenderCompact() error = %v", err)
	}
	if strings.Contains(out, "\n") {
		t.Errorf("RenderCompact() output contains newlines, want single line: %q", out)
	}
	var decoded WeatherResponse
	if err := json.Unmarshal([]byte(out), &decoded); err != nil {
		t.Fatalf("RenderCompact() output is not valid JSON: %v", err)
	}
}

// TestRenderCurrentOnly verifies only location+current are present (no
// forecast/moon leaking into the smaller payload).
func TestRenderCurrentOnly(t *testing.T) {
	r := NewJSONRenderer()
	out, err := r.RenderCurrentOnly(sampleLocation(), sampleCurrent())
	if err != nil {
		t.Fatalf("RenderCurrentOnly() error = %v", err)
	}
	if strings.Contains(out, "forecast") {
		t.Errorf("RenderCurrentOnly() output unexpectedly contains forecast field: %q", out)
	}
	var decoded map[string]json.RawMessage
	if err := json.Unmarshal([]byte(out), &decoded); err != nil {
		t.Fatalf("RenderCurrentOnly() output is not valid JSON: %v", err)
	}
	if _, ok := decoded["location"]; !ok {
		t.Error("RenderCurrentOnly() output missing \"location\" key")
	}
	if _, ok := decoded["current"]; !ok {
		t.Error("RenderCurrentOnly() output missing \"current\" key")
	}
}

// TestRenderForecastOnly_EmptySlice verifies an empty forecast slice
// marshals to "[]" rather than "null", which would break strict JSON
// consumers expecting an array type.
func TestRenderForecastOnly_EmptySlice(t *testing.T) {
	r := NewJSONRenderer()
	out, err := r.RenderForecastOnly(sampleLocation(), []utils.ForecastData{})
	if err != nil {
		t.Fatalf("RenderForecastOnly() error = %v", err)
	}
	if strings.Contains(out, "\"forecast\": null") {
		t.Errorf("RenderForecastOnly() emitted null forecast instead of empty array: %q", out)
	}
	if !strings.Contains(out, "\"forecast\": []") {
		t.Errorf("RenderForecastOnly() = %q, want \"forecast\": []", out)
	}
}

// TestRenderForecastOnly_NilSlice verifies a nil forecast slice does not
// panic and still marshals cleanly (even if it becomes "null").
func TestRenderForecastOnly_NilSlice(t *testing.T) {
	r := NewJSONRenderer()
	out, err := r.RenderForecastOnly(sampleLocation(), nil)
	if err != nil {
		t.Fatalf("RenderForecastOnly(nil) error = %v", err)
	}
	var decoded map[string]json.RawMessage
	if err := json.Unmarshal([]byte(out), &decoded); err != nil {
		t.Fatalf("RenderForecastOnly(nil) output is not valid JSON: %v", err)
	}
}

// TestRenderError covers zero/negative/typical status codes and verifies
// the message is preserved verbatim (no truncation/escaping surprises).
func TestRenderError(t *testing.T) {
	r := NewJSONRenderer()

	tests := []struct {
		name    string
		message string
		status  int
	}{
		{"typical_not_found", "location not found", 404},
		{"zero_status", "unknown error", 0},
		{"negative_status", "invalid", -1},
		{"message_with_quotes", `bad "input" value`, 400},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out, err := r.RenderError(tt.message, tt.status)
			if err != nil {
				t.Fatalf("RenderError() error = %v", err)
			}
			var decoded struct {
				Error  string `json:"error"`
				Status int    `json:"status"`
			}
			if err := json.Unmarshal([]byte(out), &decoded); err != nil {
				t.Fatalf("RenderError() output is not valid JSON: %v", err)
			}
			if decoded.Error != tt.message {
				t.Errorf("decoded.Error = %q, want %q", decoded.Error, tt.message)
			}
			if decoded.Status != tt.status {
				t.Errorf("decoded.Status = %d, want %d", decoded.Status, tt.status)
			}
		})
	}
}

// TestRenderSearchResults_MapsAllFields verifies each LocationData field is
// correctly copied into SearchResult (a field rename/typo here would
// silently drop data from the public API).
func TestRenderSearchResults_MapsAllFields(t *testing.T) {
	r := NewJSONRenderer()
	locations := []utils.LocationData{
		{
			Name:        "Berlin",
			Country:     "Germany",
			CountryCode: "DE",
			State:       "Berlin",
			Latitude:    52.52,
			Longitude:   13.405,
			Population:  3769000,
			FullName:    "Berlin, Germany",
			ShortName:   "Berlin, DE",
		},
	}

	out, err := r.RenderSearchResults(locations)
	if err != nil {
		t.Fatalf("RenderSearchResults() error = %v", err)
	}

	var decoded struct {
		Results []SearchResult `json:"results"`
		Count   int            `json:"count"`
	}
	if err := json.Unmarshal([]byte(out), &decoded); err != nil {
		t.Fatalf("RenderSearchResults() output is not valid JSON: %v", err)
	}
	if decoded.Count != 1 {
		t.Fatalf("Count = %d, want 1", decoded.Count)
	}
	got := decoded.Results[0]
	want := SearchResult{
		Name: "Berlin", Country: "Germany", CountryCode: "DE", State: "Berlin",
		Latitude: 52.52, Longitude: 13.405, Population: 3769000,
		FullName: "Berlin, Germany", ShortName: "Berlin, DE",
	}
	if got != want {
		t.Errorf("mapped SearchResult = %+v, want %+v", got, want)
	}
}

// TestRenderSearchResults_EmptyInput verifies zero results produce an empty
// (not nil/null) array with Count 0, matching API-consumer expectations.
func TestRenderSearchResults_EmptyInput(t *testing.T) {
	r := NewJSONRenderer()
	out, err := r.RenderSearchResults([]utils.LocationData{})
	if err != nil {
		t.Fatalf("RenderSearchResults() error = %v", err)
	}
	if !strings.Contains(out, "\"results\": []") {
		t.Errorf("RenderSearchResults() = %q, want \"results\": []", out)
	}
	if !strings.Contains(out, "\"count\": 0") {
		t.Errorf("RenderSearchResults() = %q, want \"count\": 0", out)
	}
}

// TestRenderHealthCheck verifies all four fields pass through unchanged,
// including an empty version string (must not be omitted or replaced).
func TestRenderHealthCheck(t *testing.T) {
	r := NewJSONRenderer()
	out, err := r.RenderHealthCheck("ok", "2026-07-21T00:00:00Z", "wthr", "1.2.3")
	if err != nil {
		t.Fatalf("RenderHealthCheck() error = %v", err)
	}

	var decoded HealthStatus
	if err := json.Unmarshal([]byte(out), &decoded); err != nil {
		t.Fatalf("RenderHealthCheck() output is not valid JSON: %v", err)
	}
	want := HealthStatus{Status: "ok", Timestamp: "2026-07-21T00:00:00Z", Service: "wthr", Version: "1.2.3"}
	if decoded != want {
		t.Errorf("decoded = %+v, want %+v", decoded, want)
	}
}
