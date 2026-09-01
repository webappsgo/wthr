package middleware

import (
	"context"
	"database/sql"
	"net/http"
	"strings"

	"github.com/webappsgo/wthr/src/database"
	"github.com/webappsgo/wthr/src/server"
	"github.com/webappsgo/wthr/src/server/model"
	"github.com/webappsgo/wthr/src/server/reqctx"
)

// ServerContext holds server-wide configuration per AI.md PART 16
type ServerContext struct {
	Title       string
	Tagline     string
	Description string
	Version     string
	// Current user language (AI.md PART 31)
	Lang string
	// Resolved theme class for <html> (AI.md PART 16): theme-dark/theme-light/theme-auto
	ThemeClass string
	// Raw resolved theme mode (AI.md PART 16): dark/light/auto - the value the
	// toggle feeds to nextTheme so its POST target is always server-computed
	Theme string
	// SEO fields per AI.md PART 16
	Keywords      string
	Author        string
	OGImage       string
	TwitterHandle string
	// Site verification per AI.md PART 16
	VerifyGoogle    string
	VerifyBing      string
	VerifyYandex    string
	VerifyBaidu     string
	VerifyPinterest string
	VerifyFacebook  string
}

// resolveThemeFromRequest mirrors server.ResolveTheme's Theme Detection Flow
// (theme cookie, defaulting to dark) without depending on gin.Context, since
// the middleware package cannot import gin. It reuses the server package's
// pure, gin-independent exports (ThemeCookieName, IsValidTheme, DefaultTheme)
// rather than duplicating their values.
func resolveThemeFromRequest(r *http.Request) string {
	if cookie, err := r.Cookie(server.ThemeCookieName); err == nil && server.IsValidTheme(cookie.Value) {
		return cookie.Value
	}
	return server.DefaultTheme
}

// InjectServerContext adds server configuration to all requests
func InjectServerContext(db *sql.DB, version string) func(http.Handler) http.Handler {
	settingsModel := &model.SettingsModel{DB: database.GetServerDB()}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Get server settings
			title := settingsModel.GetString("server.title", "Weather")
			tagline := settingsModel.GetString("server.tagline", "Your personal weather dashboard")
			description := settingsModel.GetString("server.description", "A comprehensive platform for weather forecasts, moon phases, earthquakes, and hurricane tracking.")

			// Get user language from i18n middleware
			lang, exists := reqctx.Get(r.Context(), "lang")
			if !exists {
				lang = "en"
			}

			// Get SEO settings per AI.md PART 16
			keywords := settingsModel.GetString("seo.keywords", "weather, forecast, alerts, earthquakes, hurricanes, moon phases")
			author := settingsModel.GetString("seo.author", "")
			ogImage := settingsModel.GetString("seo.og_image", "")
			twitterHandle := settingsModel.GetString("seo.twitter_handle", "")

			// Ensure Twitter handle starts with @ if provided
			if twitterHandle != "" && !strings.HasPrefix(twitterHandle, "@") {
				twitterHandle = "@" + twitterHandle
			}

			// Get site verification codes per AI.md PART 16
			verifyGoogle := settingsModel.GetString("seo.verification.google", "")
			verifyBing := settingsModel.GetString("seo.verification.bing", "")
			verifyYandex := settingsModel.GetString("seo.verification.yandex", "")
			verifyBaidu := settingsModel.GetString("seo.verification.baidu", "")
			verifyPinterest := settingsModel.GetString("seo.verification.pinterest", "")
			verifyFacebook := settingsModel.GetString("seo.verification.facebook", "")

			// Resolve theme per AI.md PART 16 Theme Detection Flow
			themeMode := resolveThemeFromRequest(r)

			// Create server context
			serverCtx := ServerContext{
				Title:           title,
				Tagline:         tagline,
				Description:     description,
				Version:         version,
				Lang:            lang.(string),
				ThemeClass:      server.ThemeClass(themeMode),
				Theme:           themeMode,
				Keywords:        keywords,
				Author:          author,
				OGImage:         ogImage,
				TwitterHandle:   twitterHandle,
				VerifyGoogle:    verifyGoogle,
				VerifyBing:      verifyBing,
				VerifyYandex:    verifyYandex,
				VerifyBaidu:     verifyBaidu,
				VerifyPinterest: verifyPinterest,
				VerifyFacebook:  verifyFacebook,
			}

			// Add to request context for handlers to use
			ctx := reqctx.Set(r.Context(), "server", serverCtx)

			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// GetServerContext retrieves server context from the request context
func GetServerContext(ctx context.Context) (ServerContext, bool) {
	serverCtx, exists := reqctx.Get(ctx, "server")
	if !exists {
		return ServerContext{
			Title:       "Weather",
			Tagline:     "Your personal weather dashboard",
			Description: "Weather information service",
			Version:     "unknown",
			Lang:        "en",
			ThemeClass:  server.ThemeClass(server.DefaultTheme),
			Theme:       server.DefaultTheme,
			Keywords:    "weather, forecast, alerts",
		}, false
	}

	srvCtx, ok := serverCtx.(ServerContext)
	return srvCtx, ok
}
