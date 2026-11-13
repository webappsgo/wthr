package handler

import (
	"net/http"
	"strings"

	httpSwagger "github.com/swaggo/http-swagger/v2"
)

// NOTE: Manual OpenAPI spec handlers removed per AI.md PART 14
// AI.md PART 14 rule: never manually edit generated OpenAPI JSON - it is
// build-time generated from swag annotations and embedded in the binary

// GetSwaggerUIAuto returns the auto-generated Swagger UI using swaggo/http-swagger
// Serves Swagger UI at /openapi per AI.md PART 14
func GetSwaggerUIAuto(apiPath string) http.HandlerFunc {
	// Swagger UI inherits the shared theme system per AI.md PART 16
	ui := httpSwagger.Handler(
		// The spec is fetched from the versioned canonical route (AI.md PART 14)
		httpSwagger.URL(apiPath+"/server/swagger"),
		httpSwagger.DocExpansion("list"),
		httpSwagger.DeepLinking(true),
		httpSwagger.PersistAuthorization(true),
	)

	return func(w http.ResponseWriter, r *http.Request) {
		if strings.TrimSuffix(r.URL.Path, "/") == "/server/docs/swagger" {
			r.URL.Path = "/server/docs/swagger/index.html"
			r.RequestURI = r.URL.Path
		}
		ui(w, r)
	}
}
