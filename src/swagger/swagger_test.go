package swagger

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
)

func TestGetTheme(t *testing.T) {
	tests := []struct {
		name        string
		query       string
		cookieValue string
		hasCookie   bool
		want        Theme
	}{
		{
			name:  "query param dark takes precedence with no cookie",
			query: "theme=dark",
			want:  ThemeDark,
		},
		{
			name:  "query param light",
			query: "theme=light",
			want:  ThemeLight,
		},
		{
			name:  "query param auto",
			query: "theme=auto",
			want:  ThemeAuto,
		},
		{
			name:        "valid cookie used when no query param",
			cookieValue: "light",
			hasCookie:   true,
			want:        ThemeLight,
		},
		{
			name:        "invalid query param falls through to cookie",
			query:       "theme=garbage",
			cookieValue: "light",
			hasCookie:   true,
			want:        ThemeLight,
		},
		{
			name:        "invalid cookie falls through to default",
			cookieValue: "garbage",
			hasCookie:   true,
			want:        ThemeDark,
		},
		{
			name: "no query no cookie defaults to dark",
			want: ThemeDark,
		},
		{
			name:        "query param wins over a differing cookie",
			query:       "theme=light",
			cookieValue: "dark",
			hasCookie:   true,
			want:        ThemeLight,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			url := "/openapi"
			if tt.query != "" {
				url += "?" + tt.query
			}
			req := httptest.NewRequest("GET", url, nil)
			if tt.hasCookie {
				req.AddCookie(&http.Cookie{Name: "theme", Value: tt.cookieValue})
			}

			got := GetTheme(req)
			if got != tt.want {
				t.Errorf("GetTheme() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestGeneratedOpenAPIRoutesMatchRouter(t *testing.T) {
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve test file path")
	}
	root := filepath.Clean(filepath.Join(filepath.Dir(filename), "..", ".."))
	registered, err := ExtractRegisteredAPIRoutes(root)
	if err != nil {
		t.Fatalf("extract registered API routes: %v", err)
	}
	specification, err := OpenAPIJSON()
	if err != nil {
		t.Fatalf("read generated OpenAPI document: %v", err)
	}
	var document struct {
		Paths map[string]map[string]json.RawMessage `json:"paths"`
	}
	if err := json.Unmarshal(specification, &document); err != nil {
		t.Fatalf("decode generated OpenAPI document: %v", err)
	}
	var documented []Route
	for path, operations := range document.Paths {
		for method := range operations {
			documented = append(documented, Route{Method: strings.ToUpper(method), Path: path})
		}
	}
	if missing := routeDifference(registered, documented); len(missing) != 0 {
		t.Errorf("generated OpenAPI is missing registered routes: %s", formatRoutes(missing))
	}
	if missing := routeDifference(documented, registered); len(missing) != 0 {
		t.Errorf("generated OpenAPI has routes absent from the router: %s", formatRoutes(missing))
	}
	contract, err := ExtractContractRoutes(filepath.Join(root, "AI.md"), filepath.Join(root, "IDEA.md"))
	if err != nil {
		t.Fatalf("read API route contract: %v", err)
	}
	if missing := routeDifference(contract, registered); len(missing) != 0 {
		t.Errorf("API contract routes are not registered: %s", formatRoutes(missing))
	}
	if missing := routeDifference(registered, contract); len(missing) != 0 {
		t.Errorf("registered routes are missing from AI.md/IDEA.md contract: %s", formatRoutes(missing))
	}
}

// ExtractContractRoutes returns the routes selected for this product in IDEA.md.
// AI.md also includes routes for optional template features that this project
// may not adopt, so its route tables are not a product feature selection.
func ExtractContractRoutes(aiPath, ideaPath string) ([]Route, error) {
	aiData, err := os.ReadFile(aiPath)
	if err != nil {
		return nil, fmt.Errorf("read AI.md: %w", err)
	}
	if len(strings.TrimSpace(string(aiData))) == 0 {
		return nil, fmt.Errorf("AI.md is empty")
	}
	ideaData, err := os.ReadFile(ideaPath)
	if err != nil {
		return nil, fmt.Errorf("read IDEA.md: %w", err)
	}
	contract := map[string]Route{}
	add := func(method, path string) {
		method = strings.ToUpper(strings.TrimSpace(method))
		path = normalizeContractPath(path)
		if path == "" || (!contractAPIPathPattern.MatchString(path) && path != "/mcp") || strings.Contains(path, "*") || method == "ALL" {
			return
		}
		contract[method+" "+path] = Route{Method: method, Path: path}
	}
	ideaLines := strings.Split(string(ideaData), "\n")
	inInventory := false
	seenInventory := false
	for _, line := range ideaLines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "### ") {
			if seenInventory {
				break
			}
			inInventory = trimmed == "### API routes"
			seenInventory = seenInventory || inInventory
			continue
		}
		if !inInventory || !strings.HasPrefix(trimmed, "|") {
			continue
		}
		cells := markdownCells(trimmed)
		if len(cells) < 2 || strings.Contains(strings.ToLower(cells[0]), "method") {
			continue
		}
		method, path := strings.Trim(cells[0], "`"), strings.Trim(cells[1], "`")
		if strings.HasPrefix(path, "/api/") || path == "/api" || path == "/mcp" {
			for _, part := range strings.Split(method, ",") {
				add(part, path)
			}
		}
	}
	if !seenInventory {
		return nil, fmt.Errorf("IDEA.md is missing the required ### API routes subsection")
	}
	if len(contract) == 0 {
		return nil, fmt.Errorf("IDEA.md ### API routes subsection contains no API routes")
	}
	routes := make([]Route, 0, len(contract))
	for _, route := range contract {
		routes = append(routes, route)
	}
	sort.Slice(routes, func(i, j int) bool {
		if routes[i].Path == routes[j].Path {
			return routes[i].Method < routes[j].Method
		}
		return routes[i].Path < routes[j].Path
	})
	return routes, nil
}

var contractAPIPathPattern = regexp.MustCompile(`^/api(?:/|$)`)

func normalizeContractPath(path string) string {
	path = regexp.MustCompile(`/+`).ReplaceAllString(strings.TrimSpace(path), "/")
	if len(path) > 1 {
		path = strings.TrimSuffix(path, "/")
	}
	return path
}

func markdownCells(line string) []string {
	line = strings.TrimSpace(line)
	if !strings.HasPrefix(line, "|") {
		return nil
	}
	parts := strings.Split(strings.Trim(line, "|"), "|")
	cells := make([]string, 0, len(parts))
	for _, part := range parts {
		cells = append(cells, strings.TrimSpace(part))
	}
	return cells
}

func routeDifference(left, right []Route) []Route {
	rightSet := make(map[string]struct{}, len(right))
	for _, route := range right {
		rightSet[route.Method+" "+route.Path] = struct{}{}
	}
	var difference []Route
	for _, route := range left {
		if _, exists := rightSet[route.Method+" "+route.Path]; !exists {
			difference = append(difference, route)
		}
	}
	sort.Slice(difference, func(i, j int) bool {
		return difference[i].Path+" "+difference[i].Method < difference[j].Path+" "+difference[j].Method
	})
	return difference
}

func formatRoutes(routes []Route) string {
	formatted := make([]string, 0, len(routes))
	for _, route := range routes {
		formatted = append(formatted, fmt.Sprintf("%s %s", route.Method, route.Path))
	}
	return strings.Join(formatted, ", ")
}

func TestHealthCheck(t *testing.T) {
	router := chi.NewRouter()
	router.Get("/openapi/health", HealthCheck())

	req := httptest.NewRequest("GET", "/openapi/health", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Fatalf("HealthCheck status = %d, want 200", w.Code)
	}

	var body map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("failed to decode response body: %v", err)
	}

	if body["status"] != "ok" {
		t.Errorf("body[status] = %q, want %q", body["status"], "ok")
	}
	if body["service"] != "swagger-ui" {
		t.Errorf("body[service] = %q, want %q", body["service"], "swagger-ui")
	}
}

