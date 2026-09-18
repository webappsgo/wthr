package handler

import (
	"net/http"

	httpSwagger "github.com/swaggo/http-swagger/v2"
)

// NOTE: Manual OpenAPI spec handlers removed per AI.md PART 14
// AI.md PART 14 rule: never manually edit generated OpenAPI JSON - it is
// build-time generated from swag annotations and embedded in the binary

// GetSwaggerUIAuto returns the auto-generated Swagger UI using swaggo/http-swagger
// Serves Swagger UI at /openapi per AI.md PART 14
func GetSwaggerUIAuto() http.HandlerFunc {
	// Swagger UI inherits the shared theme system per AI.md PART 16
	return httpSwagger.Handler(
		// Relative URL for the JSON spec
		httpSwagger.URL("doc.json"),
		httpSwagger.DocExpansion("list"),
		httpSwagger.DeepLinking(true),
		httpSwagger.PersistAuthorization(true),
	)
}
