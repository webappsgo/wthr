package service

import (
	"encoding/json"
	"html"
	"html/template"
	"net/url"
	"strings"

	"github.com/webappsgo/wthr/src/config"
)

// TrackingScriptPath is the same-origin route that serves every tracker's
// inline script body, so pages never carry inline JavaScript (AI.md PART 12).
const TrackingScriptPath = "/server/tracking.js"

// Tracker is what one analytics provider contributes: external tags for the
// page and an optional JavaScript body served from TrackingScriptPath.
type Tracker struct {
	Tags string
	Body string
	// Hosts that must be added to script-src and connect-src
	Hosts []string
	// Hosts that must be added to img-src
	ImgHosts []string
}

// jsString quotes a config value as a JavaScript string literal; config text is
// never concatenated raw into JavaScript.
func jsString(v string) string {
	b, _ := json.Marshal(v)
	return string(b)
}

// origin returns scheme://host of a configured instance URL, or "" when invalid.
func origin(raw string) string {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Scheme == "" || u.Host == "" {
		return ""
	}
	return u.Scheme + "://" + u.Host
}

// TrackerFor maps the server.tracking config to its tags, script body and the
// CSP hosts it needs. The second result is false when tracking is off.
func TrackerFor(cfg config.TrackingConfig) (Tracker, bool) {
	id := html.EscapeString(cfg.ID)
	base := strings.TrimSuffix(strings.TrimSpace(cfg.URL), "/")
	baseOrigin := origin(base)
	var hosts []string
	if baseOrigin != "" {
		hosts = []string{baseOrigin}
	}

	switch cfg.Type {
	case "google":
		if cfg.ID == "" {
			return Tracker{}, false
		}
		return Tracker{
			Tags: `<script async src="https://www.googletagmanager.com/gtag/js?id=` + url.QueryEscape(cfg.ID) + `"></script>`,
			Body: "window.dataLayer=window.dataLayer||[];function gtag(){dataLayer.push(arguments);}" +
				"gtag('js',new Date());gtag('config'," + jsString(cfg.ID) + ");",
			Hosts: []string{"https://www.googletagmanager.com", "https://www.google-analytics.com"},
		}, true
	case "matomo", "piwik":
		if cfg.ID == "" || baseOrigin == "" {
			return Tracker{}, false
		}
		return Tracker{
			Body: "var _paq=window._paq=window._paq||[];_paq.push(['trackPageView']);" +
				"_paq.push(['enableLinkTracking']);_paq.push(['setTrackerUrl'," + jsString(base+"/matomo.php") + "]);" +
				"_paq.push(['setSiteId'," + jsString(cfg.ID) + "]);var g=document.createElement('script');" +
				"g.async=true;g.src=" + jsString(base+"/matomo.js") + ";document.head.appendChild(g);",
			Hosts: hosts,
		}, true
	case "owa":
		if cfg.ID == "" || baseOrigin == "" {
			return Tracker{}, false
		}
		return Tracker{
			Body: "var owa_baseUrl=" + jsString(base+"/") + ";var owa_cmds=window.owa_cmds=window.owa_cmds||[];" +
				"owa_cmds.push(['setSiteId'," + jsString(cfg.ID) + "]);owa_cmds.push(['trackPageView']);owa_cmds.push(['trackClicks']);" +
				"var o=document.createElement('script');o.async=true;" +
				"o.src=owa_baseUrl+'modules/base/js/owa.tracker-combined-min.js';document.head.appendChild(o);",
			Hosts: hosts,
		}, true
	case "fathom":
		if cfg.ID == "" {
			return Tracker{}, false
		}
		src := "https://cdn.usefathom.com/script.js"
		fathomHosts := []string{"https://cdn.usefathom.com"}
		if baseOrigin != "" {
			src = base + "/tracker.js"
			fathomHosts = hosts
		}
		return Tracker{
			Tags:  `<script src="` + html.EscapeString(src) + `" data-site="` + id + `" defer></script>`,
			Hosts: fathomHosts,
		}, true
	case "plausible":
		if cfg.ID == "" {
			return Tracker{}, false
		}
		src := "https://plausible.io/js/script.js"
		plausibleHosts := []string{"https://plausible.io"}
		if baseOrigin != "" {
			src = base + "/js/script.js"
			plausibleHosts = hosts
		}
		return Tracker{
			Tags:  `<script defer data-domain="` + id + `" src="` + html.EscapeString(src) + `"></script>`,
			Hosts: plausibleHosts,
		}, true
	case "umami":
		if cfg.ID == "" || baseOrigin == "" {
			return Tracker{}, false
		}
		return Tracker{
			Tags:  `<script defer src="` + html.EscapeString(base) + `/script.js" data-website-id="` + id + `"></script>`,
			Hosts: hosts,
		}, true
	case "simple":
		return Tracker{
			Tags:     `<script async defer src="https://scripts.simpleanalyticscdn.com/latest.js"></script>`,
			Hosts:    []string{"https://scripts.simpleanalyticscdn.com"},
			ImgHosts: []string{"https://queue.simpleanalyticscdn.com"},
		}, true
	case "cloudflare":
		if cfg.ID == "" {
			return Tracker{}, false
		}
		return Tracker{
			Tags:  `<script defer src="https://static.cloudflareinsights.com/beacon.min.js" data-cf-beacon='{"token": "` + id + `"}'></script>`,
			Hosts: []string{"https://static.cloudflareinsights.com", "https://cloudflareinsights.com"},
		}, true
	}
	return Tracker{}, false
}

// TrackingScript returns the page markup: external tags only, plus the
// same-origin TrackingScriptPath tag when the provider has a script body. The
// caller renders it only when tracking is configured AND consent is given AND
// the request is not served through an overlay network.
func TrackingScript(cfg config.TrackingConfig) template.HTML {
	t, ok := TrackerFor(cfg)
	if !ok {
		return ""
	}
	out := t.Tags
	if t.Body != "" {
		out += `<script async src="` + TrackingScriptPath + `"></script>`
	}
	return template.HTML(out)
}

// TrackingScriptBody is what GET /server/tracking.js serves. Empty means the
// handler answers 404.
func TrackingScriptBody(cfg config.TrackingConfig) string {
	t, ok := TrackerFor(cfg)
	if !ok {
		return ""
	}
	return t.Body
}

// TrackingNoscript returns the <noscript> pixel for providers that have one
// (only "simple"). It is not valid <head> content: render it as the first child
// of <body>, under the same conditions as TrackingScript.
func TrackingNoscript(cfg config.TrackingConfig) template.HTML {
	if cfg.Type != "simple" {
		return ""
	}
	return template.HTML(`<noscript><img src="https://queue.simpleanalyticscdn.com/noscript.gif" alt="" referrerpolicy="no-referrer-when-downgrade" /></noscript>`)
}

// TrackingCSPSources returns the hosts the configured provider needs in
// script-src and connect-src, and in img-src. Nothing is added when tracking is
// off; 'unsafe-inline', nonces and hashes are never added for tracking.
func TrackingCSPSources(cfg config.TrackingConfig) (scriptConnect []string, img []string) {
	t, ok := TrackerFor(cfg)
	if !ok {
		return nil, nil
	}
	return t.Hosts, t.ImgHosts
}
