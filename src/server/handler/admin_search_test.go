package handler

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

// TestAdminSearchPattern proves LIKE wildcards in the query are escaped so a
// query of "%" cannot match every row of a searched table.
func TestAdminSearchPattern(t *testing.T) {
	tests := []struct {
		name  string
		query string
		want  string
	}{
		{"plain term", "smtp", "%smtp%"},
		{"percent escaped", "%", `%\%%`},
		{"underscore escaped", "a_b", `%a\_b%`},
		{"backslash escaped", `a\b`, `%a\\b%`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := adminSearchPattern(tt.query); got != tt.want {
				t.Errorf("adminSearchPattern(%q) = %q, want %q", tt.query, got, tt.want)
			}
		})
	}
}

// TestAdminPathFromData covers the configurable admin path lookup and its
// fallback when template data carries no admin_path.
func TestAdminPathFromData(t *testing.T) {
	if got := adminPathFromData(map[string]interface{}{"admin_path": "/server/backoffice"}); got != "/server/backoffice" {
		t.Errorf("adminPathFromData() = %q, want /server/backoffice", got)
	}
	if got := adminPathFromData(map[string]interface{}{}); got != "/server/admin" {
		t.Errorf("adminPathFromData() fallback = %q, want /server/admin", got)
	}
	if got := adminPathFromData(map[string]interface{}{"admin_path": ""}); got != "/server/admin" {
		t.Errorf("adminPathFromData() empty = %q, want /server/admin", got)
	}
}

// TestAdminSearchPageCatalog verifies the page catalog matches on the route
// path and builds links under the configured admin path, never a hardcoded one.
func TestAdminSearchPageCatalog(t *testing.T) {
	c, _ := newTestContext(http.MethodGet, "/server/backoffice/config/search?q=firewall")

	results := adminSearchPageCatalog(c, "firewall", "/server/backoffice")
	if len(results) != 1 {
		t.Fatalf("results = %d, want 1", len(results))
	}
	if results[0].URL != "/server/backoffice/config/security/firewall" {
		t.Errorf("URL = %q, want /server/backoffice/config/security/firewall", results[0].URL)
	}

	if got := adminSearchPageCatalog(c, "no-such-page", "/server/admin"); len(got) != 0 {
		t.Errorf("unmatched query returned %d results, want 0", len(got))
	}
}

// TestAdminSearchAllEmptyQuery proves an empty query returns nothing rather
// than dumping every searchable row.
func TestAdminSearchAllEmptyQuery(t *testing.T) {
	c, _ := newTestContext(http.MethodGet, "/server/admin/config/search")

	if got := adminSearchAll(c, "", "/server/admin"); len(got) != 0 {
		t.Errorf("empty query returned %d results, want 0", len(got))
	}
}

// TestAdminSearchAPIEnvelope covers the canonical success envelope of the JSON
// search endpoint.
func TestAdminSearchAPIEnvelope(t *testing.T) {
	c, w := newTestContext(http.MethodGet, "/api/v1/server/admin/config/search?q=firewall")
	AdminSearchAPI(w, c)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}

	var body struct {
		OK   bool `json:"ok"`
		Data struct {
			Query   string              `json:"query"`
			Count   int                 `json:"count"`
			Results []AdminSearchResult `json:"results"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal body %q: %v", w.Body.String(), err)
	}
	if !body.OK {
		t.Error("ok = false, want true")
	}
	if body.Data.Query != "firewall" {
		t.Errorf("query = %q, want firewall", body.Data.Query)
	}
	if body.Data.Count != len(body.Data.Results) {
		t.Errorf("count = %d, want %d", body.Data.Count, len(body.Data.Results))
	}
	for _, result := range body.Data.Results {
		if !strings.HasPrefix(result.URL, "/server/") {
			t.Errorf("result URL %q should be an admin panel path", result.URL)
		}
	}
}
