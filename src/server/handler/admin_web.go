package handler

import (
	"encoding/base64"
	"fmt"
	"net/http"
	"strconv"
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
	adminIDValue, exists := reqctx.GetValue(r.Context(), "admin_id")
	if !exists {
		http.Redirect(w, r, "/server/administration", http.StatusFound)
		return
	}

	adminID, ok := adminIDValue.(int)
	if !ok {
		http.Redirect(w, r, "/server/administration", http.StatusFound)
		return
	}

	adminModel := &models.AdminModel{DB: database.GetUsersDB()}
	admin, err := adminModel.GetByID(int64(adminID))
	if err != nil {
		http.Redirect(w, r, "/server/administration", http.StatusFound)
		return
	}

	securityTxt := ""
	securityFields := parseSecurityTxtFields("")
	robots := config.DefaultRobotsConfig()
	adminPath := config.DefaultAdminPath
	feed := config.FeedConfig{MaxItems: config.DefaultFeedMaxItems}
	if cfg != nil {
		securityTxt = cfg.Web.SecurityTxt
		securityFields = parseSecurityTxtFields(securityTxt)
		robots = cfg.Web.Robots
		adminPath = cfg.GetAdminPath()
		feed = cfg.Server.SEO.Feed
	}

	// Get app URL for template variable replacement
	appURL := util.BuildURL(r, "")

	serverCtx, _ := middleware.GetServerContext(r.Context())
	apiPath := "/api/v1"
	if cfg != nil {
		apiPath = cfg.GetAPIPath()
	}

	middleware.RenderHTML(w, r, http.StatusOK, "admin/admin_web.tmpl", AdminTemplateData(r, map[string]interface{}{
		"Title":              Translate(r, "admin.web.title"),
		"RobotsAllow":        strings.Join(robots.Allow, "\n"),
		"RobotsDeny":         strings.Join(robots.Deny, "\n"),
		"RobotsPreview":      robots.RobotsTxt(adminPath, appURL),
		"FeedMaxItems":       feed.MaxItems,
		"FeedTitle":          feed.Title,
		"FeedMaxLimit":       config.MaxFeedMaxItems,
		"CORS":               currentCORSView(),
		"flash":              TakeFlash(w, r),
		"SecurityTxt":        securityTxt,
		"SecurityContact":    securityFields["Contact"],
		"SecurityLanguages":  securityFields["Preferred-Languages"],
		"SecurityPolicy":     securityFields["Policy"],
		"SecurityEncryption": securityFields["Encryption"],
		"AppURL":             appURL,
		"User":               admin,
		"server":             serverCtx,
		"MCPEnabled":         true,
		"MCPEndpointURL":     util.BuildURL(r, apiPath+"/server/mcp"),
	}))
}

// robotsView is the robots.txt configuration as the admin API returns it.
type robotsView struct {
	Allow   []string `json:"allow"`
	Deny    []string `json:"deny"`
	Preview string   `json:"preview"`
}

// currentRobotsView builds the robots view for the request, with the preview
// rendered exactly as /robots.txt serves it.
func currentRobotsView(r *http.Request) robotsView {
	robots := config.DefaultRobotsConfig()
	adminPath := config.DefaultAdminPath
	if cfg := config.GetGlobalConfig(); cfg != nil {
		robots = cfg.Web.Robots
		adminPath = cfg.GetAdminPath()
	}
	return robotsView{
		Allow:   robots.Allow,
		Deny:    robots.Deny,
		Preview: robots.RobotsTxt(adminPath, util.BuildURL(r, "")),
	}
}

// GetRobotsTxt returns the robots.txt allow and deny paths with a preview of the
// generated file.
// GET /api/{api_version}/server/{admin_path}/web/robots (and /web/robots/preview)
func (h *AdminWebHandler) GetRobotsTxt(w http.ResponseWriter, r *http.Request) {
	RespondData(w, r, currentRobotsView(r))
}

