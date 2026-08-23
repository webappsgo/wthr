package handler

import (
	"encoding/base64"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/webappsgo/wthr/src/config"
	"github.com/webappsgo/wthr/src/database"
	"github.com/webappsgo/wthr/src/server/middleware"
	models "github.com/webappsgo/wthr/src/server/model"
	"github.com/webappsgo/wthr/src/server/reqctx"
	"github.com/webappsgo/wthr/src/util"
)

// AdminWebHandler handles web settings administration
type AdminWebHandler struct {
	db *database.DB
}

// NewAdminWebHandler creates a new admin web handler
func NewAdminWebHandler(db *database.DB) *AdminWebHandler {
	return &AdminWebHandler{db: db}
}

// ShowWebSettings renders the web settings admin page
// GET /admin/server/web
func (h *AdminWebHandler) ShowWebSettings(w http.ResponseWriter, r *http.Request) {
	cfg := config.GetGlobalConfig()
	adminIDValue, exists := reqctx.Get(r.Context(), "admin_id")
	if !exists {
		http.Redirect(w, r, "/server/admin", http.StatusFound)
		return
	}

	adminID, ok := adminIDValue.(int)
	if !ok {
		http.Redirect(w, r, "/server/admin", http.StatusFound)
		return
	}

	adminModel := &models.AdminModel{DB: database.GetServerDB()}
	admin, err := adminModel.GetByID(int64(adminID))
	if err != nil {
		http.Redirect(w, r, "/server/admin", http.StatusFound)
		return
	}

	robotsTxt := ""
	securityTxt := ""
	if cfg != nil {
		robotsTxt = cfg.Web.RobotsTxt
		securityTxt = cfg.Web.SecurityTxt
	}

	// Get app URL for template variable replacement
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	appURL := scheme + "://" + r.Host

	serverCtx, _ := middleware.GetServerContext(r.Context())

	middleware.RenderHTML(w, r, http.StatusOK, "admin/admin_web.tmpl", util.TemplateData(r, map[string]interface{}{
		"Title":       "Web Settings",
		"RobotsTxt":   robotsTxt,
		"SecurityTxt": securityTxt,
		"AppURL":      appURL,
		"User":        admin,
		"server":      serverCtx,
	}))
}

// GetRobotsTxt retrieves robots.txt content
// GET /{api_version}/admin/server/web/robots
func (h *AdminWebHandler) GetRobotsTxt(w http.ResponseWriter, r *http.Request) {
	cfg := config.GetGlobalConfig()
	robotsTxt := ""

	if cfg != nil {
		robotsTxt = cfg.Web.RobotsTxt
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"ok":      true,
		"content": robotsTxt,
	})
}

// UpdateRobotsTxt updates robots.txt content
// PATCH /{api_version}/admin/server/web/robots
func (h *AdminWebHandler) UpdateRobotsTxt(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Content string `json:"content" binding:"required"`
	}

	if !DecodeAndValidate(w, r, &req) {
		return
	}

	// Update server.yml file (config watcher will auto-reload)
	if err := config.UpdateWebRobotsTxt(req.Content); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]interface{}{
			"error": map[string]interface{}{
				"code":    "UPDATE_FAILED",
				"message": "Failed to update robots.txt in server.yml",
			},
		})
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"ok":      true,
		"message": "robots.txt updated successfully (will auto-reload)",
	})
}

// GetSecurityTxt retrieves security.txt content
// GET /{api_version}/admin/server/web/security
func (h *AdminWebHandler) GetSecurityTxt(w http.ResponseWriter, r *http.Request) {
	cfg := config.GetGlobalConfig()
	securityTxt := ""

	if cfg != nil {
		securityTxt = cfg.Web.SecurityTxt
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"ok":      true,
		"content": securityTxt,
	})
}

// UpdateSecurityTxt updates security.txt content
// PATCH /{api_version}/admin/server/web/security
func (h *AdminWebHandler) UpdateSecurityTxt(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Content string `json:"content" binding:"required"`
	}

	if !DecodeAndValidate(w, r, &req) {
		return
	}

	// Update server.yml file (config watcher will auto-reload)
	if err := config.UpdateWebSecurityTxt(req.Content); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]interface{}{
			"error": map[string]interface{}{
				"code":    "UPDATE_FAILED",
				"message": "Failed to update security.txt in server.yml",
			},
		})
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"ok":      true,
		"message": "security.txt updated successfully (will auto-reload)",
	})
}

