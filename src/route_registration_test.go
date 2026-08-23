package main

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
)

// routeMethodPaths collects the "METHOD PATH" pairs currently registered on
// the router so route assertions do not depend on registration order.
func routeMethodPaths(r chi.Router) map[string]bool {
	registered := make(map[string]bool)
	_ = chi.Walk(r, func(method, route string, handler http.Handler, middlewares ...func(http.Handler) http.Handler) error {
		registered[method+" "+route] = true
		return nil
	})
	return registered
}

// TestRegisterHealthRoutes covers the canonical health route set from AI.md
// PART 13: /server/healthz is canonical, the API counterpart lives under the
// versioned API path, /api/healthz is an unversioned alias mounting the same
// handler, and the root /healthz alias is config-gated (default off).
func TestRegisterHealthRoutes(t *testing.T) {
	frontend := func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte("frontend")) }
	api := func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte("api")) }

	t.Run("canonical routes without the root alias", func(t *testing.T) {
		r := chi.NewRouter()
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
		r := chi.NewRouter()
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
		r := chi.NewRouter()
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
	query := func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte("query")) }
	playground := func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte("playground")) }
	assets := func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte("assets")) }

	r := chi.NewRouter()
	registerGraphQLRoutes(r, "/api/v1", query, playground, assets)
	registered := routeMethodPaths(r)

	for _, want := range []string{
		"POST /graphql",
		"GET /graphql",
		"GET /graphql/assets/*",
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
