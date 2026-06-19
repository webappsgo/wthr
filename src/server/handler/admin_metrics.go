package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
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
	Name   string   `json:"name"`
	// counter, gauge, histogram, summary
	Type   string   `json:"type"`
	Help   string   `json:"help"`
	Labels []string `json:"labels"`
}

// GetConfig returns the current metrics configuration
func (h *MetricsHandler) GetConfig(c *gin.Context) {
	config := MetricsConfig{
		Enabled:               true,
		Path:                  "/metrics",
		Namespace:             "wthr",
		Subsystem:             "",
		IncludeGoMetrics:      true,
		IncludeProcessMetrics: true,
	}

	c.JSON(http.StatusOK, config)
}

// UpdateConfig updates the metrics configuration
func (h *MetricsHandler) UpdateConfig(c *gin.Context) {
	var config MetricsConfig

	if err := c.ShouldBindJSON(&config); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body"})
		return
	}

	// Validate path
	if config.Path == "" {
		config.Path = "/metrics"
	}

	// In a real implementation, this would update the Prometheus registry
	c.JSON(http.StatusOK, gin.H{
		"message": "Metrics configuration updated successfully",
		"config":  config,
	})
}

// GetStats returns metrics statistics
func (h *MetricsHandler) GetStats(c *gin.Context) {
	stats := map[string]interface{}{
		"total":   24,
		"enabled": 20,
		"custom":  3,
		"builtin": 21,
	}

	c.JSON(http.StatusOK, stats)
}

// ListMetrics returns all available metrics
func (h *MetricsHandler) ListMetrics(c *gin.Context) {
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

	c.JSON(http.StatusOK, gin.H{"metrics": metrics})
}

// CreateMetric creates a custom metric
func (h *MetricsHandler) CreateMetric(c *gin.Context) {
	var metric CustomMetric

	if err := c.ShouldBindJSON(&metric); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body"})
		return
	}

	// Validate metric
	if metric.Name == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Metric name is required"})
		return
	}

	if metric.Type == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Metric type is required"})
		return
	}

	validTypes := map[string]bool{
		"counter":   true,
		"gauge":     true,
		"histogram": true,
		"summary":   true,
	}

	if !validTypes[metric.Type] {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid metric type"})
		return
	}

	// In a real implementation, this would register the metric with Prometheus
	c.JSON(http.StatusCreated, gin.H{
		"message": "Custom metric created successfully",
		"metric":  metric,
	})
}

// DeleteMetric deletes a custom metric
func (h *MetricsHandler) DeleteMetric(c *gin.Context) {
	name := c.Param("name")

	if name == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Metric name is required"})
		return
	}

	// In a real implementation, this would unregister the metric
	c.JSON(http.StatusOK, gin.H{
		"message": "Custom metric deleted successfully",
		"name":    name,
	})
}

// ExportMetrics exports metrics in specified format
func (h *MetricsHandler) ExportMetrics(c *gin.Context) {
	format := c.Query("format")

	switch format {
	case "json":
		h.exportJSON(c)
	case "openmetrics":
		h.exportOpenMetrics(c)
	default:
		// Default to Prometheus format
		h.exportPrometheus(c)
	}
}

// Helper: Export in Prometheus format
func (h *MetricsHandler) exportPrometheus(c *gin.Context) {
	output := `# HELP weather_http_requests_total Total number of HTTP requests
# TYPE weather_http_requests_total counter
weather_http_requests_total{method="GET",path="/api/v1/weather"} 1234
weather_http_requests_total{method="POST",path="/api/v1/server/admin/settings"} 56

# HELP weather_http_request_duration_seconds HTTP request duration in seconds
# TYPE weather_http_request_duration_seconds histogram
weather_http_request_duration_seconds_bucket{le="0.1"} 1000
weather_http_request_duration_seconds_bucket{le="0.5"} 1200
weather_http_request_duration_seconds_bucket{le="1"} 1250
weather_http_request_duration_seconds_bucket{le="+Inf"} 1260
weather_http_request_duration_seconds_sum 315.5
weather_http_request_duration_seconds_count 1260

# HELP weather_active_connections Number of active connections
# TYPE weather_active_connections gauge
weather_active_connections 42`

	c.Header("Content-Type", "text/plain; version=0.0.4")
	c.String(http.StatusOK, output)
}

// Helper: Export in JSON format
func (h *MetricsHandler) exportJSON(c *gin.Context) {
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

	c.JSON(http.StatusOK, metrics)
}

// Helper: Export in OpenMetrics format
func (h *MetricsHandler) exportOpenMetrics(c *gin.Context) {
	output := `# HELP weather_http_requests Total number of HTTP requests
# TYPE weather_http_requests counter
# UNIT weather_http_requests requests
weather_http_requests_total{method="GET"} 1234
weather_http_requests_created{method="GET"} 1702598400
# EOF`

	c.Header("Content-Type", "application/openmetrics-text; version=1.0.0; charset=utf-8")
	c.String(http.StatusOK, output)
}

// ToggleMetric enables or disables a metric
func (h *MetricsHandler) ToggleMetric(c *gin.Context) {
	name := c.Param("name")
	var request struct {
		Enabled bool `json:"enabled"`
	}

	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body"})
		return
	}

	// In a real implementation, this would enable/disable the metric
	c.JSON(http.StatusOK, gin.H{
		"message": "Metric updated successfully",
		"name":    name,
		"enabled": request.Enabled,
	})
}
