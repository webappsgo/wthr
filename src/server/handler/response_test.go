package handler

import (
	"encoding/json"
	"html/template"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

// newTestContext builds a gin test context/recorder pair with a real
// http.Request so header/query/path based negotiation logic behaves the
// same as it would in production (gin.CreateTestContext alone leaves
// c.Request nil until assigned).
func newTestContext(method, target string) (*gin.Context, *httptest.ResponseRecorder) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(method, target, nil)
	return c, w
}

// TestRespondError covers the JSON envelope, status propagation, optional
// details merging, and the .txt/Accept:text/plain negotiation branch.
func TestRespondError(t *testing.T) {
	t.Run("JSON body matches ErrorResponse shape", func(t *testing.T) {
		c, w := newTestContext(http.MethodGet, "/api/v1/thing")
		RespondError(c, http.StatusBadRequest, ErrInvalidInput, "bad field")

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
		c, w := newTestContext(http.MethodGet, "/api/v1/thing")
		RespondError(c, http.StatusUnprocessableEntity, ErrValidationFailed, "invalid", map[string]interface{}{"field": "email"})

		var resp ErrorResponse
		if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if resp.Details == nil || resp.Details["field"] != "email" {
			t.Errorf("Details = %v, want map with field=email", resp.Details)
		}
	})

	t.Run(".txt extension forces plain text with code prefix", func(t *testing.T) {
		c, w := newTestContext(http.MethodGet, "/api/v1/thing.txt")
		RespondError(c, http.StatusNotFound, ErrNotFound, "missing")

		ct := w.Header().Get("Content-Type")
		if !strings.Contains(ct, "text/plain") && ct != "" {
			// gin's c.String sets text/plain; charset=utf-8 by default
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
		c, w := newTestContext(http.MethodGet, "/api/v1/thing")
		c.Request.Header.Set("Accept", "text/plain")
		RespondError(c, http.StatusInternalServerError, ErrInternal, "boom")

		if !strings.Contains(w.Body.String(), "boom") {
			t.Errorf("body = %q, want it to contain %q", w.Body.String(), "boom")
		}
	})
}

// TestRespondSuccess covers the {"ok":true,...} envelope and the optional
// data-merging behavior, plus the text-negotiation branch.
func TestRespondSuccess(t *testing.T) {
	t.Run("basic success envelope", func(t *testing.T) {
		c, w := newTestContext(http.MethodPost, "/api/v1/thing")
		RespondSuccess(c, "did the thing")

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
		c, w := newTestContext(http.MethodPost, "/api/v1/thing")
		RespondSuccess(c, "done", map[string]interface{}{"count": float64(3)})

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
		c, w := newTestContext(http.MethodPost, "/api/v1/thing.txt")
		RespondSuccess(c, "saved")
		if strings.TrimSpace(w.Body.String()) != "saved" {
			t.Errorf("body = %q, want %q", w.Body.String(), "saved")
		}
	})
}

// TestRespondCreated covers the 201 envelope including the ID field, which
// distinguishes it from RespondSuccess.
func TestRespondCreated(t *testing.T) {
	c, w := newTestContext(http.MethodPost, "/api/v1/thing")
	RespondCreated(c, "created", "item_123", map[string]interface{}{"name": "x"})

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
		c, w := newTestContext(http.MethodGet, "/api/v1/thing")
		RespondData(c, map[string]interface{}{"a": float64(1)})

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
		c, w := newTestContext(http.MethodGet, "/api/v1/thing")
		RespondData(c, nil)
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
			c, w := newTestContext(http.MethodGet, "/api/v1/things")
			RespondPaginated(c, []int{}, tt.page, tt.limit, tt.total)

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
		c, _ := newTestContext(http.MethodGet, "/api/v1/things")
		RespondPaginated(c, []int{}, 1, 0, 5)
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
			c, _ := newTestContext(http.MethodGet, tt.target)
			if tt.accept != "" {
				c.Request.Header.Set("Accept", tt.accept)
			}
			if got := shouldRespondText(c); got != tt.want {
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
			c, _ := newTestContext(http.MethodGet, tt.target)
			if tt.accept != "" {
				c.Request.Header.Set("Accept", tt.accept)
			}
			if tt.userAgent != "" {
				c.Request.Header.Set("User-Agent", tt.userAgent)
			}
			if got := WantsJSON(c); got != tt.want {
				t.Errorf("WantsJSON(target=%s, Accept=%q, UA=%q) = %v, want %v", tt.target, tt.accept, tt.userAgent, got, tt.want)
			}
		})
	}
}