// splitRobotsLines splits a textarea value into one path per line.
func splitRobotsLines(value string) []string {
	return strings.FieldsFunc(value, func(c rune) bool { return c == '\n' || c == '\r' })
}

// UpdateRobotsTxt saves the robots.txt allow and deny paths. A browser form post
// (one path per line in each textarea, or `reset` to restore the defaults)
// redirects back to the page with a flash; a JSON body carries `allow` and
// `deny` arrays.
// POST /server/{admin_path}/web/robots, PATCH /api/{api_version}/server/{admin_path}/web/robots
func (h *AdminWebHandler) UpdateRobotsTxt(w http.ResponseWriter, r *http.Request) {
	var allow, deny []string
	reset := false
	if wantsFormSubmission(r) {
		if err := r.ParseForm(); err != nil {
			RespondError(w, r, http.StatusBadRequest, ErrBadRequest, Translate(r, "errors.invalid_request"))
			return
		}
		reset = r.PostForm.Get("reset") != ""
		allow = splitRobotsLines(r.PostForm.Get("allow"))
		deny = splitRobotsLines(r.PostForm.Get("deny"))
	} else {
		var req struct {
			Allow []string `json:"allow"`
			Deny  []string `json:"deny"`
		}
		if !DecodeAndValidate(w, r, &req) {
			return
		}
		allow, deny = req.Allow, req.Deny
	}
	if reset {
		defaults := config.DefaultRobotsConfig()
		allow, deny = defaults.Allow, defaults.Deny
	}

	if config.GetGlobalConfig() == nil {
		InternalError(w, r, Translate(r, "errors.internal_error"))
		return
	}

	errs := config.UpdateWebRobots(allow, deny)
	if wantsFormSubmission(r) {
		var err error
		if len(errs) > 0 {
			err = errs[0]
		}
		redirectAdminForm(w, r, adminPagePath(r, "/web"), "flash.settings_saved", "flash.settings_save_failed", err)
		return
	}
	if len(errs) > 0 {
		ValidationFailed(w, r, Translate(r, "errors.validation_failed"), map[string]interface{}{
			"field":  errs[0].Field,
			"errors": errs,
		})
		return
	}
	RespondSuccess(w, r, Translate(r, "admin.web.robots_updated_message"), map[string]interface{}{
		"robots": currentRobotsView(r),
	})
}

// corsView is the web.cors setting as the admin API returns it, with the response
// headers a browser from the first listed origin (or any origin for *) would receive.
type corsView struct {
	CORS     string            `json:"cors"`
	AllowAll bool              `json:"allow_all"`
	Origins  string            `json:"origins"`
	Preview  map[string]string `json:"preview"`
}

// currentCORSView builds the CORS view of the live configuration.
func currentCORSView() corsView {
	setting := config.DefaultCORSSetting
	if cfg := config.GetGlobalConfig(); cfg != nil {
		setting = cfg.Web.CORS
	}
	trimmed := strings.TrimSpace(setting)
	view := corsView{CORS: trimmed, AllowAll: trimmed == config.DefaultCORSSetting, Preview: map[string]string{}}
	if !view.AllowAll {
		view.Origins = trimmed
	}

	sampleOrigin := "https://example.com"
	if first := strings.TrimSpace(strings.Split(trimmed, ",")[0]); first != "" && !view.AllowAll {
		sampleOrigin = first
	}
	for name, values := range middleware.CORSHeaderPreview(trimmed, sampleOrigin) {
		view.Preview[name] = strings.Join(values, ", ")
	}
	return view
}

// corsSettingFromInput turns the admin input into a web.cors value: allow all gives *,
// otherwise the comma-separated origins, and an empty list turns CORS off.
func corsSettingFromInput(allowAll bool, origins string) string {
	if allowAll {
		return config.DefaultCORSSetting
	}
	parts := strings.FieldsFunc(origins, func(c rune) bool { return c == ',' || c == '\n' || c == '\r' })
	cleaned := make([]string, 0, len(parts))
	for _, part := range parts {
		if part = strings.TrimSpace(part); part != "" {
			cleaned = append(cleaned, part)
		}
	}
	return strings.Join(cleaned, ",")
}

