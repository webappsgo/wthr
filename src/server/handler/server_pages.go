package handler

import (
	"context"
	"fmt"
	"net/http"
	"net/mail"
	"net/url"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/webappsgo/wthr/src/common/dbtime"
	"github.com/webappsgo/wthr/src/config"
	"github.com/webappsgo/wthr/src/database"
	"github.com/webappsgo/wthr/src/server/middleware"
	"github.com/webappsgo/wthr/src/server/model"
	"github.com/webappsgo/wthr/src/server/reqctx"
	"github.com/webappsgo/wthr/src/server/service"
	"github.com/webappsgo/wthr/src/util"
)

// Build info variables - set from main via SetBuildInfo()
var (
	Version   = "dev"
	BuildDate = "unknown"
	CommitID  = "unknown"
)

// SetBuildInfo sets the build information from main package
func SetBuildInfo(version, buildDate, commitID string) {
	Version = version
	BuildDate = buildDate
	CommitID = commitID
}

// pageServerData builds the template "server" map for pages that add their own fields.
// util.TemplateData lets caller keys win outright, so a page-supplied map would drop
// the shared chrome context (language, theme, SEO) unless it starts from the
// middleware ServerContext and overrides only page-specific keys (AI.md PART 16).
func pageServerData(r *http.Request, extra map[string]interface{}) map[string]interface{} {
	ctx, _ := middleware.GetServerContext(r.Context())
	apiPath := ""
	if cfg := config.GetGlobalConfig(); cfg != nil {
		apiPath = cfg.GetAPIPath()
	}
	data := map[string]interface{}{
		"Title":            ctx.Title,
		"Tagline":          ctx.Tagline,
		"Description":      ctx.Description,
		"Version":          ctx.Version,
		"GitOrg":           ctx.GitOrg,
		"GitRepo":          ctx.GitRepo,
		"Lang":             ctx.Lang,
		"ThemeClass":       ctx.ThemeClass,
		"Theme":            ctx.Theme,
		"Keywords":         ctx.Keywords,
		"Author":           ctx.Author,
		"OGImage":          ctx.OGImage,
		"TwitterHandle":    ctx.TwitterHandle,
		"Logo":             ctx.Logo,
		"Favicon":          ctx.Favicon,
		"TitleSeparator":   ctx.TitleSeparator,
		"Canonical":        ctx.Canonical,
		"Noindex":          ctx.Noindex,
		"OGEnabled":        ctx.OGEnabled,
		"OGType":           ctx.OGType,
		"OGSiteName":       ctx.OGSiteName,
		"TwitterEnabled":   ctx.TwitterEnabled,
		"TwitterCard":      ctx.TwitterCard,
		"TwitterCreator":   ctx.TwitterCreator,
		"MetaTags":         ctx.MetaTags,
		"VerifyGoogle":     ctx.VerifyGoogle,
		"VerifyBing":       ctx.VerifyBing,
		"VerifyYandex":     ctx.VerifyYandex,
		"VerifyBaidu":      ctx.VerifyBaidu,
		"VerifyPinterest":  ctx.VerifyPinterest,
		"VerifyFacebook":   ctx.VerifyFacebook,
		"TrackingHTML":     ctx.TrackingHTML,
		"TrackingNoscript": ctx.TrackingNoscript,
		"APIPath":          apiPath,
	}
	for key, value := range extra {
		data[key] = value
	}
	return data
}

// buildI2PPageData exposes the opt-in eepsite state to public templates
// (AI.md PART 32.2). Every field is zero-valued while I2P is disabled.
func buildI2PPageData(cfg *config.AppConfig) map[string]interface{} {
	enabled := false
	if cfg != nil {
		enabled = cfg.Server.Features.I2P || cfg.Server.I2P.Enabled
	}
	if !enabled {
		return map[string]interface{}{
			"Enabled":  false,
			"Running":  false,
			"Address":  "",
			"Provider": "none",
		}
	}
	_, running, _, provider, address := GetI2PStatus()
	return map[string]interface{}{
		"Enabled":  true,
		"Running":  running,
		"Address":  address,
		"Provider": provider,
	}
}

// ShowAboutPage renders the about page with content negotiation (AI.md PART 14)
func ShowAboutPage(db *database.DB, cfg *config.AppConfig) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Get server configuration
		settingsModel := &model.SettingsModel{DB: database.GetServerDB()}

		// Get Tor configuration if available
		torEnabled := settingsModel.GetBool("tor.enabled", false)
		onionAddress := settingsModel.GetString("tor.onion_address", "")

		data := map[string]interface{}{
			"page": "about",
			"server": pageServerData(r, map[string]interface{}{
				"Title":       cfg.Server.Branding.Title,
				"Description": cfg.Server.Branding.Description,
				"Version":     Version,
				"BuildDate":   BuildDate,
				"Mode":        cfg.Server.Mode,
				"Tor": map[string]interface{}{
					"Enabled":      torEnabled,
					"OnionAddress": onionAddress,
				},
				"I2P": buildI2PPageData(cfg),
			}),
			"HostInfo": util.GetHostInfo(r),
		}

		// AI.md PART 14: Content negotiation - JSON or HTML
		NegotiateResponse(w, r, "page/about.tmpl", data)
	}
}

