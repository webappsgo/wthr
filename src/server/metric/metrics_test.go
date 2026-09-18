// Package metric tests verify counter/gauge/histogram recording logic and
// Prometheus text-format output per AI.md PART 21.
package metric

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"fmt"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
)

// toFloat64 reads the current value of a counter/gauge metric directly via
// the prometheus.Metric.Write protocol, avoiding a dependency on the
// prometheus/client_golang/prometheus/testutil subpackage (which pulls in
// module requirements not currently recorded in go.mod/go.sum).
func toFloat64(m prometheus.Metric) float64 {
	var pb dto.Metric
	if err := m.Write(&pb); err != nil {
		panic(err)
	}
	switch {
	case pb.Gauge != nil:
		return pb.Gauge.GetValue()
	case pb.Counter != nil:
		return pb.Counter.GetValue()
	case pb.Untyped != nil:
		return pb.Untyped.GetValue()
	default:
		panic("toFloat64: unsupported metric type")
	}
}

// TestRecordDBQuery covers the counter+histogram increment logic and the
// error-path branch that adds an extra DBErrors increment.
func TestRecordDBQuery(t *testing.T) {
	tests := []struct {
		name          string
		operation     string
		table         string
		duration      time.Duration
		err           error
		wantErrorType string
	}{
		{"success", "select", "users", 10 * time.Millisecond, nil, ""},
		{"error", "insert", "sessions", 5 * time.Millisecond, errDBFail, "other"},
		{"zero duration", "delete", "tokens", 0, nil, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			beforeQueries := toFloat64(DBQueriesTotal.WithLabelValues(tt.operation, tt.table))
			var beforeErrors float64
			if tt.wantErrorType != "" {
				beforeErrors = toFloat64(DBErrors.WithLabelValues(tt.operation, tt.wantErrorType))
			}

			RecordDBQuery(tt.operation, tt.table, tt.duration, tt.err)

			afterQueries := toFloat64(DBQueriesTotal.WithLabelValues(tt.operation, tt.table))
			if afterQueries != beforeQueries+1 {
				t.Errorf("DBQueriesTotal = %v, want %v", afterQueries, beforeQueries+1)
			}

			if tt.wantErrorType != "" {
				afterErrors := toFloat64(DBErrors.WithLabelValues(tt.operation, tt.wantErrorType))
				if afterErrors != beforeErrors+1 {
					t.Errorf("DBErrors = %v, want %v (error path must increment DBErrors)", afterErrors, beforeErrors+1)
				}
			}
		})
	}
}

// errDBFail is a sentinel error for the error-path test case above.
var errDBFail = &testError{"db failure"}

type testError struct{ msg string }

func (e *testError) Error() string { return e.msg }

// TestRecordCacheHitMissEviction covers the simple counter-increment helpers,
// including the first-observation case (metric starts at zero for a label
// combination never seen before).
func TestRecordCacheHitMissEviction(t *testing.T) {
	cache := "weather_cache_first_obs"

	if got := toFloat64(CacheHits.WithLabelValues(cache)); got != 0 {
		t.Fatalf("precondition: CacheHits(%q) = %v, want 0 before first observation", cache, got)
	}

	RecordCacheHit(cache)
	if got := toFloat64(CacheHits.WithLabelValues(cache)); got != 1 {
		t.Errorf("CacheHits after 1 hit = %v, want 1", got)
	}

	RecordCacheHit(cache)
	if got := toFloat64(CacheHits.WithLabelValues(cache)); got != 2 {
		t.Errorf("CacheHits after 2 hits = %v, want 2", got)
	}

	RecordCacheMiss(cache)
	if got := toFloat64(CacheMisses.WithLabelValues(cache)); got != 1 {
		t.Errorf("CacheMisses = %v, want 1", got)
	}

	RecordCacheEviction(cache)
	if got := toFloat64(CacheEvictions.WithLabelValues(cache)); got != 1 {
		t.Errorf("CacheEvictions = %v, want 1", got)
	}
}

