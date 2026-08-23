package handler

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/webappsgo/wthr/src/common/dbtime"
	"github.com/webappsgo/wthr/src/config"
	"github.com/webappsgo/wthr/src/database"
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

// ShowAboutPage renders the about page with content negotiation (AI.md PART 14)
func ShowAboutPage(db *database.DB, cfg *config.AppConfig) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user, _ := reqctx.Get(r.Context(), "user")

		// Get server configuration
		settingsModel := &model.SettingsModel{DB: database.GetServerDB()}

		// Get Tor configuration if available
		torEnabled := settingsModel.GetBool("tor.enabled", false)
		onionAddress := settingsModel.GetString("tor.onion_address", "")

		data := map[string]interface{}{
			"user": user,
			"page": "about",
			"server": map[string]interface{}{
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
			},
			"HostInfo": util.GetHostInfo(r),
		}

		// AI.md PART 14: Content negotiation - JSON or HTML
		NegotiateResponse(w, r, "page/about.tmpl", data)
	}
}

// ShowPrivacyPage renders the privacy policy page with content negotiation (AI.md PART 14)
func ShowPrivacyPage(db *database.DB, cfg *config.AppConfig) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user, _ := reqctx.Get(r.Context(), "user")

		data := map[string]interface{}{
			"user": user,
			"page": "privacy",
			"server": map[string]interface{}{
				"Title":     cfg.Server.Branding.Title,
				"BuildDate": BuildDate,
			},
			"HostInfo": util.GetHostInfo(r),
		}

		// AI.md PART 14: Content negotiation - JSON or HTML
		NegotiateResponse(w, r, "page/privacy.tmpl", data)
	}
}

// ShowContactPage renders the contact form page with content negotiation (AI.md PART 14)
func ShowContactPage(db *database.DB, cfg *config.AppConfig) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user, _ := reqctx.Get(r.Context(), "user")

		data := map[string]interface{}{
			"user": user,
			"page": "contact",
			"server": map[string]interface{}{
				"Title":   cfg.Server.Branding.Title,
				"GitOrg":  "webappsgo",
				"GitRepo": "wthr",
			},
			"HostInfo": util.GetHostInfo(r),
		}

		// AI.md PART 14: Content negotiation - JSON or HTML
		NegotiateResponse(w, r, "page/contact.tmpl", data)
	}
}

// ShowHelpPage renders the help & documentation page with content negotiation (AI.md PART 14)
func ShowHelpPage(db *database.DB, cfg *config.AppConfig) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user, _ := reqctx.Get(r.Context(), "user")

		data := map[string]interface{}{
			"user": user,
			"page": "help",
			"server": map[string]interface{}{
				"Title":   cfg.Server.Branding.Title,
				"GitOrg":  "webappsgo",
				"GitRepo": "wthr",
			},
			"HostInfo": util.GetHostInfo(r),
		}

		// AI.md PART 14: Content negotiation - JSON or HTML
		NegotiateResponse(w, r, "page/help.tmpl", data)
	}
}

