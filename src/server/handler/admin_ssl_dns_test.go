package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/webappsgo/wthr/src/config"
	"github.com/webappsgo/wthr/src/server/service"
	"github.com/webappsgo/wthr/src/util"
)

const dnsHandlerKey = "MDEyMzQ1Njc4OWFiY2RlZjAxMjM0NTY3ODlhYmNkZWY="

// withDNSConfig installs a global config backed by a server.yml in a temporary directory.
func withDNSConfig(t *testing.T) *config.AppConfig {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "server.yml"), []byte("{}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("CONFIG_DIR", dir)
	previous := config.GetGlobalConfig()
	cfg := &config.AppConfig{}
	cfg.Server.Security.EncryptionKey = dnsHandlerKey
	config.SetGlobalConfig(cfg)
	t.Cleanup(func() { config.SetGlobalConfig(previous) })
	return cfg
}

func dnsRequest(method, path, body string) *http.Request {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	return req
}

func TestSaveDNSSettingsSealsCredentialsAndMasksTheView(t *testing.T) {
	cfg := withDNSConfig(t)
	db := newTestServerDB(t)
	h := NewSSLHandler(t.TempDir(), db, "")

	if err := service.MarkDNSValidated(db); err != nil {
		t.Fatal(err)
	}
	rec := httptest.NewRecorder()
	h.SaveDNSSettings(rec, dnsRequest(http.MethodPut, "/dns", `{"provider":"cloudflare","credentials":{"CF_DNS_API_TOKEN":"plain-token"}}`))
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "plain-token") {
		t.Error("the response must never carry a credential value")
	}
	stored := cfg.Server.SSL.LetsEncrypt.DNSCredentials["CF_DNS_API_TOKEN"]
	if !strings.HasPrefix(stored, util.DNSCredentialPrefix) {
		t.Errorf("stored value is not sealed: %q", stored)
	}
	if _, ok := service.DNSValidatedAt(db); ok {
		t.Error("changing the credentials must clear dns_validated_at")
	}

	serverFile, err := os.ReadFile(filepath.Join(os.Getenv("CONFIG_DIR"), "server.yml"))
	if err != nil || strings.Contains(string(serverFile), "plain-token") || strings.Contains(string(serverFile), "enc:v1:") {
		t.Errorf("server.yml must not carry DNS credentials: %v\n%s", err, serverFile)
	}
	secretsFile, err := os.ReadFile(filepath.Join(os.Getenv("CONFIG_DIR"), "secrets.yml"))
	if err != nil || strings.Contains(string(secretsFile), "plain-token") || !strings.Contains(string(secretsFile), "enc:v1:") {
		t.Errorf("secrets.yml must hold only the sealed DNS credential: %v\n%s", err, secretsFile)
	}

	rec = httptest.NewRecorder()
	h.GetDNSSettings(rec, httptest.NewRequest(http.MethodGet, "/dns", nil))
	var view dnsSettingsView
	if err := json.Unmarshal(rec.Body.Bytes(), &view); err != nil {
		t.Fatal(err)
	}
	if view.Credentials["CF_DNS_API_TOKEN"] != util.DNSCredentialMask || view.Provider != "cloudflare" || len(view.Providers) < 100 {
		t.Errorf("view = %+v", view)
	}
}

