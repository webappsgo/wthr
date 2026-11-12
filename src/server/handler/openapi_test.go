package handler

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestGetSwaggerUIAuto verifies the handler is constructible and, when
// invoked, responds without panicking (no DB/service dependency exists to
// mock here — httpSwagger.Handler wraps swaggerFiles.Handler directly).
func TestGetSwaggerUIAuto(t *testing.T) {
	handlerFunc := GetSwaggerUIAuto()
	if handlerFunc == nil {
		t.Fatal("GetSwaggerUIAuto() returned a nil handler")
	}

	r := httptest.NewRequest(http.MethodGet, "/server/docs/swagger", nil)
	w := httptest.NewRecorder()

	handlerFunc(w, r)

	if w.Code == 0 {
		t.Fatalf("expected a response status to be written, got 0")
	}
}

// TestSwaggerUIServesAtDocsPath verifies the Swagger UI answers 200 with HTML at the AI.md
// PART 14 path /server/docs/swagger, not only at an index.html suffix.
func TestSwaggerUIServesAtDocsPath(t *testing.T) {
	for _, path := range []string{"/server/docs/swagger", "/server/docs/swagger/", "/server/docs/swagger/index.html"} {
		r := httptest.NewRequest(http.MethodGet, path, nil)
		w := httptest.NewRecorder()
		GetSwaggerUIAuto()(w, r)
		if w.Code != http.StatusOK {
			t.Errorf("GET %s = %d, want 200", path, w.Code)
		}
		if ct := w.Header().Get("Content-Type"); !strings.Contains(ct, "text/html") {
			t.Errorf("GET %s content type = %q, want text/html", path, ct)
		}
	}
}