// ShowTermsPage renders the terms of service page with content negotiation (AI.md PART 16)
func ShowTermsPage(db *database.DB, cfg *config.AppConfig) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user, _ := reqctx.Get(r.Context(), "user")

		data := map[string]interface{}{
			"user": user,
			"page": "terms",
			"server": map[string]interface{}{
				"Title":     cfg.Server.Branding.Title,
				"BuildDate": BuildDate,
			},
			"HostInfo": util.GetHostInfo(r),
		}

		NegotiateResponse(w, r, "page/terms.tmpl", data)
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
				"Real-time weather data from Open-Meteo API",
				"Global location support with geocoding",
				"Moon phase tracking and lunar information",
				"Severe weather alerts and warnings",
				"Earthquake monitoring from USGS",
				"Hurricane and tropical storm tracking",
				"Multi-day weather forecasts (up to 16 days)",
				"Multi-format API (JSON, text/plain, GraphQL) with passkey auth mutations",
				"WebSocket real-time alert notifications",
				"Passkey / WebAuthn admin authentication",
			},
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
			"title":        "Privacy Policy",
			"last_updated": BuildDate,
			"data_stored":  true,
			"data_sold":    false,
			"cookies": map[string]interface{}{
				"essential":   true,
				"preferences": true,
				"analytics":   false,
			},
			"data_collection": "Weather collects minimal data necessary to provide weather information. Location data is used solely for delivering location-specific weather forecasts and alerts.",
			"data_retention":  "Session data is retained for the duration of your session. Saved locations are retained until you delete them. Server logs are rotated and deleted per the configured retention policy.",
			"third_parties":   []string{"Open-Meteo (weather data)", "USGS (earthquake data)", "NOAA (hurricane/alert data)"},
			"contact":         "/server/contact",
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

		help := map[string]interface{}{
			"title": "Help",
			"getting_started": map[string]interface{}{
				"description": "Get weather data with a single request",
				"examples": []map[string]interface{}{
					{"description": "Current weather for a city", "curl": "curl " + baseURL + "/London"},
					{"description": "JSON API", "curl": "curl " + baseURL + "/api/v1/weather?location=London"},
					{"description": "Forecast", "curl": "curl " + baseURL + "/api/v1/forecasts?location=Paris&days=5"},
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

// HandleContactFormSubmission handles the contact form POST request (API endpoint)
func HandleContactFormSubmission(db *database.DB, cfg *config.AppConfig) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var form struct {
			Name    string `json:"name" binding:"required"`
			Email   string `json:"email" binding:"required,email"`
			Subject string `json:"subject" binding:"required"`
			Message string `json:"message" binding:"required"`
		}

		if !DecodeAndValidate(w, r, &form) {
			return
		}

		// Try to send email via SMTP if configured (AI.md PART 26)
		smtpService := GetSMTPService(r)
		if smtpService != nil {
			// SMTP available - send email
			emailBody := fmt.Sprintf(`Contact Form Submission

From: %s <%s>
Subject: %s

Message:
%s

---
IP: %s
User Agent: %s
Time: %s`, form.Name, form.Email, form.Subject, form.Message, util.GetClientIP(r), r.UserAgent(), time.Now().Format("2006-01-02 15:04:05"))

			adminEmail := ""
			if cfg != nil {
				adminEmail = strings.TrimSpace(cfg.Server.Admin.Email)
			}
			if adminEmail == "" {
				if err := saveContactToDB(r, form.Name, form.Email, form.Subject, form.Message); err != nil {
					writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"ok": false, "error": "Failed to save message"})
					return
				}
				writeJSON(w, http.StatusOK, map[string]interface{}{"ok": true, "message": "Your message has been saved. We'll respond as soon as possible."})
				return
			}

			err := smtpService.SendEmail(adminEmail, fmt.Sprintf("Contact: %s", form.Subject), emailBody)
			if err != nil {
				// Email failed - save to database as fallback
				if err := saveContactToDB(r, form.Name, form.Email, form.Subject, form.Message); err != nil {
					writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"ok": false, "error": "Failed to send message"})
					return
				}
				writeJSON(w, http.StatusOK, map[string]interface{}{"ok": true, "message": "Your message has been saved. We'll respond as soon as possible."})
				return
			}
			writeJSON(w, http.StatusOK, map[string]interface{}{"ok": true, "message": "Thank you for contacting us. We'll get back to you soon."})
		} else {
			// No SMTP - save to database (AI.md PART 26)
			if err := saveContactToDB(r, form.Name, form.Email, form.Subject, form.Message); err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"ok": false, "error": "Failed to save message"})
				return
			}
			writeJSON(w, http.StatusOK, map[string]interface{}{"ok": true, "message": "Your message has been saved. We'll respond as soon as possible."})
		}
	}
}

// saveContactToDB saves contact form submission to database when SMTP unavailable
// Per AI.md PART 26: Graceful degradation when SMTP not configured
func saveContactToDB(r *http.Request, name, email, subject, message string) error {
	dbInterface, exists := reqctx.Get(r.Context(), "db")
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
	`, name, email, subject, message, util.GetClientIP(r), r.UserAgent(), dbtime.FormatSQLTimestamp(time.Now()))

	return err
}

// GetSMTPService returns the SMTP service from context if available
func GetSMTPService(r *http.Request) *service.SMTPService {
	if smtp, exists := reqctx.Get(r.Context(), "smtp"); exists {
		if smtpService, ok := smtp.(*service.SMTPService); ok {
			return smtpService
		}
	}
	return nil
}