func TestSaveDNSSettingsKeepsAStoredValueWhenTheMaskIsSent(t *testing.T) {
	cfg := withDNSConfig(t)
	db := newTestServerDB(t)
	h := NewSSLHandler(t.TempDir(), db, "")

	rec := httptest.NewRecorder()
	h.SaveDNSSettings(rec, dnsRequest(http.MethodPut, "/dns", `{"provider":"cloudflare","credentials":{"CF_DNS_API_TOKEN":"first"}}`))
	if rec.Code != http.StatusOK {
		t.Fatal(rec.Body.String())
	}
	before := cfg.Server.SSL.LetsEncrypt.DNSCredentials["CF_DNS_API_TOKEN"]

	if err := service.MarkDNSValidated(db); err != nil {
		t.Fatal(err)
	}
	rec = httptest.NewRecorder()
	h.SaveDNSSettings(rec, dnsRequest(http.MethodPut, "/dns", `{"provider":"cloudflare","credentials":{"CF_DNS_API_TOKEN":"********"}}`))
	if rec.Code != http.StatusOK {
		t.Fatal(rec.Body.String())
	}
	if cfg.Server.SSL.LetsEncrypt.DNSCredentials["CF_DNS_API_TOKEN"] != before {
		t.Error("an unchanged value must keep its ciphertext")
	}
	if _, ok := service.DNSValidatedAt(db); !ok {
		t.Error("nothing changed, so dns_validated_at stays")
	}
}

func TestSaveDNSSettingsReportsVariableNamesOnly(t *testing.T) {
	cfg := withDNSConfig(t)
	h := NewSSLHandler(t.TempDir(), newTestServerDB(t), "")

	rec := httptest.NewRecorder()
	h.SaveDNSSettings(rec, dnsRequest(http.MethodPut, "/dns", `{"provider":"cloudflare","credentials":{"CF_API_EMAIL":"admin@example.com"}}`))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "CLOUDFLARE_DNS_API_TOKEN") || strings.Contains(rec.Body.String(), "admin@example.com") {
		t.Errorf("body must name the variable and never a value: %s", rec.Body.String())
	}
	if cfg.Server.SSL.LetsEncrypt.DNSProvider != "" {
		t.Error("a rejected submission must store nothing")
	}
}

func TestSaveDNSSettingsRejectsUnknownProviderAndClears(t *testing.T) {
	cfg := withDNSConfig(t)
	h := NewSSLHandler(t.TempDir(), newTestServerDB(t), "")

	rec := httptest.NewRecorder()
	h.SaveDNSSettings(rec, dnsRequest(http.MethodPut, "/dns", `{"provider":"exec","credentials":{}}`))
	if rec.Code != http.StatusBadRequest {
		t.Errorf("an unsupported provider must be refused, got %d", rec.Code)
	}

	rec = httptest.NewRecorder()
	h.SaveDNSSettings(rec, dnsRequest(http.MethodPut, "/dns", `{"provider":"digitalocean","credentials":{"DO_AUTH_TOKEN":"t"}}`))
	if rec.Code != http.StatusOK {
		t.Fatal(rec.Body.String())
	}
	rec = httptest.NewRecorder()
	h.SaveDNSSettings(rec, dnsRequest(http.MethodPut, "/dns", `{"provider":"","credentials":{}}`))
	if rec.Code != http.StatusOK || cfg.Server.SSL.LetsEncrypt.DNSProvider != "" || len(cfg.Server.SSL.LetsEncrypt.DNSCredentials) != 0 {
		t.Errorf("an empty provider returns to manual mode: %d %+v", rec.Code, cfg.Server.SSL.LetsEncrypt)
	}
}

func TestCheckDNSSettingsStoresNothing(t *testing.T) {
	cfg := withDNSConfig(t)
	h := NewSSLHandler(t.TempDir(), newTestServerDB(t), "")

	rec := httptest.NewRecorder()
	h.CheckDNSSettings(rec, dnsRequest(http.MethodPost, "/dns/check", `{"provider":"digitalocean","credentials":{"DO_AUTH_TOKEN":"t"}}`))
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	if cfg.Server.SSL.LetsEncrypt.DNSProvider != "" {
		t.Error("a check must not store settings")
	}

	rec = httptest.NewRecorder()
	h.CheckDNSSettings(rec, dnsRequest(http.MethodPost, "/dns/check", `{"provider":"digitalocean","credentials":{}}`))
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "DO_AUTH_TOKEN") {
		t.Errorf("a check names the missing variable: %d %s", rec.Code, rec.Body.String())
	}
}
