package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/mail"
	"strings"
	"time"

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
	data := map[string]interface{}{
		"Title":            ctx.Title,
		"Tagline":          ctx.Tagline,
		"Description":      ctx.Description,
		"Version":          ctx.Version,
		"Lang":             ctx.Lang,
		"ThemeClass":       ctx.ThemeClass,
		"Theme":            ctx.Theme,
		"Keywords":         ctx.Keywords,
		"Author":           ctx.Author,
		"OGImage":          ctx.OGImage,
		"TwitterHandle":    ctx.TwitterHandle,
		"VerifyGoogle":     ctx.VerifyGoogle,
		"VerifyBing":       ctx.VerifyBing,
		"VerifyYandex":     ctx.VerifyYandex,
		"VerifyBaidu":      ctx.VerifyBaidu,
		"VerifyPinterest":  ctx.VerifyPinterest,
		"VerifyFacebook":   ctx.VerifyFacebook,
		"TrackingHTML":     ctx.TrackingHTML,
		"TrackingNoscript": ctx.TrackingNoscript,
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
	running, _, provider, address := GetI2PStatus()
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
				"GitOrg":      "webappsgo",
				"GitRepo":     "wthr",
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
		data := map[string]interface{}{
			"page": "privacy",
			"server": pageServerData(r, map[string]interface{}{
				"Title":     cfg.Server.Branding.Title,
				"BuildDate": BuildDate,
			}),
			"HostInfo": util.GetHostInfo(r),
		}

		// AI.md PART 14: Content negotiation - JSON or HTML
		NegotiateResponse(w, r, "page/privacy.tmpl", data)
	}
}

// ShowContactPage renders the contact form page with content negotiation (AI.md PART 14)
func ShowContactPage(db *database.DB, cfg *config.AppConfig) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		provider := cfg.Server.Pages.Contact.EffectiveCaptcha(util.OverlayHostFromRequest(r) != "")
		captcha := map[string]interface{}{"Provider": provider}
		if provider == config.CaptchaSimple {
			id, a, b := contactCaptcha.New()
			captcha["ID"], captcha["A"], captcha["B"] = id, a, b
		} else {
			captcha["SiteKey"] = cfg.Server.Pages.Contact.CaptchaSiteKey
		}
		data := map[string]interface{}{
			"page": "contact",
			"server": pageServerData(r, map[string]interface{}{
				"Title":   cfg.Server.Branding.Title,
				"GitOrg":  "webappsgo",
				"GitRepo": "wthr",
			}),
			"HostInfo": util.GetHostInfo(r),
			"flash":    TakeFlash(w, r),
			"Captcha":  captcha,
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
				"Title":   cfg.Server.Branding.Title,
				"GitOrg":  "webappsgo",
				"GitRepo": "wthr",
				"I2P":     buildI2PPageData(cfg),
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
			"title":       Translate(r, "about_datasource_weather_title"),
			"description": Translate(r, "about_datasource_weather_desc"),
		},
		{
			"title":       Translate(r, "about_datasource_severe_title"),
			"description": Translate(r, "about_datasource_severe_desc"),
		},
		{
			"title":       Translate(r, "about_datasource_hurricane_title"),
			"description": Translate(r, "about_datasource_hurricane_desc"),
		},
		{
			"title":       Translate(r, "about_datasource_earthquake_title"),
			"description": Translate(r, "about_datasource_earthquake_desc"),
		},
		{
			"title":       Translate(r, "about_datasource_moon_title"),
			"description": Translate(r, "about_datasource_moon_desc"),
		},
		{
			"title":       Translate(r, "about_datasource_geo_title"),
			"description": Translate(r, "about_datasource_geo_desc"),
		},
	}
}

