package utils

import (
	"github.com/apimgr/weather/src/config"
	"github.com/gin-gonic/gin"
)

// LanguageInfo holds metadata about a supported language for UI display.
// Defined here (in utils) to avoid import cycles with server/service.
type LanguageInfo struct {
	Code       string
	Name       string
	NativeName string
	Direction  string
}

// langInfoProvider is a local interface so template_helpers does not
// import server/service directly (which would create an import cycle).
type langInfoProvider interface {
	GetLanguageInfos() []LanguageInfo
}

// ServerContext holds server information for templates.
type ServerContext struct {
	Title       string
	Tagline     string
	Description string
	Version     string
	// SEO fields per AI.md PART 16
	Keywords      string
	Author        string
	OGImage       string
	TwitterHandle string
	Lang          string
	// Site verification per AI.md PART 16
	VerifyGoogle    string
	VerifyBing      string
	VerifyYandex    string
	VerifyBaidu     string
	VerifyPinterest string
	VerifyFacebook  string
}

// TemplateData enriches template data with server context, user info, and i18n data.
func TemplateData(c *gin.Context, data gin.H) gin.H {
	// Get server context from Gin context (set by middleware)
	serverCtxInterface, exists := c.Get("server")

	var serverCtx interface{}
	if !exists {
		// Fallback to defaults
		serverCtx = map[string]string{
			"Title":       "Weather Service",
			"Tagline":     "Your personal weather dashboard",
			"Description": "Weather information service",
			"Version":     "unknown",
			"Keywords":    "weather, forecast, alerts, earthquakes, hurricanes",
			"Author":      "apimgr",
			"Lang":        "en",
		}
	} else {
		serverCtx = serverCtxInterface
	}

	// Get user context from Gin context (set by auth middleware)
	userCtxInterface, userExists := c.Get("user")
	var userCtx interface{}
	if !userExists {
		// Fallback to empty user (guest)
		userCtx = map[string]string{
			"Email": "",
			"Role":  "guest",
		}
	} else {
		userCtx = userCtxInterface
	}

	// Get CSRF token from context (set by CSRF middleware)
	// Per AI.md line 14803: "All forms include hidden CSRF token field"
	csrfToken, _ := c.Get("csrf_token")
	if csrfToken == nil {
		csrfToken = ""
	}

	// Get current URL for OpenGraph per AI.md PART 16
	scheme := "http"
	if c.Request.TLS != nil {
		scheme = "https"
	}
	currentURL := scheme + "://" + c.Request.Host + c.Request.URL.Path

	// Get configurable paths per AI.md PART 17
	// Templates must use these instead of hardcoded "/admin/" or "/api/v1/"
	cfg, _ := config.LoadConfig()
	adminPath := "/admin"
	apiPath := "/api/v1"
	adminAPIPath := "/api/v1/admin"
	if cfg != nil {
		adminPath = "/" + cfg.GetAdminPath()
		apiPath = cfg.GetAPIPath()
		adminAPIPath = cfg.GetAdminAPIPath()
	}

	// Get active language from i18n middleware (per AI.md PART 31 fallback chain)
	lang := "en"
	if l, ok := c.Get("lang"); ok {
		if s, ok := l.(string); ok && s != "" {
			lang = s
		}
	}

	// Get available languages for language selector UI (per AI.md PART 31)
	var availableLangs []LanguageInfo
	if i18n, ok := c.Get("i18n"); ok {
		if svc, ok := i18n.(langInfoProvider); ok {
			availableLangs = svc.GetLanguageInfos()
		}
	}

	// Create enriched data
	enriched := gin.H{
		"server":              serverCtx,
		"user":                userCtx,
		"csrf_token":          csrfToken,
		"current_url":         currentURL,
		"admin_path":          adminPath,
		"api_path":            apiPath,
		"admin_api_path":      adminAPIPath,
		"lang":                lang,
		"available_languages": availableLangs,
	}

	// Merge user-provided data
	for k, v := range data {
		enriched[k] = v
	}

	return enriched
}
