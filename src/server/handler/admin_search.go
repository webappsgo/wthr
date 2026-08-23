package handler

import (
	"context"
	"database/sql"
	"github.com/webappsgo/wthr/src/server/middleware"
	"net/http"
	"strings"

	"github.com/webappsgo/wthr/src/database"
)

// adminSearchLimit caps how many rows each searched source contributes so a
// broad query cannot turn the admin search into an unbounded table dump.
const adminSearchLimit = 10

// AdminSearchResult is a single hit returned by the admin global search.
// AI.md PART 17 header spec: "Search | Center | Global search (settings, logs, etc.)"
type AdminSearchResult struct {
	Category string `json:"category"`
	Title    string `json:"title"`
	Detail   string `json:"detail"`
	URL      string `json:"url"`
}

// adminSearchPage is one entry of the admin page catalog, mapping a route under
// /{admin_path}/config/ to the translation key its sidebar link already uses.
type adminSearchPage struct {
	path     string
	titleKey string
}

// adminSearchPages mirrors the PART 17 sidebar so admins can jump to a settings
// page by name. The paths are suffixes appended to /{admin_path}/config/.
var adminSearchPages = []adminSearchPage{
	{"settings", "nav.settings"},
	{"branding", "admin.nav.branding"},
	{"ssl", "admin.nav.ssl"},
	{"scheduler", "admin.nav.scheduler"},
	{"email", "admin.nav.email"},
	{"logs", "admin.nav.logs"},
	{"backup", "admin.nav.backup"},
	{"maintenance", "admin.nav.maintenance"},
	{"updates", "admin.nav.updates"},
	{"info", "admin.nav.info"},
	{"security/auth", "admin.nav.authentication"},
	{"security/tokens", "admin.nav.tokens"},
	{"security/ratelimit", "admin.nav.ratelimit"},
	{"security/firewall", "admin.nav.firewall"},
	{"network/tor", "admin.nav.tor"},
	{"network/geoip", "admin.nav.geoip"},
	{"network/blocklists", "admin.nav.blocklists"},
	{"users", "admin.nav.user_list"},
	{"users/invites", "admin.nav.invites"},
	{"roles", "admin.nav.roles"},
}

// AdminSearchPage renders the global admin search results page.
func AdminSearchPage(w http.ResponseWriter, r *http.Request) {
	data := AdminTemplateData(r, map[string]interface{}{
		"title": AdminTranslate(r, "admin.search.title"),
		"page":  "search",
	})

	query := strings.TrimSpace(r.URL.Query().Get("q"))
	data["query"] = query
	data["results"] = adminSearchAll(r, query, adminPathFromData(data))

	middleware.RenderHTML(w, r, http.StatusOK, "admin/admin_search.tmpl", data)
}

// AdminSearchAPI returns the same results as JSON for the API surface.
func AdminSearchAPI(w http.ResponseWriter, r *http.Request) {
	query := strings.TrimSpace(r.URL.Query().Get("q"))
	results := adminSearchAll(r, query, adminPathFromData(AdminTemplateData(r, map[string]interface{}{})))

	RespondNegotiatedData(w, r, http.StatusOK, map[string]interface{}{
		"ok": true,
		"data": map[string]interface{}{
			"query":   query,
			"results": results,
			"count":   len(results),
		},
	})
}

// adminPathFromData reads the configurable admin path out of template data so
// result links never hardcode the literal "admin" segment.
func adminPathFromData(data map[string]interface{}) string {
	if adminPath, ok := data["admin_path"].(string); ok && adminPath != "" {
		return adminPath
	}
	return "/server/admin"
}

// adminSearchAll runs every source search for the given query. An empty query
// returns no results rather than everything.
func adminSearchAll(r *http.Request, query, adminPath string) []AdminSearchResult {
	results := []AdminSearchResult{}
	if query == "" {
		return results
	}

	results = append(results, adminSearchPageCatalog(r, query, adminPath)...)
	results = append(results, adminSearchSettings(r, query, adminPath)...)
	results = append(results, adminSearchTasks(r, query, adminPath)...)
	results = append(results, adminSearchAuditLog(r, query, adminPath)...)
	results = append(results, adminSearchUsers(r, query, adminPath)...)
	return results
}

// adminSearchPageCatalog matches the query against admin page names.
func adminSearchPageCatalog(r *http.Request, query, adminPath string) []AdminSearchResult {
	needle := strings.ToLower(query)
	category := AdminTranslate(r, "admin.search.category.pages")

	results := []AdminSearchResult{}
	for _, page := range adminSearchPages {
		title := AdminTranslate(r, page.titleKey)
		if !strings.Contains(strings.ToLower(title), needle) && !strings.Contains(page.path, needle) {
			continue
		}
		results = append(results, AdminSearchResult{
			Category: category,
			Title:    title,
			Detail:   adminPath + "/config/" + page.path,
			URL:      adminPath + "/config/" + page.path,
		})
	}
	return results
}

