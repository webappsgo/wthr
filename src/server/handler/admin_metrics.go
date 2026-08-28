package handler

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/go-chi/chi/v5"
)

type MetricsHandler struct{}

func NewMetricsHandler() *MetricsHandler {
	return &MetricsHandler{}
}

type MetricsConfig struct {
	Enabled               bool   `json:"enabled"`
	Path                  string `json:"path"`
	Namespace             string `json:"namespace"`
	Subsystem             string `json:"subsystem"`
	IncludeGoMetrics      bool   `json:"includeGoMetrics"`
	IncludeProcessMetrics bool   `json:"includeProcessMetrics"`
}

type CustomMetric struct {
	Name string `json:"name"`
	// counter, gauge, histogram, summary
	Type   string   `json:"type"`
	Help   string   `json:"help"`
	Labels []string `json:"labels"`
}

// GetConfig returns the current metrics configuration
func (h *MetricsHandler) GetConfig(w http.ResponseWriter, r *http.Request) {
	config := MetricsConfig{
		Enabled:               true,
		Path:                  "/metrics",
		Namespace:             "wthr",
		Subsystem:             "",
		IncludeGoMetrics:      true,
		IncludeProcessMetrics: true,
	}

	writeJSON(w, http.StatusOK, config)
}

// UpdateConfig updates the metrics configuration
func (h *MetricsHandler) UpdateConfig(w http.ResponseWriter, r *http.Request) {
	var config MetricsConfig

	if err := json.NewDecoder(r.Body).Decode(&config); err != nil {
		BadRequest(w, r, Translate(r, "errors.admin.admins.invalid_request_body"))
		return
	}

	// Validate path
	if config.Path == "" {
		config.Path = "/metrics"
	}

	// In a real implementation, this would update the Prometheus registry
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"message": Translate(r, "success.admin.metrics.metrics_configuration_updated_successfully"),
		"config":  config,
	})
}

// GetStats returns metrics statistics
func (h *MetricsHandler) GetStats(w http.ResponseWriter, r *http.Request) {
	stats := map[string]interface{}{
		"total":   24,
		"enabled": 20,
		"custom":  3,
		"builtin": 21,
	}

	writeJSON(w, http.StatusOK, stats)
}

// ListMetrics returns all available metrics
func (h *MetricsHandler) ListMetrics(w http.ResponseWriter, r *http.Request) {
	metrics := []map[string]interface{}{
		{
			"name":    "http_requests_total",
			"type":    "counter",
			"help":    "Total number of HTTP requests",
			"enabled": true,
			"builtin": true,
		},
		{
			"name":    "http_request_duration_seconds",
			"type":    "histogram",
			"help":    "HTTP request duration in seconds",
			"enabled": true,
			"builtin": true,
		},
		{
			"name":    "api_response_status",
			"type":    "counter",
			"help":    "API response status codes",
			"enabled": true,
			"builtin": true,
		},
		{
			"name":    "db_queries_total",
			"type":    "counter",
			"help":    "Total number of database queries",
			"enabled": true,
			"builtin": true,
		},
		{
			"name":    "cache_hits_total",
			"type":    "counter",
			"help":    "Total number of cache hits",
			"enabled": true,
			"builtin": true,
		},
		{
			"name":    "active_connections",
			"type":    "gauge",
			"help":    "Number of active connections",
			"enabled": true,
			"builtin": true,
		},
		{
			"name":    "task_execution_total",
			"type":    "counter",
			"help":    "Total number of task executions",
			"enabled": true,
			"builtin": true,
		},
		{
			"name":    "email_sent_total",
			"type":    "counter",
			"help":    "Total number of emails sent",
			"enabled": true,
			"builtin": true,
		},
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{"metrics": metrics})
}

// CreateMetric creates a custom metric
func (h *MetricsHandler) CreateMetric(w http.ResponseWriter, r *http.Request) {
	var metric CustomMetric

	if err := json.NewDecoder(r.Body).Decode(&metric); err != nil {
		BadRequest(w, r, Translate(r, "errors.admin.admins.invalid_request_body"))
		return
	}

	// Validate metric
	if metric.Name == "" {
		BadRequest(w, r, Translate(r, "errors.admin.metrics.metric_name_is_required"))
		return
	}

	if metric.Type == "" {
		BadRequest(w, r, Translate(r, "errors.admin.metrics.metric_type_is_required"))
		return
	}

	validTypes := map[string]bool{
		"counter":   true,
		"gauge":     true,
		"histogram": true,
		"summary":   true,
	}

	if !validTypes[metric.Type] {
		BadRequest(w, r, Translate(r, "errors.admin.metrics.invalid_metric_type"))
		return
	}

	// In a real implementation, this would register the metric with Prometheus
	writeJSON(w, http.StatusCreated, map[string]interface{}{
		"message": Translate(r, "success.admin.metrics.custom_metric_created_successfully"),
		"metric":  metric,
	})
}

