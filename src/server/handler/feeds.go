package handler

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/xml"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/webappsgo/wthr/src/config"
	"github.com/webappsgo/wthr/src/server/service"
	"github.com/webappsgo/wthr/src/util"
)

const (
	// feedEarthquakeWindow is the USGS summary feed the syndication feeds read:
	// magnitude 2.5 and above over the last week, which the detail pages also cover.
	feedEarthquakeWindow = "2.5_week"
	feedTitleMaxRunes    = 140
	feedSummaryMaxRunes  = 280
)

// FeedHandler serves /feed.xml (Atom 1.0) and /rss.xml (RSS 2.0). The primary
// public resource of this project is recent earthquakes (IDEA.md). Both feeds
// carry the same items and are always served (AI.md PART 16 "Feeds").
type FeedHandler struct {
	earthquakes *service.EarthquakeService
}

// NewFeedHandler creates the syndication feed handler.
func NewFeedHandler(earthquakes *service.EarthquakeService) *FeedHandler {
	return &FeedHandler{earthquakes: earthquakes}
}

// feedItem is one feed entry; every field is plain text.
type feedItem struct {
	ID        string
	Title     string
	Link      string
	Summary   string
	Published time.Time
	Updated   time.Time
}

// feedChannel is the feed-level metadata shared by both formats.
type feedChannel struct {
	Title       string
	Description string
	Link        string
	SelfLink    string
	Lang        string
	Updated     time.Time
	Items       []feedItem
}

// xmlSafe removes characters that are invalid in XML 1.0, so an upstream string
// can never make a feed unparsable.
func xmlSafe(s string) string {
	return strings.Map(func(r rune) rune {
		switch {
		case r == 0x9 || r == 0xA || r == 0xD,
			r >= 0x20 && r <= 0xD7FF,
			r >= 0xE000 && r <= 0xFFFD,
			r >= 0x10000 && r <= 0x10FFFF:
			return r
		}
		return -1
	}, s)
}

// truncateRunes shortens s to at most max runes, ending in an ellipsis.
func truncateRunes(s string, max int) string {
	if utf8.RuneCountInString(s) <= max {
		return s
	}
	runes := []rune(s)
	return string(runes[:max-1]) + "…"
}

// buildChannel assembles the channel and its items for the request. An
// upstream failure yields a valid feed with zero entries, never an error.
func (h *FeedHandler) buildChannel(r *http.Request, selfPath string) feedChannel {
	maxItems := config.DefaultFeedMaxItems
	title := ""
	if cfg := config.GetGlobalConfig(); cfg != nil {
		if cfg.Server.SEO.Feed.MaxItems > 0 {
			maxItems = cfg.Server.SEO.Feed.MaxItems
		}
		title = cfg.Server.SEO.Feed.Title
		if title == "" {
			title = cfg.Server.Branding.Title
		}
	}
	if title == "" {
		title = Translate(r, "app.name")
	}

	channel := feedChannel{
		Title:       xmlSafe(truncateRunes(title, feedTitleMaxRunes)),
		Description: xmlSafe(Translate(r, "feeds.description")),
		Link:        util.BuildURL(r, "/earthquakes"),
		SelfLink:    util.BuildURL(r, selfPath),
		Lang:        Lang(r),
	}

	if h.earthquakes != nil {
		if collection, err := h.earthquakes.GetEarthquakes(feedEarthquakeWindow); err == nil && collection != nil {
			quakes := append([]service.Earthquake(nil), collection.Earthquakes...)
			sort.SliceStable(quakes, func(i, j int) bool { return quakes[i].Time.After(quakes[j].Time) })
			for _, quake := range quakes {
				if quake.ID == "" || (quake.Type != "" && quake.Type != "earthquake") {
					continue
				}
				if len(channel.Items) >= maxItems {
					break
				}
				channel.Items = append(channel.Items, h.earthquakeItem(r, quake))
			}
		}
	}

	channel.Updated = time.Now().UTC()
	if len(channel.Items) > 0 {
		channel.Updated = channel.Items[0].Updated
		for _, item := range channel.Items {
			if item.Updated.After(channel.Updated) {
				channel.Updated = item.Updated
			}
		}
	}
	return channel
}

