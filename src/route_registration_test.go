package main

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

// routeMethodPaths collects the "METHOD PATH" pairs currently registered on
// the engine so route assertions do not depend on registration order.
func routeMethodPaths(r *gin.Engine) map[string]bool {
	registered := make(map[string]bool)
	for _, route := range r.Routes() {
		registered[route.Method+" "+route.Path] = true
	}
	return registered
}

// TestRegisterHealthRoutes covers the canonical health route set from AI.md
// PART 13: /server/healthz is canonical, the API counterpart lives under the
// versioned API path, /api/healthz is an unversioned alias mounting the same
// handler, and the root /healthz alias is config-gated (default off).
func TestRegisterHealthRoutes(t *testing.T) {
	gin.SetMode(gin.TestMode)

	frontend := func(c *gin.Context) { c.String(http.StatusOK, "frontend") }
	api := func(c *gin.Context) { c.String(http.StatusOK, "api") }

	t.Run("canonical routes without the root alias", func(t *testing.T) {
		r := gin.New()
		registerHealthRoutes(r, "/api/v1", false, frontend, api)
		registered := routeMethodPaths(r)

		for _, want := range []string{
			"GET /server/healthz",
			"GET /api/v1/server/healthz",
			"GET /api/healthz",
		} {
			if !registered[want] {
				t.Fatalf("route %q not registered, got %v", want, registered)
			}
		}
		if registered["GET /healthz"] {
			t.Fatal("root /healthz alias registered while server.healthz.root.enabled is false")
		}
	})

	t.Run("root alias mounted when enabled", func(t *testing.T) {
		r := gin.New()
		registerHealthRoutes(r, "/api/v1", true, frontend, api)

		if !routeMethodPaths(r)["GET /healthz"] {
			t.Fatal("root /healthz alias not registered while server.healthz.root.enabled is true")
		}

		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/healthz", nil))
		if w.Code != http.StatusOK {
			t.Fatalf("GET /healthz status = %d, want %d", w.Code, http.StatusOK)
		}
		if w.Body.String() != "frontend" {
			t.Fatalf("GET /healthz body = %q, want the /server/healthz handler output %q", w.Body.String(), "frontend")
		}
	})

	t.Run("unversioned api alias serves the same handler, never a redirect", func(t *testing.T) {
		r := gin.New()
		registerHealthRoutes(r, "/api/v1", false, frontend, api)

		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/healthz", nil))
		if w.Code != http.StatusOK {
			t.Fatalf("GET /api/healthz status = %d, want %d", w.Code, http.StatusOK)
		}
		if w.Body.String() != "api" {
			t.Fatalf("GET /api/healthz body = %q, want the API handler output %q", w.Body.String(), "api")
		}
	})
}

// TestRegisterGraphQLRoutes covers AI.md PART 14: the API-path GraphQL alias
// must mount the same handlers as /graphql instead of redirecting to it.
func TestRegisterGraphQLRoutes(t *testing.T) {
	gin.SetMode(gin.TestMode)

	query := func(c *gin.Context) { c.String(http.StatusOK, "query") }
	playground := func(c *gin.Context) { c.String(http.StatusOK, "playground") }
	assets := func(c *gin.Context) { c.String(http.StatusOK, "assets") }

	r := gin.New()
	registerGraphQLRoutes(r, "/api/v1", query, playground, assets)
	registered := routeMethodPaths(r)

	for _, want := range []string{
		"POST /graphql",
		"GET /graphql",
		"GET /graphql/assets/*filepath",
		"POST /api/v1/graphql",
		"GET /api/v1/graphql",
	} {
		if !registered[want] {
			t.Fatalf("route %q not registered, got %v", want, registered)
		}
	}

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/graphql", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("GET /api/v1/graphql status = %d, want %d (alias must not redirect)", w.Code, http.StatusOK)
	}
	if w.Body.String() != "playground" {
		t.Fatalf("GET /api/v1/graphql body = %q, want the playground output %q", w.Body.String(), "playground")
	}
}
