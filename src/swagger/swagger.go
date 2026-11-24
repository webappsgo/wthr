package swagger

//go:generate go run ./generate

import (
	"embed"
	"encoding/json"
	"html/template"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	httpSwagger "github.com/swaggo/http-swagger/v2"
	"github.com/webappsgo/wthr/src/server/buildstamp"
	"github.com/webappsgo/wthr/src/util"
)

// openAPIFiles contains the build-generated API contract.
//
//go:embed openapi.json
var openAPIFiles embed.FS

// RegisterRoutes registers all Swagger/OpenAPI routes
// Per AI.md specification: /openapi for UI, /openapi.json for spec
func RegisterRoutes(router chi.Router) {
	// Swagger UI at /openapi
	router.Get("/openapi", GetSwaggerUI())
	router.Get("/openapi/*", GetSwaggerUI())
}

// indexData is the set of fields the themed index page template needs.
// Field names and values mirror the upstream Swagger UI configuration.
type indexData struct {
	Title                    string
	URL                      string
	DocExpansion             string
	DeepLinking              bool
	PersistAuthorization     bool
	DefaultModelsExpandDepth int
	AppJS                    string
}

// themedIndexTpl is our own Swagger UI index page. httpSwagger.Config exposes
// no CSS-injection hook (confirmed against the vendored library source), so
// this reimplements the same bootstrap markup as the library's internal
// template, reusing the identical relative asset URLs (swagger-ui.css,
// swagger-ui-bundle.js, swagger-ui-standalone-preset.js, doc.json) so
// httpSwagger.Handler continues serving those unchanged - only
// index.html and the theme stylesheet are served by this handler.
var themedIndexTpl = template.Must(template.New("swagger_index").Parse(`<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="UTF-8">
  <title>{{.Title}}</title>
  <link rel="stylesheet" type="text/css" href="./swagger-ui.css">
  <link rel="icon" type="image/png" href="./favicon-32x32.png" sizes="32x32">
  <link rel="icon" type="image/png" href="./favicon-16x16.png" sizes="16x16">
  <link rel="stylesheet" type="text/css" href="./theme.css">
</head>
<body>
  <div id="swagger-ui" data-spec-url="{{.URL}}" data-doc-expansion="{{.DocExpansion}}" data-deep-linking="{{.DeepLinking}}" data-persist-authorization="{{.PersistAuthorization}}" data-models-depth="{{.DefaultModelsExpandDepth}}"></div>
  <script src="./swagger-ui-bundle.js" charset="UTF-8"></script>
  <script src="./swagger-ui-standalone-preset.js" charset="UTF-8"></script>
  <script src="{{.AppJS}}" defer></script>
</body>
</html>
`))

// isSwaggerIndexRequest reports whether the request targets the Swagger UI
// index page (bare /openapi, /openapi/, or /openapi/index.html) rather than
// a static asset (doc.json, swagger-ui.js, favicons, oauth2-redirect.html)
// that must keep being served unchanged by httpSwagger.Handler.
func isSwaggerIndexRequest(path string) bool {
	trimmed := strings.TrimSuffix(path, "/")
	return trimmed == "/openapi" || strings.HasSuffix(trimmed, "/index.html")
}

// GetSwaggerUI returns the Swagger UI handler with theme support.
// Serves auto-generated Swagger UI from swag annotations, injecting the
// resolved dark/light theme CSS (AI.md PART 16) into the index page while
// delegating every other asset path to the upstream library unchanged.
func GetSwaggerUI() http.HandlerFunc {
	return GetSwaggerUIWithURL("doc.json")
}

// GetSwaggerUIWithURL serves Swagger UI configured to load a specific OpenAPI document URL.
func GetSwaggerUIWithURL(docURL string) http.HandlerFunc {
	docExpansion := "list"
	deepLinking := true
	persistAuthorization := true
	defaultModelsExpandDepth := 1
	assetHandler := httpSwagger.Handler(
		httpSwagger.URL(docURL),
		httpSwagger.DocExpansion(docExpansion),
		httpSwagger.DeepLinking(deepLinking),
		httpSwagger.PersistAuthorization(persistAuthorization),
		httpSwagger.DefaultModelsExpandDepth(httpSwagger.ShowModel),
	)

	return func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(strings.TrimSuffix(r.URL.Path, "/"), "/theme.css") {
			w.Header().Set("Content-Type", "text/css; charset=utf-8")
			if GetTheme(r) == ThemeLight {
				_, _ = w.Write([]byte(GetLightThemeCSS()))
				return
			}
			_, _ = w.Write([]byte(GetDarkThemeCSS()))
			return
		}
		if !isSwaggerIndexRequest(r.URL.Path) {
			assetHandler.ServeHTTP(w, r)
			return
		}

		data := indexData{
			Title:                    "wthr API Documentation",
			URL:                      docURL,
			DocExpansion:             docExpansion,
			DeepLinking:              deepLinking,
			PersistAuthorization:     persistAuthorization,
			DefaultModelsExpandDepth: defaultModelsExpandDepth,
			AppJS:                    buildstamp.Asset("/static/js/app.js"),
		}

		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if err := themedIndexTpl.Execute(w, data); err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte("failed to render swagger ui"))
		}
	}
}

// OpenAPIJSON returns the build-generated OpenAPI document.
func OpenAPIJSON() ([]byte, error) {
	return openAPIFiles.ReadFile("openapi.json")
}

// GetOpenAPIJSON serves the build-generated OpenAPI JSON document.
func GetOpenAPIJSON() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		spec, err := OpenAPIJSON()
		if err != nil {
			http.Error(w, "OpenAPI specification unavailable", http.StatusInternalServerError)
			return
		}
		var document map[string]interface{}
		if err := json.Unmarshal(spec, &document); err != nil {
			http.Error(w, "OpenAPI specification unavailable", http.StatusInternalServerError)
			return
		}
		document["servers"] = []map[string]string{{"url": util.BuildURL(r, ""), "description": "Current application origin"}}
		encoded, err := json.MarshalIndent(document, "", "  ")
		if err != nil {
			http.Error(w, "OpenAPI specification unavailable", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(encoded)
	}
}

// HealthCheck for /openapi/health (separate from main health endpoint)
func HealthCheck() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"status":  "ok",
			"service": "swagger-ui",
		})
	}
}
