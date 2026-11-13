package httputil

import (
	"net/http/httptest"
	"testing"
)

func TestFrontendWantsText(t *testing.T) {
	cases := []struct {
		name, ua, accept string
		want             bool
	}{
		{"curl default", "curl/8.5.0", "*/*", true},
		{"wget", "Wget/1.21", "", true},
		{"empty user agent", "", "", true},
		{"browser", "Mozilla/5.0 Chrome/120", "text/html,application/xhtml+xml,*/*", false},
		{"curl asking for html", "curl/8.5.0", "text/html", false},
		{"browser asking for plain", "Mozilla/5.0 Chrome/120", "text/plain", true},
		{"html beats plain", "curl/8.5.0", "text/html, text/plain", false},
		{"lynx", "Lynx/2.9", "*/*", false},
		{"browsh", "Browsh/1.8", "*/*", false},
		{"python requests", "python-requests/2.31", "*/*", true},
		{"go client", "Go-http-client/1.1", "*/*", true},
		{"postman", "PostmanRuntime/7.36", "*/*", true},
		{"unknown agent is a browser", "SomeNewBrowser/1.0", "*/*", false},
		{"our cli", "wthr-cli/1.0", "*/*", false},
		{"plain browser without accept", "Mozilla/5.0 Chrome/120", "", false},
	}
	for _, c := range cases {
		req := httptest.NewRequest("GET", "/", nil)
		req.Header.Set("User-Agent", c.ua)
		if c.accept != "" {
			req.Header.Set("Accept", c.accept)
		}
		if got := FrontendWantsText(req); got != c.want {
			t.Errorf("%s: got %v want %v", c.name, got, c.want)
		}
	}
}
