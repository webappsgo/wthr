package util

import "strings"

// AI.md PART 16 "Robots Directive": {robots} is computed server-side per
// route and defaults to noindex,nofollow for every route that is not
// explicitly marked public (fail closed).
const (
	// RobotsIndexFollow is returned only for explicitly allow-listed public routes.
	RobotsIndexFollow = "index,follow"
	// RobotsNoIndexNoFollow is the fail-closed default for every other route.
	RobotsNoIndexNoFollow = "noindex,nofollow"
)

// publicExactPaths is the allow-list of public routes that are matched
// exactly, with no sub-paths. Every entry corresponds to a page-rendering
// route registered in src/main.go.
var publicExactPaths = map[string]bool{
	// Weather homepage (weatherHandler.HandleRoot)
	"/": true,
	// Historical weather page (historyHandler.ShowHistory)
	"/history": true,
	// Standard public /server/* informational pages
	"/server/about":   true,
	"/server/privacy": true,
	"/server/contact": true,
	"/server/help":    true,
	"/server/terms":   true,
}

// publicPathPrefixes is the allow-list of public route trees. A path
// matches when it equals the prefix or continues it at a segment boundary,
// so "/server/aboutus" never matches "/server/about".
var publicPathPrefixes = []string{
	// /weather/{location}
	"/weather",
	// /web and /web/{location}
	"/web",
	// /moon and /moon/{location}
	"/moon",
	// /earthquakes and /earthquakes/{location}
	"/earthquakes",
	// /severe-weather and /severe-weather/{location}
	"/severe-weather",
	// /severe/{type} and /severe/{type}/{location}
	"/severe",
}

// RobotsMetaForPath returns the value for the <meta name="robots"> tag for
// the given request path. Only explicitly allow-listed public routes are
// indexable; everything else (admin, API, user, auth, setup, debug, error
// pages, and any unrecognized path) fails closed to noindex,nofollow.
func RobotsMetaForPath(path string) string {
	normalized := normalizeRobotsPath(path)

	if publicExactPaths[normalized] {
		return RobotsIndexFollow
	}

	for _, prefix := range publicPathPrefixes {
		if normalized == prefix || strings.HasPrefix(normalized, prefix+"/") {
			return RobotsIndexFollow
		}
	}

	return RobotsNoIndexNoFollow
}

// normalizeRobotsPath lowercases the path, strips any query or fragment,
// guarantees a leading slash, and removes trailing slashes so that "/moon/"
// and "/Moon" resolve to the same allow-list entry as "/moon".
func normalizeRobotsPath(path string) string {
	if idx := strings.IndexAny(path, "?#"); idx >= 0 {
		path = path[:idx]
	}

	path = strings.ToLower(strings.TrimSpace(path))
	if path == "" {
		return "/"
	}

	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}

	trimmed := strings.TrimRight(path, "/")
	if trimmed == "" {
		return "/"
	}

	return trimmed
}
