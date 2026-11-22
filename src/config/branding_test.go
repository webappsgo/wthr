package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestLoadConfigBrandingAndSEODefaults(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "server.yml"), []byte("{}\n"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("CONFIG_DIR", dir)
	t.Setenv("I2P_ENABLED", "")

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}
	if cfg.Server.Branding.Title != "wthr" || cfg.Server.SEO.TitleSeparator != " - " || cfg.Server.SEO.MetaTags == nil {
		t.Errorf("branding/SEO defaults = %+v / %+v", cfg.Server.Branding, cfg.Server.SEO)
	}
	if !cfg.Server.SEO.OpenGraph.Enabled || cfg.Server.SEO.OpenGraph.Type != "website" || !cfg.Server.SEO.Twitter.Enabled || cfg.Server.SEO.Twitter.Card != "summary_large_image" {
		t.Errorf("social metadata defaults = OpenGraph %+v, Twitter %+v", cfg.Server.SEO.OpenGraph, cfg.Server.SEO.Twitter)
	}
	if cfg.Web.UI.Theme != "dark" {
		t.Errorf("default theme = %q, want dark", cfg.Web.UI.Theme)
	}
}

func TestBrandingAndSEOConfigYAML(t *testing.T) {
	var cfg AppConfig
	err := yaml.Unmarshal([]byte(`
server:
  branding:
    favicon: mark.svg
    logo: https://example.test/logo.png
    content:
      about:
        body:
          default: Hello
          fr: Bonjour
  seo:
    title_separator: " | "
    meta_tags:
      color-scheme: dark
    canonical: https://example.test/base
    noindex: true
    opengraph:
      enabled: false
      type: article
      site_name: Example
    twitter:
      enabled: false
      card: summary
      creator: "@author"
web:
  ui:
    accent: "#336699"
`), &cfg)
	if err != nil {
		t.Fatalf("yaml.Unmarshal() error = %v", err)
	}
	validateServerBranding(&cfg)
	validateServerSEO(&cfg)
	if cfg.Server.Branding.Favicon != "mark.svg" || cfg.Server.Branding.Logo != "https://example.test/logo.png" {
		t.Errorf("branding image values = (%q, %q)", cfg.Server.Branding.Favicon, cfg.Server.Branding.Logo)
	}
	if cfg.Server.Branding.Content.About.Format != "md" {
		t.Errorf("about format = %q, want default md", cfg.Server.Branding.Content.About.Format)
	}
	body, ok := cfg.Server.Branding.Content.About.Body.(map[string]interface{})
	if !ok || body["fr"] != "Bonjour" {
		t.Errorf("about body = %#v, want localized map", cfg.Server.Branding.Content.About.Body)
	}
	if cfg.Server.SEO.TitleSeparator != " | " || !cfg.Server.SEO.Noindex || cfg.Server.SEO.OpenGraph.Enabled || cfg.Server.SEO.OpenGraph.Type != "article" || cfg.Server.SEO.Twitter.Enabled || cfg.Server.SEO.Twitter.Card != "summary" {
		t.Errorf("SEO config was not decoded: %+v", cfg.Server.SEO)
	}
	if cfg.Server.SEO.MetaTags["color-scheme"] != "dark" || cfg.Server.SEO.Canonical != "https://example.test/base" || cfg.Web.UI.Accent != "#336699" {
		t.Errorf("branding/SEO metadata = %+v / %+v", cfg.Server.SEO, cfg.Web.UI)
	}
}

func TestValidateServerSEOReplacesInvalidBrandingValues(t *testing.T) {
	cfg := &AppConfig{}
	cfg.Server.Branding.Favicon = "../outside.png"
	cfg.Server.Branding.Logo = "data:text/html,<script>bad</script>"
	cfg.Server.Branding.Content.Header = OperatorPageContentConfig{Format: "md", Body: "<b>Not allowed</b>"}
	cfg.Server.Branding.Content.About = OperatorPageContentConfig{Format: "unknown", Body: "ignored"}
	cfg.Server.SEO.OGImage = "javascript:alert(1)"
	cfg.Web.UI.Accent = "blue"
	cfg.Server.SEO.TitleSeparator = strings.Repeat("x", 9)
	cfg.Server.SEO.Canonical = "http://example.test"
	cfg.Server.SEO.MetaTags = map[string]string{
		"description": "must be ignored",
		"bad name":    "must be ignored",
		"theme-color": "#fff",
	}
	cfg.Server.SEO.Twitter.Card = "player"
	cfg.Server.SEO.TwitterHandle = "not-a-handle"
	cfg.Server.SEO.Verification.Google = "not valid!"
	cfg.Server.SEO.Verification.Custom = []VerificationMetaTag{
		{Name: "safe_name", Content: "valid"},
		{Name: "bad name", Content: "invalid"},
	}

	validateServerBranding(cfg)
	validateServerSEO(cfg)

	if cfg.Server.Branding.Favicon != "" || cfg.Server.Branding.Logo != "" || cfg.Server.Branding.Content.Header.Format != "" || cfg.Server.Branding.Content.About.Format != "" {
		t.Errorf("invalid branding values were not cleared: %+v", cfg.Server.Branding)
	}
	if cfg.Server.SEO.OGImage != "" || cfg.Web.UI.Accent != "" || cfg.Server.SEO.TitleSeparator != " - " || cfg.Server.SEO.Canonical != "" {
		t.Errorf("invalid SEO values were not replaced: %+v", cfg.Server.SEO)
	}
	if len(cfg.Server.SEO.MetaTags) != 1 || cfg.Server.SEO.MetaTags["theme-color"] != "#fff" {
		t.Errorf("meta_tags = %#v, want only theme-color", cfg.Server.SEO.MetaTags)
	}
	if cfg.Server.SEO.Twitter.Card != "summary_large_image" || cfg.Server.SEO.TwitterHandle != "" || cfg.Server.SEO.Verification.Google != "" || len(cfg.Server.SEO.Verification.Custom) != 1 {
		t.Errorf("invalid verification/social values were not cleared: %+v", cfg.Server.SEO)
	}
}

func TestValidateServerBrandingImageValues(t *testing.T) {
	valid := []string{
		"https://example.test/logo.svg",
		"data:image/png;base64,aGVsbG8=",
		"data:image/svg+xml,%3Csvg%3E%3C/svg%3E",
		"logo.webp",
	}
	for _, value := range valid {
		if !validBrandingImageValue(value) {
			t.Errorf("validBrandingImageValue(%q) = false", value)
		}
	}
	invalid := []string{
		"http://example.test/logo.svg",
		"https://user:pass@example.test/logo.svg",
		"data:image/png;base64,%%%",
		"data:image/png;base64," + strings.Repeat("A", 350*1024),
		"../logo.png",
		"logo.gif",
	}
	for _, value := range invalid {
		if validBrandingImageValue(value) {
			t.Errorf("validBrandingImageValue(%q) = true", value)
		}
	}
}

func TestValidateServerSEOReturnsDefaultForTooManyMetaTags(t *testing.T) {
	tags := make(map[string]string, 21)
	for i := 0; i < 21; i++ {
		tags["tag"+string(rune('a'+i))] = "value"
	}
	cfg := &AppConfig{Server: ServerConfig{SEO: SEOConfig{MetaTags: tags}}}
	validateServerSEO(cfg)
	if len(cfg.Server.SEO.MetaTags) != 0 {
		t.Errorf("meta_tags has %d entries after validation, want empty default", len(cfg.Server.SEO.MetaTags))
	}
}
