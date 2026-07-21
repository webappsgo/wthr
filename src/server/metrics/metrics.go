// Package metrics provides Prometheus-compatible metrics for monitoring
// per AI.md PART 21: METRICS (NON-NEGOTIABLE)
package metrics

import (
	"runtime"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	// HTTP metrics
	HTTPRequestsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "wthr_http_requests_total",
			Help: "Total number of HTTP requests",
		},
		[]string{"method", "path", "status"},
	)

	HTTPRequestDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "wthr_http_request_duration_seconds",
			Help:    "HTTP request duration in seconds",
			Buckets: []float64{0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10},
		},
		[]string{"method", "path"},
	)

	HTTPRequestSize = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "wthr_http_request_size_bytes",
			Help:    "HTTP request size in bytes",
			Buckets: []float64{100, 1000, 10000, 100000, 1000000, 10000000},
		},
		[]string{"method", "path"},
	)

	HTTPResponseSize = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "wthr_http_response_size_bytes",
			Help:    "HTTP response size in bytes",
			Buckets: []float64{100, 1000, 10000, 100000, 1000000, 10000000},
		},
		[]string{"method", "path"},
	)

	HTTPActiveRequests = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "wthr_http_active_requests",
			Help: "Number of active HTTP requests",
		},
	)

	// Database metrics
	DBQueriesTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "wthr_db_queries_total",
			Help: "Total number of database queries",
		},
		[]string{"operation", "table"},
	)

	DBQueryDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "wthr_db_query_duration_seconds",
			Help:    "Database query duration in seconds",
			Buckets: []float64{0.0001, 0.0005, 0.001, 0.005, 0.01, 0.05, 0.1, 0.5, 1},
		},
		[]string{"operation", "table"},
	)

	DBConnectionsOpen = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "wthr_db_connections_open",
			Help: "Number of open database connections",
		},
	)

	DBConnectionsInUse = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "wthr_db_connections_in_use",
			Help: "Number of database connections in use",
		},
	)

	DBErrors = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "wthr_db_errors_total",
			Help: "Total number of database errors",
		},
		[]string{"operation", "error_type"},
	)

	// Cache metrics
	CacheHits = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "wthr_cache_hits_total",
			Help: "Total number of cache hits",
		},
		[]string{"cache"},
	)

	CacheMisses = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "wthr_cache_misses_total",
			Help: "Total number of cache misses",
		},
		[]string{"cache"},
	)

	CacheEvictions = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "wthr_cache_evictions_total",
			Help: "Total number of cache evictions",
		},
		[]string{"cache"},
	)

	CacheSize = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "wthr_cache_size",
			Help: "Current cache size (items)",
		},
		[]string{"cache"},
	)

	CacheBytes = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "wthr_cache_bytes",
			Help: "Current cache size (bytes)",
		},
		[]string{"cache"},
	)

	// Scheduler metrics
	SchedulerTasksTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "wthr_scheduler_tasks_total",
			Help: "Total number of scheduled tasks executed",
		},
		[]string{"task", "status"},
	)

	SchedulerTaskDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "wthr_scheduler_task_duration_seconds",
			Help:    "Scheduled task duration in seconds",
			Buckets: []float64{0.1, 0.5, 1, 5, 10, 30, 60, 300, 600},
		},
		[]string{"task"},
	)

	SchedulerTasksRunning = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "wthr_scheduler_tasks_running",
			Help: "Number of currently running scheduled tasks",
		},
		[]string{"task"},
	)

	SchedulerLastRun = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "wthr_scheduler_last_run_timestamp",
			Help: "Timestamp of last task run",
		},
		[]string{"task"},
	)

	// Authentication metrics
	AuthAttempts = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "wthr_auth_attempts_total",
			Help: "Total authentication attempts",
		},
		[]string{"method", "status"},
	)

	AuthSessionsActive = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "wthr_auth_sessions_active",
			Help: "Number of active sessions",
		},
	)

	// Business metrics
	UsersTotal = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "wthr_users_total",
			Help: "Total number of registered users",
		},
	)

	UsersActive = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "wthr_users_active",
			Help: "Number of users active in last 24 hours",
		},
	)

	APITokensActive = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "wthr_api_tokens_active",
			Help: "Number of active API tokens",
		},
	)

	// Application info
	AppInfo = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "wthr_app_info",
			Help: "Application information",
		},
		[]string{"version", "commit", "build_date", "go_version"},
	)

	AppUptime = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "wthr_app_uptime_seconds",
			Help: "Application uptime in seconds",
		},
	)

	AppStartTime = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "wthr_app_start_timestamp",
			Help: "Application start timestamp",
		},
	)

	// System metrics
	SystemMemoryUsed = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "wthr_system_memory_used_bytes",
			Help: "System memory used in bytes",
		},
	)

	SystemMemoryTotal = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "wthr_system_memory_total_bytes",
			Help: "System memory total in bytes",
		},
	)

	SystemGoroutines = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "wthr_system_goroutines",
			Help: "Number of goroutines",
		},
	)

	SystemGCPauseTotal = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "wthr_system_gc_pause_total_seconds",
			Help: "Total GC pause time in seconds",
		},
	)

	SystemGCRuns = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "wthr_system_gc_runs_total",
			Help: "Total number of GC runs",
		},
	)

	// Weather-specific business metrics
	WeatherRequestsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "wthr_api_weather_requests_total",
			Help: "Total weather API requests by location type",
		},
		[]string{"location_type", "status"},
	)

	AlertsActiveTotal = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "wthr_alerts_active_total",
			Help: "Number of active weather alerts",
		},
	)

	EarthquakesTracked = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "wthr_earthquakes_tracked",
			Help: "Number of earthquakes currently tracked",
		},
	)

	HurricanesTracked = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "wthr_hurricanes_tracked",
			Help: "Number of hurricanes currently tracked",
		},
	)
)