// geoipAttribution returns the PART 20 verbatim GeoIP notices. The link text
// and the NRO license line are returned as separate fields so the HTML page can
// wrap the first in an anchor and the API can carry both as plain values.
func geoipAttribution(r *http.Request) map[string]interface{} {
	return map[string]interface{}{
		"heading": Translate(r, "about_attribution_heading"),
		"link":    "https://db-ip.com/",
		"text":    Translate(r, "about_attribution_geoip_link"),
		"license": Translate(r, "about_attribution_geoip_license"),
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
				Translate(r, "about_api_feature_weather"),
				Translate(r, "about_api_feature_geocoding"),
				Translate(r, "about_api_feature_moon"),
				Translate(r, "about_api_feature_alerts"),
				Translate(r, "about_api_feature_earthquakes"),
				Translate(r, "about_api_feature_hurricanes"),
				Translate(r, "about_api_feature_forecasts"),
				Translate(r, "about_api_feature_api"),
				Translate(r, "about_api_feature_websocket"),
				Translate(r, "about_api_feature_passkey"),
			},
			"data_sources": dataSourceAttribution(r),
			"attribution":  geoipAttribution(r),
			"links": map[string]interface{}{
				"github":  "https://github.com/webappsgo/wthr",
				"docs":    "/openapi",
				"graphql": "/graphql",
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
		RespondNegotiatedData(w, r, http.StatusOK, map[string]interface{}{
			"title":        Translate(r, "privacy_page_heading"),
			"last_updated": BuildDate,
			"data_stored":  true,
			"data_sold":    false,
			"cookies": map[string]interface{}{
				"essential":   true,
				"preferences": true,
				"analytics":   false,
			},
			"data_collection": Translate(r, "privacy_api_data_collection"),
			"data_retention":  Translate(r, "privacy_api_data_retention"),
			"third_parties": []string{
				Translate(r, "privacy_api_third_party_open_meteo"),
				Translate(r, "privacy_api_third_party_usgs"),
				Translate(r, "privacy_api_third_party_noaa"),
				Translate(r, "privacy_api_third_party_alerts"),
				Translate(r, "privacy_api_third_party_nominatim"),
				Translate(r, "privacy_api_third_party_geonames"),
				Translate(r, "privacy_api_third_party_dbip"),
				Translate(r, "privacy_api_third_party_nro"),
			},
			"contact": "/server/contact",
		})
	}
}

// GetHelpAPI returns help content as JSON (AI.md PART 14: /api/v1/server/help)
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
			"title": "Help",
			"getting_started": map[string]interface{}{
				"description": "Get weather data with a single request",
				"examples": []map[string]interface{}{
					{"description": "Current weather for a city", "curl": "curl " + baseURL + "/London"},
					{"description": "JSON API", "curl": "curl " + baseURL + apiBase + "/weather?location=London"},
					{"description": "Forecast", "curl": "curl " + baseURL + apiBase + "/forecasts?location=Paris&days=5"},
				},
			},
			"features": []map[string]interface{}{
				{"name": "Weather Forecasts", "description": "16-day global forecasts with hourly/daily breakdown"},
				{"name": "Severe Weather Alerts", "description": "Real-time alerts from US, Canada, UK, Australia, Japan, Mexico"},
				{"name": "Earthquake Data", "description": "Real-time seismic activity from USGS"},
				{"name": "Hurricane Tracking", "description": "Active tropical storm monitoring from NOAA NHC"},
				{"name": "Moon Phases", "description": "Lunar cycles, illumination, rise/set times"},
				{"name": "GraphQL API", "description": "Full GraphQL endpoint at /graphql including passkey/WebAuthn auth mutations"},
				{"name": "Passkey / WebAuthn", "description": "Admin passkey authentication via beginAdminPasskeyChallenge and finishAdminPasskeyChallenge GraphQL mutations"},
			},
			"api_documentation": map[string]interface{}{
				"swagger":  "/openapi",
				"graphql":  "/graphql",
				"examples": "/examples",
			},
			"faq": []map[string]interface{}{
				{"question": "Do I need an API key?", "answer": "No, the API is free and requires no authentication for basic access."},
				{"question": "What is the rate limit?", "answer": "Anonymous: 20 requests/minute. Authenticated: 100 requests/minute."},
				{"question": "What data sources are used?", "answer": "Open-Meteo for weather, USGS for earthquakes, NOAA for hurricanes and US alerts."},
			},
		}

		if torEnabled && onionAddress != "" {
			help["tor_access"] = map[string]interface{}{
				"enabled":       true,
				"onion_address": onionAddress,
				"instructions":  "Download Tor Browser from https://www.torproject.org/download/ and navigate to the onion address.",
			}
		}

		RespondNegotiatedData(w, r, http.StatusOK, help)
	}
}

