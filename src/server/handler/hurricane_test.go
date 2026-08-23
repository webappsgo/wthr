package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
)

// TestNewHurricaneHandler verifies the constructor wires the service field
// as passed, including the nil case.
func TestNewHurricaneHandler(t *testing.T) {
	h := NewHurricaneHandler(nil)
	if h == nil {
		t.Fatal("expected non-nil handler")
	}
	if h.hurricaneService != nil {
		t.Errorf("expected hurricaneService to be nil (as passed), got %v", h.hurricaneService)
	}
}

// TestHurricaneHandler_ListActiveStorms_NilService verifies calling
// ListActiveStorms with an uninitialized hurricane service returns an error
// instead of panicking.
func TestHurricaneHandler_ListActiveStorms_NilService(t *testing.T) {
	h := &HurricaneHandler{}

	storms, err := h.ListActiveStorms()
	if err == nil {
		t.Fatal("expected an error for uninitialized hurricane service")
	}
	if storms != nil {
		t.Errorf("expected nil storms on error, got %v", storms)
	}
}

// TestHurricaneHandler_ListActiveStorms_NilHandler verifies calling
// ListActiveStorms on a nil *HurricaneHandler is handled gracefully via the
// explicit h == nil guard, rather than panicking on a nil pointer deref.
func TestHurricaneHandler_ListActiveStorms_NilHandler(t *testing.T) {
	var h *HurricaneHandler

	storms, err := h.ListActiveStorms()
	if err == nil {
		t.Fatal("expected an error for a nil handler")
	}
	if storms != nil {
		t.Errorf("expected nil storms on error, got %v", storms)
	}
}

// TestHurricaneHandler_HandleHurricaneByIDAPI_MissingID verifies an empty
// :id path parameter is rejected with 400 before the hurricane service is
// ever consulted (service is left nil here — a call would panic).
func TestHurricaneHandler_HandleHurricaneByIDAPI_MissingID(t *testing.T) {
	h := &HurricaneHandler{}
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/hurricanes/", nil)

	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", "")
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

	h.HandleHurricaneByIDAPI(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d: %s", w.Code, w.Body.String())
	}
}

// TestFormatInt verifies zero maps to "N/A" and non-zero values are
// formatted via formatIntToStr, including negatives.
func TestFormatInt(t *testing.T) {
	cases := map[int]string{
		0:    "N/A",
		5:    "5",
		42:   "42",
		-13:  "-13",
		1000: "1000",
	}
	for input, want := range cases {
		if got := formatInt(input); got != want {
			t.Errorf("formatInt(%d) = %q, want %q", input, got, want)
		}
	}
}

// TestFormatFloat verifies zero maps to "N/A" and non-zero values are
// formatted via formatFloatToStr with two decimal places.
func TestFormatFloat(t *testing.T) {
	cases := map[float64]string{
		0.0:   "N/A",
		1.5:   "1.50",
		29.92: "29.92",
		-3.25: "-3.25",
	}
	for input, want := range cases {
		if got := formatFloat(input); got != want {
			t.Errorf("formatFloat(%v) = %q, want %q", input, got, want)
		}
	}
}

// TestFormatIntToStr covers single-digit, multi-digit, and negative values.
func TestFormatIntToStr(t *testing.T) {
	cases := map[int]string{
		0:    "0",
		7:    "7",
		42:   "42",
		999:  "999",
		-42:  "-42",
		1234: "1234",
	}
	for input, want := range cases {
		if got := formatIntToStr(input); got != want {
			t.Errorf("formatIntToStr(%d) = %q, want %q", input, got, want)
		}
	}
}

// TestFormatFloatToStr covers positive, negative, and whole-number values.
func TestFormatFloatToStr(t *testing.T) {
	cases := map[float64]string{
		1.5:   "1.50",
		29.92: "29.92",
		-3.25: "-3.25",
		10.0:  "10.00",
	}
	for input, want := range cases {
		if got := formatFloatToStr(input); got != want {
			t.Errorf("formatFloatToStr(%v) = %q, want %q", input, got, want)
		}
	}
}
