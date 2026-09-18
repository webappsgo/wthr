package handler

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/webappsgo/wthr/src/config"
)

// newTestMetricsRegistry builds a registry holding one known counter so the
// admin handlers have deterministic content to report on.
func newTestMetricsRegistry(t *testing.T) *prometheus.Registry {
	t.Helper()

	registry := prometheus.NewRegistry()
	counter := prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "wthr_test_requests_total",
		Help: "Test counter for the admin metrics handler",
	}, []string{"method"})

	if err := registry.Register(counter); err != nil {
		t.Fatalf("register test counter: %v", err)
	}
	counter.WithLabelValues("GET").Inc()

	return registry
}

// TestNewAdminMetricsHandler verifies the constructor falls back to the
// default gatherer rather than returning a handler that cannot gather.
func TestNewAdminMetricsHandler(t *testing.T) {
	h := NewAdminMetricsHandler(nil)
	if h == nil || h.gatherer == nil || h.exposer == nil {
		t.Fatal("expected a handler backed by the default gatherer")
	}
}

// TestAdminMetricsHandler_GetMetricsStats verifies the stats payload counts the
// registered families and series rather than reporting fixed numbers.
func TestAdminMetricsHandler_GetMetricsStats(t *testing.T) {
	h := NewAdminMetricsHandler(newTestMetricsRegistry(t))

	w := httptest.NewRecorder()
	h.GetMetricsStats(w, httptest.NewRequest(http.MethodGet, "/stats", nil))

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}

	var got struct {
		Families int            `json:"families"`
		Series   int            `json:"series"`
		ByType   map[string]int `json:"byType"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode body: %v", err)
	}

	if got.Families != 1 || got.Series != 1 {
		t.Fatalf("families/series = %d/%d, want 1/1", got.Families, got.Series)
	}
	if got.ByType["counter"] != 1 {
		t.Fatalf("byType[counter] = %d, want 1", got.ByType["counter"])
	}
}

// TestAdminMetricsHandler_ListRegisteredMetrics verifies the listing reflects
// the live registry.
func TestAdminMetricsHandler_ListRegisteredMetrics(t *testing.T) {
	h := NewAdminMetricsHandler(newTestMetricsRegistry(t))

	w := httptest.NewRecorder()
	h.ListRegisteredMetrics(w, httptest.NewRequest(http.MethodGet, "/list", nil))

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}

	var got struct {
		Metrics []AdminMetricFamily `json:"metrics"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode body: %v", err)
	}

	if len(got.Metrics) != 1 || got.Metrics[0].Name != "wthr_test_requests_total" {
		t.Fatalf("metrics = %+v, want the registered test counter", got.Metrics)
	}
	if got.Metrics[0].Type != "counter" || got.Metrics[0].Series != 1 {
		t.Fatalf("family = %+v, want a counter with one series", got.Metrics[0])
	}
}

// TestAdminMetricsHandler_ExportMetrics_Prometheus verifies the default export
// is real exposition text produced from the registry.
func TestAdminMetricsHandler_ExportMetrics_Prometheus(t *testing.T) {
	h := NewAdminMetricsHandler(newTestMetricsRegistry(t))

	w := httptest.NewRecorder()
	h.ExportMetrics(w, httptest.NewRequest(http.MethodGet, "/export", nil))

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}
	if !strings.Contains(w.Body.String(), "wthr_test_requests_total") {
		t.Fatalf("body missing the registered metric: %q", w.Body.String())
	}
}

// TestAdminMetricsHandler_ExportMetrics_OpenMetrics verifies the OpenMetrics
// format is negotiated and terminated with the required EOF marker.
func TestAdminMetricsHandler_ExportMetrics_OpenMetrics(t *testing.T) {
	h := NewAdminMetricsHandler(newTestMetricsRegistry(t))

	w := httptest.NewRecorder()
	h.ExportMetrics(w, httptest.NewRequest(http.MethodGet, "/export?format=openmetrics", nil))

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}
	if contentType := w.Header().Get("Content-Type"); !strings.Contains(contentType, "openmetrics-text") {
		t.Fatalf("content-type = %q, want openmetrics-text", contentType)
	}
	if !strings.Contains(w.Body.String(), "# EOF") {
		t.Fatalf("openmetrics body missing EOF marker: %q", w.Body.String())
	}
}