// TestNegotiateResponse covers the three-way branch (text/JSON/HTML). The
// HTML branch is exercised with a registered dummy template so it does not
// panic on a missing template lookup.
func TestNegotiateResponse(t *testing.T) {
	t.Run("text branch", func(t *testing.T) {
		c, w := newTestContext(http.MethodGet, "/page.txt")
		NegotiateResponse(c, "page.tmpl", gin.H{"X": 1})
		if !strings.Contains(w.Body.String(), "\"X\"") {
			t.Errorf("body = %q, want pretty JSON containing X", w.Body.String())
		}
	})

	t.Run("json branch", func(t *testing.T) {
		c, w := newTestContext(http.MethodGet, "/api/v1/page")
		NegotiateResponse(c, "page.tmpl", gin.H{"X": 1})
		var got map[string]interface{}
		if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
			t.Fatalf("unmarshal: %v; body=%s", err, w.Body.String())
		}
		if got["X"] != float64(1) {
			t.Errorf("got = %v, want X=1", got)
		}
	})

	t.Run("html branch renders registered template", func(t *testing.T) {
		gin.SetMode(gin.TestMode)
		engine := gin.New()
		tmpl := template.Must(template.New("page.tmpl").Parse("<p>{{.X}}</p>"))
		engine.SetHTMLTemplate(tmpl)
		engine.GET("/page", func(c *gin.Context) {
			NegotiateResponse(c, "page.tmpl", gin.H{"X": 1})
		})
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/page", nil)
		engine.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200", w.Code)
		}
		if !strings.Contains(w.Body.String(), "<p>1</p>") {
			t.Errorf("body = %q, want rendered template containing <p>1</p>", w.Body.String())
		}
	})
}

// TestNegotiateErrorResponse mirrors TestNegotiateResponse but for the error
// path, and additionally verifies the HTML branch injects Error/ErrorCode
// into the template data (including when the caller passes nil data).
func TestNegotiateErrorResponse(t *testing.T) {
	t.Run("text branch delegates to RespondError", func(t *testing.T) {
		c, w := newTestContext(http.MethodGet, "/page.txt")
		NegotiateErrorResponse(c, http.StatusBadRequest, "page.tmpl", ErrInvalidInput, "bad", nil)
		if !strings.Contains(w.Body.String(), "bad") {
			t.Errorf("body = %q, want it to contain %q", w.Body.String(), "bad")
		}
	})

	t.Run("json branch delegates to RespondError", func(t *testing.T) {
		c, w := newTestContext(http.MethodGet, "/api/v1/page")
		NegotiateErrorResponse(c, http.StatusNotFound, "page.tmpl", ErrNotFound, "missing", nil)
		var resp ErrorResponse
		if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if resp.Error != ErrNotFound || resp.Message != "missing" {
			t.Errorf("resp = %+v, want Error=%s Message=missing", resp, ErrNotFound)
		}
	})

	t.Run("html branch with nil data does not panic and injects error fields", func(t *testing.T) {
		gin.SetMode(gin.TestMode)
		engine := gin.New()
		tmpl := template.Must(template.New("err.tmpl").Parse("<p>{{.ErrorCode}}: {{.Error}}</p>"))
		engine.SetHTMLTemplate(tmpl)
		engine.GET("/page", func(c *gin.Context) {
			NegotiateErrorResponse(c, http.StatusForbidden, "err.tmpl", ErrForbidden, "nope", nil)
		})
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/page", nil)
		engine.ServeHTTP(w, req)

		if w.Code != http.StatusForbidden {
			t.Fatalf("status = %d, want 403", w.Code)
		}
		if !strings.Contains(w.Body.String(), "FORBIDDEN: nope") {
			t.Errorf("body = %q, want it to contain injected Error/ErrorCode", w.Body.String())
		}
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
		call       func(c *gin.Context)
		wantStatus int
		wantCode   string
	}{
		{"BadRequest", func(c *gin.Context) { BadRequest(c, "m") }, http.StatusBadRequest, ErrBadRequest},
		{"InvalidInput", func(c *gin.Context) { InvalidInput(c, "m") }, http.StatusBadRequest, ErrInvalidInput},
		{"NotFound", func(c *gin.Context) { NotFound(c, "m") }, http.StatusNotFound, ErrNotFound},
		{"Unauthorized", func(c *gin.Context) { Unauthorized(c, "m") }, http.StatusUnauthorized, ErrUnauthorized},
		{"Forbidden", func(c *gin.Context) { Forbidden(c, "m") }, http.StatusForbidden, ErrForbidden},
		{"Conflict", func(c *gin.Context) { Conflict(c, "m") }, http.StatusConflict, ErrConflict},
		{"InternalError", func(c *gin.Context) { InternalError(c, "m") }, http.StatusInternalServerError, ErrInternal},
		{"ValidationFailed", func(c *gin.Context) { ValidationFailed(c, "m", nil) }, http.StatusUnprocessableEntity, ErrValidationFailed},
		{"RateLimited", func(c *gin.Context) { RateLimited(c, "m") }, http.StatusTooManyRequests, ErrRateLimited},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, w := newTestContext(http.MethodGet, "/api/v1/thing")
			tt.call(c)

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
