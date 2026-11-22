package util

import (
	"strings"
	"testing"

	"github.com/webappsgo/wthr/src/config"
)

func TestOperatorHTMLSanitizesAndLocalizes(t *testing.T) {
	cfg := &config.AppConfig{}
	cfg.Server.Branding.Content.About = config.OperatorPageContentConfig{
		Format: "md",
		Body:   map[string]interface{}{"default": "Default **content**", "fr": "Bonjour <script>alert(1)</script> [contact](/server/contact)"},
	}
	got := string(operatorHTML(cfg, "about", "fr", nil))
	if !strings.Contains(got, "Bonjour") || strings.Contains(got, "<script") || strings.Contains(got, "/server/contact") {
		t.Fatalf("localized content was not safely rendered: %s", got)
	}
	if !strings.Contains(string(operatorHTML(cfg, "about", "es", nil)), "Default") {
		t.Fatal("default language content was not selected")
	}
}

func TestOperatorHTMLUsesStrictHeaderPolicy(t *testing.T) {
	cfg := &config.AppConfig{}
	cfg.Server.Branding.Content.Header = config.OperatorPageContentConfig{Format: "html", Body: `<div><table><tr><td>content</td></tr></table><script>bad()</script></div>`}
	got := string(operatorHTML(cfg, "header", "en", nil))
	if strings.Contains(got, "<table") || strings.Contains(got, "<script") || !strings.Contains(got, "content") {
		t.Fatalf("strict header policy output = %q", got)
	}
}

func TestExpandOperatorVariablesUsesRuntimeValues(t *testing.T) {
	got := expandOperatorVariables("{project_name} {project_org} {project_version} {build_datetime} {onion_address} {i2p_address}", map[string]string{
		"project_version": "1.2.3",
		"build_datetime":  "June 3, 2026 at 12:00:00 UTC",
		"onion_address":   "example.onion",
		"i2p_address":     "example.b32.i2p",
	})
	want := "wthr webappsgo 1.2.3 June 3, 2026 at 12:00:00 UTC example.onion example.b32.i2p"
	if got != want {
		t.Fatalf("expanded variables = %q, want %q", got, want)
	}
}
