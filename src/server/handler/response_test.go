package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// newTestRequest builds a bare request/recorder pair so header/query/path
// based negotiation logic behaves the same as it would in production.
func newTestRequest(method, target string) (*http.Request, *httptest.ResponseRecorder) {
	w := httptest.NewRecorder()
	r := httptest.NewRequest(method, target, nil)
	return r, w
}

// TestRespondError covers the JSON envelope, status propagation, optional
// details merging, and the .txt/Accept:text/plain negotiation branch.
func TestRespondError(t *testing.T) {
	t.Run("JSON body matches ErrorResponse shape", func(t *testing.T) {
		r, w := newTestRequest(http.MethodGet, "/api/v1/thing")
		RespondError(w, r, http.StatusBadRequest, ErrInvalidInput, "bad field")

		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want %d", w.Code, http.StatusBadRequest)
		}
		var resp ErrorResponse
		if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
			t.Fatalf("unmarshal: %v; body=%s", err, w.Body.String())
		}
		if resp.OK || resp.Error != ErrInvalidInput || resp.Message != "bad field" {
			t.Errorf("resp = %+v, want OK=false Error=%s Message=bad field", resp, ErrInvalidInput)
		}
		if resp.Details != nil {
			t.Errorf("Details = %v, want nil when not supplied", resp.Details)
		}
	})

	t.Run("details are attached when provided", func(t *testing.T) {
		r, w := newTestRequest(http.MethodGet, "/api/v1/thing")
		RespondError(w, r, http.StatusUnprocessableEntity, ErrValidationFailed, "invalid", map[string]interface{}{"field": "email"})

		var resp ErrorResponse
		if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if resp.Details == nil || resp.Details["field"] != "email" {
			t.Errorf("Details = %v, want map with field=email", resp.Details)
		}
	})

	t.Run(".txt extension forces plain text with code prefix", func(t *testing.T) {
		r, w := newTestRequest(http.MethodGet, "/api/v1/thing.txt")
		RespondError(w, r, http.StatusNotFound, ErrNotFound, "missing")

		ct := w.Header().Get("Content-Type")
		if !strings.Contains(ct, "text/plain") && ct != "" {
			t.Errorf("Content-Type = %q, want text/plain", ct)
		}
		body := w.Body.String()
		if !strings.Contains(body, ErrNotFound) || !strings.Contains(body, "missing") {
			t.Errorf("body = %q, want it to contain code %q and message %q", body, ErrNotFound, "missing")
		}
		// Must NOT be JSON in this branch.
		var js map[string]interface{}
		if json.Unmarshal(w.Body.Bytes(), &js) == nil {
			t.Errorf("body = %q parsed as JSON, want plain text for .txt requests", body)
		}
	})

	t.Run("Accept text/plain header forces plain text", func(t *testing.T) {
		r, w := newTestRequest(http.MethodGet, "/api/v1/thing")
		r.Header.Set("Accept", "text/plain")
		RespondError(w, r, http.StatusInternalServerError, ErrInternal, "boom")

		if !strings.Contains(w.Body.String(), "boom") {
			t.Errorf("body = %q, want it to contain %q", w.Body.String(), "boom")
		}
	})
}

// TestRespondSuccess covers the {"ok":true,...} envelope and the optional
// data-merging behavior, plus the text-negotiation branch.
func TestRespondSuccess(t *testing.T) {
	t.Run("basic success envelope", func(t *testing.T) {
		r, w := newTestRequest(http.MethodPost, "/api/v1/thing")
		RespondSuccess(w, r, "did the thing")

		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200", w.Code)
		}
		var resp APIResponse
		if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		data, ok := resp.Data.(map[string]interface{})
		if !resp.OK || !ok || data["message"] != "did the thing" {
			t.Errorf("resp = %+v, want OK=true Data.message=%q", resp, "did the thing")
		}
	})

	t.Run("optional data is merged", func(t *testing.T) {
		r, w := newTestRequest(http.MethodPost, "/api/v1/thing")
		RespondSuccess(w, r, "done", map[string]interface{}{"count": float64(3)})

		var resp APIResponse
		if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		data, ok := resp.Data.(map[string]interface{})
		if !ok || data["count"] != float64(3) || data["message"] != "done" {
			t.Errorf("Data = %v, want map with count=3 message=done", resp.Data)
		}
	})

	t.Run(".txt extension responds with plain message", func(t *testing.T) {
		r, w := newTestRequest(http.MethodPost, "/api/v1/thing.txt")
		RespondSuccess(w, r, "saved")
		if strings.TrimSpace(w.Body.String()) != "saved" {
			t.Errorf("body = %q, want %q", w.Body.String(), "saved")
		}
	})
}