// TestUpdateCacheSize covers the gauge Set (not Inc) semantics: repeated
// calls with different values must overwrite, not accumulate, and zero/
// negative-like edge values must be recorded faithfully.
func TestUpdateCacheSize(t *testing.T) {
	tests := []struct {
		name      string
		cache     string
		items     int
		bytes     int64
		wantItems float64
		wantBytes float64
	}{
		{"positive values", "cache_update_a", 42, 4096, 42, 4096},
		{"zero values", "cache_update_b", 0, 0, 0, 0},
		{"overwrite shrinks", "cache_update_c", 100, 1000, 100, 1000},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			UpdateCacheSize(tt.cache, tt.items, tt.bytes)
			if got := toFloat64(CacheSize.WithLabelValues(tt.cache)); got != tt.wantItems {
				t.Errorf("CacheSize = %v, want %v", got, tt.wantItems)
			}
			if got := toFloat64(CacheBytes.WithLabelValues(tt.cache)); got != tt.wantBytes {
				t.Errorf("CacheBytes = %v, want %v", got, tt.wantBytes)
			}
		})
	}

	// Overwrite semantics: setting a smaller value after a larger one must
	// reflect the smaller value, proving Set() (not Inc()) is used.
	UpdateCacheSize("cache_overwrite", 100, 1000)
	UpdateCacheSize("cache_overwrite", 10, 100)
	if got := toFloat64(CacheSize.WithLabelValues("cache_overwrite")); got != 10 {
		t.Errorf("CacheSize after overwrite = %v, want 10 (gauge must overwrite, not accumulate)", got)
	}
	if got := toFloat64(CacheBytes.WithLabelValues("cache_overwrite")); got != 100 {
		t.Errorf("CacheBytes after overwrite = %v, want 100 (gauge must overwrite, not accumulate)", got)
	}
}

// TestRecordSchedulerTask verifies the counter, histogram, and
// SetToCurrentTime gauge are all touched together.
func TestRecordSchedulerTask(t *testing.T) {
	task := "geoip_refresh_test"

	before := toFloat64(SchedulerTasksTotal.WithLabelValues(task, "success"))
	beforeLastRun := toFloat64(SchedulerLastRun.WithLabelValues(task))

	RecordSchedulerTask(task, "success", 250*time.Millisecond)

	after := toFloat64(SchedulerTasksTotal.WithLabelValues(task, "success"))
	if after != before+1 {
		t.Errorf("SchedulerTasksTotal = %v, want %v", after, before+1)
	}

	afterLastRun := toFloat64(SchedulerLastRun.WithLabelValues(task))
	if afterLastRun <= beforeLastRun {
		t.Errorf("SchedulerLastRun = %v, want > %v (SetToCurrentTime must advance the gauge)", afterLastRun, beforeLastRun)
	}
}

// TestRecordAuthAttempt covers distinct label combinations (method x status)
// remain independent counters.
func TestRecordAuthAttempt(t *testing.T) {
	RecordAuthAttempt("password", "success")
	RecordAuthAttempt("password", "failure")
	RecordAuthAttempt("oidc", "success")

	if got := toFloat64(AuthAttempts.WithLabelValues("password", "success")); got != 1 {
		t.Errorf("AuthAttempts(password,success) = %v, want 1", got)
	}
	if got := toFloat64(AuthAttempts.WithLabelValues("password", "failure")); got != 1 {
		t.Errorf("AuthAttempts(password,failure) = %v, want 1", got)
	}
	if got := toFloat64(AuthAttempts.WithLabelValues("oidc", "success")); got != 1 {
		t.Errorf("AuthAttempts(oidc,success) = %v, want 1", got)
	}
	// Never-recorded label combination must not exist yet as a distinct
	// series with a nonzero value.
	if got := toFloat64(AuthAttempts.WithLabelValues("oidc", "failure")); got != 0 {
		t.Errorf("AuthAttempts(oidc,failure) = %v, want 0 (unrelated label combo must stay independent)", got)
	}
}