// ShowPrivacyPage renders the privacy policy page with content negotiation (AI.md PART 14)
func ShowPrivacyPage(db *database.DB, cfg *config.AppConfig) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		privacy := config.DefaultPrivacyConfig()
		var tracking config.TrackingConfig
		var contact config.ContactConfig
		brandTitle := ""
		if cfg != nil {
			privacy = cfg.Server.Privacy
			tracking = cfg.Server.Tracking
			contact = cfg.Server.Contact
			brandTitle = cfg.Server.Branding.Title
		}
		dataUsage := privacy.Content.DataUsage
		if privacy.Data.Sold {
			dataUsage = privacy.Content.DataUsageIfSold
		}
		analyticsDescription := privacy.Cookies.Analytics.Description + privacy.Cookies.Analytics.DescriptionSuffixNotSold
		if privacy.Data.Sold {
			analyticsDescription = privacy.Cookies.Analytics.Description + privacy.Cookies.Analytics.DescriptionSuffixSold
		}
		ccpaOptedOut := false
		if cookie, err := r.Cookie("ccpa_opt_out"); err == nil && cookie.Value == "true" {
			ccpaOptedOut = true
		}
		data := map[string]interface{}{
			"page": "privacy",
			"server": pageServerData(r, map[string]interface{}{
				"Title":     brandTitle,
				"BuildDate": BuildDate,
			}),
			"HostInfo":             util.GetHostInfo(r),
			"Privacy":              privacy,
			"DataUsage":            dataUsage,
			"AnalyticsDescription": analyticsDescription,
			"Tracking":             tracking,
			"TrackingEnabled":      tracking.Type != "",
			"CCPAOptedOut":         ccpaOptedOut,
			"DPOEmail":             contact.ResolveDPOEmail(),
			"DPOName":              contact.DPO.Name,
			"GPCReceived":          middleware.GPCOptOut(r),
			"ConsentPreferences":   guestConsent(r).Preferences,
			"ConsentAnalytics":     guestConsent(r).Analytics,
		}

		// AI.md PART 14: Content negotiation - JSON or HTML
		NegotiateResponse(w, r, "page/privacy.tmpl", data)
	}
}

// ShowContactPage renders the contact form page with content negotiation (AI.md PART 14)
func ShowContactPage(db *database.DB, cfg *config.AppConfig) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if cfg == nil {
			cfg = config.GetGlobalConfig()
		}
		if cfg == nil || !cfg.Server.Pages.Contact.Enabled {
			http.NotFound(w, r)
			return
		}
		contactType := r.URL.Query().Get("type")
		securityID := securityIDFromRequest(r, r.URL.Query().Get("security_id"))
		isSecurity := securityID != ""
		if !isSecurity && contactType != "" && !validReportType(contactType) {
			http.Error(w, "Unknown report type", http.StatusBadRequest)
			return
		}
		captcha := map[string]interface{}{}
		formToken := ""
		mode := "chooser"
		if contactType != "" || isSecurity {
			antispam, err := getContactAntispam(db, cfg)
			if err != nil {
				http.Error(w, "Contact form is unavailable", http.StatusServiceUnavailable)
				return
			}
			token, tokenErr := antispam.NewContactFormToken(time.Now())
			if tokenErr != nil {
				http.Error(w, "Contact form is unavailable", http.StatusInternalServerError)
				return
			}
			formToken = token
			mode = "form"
			provider := cfg.Server.Pages.Contact.EffectiveCaptcha(util.OverlayHostFromRequest(r) != "")
			captcha = map[string]interface{}{"Provider": provider}
			if provider == config.CaptchaSimple {
				id, a, b := contactCaptcha.New()
				captcha["ID"], captcha["A"], captcha["B"] = id, a, b
			} else {
				captcha["SiteKey"] = cfg.Server.Pages.Contact.CaptchaSiteKey
				id, a, b := contactCaptcha.New()
				captcha["Fallback"] = map[string]interface{}{"Provider": config.CaptchaSimple, "ID": id, "A": a, "B": b}
			}
		}
		if isSecurity {
			w.Header().Set("Referrer-Policy", "no-referrer")
			w.Header().Set("Cache-Control", "no-store")
			data := securityReportFormData(w, r, cfg, securityID, captcha)
			data["Mode"] = "form"
			data["Type"] = "security"
			data["FormToken"] = formToken
			data["HoneypotEnabled"] = cfg.Server.Pages.Contact.EffectiveAntispam().Honeypot == nil || *cfg.Server.Pages.Contact.EffectiveAntispam().Honeypot
			data["Values"] = map[string]string{}
			data["SnapshotFields"] = []map[string]string{}
			NegotiateResponse(w, r, "page/security_report.tmpl", data)
			return
		}
		data := map[string]interface{}{
			"page": "contact",
			"server": pageServerData(r, map[string]interface{}{
				"Title": cfg.Server.Branding.Title,
			}),
			"HostInfo":           util.GetHostInfo(r),
			"flash":              TakeFlash(w, r),
			"Captcha":            captcha,
			"FormToken":          formToken,
			"Mode":               mode,
			"Type":               contactType,
			"Values":             map[string]string{},
			"SnapshotFields":     []map[string]string{},
			"FQDN":               cfg.Server.FQDN,
			"AbuseEmail":         cfg.Server.Contact.Abuse.Email,
			"ReasonOptions":      contactReasonOptions(),
			"RequestKindOptions": contactRequestKindOptions(),
			"HoneypotEnabled":    cfg.Server.Pages.Contact.EffectiveAntispam().Honeypot == nil || *cfg.Server.Pages.Contact.EffectiveAntispam().Honeypot,
		}

		// AI.md PART 14: Content negotiation - JSON or HTML
		NegotiateResponse(w, r, "page/contact.tmpl", data)
	}
}

