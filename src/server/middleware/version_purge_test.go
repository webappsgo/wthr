package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/webappsgo/wthr/src/server/buildstamp"
)

func purgeHandler(contentType string, status int) http.Handler {
	return VersionPurge()(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", contentType)
		w.WriteHeader(status)
		_, _ = w.Write([]byte("<html></html>"))
	}))
}

func doPurge(h http.Handler, method string, headers map[string]string, cookie string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, "/", nil)
	for k, v := range headers {
		r.Header.Set(k, v)
	}
	if cookie != "" {
		r.AddCookie(&http.Cookie{Name: buildCookieName, Value: cookie})
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

func TestVersionPurge(t *testing.T) {
	stamp := buildstamp.Stamp()
	html := purgeHandler("text/html; charset=utf-8", http.StatusOK)

	w := doPurge(html, http.MethodGet, nil, "")
	if w.Header().Get("Clear-Site-Data") != "" {
		t.Error("a first visit never purges")
	}
	cookies := w.Result().Cookies()
	if len(cookies) != 1 || cookies[0].Name != buildCookieName || cookies[0].Value != stamp || cookies[0].SameSite != http.SameSiteLaxMode || cookies[0].MaxAge != 31536000 {
		t.Fatalf("build cookie = %+v", cookies)
	}

	if w = doPurge(html, http.MethodGet, nil, stamp); w.Header().Get("Clear-Site-Data") != "" {
		t.Error("a matching stamp does not purge")
	}

	w = doPurge(html, http.MethodGet, nil, "old-build")
	if got := w.Header().Get("Clear-Site-Data"); got != `"cache", "storage"` {
		t.Errorf("a stale stamp purges cache and storage only, got %q", got)
	}
	if c := w.Result().Cookies(); len(c) != 1 || c[0].Value != stamp {
		t.Error("the purge response re-sets the cookie so it cannot repeat")
	}

	// never for iframes, fetch calls, HEAD, non-HTML, redirects or errors
	for name, tc := range map[string]struct {
		h       http.Handler
		method  string
		headers map[string]string
	}{
		"iframe":   {html, http.MethodGet, map[string]string{"Sec-Fetch-Dest": "iframe", "Sec-Fetch-Mode": "navigate"}},
		"fetch":    {html, http.MethodGet, map[string]string{"Sec-Fetch-Dest": "empty", "Sec-Fetch-Mode": "cors"}},
		"head":     {html, http.MethodHead, nil},
		"json":     {purgeHandler("application/json", http.StatusOK), http.MethodGet, nil},
		"redirect": {purgeHandler("text/html", http.StatusFound), http.MethodGet, nil},
		"error":    {purgeHandler("text/html", http.StatusInternalServerError), http.MethodGet, nil},
	} {
		w := doPurge(tc.h, tc.method, tc.headers, "old-build")
		if w.Header().Get("Clear-Site-Data") != "" || len(w.Result().Cookies()) != 0 {
			t.Errorf("%s: no purge header or cookie expected, got %v / %v", name, w.Header(), w.Result().Cookies())
		}
	}

	// document navigations that send Sec-Fetch headers still purge
	w = doPurge(html, http.MethodGet, map[string]string{"Sec-Fetch-Dest": "document", "Sec-Fetch-Mode": "navigate"}, "old-build")
	if w.Header().Get("Clear-Site-Data") == "" {
		t.Error("a document navigation purges")
	}

	// a handler that never calls WriteHeader is treated as 200
	implicit := VersionPurge()(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte("x"))
	}))
	if w = doPurge(implicit, http.MethodGet, nil, "old-build"); w.Header().Get("Clear-Site-Data") == "" {
		t.Error("an implicit 200 purges")
	}
	sniffed := VersionPurge()(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte("<html><body>x</body></html>")) }))
	if w = doPurge(sniffed, http.MethodGet, nil, "old-build"); w.Header().Get("Clear-Site-Data") == "" {
		t.Error("sniffed html purges")
	}
	secure := doPurge(html, http.MethodGet, map[string]string{"X-Forwarded-Proto": "https"}, "")
	if c := secure.Result().Cookies(); len(c) != 1 || !c[0].Secure {
		t.Error("the cookie is Secure behind https")
	}
}
