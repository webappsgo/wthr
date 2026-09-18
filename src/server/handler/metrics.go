package handler

import (
	"crypto/subtle"
	"encoding/json"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/webappsgo/wthr/src/config"
)

// Metric service names accepted on the metrics routes per AI.md PART 21.
const (
	MetricServicePrometheus = "prometheus"
	MetricServiceGrafana    = "grafana"
	MetricServiceLoki       = "loki"
)

// credentialPattern matches key=value and key: value pairs whose key names a
// credential, so the loki service never streams a secret out of the log file.
var credentialPattern = regexp.MustCompile(`(?i)\b(password|passwd|secret|token|api[_-]?key|authorization|bearer)\b\s*[:=]\s*\S+`)

// MetricsHandler serves the three metrics services defined by AI.md PART 21.
// Every route is internal-only and gated by a per-service bearer token.
type MetricsHandler struct {
	settings config.MetricsConfig
	logs     *LogsHandler
	exposer  http.Handler
}

// NewMetricsHandler builds the metrics handler from the resolved server config.
// logDir is the directory holding the application log, which the loki service
// converts into Loki push-API streams.
func NewMetricsHandler(settings config.MetricsConfig, logDir string) *MetricsHandler {
	return &MetricsHandler{
		settings: settings,
		logs:     NewLogsHandler(logDir),
		exposer:  promhttp.Handler(),
	}
}

// RootAliasEnabled reports whether the root /metrics alias should be mounted.
// The alias is on by default because Prometheus scrapers expect that path.
func (h *MetricsHandler) RootAliasEnabled() bool {
	return h.settings.Enabled && h.settings.Root.Enabled
}

// ServeMetricsService dispatches on the {service} route parameter, defaulting to
// the prometheus service when the parameter is absent.
func (h *MetricsHandler) ServeMetricsService(w http.ResponseWriter, r *http.Request) {
	switch chi.URLParam(r, "service") {
	case "", MetricServicePrometheus:
		h.ServePrometheusExposition(w, r)
	case MetricServiceGrafana:
		h.ServeGrafanaDashboard(w, r)
	case MetricServiceLoki:
		h.ServeLokiStreams(w, r)
	default:
		http.NotFound(w, r)
	}
}

// ServePrometheusExposition writes the full Prometheus text exposition.
func (h *MetricsHandler) ServePrometheusExposition(w http.ResponseWriter, r *http.Request) {
	if !h.authorizeService(w, r, MetricServicePrometheus) {
		return
	}

	h.exposer.ServeHTTP(w, r)
}

// ServeGrafanaDashboard writes an importable Grafana dashboard covering every
// metric category this project exports.
func (h *MetricsHandler) ServeGrafanaDashboard(w http.ResponseWriter, r *http.Request) {
	if !h.authorizeService(w, r, MetricServiceGrafana) {
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)

	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	_ = encoder.Encode(grafanaDashboard())
}

// ServeLokiStreams writes recent log entries in Loki push-API stream format,
// bounded by the configured entry count and age.
func (h *MetricsHandler) ServeLokiStreams(w http.ResponseWriter, r *http.Request) {
	if !h.authorizeService(w, r, MetricServiceLoki) {
		return
	}

	maxEntries := h.settings.Loki.MaxEntries
	if maxEntries <= 0 {
		maxEntries = 1000
	}

	entries, err := h.logs.readLogs(maxEntries)
	if err != nil {
		InternalError(w, r, Translate(r, "errors.admin.logs.failed_to_read_logs"))
		return
	}

	var cutoff time.Time
	if maxAge, parseErr := time.ParseDuration(h.settings.Loki.MaxAge); parseErr == nil && maxAge > 0 {
		cutoff = time.Now().Add(-maxAge)
	}

	// One stream per log level keeps label cardinality bounded.
	values := make(map[string][][2]string)
	for _, entry := range entries {
		stamp, parseErr := time.ParseInLocation("2006-01-02 15:04:05", entry.Timestamp, time.Local)
		if parseErr != nil {
			stamp = time.Now()
		}
		if !cutoff.IsZero() && stamp.Before(cutoff) {
			continue
		}

		level := strings.ToLower(entry.Level)
		if level == "" {
			level = "info"
		}

		line := entry.Message
		if entry.Source != "" {
			line = entry.Source + ": " + entry.Message
		}

		values[level] = append(values[level], [2]string{
			strconv.FormatInt(stamp.UnixNano(), 10),
			redactLogLine(line),
		})
	}

	streams := make([]map[string]interface{}, 0, len(values))
	for level, lines := range values {
		streams = append(streams, map[string]interface{}{
			"stream": map[string]string{
				"app":   "wthr",
				"level": level,
			},
			"values": lines,
		})
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{"streams": streams})
}

