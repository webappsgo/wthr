package handler

import (
	"encoding/json"
	"net/http"
	"sort"
	"strings"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	dto "github.com/prometheus/client_model/go"

	"github.com/webappsgo/wthr/src/config"
)

// tokenMask is what a configured metrics bearer token looks like over the admin
// API. Submitting the mask back leaves the stored token untouched.
const tokenMask = "xxxxx"

// AdminMetricsHandler exposes the PART 21 metrics settings to the admin panel
// and reports on the metric families the process actually registered. The
// metric set itself is defined in code, so this surface configures and
// inspects it rather than creating or deleting metrics.
type AdminMetricsHandler struct {
	gatherer prometheus.Gatherer
	exposer  http.Handler
}

// NewAdminMetricsHandler builds the admin metrics handler around the registry
// the application collects into.
func NewAdminMetricsHandler(gatherer prometheus.Gatherer) *AdminMetricsHandler {
	if gatherer == nil {
		gatherer = prometheus.DefaultGatherer
	}

	return &AdminMetricsHandler{
		gatherer: gatherer,
		exposer: promhttp.HandlerFor(gatherer, promhttp.HandlerOpts{
			EnableOpenMetrics: true,
		}),
	}
}

// AdminMetricsSettings is the admin-facing view of server.metrics. Bearer
// tokens are masked on read and preserved when the mask is submitted back.
type AdminMetricsSettings struct {
	Enabled              bool              `json:"enabled"`
	RootAliasEnabled     bool              `json:"rootAliasEnabled"`
	AllowUnauthenticated bool              `json:"allowUnauthenticated"`
	Tokens               map[string]string `json:"tokens"`
	IncludeSystem        bool              `json:"includeSystem"`
	IncludeRuntime       bool              `json:"includeRuntime"`
	LokiMaxEntries       int               `json:"lokiMaxEntries"`
	LokiMaxAge           string            `json:"lokiMaxAge"`
	DurationBuckets      []float64         `json:"durationBuckets"`
	SizeBuckets          []float64         `json:"sizeBuckets"`
}

// AdminMetricFamily describes one registered metric family for the admin panel.
type AdminMetricFamily struct {
	Name   string `json:"name"`
	Type   string `json:"type"`
	Help   string `json:"help"`
	Series int    `json:"series"`
}

// GetMetricsSettings returns the live metrics configuration with tokens masked.
func (h *AdminMetricsHandler) GetMetricsSettings(w http.ResponseWriter, r *http.Request) {
	cfg := config.GetGlobalConfig()
	if cfg == nil {
		InternalError(w, r, Translate(r, "errors.admin.settings.failed_to_load_settings"))
		return
	}

	writeJSON(w, http.StatusOK, adminMetricsSettingsFrom(cfg.Server.Metrics))
}

// UpdateMetricsSettings validates and persists the metrics configuration.
func (h *AdminMetricsHandler) UpdateMetricsSettings(w http.ResponseWriter, r *http.Request) {
	var submitted AdminMetricsSettings

	if err := json.NewDecoder(r.Body).Decode(&submitted); err != nil {
		BadRequest(w, r, Translate(r, "errors.admin.admins.invalid_request_body"))
		return
	}

	cfg := config.GetGlobalConfig()
	if cfg == nil {
		InternalError(w, r, Translate(r, "errors.admin.settings.failed_to_load_settings"))
		return
	}

	next := cfg.Server.Metrics
	next.Enabled = submitted.Enabled
	next.Root.Enabled = submitted.RootAliasEnabled
	next.Auth.AllowUnauthenticated = submitted.AllowUnauthenticated
	next.IncludeSystem = submitted.IncludeSystem
	next.IncludeRuntime = submitted.IncludeRuntime
	next.Loki.MaxEntries = submitted.LokiMaxEntries
	next.Loki.MaxAge = strings.TrimSpace(submitted.LokiMaxAge)

	if len(submitted.DurationBuckets) > 0 {
		next.DurationBuckets = submitted.DurationBuckets
	}
	if len(submitted.SizeBuckets) > 0 {
		next.SizeBuckets = submitted.SizeBuckets
	}

	next.Auth.Tokens = mergeMetricsTokens(cfg.Server.Metrics.Auth.Tokens, submitted.Tokens)

	if errs := config.UpdateMetricsConfig(next); len(errs) > 0 {
		BadRequest(w, r, errs[0].Message)
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"message":  Translate(r, "success.admin.metrics.metrics_configuration_updated_successfully"),
		"settings": adminMetricsSettingsFrom(next),
	})
}

// GetMetricsStats reports how much the registry currently holds.
func (h *AdminMetricsHandler) GetMetricsStats(w http.ResponseWriter, r *http.Request) {
	families, err := h.gatherer.Gather()
	if err != nil {
		InternalError(w, r, Translate(r, "errors.admin.metrics.failed_to_read_metrics"))
		return
	}

	series := 0
	byType := make(map[string]int)
	for _, family := range families {
		series += len(family.GetMetric())
		byType[metricTypeName(family.GetType())]++
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"families": len(families),
		"series":   series,
		"byType":   byType,
	})
}