// adminSearchSettings matches stored server configuration keys and descriptions.
func adminSearchSettings(r *http.Request, query, adminPath string) []AdminSearchResult {
	db := database.GetServerDB()
	if db == nil {
		return nil
	}

	pattern := adminSearchPattern(query)
	rows, err := database.QueryContext(context.Background(), db, database.TimeoutSimpleSelect,
		`SELECT key, COALESCE(description, '') FROM server_config
		 WHERE key LIKE ? ESCAPE '\' OR description LIKE ? ESCAPE '\'
		 ORDER BY key LIMIT ?`, pattern, pattern, adminSearchLimit)
	if err != nil {
		return nil
	}
	defer rows.Close()

	category := AdminTranslate(r, "admin.search.category.settings")
	return adminSearchScanPairs(rows, category, adminPath+"/config/settings")
}

// adminSearchTasks matches scheduled task ids and names.
func adminSearchTasks(r *http.Request, query, adminPath string) []AdminSearchResult {
	db := database.GetServerDB()
	if db == nil {
		return nil
	}

	pattern := adminSearchPattern(query)
	rows, err := database.QueryContext(context.Background(), db, database.TimeoutSimpleSelect,
		`SELECT task_name, COALESCE(schedule, '') FROM server_scheduler_state
		 WHERE task_id LIKE ? ESCAPE '\' OR task_name LIKE ? ESCAPE '\'
		 ORDER BY task_name LIMIT ?`, pattern, pattern, adminSearchLimit)
	if err != nil {
		return nil
	}
	defer rows.Close()

	category := AdminTranslate(r, "admin.search.category.tasks")
	return adminSearchScanPairs(rows, category, adminPath+"/config/scheduler")
}

// adminSearchAuditLog matches recent audit entries by action or resource.
func adminSearchAuditLog(r *http.Request, query, adminPath string) []AdminSearchResult {
	db := database.GetServerDB()
	if db == nil {
		return nil
	}

	pattern := adminSearchPattern(query)
	rows, err := database.QueryContext(context.Background(), db, database.TimeoutSimpleSelect,
		`SELECT action, COALESCE(timestamp, '') FROM server_audit_log
		 WHERE action LIKE ? ESCAPE '\' OR resource_type LIKE ? ESCAPE '\' OR resource_id LIKE ? ESCAPE '\'
		 ORDER BY timestamp DESC LIMIT ?`, pattern, pattern, pattern, adminSearchLimit)
	if err != nil {
		return nil
	}
	defer rows.Close()

	category := AdminTranslate(r, "admin.search.category.audit")
	return adminSearchScanPairs(rows, category, adminPath+"/config/logs/audit")
}

// adminSearchUsers matches user accounts by username or display name. Email is
// deliberately not returned: AI.md PART 34 keeps a user's full email hidden from
// the Server Admin surface.
func adminSearchUsers(r *http.Request, query, adminPath string) []AdminSearchResult {
	db := database.GetUsersDB()
	if db == nil {
		return nil
	}

	pattern := adminSearchPattern(query)
	rows, err := database.QueryContext(context.Background(), db, database.TimeoutSimpleSelect,
		`SELECT username, COALESCE(display_name, '') FROM user_accounts
		 WHERE username LIKE ? ESCAPE '\' OR display_name LIKE ? ESCAPE '\'
		 ORDER BY username LIMIT ?`, pattern, pattern, adminSearchLimit)
	if err != nil {
		return nil
	}
	defer rows.Close()

	category := AdminTranslate(r, "admin.search.category.users")
	return adminSearchScanPairs(rows, category, adminPath+"/config/users")
}

// adminSearchScanPairs collects two-column (title, detail) rows into results.
func adminSearchScanPairs(rows *sql.Rows, category, url string) []AdminSearchResult {
	results := []AdminSearchResult{}
	for rows.Next() {
		var title, detail string
		if err := rows.Scan(&title, &detail); err != nil {
			continue
		}
		results = append(results, AdminSearchResult{
			Category: category,
			Title:    title,
			Detail:   detail,
			URL:      url,
		})
	}
	return results
}

// adminSearchPattern builds a LIKE pattern, escaping the wildcard characters so
// a query of "%" cannot match every row.
func adminSearchPattern(query string) string {
	escaped := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(query)
	return "%" + escaped + "%"
}