// authorizeService enforces the PART 21 bearer-token rules for one service. It
// writes the failure response itself and reports whether the request may proceed.
func (h *MetricsHandler) authorizeService(w http.ResponseWriter, r *http.Request, service string) bool {
	// A disabled metrics subsystem must not reveal that these routes exist.
	if !h.settings.Enabled {
		http.NotFound(w, r)
		return false
	}

	if h.settings.Auth.AllowUnauthenticated {
		return true
	}

	expected := h.settings.Auth.Tokens[service]
	if expected == "" {
		// An empty token disables that service with an empty 403 body.
		w.WriteHeader(http.StatusForbidden)
		return false
	}

	// Header only: query-string tokens leak into access logs and proxies.
	presented := strings.TrimSpace(strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "))
	if presented == "" || subtle.ConstantTimeCompare([]byte(presented), []byte(expected)) != 1 {
		w.Header().Set("WWW-Authenticate", `Bearer realm="metrics"`)
		w.WriteHeader(http.StatusUnauthorized)
		return false
	}

	return true
}

// redactLogLine masks credential values so they never leave the server through
// the loki service, matching the sanitization applied to log files themselves.
func redactLogLine(line string) string {
	return credentialPattern.ReplaceAllStringFunc(line, func(match string) string {
		separator := strings.LastIndexAny(match, ":=")
		if separator < 0 {
			return match
		}

		return match[:separator+1] + " xxxxx"
	})
}

// grafanaDashboard builds a schema-current dashboard definition whose panels
// cover every metric category the specification requires. The datasource stays a
// template variable so the dashboard imports against any Prometheus datasource.
func grafanaDashboard() map[string]interface{} {
	datasource := map[string]string{"type": "prometheus", "uid": "${datasource}"}

	panels := []map[string]interface{}{
		grafanaPanel(1, "HTTP Requests", "sum(rate(wthr_http_requests_total[5m])) by (status)", datasource, 0),
		grafanaPanel(2, "HTTP Latency (p95)", "histogram_quantile(0.95, sum(rate(wthr_http_request_duration_seconds_bucket[5m])) by (le))", datasource, 8),
		grafanaPanel(3, "Active Requests", "wthr_http_active_requests", datasource, 16),
		grafanaPanel(4, "Database Queries", "sum(rate(wthr_db_queries_total[5m]))", datasource, 24),
		grafanaPanel(5, "Database Connections", "wthr_db_connections_in_use", datasource, 32),
		grafanaPanel(6, "Cache Hit Ratio", "sum(rate(wthr_cache_hits_total[5m])) / clamp_min(sum(rate(wthr_cache_hits_total[5m])) + sum(rate(wthr_cache_misses_total[5m])), 1)", datasource, 40),
		grafanaPanel(7, "Scheduler Tasks", "sum(rate(wthr_scheduler_tasks_total[5m])) by (status)", datasource, 48),
		grafanaPanel(8, "CPU Usage", "wthr_system_cpu_usage_percent", datasource, 56),
		grafanaPanel(9, "Memory Usage", "wthr_system_memory_usage_percent", datasource, 64),
		grafanaPanel(10, "Disk Usage", "wthr_system_disk_usage_percent", datasource, 72),
		grafanaPanel(11, "Goroutines", "wthr_go_goroutines", datasource, 80),
		grafanaPanel(12, "Authentication Attempts", "sum(rate(wthr_auth_attempts_total[5m])) by (result)", datasource, 88),
		grafanaPanel(13, "Active Sessions", "wthr_auth_sessions_active", datasource, 96),
		grafanaPanel(14, "Uptime", "wthr_app_uptime_seconds", datasource, 104),
	}

	return map[string]interface{}{
		"schemaVersion": 39,
		"title":         "wthr",
		"uid":           "wthr-overview",
		"tags":          []string{"wthr"},
		"timezone":      "browser",
		"editable":      true,
		"time":          map[string]string{"from": "now-6h", "to": "now"},
		"refresh":       "30s",
		"templating": map[string]interface{}{
			"list": []map[string]interface{}{
				{
					"name":  "datasource",
					"label": "Prometheus",
					"type":  "datasource",
					"query": "prometheus",
				},
			},
		},
		"panels": panels,
	}
}

// grafanaPanel builds one time-series panel for the dashboard.
func grafanaPanel(id int, title, expr string, datasource map[string]string, y int) map[string]interface{} {
	return map[string]interface{}{
		"id":         id,
		"type":       "timeseries",
		"title":      title,
		"datasource": datasource,
		"gridPos":    map[string]int{"h": 8, "w": 12, "x": 0, "y": y},
		"targets": []map[string]interface{}{
			{
				"datasource": datasource,
				"expr":       expr,
				"refId":      "A",
			},
		},
	}
}