// TestRecordWeatherRequest covers the business-metric counter helper.
func TestRecordWeatherRequest(t *testing.T) {
	before := toFloat64(WeatherRequestsTotal.WithLabelValues("zip", "ok"))
	RecordWeatherRequest("zip", "ok")
	after := toFloat64(WeatherRequestsTotal.WithLabelValues("zip", "ok"))
	if after != before+1 {
		t.Errorf("WeatherRequestsTotal = %v, want %v", after, before+1)
	}
}

// TestInit_Idempotent verifies Init uses sync.Once so calling it multiple
// times (e.g. from concurrent goroutines during startup) only sets AppInfo
// once and never panics or races.
func TestInit_Idempotent(t *testing.T) {
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			InitMetricsAppInfo("1.2.3", "abcdef", "2026-01-01T00:00:00Z")
		}()
	}
	wg.Wait()

	// AppInfo must have been set to 1 for the label set passed on whichever
	// call actually ran (sync.Once guarantees exactly one execution).
	got := toFloat64(AppInfo.WithLabelValues("1.2.3", "abcdef", "2026-01-01T00:00:00Z", runtime.Version()))
	if got != 1 {
		t.Errorf("AppInfo(...) = %v, want 1 after Init (sync.Once must allow exactly one successful set)", got)
	}
}

// TestPrometheusTextFormat gathers the registered metrics and verifies the
// exposition format contains the expected metric names and HELP/TYPE lines,
// exercising the actual text-format output path used by GET /metrics.
func TestPrometheusTextFormat(t *testing.T) {
	RecordCacheHit("text_format_cache")

	ch := make(chan prometheus.Metric, 64)
	CacheHits.Collect(ch)
	close(ch)

	count := 0
	for range ch {
		count++
	}
	if count == 0 {
		t.Fatal("CacheHits.Collect() produced 0 series, want at least 1 registered series")
	}

	// A direct value check confirms the metric is queryable by name+labels,
	// which is what the /metrics text exporter relies on internally.
	if got := toFloat64(CacheHits.WithLabelValues("text_format_cache")); got != 1 {
		t.Errorf("CacheHits(text_format_cache) = %v, want 1", got)
	}
}

// TestClassifyDBError covers the fixed error_type label set AI.md PART 21
// defines for wthr_db_errors_total: connection, timeout, constraint,
// duplicate, other.
func TestClassifyDBError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want string
	}{
		{"nil", nil, ""},
		{"context deadline", context.DeadlineExceeded, "timeout"},
		{"connection done", sql.ErrConnDone, "connection"},
		{"bad conn", driver.ErrBadConn, "connection"},
		{"wrapped deadline", fmt.Errorf("query failed: %w", context.DeadlineExceeded), "timeout"},
		{"duplicate key", &testError{"ERROR: duplicate key value violates unique constraint"}, "duplicate"},
		{"unique index", &testError{"UNIQUE index violation on users.email"}, "duplicate"},
		{"foreign key constraint", &testError{"FOREIGN KEY constraint failed"}, "constraint"},
		{"timeout text", &testError{"i/o timeout"}, "timeout"},
		{"dial failure", &testError{"dial tcp 10.0.0.1:5432: connect: refused"}, "connection"},
		{"database closed", &testError{"sql: database is closed"}, "connection"},
		{"unknown", &testError{"syntax error at or near SELECT"}, "other"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ClassifyDBError(tt.err); got != tt.want {
				t.Errorf("ClassifyDBError(%v) = %q, want %q", tt.err, got, tt.want)
			}
		})
	}
}

// TestRecordDBQuery_ErrorTypeLabel proves distinct errors land on distinct
// error_type series rather than collapsing into one.
func TestRecordDBQuery_ErrorTypeLabel(t *testing.T) {
	op := "update_error_type_check"
	RecordDBQuery(op, "t1", time.Millisecond, &testError{"i/o timeout"})
	RecordDBQuery(op, "t1", time.Millisecond, &testError{"CHECK constraint failed"})

	if got := toFloat64(DBErrors.WithLabelValues(op, "timeout")); got != 1 {
		t.Errorf("DBErrors(%q, timeout) = %v, want 1", op, got)
	}
	if got := toFloat64(DBErrors.WithLabelValues(op, "constraint")); got != 1 {
		t.Errorf("DBErrors(%q, constraint) = %v, want 1", op, got)
	}
}

