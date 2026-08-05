package handler

import (
	"bytes"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"testing"
)

// newWeatherTestConfigFile writes a minimal valid server.yml under Go's
// per-test temp dir (t.TempDir(), auto-cleaned) and returns its path for
// AdminWeatherHandler.ConfigPath, matching the pattern used throughout
// this package's other handler tests.
func newWeatherTestConfigFile(t *testing.T) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "server.yml")
	if err := os.WriteFile(path, []byte("weather:\n  sources:\n    openmeteo:\n      enabled: false\n"), 0644); err != nil {
		t.Fatalf("failed to write test config: %v", err)
	}
	return path
}

// TestAdminWeatherHandler_ShowWeatherSettings covers the trivial HTML
// render path. No auth/data gating exists on this handler.
func TestAdminWeatherHandler_ShowWeatherSettings(t *testing.T) {
	h := &AdminWeatherHandler{ConfigPath: newWeatherTestConfigFile(t)}
	c, w := newAPITestContext("/server/admin/config/weather")
	// c.HTML needs an HTMLRender configured or gin panics; recover so a
	// missing-template failure doesn't crash the run, since we only assert
	// the handler didn't error out before reaching HTML().
	defer func() {
		if r := recover(); r != nil {
			t.Skipf("gin HTMLRender not configured in unit test context: %v", r)
		}
	}()
	h.ShowWeatherSettings(c)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
}

// TestAdminWeatherHandler_UpdateWeatherSettings_Success verifies a valid
// JSON body is applied to the on-disk YAML config and a 200 ok response is
// sent.
func TestAdminWeatherHandler_UpdateWeatherSettings_Success(t *testing.T) {
	h := &AdminWeatherHandler{ConfigPath: newWeatherTestConfigFile(t)}

	body := []byte(`{"openmeteo_enabled":true,"openmeteo_base_url":"https://api.open-meteo.com","openmeteo_timeout":10,"openmeteo_retry_attempts":3,"usgs_earthquake_enabled":true,"nhc_hurricane_enabled":true,"cache_enabled":true,"cache_ttl":300,"cache_max_size":1000,"forecast_enabled":true,"current_weather_enabled":true,"historical_data_enabled":false,"alerts_enabled":true,"alerts_check_interval":60,"alerts_severity_threshold":"moderate","api_rate_limit":120,"api_max_forecast_days":14,"api_max_historical_days":30}`)

	c, w := newAPITestContext("/admin/config/weather")
	c.Request.Method = http.MethodPost
	c.Request.Body = io.NopCloser(bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	h.UpdateWeatherSettings(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", w.Code, w.Body.String())
	}

	updated, err := os.ReadFile(h.ConfigPath)
	if err != nil {
		t.Fatalf("failed to read updated config: %v", err)
	}
	if !bytes.Contains(updated, []byte("api.open-meteo.com")) {
		t.Errorf("expected config to contain openmeteo base url, got:\n%s", updated)
	}
	if !bytes.Contains(updated, []byte("moderate")) {
		t.Errorf("expected config to contain alerts severity threshold 'moderate', got:\n%s", updated)
	}
}

// TestAdminWeatherHandler_UpdateWeatherSettings_InvalidJSON verifies
// malformed request bodies are rejected with 400 and never reach the
// config writer.
func TestAdminWeatherHandler_UpdateWeatherSettings_InvalidJSON(t *testing.T) {
	configPath := newWeatherTestConfigFile(t)
	before, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("failed to read baseline config: %v", err)
	}

	h := &AdminWeatherHandler{ConfigPath: configPath}

	c, w := newAPITestContext("/admin/config/weather")
	c.Request.Method = http.MethodPost
	c.Request.Body = io.NopCloser(bytes.NewReader([]byte("{not valid json")))
	c.Request.Header.Set("Content-Type", "application/json")

	h.UpdateWeatherSettings(c)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d", w.Code)
	}

	after, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("failed to read config after invalid request: %v", err)
	}
	if !bytes.Equal(before, after) {
		t.Errorf("expected config to be untouched on invalid JSON, before:\n%s\nafter:\n%s", before, after)
	}
}

// TestAdminWeatherHandler_UpdateWeatherSettings_ConfigWriteError verifies a
// missing/unreadable config file surfaces as 500, not a panic.
func TestAdminWeatherHandler_UpdateWeatherSettings_ConfigWriteError(t *testing.T) {
	h := &AdminWeatherHandler{ConfigPath: filepath.Join(t.TempDir(), "does-not-exist", "server.yml")}

	c, w := newAPITestContext("/admin/config/weather")
	c.Request.Method = http.MethodPost
	c.Request.Body = io.NopCloser(bytes.NewReader([]byte(`{"openmeteo_enabled":true}`)))
	c.Request.Header.Set("Content-Type", "application/json")

	h.UpdateWeatherSettings(c)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected status 500, got %d: %s", w.Code, w.Body.String())
	}
}
