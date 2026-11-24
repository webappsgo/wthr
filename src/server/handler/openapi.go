package handler

import (
	"net/http"
	"strings"

	"github.com/webappsgo/wthr/src/swagger"
)

// GetOpenAPIJSON serves the OpenAPI document generated before the application
// build and embedded in the binary.
func GetOpenAPIJSON() http.HandlerFunc {
	return swagger.GetOpenAPIJSON()
}

// GetSwaggerUIAuto returns the auto-generated Swagger UI using swaggo/http-swagger
// Serves Swagger UI at /openapi per AI.md PART 14
func GetSwaggerUIAuto(apiPath string) http.HandlerFunc {
	ui := swagger.GetSwaggerUIWithURL(apiPath + "/server/swagger")

	return func(w http.ResponseWriter, r *http.Request) {
		if strings.TrimSuffix(r.URL.Path, "/") == "/server/docs/swagger" {
			r.URL.Path = "/server/docs/swagger/index.html"
			r.RequestURI = r.URL.Path
		}
		ui(w, r)
	}
}
