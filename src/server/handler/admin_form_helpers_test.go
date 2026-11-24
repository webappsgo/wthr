package handler

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/webappsgo/wthr/src/config"
	"github.com/webappsgo/wthr/src/server/service"
)

func TestAuthSettingsFormHelpers(t *testing.T) {
	userinfoURL := func(path string) string {
		return (&url.URL{Scheme: "https", User: url.User("fixture-user"), Host: "idp.example", Path: path}).String()
	}
	for _, test := range []struct {
		value string
		want  bool
	}{
		{value: "https://idp.example/issuer", want: true},
		{value: "http://idp.example/issuer", want: true},
		{value: "https://idp.example/issuer#fragment"},
		{value: userinfoURL("/issuer")},
	} {
		if got := validProviderURL(test.value); got != test.want {
			t.Errorf("validProviderURL(%q) = %t, want %t", test.value, got, test.want)
		}
	}
	for _, test := range []struct {
		value string
		want  bool
	}{
		{value: "https://idp.example/metadata", want: true},
		{value: "http://idp.example/metadata"},
		{value: userinfoURL("/metadata")},
		{value: "https://idp.example/metadata#fragment"},
		{value: "/metadata"},
	} {
		if got := validSAMLMetadataURL(test.value); got != test.want {
			t.Errorf("validSAMLMetadataURL(%q) = %t, want %t", test.value, got, test.want)
		}
	}

	provider := service.SAMLProviderConfig{
		AdminGroups: []string{"admins", "operators"},
		RoleMapping: map[string][]string{"admin": {"root", "owner"}, "moderator": {"mod"}, "user": {"member"}},
	}
	form := makeSAMLProviderForm(provider)
	if form.AdminGroupsCSV != "admins, operators" || form.RoleAdminCSV != "root, owner" || form.RoleModeratorCSV != "mod" || form.RoleUserCSV != "member" {
		t.Fatalf("makeSAMLProviderForm() = %#v", form)
	}
	if got := splitAuthList(" admins, , operators "); len(got) != 2 || got[0] != "admins" || got[1] != "operators" {
		t.Fatalf("splitAuthList() = %#v", got)
	}
	if got, err := parseAuthFormInt("", 389); err != nil || got != 389 {
		t.Fatalf("parseAuthFormInt(empty) = %d, %v; want 389, nil", got, err)
	}
	if _, err := parseAuthFormInt("not-a-port", 389); err == nil {
		t.Fatal("parseAuthFormInt accepted an invalid integer")
	}
}

func TestBrandingFormHelpers(t *testing.T) {
	if got := splitBrandingKeywords("weather, forecast\nstorm"); strings.Join(got, "|") != "weather|forecast|storm" {
		t.Fatalf("splitBrandingKeywords() = %#v", got)
	}
	if got := normalizeTwitterHandle(" weather "); got != "@weather" {
		t.Fatalf("normalizeTwitterHandle() = %q", got)
	}
	if got := normalizeTwitterHandle("@weather"); got != "@weather" {
		t.Fatalf("normalizeTwitterHandle() changed a normalized handle: %q", got)
	}
	if got := operatorContentFromForm(url.Values{"body": {"  About us  "}}, "body", "md"); got.Body != "About us" || got.Format != "md" {
		t.Fatalf("operatorContentFromForm() = %#v", got)
	}
	if got := operatorContentFromForm(url.Values{}, "body", "md"); got.Body != nil || got.Format != "" {
		t.Fatalf("operatorContentFromForm(empty) = %#v", got)
	}
	for _, test := range []struct {
		value interface{}
		want  string
	}{
		{value: "plain", want: "plain"},
		{value: map[string]string{"default": "default", "fr": "bonjour"}, want: "default"},
		{value: map[string]string{"fr": "bonjour"}, want: "bonjour"},
		{value: map[string]interface{}{"default": "default", "fr": "bonjour"}, want: "default"},
		{value: map[string]interface{}{"fr": "bonjour"}, want: "bonjour"},
		{value: 42, want: ""},
	} {
		if got := operatorBodyForDisplay(test.value); got != test.want {
			t.Errorf("operatorBodyForDisplay(%#v) = %q, want %q", test.value, got, test.want)
		}
	}

	meta, err := parseMetaTags("og:site_name = Wthr\ndescription=Forecasts")
	if err != nil || formatMetaTags(meta) != "description=Forecasts\nog:site_name=Wthr" {
		t.Fatalf("metadata parse/format = %#v, %v", meta, err)
	}
	for _, invalid := range []string{"missing separator", "name=  "} {
		if _, err := parseMetaTags(invalid); err == nil {
			t.Errorf("parseMetaTags(%q) accepted invalid input", invalid)
		}
	}
}

