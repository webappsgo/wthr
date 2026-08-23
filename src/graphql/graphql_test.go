package graphql

import (
	"context"
	"database/sql"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/webappsgo/wthr/src/database"
	"github.com/webappsgo/wthr/src/server/handler"
	models "github.com/webappsgo/wthr/src/server/model"
	"github.com/webappsgo/wthr/src/server/service"
)

// TestGetTheme mirrors src/swagger/swagger_test.go's TestGetTheme pattern
// (query param > cookie > default), applied to graphql/theme.go's local
// GetTheme implementation, which is a separate function from swagger's.
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
			url := "/graphql"
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

// Fixed zones far from UTC in both directions, so the timestamp assertions in
// this package hold on a host in any timezone: the stored wall-clock text of a
// west-zone row reads as long past while the instant is still in the future,
// and an east-zone row reads as far future while its instant has already gone.
// The zone abbreviations must be three to five upper-case letters ending in T:
// that is the only shape time.Parse accepts for the trailing "MST" element of
// the local layout below, so a name like "EST13" would make every fixture
// written in that zone unparseable and the tests would assert nothing.
var (
	graphQLZoneWest = time.FixedZone("WST", -11*60*60)
	graphQLZoneEast = time.FixedZone("EAST", 13*60*60)
)

// graphQLLocalTimestampLayout is the layout modernc.org/sqlite writes when a Go
// time.Time is bound directly, which is how older builds stored expires_at.
const graphQLLocalTimestampLayout = "2006-01-02 15:04:05.999999999 -0700 MST"

// newGraphQLTimestampTestDB brings up a real DualDB and installs it as the
// package global, which is what buildGraphQLAdminSessionContext reads through
// database.GetServerDB().
func newGraphQLTimestampTestDB(t *testing.T) *database.DualDB {
	t.Helper()
	ddb, err := database.InitDualDB(t.TempDir())
	if err != nil {
		t.Fatalf("InitDualDB: %v", err)
	}
	database.SetGlobalDualDB(ddb)
	t.Cleanup(func() {
		database.SetGlobalDualDB(nil)
		ddb.Close()
	})
	return ddb
}

// TestBuildGraphQLAdminSessionContext_ExpiryIsJudgedByInstantNotText proves the
// GraphQL admin session lookup decides expiry on the absolute instant in Go
// rather than by a lexicographic TEXT comparison in SQL.
//
// Against the previous "WHERE id = ? AND expires_at > CURRENT_TIMESTAMP" query
// both zone cases failed: the still-valid west-zone session sorted below
// CURRENT_TIMESTAMP, produced sql.ErrNoRows and left the request anonymous,
// while the already-expired east-zone session sorted above it and was handed a
// full admin context. The unparseable case proves the replacement fails closed.
func TestBuildGraphQLAdminSessionContext_ExpiryIsJudgedByInstantNotText(t *testing.T) {
	now := time.Now()

	cases := []struct {
		name        string
		expiresAt   string
		wantAdmin   bool
		explanation string
	}{
		{
			name:        "live-west-zone-text-reads-as-past",
			expiresAt:   now.Add(time.Hour).In(graphQLZoneWest).Format(graphQLLocalTimestampLayout),
			wantAdmin:   true,
			explanation: "session expires an hour from now and must authenticate",
		},
		{
			name:        "expired-east-zone-text-reads-as-future",
			expiresAt:   now.Add(-time.Hour).In(graphQLZoneEast).Format(graphQLLocalTimestampLayout),
			wantAdmin:   false,
			explanation: "session expired an hour ago and must stay anonymous",
		},
		{
			name:        "unparseable-is-rejected",
			expiresAt:   "whenever-o-clock",
			wantAdmin:   false,
			explanation: "an expiry this project cannot parse must fail closed",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ddb := newGraphQLTimestampTestDB(t)

			admin, err := (&models.AdminModel{DB: ddb.Server}).Create("sessionadmin", "sessionadmin@example.com", "password123", true)
			if err != nil {
				t.Fatalf("create admin: %v", err)
			}

			const token = "graphql-zone-safety-session"
			if _, err := ddb.Server.Exec(`
				INSERT INTO server_admin_sessions (id, admin_id, ip_address, user_agent, expires_at, created_at)
				VALUES (?, ?, ?, ?, ?, ?)
			`, token, admin.ID, "127.0.0.1", "test-agent", tc.expiresAt, now.UTC().Format("2006-01-02 15:04:05")); err != nil {
				t.Fatalf("seed admin session: %v", err)
			}

			ctx, err := buildGraphQLAdminSessionContext(context.Background(), token)
			if err != nil {
				t.Fatalf("buildGraphQLAdminSessionContext: %v", err)
			}

			gotAdminID, gotAdmin := ctx.Value(ctxKeyAdminID).(int)
			if gotAdmin != tc.wantAdmin {
				t.Errorf("admin context present = %v, want %v (%s)", gotAdmin, tc.wantAdmin, tc.explanation)
			}
			if tc.wantAdmin && gotAdminID != int(admin.ID) {
				t.Errorf("admin_id = %d, want %d", gotAdminID, admin.ID)
			}
		})
	}
}
