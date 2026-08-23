package swagger

import (
	"encoding/json"
	"html/template"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	httpSwagger "github.com/swaggo/http-swagger/v2"
)

// RegisterRoutes registers all Swagger/OpenAPI routes
// Per AI.md specification: /openapi for UI, /openapi.json for spec
func RegisterRoutes(router chi.Router) {
	// Swagger UI at /openapi
	router.Get("/openapi", GetSwaggerUI())
	router.Get("/openapi/*", GetSwaggerUI())
}

// indexData is the set of fields the themed index page template needs.
// Field names/values mirror ginSwagger.Config so behavior stays identical
// to the library's own default index.html - only the injected <style>
// theme block and page title differ.
type indexData struct {
	Title                    string
	URL                      string
	DocExpansion             string
	DeepLinking              bool
	PersistAuthorization     bool
	DefaultModelsExpandDepth int
	Oauth2DefaultClientID    string
	Oauth2RedirectURL        template.JS
	ThemeCSS                 template.CSS
}

// themedIndexTpl is our own Swagger UI index page. httpSwagger.Config exposes
// no CSS-injection hook (confirmed against the vendored library source), so
// this reimplements the same bootstrap markup as the library's internal
// template, reusing the identical relative asset URLs (swagger-ui.css,
// swagger-ui-bundle.js, swagger-ui-standalone-preset.js, doc.json) so
// httpSwagger.Handler continues serving those unchanged - only
// index.html/the bare /openapi route is served by this handler.
var themedIndexTpl = template.Must(template.New("swagger_index").Parse(`<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="UTF-8">
  <title>{{.Title}}</title>
  <link rel="stylesheet" type="text/css" href="./swagger-ui.css">
  <link rel="icon" type="image/png" href="./favicon-32x32.png" sizes="32x32">
  <link rel="icon" type="image/png" href="./favicon-16x16.png" sizes="16x16">
  <style>
    html { box-sizing: border-box; overflow: -moz-scrollbars-vertical; overflow-y: scroll; }
    *, *:before, *:after { box-sizing: inherit; }
    body { margin: 0; background: #fafafa; }
  </style>
  <style>
    {{.ThemeCSS}}
  </style>
</head>
<body>
  <div id="swagger-ui"></div>
  <script src="./swagger-ui-bundle.js" charset="UTF-8"></script>
  <script src="./swagger-ui-standalone-preset.js" charset="UTF-8"></script>
  <script>
    window.onload = function() {
      var configObject = {
        url: "{{.URL}}",
        dom_id: "#swagger-ui",
        deepLinking: {{.DeepLinking}},
        docExpansion: "{{.DocExpansion}}",
        persistAuthorization: {{.PersistAuthorization}},
        defaultModelsExpandDepth: {{.DefaultModelsExpandDepth}},
        presets: [
          SwaggerUIBundle.presets.apis,
          SwaggerUIStandalonePreset
        ],
        plugins: [
          SwaggerUIBundle.plugins.DownloadUrl
        ],
        layout: "StandaloneLayout",
        oauth2RedirectUrl: {{.Oauth2RedirectURL}}
      };
      {{if .Oauth2DefaultClientID}}
      configObject.initOAuth = {
        clientId: "{{.Oauth2DefaultClientID}}"
      };
      {{end}}
      window.ui = SwaggerUIBundle(configObject);
    };
  </script>
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
	docExpansion := "list"
	deepLinking := true
	persistAuthorization := true
	defaultModelsExpandDepth := 1
	docURL := "doc.json"

	assetHandler := httpSwagger.Handler(
		httpSwagger.URL(docURL),
		httpSwagger.DocExpansion(docExpansion),
		httpSwagger.DeepLinking(deepLinking),
		httpSwagger.PersistAuthorization(persistAuthorization),
		httpSwagger.DefaultModelsExpandDepth(httpSwagger.ShowModel),
	)

	return func(w http.ResponseWriter, r *http.Request) {
		if !isSwaggerIndexRequest(r.URL.Path) {
			assetHandler.ServeHTTP(w, r)
			return
		}

		theme := GetTheme(r)
		themeCSS := GetDarkThemeCSS()
		if theme == ThemeLight {
			themeCSS = GetLightThemeCSS()
		}

		data := indexData{
			Title:                    "wthr API Documentation",
			URL:                      docURL,
			DocExpansion:             docExpansion,
			DeepLinking:              deepLinking,
			PersistAuthorization:     persistAuthorization,
			DefaultModelsExpandDepth: defaultModelsExpandDepth,
			Oauth2DefaultClientID:    "",
			Oauth2RedirectURL:        template.JS("`${window.location.protocol}//${window.location.host}${window.location.pathname.split('/').slice(0, window.location.pathname.split('/').length - 1).join('/')}/oauth2-redirect.html`"),
			ThemeCSS:                 themeCSS,
		}

		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if err := themedIndexTpl.Execute(w, data); err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte("failed to render swagger ui"))
		}
	}
}

// GetOpenAPIJSON returns the OpenAPI JSON specification
// Auto-generated from swag annotations
func GetOpenAPIJSON() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		http.ServeFile(w, r, "./docs/swagger.json")
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
