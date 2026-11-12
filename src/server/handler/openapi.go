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
func GetSwaggerUIAuto() http.HandlerFunc {
	// Swagger UI inherits the shared theme system per AI.md PART 16
	ui := httpSwagger.Handler(
		// The spec is fetched from the unversioned alias (AI.md PART 14)
		httpSwagger.URL("/api/swagger"),
		httpSwagger.DocExpansion("list"),
		httpSwagger.DeepLinking(true),
		httpSwagger.PersistAuthorization(true),
	)

	return func(w http.ResponseWriter, r *http.Request) {
		if strings.TrimSuffix(r.URL.Path, "/") == "/server/docs/swagger" {
			r.URL.Path = "/server/docs/swagger/index.html"
		}
		ui(w, r)
	}
}