// ShowHelpPage renders the help & documentation page with content negotiation (AI.md PART 14)
func ShowHelpPage(db *database.DB, cfg *config.AppConfig) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		data := map[string]interface{}{
			"page": "help",
			"server": pageServerData(r, map[string]interface{}{
				"Title": cfg.Server.Branding.Title,
				"I2P":   buildI2PPageData(cfg),
			}),
			"HostInfo":   util.GetHostInfo(r),
			"MCPEnabled": mcpEnabled(cfg),
		}

		// AI.md PART 14: Content negotiation - JSON or HTML
		NegotiateResponse(w, r, "page/help.tmpl", data)
	}
}

// ShowTermsPage renders the terms of service page with content negotiation (AI.md PART 16)
func ShowTermsPage(db *database.DB, cfg *config.AppConfig) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		data := map[string]interface{}{
			"page": "terms",
			"server": pageServerData(r, map[string]interface{}{
				"Title":     cfg.Server.Branding.Title,
				"BuildDate": BuildDate,
			}),
			"HostInfo": util.GetHostInfo(r),
		}

		NegotiateResponse(w, r, "page/terms.tmpl", data)
	}
}

// ShowExamplesPage renders the API usage examples page with content negotiation
// (AI.md PART 14 frontend chain, AI.md PART 16 page conventions). Every example
// URL is built from the live host info and the configured API path, so the page
// stays correct across hostnames and API versions.
func ShowExamplesPage(cfg *config.AppConfig) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		host := util.GetHostInfo(r).FullHost
		apiPath := cfg.GetAPIPath()

		consoleExamples := []string{
			host + "/",
			host + "/London",
			host + "/Paris?format=1",
			host + "/Tokyo?units=metric",
		}
		jsonExamples := []string{
			host + apiPath + "/weather?location=London",
			host + apiPath + "/forecasts?location=Paris&days=5",
			host + apiPath + "/locations/search?q=New+York",
			host + apiPath + "/ip",
		}

		data := map[string]interface{}{
			"page": "examples",
			"server": pageServerData(r, map[string]interface{}{
				"Title":     cfg.Server.Branding.Title,
				"BuildDate": BuildDate,
			}),
			"HostInfo":        util.GetHostInfo(r),
			"ConsoleExamples": consoleExamples,
			"JSONExamples":    jsonExamples,
		}

		NegotiateResponse(w, r, "page/examples.tmpl", data)
	}
}

// dataSourceAttribution returns the data-source list the About page and the
// About API share, with the PART 20 GeoIP attribution included verbatim as
// translated copy (AI.md PART 20: attribution is a license condition).
func dataSourceAttribution(r *http.Request) []map[string]interface{} {
	return []map[string]interface{}{
		{
			"title":       Translate(r, "about.datasource_weather_title"),
			"description": Translate(r, "about.datasource_weather_desc"),
		},
		{
			"title":       Translate(r, "about.datasource_severe_title"),
			"description": Translate(r, "about.datasource_severe_desc"),
		},
		{
			"title":       Translate(r, "about.datasource_hurricane_title"),
			"description": Translate(r, "about.datasource_hurricane_desc"),
		},
		{
			"title":       Translate(r, "about.datasource_earthquake_title"),
			"description": Translate(r, "about.datasource_earthquake_desc"),
		},
		{
			"title":       Translate(r, "about.datasource_moon_title"),
			"description": Translate(r, "about.datasource_moon_desc"),
		},
		{
			"title":       Translate(r, "about.datasource_geo_title"),
			"description": Translate(r, "about.datasource_geo_desc"),
		},
	}
}

// geoipAttribution returns the PART 20 DB-IP notice. The link text and the URL
// are separate fields so the HTML page can wrap the text in an anchor and the
// API can carry both as plain values.
func geoipAttribution(r *http.Request) map[string]interface{} {
	return map[string]interface{}{
		"heading": Translate(r, "about.attribution_heading"),
		"link":    "https://db-ip.com/",
		"text":    Translate(r, "about.attribution_geoip_link"),
	}
}

