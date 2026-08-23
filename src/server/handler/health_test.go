package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/webappsgo/wthr/src/database"
)

// resetInitStatus restores package-level init state to "nothing initialized"
// before/after each test that touches it, since initStatus is shared
// mutable state across the whole handler package's test run.
func resetInitStatus(t *testing.T) {
	t.Helper()
	SetInitStatus(false, false, false)
	t.Cleanup(func() { SetInitStatus(false, false, false) })
}

// TestSetInitStatus_GetInitStatus_IsInitialized covers the round trip of the
// package-level init-status globals, including the "partially initialized"
// states that must NOT count as ready.
func TestSetInitStatus_GetInitStatus_IsInitialized(t *testing.T) {
	resetInitStatus(t)

	if IsInitialized() {
		t.Fatal("IsInitialized() = true before any SetInitStatus call, want false")
	}

	tests := []struct {
		name                       string
		countries, cities, weather bool
		wantInitialized            bool
	}{
		{"all false", false, false, false, false},
		{"only countries", true, false, false, false},
		{"only countries+cities", true, true, false, false},
		{"all true", true, true, true, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			SetInitStatus(tt.countries, tt.cities, tt.weather)
			if got := IsInitialized(); got != tt.wantInitialized {
				t.Errorf("IsInitialized() = %v, want %v", got, tt.wantInitialized)
			}
			status := GetInitStatus()
			if status.Countries != tt.countries || status.Cities != tt.cities || status.Weather != tt.weather {
				t.Errorf("GetInitStatus() = %+v, want Countries=%v Cities=%v Weather=%v", status, tt.countries, tt.cities, tt.weather)
			}
		})
	}
}

// TestGetTorStatus_NoProvider verifies the zero-value/unset-provider path
// returns false/"" instead of panicking on a nil torStatusGetter.
func TestGetTorStatus_NoProvider(t *testing.T) {
	SetTorStatusProvider(nil)
	running, addr := GetTorStatus()
	if running || addr != "" {
		t.Errorf("GetTorStatus() = (%v, %q), want (false, \"\") when no provider set", running, addr)
	}
}

type fakeTorProvider struct {
	running bool
	onion   string
}

func (f fakeTorProvider) IsRunning() bool         { return f.running }
func (f fakeTorProvider) GetOnionAddress() string { return f.onion }

// TestGetTorStatus_WithProvider confirms the provider's values are passed
// through unmodified.
func TestGetTorStatus_WithProvider(t *testing.T) {
	SetTorStatusProvider(fakeTorProvider{running: true, onion: "abc123.onion"})
	t.Cleanup(func() { SetTorStatusProvider(nil) })

	running, addr := GetTorStatus()
	if !running || addr != "abc123.onion" {
		t.Errorf("GetTorStatus() = (%v, %q), want (true, %q)", running, addr, "abc123.onion")
	}
}

// TestLivenessCheck confirms the liveness probe is unconditional: it must
// always return 200/alive regardless of init state, since Kubernetes uses
// this endpoint to decide whether to kill the container.
func TestLivenessCheck(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()
	LivenessCheck(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	var body map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if body["status"] != "alive" {
		t.Errorf("status field = %q, want %q", body["status"], "alive")
	}
}

// TestReadinessCheck covers the three-way branch: not-initialized,
// initialized-but-db-down, and fully ready. This is the endpoint that gates
// production traffic, so each branch returning the wrong status code would
// mean either dropping traffic that should be served, or routing traffic to
// a broken instance.
func TestReadinessCheck(t *testing.T) {
	t.Run("not initialized returns 503 initializing", func(t *testing.T) {
		resetInitStatus(t)
		db := newTestDatabaseDB(t)
		r := httptest.NewRequest(http.MethodGet, "/health/ready", nil)
		w := httptest.NewRecorder()

		ReadinessCheck(db, time.Now())(w, r)

		if w.Code != http.StatusServiceUnavailable {
			t.Fatalf("status = %d, want 503", w.Code)
		}
		var body map[string]string
		if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if body["reason"] != "initializing" {
			t.Errorf("reason = %q, want %q", body["reason"], "initializing")
		}
	})

	t.Run("initialized but database unreachable returns 503 database_unavailable", func(t *testing.T) {
		resetInitStatus(t)
		SetInitStatus(true, true, true)
		db := newTestDatabaseDB(t)
		db.DB.Close() // force HealthCheck's "SELECT 1" to fail
		r := httptest.NewRequest(http.MethodGet, "/health/ready", nil)
		w := httptest.NewRecorder()

		ReadinessCheck(db, time.Now())(w, r)

		if w.Code != http.StatusServiceUnavailable {
			t.Fatalf("status = %d, want 503", w.Code)
		}
		var body map[string]string
		if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if body["reason"] != "database_unavailable" {
			t.Errorf("reason = %q, want %q", body["reason"], "database_unavailable")
		}
	})

	t.Run("initialized and database healthy returns 200 ready", func(t *testing.T) {
		resetInitStatus(t)
		SetInitStatus(true, true, true)
		db := newTestDatabaseDB(t)
		r := httptest.NewRequest(http.MethodGet, "/health/ready", nil)
		w := httptest.NewRecorder()

		ReadinessCheck(db, time.Now().Add(-90*time.Second))(w, r)

		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
		}
		var body map[string]string
		if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if body["status"] != "ready" {
			t.Errorf("status field = %q, want %q", body["status"], "ready")
		}
		if body["uptime"] == "" {
			t.Errorf("uptime field is empty, want a formatted duration")
		}
	})
}