// ListRegisteredMetrics lists every metric family the process exports.
func (h *AdminMetricsHandler) ListRegisteredMetrics(w http.ResponseWriter, r *http.Request) {
	families, err := h.gatherer.Gather()
	if err != nil {
		InternalError(w, r, Translate(r, "errors.admin.metrics.failed_to_read_metrics"))
		return
	}

	listed := make([]AdminMetricFamily, 0, len(families))
	for _, family := range families {
		listed = append(listed, AdminMetricFamily{
			Name:   family.GetName(),
			Type:   metricTypeName(family.GetType()),
			Help:   family.GetHelp(),
			Series: len(family.GetMetric()),
		})
	}

	sort.Slice(listed, func(i, j int) bool { return listed[i].Name < listed[j].Name })

	writeJSON(w, http.StatusOK, map[string]interface{}{"metrics": listed})
}

// ExportMetrics writes the live registry in the requested format. Prometheus
// text is the default; openmetrics and json are opt-in via ?format=.
func (h *AdminMetricsHandler) ExportMetrics(w http.ResponseWriter, r *http.Request) {
	switch strings.ToLower(strings.TrimSpace(r.URL.Query().Get("format"))) {
	case "json":
		h.exportJSON(w, r)
	case "openmetrics":
		// promhttp emits OpenMetrics when the client negotiates it
		r.Header.Set("Accept", "application/openmetrics-text; version=1.0.0; charset=utf-8")
		h.exposer.ServeHTTP(w, r)
	default:
		r.Header.Set("Accept", "text/plain; version=0.0.4; charset=utf-8")
		h.exposer.ServeHTTP(w, r)
	}
}

// exportJSON renders the gathered families as JSON for admin tooling that does
// not parse the Prometheus text format.
func (h *AdminMetricsHandler) exportJSON(w http.ResponseWriter, r *http.Request) {
	families, err := h.gatherer.Gather()
	if err != nil {
		InternalError(w, r, Translate(r, "errors.admin.metrics.failed_to_read_metrics"))
		return
	}

	exported := make(map[string]interface{}, len(families))
	for _, family := range families {
		values := make([]map[string]interface{}, 0, len(family.GetMetric()))
		for _, metric := range family.GetMetric() {
			labels := make(map[string]string, len(metric.GetLabel()))
			for _, label := range metric.GetLabel() {
				labels[label.GetName()] = label.GetValue()
			}

			values = append(values, map[string]interface{}{
				"labels": labels,
				"value":  metricValue(family.GetType(), metric),
			})
		}

		exported[family.GetName()] = map[string]interface{}{
			"type":   metricTypeName(family.GetType()),
			"help":   family.GetHelp(),
			"values": values,
		}
	}

	writeJSON(w, http.StatusOK, exported)
}

// adminMetricsSettingsFrom converts stored config into the admin view, masking
// every configured bearer token.
func adminMetricsSettingsFrom(settings config.MetricsConfig) AdminMetricsSettings {
	tokens := make(map[string]string, len(settings.Auth.Tokens))
	for _, service := range []string{MetricServicePrometheus, MetricServiceGrafana, MetricServiceLoki} {
		if settings.Auth.Tokens[service] != "" {
			tokens[service] = tokenMask
			continue
		}
		tokens[service] = ""
	}

	return AdminMetricsSettings{
		Enabled:              settings.Enabled,
		RootAliasEnabled:     settings.Root.Enabled,
		AllowUnauthenticated: settings.Auth.AllowUnauthenticated,
		Tokens:               tokens,
		IncludeSystem:        settings.IncludeSystem,
		IncludeRuntime:       settings.IncludeRuntime,
		LokiMaxEntries:       settings.Loki.MaxEntries,
		LokiMaxAge:           settings.Loki.MaxAge,
		DurationBuckets:      settings.DurationBuckets,
		SizeBuckets:          settings.SizeBuckets,
	}
}

// mergeMetricsTokens keeps the stored token whenever the admin submits the mask
// back unchanged, and clears it when an empty value is submitted.
func mergeMetricsTokens(stored, submitted map[string]string) map[string]string {
	merged := make(map[string]string, 3)
	for _, service := range []string{MetricServicePrometheus, MetricServiceGrafana, MetricServiceLoki} {
		value, present := submitted[service]
		if !present || strings.TrimSpace(value) == tokenMask {
			merged[service] = stored[service]
			continue
		}
		merged[service] = strings.TrimSpace(value)
	}

	return merged
}

// metricTypeName maps the protobuf metric type onto the Prometheus type name.
func metricTypeName(metricType dto.MetricType) string {
	switch metricType {
	case dto.MetricType_COUNTER:
		return "counter"
	case dto.MetricType_GAUGE:
		return "gauge"
	case dto.MetricType_HISTOGRAM:
		return "histogram"
	case dto.MetricType_SUMMARY:
		return "summary"
	case dto.MetricType_UNTYPED:
		return "untyped"
	default:
		return "unknown"
	}
}

// metricValue extracts the scalar an admin cares about for one series: the
// sample value for counters and gauges, the observation count otherwise.
func metricValue(metricType dto.MetricType, metric *dto.Metric) float64 {
	switch metricType {
	case dto.MetricType_COUNTER:
		return metric.GetCounter().GetValue()
	case dto.MetricType_GAUGE:
		return metric.GetGauge().GetValue()
	case dto.MetricType_HISTOGRAM:
		return float64(metric.GetHistogram().GetSampleCount())
	case dto.MetricType_SUMMARY:
		return float64(metric.GetSummary().GetSampleCount())
	default:
		return metric.GetUntyped().GetValue()
	}
}