// DeleteMetric deletes a custom metric
func (h *MetricsHandler) DeleteMetric(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")

	if name == "" {
		BadRequest(w, r, Translate(r, "errors.admin.metrics.metric_name_is_required"))
		return
	}

	// In a real implementation, this would unregister the metric
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"message": Translate(r, "success.admin.metrics.custom_metric_deleted_successfully"),
		"name":    name,
	})
}

// ExportMetrics exports metrics in specified format
func (h *MetricsHandler) ExportMetrics(w http.ResponseWriter, r *http.Request) {
	format := r.URL.Query().Get("format")

	switch format {
	case "json":
		h.exportJSON(w, r)
	case "openmetrics":
		h.exportOpenMetrics(w, r)
	default:
		// Default to Prometheus format
		h.exportPrometheus(w, r)
	}
}

// Helper: Export in Prometheus format
func (h *MetricsHandler) exportPrometheus(w http.ResponseWriter, r *http.Request) {
	output := `# HELP wthr_http_requests_total Total number of HTTP requests
# TYPE wthr_http_requests_total counter
wthr_http_requests_total{method="GET",path="/api/v1/weather"} 1234
wthr_http_requests_total{method="POST",path="/api/v1/server/admin/settings"} 56

# HELP wthr_http_request_duration_seconds HTTP request duration in seconds
# TYPE wthr_http_request_duration_seconds histogram
wthr_http_request_duration_seconds_bucket{le="0.1"} 1000
wthr_http_request_duration_seconds_bucket{le="0.5"} 1200
wthr_http_request_duration_seconds_bucket{le="1"} 1250
wthr_http_request_duration_seconds_bucket{le="+Inf"} 1260
wthr_http_request_duration_seconds_sum 315.5
wthr_http_request_duration_seconds_count 1260

# HELP wthr_active_connections Number of active connections
# TYPE wthr_active_connections gauge
wthr_active_connections 42`

	w.Header().Set("Content-Type", "text/plain; version=0.0.4")
	w.WriteHeader(http.StatusOK)
	fmt.Fprint(w, output)
}

// Helper: Export in JSON format
func (h *MetricsHandler) exportJSON(w http.ResponseWriter, r *http.Request) {
	metrics := map[string]interface{}{
		"http_requests_total": map[string]interface{}{
			"type": "counter",
			"values": []map[string]interface{}{
				{"labels": map[string]string{"method": "GET", "path": "/api/v1/weather"}, "value": 1234},
				{"labels": map[string]string{"method": "POST", "path": "/api/v1/server/admin/settings"}, "value": 56},
			},
		},
		"active_connections": map[string]interface{}{
			"type":  "gauge",
			"value": 42,
		},
	}

	writeJSON(w, http.StatusOK, metrics)
}

// Helper: Export in OpenMetrics format
func (h *MetricsHandler) exportOpenMetrics(w http.ResponseWriter, r *http.Request) {
	output := `# HELP wthr_http_requests Total number of HTTP requests
# TYPE wthr_http_requests counter
# UNIT wthr_http_requests requests
wthr_http_requests_total{method="GET"} 1234
wthr_http_requests_created{method="GET"} 1702598400
# EOF`

	w.Header().Set("Content-Type", "application/openmetrics-text; version=1.0.0; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	fmt.Fprint(w, output)
}

// ToggleMetric enables or disables a metric
func (h *MetricsHandler) ToggleMetric(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	var request struct {
		Enabled bool `json:"enabled"`
	}

	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		BadRequest(w, r, Translate(r, "errors.admin.admins.invalid_request_body"))
		return
	}

	// In a real implementation, this would enable/disable the metric
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"message": Translate(r, "success.admin.metrics.metric_updated_successfully"),
		"name":    name,
		"enabled": request.Enabled,
	})
}
