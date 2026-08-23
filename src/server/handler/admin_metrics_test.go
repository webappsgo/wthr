package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
)

// TestNewMetricsHandler verifies the constructor returns a non-nil,
// zero-value handler (MetricsHandler carries no fields/dependencies).
func TestNewMetricsHandler(t *testing.T) {
	h := NewMetricsHandler()
	if h == nil {
		t.Fatal("expected non-nil handler")
	}
}

// newMetricsAPIRequest builds a bare GET request/recorder pair.
func newMetricsAPIRequest(target string) (*http.Request, *httptest.ResponseRecorder) {
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, target, nil)
	return r, w
}

// newMetricsJSONRequest builds a request/recorder pair with a JSON body
// attached to the request, mirroring the pattern used elsewhere in this
// package for JSON-bound handlers.
func newMetricsJSONRequest(t *testing.T, method, target string, body interface{}) (*http.Request, *httptest.ResponseRecorder) {
	t.Helper()
	w := httptest.NewRecorder()

	var raw []byte
	switch v := body.(type) {
	case string:
		raw = []byte(v)
	case []byte:
		raw = v
	default:
		var err error
		raw, err = json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal request body: %v", err)
		}
	}

	r := httptest.NewRequest(method, target, bytes.NewReader(raw))
	r.Header.Set("Content-Type", "application/json")
	return r, w
}

// setMetricNameParam sets the chi :name route param on r.
func setMetricNameParam(r *http.Request, name string) *http.Request {
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("name", name)
	return r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, rctx))
}

// TestMetricsHandler_GetConfig verifies the static config payload is
// returned with the expected enabled flag and default path.
func TestMetricsHandler_GetConfig(t *testing.T) {
	h := NewMetricsHandler()
	r, w := newMetricsAPIRequest("/server/admin/config/metrics")
	h.GetConfig(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	var got MetricsConfig
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if !got.Enabled || got.Path != "/metrics" {
		t.Errorf("unexpected config: %+v", got)
	}
}

// TestMetricsHandler_UpdateConfig_Success verifies a valid config body is
// accepted and echoed back.
func TestMetricsHandler_UpdateConfig_Success(t *testing.T) {
	h := NewMetricsHandler()
	r, w := newMetricsJSONRequest(t, http.MethodPost, "/server/admin/config/metrics", MetricsConfig{
		Enabled: true,
		Path:    "/custom-metrics",
	})
	h.UpdateConfig(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	if !bytes.Contains(w.Body.Bytes(), []byte("custom-metrics")) {
		t.Errorf("expected response to echo custom path, got %s", w.Body.String())
	}
}

// TestMetricsHandler_UpdateConfig_EmptyPathDefaults verifies an empty
// path in the request body is normalized to the default "/metrics".
func TestMetricsHandler_UpdateConfig_EmptyPathDefaults(t *testing.T) {
	h := NewMetricsHandler()
	r, w := newMetricsJSONRequest(t, http.MethodPost, "/server/admin/config/metrics", MetricsConfig{
		Enabled: true,
		Path:    "",
	})
	h.UpdateConfig(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	var resp struct {
		Config MetricsConfig `json:"config"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if resp.Config.Path != "/metrics" {
		t.Errorf("expected default path /metrics, got %q", resp.Config.Path)
	}
}

// TestMetricsHandler_UpdateConfig_InvalidJSON verifies malformed request
// bodies are rejected with 400 rather than silently accepted.
func TestMetricsHandler_UpdateConfig_InvalidJSON(t *testing.T) {
	h := NewMetricsHandler()
	r, w := newMetricsJSONRequest(t, http.MethodPost, "/server/admin/config/metrics", "{not-json")

	h.UpdateConfig(w, r)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Code)
	}
}

// TestMetricsHandler_GetStats verifies the stats payload contains the
// expected keys.
func TestMetricsHandler_GetStats(t *testing.T) {
	h := NewMetricsHandler()
	r, w := newMetricsAPIRequest("/server/admin/config/metrics/stats")
	h.GetStats(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	var got map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	for _, key := range []string{"total", "enabled", "custom", "builtin"} {
		if _, ok := got[key]; !ok {
			t.Errorf("expected key %q in stats response", key)
		}
	}
}

// TestMetricsHandler_ListMetrics verifies the metrics list is non-empty
// and wrapped under the "metrics" key.
func TestMetricsHandler_ListMetrics(t *testing.T) {
	h := NewMetricsHandler()
	r, w := newMetricsAPIRequest("/server/admin/config/metrics/list")
	h.ListMetrics(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	var resp struct {
		Metrics []map[string]interface{} `json:"metrics"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if len(resp.Metrics) == 0 {
		t.Error("expected non-empty metrics list")
	}
}

// TestMetricsHandler_CreateMetric_Success verifies a well-formed custom
// metric is accepted with 201.
func TestMetricsHandler_CreateMetric_Success(t *testing.T) {
	h := NewMetricsHandler()
	r, w := newMetricsJSONRequest(t, http.MethodPost, "/server/admin/config/metrics/custom", CustomMetric{
		Name: "custom_widget_total",
		Type: "counter",
		Help: "Total widgets processed",
	})
	h.CreateMetric(w, r)

	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201", w.Code)
	}
}

// TestMetricsHandler_CreateMetric_InvalidJSON verifies malformed bodies
// are rejected.
func TestMetricsHandler_CreateMetric_InvalidJSON(t *testing.T) {
	h := NewMetricsHandler()
	r, w := newMetricsJSONRequest(t, http.MethodPost, "/server/admin/config/metrics/custom", "{not-json")

	h.CreateMetric(w, r)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Code)
	}
}

// TestMetricsHandler_CreateMetric_MissingName verifies an empty metric
// name is rejected with 400.
func TestMetricsHandler_CreateMetric_MissingName(t *testing.T) {
	h := NewMetricsHandler()
	r, w := newMetricsJSONRequest(t, http.MethodPost, "/server/admin/config/metrics/custom", CustomMetric{
		Type: "counter",
	})
	h.CreateMetric(w, r)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Code)
	}
}

