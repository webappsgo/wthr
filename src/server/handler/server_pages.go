package handler

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/apimgr/weather/src/config"
	"github.com/apimgr/weather/src/database"
	"github.com/apimgr/weather/src/server/model"
	"github.com/apimgr/weather/src/server/service"
	"github.com/apimgr/weather/src/util"

	"github.com/gin-gonic/gin"
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
func ShowAboutPage(db *database.DB, cfg *config.AppConfig) gin.HandlerFunc {
	return func(c *gin.Context) {
		user, _ := c.Get("user")

		// Get server configuration
		settingsModel := &models.SettingsModel{DB: db.DB}

		// Get Tor configuration if available
		torEnabled := settingsModel.GetBool("tor.enabled", false)
		onionAddress := settingsModel.GetString("tor.onion_address", "")

		data := gin.H{
			"user": user,
			"page": "about",
			"server": gin.H{
				"Title":       cfg.Server.Branding.Title,
				"Description": cfg.Server.Branding.Description,
				"Version":     Version,
				"BuildDate":   BuildDate,
				"Mode":        cfg.Server.Mode,
				"GitOrg":      "apimgr",
				"GitRepo":     "weather",
				"Tor": gin.H{
					"Enabled":      torEnabled,
					"OnionAddress": onionAddress,
				},
			},
			"HostInfo": utils.GetHostInfo(c),
		}

		// AI.md PART 14: Content negotiation - JSON or HTML
		NegotiateResponse(c, "about.tmpl", data)
	}
}

// ShowPrivacyPage renders the privacy policy page with content negotiation (AI.md PART 14)
func ShowPrivacyPage(db *database.DB, cfg *config.AppConfig) gin.HandlerFunc {
	return func(c *gin.Context) {
		user, _ := c.Get("user")

		data := gin.H{
			"user": user,
			"page": "privacy",
			"server": gin.H{
				"Title":     cfg.Server.Branding.Title,
				"BuildDate": BuildDate,
			},
			"HostInfo": utils.GetHostInfo(c),
		}

		// AI.md PART 14: Content negotiation - JSON or HTML
		NegotiateResponse(c, "privacy.tmpl", data)
	}
}

// ShowContactPage renders the contact form page with content negotiation (AI.md PART 14)
func ShowContactPage(db *database.DB, cfg *config.AppConfig) gin.HandlerFunc {
	return func(c *gin.Context) {
		user, _ := c.Get("user")

		data := gin.H{
			"user": user,
			"page": "contact",
			"server": gin.H{
				"Title":   cfg.Server.Branding.Title,
				"GitOrg":  "apimgr",
				"GitRepo": "weather",
			},
			"HostInfo": utils.GetHostInfo(c),
		}

		// AI.md PART 14: Content negotiation - JSON or HTML
		NegotiateResponse(c, "contact.tmpl", data)
	}
}

// ShowHelpPage renders the help & documentation page with content negotiation (AI.md PART 14)
func ShowHelpPage(db *database.DB, cfg *config.AppConfig) gin.HandlerFunc {
	return func(c *gin.Context) {
		user, _ := c.Get("user")

		data := gin.H{
			"user": user,
			"page": "help",
			"server": gin.H{
				"Title":   cfg.Server.Branding.Title,
				"GitOrg":  "apimgr",
				"GitRepo": "weather",
			},
			"HostInfo": utils.GetHostInfo(c),
		}

		// AI.md PART 14: Content negotiation - JSON or HTML
		NegotiateResponse(c, "help.tmpl", data)
	}
}

// ShowTermsPage renders the terms of service page with content negotiation (AI.md PART 16)
func ShowTermsPage(db *database.DB, cfg *config.AppConfig) gin.HandlerFunc {
	return func(c *gin.Context) {
		user, _ := c.Get("user")

		data := gin.H{
			"user": user,
			"page": "terms",
			"server": gin.H{
				"Title":     cfg.Server.Branding.Title,
				"BuildDate": BuildDate,
			},
			"HostInfo": utils.GetHostInfo(c),
		}

		NegotiateResponse(c, "terms.tmpl", data)
	}
}

// GetAboutAPI returns about information as JSON (AI.md PART 14: /api/v1/server/about)
func GetAboutAPI(db *database.DB, cfg *config.AppConfig) gin.HandlerFunc {
	return func(c *gin.Context) {
		settingsModel := &models.SettingsModel{DB: db.DB}
		torEnabled := settingsModel.GetBool("tor.enabled", false)
		onionAddress := settingsModel.GetString("tor.onion_address", "")

		RespondNegotiatedData(c, http.StatusOK, gin.H{
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
				"Multi-format API (JSON, text/plain, GraphQL)",
				"WebSocket real-time alert notifications",
			},
			"links": gin.H{
				"github":  "https://github.com/apimgr/weather",
				"docs":    "/openapi",
				"graphql": "/graphql",
			},
			"tor": gin.H{
				"enabled":       torEnabled,
				"onion_address": onionAddress,
			},
		})
	}
}