// ServeRobotsTxt serves robots.txt file
// GET /robots.txt
func (h *AdminWebHandler) ServeRobotsTxt(w http.ResponseWriter, r *http.Request) {
	cfg := config.GetGlobalConfig()
	robotsTxt := `User-agent: *
Allow: /`

	if cfg != nil && cfg.Web.RobotsTxt != "" {
		robotsTxt = cfg.Web.RobotsTxt
	}

	// Replace template variables
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	appURL := scheme + "://" + r.Host

	robotsTxt = strings.ReplaceAll(robotsTxt, "{app_url}", appURL)

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	// Cache for 24 hours
	w.Header().Set("Cache-Control", "public, max-age=86400")
	w.WriteHeader(http.StatusOK)
	fmt.Fprint(w, robotsTxt)
}

// ServeSecurityTxt serves security.txt file
// GET /.well-known/security.txt
func (h *AdminWebHandler) ServeSecurityTxt(w http.ResponseWriter, r *http.Request) {
	cfg := config.GetGlobalConfig()
	securityTxt := ""

	if cfg != nil && cfg.Web.SecurityTxt != "" {
		securityTxt = cfg.Web.SecurityTxt
	}

	if securityTxt == "" {
		// Generate default security.txt if not configured
		securityTxt = fmt.Sprintf(
			`Contact: mailto:%s
Expires: %s
Preferred-Languages: en`,
			config.DefaultEmailAddress("security", cfg),
			time.Now().AddDate(1, 0, 0).UTC().Format(time.RFC3339),
		)
	}

	// Replace template variables
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	appURL := scheme + "://" + r.Host

	securityTxt = strings.ReplaceAll(securityTxt, "{app_url}", appURL)

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	// Cache for 24 hours
	w.Header().Set("Cache-Control", "public, max-age=86400")
	w.WriteHeader(http.StatusOK)
	fmt.Fprint(w, securityTxt)
}

// ServeSitemap serves dynamically generated sitemap.xml
// GET /sitemap.xml
// Per AI.md PART 16: Include homepage, public pages, docs, API docs
// NEVER include: admin pages, auth pages, api endpoints
func (h *AdminWebHandler) ServeSitemap(w http.ResponseWriter, r *http.Request) {
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	appURL := scheme + "://" + r.Host
	lastmod := time.Now().Format("2006-01-02")

	// Build sitemap XML
	var sb strings.Builder
	sb.WriteString(`<?xml version="1.0" encoding="UTF-8"?>`)
	sb.WriteString("\n")
	sb.WriteString(`<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">`)
	sb.WriteString("\n")

	// Homepage - priority 1.0, daily
	sb.WriteString(fmt.Sprintf("  <url>\n    <loc>%s/</loc>\n    <lastmod>%s</lastmod>\n    <changefreq>daily</changefreq>\n    <priority>1.0</priority>\n  </url>\n", appURL, lastmod))

	// Weather page - public content, priority 0.9, daily
	sb.WriteString(fmt.Sprintf("  <url>\n    <loc>%s/weather</loc>\n    <lastmod>%s</lastmod>\n    <changefreq>daily</changefreq>\n    <priority>0.9</priority>\n  </url>\n", appURL, lastmod))

	// Severe weather - public content, priority 0.8, daily
	sb.WriteString(fmt.Sprintf("  <url>\n    <loc>%s/severe-weather</loc>\n    <lastmod>%s</lastmod>\n    <changefreq>daily</changefreq>\n    <priority>0.8</priority>\n  </url>\n", appURL, lastmod))

	// Earthquakes - public content, priority 0.8, hourly
	sb.WriteString(fmt.Sprintf("  <url>\n    <loc>%s/earthquakes</loc>\n    <lastmod>%s</lastmod>\n    <changefreq>hourly</changefreq>\n    <priority>0.8</priority>\n  </url>\n", appURL, lastmod))

	// Hurricanes - public content, priority 0.8, daily
	sb.WriteString(fmt.Sprintf("  <url>\n    <loc>%s/hurricanes</loc>\n    <lastmod>%s</lastmod>\n    <changefreq>daily</changefreq>\n    <priority>0.8</priority>\n  </url>\n", appURL, lastmod))

	// Moon phase - public content, priority 0.7, daily
	sb.WriteString(fmt.Sprintf("  <url>\n    <loc>%s/moon</loc>\n    <lastmod>%s</lastmod>\n    <changefreq>daily</changefreq>\n    <priority>0.7</priority>\n  </url>\n", appURL, lastmod))

	// OpenAPI docs - priority 0.7, weekly
	sb.WriteString(fmt.Sprintf("  <url>\n    <loc>%s/openapi</loc>\n    <lastmod>%s</lastmod>\n    <changefreq>weekly</changefreq>\n    <priority>0.7</priority>\n  </url>\n", appURL, lastmod))

	// GraphQL endpoint - priority 0.7, weekly
	sb.WriteString(fmt.Sprintf("  <url>\n    <loc>%s/graphql</loc>\n    <lastmod>%s</lastmod>\n    <changefreq>weekly</changefreq>\n    <priority>0.7</priority>\n  </url>\n", appURL, lastmod))

	// Health check - priority 0.5, weekly
	sb.WriteString(fmt.Sprintf("  <url>\n    <loc>%s/server/healthz</loc>\n    <lastmod>%s</lastmod>\n    <changefreq>weekly</changefreq>\n    <priority>0.5</priority>\n  </url>\n", appURL, lastmod))

	sb.WriteString("</urlset>\n")

	w.Header().Set("Content-Type", "application/xml; charset=utf-8")
	// Cache for 1 hour
	w.Header().Set("Cache-Control", "public, max-age=3600")
	w.WriteHeader(http.StatusOK)
	fmt.Fprint(w, sb.String())
}