// GetTermsAPI returns terms of service as JSON (AI.md PART 14: /api/v1/server/terms)
func GetTermsAPI(db *database.DB, cfg *config.AppConfig) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		RespondNegotiatedData(w, r, http.StatusOK, map[string]interface{}{
			"title":        "Terms of Service",
			"last_updated": BuildDate,
			"sections": []map[string]interface{}{
				{"title": "Acceptance of Terms", "content": "By accessing or using Weather, you agree to be bound by these terms. If you do not agree, do not use the service."},
				{"title": "Description of Service", "content": "Weather provides weather forecasts, severe weather alerts, earthquake data, hurricane tracking, and moon phase information through a web interface and API."},
				{"title": "Acceptable Use", "content": "You may use the service for lawful purposes. You must not attempt to disrupt the service, circumvent rate limits, or use the service to harm others."},
				{"title": "Data Accuracy", "content": "Weather data is sourced from third-party providers (Open-Meteo, USGS, NOAA). We do not guarantee the accuracy, completeness, or timeliness of data. Do not rely solely on this service for safety-critical decisions."},
				{"title": "Limitation of Liability", "content": "Weather is provided as-is without warranty. We are not liable for any damages arising from use of the service or reliance on data provided."},
				{"title": "Changes to Terms", "content": "We may update these terms at any time. Continued use after changes constitutes acceptance of new terms."},
			},
		})
	}
}

// contactCaptcha is the built-in captcha challenge store shared by the contact page and the contact API.
var contactCaptcha = service.NewSimpleCaptcha()

// contactForm is one contact submission after trimming.
type contactForm struct {
	Name    string
	Email   string
	Subject string
	Message string
	// Built-in captcha answer and challenge id
	CaptchaID     string
	CaptchaAnswer string
	// recaptcha / hcaptcha response token
	CaptchaToken string
}

// Result keys of a contact submission; the API and the web form map them to responses.
const (
	contactSent           = "sent"
	contactSaved          = "saved"
	contactFailed         = "failed"
	contactInvalid        = "invalid"
	contactCaptchaFailed  = "captcha_failed"
	contactCaptchaOffline = "captcha_unavailable"
)