// GetCORSSettings returns web.cors with a preview of the headers it produces.
// GET /api/{api_version}/server/{admin_path}/web/cors
func (h *AdminWebHandler) GetCORSSettings(w http.ResponseWriter, r *http.Request) {
	RespondData(w, r, currentCORSView())
}

// UpdateCORSSettings saves web.cors. A browser form post carries `allow_all` and
// `origins` (comma or one per line) and redirects back with a flash; a JSON body
// carries the same two fields. The change applies to the next request.
// POST /server/{admin_path}/web/cors, PATCH /api/{api_version}/server/{admin_path}/web/cors
func (h *AdminWebHandler) UpdateCORSSettings(w http.ResponseWriter, r *http.Request) {
	var allowAll bool
	var origins string
	if wantsFormSubmission(r) {
		if err := r.ParseForm(); err != nil {
			RespondError(w, r, http.StatusBadRequest, ErrBadRequest, Translate(r, "errors.invalid_request"))
			return
		}
		allowAll = r.PostForm.Get("allow_all") != ""
		origins = r.PostForm.Get("origins")
	} else {
		var req struct {
			AllowAll bool   `json:"allow_all"`
			Origins  string `json:"origins"`
		}
		if !DecodeAndValidate(w, r, &req) {
			return
		}
		allowAll, origins = req.AllowAll, req.Origins
	}

	errs := config.UpdateWebCORS(corsSettingFromInput(allowAll, origins))
	if wantsFormSubmission(r) {
		if len(errs) > 0 {
			SetFlash(w, r, "error", "flash.cors_invalid")
			http.Redirect(w, r, adminPagePath(r, "/web"), http.StatusSeeOther)
			return
		}
		SetFlash(w, r, "success", "flash.settings_saved")
		http.Redirect(w, r, adminPagePath(r, "/web"), http.StatusSeeOther)
		return
	}
	if len(errs) > 0 {
		details := make(map[string]string, len(errs))
		for _, e := range errs {
			details[e.Field] = Translate(r, e.Message)
		}
		ValidationFailed(w, r, Translate(r, "errors.validation_failed"), map[string]interface{}{"fields": details})
		return
	}
	RespondSuccess(w, r, Translate(r, "admin.web.cors_updated_message"), map[string]interface{}{"cors": currentCORSView()})
}