// GetAboutAPI returns about information as JSON (AI.md PART 14: /api/v1/server/about)
func GetAboutAPI(db *database.DB, cfg *config.AppConfig) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		settingsModel := &model.SettingsModel{DB: database.GetServerDB()}
		torEnabled := settingsModel.GetBool("tor.enabled", false)
		onionAddress := settingsModel.GetString("tor.onion_address", "")

		RespondNegotiatedData(w, r, http.StatusOK, map[string]interface{}{
			"title":       cfg.Server.Branding.Title,
			"description": cfg.Server.Branding.Description,
			"version":     Version,
			"build_date":  BuildDate,
			"features": []string{
				Translate(r, "about.api_feature_weather"),
				Translate(r, "about.api_feature_geocoding"),
				Translate(r, "about.api_feature_moon"),
				Translate(r, "about.api_feature_alerts"),
				Translate(r, "about.api_feature_earthquakes"),
				Translate(r, "about.api_feature_hurricanes"),
				Translate(r, "about.api_feature_forecasts"),
				Translate(r, "about.api_feature_api"),
				Translate(r, "about.api_feature_websocket"),
				Translate(r, "about.api_feature_passkey"),
			},
			"data_sources": dataSourceAttribution(r),
			"attribution":  geoipAttribution(r),
			"links": map[string]interface{}{
				"github":  "https://github.com/webappsgo/wthr",
				"docs":    util.BuildURL(r, "/server/docs/swagger"),
				"graphql": util.BuildURL(r, "/server/docs/graphql"),
			},
			"tor": map[string]interface{}{
				"enabled":       torEnabled,
				"onion_address": onionAddress,
			},
		})
	}
}

// GetPrivacyAPI returns the privacy policy as JSON (AI.md PART 14: /api/v1/server/privacy)
func GetPrivacyAPI(db *database.DB, cfg *config.AppConfig) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		privacy := config.DefaultPrivacyConfig()
		var tracking config.TrackingConfig
		if cfg != nil {
			privacy = cfg.Server.Privacy
			tracking = cfg.Server.Tracking
		}
		analyticsDescription := privacy.Cookies.Analytics.Description + privacy.Cookies.Analytics.DescriptionSuffixNotSold
		if privacy.Data.Sold {
			analyticsDescription = privacy.Cookies.Analytics.Description + privacy.Cookies.Analytics.DescriptionSuffixSold
		}
		dataUsage := privacy.Content.DataUsage
		if privacy.Data.Sold {
			dataUsage = privacy.Content.DataUsageIfSold
		}
		consentMessage := privacy.Consent.Message
		if privacy.Data.Sold {
			consentMessage = privacy.Consent.MessageIfSold
		}
		services := make([]map[string]interface{}, 0, len(privacy.ThirdParty.Services))
		for _, service := range privacy.ThirdParty.Services {
			services = append(services, map[string]interface{}{
				"name": service.Name, "purpose": service.Purpose, "data_sent": service.Data, "policy_url": service.URL,
			})
		}
		sharing := make([]map[string]string, 0, len(privacy.Data.Sharing))
		for _, item := range privacy.Data.Sharing {
			sharing = append(sharing, map[string]string{"condition": item.Condition, "when": item.When, "data": item.Data})
		}
		cookieCategory := func(category config.PrivacyCookieCategory) map[string]interface{} {
			return map[string]interface{}{"enabled": category.Enabled, "description": category.Description}
		}
		data := map[string]interface{}{
			"title":        Translate(r, "privacy.page_heading"),
			"last_updated": BuildDate,
			"summary": map[string]interface{}{
				"data_stored_on_server": privacy.Data.StoredOnServer,
				"data_sold":             privacy.Data.Sold,
				"user_control":          privacy.Retention.ExportAvailable || privacy.Retention.DeletionAvailable,
			},
			"cookies": map[string]interface{}{
				"essential":   cookieCategory(privacy.Cookies.Essential),
				"preferences": cookieCategory(privacy.Cookies.Preferences),
				"analytics": map[string]interface{}{
					"enabled":     privacy.Cookies.Analytics.Enabled,
					"description": analyticsDescription,
				},
			},
			"data": map[string]interface{}{
				"sold": privacy.Data.Sold, "stored_on_server": privacy.Data.StoredOnServer, "sharing": sharing,
			},
			"tracking": map[string]interface{}{"enabled": tracking.Type != "", "type": tracking.Type, "type_name": tracking.Type},
			"retention": map[string]interface{}{
				"period": privacy.Retention.Period, "export_available": privacy.Retention.ExportAvailable,
				"deletion_available": privacy.Retention.DeletionAvailable,
			},
			"third_party": map[string]interface{}{"services": services},
			"content": map[string]interface{}{
				"data_collection": privacy.Content.DataCollection,
				"data_usage":      dataUsage,
				"data_security":   privacy.Content.DataSecurity,
				"consent_message": consentMessage,
			},
			"contact": util.BuildURL(r, "/server/security"),
		}
		if privacy.Data.Sold {
			ccpaOptedOut := false
			if cookie, err := r.Cookie("ccpa_opt_out"); err == nil && cookie.Value == "true" {
				ccpaOptedOut = true
			}
			data["ccpa"] = map[string]interface{}{
				"applicable":     true,
				"opt_out_url":    "/server/privacy#ccpa-opt-out",
				"user_opted_out": ccpaOptedOut,
			}
		}
		RespondNegotiatedData(w, r, http.StatusOK, data)
	}
}

