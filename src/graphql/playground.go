package graphql

import (
	"embed"
	"fmt"
	"io/fs"
	"net/http"
	"path"
	"strings"
)

//go:embed static/vendor/react.production.min.js
//go:embed static/vendor/react-dom.production.min.js
//go:embed static/vendor/graphiql.min.js
//go:embed static/vendor/graphiql.min.css
//go:embed static/graphiql-init.js
//go:embed static/graphiql-theme.css
var playgroundAssets embed.FS

// playgroundAssetPrefix is the route prefix the embedded playground assets
// are served under. Kept local (never a CDN) per the project's CSP
// (script-src 'self'), offline-first, and self-contained-binary rules.
const playgroundAssetPrefix = "/graphql/assets/"

// playgroundAssetContentType maps served file extensions to their MIME type;
// embed.FS carries no metadata and minimal container images may lack a
// system mime.types entry, so the type is set explicitly rather than relying
// on auto-detection.
var playgroundAssetContentType = map[string]string{
	".js":  "application/javascript; charset=utf-8",
	".css": "text/css; charset=utf-8",
}

// PlaygroundAssetHandler serves the embedded GraphiQL/React/init-script/theme
// assets that back the local (non-CDN) playground page.
func PlaygroundAssetHandler() http.HandlerFunc {
	sub, err := fs.Sub(playgroundAssets, "static")
	if err != nil {
		panic(fmt.Sprintf("graphql: invalid embedded playground assets: %v", err))
	}

	fileServer := http.StripPrefix(playgroundAssetPrefix, http.FileServer(http.FS(sub)))

	return func(w http.ResponseWriter, r *http.Request) {
		if contentType, ok := playgroundAssetContentType[path.Ext(r.URL.Path)]; ok {
			w.Header().Set("Content-Type", contentType)
		}
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		fileServer.ServeHTTP(w, r)
	}
}

// renderPlaygroundHTML builds the GraphiQL playground page from the
// project's own embedded assets. The init script reads its endpoint from a
// data attribute instead of an inline <script> block, so the page complies
// with the no-inline-JS / CSP script-src 'self' rules.
func renderPlaygroundHTML(theme Theme, endpoint string) string {
	themeClass := "theme-" + string(theme)

	var b strings.Builder
	b.WriteString("<!DOCTYPE html>\n<html lang=\"en\">\n<head>\n")
	b.WriteString("<meta charset=\"utf-8\">\n")
	b.WriteString("<meta name=\"viewport\" content=\"width=device-width, initial-scale=1\">\n")
	b.WriteString("<title>GraphQL Playground</title>\n")
	b.WriteString("<link rel=\"stylesheet\" href=\"" + playgroundAssetPrefix + "vendor/graphiql.min.css\">\n")
	b.WriteString("<link rel=\"stylesheet\" href=\"" + playgroundAssetPrefix + "graphiql-theme.css\">\n")
	b.WriteString("</head>\n<body>\n")
	b.WriteString("<div id=\"graphiql\" class=\"graphiql-container " + themeClass +
		"\" data-endpoint=\"" + endpoint + "\">Loading GraphQL Playground...</div>\n")
	b.WriteString("<script src=\"" + playgroundAssetPrefix + "vendor/react.production.min.js\"></script>\n")
	b.WriteString("<script src=\"" + playgroundAssetPrefix + "vendor/react-dom.production.min.js\"></script>\n")
	b.WriteString("<script src=\"" + playgroundAssetPrefix + "vendor/graphiql.min.js\"></script>\n")
	b.WriteString("<script src=\"" + playgroundAssetPrefix + "graphiql-init.js\"></script>\n")
	b.WriteString("</body>\n</html>\n")
	return b.String()
}
