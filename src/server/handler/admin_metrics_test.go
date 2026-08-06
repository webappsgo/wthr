package handler

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

// TestNewMetricsHandler verifies the constructor returns a non-nil,
// zero-value handler (MetricsHandler carries no fields/dependencies).
func TestNewMetricsHandler(t *testing.T) {
	h := NewMetricsHandler()
	if h == nil {
		t.Fatal("expected non-nil handler")
	}
}

// newMetricsJSONContext builds a gin test context with an optional JSON
// body attached to the request, mirroring the pattern used elsewhere in
// this package for JSON-bound handlers.
func newMetricsJSONContext(method, target string, body interface{}) (*gin.Context, *httptest.ResponseRecorder) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	var reqBody *bytes.Buffer
	if body != nil {
		raw, _ := json.Marshal(body)
		reqBody = bytes.NewBuffer(raw)
	} else {
		reqBody = bytes.NewBuffer(nil)
	}

	c.Request = httptest.NewRequest(method, target, reqBody)
	c.Request.Header.Set("Content-Type", "application/json")
	return c, w
}

// TestMetricsHandler_GetConfig verifies the static config payload is
// returned with the expected enabled flag and default path.
func TestMetricsHandler_GetConfig(t *testing.T) {
	h := NewMetricsHandler()
	c, w := newAPITestContext("/server/admin/config/metrics")
	h.GetConfig(c)

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
	c, w := newMetricsJSONContext(http.MethodPost, "/server/admin/config/metrics", MetricsConfig{
		Enabled: true,
		Path:    "/custom-metrics",
	})
	h.UpdateConfig(c)

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
	c, w := newMetricsJSONContext(http.MethodPost, "/server/admin/config/metrics", MetricsConfig{
		Enabled: true,
		Path:    "",
	})
	h.UpdateConfig(c)

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
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/server/admin/config/metrics", bytes.NewBufferString("{not-json"))
	c.Request.Header.Set("Content-Type", "application/json")

	h.UpdateConfig(c)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Code)
	}
}

// TestMetricsHandler_GetStats verifies the stats payload contains the
// expected keys.
func TestMetricsHandler_GetStats(t *testing.T) {
	h := NewMetricsHandler()
	c, w := newAPITestContext("/server/admin/config/metrics/stats")
	h.GetStats(c)

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
	c, w := newAPITestContext("/server/admin/config/metrics/list")
	h.ListMetrics(c)

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
	c, w := newMetricsJSONContext(http.MethodPost, "/server/admin/config/metrics/custom", CustomMetric{
		Name: "custom_widget_total",
		Type: "counter",
		Help: "Total widgets processed",
	})
	h.CreateMetric(c)

	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201", w.Code)
	}
}

// TestMetricsHandler_CreateMetric_InvalidJSON verifies malformed bodies
// are rejected.
func TestMetricsHandler_CreateMetric_InvalidJSON(t *testing.T) {
	h := NewMetricsHandler()
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/server/admin/config/metrics/custom", bytes.NewBufferString("{not-json"))
	c.Request.Header.Set("Content-Type", "application/json")

	h.CreateMetric(c)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Code)
	}
}

// TestMetricsHandler_CreateMetric_MissingName verifies an empty metric
// name is rejected with 400.
func TestMetricsHandler_CreateMetric_MissingName(t *testing.T) {
	h := NewMetricsHandler()
	c, w := newMetricsJSONContext(http.MethodPost, "/server/admin/config/metrics/custom", CustomMetric{
		Type: "counter",
	})
	h.CreateMetric(c)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Code)
	}
}

// TestMetricsHandler_CreateMetric_MissingType verifies an empty metric
// type is rejected with 400.
func TestMetricsHandler_CreateMetric_MissingType(t *testing.T) {
	h := NewMetricsHandler()
	c, w := newMetricsJSONContext(http.MethodPost, "/server/admin/config/metrics/custom", CustomMetric{
		Name: "custom_widget_total",
	})
	h.CreateMetric(c)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Code)
	}
}

// TestMetricsHandler_CreateMetric_InvalidType verifies a metric type
// outside the allowed set (counter/gauge/histogram/summary) is rejected.
func TestMetricsHandler_CreateMetric_InvalidType(t *testing.T) {
	h := NewMetricsHandler()
	c, w := newMetricsJSONContext(http.MethodPost, "/server/admin/config/metrics/custom", CustomMetric{
		Name: "custom_widget_total",
		Type: "not-a-real-type",
	})
	h.CreateMetric(c)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Code)
	}
}

// TestMetricsHandler_DeleteMetric_Success verifies a named metric delete
// returns 200 and echoes the name.
func TestMetricsHandler_DeleteMetric_Success(t *testing.T) {
	h := NewMetricsHandler()
	c, w := newAPITestContext("/server/admin/config/metrics/custom/custom_widget_total")
	c.Params = gin.Params{{Key: "name", Value: "custom_widget_total"}}
	h.DeleteMetric(c)

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
	c, w := newAPITestContext("/server/admin/config/metrics/custom/")
	c.Params = gin.Params{{Key: "name", Value: ""}}
	h.DeleteMetric(c)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Code)
	}
}

// TestMetricsHandler_ExportMetrics_Prometheus verifies the default (no
// format query param) export uses Prometheus text format.
func TestMetricsHandler_ExportMetrics_Prometheus(t *testing.T) {
	h := NewMetricsHandler()
	c, w := newAPITestContext("/server/admin/config/metrics/export")
	h.ExportMetrics(c)

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
	c, w := newAPITestContext("/server/admin/config/metrics/export?format=json")
	h.ExportMetrics(c)

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
	c, w := newAPITestContext("/server/admin/config/metrics/export?format=openmetrics")
	h.ExportMetrics(c)

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
	c, w := newMetricsJSONContext(http.MethodPost, "/server/admin/config/metrics/custom/custom_widget_total/toggle", map[string]bool{"enabled": false})
	c.Params = gin.Params{{Key: "name", Value: "custom_widget_total"}}
	h.ToggleMetric(c)

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
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/server/admin/config/metrics/custom/x/toggle", bytes.NewBufferString("{not-json"))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Params = gin.Params{{Key: "name", Value: "x"}}

	h.ToggleMetric(c)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Code)
	}
}
