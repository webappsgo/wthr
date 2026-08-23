package swagger

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
)

func TestGetTheme(t *testing.T) {
	tests := []struct {
		name        string
		query       string
		cookieValue string
		hasCookie   bool
		want        Theme
	}{
		{
			name:  "query param dark takes precedence with no cookie",
			query: "theme=dark",
			want:  ThemeDark,
		},
		{
			name:  "query param light",
			query: "theme=light",
			want:  ThemeLight,
		},
		{
			name:  "query param auto",
			query: "theme=auto",
			want:  ThemeAuto,
		},
		{
			name:        "valid cookie used when no query param",
			cookieValue: "light",
			hasCookie:   true,
			want:        ThemeLight,
		},
		{
			name:        "invalid query param falls through to cookie",
			query:       "theme=garbage",
			cookieValue: "light",
			hasCookie:   true,
			want:        ThemeLight,
		},
		{
			name:        "invalid cookie falls through to default",
			cookieValue: "garbage",
			hasCookie:   true,
			want:        ThemeDark,
		},
		{
			name: "no query no cookie defaults to dark",
			want: ThemeDark,
		},
		{
			name:        "query param wins over a differing cookie",
			query:       "theme=light",
			cookieValue: "dark",
			hasCookie:   true,
			want:        ThemeLight,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			url := "/openapi"
			if tt.query != "" {
				url += "?" + tt.query
			}
			req := httptest.NewRequest("GET", url, nil)
			if tt.hasCookie {
				req.AddCookie(&http.Cookie{Name: "theme", Value: tt.cookieValue})
			}

			got := GetTheme(req)
			if got != tt.want {
				t.Errorf("GetTheme() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestHealthCheck(t *testing.T) {
	router := chi.NewRouter()
	router.Get("/openapi/health", HealthCheck())

	req := httptest.NewRequest("GET", "/openapi/health", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Fatalf("HealthCheck status = %d, want 200", w.Code)
	}

	var body map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("failed to decode response body: %v", err)
	}

	if body["status"] != "ok" {
		t.Errorf("body[status] = %q, want %q", body["status"], "ok")
	}
	if body["service"] != "swagger-ui" {
		t.Errorf("body[service] = %q, want %q", body["service"], "swagger-ui")
	}
}

// TestGetOpenAPIJSON_FileAbsent documents current behavior when
// ./docs/swagger.json is not present relative to the test working
// directory (verified absent at repo docs/swagger.json before writing this
// test): http.ServeFile does not panic on a missing file, it just writes a
// non-2xx status (typically 404). This test asserts that contract holds.
func TestGetOpenAPIJSON_FileAbsent(t *testing.T) {
	router := chi.NewRouter()
	router.Get("/openapi.json", GetOpenAPIJSON())

	req := httptest.NewRequest("GET", "/openapi.json", nil)
	w := httptest.NewRecorder()

	func() {
		defer func() {
			if r := recover(); r != nil {
				t.Fatalf("GetOpenAPIJSON panicked with missing file: %v", r)
			}
		}()
		router.ServeHTTP(w, req)
	}()

	if w.Code >= 200 && w.Code < 300 {
		t.Errorf("GetOpenAPIJSON with absent file: status = %d, want a non-2xx status", w.Code)
	}
}

// TestGetSwaggerUI exercises the swagger UI handler's code paths, including
// the theme cookie/query parsing branch, without asserting exact embedded
// HTML content (which depends on the swaggo/files embedded asset set).
func TestGetSwaggerUI(t *testing.T) {
	router := chi.NewRouter()
	router.Get("/openapi", GetSwaggerUI())
	router.Get("/openapi/*", GetSwaggerUI())

	t.Run("plain request does not panic", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/openapi/index.html", nil)
		w := httptest.NewRecorder()

		func() {
			defer func() {
				if r := recover(); r != nil {
					t.Fatalf("GetSwaggerUI panicked: %v", r)
				}
			}()
			router.ServeHTTP(w, req)
		}()

		if w.Code == 0 {
			t.Error("expected some HTTP status to be written, got 0")
		}
	})

	t.Run("theme query param does not panic", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/openapi/index.html?theme=dark", nil)
		w := httptest.NewRecorder()

		func() {
			defer func() {
				if r := recover(); r != nil {
					t.Fatalf("GetSwaggerUI panicked with theme query param: %v", r)
				}
			}()
			router.ServeHTTP(w, req)
		}()

		if w.Code == 0 {
			t.Error("expected some HTTP status to be written, got 0")
		}
	})

	t.Run("theme cookie does not panic", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/openapi/index.html", nil)
		req.AddCookie(&http.Cookie{Name: "theme", Value: "light"})
		w := httptest.NewRecorder()

		func() {
			defer func() {
				if r := recover(); r != nil {
					t.Fatalf("GetSwaggerUI panicked with theme cookie: %v", r)
				}
			}()
			router.ServeHTTP(w, req)
		}()

		if w.Code == 0 {
			t.Error("expected some HTTP status to be written, got 0")
		}
	})
}

func TestRegisterRoutes(t *testing.T) {
	router := chi.NewRouter()
	RegisterRoutes(router)

	var hasOpenAPI, hasOpenAPIWildcard bool
	err := chi.Walk(router, func(method string, route string, handler http.Handler, middlewares ...func(http.Handler) http.Handler) error {
		if method != "GET" {
			return nil
		}
		switch route {
		case "/openapi":
			hasOpenAPI = true
		case "/openapi/*":
			hasOpenAPIWildcard = true
		}
		return nil
	})
	if err != nil {
		t.Fatalf("chi.Walk failed: %v", err)
	}

	if !hasOpenAPI {
		t.Error("expected a GET route registered at /openapi")
	}
	if !hasOpenAPIWildcard {
		t.Error("expected a GET route registered at /openapi/*")
	}
}

func TestGetDarkThemeCSS(t *testing.T) {
	css := string(GetDarkThemeCSS())

	if css == "" {
		t.Fatal("GetDarkThemeCSS() returned empty string")
	}
	if !strings.Contains(css, "#282a36") {
		t.Error("GetDarkThemeCSS() missing expected Dracula background color #282a36")
	}
}

func TestGetLightThemeCSS(t *testing.T) {
	css := string(GetLightThemeCSS())

	if css == "" {
		t.Fatal("GetLightThemeCSS() returned empty string")
	}
	if !strings.Contains(css, "#ffffff") {
		t.Error("GetLightThemeCSS() missing expected light background color #ffffff")
	}
}

func TestThemeCSS_DarkAndLightDiffer(t *testing.T) {
	dark := string(GetDarkThemeCSS())
	light := string(GetLightThemeCSS())

	if dark == light {
		t.Error("GetDarkThemeCSS() and GetLightThemeCSS() returned identical output")
	}
}