var (
	initOnce      sync.Once
	startTime     time.Time
	lastGCRuns    uint32
	lastGCPauseNs uint64
)

// Init initializes application info metrics
func Init(version, commit, buildDate string) {
	initOnce.Do(func() {
		startTime = time.Now()
		AppInfo.WithLabelValues(version, commit, buildDate, runtime.Version()).Set(1)
		AppStartTime.SetToCurrentTime()

		// Start background goroutine to update uptime and system metrics
		go updateMetrics()
	})
}

// updateMetrics periodically updates uptime and system metrics
func updateMetrics() {
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		// Update uptime
		AppUptime.Set(time.Since(startTime).Seconds())

		// Update system metrics
		var m runtime.MemStats
		runtime.ReadMemStats(&m)

		SystemMemoryUsed.Set(float64(m.Alloc))
		SystemMemoryTotal.Set(float64(m.Sys))
		SystemGoroutines.Set(float64(runtime.NumGoroutine()))

		// Update GC metrics (only count new runs/pause)
		if m.NumGC > lastGCRuns {
			SystemGCRuns.Add(float64(m.NumGC - lastGCRuns))
			lastGCRuns = m.NumGC
		}
		if m.PauseTotalNs > lastGCPauseNs {
			SystemGCPauseTotal.Add(float64(m.PauseTotalNs-lastGCPauseNs) / 1e9)
			lastGCPauseNs = m.PauseTotalNs
		}
	}
}

// RecordDBQuery records database query metrics
func RecordDBQuery(operation, table string, duration time.Duration, err error) {
	DBQueriesTotal.WithLabelValues(operation, table).Inc()
	DBQueryDuration.WithLabelValues(operation, table).Observe(duration.Seconds())
	if err != nil {
		DBErrors.WithLabelValues(operation, "query_error").Inc()
	}
}

// RecordCacheHit records a cache hit
func RecordCacheHit(cache string) {
	CacheHits.WithLabelValues(cache).Inc()
}

// RecordCacheMiss records a cache miss
func RecordCacheMiss(cache string) {
	CacheMisses.WithLabelValues(cache).Inc()
}

// RecordCacheEviction records a cache eviction
func RecordCacheEviction(cache string) {
	CacheEvictions.WithLabelValues(cache).Inc()
}

// UpdateCacheSize updates cache size metrics
func UpdateCacheSize(cache string, items int, bytes int64) {
	CacheSize.WithLabelValues(cache).Set(float64(items))
	CacheBytes.WithLabelValues(cache).Set(float64(bytes))
}

// RecordSchedulerTask records scheduler task execution
func RecordSchedulerTask(task, status string, duration time.Duration) {
	SchedulerTasksTotal.WithLabelValues(task, status).Inc()
	SchedulerTaskDuration.WithLabelValues(task).Observe(duration.Seconds())
	SchedulerLastRun.WithLabelValues(task).SetToCurrentTime()
}

// RecordAuthAttempt records an authentication attempt
func RecordAuthAttempt(method, status string) {
	AuthAttempts.WithLabelValues(method, status).Inc()
}

// RecordWeatherRequest records a weather API request
func RecordWeatherRequest(locationType, status string) {
	WeatherRequestsTotal.WithLabelValues(locationType, status).Inc()
}
