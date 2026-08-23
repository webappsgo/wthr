package handler

import (
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
)

func newHealthAdminTestContext(target string) (*http.Request, *httptest.ResponseRecorder) {
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, target, nil)
	return r, w
}

// readVersion returns the build-time-injected package Version, which defaults
// to "dev" when no ldflags value was set — always the case under `go test`.
func TestReadVersion(t *testing.T) {
	got := readVersion()
	if got != "dev" {
		t.Errorf("readVersion() = %q, want %q (Version defaults to dev without ldflags)", got, "dev")
	}
}

// getDataDir/getLogDir/SetDirectoryPaths share the same
// global-override-then-env-fallback-then-default pattern; SetDirectoryPaths
// mutates package-level state so each subtest resets it afterward.
func TestDirectoryPaths(t *testing.T) {
	t.Run("global override wins", func(t *testing.T) {
		SetDirectoryPaths("/custom/data", "/custom/log")
		defer SetDirectoryPaths("", "")

		if got := getDataDir(); got != "/custom/data" {
			t.Errorf("getDataDir() = %q, want %q", got, "/custom/data")
		}
		if got := getLogDir(); got != "/custom/log" {
			t.Errorf("getLogDir() = %q, want %q", got, "/custom/log")
		}
	})

	t.Run("env var fallback when no global override", func(t *testing.T) {
		SetDirectoryPaths("", "")
		defer SetDirectoryPaths("", "")

		t.Setenv("DATA_DIR", "/env/data")
		t.Setenv("LOG_DIR", "/env/log")

		if got := getDataDir(); got != "/env/data" {
			t.Errorf("getDataDir() = %q, want %q", got, "/env/data")
		}
		if got := getLogDir(); got != "/env/log" {
			t.Errorf("getLogDir() = %q, want %q", got, "/env/log")
		}
	})

	t.Run("hardcoded default when neither override nor env set", func(t *testing.T) {
		SetDirectoryPaths("", "")
		defer SetDirectoryPaths("", "")

		os.Unsetenv("DATA_DIR")
		os.Unsetenv("LOG_DIR")

		if got := getDataDir(); got != "./data" {
			t.Errorf("getDataDir() = %q, want %q", got, "./data")
		}
		if got := getLogDir(); got != "./logs" {
			t.Errorf("getLogDir() = %q, want %q", got, "./logs")
		}
	})
}

// getSSLStatus is nil-safe and falls back to type-asserting a local
// certInfoGetter interface, so it is fully testable without a real SSL
// manager by passing nil, an unrelated type, or a fake implementing the
// GetCertInfo() method.
type fakeCertInfoGetter struct{}

func (fakeCertInfoGetter) GetCertInfo() map[string]interface{} {
	return map[string]interface{}{
		"enabled":        true,
		"status":         "active",
		"expires_at":     "2027-01-01T00:00:00Z",
		"days_remaining": 180,
		"issuer":         "Let's Encrypt",
	}
}

func TestGetSSLStatus(t *testing.T) {
	t.Run("nil manager returns disabled defaults", func(t *testing.T) {
		got := getSSLStatus(nil)
		if got["enabled"] != false {
			t.Errorf("enabled = %v, want false", got["enabled"])
		}
		if got["status"] != "none" {
			t.Errorf("status = %v, want %q", got["status"], "none")
		}
	})

	t.Run("manager not implementing certInfoGetter falls back to disabled defaults", func(t *testing.T) {
		got := getSSLStatus("not a cert manager")
		if got["enabled"] != false {
			t.Errorf("enabled = %v, want false", got["enabled"])
		}
		if got["issuer"] != "Unknown" {
			t.Errorf("issuer = %v, want %q", got["issuer"], "Unknown")
		}
	})

	t.Run("manager implementing certInfoGetter returns its info", func(t *testing.T) {
		got := getSSLStatus(fakeCertInfoGetter{})
		if got["enabled"] != true {
			t.Errorf("enabled = %v, want true", got["enabled"])
		}
		if got["issuer"] != "Let's Encrypt" {
			t.Errorf("issuer = %v, want %q", got["issuer"], "Let's Encrypt")
		}
	})
}

// getSchedulerStatus and getRequestStats both call database.GetServerDB(),
// which returns nil in this test binary since no server DB is initialized
// here, so both exercise their nil-db "unavailable" branches.
func TestGetSchedulerStatusNilDB(t *testing.T) {
	got := getSchedulerStatus()
	if got["status"] != "unknown" {
		t.Errorf("status = %v, want %q", got["status"], "unknown")
	}
	if got["tasks_total"] != 0 {
		t.Errorf("tasks_total = %v, want 0", got["tasks_total"])
	}
	if got["tasks_enabled"] != 0 {
		t.Errorf("tasks_enabled = %v, want 0", got["tasks_enabled"])
	}
	if _, ok := got["next_run"]; !ok {
		t.Error("expected next_run key to be present")
	}
}

func TestGetRequestStatsNilDB(t *testing.T) {
	got := getRequestStats()
	if got["total_today"] != 0 {
		t.Errorf("total_today = %v, want 0", got["total_today"])
	}
	if got["errors_today"] != 0 {
		t.Errorf("errors_today = %v, want 0", got["errors_today"])
	}
	if got["error_rate"] != 0.0 {
		t.Errorf("error_rate = %v, want 0.0", got["error_rate"])
	}
	if got["source"] != "unavailable" {
		t.Errorf("source = %v, want %q", got["source"], "unavailable")
	}
}

// getServerInfo is nil-safe for its sslManager argument (only used to
// probe an optional httpsChecker interface) and only reads httpPort/
// httpsPort plus request/host info off the *http.Request.
type fakeHTTPSChecker struct{ enabled bool }

func (f fakeHTTPSChecker) IsHTTPSEnabled() bool { return f.enabled }

func TestGetServerInfo(t *testing.T) {
	t.Run("nil ssl manager reports https disabled", func(t *testing.T) {
		r, _ := newHealthAdminTestContext("/healthz")
		got := getServerInfo(r, "8080", 8443, nil)

		if got["http_port"] != "8080" {
			t.Errorf("http_port = %v, want %q", got["http_port"], "8080")
		}
		if got["https_port"] != 8443 {
			t.Errorf("https_port = %v, want %v", got["https_port"], 8443)
		}
		if got["https_enabled"] != false {
			t.Errorf("https_enabled = %v, want false", got["https_enabled"])
		}
		if _, ok := got["pid"]; !ok {
			t.Error("expected pid key to be present")
		}
		if _, ok := got["started_at"]; !ok {
			t.Error("expected started_at key to be present")
		}
	})

	t.Run("manager implementing httpsChecker reports its value", func(t *testing.T) {
		r, _ := newHealthAdminTestContext("/healthz")
		got := getServerInfo(r, "8080", 8443, fakeHTTPSChecker{enabled: true})

		if got["https_enabled"] != true {
			t.Errorf("https_enabled = %v, want true", got["https_enabled"])
		}
	})

	t.Run("manager not implementing httpsChecker reports https disabled", func(t *testing.T) {
		r, _ := newHealthAdminTestContext("/healthz")
		got := getServerInfo(r, "8080", 8443, "not a checker")

		if got["https_enabled"] != false {
			t.Errorf("https_enabled = %v, want false", got["https_enabled"])
		}
	})
}
