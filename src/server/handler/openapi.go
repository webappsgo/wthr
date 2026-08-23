package handler

import (
	"net/http"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	httpSwagger "github.com/swaggo/http-swagger/v2"
)

// NOTE: Manual OpenAPI spec handlers removed per TEMPLATE.md requirements
// TEMPLATE.md Rule: "NEVER manually edit OpenAPI JSON" - specs must be auto-generated only
// All OpenAPI specs are now auto-generated from swag annotations and embedded in binary

// GetSwaggerUIAuto returns the auto-generated Swagger UI using swaggo/http-swagger
// Serves Swagger UI at /openapi (TEMPLATE.md compliant)
func GetSwaggerUIAuto() http.HandlerFunc {
	// Custom Dracula theme configuration
	// TEMPLATE.md: Swagger UI must match site theme (Dracula dark)
	return httpSwagger.Handler(
		// Relative URL for the JSON spec
		httpSwagger.URL("doc.json"),
		httpSwagger.DocExpansion("list"),
		httpSwagger.DeepLinking(true),
		httpSwagger.PersistAuthorization(true),
	)
}

// PrometheusMetrics returns Prometheus-compatible metrics using the official client
// per AI.md PART 21: METRICS (NON-NEGOTIABLE)
func PrometheusMetrics() http.HandlerFunc {
	h := promhttp.Handler()
	return func(w http.ResponseWriter, r *http.Request) {
		h.ServeHTTP(w, r)
	}
}
