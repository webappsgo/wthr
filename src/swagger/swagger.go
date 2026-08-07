package swagger

import (
	"html/template"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"
)

// RegisterRoutes registers all Swagger/OpenAPI routes
// Per AI.md specification: /openapi for UI, /openapi.json for spec
func RegisterRoutes(router *gin.Engine) {
	// Swagger UI at /openapi
	router.GET("/openapi", GetSwaggerUI())
	router.GET("/openapi/*any", GetSwaggerUI())
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

// themedIndexTpl is our own Swagger UI index page. ginSwagger.Config exposes
// no CSS-injection hook (confirmed against the vendored library source), so
// this reimplements the same bootstrap markup as the library's internal
// template, reusing the identical relative asset URLs (swagger-ui.css,
// swagger-ui-bundle.js, swagger-ui-standalone-preset.js, doc.json) so
// ginSwagger.CustomWrapHandler continues serving those unchanged - only
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
// that must keep being served unchanged by ginSwagger.CustomWrapHandler.
func isSwaggerIndexRequest(path string) bool {
	trimmed := strings.TrimSuffix(path, "/")
	return trimmed == "/openapi" || strings.HasSuffix(trimmed, "/index.html")
}

// GetSwaggerUI returns the Swagger UI handler with theme support.
// Serves auto-generated Swagger UI from swag annotations, injecting the
// resolved dark/light theme CSS (AI.md PART 16) into the index page while
// delegating every other asset path to the upstream library unchanged.
func GetSwaggerUI() gin.HandlerFunc {
	config := ginSwagger.Config{
		URL:                      "doc.json",
		DocExpansion:             "list",
		DeepLinking:              true,
		PersistAuthorization:     true,
		DefaultModelsExpandDepth: 1,
	}
	assetHandler := ginSwagger.CustomWrapHandler(&config, swaggerFiles.Handler)

	return func(c *gin.Context) {
		if !isSwaggerIndexRequest(c.Request.URL.Path) {
			assetHandler(c)
			return
		}

		theme := GetTheme(c)
		themeCSS := GetDarkThemeCSS()
		if theme == ThemeLight {
			themeCSS = GetLightThemeCSS()
		}

		data := indexData{
			Title:                    "wthr API Documentation",
			URL:                      config.URL,
			DocExpansion:             config.DocExpansion,
			DeepLinking:              config.DeepLinking,
			PersistAuthorization:     config.PersistAuthorization,
			DefaultModelsExpandDepth: config.DefaultModelsExpandDepth,
			Oauth2DefaultClientID:    config.Oauth2DefaultClientID,
			Oauth2RedirectURL:        template.JS("`${window.location.protocol}//${window.location.host}${window.location.pathname.split('/').slice(0, window.location.pathname.split('/').length - 1).join('/')}/oauth2-redirect.html`"),
			ThemeCSS:                 themeCSS,
		}

		c.Header("Content-Type", "text/html; charset=utf-8")
		if err := themedIndexTpl.Execute(c.Writer, data); err != nil {
			c.String(http.StatusInternalServerError, "failed to render swagger ui")
		}
	}
}

// GetOpenAPIJSON returns the OpenAPI JSON specification
// Auto-generated from swag annotations
func GetOpenAPIJSON() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Content-Type", "application/json")
		c.File("./docs/swagger.json")
	}
}

// HealthCheck for /openapi/health (separate from main health endpoint)
func HealthCheck() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"status":  "ok",
			"service": "swagger-ui",
		})
	}
}
