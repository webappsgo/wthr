package handler

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"sort"
	"strings"

	"github.com/webappsgo/wthr/src/config"
	"github.com/webappsgo/wthr/src/server/middleware"
)

// AdminBrandingHandler manages the branding and SEO area file.
type AdminBrandingHandler struct{}

// NewAdminBrandingHandler creates the server branding handler.
func NewAdminBrandingHandler() *AdminBrandingHandler {
	return &AdminBrandingHandler{}
}

// Show renders the server branding and SEO settings page.
func (h *AdminBrandingHandler) Show(w http.ResponseWriter, r *http.Request) {
	branding, seo, accent := currentBrandingSettings()
	middleware.RenderHTML(w, r, http.StatusOK, "admin/admin_branding.tmpl", AdminTemplateData(r, map[string]interface{}{
		"title":          Translate(r, "admin.branding.title"),
		"page":           "branding",
		"breadcrumb":     Translate(r, "admin.branding.title"),
		"Branding":       branding,
		"HeaderContent":  operatorBodyForDisplay(branding.Content.Header.Body),
		"AboutContent":   operatorBodyForDisplay(branding.Content.About.Body),
		"PrivacyContent": operatorBodyForDisplay(branding.Content.Privacy.Body),
		"FooterContent":  operatorBodyForDisplay(branding.Content.Footer.Body),
		"TermsContent":   operatorBodyForDisplay(branding.Content.Terms.Body),
		"SEO":            seo,
		"Accent":         accent,
		"MetaTagsText":   formatMetaTags(seo.MetaTags),
		"flash":          TakeFlash(w, r),
	}))
}

// Get returns the current branding and SEO configuration.
func (h *AdminBrandingHandler) Get(w http.ResponseWriter, r *http.Request) {
	branding, seo, accent := currentBrandingSettings()
	RespondData(w, r, map[string]interface{}{
		"branding": branding,
		"seo":      seo,
		"accent":   accent,
	})
}

// Update accepts a no-JavaScript admin form submission or an API patch and
// persists the settings to yaml.d/branding.yml.
func (h *AdminBrandingHandler) Update(w http.ResponseWriter, r *http.Request) {
	cfg := config.GetGlobalConfig()
	if cfg == nil {
		respondBrandingUpdateError(w, r, fmt.Errorf("global config is not initialized"))
		return
	}

	if wantsFormSubmission(r) {
		r.Body = http.MaxBytesReader(w, r.Body, 8<<20)
		var parseErr error
		if strings.Contains(r.Header.Get("Content-Type"), "multipart/form-data") {
			parseErr = r.ParseMultipartForm(1 << 20)
			if r.MultipartForm != nil {
				defer func() { _ = r.MultipartForm.RemoveAll() }()
			}
		} else {
			parseErr = r.ParseForm()
		}
		if parseErr != nil {
			respondBrandingUpdateError(w, r, fmt.Errorf("parse branding form: %w", parseErr))
			return
		}
		if err := updateBrandingFromForm(cfg, r); err != nil {
			redirectAdminForm(w, r, adminPagePath(r, "/branding"), "flash.settings_saved", "flash.settings_save_failed", err)
			return
		}
		if err := saveBrandingUploads(cfg, r); err != nil {
			redirectAdminForm(w, r, adminPagePath(r, "/branding"), "flash.settings_saved", "flash.settings_save_failed", err)
			return
		}
	} else if err := updateBrandingFromJSON(cfg, w, r); err != nil {
		respondBrandingUpdateError(w, r, err)
		return
	}

	config.ValidateBrandingConfig(cfg)
	if err := config.SaveBrandingArea(cfg); err != nil {
		respondBrandingUpdateError(w, r, fmt.Errorf("save branding settings: %w", err))
		return
	}
	if wantsFormSubmission(r) {
		redirectAdminForm(w, r, adminPagePath(r, "/branding"), "flash.settings_saved", "flash.settings_save_failed", nil)
		return
	}
	RespondSuccess(w, r, Translate(r, "admin.web.branding_updated_message"), map[string]interface{}{
		"branding": cfg.Server.Branding,
		"seo":      cfg.Server.SEO,
		"accent":   cfg.Web.UI.Accent,
	})
}

