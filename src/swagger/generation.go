package swagger

import "github.com/webappsgo/wthr/src/swagger/generator"

// Route identifies one API route in the generated OpenAPI contract.
type Route = generator.Route

// ExtractRegisteredAPIRoutes returns the API routes found in the server router.
func ExtractRegisteredAPIRoutes(root string) ([]Route, error) {
	return generator.ExtractRegisteredAPIRoutes(root)
}

// GenerateOpenAPI writes the deterministic OpenAPI artifact used by the server.
func GenerateOpenAPI(root string) error {
	return generator.GenerateOpenAPI(root)
}
