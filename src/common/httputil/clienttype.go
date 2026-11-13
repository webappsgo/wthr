package httputil

import (
	"net/http"
	"strings"
)

// httpToolAgents is the AI.md PART 14 denylist of non-interactive HTTP clients.
// Any User-Agent not matched here is treated as a browser.
var httpToolAgents = []string{
	"curl/", "wget/", "httpie/", "xh/", "hurl/", "libcurl/",
	"python-requests/", "python-urllib/", "python-httpx/", "aiohttp/", "urllib3/",
	"go-http-client/", "go-resty/", "fasthttp", "reqwest/", "ureq/", "hyper/",
	"axios/", "node-fetch/", "undici", "superagent/", "deno/", "bun/",
	"okhttp/", "apache-httpclient/", "java/", "guzzlehttp/", "libwww-perl/", "lwp-trivial/", "faraday", "dart/", "windowspowershell/",
	"postmanruntime/", "insomnia/", "bruno-runtime/", "k6/",
}

// textBrowserAgents are interactive text-mode browsers that receive no-JS HTML.
var textBrowserAgents = []string{
	"lynx/", "w3m/", "links ", "links/", "elinks/", "browsh/", "carbonyl/", "netsurf",
}

// IsHTTPTool reports whether the request came from a non-interactive HTTP tool
// (curl, wget, httpie, scripting libraries, API clients, or no User-Agent at
// all), which AI.md PART 14 answers with formatted text on frontend routes.
func IsHTTPTool(r *http.Request) bool {
	ua := strings.ToLower(r.Header.Get("User-Agent"))
	if ua == "" {
		return true
	}
	for _, tool := range httpToolAgents {
		if strings.Contains(ua, tool) {
			return true
		}
	}

	return false
}

// IsTextBrowser reports whether the request came from a text-mode browser,
// which receives regular no-JavaScript HTML.
func IsTextBrowser(r *http.Request) bool {
	ua := strings.ToLower(r.Header.Get("User-Agent"))
	for _, name := range textBrowserAgents {
		if strings.Contains(ua, name) {
			return true
		}
	}

	return false
}

// FrontendWantsText applies the AI.md PART 14 frontend chain to a request whose
// page is already rendered as HTML: an explicit Accept: text/html keeps HTML,
// an explicit Accept: text/plain asks for formatted text, text browsers keep
// HTML, and HTTP tools get formatted text. Everything else gets HTML.
func FrontendWantsText(r *http.Request) bool {
	accept := r.Header.Get("Accept")
	switch {
	case strings.Contains(accept, "text/html"):
		return false
	case strings.Contains(accept, "text/plain"):
		return true
	case strings.HasPrefix(r.Header.Get("User-Agent"), "wthr-cli/"):
		return false
	case IsTextBrowser(r):
		return false
	}

	return IsHTTPTool(r)
}
