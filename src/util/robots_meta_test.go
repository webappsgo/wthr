package util

import "testing"

// TestRobotsMetaForPath verifies that only explicitly allow-listed public
// routes are indexable and that every other path fails closed.
func TestRobotsMetaForPath(t *testing.T) {
	tests := []struct {
		name string
		path string
		want string
	}{
		// Explicitly public: homepage and weather pages
		{"root", "/", RobotsIndexFollow},
		{"weather location", "/weather/london", RobotsIndexFollow},
		{"weather bare", "/weather", RobotsIndexFollow},
		{"web interface", "/web", RobotsIndexFollow},
		{"web location", "/web/paris", RobotsIndexFollow},
		{"moon", "/moon", RobotsIndexFollow},
		{"moon location", "/moon/tokyo", RobotsIndexFollow},
		{"history", "/history", RobotsIndexFollow},
		{"earthquakes", "/earthquakes", RobotsIndexFollow},
		{"earthquakes location", "/earthquakes/alaska", RobotsIndexFollow},
		{"severe weather", "/severe-weather", RobotsIndexFollow},
		{"severe weather location", "/severe-weather/miami", RobotsIndexFollow},
		{"severe by type", "/severe/tornado", RobotsIndexFollow},
		{"severe by type and location", "/severe/tornado/kansas", RobotsIndexFollow},

		// Explicitly public: standard /server/* informational pages
		{"about", "/server/about", RobotsIndexFollow},
		{"privacy", "/server/privacy", RobotsIndexFollow},
		{"contact", "/server/contact", RobotsIndexFollow},
		{"help", "/server/help", RobotsIndexFollow},
		{"terms", "/server/terms", RobotsIndexFollow},

		// Admin panel
		{"admin root", "/server/admin", RobotsNoIndexNoFollow},
		{"admin dashboard", "/server/admin/dashboard", RobotsNoIndexNoFollow},
		{"admin config", "/server/admin/config/settings", RobotsNoIndexNoFollow},
		{"admin custom path", "/server/controlpanel/dashboard", RobotsNoIndexNoFollow},

		// API routes
		{"api root", "/api", RobotsNoIndexNoFollow},
		{"api versioned", "/api/v1/weather", RobotsNoIndexNoFollow},
		{"api search", "/api/v1/search", RobotsNoIndexNoFollow},
		{"api autodiscover", "/api/autodiscover", RobotsNoIndexNoFollow},

		// User routes
		{"users dashboard", "/users/dashboard", RobotsNoIndexNoFollow},
		{"users profile", "/users/profile", RobotsNoIndexNoFollow},
		{"users tokens", "/users/tokens", RobotsNoIndexNoFollow},

		// Auth routes
		{"login", "/server/auth/login", RobotsNoIndexNoFollow},
		{"register", "/server/auth/register", RobotsNoIndexNoFollow},
		{"password reset", "/server/auth/password/reset", RobotsNoIndexNoFollow},
		{"logout", "/server/auth/logout", RobotsNoIndexNoFollow},

		// Setup routes
		{"admin setup", "/server/admin/config/setup", RobotsNoIndexNoFollow},
		{"server setup", "/server/setup", RobotsNoIndexNoFollow},

		// Debug, health, and internal endpoints
		{"debug pprof", "/debug/pprof/", RobotsNoIndexNoFollow},
		{"debug vars", "/debug/vars", RobotsNoIndexNoFollow},
		{"debug ip", "/debug/ip", RobotsNoIndexNoFollow},
		{"healthz", "/server/healthz", RobotsNoIndexNoFollow},
		{"health", "/health", RobotsNoIndexNoFollow},
		{"metrics", "/metrics", RobotsNoIndexNoFollow},

		// Error pages
		{"error 404", "/errors/404", RobotsNoIndexNoFollow},
		{"error 500", "/errors/500", RobotsNoIndexNoFollow},

		// Unknown / unmatched paths fail closed
		{"unknown path", "/some/unknown/page", RobotsNoIndexNoFollow},
		{"empty path", "", RobotsIndexFollow},
		{"catch-all location", "/albany", RobotsNoIndexNoFollow},
		{"random segment", "/zzzz", RobotsNoIndexNoFollow},

		// Trailing-slash variations resolve to the same decision
		{"root double slash", "//", RobotsIndexFollow},
		{"moon trailing slash", "/moon/", RobotsIndexFollow},
		{"about trailing slash", "/server/about/", RobotsIndexFollow},
		{"earthquakes trailing slash", "/earthquakes/", RobotsIndexFollow},
		{"admin trailing slash", "/server/admin/", RobotsNoIndexNoFollow},
		{"api trailing slash", "/api/v1/", RobotsNoIndexNoFollow},

		// Case variations resolve to the same decision
		{"uppercase moon", "/MOON", RobotsIndexFollow},
		{"mixed case about", "/Server/About", RobotsIndexFollow},
		{"mixed case weather", "/Weather/London", RobotsIndexFollow},
		{"uppercase admin", "/SERVER/ADMIN", RobotsNoIndexNoFollow},

		// A public route as a bare string prefix, but a different route
		{"aboutus not about", "/server/aboutus", RobotsNoIndexNoFollow},
		{"helpdesk not help", "/server/helpdesk", RobotsNoIndexNoFollow},
		{"terms-of-sale not terms", "/server/terms-of-sale", RobotsNoIndexNoFollow},
		{"moonshot not moon", "/moonshot", RobotsNoIndexNoFollow},
		{"weathermap not weather", "/weathermap", RobotsNoIndexNoFollow},
		{"webhooks not web", "/webhooks", RobotsNoIndexNoFollow},
		{"severe-storms not severe", "/severe-storms", RobotsNoIndexNoFollow},
		{"earthquakes-archive not earthquakes", "/earthquakes-archive", RobotsNoIndexNoFollow},
		{"historyx not history", "/historyx", RobotsNoIndexNoFollow},
		{"history subpath not allow-listed", "/history/2024", RobotsNoIndexNoFollow},

		// Query strings and fragments are ignored for the decision
		{"public path with query", "/moon?units=metric", RobotsIndexFollow},
		{"private path with query", "/server/admin?tab=users", RobotsNoIndexNoFollow},
		{"public path with fragment", "/server/help#api", RobotsIndexFollow},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := RobotsMetaForPath(tt.path)
			if got != tt.want {
				t.Errorf("RobotsMetaForPath(%q) = %q, want %q", tt.path, got, tt.want)
			}
		})
	}
}

// TestRobotsMetaForPathFailsClosed asserts that anything outside the
// allow-list is never indexable, regardless of shape.
func TestRobotsMetaForPathFailsClosed(t *testing.T) {
	paths := []string{
		"/server",
		"/server/",
		"/server/anything",
		"/graphql",
		"/openapi",
		"/docs",
		"/examples",
		"/robots.txt",
		"/sitemap.xml",
		"/.well-known/security.txt",
		"/static/js/app.js",
		"/ws/notifications",
		"/theme",
	}

	for _, path := range paths {
		t.Run(path, func(t *testing.T) {
			if got := RobotsMetaForPath(path); got != RobotsNoIndexNoFollow {
				t.Errorf("RobotsMetaForPath(%q) = %q, want %q", path, got, RobotsNoIndexNoFollow)
			}
		})
	}
}
