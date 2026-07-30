package graphql

import (
	"database/sql"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/webappsgo/wthr/src/server/handler"
	"github.com/webappsgo/wthr/src/server/service"
)

// TestGetTheme mirrors src/swagger/swagger_test.go's TestGetTheme pattern
// (query param > cookie > default), applied to graphql/theme.go's local
// GetTheme implementation, which is a separate function from swagger's.
func TestGetTheme(t *testing.T) {
	gin.SetMode(gin.TestMode)

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
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)

			url := "/graphql"
			if tt.query != "" {
				url += "?" + tt.query
			}
			req := httptest.NewRequest("GET", url, nil)
			if tt.hasCookie {
				req.AddCookie(&http.Cookie{Name: "theme", Value: tt.cookieValue})
			}
			c.Request = req

			got := GetTheme(c)
			if got != tt.want {
				t.Errorf("GetTheme() = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestGetDarkThemeCSS and TestGetLightThemeCSS assert the GraphiQL theme CSS
// helpers return non-empty, theme-appropriate content, matching the same
// assertions swagger_test.go makes for its own (separate) copy of this CSS.
func TestGetDarkThemeCSS(t *testing.T) {
	css := GetDarkThemeCSS()

	if css == "" {
		t.Fatal("GetDarkThemeCSS() returned empty string")
	}
	if !strings.Contains(css, "#282a36") {
		t.Error("GetDarkThemeCSS() missing expected Dracula background color #282a36")
	}
}

func TestGetLightThemeCSS(t *testing.T) {
	css := GetLightThemeCSS()

	if css == "" {
		t.Fatal("GetLightThemeCSS() returned empty string")
	}
	if !strings.Contains(css, "#ffffff") {
		t.Error("GetLightThemeCSS() missing expected light background color #ffffff")
	}
}

func TestThemeCSS_DarkAndLightDiffer(t *testing.T) {
	dark := GetDarkThemeCSS()
	light := GetLightThemeCSS()

	if dark == light {
		t.Error("GetDarkThemeCSS() and GetLightThemeCSS() returned identical output")
	}
}

// TestNewResolver verifies the constructor assigns every field positionally
// and correctly -- a real bug here (e.g. two params swapped) would silently
// wire the wrong handler/service into the GraphQL resolver tree at runtime.
func TestNewResolver(t *testing.T) {
	serverDB := &sql.DB{}
	usersDB := &sql.DB{}
	weatherService := &service.WeatherService{}
	apiHandler := &handler.APIHandler{}
	authHandler := &handler.AuthHandler{}
	locationHandler := &handler.LocationHandler{}
	notificationHandler := &handler.NotificationHandler{}
	adminHandler := &handler.AdminHandler{}
	torAdminHandler := &handler.TorAdminHandler{}
	settingsHandler := &handler.AdminSettingsHandler{}
	schedulerHandler := &handler.SchedulerHandler{}
	earthquakeHandler := &handler.EarthquakeHandler{}
	hurricaneHandler := &handler.HurricaneHandler{}
	severeWeatherHandler := &handler.SevereWeatherHandler{}
	moonHandler := &handler.MoonHandler{}

	r := NewResolver(
		serverDB, usersDB,
		weatherService,
		apiHandler,
		authHandler,
		locationHandler,
		notificationHandler,
		adminHandler,
		torAdminHandler,
		settingsHandler,
		schedulerHandler,
		earthquakeHandler,
		hurricaneHandler,
		severeWeatherHandler,
		moonHandler,
	)

	if r == nil {
		t.Fatal("NewResolver() returned nil")
	}
	if r.ServerDB != serverDB {
		t.Error("ServerDB not wired to the correct argument")
	}
	if r.UsersDB != usersDB {
		t.Error("UsersDB not wired to the correct argument")
	}
	if r.WeatherService != weatherService {
		t.Error("WeatherService not wired to the correct argument")
	}
	if r.APIHandler != apiHandler {
		t.Error("APIHandler not wired to the correct argument")
	}
	if r.AuthHandler != authHandler {
		t.Error("AuthHandler not wired to the correct argument")
	}
	if r.LocationHandler != locationHandler {
		t.Error("LocationHandler not wired to the correct argument")
	}
	if r.NotificationHandler != notificationHandler {
		t.Error("NotificationHandler not wired to the correct argument")
	}
	if r.AdminHandler != adminHandler {
		t.Error("AdminHandler not wired to the correct argument")
	}
	if r.TorAdminHandler != torAdminHandler {
		t.Error("TorAdminHandler not wired to the correct argument")
	}
	if r.SettingsHandler != settingsHandler {
		t.Error("SettingsHandler not wired to the correct argument")
	}
	if r.SchedulerHandler != schedulerHandler {
		t.Error("SchedulerHandler not wired to the correct argument")
	}
	if r.EarthquakeHandler != earthquakeHandler {
		t.Error("EarthquakeHandler not wired to the correct argument")
	}
	if r.HurricaneHandler != hurricaneHandler {
		t.Error("HurricaneHandler not wired to the correct argument")
	}
	if r.SevereWeatherHandler != severeWeatherHandler {
		t.Error("SevereWeatherHandler not wired to the correct argument")
	}
	if r.MoonHandler != moonHandler {
		t.Error("MoonHandler not wired to the correct argument")
	}
}
