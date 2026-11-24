package middleware

import (
	"context"
	"database/sql"
	"encoding/json"
	"html/template"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/webappsgo/wthr/src/config"
	"github.com/webappsgo/wthr/src/server"
	"github.com/webappsgo/wthr/src/server/reqctx"
	"github.com/webappsgo/wthr/src/server/service"
	"github.com/webappsgo/wthr/src/util"
)

// ServerContext holds server-wide configuration per AI.md PART 16
type ServerContext struct {
	Title       string
	Tagline     string
	Description string
	Version     string
	GitOrg      string
	GitRepo     string
	// Current user language (AI.md PART 31)
	Lang string
	// Resolved theme class for <html> (AI.md PART 16): theme-dark/theme-light/theme-auto
	ThemeClass string
	// Raw resolved theme mode (AI.md PART 16): dark/light/auto - the value the
	// toggle feeds to nextTheme so its POST target is always server-computed
	Theme string
	// SEO fields per AI.md PART 16
	Keywords       string
	Author         string
	OGImage        string
	TwitterHandle  string
	Logo           string
	Logo200        string
	Logo50         string
	Favicon        string
	Favicon16      string
	Favicon32      string
	Favicon48      string
	Favicon180     string
	Favicon192     string
	Favicon512     string
	OGImage1200    string
	TitleSeparator string
	Canonical      string
	Noindex        bool
	OGEnabled      bool
	OGType         string
	OGSiteName     string
	TwitterEnabled bool
	TwitterCard    string
	TwitterCreator string
	MetaTags       map[string]string
	// Site verification per AI.md PART 16
	VerifyGoogle    string
	VerifyBing      string
	VerifyYandex    string
	VerifyBaidu     string
	VerifyPinterest string
	VerifyFacebook  string
	// Analytics tags for <head> and the <noscript> pixel for <body>; both empty
	// unless tracking is configured, consent is given and the request is clearnet
	TrackingHTML     template.HTML
	TrackingNoscript template.HTML
	HasConsentCookie bool
	ConsentMessage   string
	ConsentPolicyURL string
	ConsentPolicyText string
	ConsentDeclineText string
	ConsentAcceptText string
	ConsentShowPreferences bool
	ConsentPreferencesText string
	ConsentDataSold bool
}

// analyticsConsentGiven reports whether the visitor opted in to analytics. Tracking is
// strictly opt-in: no cookie, a malformed cookie or a GPC signal all mean no.
func analyticsConsentGiven(r *http.Request) bool {
	if GPCOptOut(r) {
		return false
	}
	cookie, err := r.Cookie("cookie_consent")
	if err != nil {
		return false
	}
	value, err := url.QueryUnescape(cookie.Value)
	if err != nil {
		return false
	}
	var state struct {
		Analytics bool `json:"analytics"`
	}
	if json.Unmarshal([]byte(value), &state) != nil {
		return false
	}
	return state.Analytics
}