// TestAdminMetricsHandler_ExportMetrics_JSON verifies the JSON export carries
// the gathered value rather than a fabricated one.
func TestAdminMetricsHandler_ExportMetrics_JSON(t *testing.T) {
	h := NewAdminMetricsHandler(newTestMetricsRegistry(t))

	w := httptest.NewRecorder()
	h.ExportMetrics(w, httptest.NewRequest(http.MethodGet, "/export?format=json", nil))

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}

	var got map[string]struct {
		Type   string `json:"type"`
		Values []struct {
			Labels map[string]string `json:"labels"`
			Value  float64           `json:"value"`
		} `json:"values"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode body: %v", err)
	}

	family, ok := got["wthr_test_requests_total"]
	if !ok {
		t.Fatalf("export missing the registered metric: %v", got)
	}
	if family.Type != "counter" {
		t.Fatalf("type = %q, want counter", family.Type)
	}
	if len(family.Values) != 1 || family.Values[0].Value != 1 {
		t.Fatalf("values = %+v, want a single sample of 1", family.Values)
	}
	if family.Values[0].Labels["method"] != "GET" {
		t.Fatalf("labels = %v, want method=GET", family.Values[0].Labels)
	}
}

// TestAdminMetricsSettingsFrom verifies configured tokens are masked and unset
// tokens stay empty so the admin panel can tell the two apart.
func TestAdminMetricsSettingsFrom(t *testing.T) {
	view := adminMetricsSettingsFrom(config.MetricsConfig{
		Enabled: true,
		Root:    config.MetricsRootConfig{Enabled: true},
		Auth: config.MetricsAuthConfig{
			Tokens: map[string]string{
				MetricServicePrometheus: "secret-value",
				MetricServiceGrafana:    "",
			},
		},
	})

	if view.Tokens[MetricServicePrometheus] != tokenMask {
		t.Fatalf("prometheus token = %q, want %q", view.Tokens[MetricServicePrometheus], tokenMask)
	}
	if view.Tokens[MetricServiceGrafana] != "" {
		t.Fatalf("grafana token = %q, want empty", view.Tokens[MetricServiceGrafana])
	}
	if view.Tokens[MetricServiceLoki] != "" {
		t.Fatalf("loki token = %q, want empty", view.Tokens[MetricServiceLoki])
	}
	if !view.Enabled || !view.RootAliasEnabled {
		t.Fatalf("view = %+v, want enabled with the root alias on", view)
	}
}

// TestMergeMetricsTokens verifies a submitted mask preserves the stored token,
// a new value replaces it, and an empty value clears it.
func TestMergeMetricsTokens(t *testing.T) {
	stored := map[string]string{
		MetricServicePrometheus: "stored-prometheus",
		MetricServiceGrafana:    "stored-grafana",
		MetricServiceLoki:       "stored-loki",
	}

	merged := mergeMetricsTokens(stored, map[string]string{
		MetricServicePrometheus: tokenMask,
		MetricServiceGrafana:    "rotated-grafana",
		MetricServiceLoki:       "",
	})

	if merged[MetricServicePrometheus] != "stored-prometheus" {
		t.Fatalf("prometheus = %q, want the stored token preserved", merged[MetricServicePrometheus])
	}
	if merged[MetricServiceGrafana] != "rotated-grafana" {
		t.Fatalf("grafana = %q, want the rotated token", merged[MetricServiceGrafana])
	}
	if merged[MetricServiceLoki] != "" {
		t.Fatalf("loki = %q, want cleared", merged[MetricServiceLoki])
	}
}

// TestAdminMetricsHandler_UpdateMetricsSettings_InvalidJSON verifies malformed
// bodies are rejected before any config write is attempted.
func TestAdminMetricsHandler_UpdateMetricsSettings_InvalidJSON(t *testing.T) {
	h := NewAdminMetricsHandler(newTestMetricsRegistry(t))

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPut, "/config", bytes.NewReader([]byte("{invalid")))
	r.Header.Set("Content-Type", "application/json")

	h.UpdateMetricsSettings(w, r)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}