// TestDebugInfo confirms the debug endpoint always returns 200 with the
// expected top-level sections, and does not panic when init state is zero.
func TestDebugInfo(t *testing.T) {
	resetInitStatus(t)
	r := httptest.NewRequest(http.MethodGet, "/debug/info", nil)
	w := httptest.NewRecorder()

	DebugInfo(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	var body map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	for _, key := range []string{"service", "initialization", "runtime", "memory", "timestamp"} {
		if _, ok := body[key]; !ok {
			t.Errorf("body missing key %q; body=%v", key, body)
		}
	}
}

// TestFormatUptime covers the day/hour/minute rollover boundaries, since an
// off-by-one in the modulo math would silently show the wrong uptime on
// every health page for the life of the process.
func TestFormatUptime(t *testing.T) {
	tests := []struct {
		name string
		d    time.Duration
		want string
	}{
		{"zero", 0, "0m"},
		{"minutes only", 45 * time.Minute, "45m"},
		{"exact hour", 1 * time.Hour, "1h 0m"},
		{"hours and minutes", 2*time.Hour + 30*time.Minute, "2h 30m"},
		{"exact day", 24 * time.Hour, "1d 0h 0m"},
		{"days hours minutes", 26*time.Hour + 5*time.Minute, "1d 2h 5m"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := formatUptime(tt.d); got != tt.want {
				t.Errorf("formatUptime(%v) = %q, want %q", tt.d, got, tt.want)
			}
		})
	}
}

// TestCheckmarkAndContains covers the tiny pure helpers used by
// ServeLoadingPage's CLI-friendly output.
func TestCheckmarkAndContains(t *testing.T) {
	if got := checkmark(true); got != "✓" {
		t.Errorf("checkmark(true) = %q, want checkmark", got)
	}
	if got := checkmark(false); got != "⋯" {
		t.Errorf("checkmark(false) = %q, want ellipsis", got)
	}

	tests := []struct {
		s, substr string
		want      bool
	}{
		{"curl/8.0", "curl", true},
		{"Mozilla/5.0", "curl", false},
		{"", "curl", false},
		{"curl", "", true}, // empty substr trivially contained
		{"cur", "curl", false},
	}
	for _, tt := range tests {
		if got := contains(tt.s, tt.substr); got != tt.want {
			t.Errorf("contains(%q, %q) = %v, want %v", tt.s, tt.substr, got, tt.want)
		}
	}
}