// validateContactForm trims every field and reports whether all required fields are present and well formed.
func validateContactForm(f *contactForm) bool {
	f.Name, f.Email = strings.TrimSpace(f.Name), strings.TrimSpace(f.Email)
	f.Subject, f.Message = strings.TrimSpace(f.Subject), strings.TrimSpace(f.Message)
	if f.Name == "" || f.Subject == "" || f.Message == "" || len(f.Name) > 100 || len(f.Subject) > 200 || len(f.Message) > 5000 {
		return false
	}
	if strings.ContainsAny(f.Name+f.Email+f.Subject, "\r\n") {
		return false
	}
	addr, err := mail.ParseAddress(f.Email)
	return err == nil && addr.Address == f.Email
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
	if result := checkContactCaptcha(r, cfg, f); result != "" {
		return result
	}

	smtpService := GetSMTPService(r)
	recipient := ""
	if cfg != nil {
		recipient = cfg.Server.Contact.ResolveContactEmail("general")
	}
	if smtpService == nil || recipient == "" {
		if err := saveContactToDB(r, f.Name, f.Email, f.Subject, f.Message); err != nil {
			return contactFailed
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
	if err := smtpService.SendEmail(recipient, Translate(r, "email.contact_notification.subject_prefix")+f.Subject, emailBody); err != nil {
		if err := saveContactToDB(r, f.Name, f.Email, f.Subject, f.Message); err != nil {
			return contactFailed
		}
		return contactSaved
	}
	return contactSent
}

// HandleContactFormSubmission handles the contact form POST request (API endpoint).
// POST /api/{api_version}/server/contact
func HandleContactFormSubmission(db *database.DB, cfg *config.AppConfig) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Name          string `json:"name"`
			Email         string `json:"email"`
			Subject       string `json:"subject"`
			Message       string `json:"message"`
			CaptchaID     string `json:"captcha_id"`
			CaptchaAnswer string `json:"captcha_answer"`
			CaptchaToken  string `json:"captcha_response"`
		}
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&body); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]interface{}{"ok": false, "error": "BAD_REQUEST", "message": Translate(r, "errors.contact.invalid")})
			return
		}
		result := deliverContact(r, cfg, contactForm{Name: body.Name, Email: body.Email, Subject: body.Subject, Message: body.Message,
			CaptchaID: body.CaptchaID, CaptchaAnswer: body.CaptchaAnswer, CaptchaToken: body.CaptchaToken})
		switch result {
		case contactSent:
			writeJSON(w, http.StatusOK, map[string]interface{}{"ok": true, "data": map[string]interface{}{"message": Translate(r, "success.contact.message_sent")}})
		case contactSaved:
			writeJSON(w, http.StatusOK, map[string]interface{}{"ok": true, "data": map[string]interface{}{"message": Translate(r, "success.contact.message_saved")}})
		case contactInvalid:
			writeJSON(w, http.StatusBadRequest, map[string]interface{}{"ok": false, "error": "VALIDATION_FAILED", "message": Translate(r, "errors.contact.invalid")})
		case contactCaptchaFailed:
			writeJSON(w, http.StatusBadRequest, map[string]interface{}{"ok": false, "error": "CAPTCHA_FAILED", "message": Translate(r, "errors.contact.captcha_failed")})
		case contactCaptchaOffline:
			w.Header().Set("Retry-After", "30")
			writeJSON(w, http.StatusServiceUnavailable, map[string]interface{}{"ok": false, "error": "CAPTCHA_UNAVAILABLE", "message": Translate(r, "errors.contact.captcha_unavailable")})
		default:
			writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"ok": false, "error": "INTERNAL_ERROR", "message": Translate(r, "errors.contact.failed_to_send_message")})
		}
	}
}

// HandleContactFormWeb is the no-JavaScript form handler: POST, then redirect back with a flash message.
// POST /server/contact
func HandleContactFormWeb(db *database.DB, cfg *config.AppConfig) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, 1<<16)
		if err := r.ParseForm(); err != nil {
			SetFlash(w, r, "error", "flash_contact_invalid")
			http.Redirect(w, r, "/server/contact", http.StatusSeeOther)
			return
		}
		result := deliverContact(r, cfg, contactForm{
			Name: r.PostFormValue("name"), Email: r.PostFormValue("email"), Subject: r.PostFormValue("subject"), Message: r.PostFormValue("message"),
			CaptchaID: r.PostFormValue("captcha_id"), CaptchaAnswer: r.PostFormValue("captcha_answer"),
			CaptchaToken: firstNonEmpty(r.PostFormValue("g-recaptcha-response"), r.PostFormValue("h-captcha-response")),
		})
		kind, key := "error", "flash_contact_failed"
		switch result {
		case contactSent:
			kind, key = "success", "flash_contact_sent"
		case contactSaved:
			kind, key = "success", "flash_contact_saved"
		case contactInvalid:
			key = "flash_contact_invalid"
		case contactCaptchaFailed:
			key = "flash_contact_captcha_failed"
		case contactCaptchaOffline:
			key = "flash_contact_captcha_unavailable"
		}
		SetFlash(w, r, kind, key)
		http.Redirect(w, r, "/server/contact", http.StatusSeeOther)
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
	enabled := cfg != nil && cfg.Server.MCP.Enabled
	if db := database.GetServerDB(); db != nil {
		enabled = (&model.SettingsModel{DB: db}).GetBool("server.mcp.enabled", enabled)
	}
	return enabled
}