// TrackingAllowed reports whether analytics may run for this request: consent is given and the
// request is not served through an onion or I2P address (AI.md PART 12 "Tracking and CSP").
func TrackingAllowed(r *http.Request) bool {
	return util.OverlayHostFromRequest(r) == "" && analyticsConsentGiven(r)
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
func InjectServerContext(db *sql.DB, version string, buildDate ...string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Branding and SEO are sourced from the branding area file.
			cfg := config.GetGlobalConfig()
			title := "wthr"
			tagline := ""
			description := ""
			logo, favicon := "", ""
			seo := config.SEOConfig{TitleSeparator: " - ", OpenGraph: config.OpenGraphConfig{Enabled: true, Type: "website"}, Twitter: config.TwitterCardConfig{Enabled: true, Card: "summary_large_image"}}
			if cfg != nil {
				title, tagline, description, seo = cfg.Server.Branding.Title, cfg.Server.Branding.Tagline, cfg.Server.Branding.Description, cfg.Server.SEO
				logo = brandingImageURL(cfg.Server.Branding.Logo)
				favicon = brandingImageURL(cfg.Server.Branding.Favicon)
				if description == "" {
					description = tagline
				}
			}

			// Get user language from i18n middleware
			lang, exists := reqctx.GetValue(r.Context(), "lang")
			if !exists {
				lang = "en"
			}

			// Get SEO settings per AI.md PART 16
			keywords := strings.Join(seo.Keywords, ", ")
			author := seo.Author
			ogImage := brandingImageURL(seo.OGImage)
			logo200 := brandingVariantURL(logo, "-logo-200.png")
			logo50 := brandingVariantURL(logo, "-logo-50.png")
			favicon16 := brandingVariantURL(favicon, "-favicon-16.png")
			favicon32 := brandingVariantURL(favicon, "-favicon-32.png")
			favicon48 := brandingVariantURL(favicon, "-favicon-48.png")
			favicon180 := brandingVariantURL(favicon, "-favicon-180.png")
			favicon192 := brandingVariantURL(favicon, "-favicon-192.png")
			favicon512 := brandingVariantURL(favicon, "-favicon-512.png")
			ogImage1200 := brandingVariantURL(ogImage, "-og-1200x630.jpg")
			if logo200 == "" {
				logo200 = logo
			}
			if logo50 == "" {
				logo50 = logo
			}
			if ogImage1200 == "" {
				ogImage1200 = ogImage
			}
			twitterHandle := seo.TwitterHandle

			// Ensure Twitter handle starts with @ if provided
			if twitterHandle != "" && !strings.HasPrefix(twitterHandle, "@") {
				twitterHandle = "@" + twitterHandle
			}

			// Get site verification codes per AI.md PART 16
			verifyGoogle := seo.Verification.Google
			verifyBing := seo.Verification.Bing
			verifyYandex := seo.Verification.Yandex
			verifyBaidu := seo.Verification.Baidu
			verifyPinterest := seo.Verification.Pinterest
			verifyFacebook := seo.Verification.Facebook

			// Resolve theme per AI.md PART 16 Theme Detection Flow
			themeMode := resolveThemeFromRequest(r)
			privacy := config.DefaultPrivacyConfig()
			if cfg != nil {
				privacy = cfg.Server.Privacy
			}
			consentMessage := privacy.Consent.Message
			if privacy.Data.Sold && privacy.Consent.MessageIfSold != "" {
				consentMessage = privacy.Consent.MessageIfSold
			}
			_, consentCookieErr := r.Cookie("cookie_consent")
			hasConsentCookie := consentCookieErr == nil

			// Create server context
			serverCtx := ServerContext{
				Title:           title,
				Tagline:         tagline,
				Description:     description,
				Version:         version,
				GitOrg:          "webappsgo",
				GitRepo:         "wthr",
				Lang:            lang.(string),
				ThemeClass:      server.ThemeClass(themeMode),
				Theme:           themeMode,
				Keywords:        keywords,
				Author:          author,
				OGImage:         ogImage,
				TwitterHandle:   twitterHandle,
				Logo:            logo,
				Logo200:         logo200,
				Logo50:          logo50,
				Favicon:         favicon,
				Favicon16:       favicon16,
				Favicon32:       favicon32,
				Favicon48:       favicon48,
				Favicon180:      favicon180,
				Favicon192:      favicon192,
				Favicon512:      favicon512,
				OGImage1200:     ogImage1200,
				TitleSeparator:  seo.TitleSeparator,
				Canonical:       seo.Canonical,
				Noindex:         seo.Noindex,
				OGEnabled:       seo.OpenGraph.Enabled,
				OGType:          seo.OpenGraph.Type,
				OGSiteName:      seo.OpenGraph.SiteName,
				TwitterEnabled:  seo.Twitter.Enabled,
				TwitterCard:     seo.Twitter.Card,
				TwitterCreator:  seo.Twitter.Creator,
				MetaTags:        seo.MetaTags,
				VerifyGoogle:    verifyGoogle,
				VerifyBing:      verifyBing,
				VerifyYandex:    verifyYandex,
				VerifyBaidu:     verifyBaidu,
				VerifyPinterest: verifyPinterest,
				VerifyFacebook:  verifyFacebook,
				HasConsentCookie: hasConsentCookie,
				ConsentMessage: consentMessage,
				ConsentPolicyURL: privacy.Consent.Policy.URL,
				ConsentPolicyText: privacy.Consent.Policy.Text,
				ConsentDeclineText: privacy.Consent.Buttons.Decline,
				ConsentAcceptText: privacy.Consent.Buttons.Accept,
				ConsentShowPreferences: privacy.Consent.ShowPreferences,
				ConsentPreferencesText: privacy.Consent.PreferencesText,
				ConsentDataSold: privacy.Data.Sold,
			}

			if cfg != nil && cfg.Server.SEO.Noindex {
				w.Header().Set("X-Robots-Tag", "noindex, nofollow")
			}
			if cfg != nil && TrackingAllowed(r) {
				serverCtx.TrackingHTML = service.TrackingScript(cfg.Server.Tracking)
				serverCtx.TrackingNoscript = service.TrackingNoscript(cfg.Server.Tracking)
			}

			// Add to request context for handlers to use
			ctx := reqctx.SetValue(r.Context(), "server", serverCtx)
			ctx = reqctx.SetValue(ctx, "operator_version", version)
			if len(buildDate) != 0 {
				ctx = reqctx.SetValue(ctx, "operator_build_datetime", buildDate[0])
			}

			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func brandingImageURL(value string) string {
	value = strings.TrimSpace(value)
	if value == "" || strings.HasPrefix(value, "data:image/") {
		return value
	}
	if strings.HasPrefix(strings.ToLower(value), "https://") {
		return resolveRemoteBrandingImage(value)
	}
	if filepath.Base(value) != value {
		return ""
	}
	path := filepath.Join(config.AssetsDir(), value)
	directory, err := os.Stat(filepath.Dir(path))
	if err != nil || !directory.IsDir() || directory.Mode().Perm() != 0o700 {
		return ""
	}
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm() != 0o600 || info.Size() > 2<<20 {
		return ""
	}
	ext := strings.ToLower(filepath.Ext(value))
	if ext != ".svg" && ext != ".png" && ext != ".ico" && ext != ".webp" && ext != ".jpg" {
		return ""
	}
	return "/assets/" + url.PathEscape(value)
}

// GetServerContext retrieves server context from the request context
func GetServerContext(ctx context.Context) (ServerContext, bool) {
	serverCtx, exists := reqctx.GetValue(ctx, "server")
	if !exists {
		return ServerContext{
			Title:       "wthr",
			Tagline:     "",
			Description: "",
			Version:     "unknown",
			GitOrg:      "webappsgo",
			GitRepo:     "wthr",
			Lang:        "en",
			ThemeClass:  server.ThemeClass(server.DefaultTheme),
			Theme:       server.DefaultTheme,
			Keywords:    "",
		}, false
	}

	srvCtx, ok := serverCtx.(ServerContext)
	return srvCtx, ok
}
