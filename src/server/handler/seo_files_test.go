package handler

import (
	"encoding/xml"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/webappsgo/wthr/src/config"
	"github.com/webappsgo/wthr/src/server/service"
)

func TestServeRobotsTxtIsGeneratedWithAbsoluteSitemap(t *testing.T) {
	resetGlobalConfig(t)
	cfg := &config.AppConfig{}
	cfg.Server.AdminPath = "console"
	cfg.Web.Robots = config.RobotsConfig{Allow: []string{"/", "/api"}, Deny: []string{"/server/{admin_path}"}}
	config.SetGlobalConfig(cfg)

	t.Setenv("DOMAIN", "wthr.example")
	h := &AdminWebHandler{}
	r := httptest.NewRequest(http.MethodGet, "http://wthr.example/robots.txt", nil)
	w := httptest.NewRecorder()
	h.ServeRobotsTxt(w, r)

	body := w.Body.String()
	for _, want := range []string{"User-agent: *\n", "Allow: /\n", "Allow: /api\n", "Disallow: /server/console\n", "Sitemap: http://wthr.example"} {
		if !strings.Contains(body, want) {
			t.Errorf("robots.txt missing %q:\n%s", want, body)
		}
	}
	if strings.Contains(strings.ToLower(body), "gptbot") || strings.Count(body, "User-agent:") != 1 {
		t.Errorf("robots.txt must carry a single User-agent: * stanza and no AI-specific rules:\n%s", body)
	}
}

func TestSitemapListsOnlyPublicPagesRobotsAllows(t *testing.T) {
	resetGlobalConfig(t)
	cfg := &config.AppConfig{}
	cfg.Server.AdminPath = "administration"
	cfg.Web.Robots = config.RobotsConfig{Allow: []string{"/"}, Deny: []string{"/server/administration", "/earthquakes"}}
	config.SetGlobalConfig(cfg)

	t.Setenv("DOMAIN", "wthr.example")
	h := &AdminWebHandler{}
	r := httptest.NewRequest(http.MethodGet, "http://wthr.example/sitemap.xml", nil)
	w := httptest.NewRecorder()
	h.ServeSitemap(w, r)

	if got := w.Header().Get("Content-Type"); got != "application/xml; charset=utf-8" {
		t.Errorf("Content-Type = %q", got)
	}
	var parsed sitemapURLSet
	if err := xml.Unmarshal(w.Body.Bytes(), &parsed); err != nil {
		t.Fatalf("sitemap is not valid XML: %v\n%s", err, w.Body.String())
	}
	locs := map[string]bool{}
	for _, entry := range parsed.URLs {
		if !strings.HasPrefix(entry.Loc, "http://wthr.example") {
			t.Errorf("loc %q is not absolute for the request host", entry.Loc)
		}
		locs[entry.Loc] = true
	}
	for loc := range locs {
		if strings.HasSuffix(loc, "/earthquakes") {
			t.Errorf("a path robots.txt denies must never be in the sitemap: %q", loc)
		}
	}
	homeFound, weatherFound := false, false
	for loc := range locs {
		homeFound = homeFound || loc == "http://wthr.example/"
		weatherFound = weatherFound || loc == "http://wthr.example/weather"
	}
	if !homeFound || !weatherFound {
		t.Errorf("home and weather pages missing: %v", locs)
	}
	for loc := range locs {
		for _, banned := range []string{"/api/", "/server/administration", "/server/auth", "/feed.xml", "/rss.xml", "healthz"} {
			if strings.Contains(loc, banned) {
				t.Errorf("sitemap lists forbidden URL %q", loc)
			}
		}
	}
}

func TestSitemapHeadHasNoBody(t *testing.T) {
	resetGlobalConfig(t)
	h := &AdminWebHandler{}
	r := httptest.NewRequest(http.MethodHead, "http://wthr.example/sitemap.xml", nil)
	w := httptest.NewRecorder()
	h.ServeSitemap(w, r)
	if w.Code != http.StatusOK || w.Body.Len() != 0 {
		t.Errorf("HEAD status=%d bodyLen=%d, want 200 and empty", w.Code, w.Body.Len())
	}
}

func feedTestChannel() feedChannel {
	when := time.Date(2026, 11, 13, 12, 0, 0, 0, time.UTC)
	return feedChannel{
		Title:       "wthr",
		Description: "Recent earthquakes",
		Link:        "http://wthr.example/earthquakes",
		SelfLink:    "http://wthr.example/feed.xml",
		Lang:        "en",
		Updated:     when,
		Items: []feedItem{{
			ID:        "http://wthr.example/earthquakes/detail/us1",
			Title:     "M 4.6 earthquake - 12 km N of Somewhere",
			Link:      "http://wthr.example/earthquakes/detail/us1",
			Summary:   "Magnitude 4.6 earthquake.",
			Published: when,
			Updated:   when,
		}},
	}
}