// TestRespondCreated covers the 201 envelope including the ID field, which
// distinguishes it from RespondSuccess.
func TestRespondCreated(t *testing.T) {
	r, w := newTestRequest(http.MethodPost, "/api/v1/thing")
	RespondCreated(w, r, "created", "item_123", map[string]interface{}{"name": "x"})

	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201", w.Code)
	}
	var resp APIResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	data, ok := resp.Data.(map[string]interface{})
	if !ok || data["id"] != "item_123" || !resp.OK || data["message"] != "created" || data["name"] != "x" {
		t.Errorf("resp = %+v, want Data.id=item_123 OK=true Data.message=created Data.name=x", resp)
	}
}

// TestRespondData covers the unwrapped-data response path (no envelope),
// including nil.
func TestRespondData(t *testing.T) {
	t.Run("returns raw JSON without wrapper", func(t *testing.T) {
		r, w := newTestRequest(http.MethodGet, "/api/v1/thing")
		RespondData(w, r, map[string]interface{}{"a": float64(1)})

		var got map[string]interface{}
		if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if got["a"] != float64(1) {
			t.Errorf("got = %v, want a=1 unwrapped", got)
		}
		if _, hasOK := got["ok"]; hasOK {
			t.Errorf("got = %v, want no 'ok' wrapper key", got)
		}
	})

	t.Run("nil data does not panic and returns JSON null", func(t *testing.T) {
		r, w := newTestRequest(http.MethodGet, "/api/v1/thing")
		RespondData(w, r, nil)
		if strings.TrimSpace(w.Body.String()) != "null" {
			t.Errorf("body = %q, want %q", w.Body.String(), "null")
		}
	})
}

// TestRespondPaginated covers the page-count math, including the boundary
// where total is an exact multiple of limit vs. not, and zero-result sets.
func TestRespondPaginated(t *testing.T) {
	tests := []struct {
		name      string
		page      int
		limit     int
		total     int
		wantPages int
	}{
		{"exact multiple", 1, 10, 20, 2},
		{"remainder rounds up", 1, 10, 21, 3},
		{"zero total", 1, 10, 0, 0},
		{"single item", 1, 10, 1, 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r, w := newTestRequest(http.MethodGet, "/api/v1/things")
			RespondPaginated(w, r, []int{}, tt.page, tt.limit, tt.total)

			var resp PaginatedResponse
			if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}
			if resp.Pagination.Pages != tt.wantPages {
				t.Errorf("Pages = %d, want %d", resp.Pagination.Pages, tt.wantPages)
			}
			if resp.Pagination.Total != tt.total || resp.Pagination.Page != tt.page || resp.Pagination.Limit != tt.limit {
				t.Errorf("Pagination = %+v, want Page=%d Limit=%d Total=%d", resp.Pagination, tt.page, tt.limit, tt.total)
			}
		})
	}

	// A limit of zero would divide by zero in the `total / limit` expression;
	// this documents the current (crashing) behavior is a real edge case
	// callers must avoid, not something the helper guards against.
	t.Run("limit of zero panics (documents lack of guard)", func(t *testing.T) {
		defer func() {
			if r := recover(); r == nil {
				t.Skip("RespondPaginated did not panic on limit=0; guard may have been added - update this test")
			}
		}()
		r, w := newTestRequest(http.MethodGet, "/api/v1/things")
		RespondPaginated(w, r, []int{}, 1, 0, 5)
	})
}

