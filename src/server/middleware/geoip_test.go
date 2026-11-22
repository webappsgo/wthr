package middleware

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/webappsgo/wthr/src/config"
)

func withGeoIPConfig(t *testing.T, cfg *config.AppConfig) {
	t.Helper()
	previous := config.GetGlobalConfig()
	config.SetGlobalConfig(cfg)
	t.Cleanup(func() { config.SetGlobalConfig(previous) })
}

func TestGeoIPCountryBlockerAppliesDenyPolicy(t *testing.T) {
	cfg := &config.AppConfig{}
	cfg.Server.GeoIP.Enabled = true
	cfg.Server.GeoIP.DenyCountries = []string{"CN"}
	withGeoIPConfig(t, cfg)

	called := false
	handler := GeoIPCountryBlocker(func(string) (string, error) { return "CN", nil })(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		called = true
	}))
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/private", nil)
	request.RemoteAddr = "203.0.113.8:4567"
	handler.ServeHTTP(recorder, request)
	if called {
		t.Fatal("blocked country request reached the next handler")
	}
	if recorder.Code != http.StatusForbidden || !strings.Contains(recorder.Body.String(), "GEOIP_BLOCKED") {
		t.Fatalf("response = %d %q, want GEOIP_BLOCKED forbidden response", recorder.Code, recorder.Body.String())
	}
}

func TestGeoIPCountryBlockerRendersHTMLForBrowser(t *testing.T) {
	cfg := &config.AppConfig{}
	cfg.Server.GeoIP.Enabled = true
	cfg.Server.GeoIP.DenyCountries = []string{"CN"}
	withGeoIPConfig(t, cfg)
	previousRenderer := RenderHTML
	RenderHTML = func(w http.ResponseWriter, _ *http.Request, status int, template string, data map[string]interface{}) {
		w.WriteHeader(status)
		_, _ = w.Write([]byte(template + ":" + data["error"].(string)))
	}
	t.Cleanup(func() { RenderHTML = previousRenderer })
	request := httptest.NewRequest(http.MethodGet, "/", nil)
	request.Header.Set("Accept", "text/html")
	request.RemoteAddr = "203.0.113.11:4567"
	recorder := httptest.NewRecorder()
	GeoIPCountryBlocker(func(string) (string, error) { return "CN", nil })(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {})).ServeHTTP(recorder, request)
	if recorder.Code != http.StatusForbidden || !strings.Contains(recorder.Body.String(), "page/error.tmpl:") {
		t.Fatalf("browser response = %d %q, want rendered HTML error", recorder.Code, recorder.Body.String())
	}
}

func TestGeoIPCountryBlockerFailsOpenForLookupErrorsAndPrivateIPs(t *testing.T) {
	cfg := &config.AppConfig{}
	cfg.Server.GeoIP.Enabled = true
	cfg.Server.GeoIP.AllowCountries = []string{"US"}
	withGeoIPConfig(t, cfg)

	t.Run("lookup error", func(t *testing.T) {
		called := false
		handler := GeoIPCountryBlocker(func(string) (string, error) { return "", http.ErrNotSupported })(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
			called = true
		}))
		recorder := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodGet, "/", nil)
		request.RemoteAddr = "203.0.113.9:4567"
		handler.ServeHTTP(recorder, request)
		if !called || recorder.Code != http.StatusOK {
			t.Fatalf("request was not allowed on lookup error: called=%t status=%d", called, recorder.Code)
		}
	})

	t.Run("private address", func(t *testing.T) {
		lookupCalled := false
		handler := GeoIPCountryBlocker(func(string) (string, error) {
			lookupCalled = true
			return "CN", nil
		})(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
		request := httptest.NewRequest(http.MethodGet, "/", nil)
		request.RemoteAddr = "192.168.1.2:4567"
		handler.ServeHTTP(httptest.NewRecorder(), request)
		if lookupCalled {
			t.Fatal("private address was sent to GeoIP lookup")
		}
	})
}

func TestGeoIPCountryBlockerAllowlistBypassesCountryPolicy(t *testing.T) {
	cfg := &config.AppConfig{}
	cfg.Server.GeoIP.Enabled = true
	cfg.Server.GeoIP.DenyCountries = []string{"CN"}
	cfg.Server.Security.Allowlist = []config.IPAllowlistEntry{{CIDR: "203.0.113.0/24"}}
	withGeoIPConfig(t, cfg)

	lookupCalled := false
	called := false
	handler := GeoIPCountryBlocker(func(string) (string, error) {
		lookupCalled = true
		return "CN", nil
	})(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { called = true }))
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/", nil)
	request.RemoteAddr = "203.0.113.10:4567"
	handler.ServeHTTP(recorder, request)
	if !called || lookupCalled || recorder.Code != http.StatusOK {
		t.Fatalf("allowlisted request was not passed through: next=%t lookup=%t status=%d", called, lookupCalled, recorder.Code)
	}
}
