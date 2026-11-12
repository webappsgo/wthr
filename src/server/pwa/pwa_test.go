package pwa

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/webappsgo/wthr/src/server/buildstamp"
)

func TestServiceWorkerRendersStamp(t *testing.T) {
	buildstamp.Set("2.0.0", "abc1234")
	w := httptest.NewRecorder()
	ServiceWorker(w, httptest.NewRequest(http.MethodGet, "/sw.js", nil))

	body := w.Body.String()
	if strings.Contains(body, "{asset_stamp}") || strings.Contains(body, "{app_name}") {
		t.Fatal("placeholders left in /sw.js")
	}
	if !strings.Contains(body, "'2.0.0-abc1234'") || !strings.Contains(body, "wthr-cache-") {
		t.Fatalf("stamp or cache name missing: %s", body[:200])
	}
	if w.Header().Get("Cache-Control") != "no-cache" || w.Header().Get("ETag") != `"2.0.0-abc1234"` {
		t.Fatalf("headers = %v", w.Header())
	}
}

func TestNotModifiedOnMatchingETag(t *testing.T) {
	buildstamp.Set("2.0.0", "abc1234")
	r := httptest.NewRequest(http.MethodGet, "/manifest.json", nil)
	r.Header.Set("If-None-Match", buildstamp.ETag())
	w := httptest.NewRecorder()
	Manifest(w, r)
	if w.Code != http.StatusNotModified {
		t.Fatalf("status = %d, want 304", w.Code)
	}
}

func TestManifestIsValidJSON(t *testing.T) {
	var m map[string]interface{}
	if err := json.Unmarshal(manifestSource, &m); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"name", "short_name", "start_url", "display", "icons"} {
		if m[key] == nil {
			t.Errorf("manifest missing %s", key)
		}
	}
}

func TestStaticCache(t *testing.T) {
	buildstamp.Set("2.0.0", "abc1234")
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {})

	w := httptest.NewRecorder()
	StaticCache(next).ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/x.css?v=2.0.0-abc1234", nil))
	if got := w.Header().Get("Cache-Control"); !strings.Contains(got, "immutable") {
		t.Fatalf("matching stamp = %q", got)
	}

	w = httptest.NewRecorder()
	StaticCache(next).ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/x.css?v=old", nil))
	if got := w.Header().Get("Cache-Control"); got != "no-cache" || w.Header().Get("ETag") == "" {
		t.Fatalf("mismatched stamp = %q", got)
	}
}
