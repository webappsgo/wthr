package handler

import (
	"encoding/xml"
	"net/http"
	"time"

	"github.com/webappsgo/wthr/src/config"
	"github.com/webappsgo/wthr/src/util"
)

// sitemapPage is one public frontend page the sitemap may list (AI.md PART 16).
type sitemapPage struct {
	path       string
	priority   string
	changefreq string
	// dynamic pages show data that changes daily, so they carry a lastmod
	dynamic bool
}

// sitemapPages are the public HTML GET routes an anonymous visitor can open.
// Admin, auth, account and API routes, feeds and files never appear here.
var sitemapPages = []sitemapPage{
	{"/", "1.0", "daily", true},
	{"/weather", "0.9", "daily", true},
	{"/severe-weather", "0.8", "daily", true},
	{"/earthquakes", "0.8", "hourly", true},
	{"/hurricanes", "0.8", "daily", true},
	{"/moon", "0.7", "daily", true},
	{"/server/about", "0.8", "weekly", false},
	{"/server/help", "0.8", "weekly", false},
	{"/server/contact", "0.8", "weekly", false},
	{"/server/privacy", "0.8", "weekly", false},
	{"/server/terms", "0.8", "weekly", false},
	{"/server/security", "0.8", "weekly", false},
	{"/server/docs/swagger", "0.7", "weekly", false},
	{"/server/docs/graphql", "0.7", "weekly", false},
}

type sitemapURL struct {
	Loc        string `xml:"loc"`
	LastMod    string `xml:"lastmod,omitempty"`
	ChangeFreq string `xml:"changefreq"`
	Priority   string `xml:"priority"`
}

type sitemapURLSet struct {
	XMLName xml.Name     `xml:"urlset"`
	XMLNS   string       `xml:"xmlns,attr"`
	URLs    []sitemapURL `xml:"url"`
}

// buildSitemap lists the public pages that robots.txt allows, as absolute URLs
// built for the request host, capped at max_urls (AI.md PART 16 "Scope").
func buildSitemap(r *http.Request) sitemapURLSet {
	robots := config.DefaultRobotsConfig()
	adminPath := config.DefaultAdminPath
	maxURLs := config.DefaultSitemapMaxURLs
	if cfg := config.GetGlobalConfig(); cfg != nil {
		robots = cfg.Web.Robots
		adminPath = cfg.GetAdminPath()
		if cfg.Server.SEO.Sitemap.MaxURLs > 0 {
			maxURLs = cfg.Server.SEO.Sitemap.MaxURLs
		}
	}

	today := time.Now().UTC().Format("2006-01-02")
	set := sitemapURLSet{XMLNS: "http://www.sitemaps.org/schemas/sitemap/0.9"}
	for _, page := range sitemapPages {
		if len(set.URLs) >= maxURLs {
			break
		}
		if !robots.Allows(page.path, adminPath) {
			continue
		}
		entry := sitemapURL{
			Loc:        util.BuildURL(r, page.path),
			ChangeFreq: page.changefreq,
			Priority:   page.priority,
		}
		if page.dynamic {
			entry.LastMod = today
		}
		set.URLs = append(set.URLs, entry)
	}
	return set
}

// ServeSitemap serves /sitemap.xml. It is always served; there is no setting
// that turns it off (AI.md PART 16). It is a fixed-format file: no content
// negotiation, GET and HEAD only.
func (h *AdminWebHandler) ServeSitemap(w http.ResponseWriter, r *http.Request) {
	body, err := xml.MarshalIndent(buildSitemap(r), "", "  ")
	if err != nil {
		InternalError(w, r, Translate(r, "errors.internal_error"))
		return
	}

	w.Header().Set("Content-Type", "application/xml; charset=utf-8")
	w.Header().Set("Cache-Control", "public, max-age=3600")
	w.WriteHeader(http.StatusOK)
	if r.Method == http.MethodHead {
		return
	}
	_, _ = w.Write([]byte(xml.Header))
	_, _ = w.Write(body)
	_, _ = w.Write([]byte("\n"))
}