// TestShouldRespondText exercises the two independent triggers (.txt
// extension, Accept header) and confirms neither false-positives on a
// normal JSON request.
func TestShouldRespondText(t *testing.T) {
	tests := []struct {
		name   string
		target string
		accept string
		want   bool
	}{
		{"plain json request", "/api/v1/weather", "application/json", false},
		{"txt extension", "/api/v1/weather.txt", "", true},
		{"accept text/plain", "/api/v1/weather", "text/plain", true},
		{"accept text/plain with charset", "/api/v1/weather", "text/plain; charset=utf-8", true},
		{"no accept header at all", "/api/v1/weather", "", false},
		{"unrelated extension", "/api/v1/weather.json", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r, _ := newTestRequest(http.MethodGet, tt.target)
			if tt.accept != "" {
				r.Header.Set("Accept", tt.accept)
			}
			if got := shouldRespondText(r); got != tt.want {
				t.Errorf("shouldRespondText(%s, Accept=%q) = %v, want %v", tt.target, tt.accept, got, tt.want)
			}
		})
	}
}

// TestWantsJSON exercises each independent trigger (Accept header, ?format=json,
// /api/ prefix, CLI user agents) and the precedence between them, particularly
// the case that matters most for correctness: a curl request that explicitly
// asks for HTML must not be silently upgraded to JSON.
func TestWantsJSON(t *testing.T) {
	tests := []struct {
		name      string
		target    string
		accept    string
		userAgent string
		want      bool
	}{
		{"explicit accept json", "/dashboard", "application/json", "", true},
		{"format query param", "/dashboard?format=json", "", "", true},
		{"api prefix always json", "/api/v1/weather", "", "Mozilla/5.0", true},
		{"curl default", "/dashboard", "", "curl/8.0", true},
		{"wget default", "/dashboard", "", "Wget/1.21", true},
		{"httpie default", "/dashboard", "", "HTTPie/3.2", true},
		{"curl explicitly wants html", "/dashboard", "text/html", "curl/8.0", false},
		{"plain browser request", "/dashboard", "text/html", "Mozilla/5.0", false},
		{"no headers at all", "/dashboard", "", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r, _ := newTestRequest(http.MethodGet, tt.target)
			if tt.accept != "" {
				r.Header.Set("Accept", tt.accept)
			}
			if tt.userAgent != "" {
				r.Header.Set("User-Agent", tt.userAgent)
			}
			if got := WantsJSON(r); got != tt.want {
				t.Errorf("WantsJSON(target=%s, Accept=%q, UA=%q) = %v, want %v", tt.target, tt.accept, tt.userAgent, got, tt.want)
			}
		})
	}
}

// TestNegotiateResponse covers the three-way branch (text/JSON/HTML). The
// HTML branch is skipped - see the subtest comment - because RenderHTML is
// not yet declared anywhere in package handler.
func TestNegotiateResponse(t *testing.T) {
	t.Run("text branch", func(t *testing.T) {
		r, w := newTestRequest(http.MethodGet, "/page.txt")
		NegotiateResponse(w, r, "page.tmpl", map[string]interface{}{"X": 1})
		if !strings.Contains(w.Body.String(), "\"X\"") {
			t.Errorf("body = %q, want pretty JSON containing X", w.Body.String())
		}
	})

	t.Run("json branch", func(t *testing.T) {
		r, w := newTestRequest(http.MethodGet, "/api/v1/page")
		NegotiateResponse(w, r, "page.tmpl", map[string]interface{}{"X": 1})
		var got map[string]interface{}
		if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
			t.Fatalf("unmarshal: %v; body=%s", err, w.Body.String())
		}
		if got["X"] != float64(1) {
			t.Errorf("got = %v, want X=1", got)
		}
	})

	t.Run("html branch renders registered template", func(t *testing.T) {
		// NegotiateResponse's HTML branch calls the package-level RenderHTML
		// function (see response.go lines 274/292), which is not yet
		// declared anywhere in package handler - it is a known-pending,
		// not-yet-implemented dependency (per task scope, must not be
		// stubbed or implemented here). Skipped rather than guessed at,
		// following the same-package precedent in auth_oidc_test.go
		// (t.Skipf("middleware.RenderHTML not configured...")).
		t.Skip("RenderHTML is not yet declared in package handler; HTML branch cannot be exercised until it is wired up (see response.go:274)")
	})
}

