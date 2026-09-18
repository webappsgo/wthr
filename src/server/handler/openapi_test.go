package handler

import (
	"net/http"
	"net/http/httptest"
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

	r := httptest.NewRequest(http.MethodGet, "/openapi/index.html", nil)
	w := httptest.NewRecorder()

	handlerFunc(w, r)

	if w.Code == 0 {
		t.Fatalf("expected a response status to be written, got 0")
	}
}