// UpdateFeedSettings saves the syndication feed limits (server.seo.feed.*).
// POST /server/{admin_path}/web/feeds, PATCH /api/{api_version}/server/{admin_path}/web/feeds
func (h *AdminWebHandler) UpdateFeedSettings(w http.ResponseWriter, r *http.Request) {
	var req struct {
		MaxItems int    `json:"max_items" binding:"min=1,max=200"`
		Title    string `json:"title" binding:"max=140"`
	}
	if wantsFormSubmission(r) {
		if err := r.ParseForm(); err != nil {
			redirectAdminForm(w, r, adminPagePath(r, "/web"), "flash.settings_saved", "flash.settings_save_failed", err)
			return
		}
		maxItems, err := strconv.Atoi(r.PostForm.Get("max_items"))
		req.MaxItems, req.Title = maxItems, r.PostForm.Get("title")
		if err != nil || maxItems < 1 || maxItems > config.MaxFeedMaxItems || len(req.Title) > 140 {
			redirectAdminForm(w, r, adminPagePath(r, "/web"), "flash.settings_saved", "flash.settings_save_failed", fmt.Errorf("invalid feed settings"))
			return
		}
	} else if !DecodeAndValidate(w, r, &req) {
		return
	}

	cfg := config.GetGlobalConfig()
	var saveErr error
	if cfg == nil {
		saveErr = fmt.Errorf("global config not initialized")
	} else {
		cfg.Server.SEO.Feed = config.FeedConfig{MaxItems: req.MaxItems, Title: strings.TrimSpace(req.Title)}
		config.ValidateBrandingConfig(cfg)
		saveErr = config.SaveBrandingArea(cfg)
	}

	if wantsFormSubmission(r) {
		redirectAdminForm(w, r, adminPagePath(r, "/web"), "flash.settings_saved", "flash.settings_save_failed", saveErr)
		return
	}
	if saveErr != nil {
		InternalError(w, r, Translate(r, "errors.admin.web.feed_update_failed"))
		return
	}
	RespondSuccess(w, r, Translate(r, "admin.web.feed_updated_message"), map[string]interface{}{
		"max_items": req.MaxItems,
		"title":     strings.TrimSpace(req.Title),
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
	if wantsFormSubmission(r) {
		if err := r.ParseForm(); err != nil {
			redirectAdminForm(w, r, adminPagePath(r, "/web"), "flash.settings_saved", "flash.settings_save_failed", err)
			return
		}
		contact := strings.TrimSpace(r.PostForm.Get("security.contact"))
		languages := strings.TrimSpace(r.PostForm.Get("security.languages"))
		policy := strings.TrimSpace(r.PostForm.Get("web.security.report_url"))
		encryption := strings.TrimSpace(r.PostForm.Get("web.security.keyservers"))
		if contact == "" || len(contact) > 500 || strings.ContainsAny(contact, "\r\n") || len(languages) > 100 || len(policy) > 2048 || len(encryption) > 2048 || strings.ContainsAny(languages+policy+encryption, "\r\n") {
			redirectAdminForm(w, r, adminPagePath(r, "/web"), "flash.settings_saved", "flash.settings_save_failed", fmt.Errorf("invalid security.txt settings"))
			return
		}
		var fields []string
		fields = append(fields, "Contact: "+contact)
		if languages != "" {
			fields = append(fields, "Preferred-Languages: "+languages)
		}
		if policy != "" {
			fields = append(fields, "Policy: "+policy)
		}
		if encryption != "" {
			fields = append(fields, "Encryption: "+encryption)
		}
		if err := config.UpdateWebSecurityTxt(strings.Join(fields, "\n")); err != nil {
			redirectAdminForm(w, r, adminPagePath(r, "/web"), "flash.settings_saved", "flash.settings_save_failed", err)
			return
		}
		redirectAdminForm(w, r, adminPagePath(r, "/web"), "flash.settings_saved", "flash.settings_save_failed", nil)
		return
	}
	var req struct {
		Content string `json:"content" binding:"required"`
	}

	if !DecodeAndValidate(w, r, &req) {
		return
	}

	// Update server.yml file (config watcher will auto-reload)
	if err := config.UpdateWebSecurityTxt(req.Content); err != nil {
		RespondError(w, r, http.StatusInternalServerError, "UPDATE_FAILED", Translate(r, "errors.admin.web.security_update_failed"))
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"ok":      true,
		"message": Translate(r, "admin.web.security_updated_message"),
	})
}

func parseSecurityTxtFields(content string) map[string]string {
	fields := make(map[string]string)
	for _, line := range strings.Split(content, "\n") {
		name, value, found := strings.Cut(line, ":")
		if !found {
			continue
		}
		name = strings.TrimSpace(name)
		if name == "Contact" || name == "Preferred-Languages" || name == "Policy" || name == "Encryption" {
			fields[name] = strings.TrimSpace(value)
		}
	}
	return fields
}

// ServeRobotsTxt serves robots.txt generated from web.robots.allow and
// web.robots.deny, with the absolute sitemap URL for the host the client used.
// GET /robots.txt
func (h *AdminWebHandler) ServeRobotsTxt(w http.ResponseWriter, r *http.Request) {
	view := currentRobotsView(r)

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Cache-Control", "public, max-age=3600")
	w.WriteHeader(http.StatusOK)
	if r.Method == http.MethodHead {
		return
	}
	if _, err := fmt.Fprint(w, view.Preview); err != nil {
		return
	}
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
	if _, err := fmt.Fprint(w, securityTxt); err != nil {
		return
	}
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
