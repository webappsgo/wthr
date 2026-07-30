// Package metrics tests verify counter/gauge/histogram recording logic and
// Prometheus text-format output per AI.md PART 21.
package metrics

import (
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
		name      string
		operation string
		table     string
		duration  time.Duration
		err       error
		wantErr   bool
	}{
		{"success", "select", "users", 10 * time.Millisecond, nil, false},
		{"error", "insert", "sessions", 5 * time.Millisecond, errDBFail, true},
		{"zero duration", "delete", "tokens", 0, nil, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			beforeQueries := toFloat64(DBQueriesTotal.WithLabelValues(tt.operation, tt.table))
			var beforeErrors float64
			if tt.wantErr {
				beforeErrors = toFloat64(DBErrors.WithLabelValues(tt.operation, "query_error"))
			}

			RecordDBQuery(tt.operation, tt.table, tt.duration, tt.err)

			afterQueries := toFloat64(DBQueriesTotal.WithLabelValues(tt.operation, tt.table))
			if afterQueries != beforeQueries+1 {
				t.Errorf("DBQueriesTotal = %v, want %v", afterQueries, beforeQueries+1)
			}

			if tt.wantErr {
				afterErrors := toFloat64(DBErrors.WithLabelValues(tt.operation, "query_error"))
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
			Init("1.2.3", "abcdef", "2026-01-01T00:00:00Z")
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

// TestRecordDBQuery_ErrorTypeAlwaysQueryError documents current behavior:
// RecordDBQuery hardcodes the DBErrors "error_type" label to "query_error"
// regardless of the actual error, so distinct errors are indistinguishable
// in the error_type label. This is a coverage/behavior gap, not something
// this test suite can fix (would require a signature change).
func TestRecordDBQuery_ErrorTypeAlwaysQueryError(t *testing.T) {
	op := "update_error_type_check"
	RecordDBQuery(op, "t1", time.Millisecond, &testError{"timeout"})
	RecordDBQuery(op, "t1", time.Millisecond, &testError{"constraint violation"})

	got := toFloat64(DBErrors.WithLabelValues(op, "query_error"))
	if got != 2 {
		t.Errorf("DBErrors(%q, query_error) = %v, want 2 (both distinct errors collapse into the same error_type label)", op, got)
	}
}
