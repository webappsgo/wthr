package handler

import (
	"net/http/httptest"
	"strconv"
	"testing"
)

// TestShouldRespondTextFollowsPart14 covers the AI.md PART 14 priority chain:
// .txt first, explicit JSON beats plain text and HTTP-tool detection, explicit
// HTML beats plain text on frontend routes, and non-interactive tools get text
// on API routes only when they sent no explicit preference.
func TestShouldRespondTextFollowsPart14(t *testing.T) {
	cases := []struct {
		name, path, ua, accept string
		want                   bool
	}{
		{"api json beats curl", "/api/v1/x", "curl/8", "application/json", false},
		{"api json beats plain", "/api/v1/x", "Mozilla/5.0", "application/json, text/plain", false},
		{"api plain", "/api/v1/x", "Mozilla/5.0", "text/plain", true},
		{"api curl default", "/api/v1/x", "curl/8", "*/*", true},
		{"api browser default", "/api/v1/x", "Mozilla/5.0", "*/*", false},
		{"api our cli", "/api/v1/x", "wthr-cli/1", "*/*", false},
		{"frontend html beats plain", "/moon", "Mozilla/5.0", "text/html, text/plain", false},
		{"frontend plain", "/moon", "Mozilla/5.0", "text/plain", true},
		{"frontend json", "/server/healthz", "curl/8", "application/json", false},
		{"frontend curl stays html for the renderer", "/moon", "curl/8", "*/*", false},
	}
	for _, c := range cases {
		req := httptest.NewRequest("GET", c.path, nil)
		req.Header.Set("User-Agent", c.ua)
		req.Header.Set("Accept", c.accept)
		if got := shouldRespondText(req); got != c.want {
			t.Errorf("%s: got %v want %v", c.name, got, c.want)
		}
	}
}

// TestTextFromData covers the AI.md PART 14 Text Serializer mapping.
func TestTextFromData(t *testing.T) {
	items := func(n int) string { return strconv.Itoa(n) + " items found" }
	type stats struct {
		Requests int `json:"requests_24h"`
	}
	type model struct {
		Name  string   `json:"name"`
		Stats stats    `json:"stats"`
		Tags  []string `json:"tags"`
	}
	cases := []struct {
		name string
		data interface{}
		want string
	}{
		{"object in field order", model{"x", stats{5}, []string{"a", "b"}}, "OK: ok\nname: x\nstats.requests_24h: 5\ntags: a, b\n"},
		{"message header", map[string]interface{}{"message": "saved", "id": "7"}, "OK: saved\nid: 7\n"},
		{"scalar list", []string{"a", "b"}, "OK: 2 items found\na\nb\n"},
		{"object list", []map[string]interface{}{{"a": 1}, {"a": 2}}, "OK: 2 items found\na: 1\n\na: 2\n"},
		{"bare scalar", "value", "value\n"},
	}
	for _, tc := range cases {
		if got := TextFromData(tc.data, "ok", items); got != tc.want {
			t.Errorf("%s: got %q want %q", tc.name, got, tc.want)
		}
	}
}
