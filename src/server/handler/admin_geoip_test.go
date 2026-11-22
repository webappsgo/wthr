package handler

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/webappsgo/wthr/src/config"
)

func setGeoIPTestConfig(t *testing.T, cfg *config.AppConfig) {
	t.Helper()
	previous := config.GetGlobalConfig()
	config.SetGlobalConfig(cfg)
	t.Cleanup(func() { config.SetGlobalConfig(previous) })
}

func TestAdminGeoIPHandlerShowSettings(t *testing.T) {
	setGeoIPTestConfig(t, &config.AppConfig{})
	request, response := newAPITestContext("/server/administration/network/geoip")
	defer htmlRenderGuard(t)
	(&AdminGeoIPHandler{}).ShowGeoIPSettings(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusOK)
	}
}

func TestGeoIPDatabaseStatus(t *testing.T) {
	dataDir := t.TempDir()
	geoipDir := filepath.Join(dataDir, "security", "geoip")
	if err := os.MkdirAll(geoipDir, 0700); err != nil {
		t.Fatal(err)
	}
	for name, body := range map[string]string{
		"dbip-city-ipv4.mmdb":  "city-v4",
		"dbip-city-ipv6.mmdb":  "city-v6",
		"iptoasn-country.mmdb": "country",
	} {
		if err := os.WriteFile(filepath.Join(geoipDir, name), []byte(body), 0600); err != nil {
			t.Fatal(err)
		}
	}
	updated, size, present := geoIPDatabaseStatus(dataDir)
	if updated == "" || size != int64(len("city-v4city-v6country")) || present != 3 {
		t.Fatalf("geoIPDatabaseStatus() = (%q, %d, %d), want non-empty time, size 21 and 3 files", updated, size, present)
	}
	if updated, size, present := geoIPDatabaseStatus(""); updated != "" || size != 0 || present != 0 {
		t.Fatalf("empty data directory status = (%q, %d, %d), want empty status", updated, size, present)
	}
}

func TestAdminGeoIPHandlerUpdateSettingsWritesAreaFile(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("CONFIG_DIR", dir)
	cfg := &config.AppConfig{}
	setGeoIPTestConfig(t, cfg)
	request := httptest.NewRequest(http.MethodPost, "/api/v1/server/administration/network/geoip", strings.NewReader(`{"enabled":true,"deny_countries":["cn","RU"],"allow_countries":[]}`))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	(&AdminGeoIPHandler{}).UpdateGeoIPSettings(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	if got := config.GetGlobalConfig().Server.GeoIP.DenyCountries; !reflect.DeepEqual(got, []string{"CN", "RU"}) {
		t.Fatalf("deny countries = %v", got)
	}
	if _, err := os.Stat(filepath.Join(dir, "yaml.d", "security.yml")); err != nil {
		t.Fatalf("security.yml was not written: %v", err)
	}
}

func TestAdminGeoIPHandlerRejectsMalformedAndInvalidSettings(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("CONFIG_DIR", dir)
	setGeoIPTestConfig(t, &config.AppConfig{})
	for _, body := range []string{"{invalid", `{"enabled":true,"deny_countries":["USA"]}`} {
		request := httptest.NewRequest(http.MethodPost, "/api/v1/server/administration/network/geoip", strings.NewReader(body))
		request.Header.Set("Content-Type", "application/json")
		response := httptest.NewRecorder()
		(&AdminGeoIPHandler{}).UpdateGeoIPSettings(response, request)
		if response.Code != http.StatusBadRequest {
			t.Errorf("body %q: status = %d, want %d", body, response.Code, http.StatusBadRequest)
		}
	}
	if _, err := os.Stat(filepath.Join(dir, "yaml.d", "security.yml")); !os.IsNotExist(err) {
		t.Fatalf("invalid request created security.yml: %v", err)
	}
}

func TestAdminGeoIPHandlerReportsAreaWriteFailure(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("CONFIG_DIR", dir)
	if err := os.Mkdir(filepath.Join(dir, "yaml.d"), 0700); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(dir, "target.yml")
	if err := os.WriteFile(target, []byte("server: {}\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, filepath.Join(dir, "yaml.d", "security.yml")); err != nil {
		t.Fatal(err)
	}
	setGeoIPTestConfig(t, &config.AppConfig{})
	request := httptest.NewRequest(http.MethodPost, "/api/v1/server/administration/network/geoip", strings.NewReader(`{"enabled":true}`))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	(&AdminGeoIPHandler{}).UpdateGeoIPSettings(response, request)
	if response.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusInternalServerError)
	}
}

func TestParseCountryCodes(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  []string
		bad   bool
	}{
		{name: "empty", input: "", want: []string{}},
		{name: "split and normalize", input: "us, ca\nUS", want: []string{"US", "CA"}},
		{name: "reject invalid code", input: "USA", bad: true},
		{name: "reject punctuation", input: "U!", bad: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseCountryCodes(tt.input)
			if (err != nil) != tt.bad {
				t.Fatalf("parseCountryCodes() error = %v, want error %v", err, tt.bad)
			}
			if err == nil && !reflect.DeepEqual(got, tt.want) {
				t.Errorf("parseCountryCodes() = %v, want %v", got, tt.want)
			}
		})
	}
}