func saveBrandingUploads(cfg *config.AppConfig, r *http.Request) error {
	if r.MultipartForm == nil {
		return nil
	}
	for _, upload := range []struct {
		field  string
		target *string
	}{
		{field: "logo_upload", target: &cfg.Server.Branding.Logo},
		{field: "favicon_upload", target: &cfg.Server.Branding.Favicon},
		{field: "og_image_upload", target: &cfg.Server.SEO.OGImage},
	} {
		files := r.MultipartForm.File[upload.field]
		if len(files) == 0 {
			continue
		}
		file, err := files[0].Open()
		if err != nil {
			return fmt.Errorf("open uploaded branding image: %w", err)
		}
		data, readErr := io.ReadAll(io.LimitReader(file, config.BrandingAssetMaxSize()+1))
		closeErr := file.Close()
		if readErr != nil {
			return fmt.Errorf("read uploaded branding image: %w", readErr)
		}
		if closeErr != nil {
			return fmt.Errorf("close uploaded branding image: %w", closeErr)
		}
		if int64(len(data)) > config.BrandingAssetMaxSize() {
			return fmt.Errorf("uploaded branding image exceeds the 2 MiB limit")
		}
		extension := strings.ToLower(filepath.Ext(files[0].Filename))
		digest := sha256.Sum256(data)
		name := hex.EncodeToString(digest[:12]) + extension
		if err := config.SaveBrandingAsset(name, data); err != nil {
			return fmt.Errorf("save uploaded branding image: %w", err)
		}
		contentType := map[string]string{".png": "image/png", ".jpg": "image/jpeg", ".webp": "image/webp", ".ico": "image/x-icon", ".svg": "image/svg+xml"}[extension]
		if err := middleware.SaveBrandingImageVariants(strings.TrimSuffix(name, extension), data, contentType); err != nil {
			return fmt.Errorf("generate uploaded branding image variants: %w", err)
		}
		*upload.target = name
	}
	return nil
}

func currentBrandingSettings() (config.BrandingConfig, config.SEOConfig, string) {
	cfg := config.GetGlobalConfig()
	if cfg == nil {
		return config.BrandingConfig{Title: "wthr"}, config.SEOConfig{TitleSeparator: " - ", OpenGraph: config.OpenGraphConfig{Enabled: true, Type: "website"}, Twitter: config.TwitterCardConfig{Enabled: true, Card: "summary_large_image"}}, ""
	}
	return cfg.Server.Branding, cfg.Server.SEO, cfg.Web.UI.Accent
}

func updateBrandingFromForm(cfg *config.AppConfig, r *http.Request) error {
	form := r.PostForm
	cfg.Server.Branding.Title = strings.TrimSpace(form.Get("title"))
	cfg.Server.Branding.Tagline = strings.TrimSpace(form.Get("tagline"))
	cfg.Server.Branding.Description = strings.TrimSpace(form.Get("description"))
	cfg.Server.Branding.Logo = strings.TrimSpace(form.Get("logo"))
	cfg.Server.Branding.Favicon = strings.TrimSpace(form.Get("favicon"))
	cfg.Server.Branding.Content.Header = operatorContentFromForm(form, "header_content", "html")
	cfg.Server.Branding.Content.About = operatorContentFromForm(form, "about_content", "md")
	cfg.Server.Branding.Content.Privacy = operatorContentFromForm(form, "privacy_content", "md")
	cfg.Server.Branding.Content.Footer = operatorContentFromForm(form, "footer_content", "html")
	cfg.Server.Branding.Content.Terms = operatorContentFromForm(form, "terms_content", "md")
	cfg.Web.UI.Accent = strings.TrimSpace(form.Get("accent"))

	seo := &cfg.Server.SEO
	seo.Keywords = splitBrandingKeywords(form.Get("keywords"))
	seo.Author = strings.TrimSpace(form.Get("author"))
	seo.OGImage = strings.TrimSpace(form.Get("og_image"))
	seo.TwitterHandle = normalizeTwitterHandle(form.Get("twitter_handle"))
	seo.TitleSeparator = form.Get("title_separator")
	seo.Canonical = strings.TrimSpace(form.Get("canonical"))
	seo.Noindex = config.IsTruthy(form.Get("noindex"))
	seo.OpenGraph.Enabled = config.IsTruthy(form.Get("opengraph_enabled"))
	seo.OpenGraph.Type = strings.TrimSpace(form.Get("opengraph_type"))
	seo.OpenGraph.SiteName = strings.TrimSpace(form.Get("opengraph_site_name"))
	seo.Twitter.Enabled = config.IsTruthy(form.Get("twitter_enabled"))
	seo.Twitter.Card = strings.TrimSpace(form.Get("twitter_card"))
	seo.Twitter.Creator = normalizeTwitterHandle(form.Get("twitter_creator"))
	metaTags, err := parseMetaTags(form.Get("meta_tags"))
	if err != nil {
		return err
	}
	seo.MetaTags = metaTags
	return nil
}