func TestRenderAtomAndRSSAreValidXML(t *testing.T) {
	channel := feedTestChannel()

	atom, err := renderAtom(channel)
	if err != nil {
		t.Fatalf("renderAtom: %v", err)
	}
	var atomDoc struct {
		XMLName xml.Name `xml:"feed"`
		Entries []struct {
			ID string `xml:"id"`
		} `xml:"entry"`
	}
	if err := xml.Unmarshal(atom, &atomDoc); err != nil {
		t.Fatalf("atom is not valid XML: %v\n%s", err, atom)
	}
	if atomDoc.XMLName.Space != "http://www.w3.org/2005/Atom" || len(atomDoc.Entries) != 1 || atomDoc.Entries[0].ID != channel.Items[0].ID {
		t.Errorf("atom document = %+v\n%s", atomDoc, atom)
	}

	rss, err := renderRSS(channel)
	if err != nil {
		t.Fatalf("renderRSS: %v", err)
	}
	var rssDoc struct {
		Version string `xml:"version,attr"`
		Items   []struct {
			GUID string `xml:"guid"`
		} `xml:"channel>item"`
	}
	if err := xml.Unmarshal(rss, &rssDoc); err != nil {
		t.Fatalf("rss is not valid XML: %v\n%s", err, rss)
	}
	if rssDoc.Version != "2.0" || len(rssDoc.Items) != 1 || rssDoc.Items[0].GUID != channel.Items[0].ID {
		t.Errorf("rss document = %+v\n%s", rssDoc, rss)
	}
}

func TestFeedWithNoItemsIsStillValid(t *testing.T) {
	channel := feedTestChannel()
	channel.Items = nil
	h := &FeedHandler{earthquakes: service.NewEarthquakeService()}
	_ = h

	for name, render := range map[string]func(feedChannel) ([]byte, error){"atom": renderAtom, "rss": renderRSS} {
		body, err := render(channel)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		var anyDoc struct{ XMLName xml.Name }
		if err := xml.Unmarshal(body, &anyDoc); err != nil {
			t.Errorf("%s with zero entries is not valid XML: %v", name, err)
		}
	}
}

func TestXMLSafeAndTruncate(t *testing.T) {
	if got := xmlSafe("a\x00b\x0bc\u0001d"); got != "abcd" {
		t.Errorf("xmlSafe = %q, want invalid XML characters removed", got)
	}
	if got := xmlSafe("tab\tnl\n\u00e9\u4e16"); got != "tab\tnl\n\u00e9\u4e16" {
		t.Errorf("xmlSafe altered valid text: %q", got)
	}
	long := strings.Repeat("x", 300)
	if got := truncateRunes(long, 140); len([]rune(got)) != 140 || !strings.HasSuffix(got, "\u2026") {
		t.Errorf("truncateRunes length = %d", len([]rune(got)))
	}
	if got := truncateRunes("short", 140); got != "short" {
		t.Errorf("truncateRunes changed a short string: %q", got)
	}
}

func TestWriteFeedConditionalRequests(t *testing.T) {
	body := []byte("<feed/>")
	modified := time.Date(2026, 11, 13, 12, 0, 0, 0, time.UTC)

	first := httptest.NewRecorder()
	writeFeed(first, httptest.NewRequest(http.MethodGet, "/feed.xml", nil), "application/atom+xml; charset=utf-8", body, modified)
	if first.Code != http.StatusOK || first.Header().Get("ETag") == "" || first.Header().Get("Last-Modified") == "" {
		t.Fatalf("first response: code=%d headers=%v", first.Code, first.Header())
	}
	if got := first.Header().Get("Cache-Control"); got != "public, max-age=300" {
		t.Errorf("Cache-Control = %q", got)
	}

	byTag := httptest.NewRequest(http.MethodGet, "/feed.xml", nil)
	byTag.Header.Set("If-None-Match", first.Header().Get("ETag"))
	w := httptest.NewRecorder()
	writeFeed(w, byTag, "application/atom+xml; charset=utf-8", body, modified)
	if w.Code != http.StatusNotModified || w.Body.Len() != 0 {
		t.Errorf("If-None-Match: code=%d bodyLen=%d, want 304 and empty", w.Code, w.Body.Len())
	}

	bySince := httptest.NewRequest(http.MethodGet, "/feed.xml", nil)
	bySince.Header.Set("If-Modified-Since", modified.Format(http.TimeFormat))
	w = httptest.NewRecorder()
	writeFeed(w, bySince, "application/atom+xml; charset=utf-8", body, modified)
	if w.Code != http.StatusNotModified {
		t.Errorf("If-Modified-Since: code=%d, want 304", w.Code)
	}

	head := httptest.NewRecorder()
	writeFeed(head, httptest.NewRequest(http.MethodHead, "/feed.xml", nil), "application/atom+xml; charset=utf-8", body, modified)
	if head.Code != http.StatusOK || head.Body.Len() != 0 {
		t.Errorf("HEAD: code=%d bodyLen=%d, want 200 and empty", head.Code, head.Body.Len())
	}
}