// earthquakeItem renders one earthquake as a feed entry with an absolute permalink.
func (h *FeedHandler) earthquakeItem(r *http.Request, quake service.Earthquake) feedItem {
	args := map[string]string{
		"magnitude": formatMagnitude(quake.Magnitude),
		"place":     quake.Place,
		"depth":     formatMagnitude(quake.Depth),
		"time":      quake.Time.UTC().Format("2006-01-02 15:04"),
	}
	link := util.BuildURL(r, "/earthquakes/detail/"+url.PathEscape(quake.ID))
	updated := quake.UpdatedTime
	if updated.IsZero() || updated.Before(quake.Time) {
		updated = quake.Time
	}
	return feedItem{
		ID:        link,
		Title:     xmlSafe(truncateRunes(TranslateFormat(r, "feeds.earthquake_title", args), feedTitleMaxRunes)),
		Link:      link,
		Summary:   xmlSafe(truncateRunes(TranslateFormat(r, "feeds.earthquake_summary", args), feedSummaryMaxRunes)),
		Published: quake.Time.UTC(),
		Updated:   updated.UTC(),
	}
}

// formatMagnitude prints a value with one decimal.
func formatMagnitude(v float64) string {
	return strconv.FormatFloat(v, 'f', 1, 64)
}

type atomLink struct {
	Rel  string `xml:"rel,attr,omitempty"`
	Type string `xml:"type,attr,omitempty"`
	Href string `xml:"href,attr"`
}

type atomSummary struct {
	Type string `xml:"type,attr"`
	Text string `xml:",chardata"`
}

type atomEntry struct {
	ID        string      `xml:"id"`
	Title     string      `xml:"title"`
	Link      atomLink    `xml:"link"`
	Published string      `xml:"published"`
	Updated   string      `xml:"updated"`
	Summary   atomSummary `xml:"summary"`
}

type atomAuthor struct {
	Name string `xml:"name"`
}

type atomFeed struct {
	XMLName  xml.Name    `xml:"http://www.w3.org/2005/Atom feed"`
	Lang     string      `xml:"http://www.w3.org/XML/1998/namespace lang,attr"`
	Title    string      `xml:"title"`
	Subtitle string      `xml:"subtitle"`
	ID       string      `xml:"id"`
	Updated  string      `xml:"updated"`
	Author   atomAuthor  `xml:"author"`
	Links    []atomLink  `xml:"link"`
	Entries  []atomEntry `xml:"entry"`
}

type rssGUID struct {
	IsPermaLink bool   `xml:"isPermaLink,attr"`
	Value       string `xml:",chardata"`
}

type rssItem struct {
	Title       string  `xml:"title"`
	Link        string  `xml:"link"`
	GUID        rssGUID `xml:"guid"`
	PubDate     string  `xml:"pubDate"`
	Description string  `xml:"description"`
}

type rssAtomLink struct {
	Href string `xml:"href,attr"`
	Rel  string `xml:"rel,attr"`
	Type string `xml:"type,attr"`
}

type rssChannel struct {
	Title         string      `xml:"title"`
	Link          string      `xml:"link"`
	Description   string      `xml:"description"`
	Language      string      `xml:"language"`
	LastBuildDate string      `xml:"lastBuildDate"`
	Self          rssAtomLink `xml:"http://www.w3.org/2005/Atom link"`
	Items         []rssItem   `xml:"item"`
}

type rssFeed struct {
	XMLName xml.Name   `xml:"rss"`
	Version string     `xml:"version,attr"`
	Channel rssChannel `xml:"channel"`
}