func operatorContentFromForm(form interface{ Get(string) string }, key, format string) config.OperatorPageContentConfig {
	body := strings.TrimSpace(form.Get(key))
	if body == "" {
		return config.OperatorPageContentConfig{}
	}
	return config.OperatorPageContentConfig{Format: format, Body: body}
}

func operatorBodyForDisplay(body interface{}) string {
	switch value := body.(type) {
	case string:
		return value
	case map[string]string:
		if text := value["default"]; text != "" {
			return text
		}
		for _, text := range value {
			return text
		}
	case map[string]interface{}:
		if text, ok := value["default"].(string); ok {
			return text
		}
		for _, item := range value {
			if text, ok := item.(string); ok {
				return text
			}
		}
	}
	return ""
}

func updateBrandingFromJSON(cfg *config.AppConfig, w http.ResponseWriter, r *http.Request) error {
	var update struct {
		Branding *config.BrandingConfig `json:"branding"`
		SEO      *config.SEOConfig      `json:"seo"`
		Accent   *string                `json:"accent"`
	}
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	if err := decoder.Decode(&update); err != nil {
		return fmt.Errorf("decode branding JSON request: %w", err)
	}
	var trailing interface{}
	if err := decoder.Decode(&trailing); err != io.EOF {
		return fmt.Errorf("decode branding JSON request: trailing data")
	}
	if update.Branding == nil && update.SEO == nil && update.Accent == nil {
		return fmt.Errorf("branding update contains no supported fields")
	}
	if update.Branding != nil {
		cfg.Server.Branding = *update.Branding
	}
	if update.SEO != nil {
		cfg.Server.SEO = *update.SEO
	}
	if update.Accent != nil {
		cfg.Web.UI.Accent = *update.Accent
	}
	return nil
}

func splitBrandingKeywords(value string) []string {
	items := strings.FieldsFunc(value, func(char rune) bool { return char == ',' || char == '\n' || char == '\r' })
	keywords := make([]string, 0, len(items))
	for _, item := range items {
		if keyword := strings.TrimSpace(item); keyword != "" {
			keywords = append(keywords, keyword)
		}
	}
	return keywords
}

func normalizeTwitterHandle(value string) string {
	value = strings.TrimSpace(value)
	if value != "" && !strings.HasPrefix(value, "@") {
		return "@" + value
	}
	return value
}

func parseMetaTags(value string) (map[string]string, error) {
	metaTags := make(map[string]string)
	for lineNumber, line := range strings.Split(value, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		name, content, found := strings.Cut(line, "=")
		name = strings.TrimSpace(name)
		if !found || name == "" || strings.TrimSpace(content) == "" {
			return nil, fmt.Errorf("meta tag line %d must use name=content format", lineNumber+1)
		}
		metaTags[name] = strings.TrimSpace(content)
	}
	return metaTags, nil
}

func formatMetaTags(metaTags map[string]string) string {
	keys := make([]string, 0, len(metaTags))
	for key := range metaTags {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	lines := make([]string, 0, len(keys))
	for _, key := range keys {
		lines = append(lines, key+"="+metaTags[key])
	}
	return strings.Join(lines, "\n")
}

func respondBrandingUpdateError(w http.ResponseWriter, r *http.Request, err error) {
	if wantsFormSubmission(r) {
		redirectAdminForm(w, r, adminPagePath(r, "/branding"), "flash.settings_saved", "flash.settings_save_failed", err)
		return
	}
	RespondError(w, r, http.StatusBadRequest, ErrBadRequest, Translate(r, "errors.invalid_request"))
}