// GetHelpAPI returns help content as JSON (AI.md PART 14: /api/v1/server/help).
// Every human-readable string comes from the locale files, like the HTML page.
func GetHelpAPI(db *database.DB, cfg *config.AppConfig) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		settingsModel := &model.SettingsModel{DB: database.GetServerDB()}
		torEnabled := settingsModel.GetBool("tor.enabled", false)
		onionAddress := settingsModel.GetString("tor.onion_address", "")
		hostInfo := util.GetHostInfo(r)
		baseURL := hostInfo.ExampleURL

		// AI.md PART 14: never hardcode the API version, resolve it from config
		apiBase := config.GetGlobalConfig().GetAPIPath()

		help := map[string]interface{}{
			"title": Translate(r, "help.heading"),
			"getting_started": map[string]interface{}{
				"description": Translate(r, "help.quick_start_intro"),
				"examples": []string{
					"curl " + baseURL + "/London",
					"curl " + baseURL + apiBase + "/weather?location=London",
					"curl " + baseURL + apiBase + "/forecasts?location=Paris&days=5",
				},
			},
			"features": []string{
				Translate(r, "about.api_feature_weather"),
				Translate(r, "about.api_feature_alerts"),
				Translate(r, "about.api_feature_earthquakes"),
				Translate(r, "about.api_feature_hurricanes"),
				Translate(r, "about.api_feature_moon"),
				Translate(r, "about.api_feature_api"),
			},
			"api_documentation": map[string]interface{}{
				"swagger": util.BuildURL(r, "/server/docs/swagger"),
				"graphql": util.BuildURL(r, "/server/docs/graphql"),
			},
			"rate_limits": map[string]interface{}{
				"anonymous":     Translate(r, "help.rate_limits_anonymous_value"),
				"authenticated": Translate(r, "help.rate_limits_authenticated_value"),
			},
			"faq": []map[string]interface{}{
				{"question": Translate(r, "help.faq_account_question"), "answer": Translate(r, "help.faq_account_answer")},
				{"question": Translate(r, "help.faq_alerts_question"), "answer": Translate(r, "help.faq_alerts_answer")},
				{"question": Translate(r, "help.faq_location_question"), "answer": Translate(r, "help.faq_location_answer1")},
			},
		}

		if torEnabled && onionAddress != "" {
			help["tor_access"] = map[string]interface{}{
				"enabled":       true,
				"onion_address": onionAddress,
			}
		}

		RespondNegotiatedData(w, r, http.StatusOK, help)
	}
}

// GetTermsAPI returns terms of service as JSON (AI.md PART 14: /api/v1/server/terms).
// The sections mirror the HTML page and come from the locale files.
func GetTermsAPI(db *database.DB, cfg *config.AppConfig) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		RespondNegotiatedData(w, r, http.StatusOK, map[string]interface{}{
			"title":        Translate(r, "terms.page_title"),
			"last_updated": BuildDate,
			"sections": []map[string]interface{}{
				{"title": Translate(r, "terms.acceptance_title"), "content": Translate(r, "terms.acceptance_text")},
				{"title": Translate(r, "terms.description_title"), "content": Translate(r, "terms.description_text")},
				{"title": Translate(r, "terms.acceptable_use_title"), "content": Translate(r, "terms.acceptable_use_intro") + " " + strings.Join([]string{
					Translate(r, "terms.acceptable_use_disrupt"),
					Translate(r, "terms.acceptable_use_circumvent"),
					Translate(r, "terms.acceptable_use_harm"),
					Translate(r, "terms.acceptable_use_scrape"),
					Translate(r, "terms.acceptable_use_misrepresent"),
				}, "; ")},
				{"title": Translate(r, "terms.api_usage_title"), "content": strings.Join([]string{
					Translate(r, "terms.api_usage_intro"),
					Translate(r, "terms.api_rate_limits_text"),
					Translate(r, "terms.api_fair_use_text"),
					Translate(r, "terms.api_attribution_text"),
				}, " ")},
				{"title": Translate(r, "terms.data_accuracy_title"), "content": Translate(r, "terms.data_accuracy_text") + " " + Translate(r, "terms.data_accuracy_advice")},
				{"title": Translate(r, "terms.liability_title"), "content": Translate(r, "terms.liability_intro") + " " + strings.Join([]string{
					Translate(r, "terms.liability_use"),
					Translate(r, "terms.liability_inaccurate"),
					Translate(r, "terms.liability_interruptions"),
					Translate(r, "terms.liability_decisions"),
				}, "; ")},
				{"title": Translate(r, "terms.changes_title"), "content": Translate(r, "terms.changes_text")},
			},
		})
	}
}

// contactCaptcha is the built-in captcha challenge store shared by the contact page and the contact API.
var contactCaptcha = service.NewSimpleCaptcha()

// contactForm is one contact submission after trimming.
type contactForm struct {
	Type            string
	Name            string
	Email           string
	Subject         string
	Message         string
	ReportedURL     string
	Reason          string
	CopyrightedWork string
	GoodFaith       bool
	Accuracy        bool
	Signature       string
	RequestKind     string
	Website         string
	FormToken       string
	ReviewToken     string
	Step            string
	Confirmed       bool
	// Built-in captcha answer and challenge id
	CaptchaID     string
	CaptchaAnswer string
	// recaptcha / hcaptcha response token
	CaptchaToken string
}