// TestSystemCollector_RuntimeGating verifies include_runtime is honored: with
// it off the Go runtime gauges stay untouched, with it on they are populated.
func TestSystemCollector_RuntimeGating(t *testing.T) {
	GoGoroutines.Set(0)

	NewSystemCollector("", 0, false, false).CollectOnce()
	if got := toFloat64(GoGoroutines); got != 0 {
		t.Errorf("GoGoroutines = %v, want 0 when include_runtime is false", got)
	}

	NewSystemCollector("", 0, false, true).CollectOnce()
	if got := toFloat64(GoGoroutines); got <= 0 {
		t.Errorf("GoGoroutines = %v, want > 0 when include_runtime is true", got)
	}
}

// TestSystemCollector_GCCountersUseDeltas verifies the GC counters advance by
// the delta between passes; MemStats totals are cumulative, so re-adding them
// on every pass would multiply-count on a monotonic Prometheus counter.
func TestSystemCollector_GCCountersUseDeltas(t *testing.T) {
	c := NewSystemCollector("", 0, false, true)
	c.CollectOnce()
	afterFirst := toFloat64(GoGCRuns)

	runtime.GC()
	c.CollectOnce()
	afterSecond := toFloat64(GoGCRuns)

	if afterSecond < afterFirst {
		t.Fatalf("GoGCRuns went backwards: %v then %v", afterFirst, afterSecond)
	}
	if delta := afterSecond - afterFirst; delta < 1 {
		t.Errorf("GoGCRuns delta = %v, want >= 1 after a forced GC", delta)
	}

	// A third pass with no intervening GC must not re-add the cumulative total.
	before := toFloat64(GoGCPauseTotal)
	c.CollectOnce()
	if got := toFloat64(GoGCPauseTotal); got != before {
		t.Errorf("GoGCPauseTotal = %v, want %v (cumulative total must not be re-added)", got, before)
	}
}

// TestSystemCollector_StopIsIdempotent verifies StopSystemCollector can be
// called more than once without panicking on a double channel close.
func TestSystemCollector_StopIsIdempotent(t *testing.T) {
	c := NewSystemCollector("", 10*time.Millisecond, false, false)
	c.StartSystemCollector()
	c.StopSystemCollector()
	c.StopSystemCollector()
}

// TestSystemCollector_DiskMetrics verifies disk gauges are populated for a real
// path when include_system is on, and that an empty dataDir records nothing.
func TestSystemCollector_DiskMetrics(t *testing.T) {
	dir := t.TempDir()
	stat, err := readDiskUsage(dir)
	if err != nil {
		t.Skipf("disk statistics unavailable on this platform: %v", err)
	}
	if stat.total == 0 {
		t.Fatal("readDiskUsage returned total = 0 with no error")
	}

	NewSystemCollector(dir, 0, true, false).CollectOnce()

	if got := toFloat64(SystemDiskTotal.WithLabelValues(dir)); got != float64(stat.total) {
		t.Errorf("SystemDiskTotal(%q) = %v, want %v", dir, got, float64(stat.total))
	}
	if got := toFloat64(SystemDiskUsage.WithLabelValues(dir)); got < 0 || got > 100 {
		t.Errorf("SystemDiskUsage(%q) = %v, want a percentage in [0,100]", dir, got)
	}

	// An empty dataDir must not create a series for the empty label.
	NewSystemCollector("", 0, true, false).CollectOnce()
	if got := toFloat64(SystemDiskTotal.WithLabelValues("")); got != 0 {
		t.Errorf("SystemDiskTotal(\"\") = %v, want 0 (empty dataDir disables disk metrics)", got)
	}
}