func TestGetOpenAPIJSONServesGeneratedDocument(t *testing.T) {
	router := chi.NewRouter()
	router.Get("/openapi.json", GetOpenAPIJSON())

	req := httptest.NewRequest("GET", "/openapi.json", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("GetOpenAPIJSON status = %d, want %d: %s", w.Code, http.StatusOK, w.Body.String())
	}
	if got := w.Header().Get("Content-Type"); !strings.HasPrefix(got, "application/json") {
		t.Fatalf("Content-Type = %q, want application/json", got)
	}
	var document map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &document); err != nil {
		t.Fatalf("generated OpenAPI document is invalid JSON: %v", err)
	}
	if document["openapi"] != "3.0.3" {
		t.Errorf("OpenAPI version = %v, want 3.0.3", document["openapi"])
	}
}

// TestGetSwaggerUI exercises the swagger UI handler's code paths, including
// the theme cookie/query parsing branch, without asserting exact embedded
// HTML content (which depends on the swaggo/files embedded asset set).
func TestGetSwaggerUI(t *testing.T) {
	router := chi.NewRouter()
	router.Get("/openapi", GetSwaggerUI())
	router.Get("/openapi/*", GetSwaggerUI())

	t.Run("plain request does not panic", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/openapi/index.html", nil)
		w := httptest.NewRecorder()

		func() {
			defer func() {
				if r := recover(); r != nil {
					t.Fatalf("GetSwaggerUI panicked: %v", r)
				}
			}()
			router.ServeHTTP(w, req)
		}()

		if w.Code == 0 {
			t.Error("expected some HTTP status to be written, got 0")
		}
	})

	t.Run("theme query param does not panic", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/openapi/index.html?theme=dark", nil)
		w := httptest.NewRecorder()

		func() {
			defer func() {
				if r := recover(); r != nil {
					t.Fatalf("GetSwaggerUI panicked with theme query param: %v", r)
				}
			}()
			router.ServeHTTP(w, req)
		}()

		if w.Code == 0 {
			t.Error("expected some HTTP status to be written, got 0")
		}
	})

	t.Run("theme cookie does not panic", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/openapi/index.html", nil)
		req.AddCookie(&http.Cookie{Name: "theme", Value: "light"})
		w := httptest.NewRecorder()

		func() {
			defer func() {
				if r := recover(); r != nil {
					t.Fatalf("GetSwaggerUI panicked with theme cookie: %v", r)
				}
			}()
			router.ServeHTTP(w, req)
		}()

		if w.Code == 0 {
			t.Error("expected some HTTP status to be written, got 0")
		}
	})
}