// GetPrivacyAPI returns the privacy policy as JSON (AI.md PART 14: /api/v1/server/privacy)
func GetPrivacyAPI(db *database.DB, cfg *config.AppConfig) gin.HandlerFunc {
	return func(c *gin.Context) {
		RespondNegotiatedData(c, http.StatusOK, gin.H{
			"title":        "Privacy Policy",
			"last_updated": BuildDate,
			"data_stored":  true,
			"data_sold":    false,
			"cookies": gin.H{
				"essential":   true,
				"preferences": true,
				"analytics":   false,
			},
			"data_collection": "Weather Service collects minimal data necessary to provide weather information. Location data is used solely for delivering location-specific weather forecasts and alerts.",
			"data_retention":  "Session data is retained for the duration of your session. Saved locations are retained until you delete them. Server logs are rotated and deleted per the configured retention policy.",
			"third_parties":   []string{"Open-Meteo (weather data)", "USGS (earthquake data)", "NOAA (hurricane/alert data)"},
			"contact":         "/server/contact",
		})
	}
}

// GetHelpAPI returns help content as JSON (AI.md PART 14: /api/v1/server/help)
func GetHelpAPI(db *database.DB, cfg *config.AppConfig) gin.HandlerFunc {
	return func(c *gin.Context) {
		settingsModel := &models.SettingsModel{DB: db.DB}
		torEnabled := settingsModel.GetBool("tor.enabled", false)
		onionAddress := settingsModel.GetString("tor.onion_address", "")
		hostInfo := utils.GetHostInfo(c)
		baseURL := hostInfo.ExampleURL

		help := gin.H{
			"title": "Help",
			"getting_started": gin.H{
				"description": "Get weather data with a single request",
				"examples": []gin.H{
					{"description": "Current weather for a city", "curl": "curl " + baseURL + "/London"},
					{"description": "JSON API", "curl": "curl " + baseURL + "/api/v1/weather?location=London"},
					{"description": "Forecast", "curl": "curl " + baseURL + "/api/v1/forecasts?location=Paris&days=5"},
				},
			},
			"features": []gin.H{
				{"name": "Weather Forecasts", "description": "16-day global forecasts with hourly/daily breakdown"},
				{"name": "Severe Weather Alerts", "description": "Real-time alerts from US, Canada, UK, Australia, Japan, Mexico"},
				{"name": "Earthquake Data", "description": "Real-time seismic activity from USGS"},
				{"name": "Hurricane Tracking", "description": "Active tropical storm monitoring from NOAA NHC"},
				{"name": "Moon Phases", "description": "Lunar cycles, illumination, rise/set times"},
			},
			"api_documentation": gin.H{
				"swagger":  "/openapi",
				"graphql":  "/graphql",
				"examples": "/examples",
			},
			"faq": []gin.H{
				{"question": "Do I need an API key?", "answer": "No, the API is free and requires no authentication for basic access."},
				{"question": "What is the rate limit?", "answer": "Anonymous: 20 requests/minute. Authenticated: 100 requests/minute."},
				{"question": "What data sources are used?", "answer": "Open-Meteo for weather, USGS for earthquakes, NOAA for hurricanes and US alerts."},
			},
		}

		if torEnabled && onionAddress != "" {
			help["tor_access"] = gin.H{
				"enabled":       true,
				"onion_address": onionAddress,
				"instructions":  "Download Tor Browser from https://www.torproject.org/download/ and navigate to the onion address.",
			}
		}

		RespondNegotiatedData(c, http.StatusOK, help)
	}
}

// GetTermsAPI returns terms of service as JSON (AI.md PART 14: /api/v1/server/terms)
func GetTermsAPI(db *database.DB, cfg *config.AppConfig) gin.HandlerFunc {
	return func(c *gin.Context) {
		RespondNegotiatedData(c, http.StatusOK, gin.H{
			"title":        "Terms of Service",
			"last_updated": BuildDate,
			"sections": []gin.H{
				{"title": "Acceptance of Terms", "content": "By accessing or using Weather Service, you agree to be bound by these terms. If you do not agree, do not use the service."},
				{"title": "Description of Service", "content": "Weather Service provides weather forecasts, severe weather alerts, earthquake data, hurricane tracking, and moon phase information through a web interface and API."},
				{"title": "Acceptable Use", "content": "You may use the service for lawful purposes. You must not attempt to disrupt the service, circumvent rate limits, or use the service to harm others."},
				{"title": "Data Accuracy", "content": "Weather data is sourced from third-party providers (Open-Meteo, USGS, NOAA). We do not guarantee the accuracy, completeness, or timeliness of data. Do not rely solely on this service for safety-critical decisions."},
				{"title": "Limitation of Liability", "content": "Weather Service is provided as-is without warranty. We are not liable for any damages arising from use of the service or reliance on data provided."},
				{"title": "Changes to Terms", "content": "We may update these terms at any time. Continued use after changes constitutes acceptance of new terms."},
			},
		})
	}
}

