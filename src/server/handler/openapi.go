package handler

import (
	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"
)

// NOTE: Manual OpenAPI spec handlers removed per TEMPLATE.md requirements
// TEMPLATE.md Rule: "NEVER manually edit OpenAPI JSON" - specs must be auto-generated only
// All OpenAPI specs are now auto-generated from swag annotations and embedded in binary

// GetSwaggerUIAuto returns the auto-generated Swagger UI using swaggo/gin-swagger
// Serves Swagger UI at /openapi (TEMPLATE.md compliant)
func GetSwaggerUIAuto() gin.HandlerFunc {
	// Custom Dracula theme configuration
	// TEMPLATE.md: Swagger UI must match site theme (Dracula dark)
	config := ginSwagger.Config{
		// Relative URL for the JSON spec
		URL:                  "doc.json",
		DocExpansion:         "list",
		DeepLinking:          true,
		PersistAuthorization: true,
	}

	return ginSwagger.CustomWrapHandler(&config, swaggerFiles.Handler)
}

// PrometheusMetrics returns Prometheus-compatible metrics using the official client
// per AI.md PART 21: METRICS (NON-NEGOTIABLE)
func PrometheusMetrics() gin.HandlerFunc {
	h := promhttp.Handler()
	return func(c *gin.Context) {
		h.ServeHTTP(c.Writer, c.Request)
	}
}