// ServeFavicon serves the favicon.ico
// GET /favicon.ico
// Per AI.md PART 16: Embedded default, customizable via admin panel
func (h *AdminWebHandler) ServeFavicon(w http.ResponseWriter, r *http.Request) {
	cfg := config.GetGlobalConfig()

	// Check if custom favicon is configured
	if cfg != nil && cfg.Web.FaviconURL != "" {
		// Redirect to custom favicon URL
		http.Redirect(w, r, cfg.Web.FaviconURL, http.StatusFound)
		return
	}

	// Serve embedded default favicon (16x16 weather icon)
	// Base64 encoded minimal ICO file - simple cloud/sun icon
	faviconB64 := "AAABAAEAEBAAAAEAIABoBAAAFgAAACgAAAAQAAAAIAAAAAEAIAAAAAAAAAQAABILAAASCwAAAAAAAAAAAAD///8A////AP///wD///8A////AP///wCRub7/kbm+/5G5vv+Rub7/////AP///wD///8A////AP///wD///8A////AP///wD///8A////AP///wCRub7/tbKe/7Wynv+1sp7/kbm+/////wD///8A////AP///wD///8A////AP///wD///8A////AP///wCRub7/tbKe/7Wynv+1sp7/tbKe/5G5vv////8A////AP///wD///8A////AP///wD///8A////AP///wCRub7/tbKe/8a7n//Gu5//xruf/7Wynv+Rub7/////AP///wD///8A////AP///wD///8A////AP///wCRub7/tbKe/7Wynv+1sp7/tbKe/7Wynv+1sp7/kbm+/////wD///8A////AP///wD///8A////AP///wCRub7/kbm+/5G5vv+Rub7/kbm+/5G5vv+Rub7/kbm+/////wD///8A////AP///wD///8A////AP///wD///8A////AP///wD///8A////AP///wD///8A////AP///wD///8A////AP///wD///8A////AP///wD//wAA/D8AAP4/AAD/fwAA/38AAP9/AAD/fwAA/38AAP9/AAD/fwAA/38AAP9/AAD/fwAA/38AAP9/AAD//wAA"

	faviconData, err := base64.StdEncoding.DecodeString(faviconB64)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "image/x-icon")
	// Cache for 7 days
	w.Header().Set("Cache-Control", "public, max-age=604800")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(faviconData)
}
