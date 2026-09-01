package server

import (
	"fmt"
	"html/template"
	"io/fs"
	"regexp"
	"strings"
	"testing"
)

// buildProductionLikeTemplate replicates the production template loader in
// src/main.go: every .tmpl under the embedded template/ tree is read, and any
// file that does NOT already contain a {{define}} block is wrapped in
// {{define "<relative-path>"}}...{{end}} so it registers under its full
// relative path. Files that DO contain {{define}} register under whatever
// name(s) they declare. This mirrors exactly how gin resolves the name passed
// to c.HTML/NegotiateResponse, so a name that fails to register here is a
// guaranteed HTTP 500 at runtime.
func buildProductionLikeTemplate(t *testing.T) map[string]bool {
	t.Helper()
	sub, err := fs.Sub(GetTemplatesFS(), "template")
	if err != nil {
		t.Fatalf("fs.Sub(template): %v", err)
	}
	var paths []string
	if err := fs.WalkDir(sub, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() && strings.HasSuffix(path, ".tmpl") {
			paths = append(paths, path)
		}
		return nil
	}); err != nil {
		t.Fatalf("walk templates: %v", err)
	}
	funcs := template.FuncMap{
		"upper":     strings.ToUpper,
		"lower":     strings.ToLower,
		"nextTheme": NextTheme,
		"title":     func(s string) string { return s },
		"add":       func(a, b int) int { return a + b },
		"sub":       func(a, b int) int { return a - b },
		"t":         func(_, key string) string { return key },
		"tf":        func(_, key string, _ ...string) string { return key },
	}
	tmpl := template.New("").Funcs(funcs)
	for _, path := range paths {
		content, err := fs.ReadFile(sub, path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		cs := string(content)
		if !strings.Contains(cs, "{{define ") {
			cs = fmt.Sprintf("{{define %q}}%s{{end}}", path, cs)
		}
		if _, err := tmpl.Parse(cs); err != nil {
			t.Fatalf("parse %s: %v", path, err)
		}
	}
	names := map[string]bool{}
	for _, x := range tmpl.Templates() {
		if x.Name() != "" {
			names[x.Name()] = true
		}
	}
	return names
}

// mustResolveRenderNames is every template name a handler passes to
// c.HTML/NegotiateResponse. Each MUST register under the production-like
// loader, otherwise the corresponding route returns a 500 with an empty body.
// Every admin and page render name now has a structurally complete backing
// template; there is no excluded/unimplemented set.
var mustResolveRenderNames = []string{
	"admin/admin.tmpl",
	"admin/admin_auth_settings.tmpl",
	"admin/admin_backup_enhanced.tmpl",
	"admin/admin_database.tmpl",
	"admin/admin_email.tmpl",
	"admin/admin_email_editor.tmpl",
	"admin/admin_geoip.tmpl",
	"admin/admin_i2p.tmpl",
	"admin/admin_logs.tmpl",
	"admin/admin_metrics.tmpl",
	"admin/admin_notifications.tmpl",
	"admin/admin_passkey_login.tmpl",
	"admin/admin_scheduler.tmpl",
	"admin/admin_search.tmpl",
	"admin/admin_security.tmpl",
	"admin/admin_settings.tmpl",
	"admin/admin_ssl.tmpl",
	"admin/admin_system.tmpl",
	"admin/admin_tasks_enhanced.tmpl",
	"admin/admin_tor.tmpl",
	"admin/admin_user_invites.tmpl",
	"admin/admin_users.tmpl",
	"admin/admin_weather.tmpl",
	"admin/admin_web.tmpl",
	"admin/login.tmpl",
	"admin/setup_token.tmpl",
	"component/loading.tmpl",
	"page/about.tmpl",
	"page/add_location.tmpl",
	"page/api_docs.tmpl",
	"page/contact.tmpl",
	"page/dashboard.tmpl",
	"page/earthquake.tmpl",
	"page/earthquake_detail.tmpl",
	"page/edit_location.tmpl",
	"page/error.tmpl",
	"page/forgot_password.tmpl",
	"page/forgot_username.tmpl",
	"page/healthz.tmpl",
	"page/help.tmpl",
	"page/history.tmpl",
	"page/hurricane.tmpl",
	"page/login.tmpl",
	"page/moon.tmpl",
	"page/notifications.tmpl",
	"page/oidc_callback.tmpl",
	"page/oidc_redirect.tmpl",
	"page/passkey.tmpl",
	"page/preferences.tmpl",
	"page/privacy.tmpl",
	"page/recovery_key.tmpl",
	"page/register.tmpl",
	"page/reset_password.tmpl",
	"page/server_invite.tmpl",
	"page/setup_admin.tmpl",
	"page/setup_api_token.tmpl",
	"page/setup_complete.tmpl",
	"page/setup_security.tmpl",
	"page/setup_server_config.tmpl",
	"page/setup_services.tmpl",
	"page/setup_token.tmpl",
	"page/severe_weather.tmpl",
	"page/terms.tmpl",
	"page/two_factor.tmpl",
	"page/user/profile.tmpl",
	"page/user/security.tmpl",
	"page/user/settings.tmpl",
	"page/user/settings_appearance.tmpl",
	"page/user/settings_notifications.tmpl",
	"page/user/settings_privacy.tmpl",
	"page/user/settings_tokens.tmpl",
	"page/user_invite.tmpl",
	"page/verify_email.tmpl",
	"page/weather.tmpl",
}

// TestRenderNamesResolve guarantees every handler render name backed by a real
// template registers under the production loader. This is a regression guard
// for the template-name/embed-path mismatch class of bug (nested go:embed not
// shipping page/user/*, hyphen-vs-underscore define names, bare vs page/ vs
// admin/ prefixes) — all of which produce a silent 500 with no build error.
func TestRenderNamesResolve(t *testing.T) {
	names := buildProductionLikeTemplate(t)
	for _, want := range mustResolveRenderNames {
		if !names[want] {
			t.Errorf("render name %q does not resolve under the production loader (route would 500)", want)
		}
	}
}

// templateRefPattern matches every {{template "NAME" ...}} partial reference,
// tolerating the {{- whitespace-trim marker and arbitrary surrounding spacing.
var templateRefPattern = regexp.MustCompile(`\{\{-?\s*template\s+"([^"]+)"`)

// TestTemplatePartialsResolve guarantees every {{template "X"}} reference in
// every embedded .tmpl points at a template name that actually registers under
// the production loader. A reference to an undefined partial parses cleanly and
// even resolves the outer render name, but fails at EXECUTE time with
// "template ... not defined" — a silent HTTP 500 that name-resolution and the
// Go build both miss (this is exactly how admin/admin_web.tmpl shipped a broken
// /server/web referencing the never-defined components/admin-{header,sidebar}).
func TestTemplatePartialsResolve(t *testing.T) {
	names := buildProductionLikeTemplate(t)
	sub, err := fs.Sub(GetTemplatesFS(), "template")
	if err != nil {
		t.Fatalf("fs.Sub(template): %v", err)
	}
	if err := fs.WalkDir(sub, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, ".tmpl") {
			return nil
		}
		content, err := fs.ReadFile(sub, path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		for _, m := range templateRefPattern.FindAllStringSubmatch(string(content), -1) {
			if ref := m[1]; !names[ref] {
				t.Errorf("%s references undefined partial %q (route would 500 at execute)", path, ref)
			}
		}
		return nil
	}); err != nil {
		t.Fatalf("walk templates: %v", err)
	}
}