func TestUpdateBrandingFromFormAndJSON(t *testing.T) {
	cfg := &config.AppConfig{}
	request := &http.Request{PostForm: url.Values{
		"title": {" Wthr "}, "keywords": {"weather, forecast"}, "twitter_handle": {"weather"},
		"header_content": {" Welcome "}, "header_content_format": {"html"},
		"meta_tags": {"description=Forecast"}, "noindex": {"true"}, "opengraph_enabled": {"true"},
	}}
	if err := updateBrandingFromForm(cfg, request); err != nil {
		t.Fatalf("updateBrandingFromForm() error = %v", err)
	}
	if cfg.Server.Branding.Title != "Wthr" || cfg.Server.Branding.Content.Header.Body != "Welcome" || cfg.Server.SEO.TwitterHandle != "@weather" || !cfg.Server.SEO.Noindex || !cfg.Server.SEO.OpenGraph.Enabled {
		t.Fatalf("branding form update = %#v %#v", cfg.Server.Branding, cfg.Server.SEO)
	}

	jsonRequest := httptest.NewRequest(http.MethodPost, "/branding", strings.NewReader(`{"branding":{"title":"New title"},"accent":"violet"}`))
	if err := updateBrandingFromJSON(cfg, httptest.NewRecorder(), jsonRequest); err != nil {
		t.Fatalf("updateBrandingFromJSON() error = %v", err)
	}
	if cfg.Server.Branding.Title != "New title" || cfg.Web.UI.Accent != "violet" {
		t.Fatalf("JSON branding update = %#v, accent %q", cfg.Server.Branding, cfg.Web.UI.Accent)
	}
	for _, body := range []string{"not json", `{}`, `{} {}`} {
		r := httptest.NewRequest(http.MethodPost, "/branding", strings.NewReader(body))
		if err := updateBrandingFromJSON(cfg, httptest.NewRecorder(), r); err == nil {
			t.Errorf("updateBrandingFromJSON(%q) accepted invalid or empty input", body)
		}
	}
}

func TestCurrentBrandingSettings(t *testing.T) {
	previous := config.GetGlobalConfig()
	t.Cleanup(func() { config.SetGlobalConfig(previous) })
	config.SetGlobalConfig(nil)
	branding, seo, accent := currentBrandingSettings()
	if branding.Title != "wthr" || seo.TitleSeparator != " - " || accent != "" {
		t.Fatalf("defaults = %#v %#v %q", branding, seo, accent)
	}
	cfg := &config.AppConfig{Server: config.ServerConfig{Branding: config.BrandingConfig{Title: "Custom"}, SEO: config.SEOConfig{TitleSeparator: " | "}}, Web: config.WebConfig{UI: config.UIConfig{Accent: "violet"}}}
	config.SetGlobalConfig(cfg)
	branding, seo, accent = currentBrandingSettings()
	if branding.Title != "Custom" || seo.TitleSeparator != " | " || accent != "violet" {
		t.Fatalf("configured values = %#v %#v %q", branding, seo, accent)
	}
}

func TestClusterFormValueHelpers(t *testing.T) {
	if cacheTypeForURL("valkey") != "valkey" || cacheTypeForURL("redis") != "redis" {
		t.Fatal("cacheTypeForURL did not map supported cache schemes")
	}
	if got := hostnameOrEmpty(); got == "" {
		t.Fatal("hostnameOrEmpty returned an empty hostname")
	}
}