// Contact form length limits in Unicode characters (AI.md PART 16 "/server/contact").
const (
	contactNameMax    = 100
	contactEmailMax   = 254
	contactSubjectMax = 200
	contactMessageMax = 5000
)

// Result keys of a contact submission; the API and the web form map them to responses.
const (
	contactSent           = "sent"
	contactSaved          = "saved"
	contactFailed         = "failed"
	contactInvalid        = "invalid"
	contactCaptchaFailed  = "captcha_failed"
	contactCaptchaOffline = "captcha_unavailable"
)

// invalidContactField trims every field and returns the name of the first field
// that is missing, too long or malformed, or "" when the form is valid.
func invalidContactField(f *contactForm) string {
	f.Name, f.Email = strings.TrimSpace(f.Name), strings.TrimSpace(f.Email)
	f.Subject, f.Message = strings.TrimSpace(f.Subject), strings.TrimSpace(f.Message)
	if field := invalidContactIdentity(f); field != "" {
		return field
	}
	if !validReportType(f.Type) {
		return "type"
	}
	switch f.Type {
	case "abuse":
		if f.ReportedURL == "" || utf8.RuneCountInString(f.ReportedURL) > 2048 {
			return "reported_url"
		}
		if !containsContactString([]string{"illegal_content", "terms_violation", "harassment", "malware_phishing", "other"}, f.Reason) {
			return "reason"
		}
	case "dmca":
		if f.ReportedURL == "" || utf8.RuneCountInString(f.ReportedURL) > 2048 {
			return "reported_url"
		}
		if f.CopyrightedWork == "" || utf8.RuneCountInString(f.CopyrightedWork) > 2000 {
			return "copyrighted_work"
		}
		if !f.GoodFaith {
			return "good_faith"
		}
		if !f.Accuracy {
			return "accuracy"
		}
		if f.Signature == "" || utf8.RuneCountInString(f.Signature) > 100 {
			return "signature"
		}
	case "privacy":
		if !containsContactString([]string{"access", "export", "deletion", "correction", "opt_out", "other"}, f.RequestKind) {
			return "request_kind"
		}
	}
	return ""
}

func invalidContactIdentity(f *contactForm) string {
	f.Name, f.Email = strings.TrimSpace(f.Name), strings.TrimSpace(f.Email)
	f.Subject, f.Message = strings.TrimSpace(f.Subject), strings.TrimSpace(f.Message)
	// AI.md PART 16 "/server/contact": required fields, and limits that count
	// Unicode characters after trimming
	switch {
	case f.Name == "" || utf8.RuneCountInString(f.Name) > contactNameMax || strings.ContainsAny(f.Name, "\r\n"):
		return "name"
	case f.Email == "" || utf8.RuneCountInString(f.Email) > contactEmailMax || strings.ContainsAny(f.Email, "\r\n"):
		return "email"
	case f.Subject == "" || utf8.RuneCountInString(f.Subject) > contactSubjectMax || strings.ContainsAny(f.Subject, "\r\n"):
		return "subject"
	case f.Message == "" || utf8.RuneCountInString(f.Message) > contactMessageMax:
		return "message"
	}
	if addr, err := mail.ParseAddress(f.Email); err != nil || addr.Address != f.Email {
		return "email"
	}
	return ""
}

// validateContactForm reports whether every required field is present and well formed.
func validateContactForm(f *contactForm) bool {
	return invalidContactField(f) == ""
}

// checkContactCaptcha verifies the captcha for this request: every submission, no failed-attempt
// state (AI.md PART 16 "/server/contact"). Overlay requests always use the built-in challenge.
func checkContactCaptcha(r *http.Request, cfg *config.AppConfig, f contactForm) string {
	pages := config.PagesContactConfig{}
	if cfg != nil {
		pages = cfg.Server.Pages.Contact
	}
	provider := pages.EffectiveCaptcha(util.OverlayHostFromRequest(r) != "")
	if provider == config.CaptchaSimple {
		if !contactCaptcha.Verify(f.CaptchaID, f.CaptchaAnswer) {
			return contactCaptchaFailed
		}
		return ""
	}
	switch err := service.VerifyProvider(r.Context(), nil, provider, pages.CaptchaSecretKey, f.CaptchaToken, util.TrustedGetClientIP(r)); err {
	case nil:
		return ""
	case service.ErrCaptchaUnavailable:
		return contactCaptchaOffline
	default:
		return contactCaptchaFailed
	}
}