func TestRegisterRoutes(t *testing.T) {
	router := chi.NewRouter()
	RegisterRoutes(router)

	var hasOpenAPI, hasOpenAPIWildcard bool
	err := chi.Walk(router, func(method string, route string, handler http.Handler, middlewares ...func(http.Handler) http.Handler) error {
		if method != "GET" {
			return nil
		}
		switch route {
		case "/openapi":
			hasOpenAPI = true
		case "/openapi/*":
			hasOpenAPIWildcard = true
		}
		return nil
	})
	if err != nil {
		t.Fatalf("chi.Walk failed: %v", err)
	}

	if !hasOpenAPI {
		t.Error("expected a GET route registered at /openapi")
	}
	if !hasOpenAPIWildcard {
		t.Error("expected a GET route registered at /openapi/*")
	}
}

func TestGetDarkThemeCSS(t *testing.T) {
	css := string(GetDarkThemeCSS())

	if css == "" {
		t.Fatal("GetDarkThemeCSS() returned empty string")
	}
	if !strings.Contains(css, "#282a36") {
		t.Error("GetDarkThemeCSS() missing expected Dracula background color #282a36")
	}
}

func TestGetLightThemeCSS(t *testing.T) {
	css := string(GetLightThemeCSS())

	if css == "" {
		t.Fatal("GetLightThemeCSS() returned empty string")
	}
	if !strings.Contains(css, "#ffffff") {
		t.Error("GetLightThemeCSS() missing expected light background color #ffffff")
	}
}

func TestThemeCSS_DarkAndLightDiffer(t *testing.T) {
	dark := string(GetDarkThemeCSS())
	light := string(GetLightThemeCSS())

	if dark == light {
		t.Error("GetDarkThemeCSS() and GetLightThemeCSS() returned identical output")
	}
}