// TestAPIHealthCheck exercises the full buildPublicHealthResponse pipeline
// against a real in-memory ServerSchema database, covering both the healthy
// and not-initialized (unhealthy/503) branches end to end.
//
// NOTE: buildPublicHealthResponse's getPublicGeoIPStatus/getMaintenanceMode
// helpers build a models.SettingsModel{DB: db.DB} but SettingsModel.Get
// (src/server/model/settings.go:35) ignores that injected DB field and
// queries database.GetServerDB() (the package-level global) instead. So the
// global dual-DB must be wired via setGlobalTestDualDB for these calls to
// reach a live connection instead of nil-pointer-panicking. This mirrors how
// the real server wires the global at startup; it is test setup, not a
// workaround for the underlying bug (see final report).
func TestAPIHealthCheck(t *testing.T) {
	t.Run("healthy when initialized and db reachable", func(t *testing.T) {
		resetInitStatus(t)
		SetInitStatus(true, true, true)
		serverDB := newTestServerDB(t)
		setGlobalTestDualDB(t, serverDB, nil)
		db := &database.DB{DB: serverDB}
		r := httptest.NewRequest(http.MethodGet, "/api/v1/healthz", nil)
		w := httptest.NewRecorder()

		APIHealthCheck(db, time.Now())(w, r)

		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
		}
		var body map[string]interface{}
		if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
			t.Fatalf("unmarshal: %v; body=%s", err, w.Body.String())
		}
		// Overall "status" also folds in disk/scheduler checks that read real
		// host state (disk usage, scheduler goroutine) unrelated to the DB
		// path under test here and are "degraded" in this sandboxed
		// container, so assert the DB-specific check instead of the
		// aggregated status field.
		checks, _ := body["checks"].(map[string]interface{})
		if checks["database"] != "ok" {
			t.Errorf("checks.database = %v, want ok; body=%v", checks["database"], body)
		}
		if body["status"] == "unhealthy" || body["status"] == "maintenance" {
			t.Errorf("status = %v, want healthy or degraded (not unhealthy/maintenance); body=%v", body["status"], body)
		}
	})

	t.Run("unhealthy 503 when not initialized", func(t *testing.T) {
		resetInitStatus(t)
		serverDB := newTestServerDB(t)
		setGlobalTestDualDB(t, serverDB, nil)
		db := &database.DB{DB: serverDB}
		r := httptest.NewRequest(http.MethodGet, "/api/v1/healthz", nil)
		w := httptest.NewRecorder()

		APIHealthCheck(db, time.Now())(w, r)

		if w.Code != http.StatusServiceUnavailable {
			t.Fatalf("status = %d, want 503; body=%s", w.Code, w.Body.String())
		}
		var body map[string]interface{}
		if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if body["status"] != "unhealthy" {
			t.Errorf("status = %v, want unhealthy", body["status"])
		}
	})

	t.Run("does not panic when the underlying db is closed", func(t *testing.T) {
		resetInitStatus(t)
		SetInitStatus(true, true, true)
		serverDB := newTestServerDB(t)
		setGlobalTestDualDB(t, serverDB, nil)
		db := &database.DB{DB: serverDB}
		db.DB.Close()
		r := httptest.NewRequest(http.MethodGet, "/api/v1/healthz", nil)
		w := httptest.NewRecorder()

		APIHealthCheck(db, time.Now())(w, r)

		if w.Code != http.StatusServiceUnavailable {
			t.Errorf("status = %d, want 503 when db is closed", w.Code)
		}
	})
}

// TestServeLoadingPage_JSON confirms the API-negotiated branch of the
// loading page (503 + JSON body) since that's the branch other services
// polling readiness during startup will hit.
func TestServeLoadingPage_JSON(t *testing.T) {
	resetInitStatus(t)
	r := httptest.NewRequest(http.MethodGet, "/api/v1/loading", nil)
	r.Header.Set("Accept", "application/json")
	w := httptest.NewRecorder()

	ServeLoadingPage(w, r)

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", w.Code)
	}
	if !strings.Contains(w.Body.String(), "Initializing") {
		t.Errorf("body = %q, want it to contain %q", w.Body.String(), "Initializing")
	}
}

// TestServeLoadingPage_CLI confirms curl/wget-style clients get the
// plain-text ASCII branch rather than HTML (which would require a
// registered template and isn't what a terminal client wants anyway).
func TestServeLoadingPage_CLI(t *testing.T) {
	resetInitStatus(t)
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.Header.Set("User-Agent", "curl/8.0")
	w := httptest.NewRecorder()

	ServeLoadingPage(w, r)

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", w.Code)
	}
	if !strings.Contains(w.Body.String(), "Starting Up") {
		t.Errorf("body = %q, want the CLI ASCII banner", w.Body.String())
	}
}