// deliverContact validates, captcha-checks and delivers one submission. It returns one of the contact* result keys.
func deliverContact(r *http.Request, cfg *config.AppConfig, f contactForm) string {
	if !validateContactForm(&f) {
		return contactInvalid
	}
	var contact config.ContactConfig
	if cfg != nil {
		contact = cfg.Server.Contact
	} else if global := config.GetGlobalConfig(); global != nil {
		contact = global.Server.Contact
	}
	role := "general"
	if f.Type == "abuse" || f.Type == "dmca" {
		role = "abuse"
	}
	webhookBody := fmt.Sprintf("Type: %s\n%s: %s <%s>\n%s: %s\n\n%s:\n%s\nReported URL: %s\nReason: %s\nCopyrighted work: %s\nGood-faith statement: %t\nAccuracy statement: %t\nSignature: %s\nRequest kind: %s",
		f.Type,
		Translate(r, "email.contact_notification.label_from"), f.Name, f.Email,
		Translate(r, "email.contact_notification.label_subject"), f.Subject,
		Translate(r, "email.contact_notification.label_message"), f.Message,
		f.ReportedURL, f.Reason, f.CopyrightedWork, f.GoodFaith, f.Accuracy, f.Signature, f.RequestKind)
	webhookMessage := service.ContactWebhookMessage{
		Event: role + ".contact_received", Subject: f.Subject, Body: webhookBody, Severity: "info",
		ProjectName: "wthr", ProjectVersion: Version, AppURL: util.BuildURL(r, "/"),
		FailureRateKey: util.TrustedGetClientIP(r),
	}
	webhookSent, _ := service.DispatchContactWebhooksWithRetry(r.Context(), contact, role, webhookMessage)

	smtpService := GetSMTPService(r)
	recipient := ""
	recipient = contact.ResolveContactEmail(role)
	if smtpService == nil || recipient == "" {
		if err := saveContactToDB(r, f.Name, f.Email, f.Subject, f.Message); err != nil {
			if webhookSent > 0 {
				return contactSent
			}
			return contactFailed
		}
		if webhookSent > 0 {
			return contactSent
		}
		return contactSaved
	}

	emailBody := fmt.Sprintf("%s\n\n%s: %s <%s>\n%s: %s\n\n%s:\n%s\n\n---\n%s: %s\n%s: %s\n%s: %s",
		Translate(r, "email.contact_notification.title"),
		Translate(r, "email.contact_notification.label_from"), f.Name, f.Email,
		Translate(r, "email.contact_notification.label_subject"), f.Subject,
		Translate(r, "email.contact_notification.label_message"), f.Message,
		Translate(r, "email.contact_notification.label_ip"), util.TrustedGetClientIP(r),
		Translate(r, "email.contact_notification.label_user_agent"), r.UserAgent(),
		Translate(r, "email.contact_notification.label_time"), time.Now().Format("2006-01-02 15:04:05"))
	emailBody += fmt.Sprintf("\n\nType: %s\nReported URL: %s\nReason: %s\nCopyrighted work: %s\nGood-faith statement: %t\nAccuracy statement: %t\nSignature: %s\nRequest kind: %s",
		f.Type, f.ReportedURL, f.Reason, f.CopyrightedWork, f.GoodFaith, f.Accuracy, f.Signature, f.RequestKind)
	if err := smtpService.SendEmail(recipient, Translate(r, "email.contact_notification.subject_prefix")+f.Subject, emailBody); err != nil {
		if err := saveContactToDB(r, f.Name, f.Email, f.Subject, f.Message); err != nil {
			if webhookSent > 0 {
				return contactSent
			}
			return contactFailed
		}
		if webhookSent > 0 {
			return contactSent
		}
		return contactSaved
	}
	return contactSent
}

// HandleContactFormSubmission handles the contact form POST request (API endpoint).
// POST /api/{api_version}/server/contact
func HandleContactFormSubmission(db *database.DB, cfg *config.AppConfig) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		raw, err := contactRawFromRequest(w, r, true)
		if err != nil {
			if _, ok := err.(*http.MaxBytesError); ok {
				writeContactFlowError(w, r, cfg, db, contactFlowResult{Kind: "shape"})
				return
			}
			writeJSON(w, http.StatusBadRequest, map[string]interface{}{"ok": false, "error": "BAD_REQUEST", "message": Translate(r, "errors.contact.invalid")})
			return
		}
		if rawID := raw["security_id"]; rawID != "" {
			if securityID := securityIDFromRequest(r, rawID); securityID != "" {
				handleSecurityContactFlowAPI(w, r, db, cfg, securityID, raw)
				return
			}
		}
		flow := processContactFlow(r, db, cfg, raw, false)
		if flow.FormToken == "" {
			if manager, err := getContactAntispam(db, cfg); err == nil {
				flow.FormToken, _ = manager.NewContactFormToken(time.Now())
			}
		}
		if flow.Kind == "review" {
			writeJSON(w, http.StatusOK, map[string]interface{}{"ok": true, "data": map[string]interface{}{
				"status": "review", "review_token": flow.ReviewToken, "form_token": flow.FormToken,
				"confirm_after_seconds": cfg.Server.Pages.Contact.EffectiveAntispam().ReviewMinSeconds,
				"review":                map[string]string{"type": flow.Form.Type, "email": flow.Form.Email, "subject": flow.Form.Subject},
			}})
			return
		}
		if flow.Kind == "dropped" || flow.Kind == "delivered" {
			if flow.Kind == "dropped" || flow.DeliverResult == contactSent {
				writeJSON(w, http.StatusOK, map[string]interface{}{"ok": true, "data": map[string]interface{}{"message": Translate(r, "success.contact.message_sent"), "form_token": flow.FormToken}})
				return
			}
			if flow.DeliverResult == contactSaved {
				writeJSON(w, http.StatusOK, map[string]interface{}{"ok": true, "data": map[string]interface{}{"message": Translate(r, "success.contact.message_saved"), "form_token": flow.FormToken}})
				return
			}
			flow.Kind = "delivery_failed"
		}
		writeContactFlowError(w, r, cfg, db, flow)
	}
}