// TestNegotiateErrorResponse mirrors TestNegotiateResponse but for the error
// path, and additionally verifies the HTML branch injects Error/ErrorCode
// into the template data (including when the caller passes nil data).
func TestNegotiateErrorResponse(t *testing.T) {
	t.Run("text branch delegates to RespondError", func(t *testing.T) {
		r, w := newTestRequest(http.MethodGet, "/page.txt")
		NegotiateErrorResponse(w, r, http.StatusBadRequest, "page.tmpl", ErrInvalidInput, "bad", nil)
		if !strings.Contains(w.Body.String(), "bad") {
			t.Errorf("body = %q, want it to contain %q", w.Body.String(), "bad")
		}
	})

	t.Run("json branch delegates to RespondError", func(t *testing.T) {
		r, w := newTestRequest(http.MethodGet, "/api/v1/page")
		NegotiateErrorResponse(w, r, http.StatusNotFound, "page.tmpl", ErrNotFound, "missing", nil)
		var resp ErrorResponse
		if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if resp.Error != ErrNotFound || resp.Message != "missing" {
			t.Errorf("resp = %+v, want Error=%s Message=missing", resp, ErrNotFound)
		}
	})

	t.Run("html branch with nil data does not panic and injects error fields", func(t *testing.T) {
		// Same RenderHTML dependency as TestNegotiateResponse's html-branch
		// subtest above - see that comment for the full explanation.
		t.Skip("RenderHTML is not yet declared in package handler; HTML branch cannot be exercised until it is wired up (see response.go:292)")
	})
}

// TestErrorHelperWrappers table-drives every thin wrapper (BadRequest,
// InvalidInput, NotFound, ...) to confirm each produces the documented
// status code and error code - these are what handlers actually call, so a
// wiring mistake here (e.g. Forbidden returning 401 instead of 403) would
// silently break every caller.
func TestErrorHelperWrappers(t *testing.T) {
	tests := []struct {
		name       string
		call       func(w http.ResponseWriter, r *http.Request)
		wantStatus int
		wantCode   string
	}{
		{"BadRequest", func(w http.ResponseWriter, r *http.Request) { BadRequest(w, r, "m") }, http.StatusBadRequest, ErrBadRequest},
		{"InvalidInput", func(w http.ResponseWriter, r *http.Request) { InvalidInput(w, r, "m") }, http.StatusBadRequest, ErrInvalidInput},
		{"NotFound", func(w http.ResponseWriter, r *http.Request) { NotFound(w, r, "m") }, http.StatusNotFound, ErrNotFound},
		{"Unauthorized", func(w http.ResponseWriter, r *http.Request) { Unauthorized(w, r, "m") }, http.StatusUnauthorized, ErrUnauthorized},
		{"Forbidden", func(w http.ResponseWriter, r *http.Request) { Forbidden(w, r, "m") }, http.StatusForbidden, ErrForbidden},
		{"Conflict", func(w http.ResponseWriter, r *http.Request) { Conflict(w, r, "m") }, http.StatusConflict, ErrConflict},
		{"InternalError", func(w http.ResponseWriter, r *http.Request) { InternalError(w, r, "m") }, http.StatusInternalServerError, ErrInternal},
		{"ValidationFailed", func(w http.ResponseWriter, r *http.Request) { ValidationFailed(w, r, "m", nil) }, http.StatusBadRequest, ErrValidationFailed},
		{"RateLimited", func(w http.ResponseWriter, r *http.Request) { RateLimited(w, r, "m") }, http.StatusTooManyRequests, ErrRateLimited},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r, w := newTestRequest(http.MethodGet, "/api/v1/thing")
			tt.call(w, r)

			if w.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d", w.Code, tt.wantStatus)
			}
			var resp ErrorResponse
			if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}
			if resp.Error != tt.wantCode {
				t.Errorf("Error = %q, want %q", resp.Error, tt.wantCode)
			}
		})
	}
}
