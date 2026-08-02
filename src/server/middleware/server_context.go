package middleware

import (
	"database/sql"
	"strings"

	"github.com/webappsgo/wthr/src/server/model"

	"github.com/gin-gonic/gin"
)

// ServerContext holds server-wide configuration per AI.md PART 16
type ServerContext struct {
	Title       string
	Tagline     string
	Description string
	Version     string
	// Current user language (AI.md PART 31)
	Lang string
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

// InjectServerContext adds server configuration to all requests
func InjectServerContext(db *sql.DB, version string) gin.HandlerFunc {
	settingsModel := &model.SettingsModel{DB: db}

	return func(c *gin.Context) {
		// Get server settings
		title := settingsModel.GetString("server.title", "Weather")
		tagline := settingsModel.GetString("server.tagline", "Your personal weather dashboard")
		description := settingsModel.GetString("server.description", "A comprehensive platform for weather forecasts, moon phases, earthquakes, and hurricane tracking.")

		// Get user language from i18n middleware
		lang, exists := c.Get("lang")
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

		// Create server context
		serverCtx := ServerContext{
			Title:           title,
			Tagline:         tagline,
			Description:     description,
			Version:         version,
			Lang:            lang.(string),
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

		// Add to gin context for handlers to use
		c.Set("server", serverCtx)

		c.Next()
	}
}

// GetServerContext retrieves server context from gin context
func GetServerContext(c *gin.Context) (ServerContext, bool) {
	serverCtx, exists := c.Get("server")
	if !exists {
		return ServerContext{
			Title:       "Weather",
			Tagline:     "Your personal weather dashboard",
			Description: "Weather information service",
			Version:     "unknown",
			Lang:        "en",
			Keywords:    "weather, forecast, alerts",
		}, false
	}

	ctx, ok := serverCtx.(ServerContext)
	return ctx, ok
}