// renderAtom encodes the channel as Atom 1.0 (RFC 4287).
func renderAtom(channel feedChannel) ([]byte, error) {
	feed := atomFeed{
		Lang:     channel.Lang,
		Title:    channel.Title,
		Subtitle: channel.Description,
		ID:       channel.SelfLink,
		Updated:  channel.Updated.UTC().Format(time.RFC3339),
		Author:   atomAuthor{Name: channel.Title},
		Links: []atomLink{
			{Rel: "self", Type: "application/atom+xml", Href: channel.SelfLink},
			{Rel: "alternate", Type: "text/html", Href: channel.Link},
		},
	}
	for _, item := range channel.Items {
		feed.Entries = append(feed.Entries, atomEntry{
			ID:        item.ID,
			Title:     item.Title,
			Link:      atomLink{Rel: "alternate", Href: item.Link},
			Published: item.Published.Format(time.RFC3339),
			Updated:   item.Updated.Format(time.RFC3339),
			Summary:   atomSummary{Type: "text", Text: item.Summary},
		})
	}
	body, err := xml.MarshalIndent(feed, "", "  ")
	if err != nil {
		return nil, err
	}
	return append([]byte(xml.Header), append(body, '\n')...), nil
}

// renderRSS encodes the channel as RSS 2.0.
func renderRSS(channel feedChannel) ([]byte, error) {
	feed := rssFeed{
		Version: "2.0",
		Channel: rssChannel{
			Title:         channel.Title,
			Link:          channel.Link,
			Description:   channel.Description,
			Language:      channel.Lang,
			LastBuildDate: channel.Updated.UTC().Format(time.RFC1123Z),
			Self:          rssAtomLink{Href: channel.SelfLink, Rel: "self", Type: "application/rss+xml"},
		},
	}
	for _, item := range channel.Items {
		feed.Channel.Items = append(feed.Channel.Items, rssItem{
			Title:       item.Title,
			Link:        item.Link,
			GUID:        rssGUID{IsPermaLink: true, Value: item.ID},
			PubDate:     item.Published.UTC().Format(time.RFC1123Z),
			Description: item.Summary,
		})
	}
	body, err := xml.MarshalIndent(feed, "", "  ")
	if err != nil {
		return nil, err
	}
	return append([]byte(xml.Header), append(body, '\n')...), nil
}

// writeFeed sends a feed with ETag, Last-Modified and conditional-request
// handling. Feeds are fixed-format files: GET and HEAD only, no negotiation.
func writeFeed(w http.ResponseWriter, r *http.Request, contentType string, body []byte, modified time.Time) {
	sum := sha256.Sum256(body)
	etag := `"` + hex.EncodeToString(sum[:16]) + `"`
	modified = modified.UTC().Truncate(time.Second)

	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Cache-Control", "public, max-age=300")
	w.Header().Set("ETag", etag)
	w.Header().Set("Last-Modified", modified.Format(http.TimeFormat))

	if match := r.Header.Get("If-None-Match"); match != "" {
		for _, candidate := range strings.Split(match, ",") {
			if strings.TrimSpace(candidate) == etag || strings.TrimSpace(candidate) == "*" {
				w.WriteHeader(http.StatusNotModified)
				return
			}
		}
	} else if since, err := http.ParseTime(r.Header.Get("If-Modified-Since")); err == nil && !modified.After(since) {
		w.WriteHeader(http.StatusNotModified)
		return
	}

	w.WriteHeader(http.StatusOK)
	if r.Method == http.MethodHead {
		return
	}
	_, _ = w.Write(body)
}

// ServeAtom serves GET /feed.xml as Atom 1.0.
func (h *FeedHandler) ServeAtom(w http.ResponseWriter, r *http.Request) {
	channel := h.buildChannel(r, "/feed.xml")
	body, err := renderAtom(channel)
	if err != nil {
		InternalError(w, r, Translate(r, "errors.internal_error"))
		return
	}
	writeFeed(w, r, "application/atom+xml; charset=utf-8", body, channel.Updated)
}

// ServeRSS serves GET /rss.xml as RSS 2.0.
func (h *FeedHandler) ServeRSS(w http.ResponseWriter, r *http.Request) {
	channel := h.buildChannel(r, "/rss.xml")
	body, err := renderRSS(channel)
	if err != nil {
		InternalError(w, r, Translate(r, "errors.internal_error"))
		return
	}
	writeFeed(w, r, "application/rss+xml; charset=utf-8", body, channel.Updated)
}