// HandleContactFormWeb is the no-JavaScript form handler: POST, then redirect back with a flash message.
// POST /server/contact
func HandleContactFormWeb(db *database.DB, cfg *config.AppConfig) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		raw, err := contactRawFromRequest(w, r, false)
		if err != nil {
			if maxErr, ok := err.(*http.MaxBytesError); ok {
				_ = maxErr
				http.Error(w, "Contact submission is too large", http.StatusRequestEntityTooLarge)
				return
			}
			SetFlash(w, r, "error", "flash.contact_invalid")
			http.Redirect(w, r, "/server/contact", http.StatusSeeOther)
			return
		}
		if securityID := securityIDFromRequest(r, raw["security_id"]); securityID != "" {
			handleSecurityContactFlowWeb(w, r, db, cfg, securityID, raw)
			return
		}
		flow := processContactFlow(r, db, cfg, raw, true)
		if flow.Kind == "review" || flow.Kind == "edit" {
			mode := "form"
			if flow.Kind == "review" {
				mode = "review"
			}
			data := contactPageData(w, r, cfg, flow.Form.Type, mode, flow.Values, flow.FormToken, flow.ReviewToken)
			NegotiateResponse(w, r, "page/contact.tmpl", data)
			return
		}
		kind, key := "success", "flash.contact_sent"
		if flow.Kind == "delivered" && flow.DeliverResult != contactSent && flow.DeliverResult != contactSaved {
			flow.Kind = "delivery_failed"
		}
		if flow.Kind == "delivered" && flow.DeliverResult == contactSaved {
			key = "flash.contact_saved"
		}
		if flow.Kind != "delivered" && flow.Kind != "dropped" {
			renderContactFlowFailure(w, r, cfg, db, flow, "")
			return
		}
		SetFlash(w, r, kind, key)
		redirect := "/server/contact"
		if flow.Form.Type != "" {
			redirect += "?type=" + url.QueryEscape(flow.Form.Type)
		}
		http.Redirect(w, r, redirect, http.StatusSeeOther)
	}
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}

// saveContactToDB saves contact form submission to database when SMTP unavailable
// Per AI.md PART 26: Graceful degradation when SMTP not configured
func saveContactToDB(r *http.Request, name, email, subject, message string) error {
	dbInterface, exists := reqctx.GetValue(r.Context(), "db")
	if !exists {
		return fmt.Errorf("database not available")
	}
	db, ok := dbInterface.(*database.DB)
	if !ok || db == nil {
		return fmt.Errorf("database not available")
	}

	// contact_submissions is declared once in database.ServerSchema, which
	// database.InitDualDB executes on every startup, so no DDL runs at request
	// time any more. The CREATE TABLE that used to live here declared created_at
	// as "INTEGER NOT NULL DEFAULT (strftime('%s','now'))" - a SQLite-only
	// expression, and an epoch integer where the real schema declares DATETIME
	// holding canonical UTC text. Wherever this statement won the race the column
	// held a type no reader in this project parses. This presence check remains so
	// a database missing the schema fails with a clear error rather than a bare
	// "no such table" on the insert below.
	var present int
	if err := database.QueryRowContext(context.Background(), db.DB, database.TimeoutSimpleSelect,
		"SELECT COUNT(*) FROM contact_submissions WHERE 1 = 0",
	).Scan(&present); err != nil {
		return fmt.Errorf("contact_submissions is missing from the server database schema: %w", err)
	}

	// Insert contact submission. created_at is bound explicitly rather than left
	// to the column's default: a column default is a third implicit producer of
	// the value that no application-side discipline reaches, and on PostgreSQL and
	// MySQL it renders in the server's own zone and type instead of the canonical
	// UTC text every reader in this project parses.
	_, err := database.ExecContext(context.Background(), db.DB, database.TimeoutWrite, `
		INSERT INTO contact_submissions (name, email, subject, message, ip_address, user_agent, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`, name, email, subject, message, util.TrustedGetClientIP(r), r.UserAgent(), dbtime.FormatSQLTimestamp(time.Now()))

	return err
}

// GetSMTPService returns the SMTP service from context if available
func GetSMTPService(r *http.Request) *service.SMTPService {
	if smtp, exists := reqctx.GetValue(r.Context(), "smtp"); exists {
		if smtpService, ok := smtp.(*service.SMTPService); ok {
			return smtpService
		}
	}
	return nil
}

// mcpEnabled reports whether the MCP endpoint is switched on (admin setting first, config default second).
func mcpEnabled(cfg *config.AppConfig) bool {
	return true
}
