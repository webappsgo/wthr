package handler

import (
	"bytes"
	"io/fs"
	"path"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/webappsgo/wthr/src/server"
)

// TestServerPageTemplatesDeclareSharedShell checks every page template,
// including nested user pages, for the PART 16 shared page-shell structure.
func TestServerPageTemplatesDeclareSharedShell(t *testing.T) {
	root := server.GetTemplatesFS()
	templates, err := server.LoadTemplates()
	if err != nil {
		t.Fatalf("load page templates: %v", err)
	}
	data := map[string]interface{}{
		"server": map[string]interface{}{
			"Lang": "en", "Title": "wthr", "Description": "Weather", "Theme": "dark", "ThemeClass": "theme-dark",
			"Logo": "", "Logo200": "", "Logo50": "", "Tor": map[string]interface{}{"Enabled": false},
			"I2P": map[string]interface{}{"Enabled": false, "Running": false},
		},
		"lang": "en", "user": map[string]interface{}{
			"Email": "", "Role": "guest", "Bio": "", "DisplayName": "", "Language": "en",
			"Location": "", "Timezone": "UTC", "Website": "", "Visibility": "public",
			"CreatedAt": time.Time{}, "UpdatedAt": time.Time{}, "ID": 1, "Phone": "",
			"TwoFactorEnabled": false, "Username": "guest",
		},
		"csrf_token": "test-csrf-token", "available_languages": []interface{}{},
		"locationCount": 0,
		"Earthquake":    map[string]interface{}{"Time": time.Unix(0, 0), "Place": "Test location", "Magnitude": 1.0, "Tsunami": 0, "Status": "reviewed", "Type": "earthquake", "MagnitudeType": "mb"},
		"Earthquakes":   []interface{}{},
	}
	err = fs.WalkDir(root, "template/page", func(name string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil || entry.IsDir() || path.Ext(name) != ".tmpl" {
			return walkErr
		}
		var rendered bytes.Buffer
		templateName := strings.TrimPrefix(name, "template/")
		if err := templates.ExecuteTemplate(&rendered, templateName, data); err != nil {
			t.Errorf("render %s: %v", name, err)
			return nil
		}
		page := rendered.String()
		if strings.Count(page, "<main") != 1 || !strings.Contains(page, `<main id="main-content">`) {
			t.Errorf("%s must declare exactly one main element with id main-content", name)
		}
		if strings.Count(page, "<h1") != 1 || !strings.Contains(page, `class="page-title"`) {
			t.Errorf("%s must declare exactly one h1 with class page-title", name)
		}
		if !strings.Contains(page, `class="page-header"`) {
			t.Errorf("%s must place its title in the shared page header", name)
		}
		if !strings.Contains(page, `class="container container-sm"`) &&
			!strings.Contains(page, `class="container container-md"`) &&
			!strings.Contains(page, `class="container container-lg"`) {
			t.Errorf("%s must use one shared container width modifier", name)
		}
		if !strings.Contains(page, `href="#main-content"`) || !strings.Contains(page, `href="#navigation"`) {
			t.Errorf("%s must render both skip links", name)
		}
		if strings.Index(page, `href="#main-content"`) > strings.Index(page, `href="#navigation"`) {
			t.Errorf("%s must render the main-content skip link before the navigation skip link", name)
		}
		if strings.Contains(page, `type="application/json"`) {
			t.Errorf("%s must not embed executable-script data blocks; use escaped data markup", name)
		}
		if !strings.Contains(page, `class="site-header"`) || !strings.Contains(page, `class="nav-links"`) || !strings.Contains(page, `class="header-actions"`) {
			t.Errorf("%s must render the shared public header and navigation", name)
		}
		for _, script := range regexp.MustCompile(`(?is)<script\b([^>]*)>(.*?)</script>`).FindAllStringSubmatch(page, -1) {
			attrs, body := script[1], strings.TrimSpace(script[2])
			if body != "" && !regexp.MustCompile(`(?i)type=["']application/ld\+json["']`).MatchString(attrs) {
				t.Errorf("%s must not render inline executable script bodies", name)
			}
		}
		if regexp.MustCompile(`(?i)\sstyle\s*=|\son[a-z]+\s*=`).MatchString(page) {
			t.Errorf("%s must not render inline styles or event handlers", name)
		}
		for _, form := range regexp.MustCompile(`(?is)<form\b[^>]*>.*?</form>`).FindAllString(page, -1) {
			post := regexp.MustCompile(`(?i)\bmethod=["']?post`).MatchString(form)
			action := regexp.MustCompile(`(?i)\baction=["']([^"']*)["']`).FindStringSubmatch(form)
			internalPost := action == nil || strings.HasPrefix(action[1], "/")
			if post && internalPost && !strings.Contains(form, `name="csrf_token"`) {
				t.Errorf("%s renders a POST form without a CSRF token", name)
			}
			for _, control := range regexp.MustCompile(`(?is)<(input|select|textarea)\b([^>]*)>`).FindAllStringSubmatch(form, -1) {
				attrs := control[2]
				if regexp.MustCompile(`(?i)\btype=["']?(hidden|submit|button|reset|image)\b`).MatchString(attrs) || regexp.MustCompile(`(?i)\baria-label\s*=`).MatchString(attrs) {
					continue
				}
				id := regexp.MustCompile(`(?i)\bid=["']([^"']+)["']`).FindStringSubmatch(attrs)
				if id != nil && strings.Contains(form, `for="`+id[1]+`"`) {
					continue
				}
				position := strings.Index(form, control[0])
				labelStart := strings.LastIndex(form[:position], "<label")
				labelEnd := strings.LastIndex(form[:position], "</label>")
				if labelStart <= labelEnd {
					t.Errorf("%s renders a form control without an associated label", name)
				}
			}
		}
		if strings.Contains(strings.ToLower(page), `href="javascript:`) || strings.Contains(strings.ToLower(page), `action="javascript:`) {
			t.Errorf("%s must not use javascript: URLs", name)
		}
		if strings.Contains(page, "<table") {
			if !strings.Contains(page, `class="data-table"`) && !strings.Contains(page, " data-table") {
				t.Errorf("%s tables must use the shared data-table class", name)
			}
			if !strings.Contains(page, "<thead") {
				t.Errorf("%s tables must have a thead", name)
			}
			if !strings.Contains(page, "table-wrapper") {
				t.Errorf("%s tables must be inside a table-wrapper", name)
			}
		}
		for _, match := range regexp.MustCompile(`<section\b[^>]*class="[^"]*\bsection-card\b[^"]*"[^>]*aria-labelledby="([^"]+)"[^>]*>([\s\S]*?)</section>`).FindAllStringSubmatch(page, -1) {
			heading := regexp.MustCompile(`<h2\b[^>]*id="([^"]+)"`).FindStringSubmatch(match[2])
			if heading == nil || heading[1] != match[1] {
				t.Errorf("%s has a section card whose aria-labelledby does not match its H2", name)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk page templates: %v", err)
	}
}