// TestMetricsHandler_CreateMetric_MissingType verifies an empty metric
// type is rejected with 400.
func TestMetricsHandler_CreateMetric_MissingType(t *testing.T) {
	h := NewMetricsHandler()
	r, w := newMetricsJSONRequest(t, http.MethodPost, "/server/admin/config/metrics/custom", CustomMetric{
		Name: "custom_widget_total",
	})
	h.CreateMetric(w, r)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Code)
	}
}

// TestMetricsHandler_CreateMetric_InvalidType verifies a metric type
// outside the allowed set (counter/gauge/histogram/summary) is rejected.
func TestMetricsHandler_CreateMetric_InvalidType(t *testing.T) {
	h := NewMetricsHandler()
	r, w := newMetricsJSONRequest(t, http.MethodPost, "/server/admin/config/metrics/custom", CustomMetric{
		Name: "custom_widget_total",
		Type: "not-a-real-type",
	})
	h.CreateMetric(w, r)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Code)
	}
}

// TestMetricsHandler_DeleteMetric_Success verifies a named metric delete
// returns 200 and echoes the name.
func TestMetricsHandler_DeleteMetric_Success(t *testing.T) {
	h := NewMetricsHandler()
	r, w := newMetricsAPIRequest("/server/admin/config/metrics/custom/custom_widget_total")
	r = setMetricNameParam(r, "custom_widget_total")
	h.DeleteMetric(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	if !bytes.Contains(w.Body.Bytes(), []byte("custom_widget_total")) {
		t.Errorf("expected response to echo metric name, got %s", w.Body.String())
	}
}

// TestMetricsHandler_DeleteMetric_MissingName verifies an empty :name
// param is rejected with 400.
func TestMetricsHandler_DeleteMetric_MissingName(t *testing.T) {
	h := NewMetricsHandler()
	r, w := newMetricsAPIRequest("/server/admin/config/metrics/custom/")
	r = setMetricNameParam(r, "")
	h.DeleteMetric(w, r)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Code)
	}
}

// TestMetricsHandler_ExportMetrics_Prometheus verifies the default (no
// format query param) export uses Prometheus text format.
func TestMetricsHandler_ExportMetrics_Prometheus(t *testing.T) {
	h := NewMetricsHandler()
	r, w := newMetricsAPIRequest("/server/admin/config/metrics/export")
	h.ExportMetrics(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	if ct := w.Header().Get("Content-Type"); ct != "text/plain; version=0.0.4" {
		t.Errorf("unexpected content type: %q", ct)
	}
	if !bytes.Contains(w.Body.Bytes(), []byte("wthr_http_requests_total")) {
		t.Error("expected Prometheus-format body")
	}
}

// TestMetricsHandler_ExportMetrics_JSON verifies format=json dispatches
// to the JSON exporter.
func TestMetricsHandler_ExportMetrics_JSON(t *testing.T) {
	h := NewMetricsHandler()
	r, w := newMetricsAPIRequest("/server/admin/config/metrics/export?format=json")
	h.ExportMetrics(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	var got map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("expected valid JSON body: %v", err)
	}
	if _, ok := got["http_requests_total"]; !ok {
		t.Error("expected http_requests_total key in JSON export")
	}
}

// TestMetricsHandler_ExportMetrics_OpenMetrics verifies
// format=openmetrics dispatches to the OpenMetrics exporter.
func TestMetricsHandler_ExportMetrics_OpenMetrics(t *testing.T) {
	h := NewMetricsHandler()
	r, w := newMetricsAPIRequest("/server/admin/config/metrics/export?format=openmetrics")
	h.ExportMetrics(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	if ct := w.Header().Get("Content-Type"); ct != "application/openmetrics-text; version=1.0.0; charset=utf-8" {
		t.Errorf("unexpected content type: %q", ct)
	}
	if !bytes.Contains(w.Body.Bytes(), []byte("# EOF")) {
		t.Error("expected OpenMetrics-format body ending in # EOF")
	}
}

// TestMetricsHandler_ToggleMetric_Success verifies a valid toggle body
// returns 200 and echoes the requested state.
func TestMetricsHandler_ToggleMetric_Success(t *testing.T) {
	h := NewMetricsHandler()
	r, w := newMetricsJSONRequest(t, http.MethodPost, "/server/admin/config/metrics/custom/custom_widget_total/toggle", map[string]bool{"enabled": false})
	r = setMetricNameParam(r, "custom_widget_total")
	h.ToggleMetric(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	if !bytes.Contains(w.Body.Bytes(), []byte(`"enabled":false`)) {
		t.Errorf("expected enabled:false in response, got %s", w.Body.String())
	}
}

// TestMetricsHandler_ToggleMetric_InvalidJSON verifies malformed toggle
// bodies are rejected with 400.
func TestMetricsHandler_ToggleMetric_InvalidJSON(t *testing.T) {
	h := NewMetricsHandler()
	r, w := newMetricsJSONRequest(t, http.MethodPost, "/server/admin/config/metrics/custom/x/toggle", "{not-json")
	r = setMetricNameParam(r, "x")

	h.ToggleMetric(w, r)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Code)
	}
}