// HandleContactFormSubmission handles the contact form POST request (API endpoint)
func HandleContactFormSubmission(db *database.DB, cfg *config.AppConfig) gin.HandlerFunc {
	return func(c *gin.Context) {
		var form struct {
			Name    string `json:"name" binding:"required"`
			Email   string `json:"email" binding:"required,email"`
			Subject string `json:"subject" binding:"required"`
			Message string `json:"message" binding:"required"`
		}

		if err := c.ShouldBindJSON(&form); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid form data"})
			return
		}

		// Try to send email via SMTP if configured (AI.md PART 26)
		smtpService := GetSMTPService(c)
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
Time: %s`, form.Name, form.Email, form.Subject, form.Message, c.ClientIP(), c.Request.UserAgent(), time.Now().Format("2006-01-02 15:04:05"))

			adminEmail := ""
			if cfg != nil {
				adminEmail = strings.TrimSpace(cfg.Server.Admin.Email)
			}
			if adminEmail == "" {
				if err := saveContactToDB(c, form.Name, form.Email, form.Subject, form.Message); err != nil {
					c.JSON(http.StatusInternalServerError, gin.H{"ok": false, "error": "Failed to save message"})
					return
				}
				c.JSON(http.StatusOK, gin.H{"ok": true, "message": "Your message has been saved. We'll respond as soon as possible."})
				return
			}

			err := smtpService.SendEmail(adminEmail, fmt.Sprintf("Contact: %s", form.Subject), emailBody)
			if err != nil {
				// Email failed - save to database as fallback
				if err := saveContactToDB(c, form.Name, form.Email, form.Subject, form.Message); err != nil {
					c.JSON(http.StatusInternalServerError, gin.H{"ok": false, "error": "Failed to send message"})
					return
				}
				c.JSON(http.StatusOK, gin.H{"ok": true, "message": "Your message has been saved. We'll respond as soon as possible."})
				return
			}
			c.JSON(http.StatusOK, gin.H{"ok": true, "message": "Thank you for contacting us. We'll get back to you soon."})
		} else {
			// No SMTP - save to database (AI.md PART 26)
			if err := saveContactToDB(c, form.Name, form.Email, form.Subject, form.Message); err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"ok": false, "error": "Failed to save message"})
				return
			}
			c.JSON(http.StatusOK, gin.H{"ok": true, "message": "Your message has been saved. We'll respond as soon as possible."})
		}
	}
}

// saveContactToDB saves contact form submission to database when SMTP unavailable
// Per AI.md PART 26: Graceful degradation when SMTP not configured
func saveContactToDB(c *gin.Context, name, email, subject, message string) error {
	dbInterface, exists := c.Get("db")
	if !exists {
		return fmt.Errorf("database not available")
	}
	db, ok := dbInterface.(*database.DB)
	if !ok || db == nil {
		return fmt.Errorf("database not available")
	}

	// Create contact_submissions table if not exists
	_, err := db.DB.Exec(`
		CREATE TABLE IF NOT EXISTS contact_submissions (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT NOT NULL,
			email TEXT NOT NULL,
			subject TEXT NOT NULL,
			message TEXT NOT NULL,
			ip_address TEXT,
			user_agent TEXT,
			status TEXT NOT NULL DEFAULT 'pending',
			created_at INTEGER NOT NULL DEFAULT (strftime('%s', 'now'))
		)
	`)
	if err != nil {
		return fmt.Errorf("failed to create table: %w", err)
	}

	// Insert contact submission
	_, err = db.DB.Exec(`
		INSERT INTO contact_submissions (name, email, subject, message, ip_address, user_agent)
		VALUES (?, ?, ?, ?, ?, ?)
	`, name, email, subject, message, c.ClientIP(), c.Request.UserAgent())

	return err
}

// GetSMTPService returns the SMTP service from context if available
func GetSMTPService(c *gin.Context) *service.SMTPService {
	if smtp, exists := c.Get("smtp"); exists {
		if smtpService, ok := smtp.(*service.SMTPService); ok {
			return smtpService
		}
	}
	return nil
}
