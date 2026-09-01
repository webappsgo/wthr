package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/webappsgo/wthr/src/config"
	models "github.com/webappsgo/wthr/src/server/model"
	"github.com/webappsgo/wthr/src/server/service"
)

// newI2PTestHandler wires an I2PAdminHandler against a fresh in-memory server
// DB and a temp data dir. I2P is opt-in and no provider exists in the test
// environment, so every path exercises the disabled or error branch rather
// than a real i2pd process or SAM bridge.
func newI2PTestHandler(t *testing.T) *I2PAdminHandler {
	t.Helper()
	serverDB := newTestServerDB(t)
	setGlobalTestDualDB(t, serverDB, serverDB)
	dataDir := t.TempDir()
	cfg := config.DefaultI2PConfig()
	i2pManager := service.NewI2PManager(context.Background(), &cfg)
	settingsModel := &models.SettingsModel{DB: serverDB}
	return NewI2PAdminHandler(i2pManager, settingsModel, dataDir)
}

func TestI2PAdminHandler_GetStatusDisabled(t *testing.T) {
	h := newI2PTestHandler(t)
	c, w := newTestContext(http.MethodGet, "/admin/config/i2p")
	h.GetStatus(w, c)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
	}

	var resp struct {
		Status map[string]interface{} `json:"status"`
		Config config.I2PConfig       `json:"config"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.Status == nil {
		t.Fatalf("missing status key: %s", w.Body.String())
	}

	tests := []struct {
		key  string
		want interface{}
	}{
		{"enabled", false},
		{"running", false},
		{"status", "disabled"},
		{"provider", "none"},
		{"address", ""},
	}
	for _, tt := range tests {
		t.Run("status "+tt.key, func(t *testing.T) {
			got, ok := resp.Status[tt.key]
			if !ok {
				t.Fatalf("status missing key %q: %+v", tt.key, resp.Status)
			}
			if got != tt.want {
				t.Fatalf("status[%q] = %v, want %v", tt.key, got, tt.want)
			}
		})
	}

	if resp.Config.Enabled {
		t.Fatalf("config.enabled = true, want false (I2P is opt-in)")
	}
}

func TestI2PAdminHandler_UpdateSettingsValidation(t *testing.T) {
	tests := []struct {
		name  string
		body  interface{}
		field string
	}{
		{
			name:  "virtual port out of range",
			body:  map[string]interface{}{"enabled": true, "virtual_port": 0},
			field: "virtual_port",
		},
		{
			name:  "inbound length above maximum",
			body:  map[string]interface{}{"enabled": true, "inbound_length": 9},
			field: "inbound_length",
		},
		{
			name:  "outbound quantity above maximum",
			body:  map[string]interface{}{"enabled": true, "outbound_quantity": 99},
			field: "outbound_quantity",
		},
		{
			name:  "unsupported signature type",
			body:  map[string]interface{}{"enabled": true, "signature_type": 3},
			field: "signature_type",
		},
		{
			name:  "bootstrap timeout below minimum",
			body:  map[string]interface{}{"enabled": true, "bootstrap_timeout": 5},
			field: "bootstrap_timeout",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newI2PTestHandler(t)
			c, w := newTestContextJSON(t, http.MethodPatch, "/admin/config/i2p", tt.body)
			h.UpdateSettings(w, c)
			if w.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400; body=%s", w.Code, w.Body.String())
			}

			var resp struct {
				OK      bool                   `json:"ok"`
				Error   string                 `json:"error"`
				Message string                 `json:"message"`
				Details map[string]interface{} `json:"details"`
			}
			if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}
			if resp.OK {
				t.Fatalf("ok = true, want false: %s", w.Body.String())
			}
			if resp.Error != ErrValidationFailed {
				t.Fatalf("error = %q, want %q", resp.Error, ErrValidationFailed)
			}
			if _, ok := resp.Details[tt.field]; !ok {
				t.Fatalf("details missing field %q: %+v", tt.field, resp.Details)
			}
		})
	}

	t.Run("invalid body returns 400", func(t *testing.T) {
		h := newI2PTestHandler(t)
		c, w := newTestContextJSON(t, http.MethodPatch, "/admin/config/i2p", "{not json")
		h.UpdateSettings(w, c)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400; body=%s", w.Code, w.Body.String())
		}
		var resp struct {
			OK    bool   `json:"ok"`
			Error string `json:"error"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if resp.OK || resp.Error != ErrBadRequest {
			t.Fatalf("got ok=%v error=%q, want ok=false error=%q", resp.OK, resp.Error, ErrBadRequest)
		}
	})
}

func TestI2PAdminHandler_ValidateAcceptsDefaults(t *testing.T) {
	h := newI2PTestHandler(t)
	body := config.DefaultI2PConfig()
	c, w := newTestContextJSON(t, http.MethodPost, "/admin/config/i2p/validate", body)
	h.Validate(w, c)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
	}

	var resp struct {
		OK   bool                   `json:"ok"`
		Data map[string]interface{} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !resp.OK {
		t.Fatalf("ok = false, want true: %s", w.Body.String())
	}
	if _, ok := resp.Data["config"]; !ok {
		t.Fatalf("data missing config key: %+v", resp.Data)
	}
}

func TestI2PAdminHandler_ActionsRejectedWhenDisabled(t *testing.T) {
	tests := []struct {
		name    string
		target  string
		invoke  func(h *I2PAdminHandler, w http.ResponseWriter, r *http.Request)
		wantErr string
	}{
		{
			name:   "regenerate",
			target: "/admin/config/i2p/regenerate",
			invoke: func(h *I2PAdminHandler, w http.ResponseWriter, r *http.Request) {
				h.Regenerate(w, r)
			},
			wantErr: ErrBadRequest,
		},
		{
			name:   "restart",
			target: "/admin/config/i2p/restart",
			invoke: func(h *I2PAdminHandler, w http.ResponseWriter, r *http.Request) {
				h.Restart(w, r)
			},
			wantErr: ErrBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newI2PTestHandler(t)
			c, w := newTestContextJSON(t, http.MethodPost, tt.target, map[string]interface{}{})
			tt.invoke(h, w, c)
			if w.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400; body=%s", w.Code, w.Body.String())
			}
			var resp struct {
				OK    bool   `json:"ok"`
				Error string `json:"error"`
			}
			if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}
			if resp.OK || resp.Error != tt.wantErr {
				t.Fatalf("got ok=%v error=%q, want ok=false error=%q", resp.OK, resp.Error, tt.wantErr)
			}
		})
	}
}

// TestI2PValidationFields covers the comma-joined field list handed to the
// no-JavaScript redirect so the form can highlight the rejected inputs.
func TestI2PValidationFields(t *testing.T) {
	if got := i2pValidationFields(nil); got != "" {
		t.Errorf("i2pValidationFields(nil) = %q, want empty", got)
	}

	errs := []config.ValidationError{
		{Field: "virtual_port", Message: "out of range"},
		{Field: "inbound_length", Message: "out of range"},
	}
	if got, want := i2pValidationFields(errs), "virtual_port,inbound_length"; got != want {
		t.Errorf("i2pValidationFields() = %q, want %q", got, want)
	}
}

// TestI2PPagePath covers the action-suffix trimming that sends every form
// submission back to the settings page rather than the action endpoint.
func TestI2PPagePath(t *testing.T) {
	cases := []struct {
		path string
		want string
	}{
		{"/server/admin/config/network/i2p", "/server/admin/config/network/i2p"},
		{"/server/admin/config/network/i2p/regenerate", "/server/admin/config/network/i2p"},
		{"/server/admin/config/network/i2p/restart", "/server/admin/config/network/i2p"},
		{"/server/admin/config/network/i2p/validate", "/server/admin/config/network/i2p"},
	}
	for _, tc := range cases {
		req := httptest.NewRequest(http.MethodPost, tc.path, nil)
		if got := i2pPagePath(req); got != tc.want {
			t.Errorf("i2pPagePath(%q) = %q, want %q", tc.path, got, tc.want)
		}
	}
}

// TestRedirectToI2PPage covers the post-redirect-get half of the JS-free flow,
// including the optional rejected-fields query parameter.
func TestRedirectToI2PPage(t *testing.T) {
	t.Run("status only", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/server/admin/config/network/i2p", nil)
		rec := httptest.NewRecorder()
		redirectToI2PPage(rec, req, "saved", "")

		if rec.Code != http.StatusSeeOther {
			t.Errorf("status = %d, want %d", rec.Code, http.StatusSeeOther)
		}
		want := "/server/admin/config/network/i2p?status=saved"
		if got := rec.Header().Get("Location"); got != want {
			t.Errorf("Location = %q, want %q", got, want)
		}
	})

	t.Run("status with rejected fields", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/server/admin/config/network/i2p/regenerate", nil)
		rec := httptest.NewRecorder()
		redirectToI2PPage(rec, req, "invalid", "virtual_port")

		want := "/server/admin/config/network/i2p?status=invalid&fields=virtual_port"
		if got := rec.Header().Get("Location"); got != want {
			t.Errorf("Location = %q, want %q", got, want)
		}
	})
}

// TestI2PFormNumber covers numeric form parsing: an omitted field keeps the
// persisted value, a valid field replaces it, and an unparsable field yields
// the sentinel so the validator reports it as out of range.
func TestI2PFormNumber(t *testing.T) {
	cases := []struct {
		name    string
		form    string
		current int
		want    int
	}{
		{"omitted field keeps the current value", "other=1", 3, 3},
		{"valid value replaces the current one", "inbound_length=5", 3, 5},
		{"surrounding whitespace is trimmed", "inbound_length=%20+5+%20", 3, 5},
		{"unparsable value becomes the sentinel", "inbound_length=abc", 3, i2pInvalidNumber},
		{"empty value becomes the sentinel", "inbound_length=", 3, i2pInvalidNumber},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(tc.form))
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			if err := req.ParseForm(); err != nil {
				t.Fatalf("ParseForm() = %v", err)
			}
			if got := i2pFormNumber(req, "inbound_length", tc.current); got != tc.want {
				t.Errorf("i2pFormNumber() = %d, want %d", got, tc.want)
			}
		})
	}
}

// TestWantsI2PJSON covers the content-type sniff that decides between a JSON
// API response and the browser post-redirect-get flow.
func TestWantsI2PJSON(t *testing.T) {
	cases := []struct {
		contentType string
		want        bool
	}{
		{"application/json", true},
		{"application/json; charset=utf-8", true},
		{"application/x-www-form-urlencoded", false},
		{"", false},
	}
	for _, tc := range cases {
		req := httptest.NewRequest(http.MethodPost, "/", nil)
		if tc.contentType != "" {
			req.Header.Set("Content-Type", tc.contentType)
		}
		if got := wantsI2PJSON(req); got != tc.want {
			t.Errorf("wantsI2PJSON(%q) = %v, want %v", tc.contentType, got, tc.want)
		}
	}
}

// withI2PGlobalConfig installs a global AppConfig carrying the supplied I2P
// settings and restores the previous instance when the test ends, so enabling
// I2P for one case never leaks into another.
func withI2PGlobalConfig(t *testing.T, i2p config.I2PConfig) {
	t.Helper()
	previous := config.GetGlobalConfig()
	cfg := &config.AppConfig{}
	cfg.Server.I2P = i2p
	cfg.Server.Features.I2P = i2p.Enabled
	config.SetGlobalConfig(cfg)
	t.Cleanup(func() {
		config.SetGlobalConfig(previous)
	})
}

// newI2PFormRequest builds a form-encoded admin submission, the no-JavaScript
// path that answers with a redirect rather than a JSON body.
func newI2PFormRequest(method, target, body string) *http.Request {
	req := httptest.NewRequest(method, target, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	return req
}

// TestI2PAdminHandler_FormUpdateRejectsInvalid covers the form half of
// UpdateSettings: a rejected field must redirect back to the settings page with
// the status and the offending field name rather than emit JSON.
func TestI2PAdminHandler_FormUpdateRejectsInvalid(t *testing.T) {
	h := newI2PTestHandler(t)
	withI2PGlobalConfig(t, config.DefaultI2PConfig())

	req := newI2PFormRequest(http.MethodPost, "/server/admin/config/network/i2p", "enabled=yes&virtual_port=0")
	rec := httptest.NewRecorder()
	h.UpdateSettings(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusSeeOther, rec.Body.String())
	}
	location := rec.Header().Get("Location")
	if !strings.Contains(location, "status=invalid") || !strings.Contains(location, "fields=virtual_port") {
		t.Fatalf("Location = %q, want status=invalid with fields=virtual_port", location)
	}
}

// TestI2PAdminHandler_FormUpdateAcceptsTruthyEnabled covers the truthy boolean
// parsing and the persistence failure branch: the values validate, but the
// global config is missing so UpdateI2PConfig reports the i2p field.
func TestI2PAdminHandler_FormUpdateWithoutGlobalConfig(t *testing.T) {
	h := newI2PTestHandler(t)
	previous := config.GetGlobalConfig()
	config.SetGlobalConfig(nil)
	t.Cleanup(func() { config.SetGlobalConfig(previous) })

	req := newI2PFormRequest(http.MethodPost, "/server/admin/config/network/i2p", "enabled=on&binary=%20i2pd%20&sam_address=127.0.0.1%3A7656")
	rec := httptest.NewRecorder()
	h.UpdateSettings(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusSeeOther, rec.Body.String())
	}
	if location := rec.Header().Get("Location"); !strings.Contains(location, "fields=i2p") {
		t.Fatalf("Location = %q, want fields=i2p", location)
	}
}

// TestI2PAdminHandler_JSONUpdateWithoutGlobalConfig covers the JSON half of the
// same persistence failure, which must answer with the canonical error shape.
func TestI2PAdminHandler_JSONUpdateWithoutGlobalConfig(t *testing.T) {
	h := newI2PTestHandler(t)
	previous := config.GetGlobalConfig()
	config.SetGlobalConfig(nil)
	t.Cleanup(func() { config.SetGlobalConfig(previous) })

	c, w := newTestContextJSON(t, http.MethodPatch, "/admin/config/i2p", config.DefaultI2PConfig())
	h.UpdateSettings(w, c)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body=%s", w.Code, w.Body.String())
	}
	var resp struct {
		OK      bool                   `json:"ok"`
		Error   string                 `json:"error"`
		Details map[string]interface{} `json:"details"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.OK || resp.Error != ErrValidationFailed {
		t.Fatalf("got ok=%v error=%q, want ok=false error=%q", resp.OK, resp.Error, ErrValidationFailed)
	}
	if _, ok := resp.Details["i2p"]; !ok {
		t.Fatalf("details missing i2p key: %+v", resp.Details)
	}
}

// TestI2PAdminHandler_StatusPayloadEnabled covers the enabled branch of the
// status payload, which reads live manager state instead of the fixed disabled
// block. No provider exists in the test environment, so the eepsite is enabled
// but not running.
func TestI2PAdminHandler_StatusPayloadEnabled(t *testing.T) {
	h := newI2PTestHandler(t)
	enabled := config.DefaultI2PConfig()
	enabled.Enabled = true
	withI2PGlobalConfig(t, enabled)

	payload := h.i2pStatusPayload()
	if payload["enabled"] != true {
		t.Fatalf("enabled = %v, want true", payload["enabled"])
	}
	if payload["running"] != false {
		t.Fatalf("running = %v, want false (no provider available)", payload["running"])
	}
	for _, key := range []string{"status", "provider", "address", "binary", "sam_address", "uptime_seconds", "started_at"} {
		if _, ok := payload[key]; !ok {
			t.Fatalf("payload missing key %q: %+v", key, payload)
		}
	}
	if payload["uptime_seconds"] != int64(0) {
		t.Fatalf("uptime_seconds = %v, want 0 while stopped", payload["uptime_seconds"])
	}
}

// TestI2PAdminHandler_FormActionsWhenDisabled covers the redirect branch that
// browsers get when they submit an action form while I2P is off.
func TestI2PAdminHandler_FormActionsWhenDisabled(t *testing.T) {
	cases := []struct {
		name   string
		target string
		invoke func(h *I2PAdminHandler, w http.ResponseWriter, r *http.Request)
	}{
		{"regenerate", "/server/admin/config/network/i2p/regenerate", func(h *I2PAdminHandler, w http.ResponseWriter, r *http.Request) { h.Regenerate(w, r) }},
		{"restart", "/server/admin/config/network/i2p/restart", func(h *I2PAdminHandler, w http.ResponseWriter, r *http.Request) { h.Restart(w, r) }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := newI2PTestHandler(t)
			withI2PGlobalConfig(t, config.DefaultI2PConfig())

			req := newI2PFormRequest(http.MethodPost, tc.target, "")
			rec := httptest.NewRecorder()
			tc.invoke(h, rec, req)

			if rec.Code != http.StatusSeeOther {
				t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusSeeOther, rec.Body.String())
			}
			want := "/server/admin/config/network/i2p?status=disabled"
			if got := rec.Header().Get("Location"); got != want {
				t.Fatalf("Location = %q, want %q", got, want)
			}
		})
	}
}

// TestI2PAdminHandler_ActionsReportProviderError covers the enabled-but-no-
// provider branch of both actions, in the JSON and the redirect flavour.
func TestI2PAdminHandler_ActionsReportProviderError(t *testing.T) {
	cases := []struct {
		name   string
		target string
		invoke func(h *I2PAdminHandler, w http.ResponseWriter, r *http.Request)
	}{
		{"regenerate", "/server/admin/config/network/i2p/regenerate", func(h *I2PAdminHandler, w http.ResponseWriter, r *http.Request) { h.Regenerate(w, r) }},
		{"restart", "/server/admin/config/network/i2p/restart", func(h *I2PAdminHandler, w http.ResponseWriter, r *http.Request) { h.Restart(w, r) }},
	}

	enabled := config.DefaultI2PConfig()
	enabled.Enabled = true

	for _, tc := range cases {
		t.Run(tc.name+" json", func(t *testing.T) {
			h := newI2PTestHandler(t)
			withI2PGlobalConfig(t, enabled)

			c, w := newTestContextJSON(t, http.MethodPost, tc.target, map[string]interface{}{})
			tc.invoke(h, w, c)

			if w.Code != http.StatusInternalServerError {
				t.Fatalf("status = %d, want 500; body=%s", w.Code, w.Body.String())
			}
			var resp struct {
				OK    bool   `json:"ok"`
				Error string `json:"error"`
			}
			if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}
			if resp.OK || resp.Error != ErrServiceUnavail {
				t.Fatalf("got ok=%v error=%q, want ok=false error=%q", resp.OK, resp.Error, ErrServiceUnavail)
			}
		})

		t.Run(tc.name+" form", func(t *testing.T) {
			h := newI2PTestHandler(t)
			withI2PGlobalConfig(t, enabled)

			req := newI2PFormRequest(http.MethodPost, tc.target, "")
			rec := httptest.NewRecorder()
			tc.invoke(h, rec, req)

			if rec.Code != http.StatusSeeOther {
				t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusSeeOther, rec.Body.String())
			}
			want := "/server/admin/config/network/i2p?status=provider-error"
			if got := rec.Header().Get("Location"); got != want {
				t.Fatalf("Location = %q, want %q", got, want)
			}
		})
	}
}

// TestI2PAdminHandler_ValidateFormSubmission covers the validate endpoint fed
// from a form rather than JSON; it always answers with JSON because the admin
// page calls it from the client side.
func TestI2PAdminHandler_ValidateFormSubmission(t *testing.T) {
	h := newI2PTestHandler(t)
	withI2PGlobalConfig(t, config.DefaultI2PConfig())

	t.Run("valid values", func(t *testing.T) {
		req := newI2PFormRequest(http.MethodPost, "/server/admin/config/network/i2p/validate", "enabled=true&inbound_length=3&outbound_length=3")
		rec := httptest.NewRecorder()
		h.Validate(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
		}
	})

	t.Run("rejected values", func(t *testing.T) {
		req := newI2PFormRequest(http.MethodPost, "/server/admin/config/network/i2p/validate", "enabled=true&inbound_length=abc")
		rec := httptest.NewRecorder()
		h.Validate(rec, req)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400; body=%s", rec.Code, rec.Body.String())
		}
		var resp struct {
			Details map[string]interface{} `json:"details"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if _, ok := resp.Details["inbound_length"]; !ok {
			t.Fatalf("details missing inbound_length: %+v", resp.Details)
		}
	})
}

// TestI2PValidationDetails covers the field-keyed details map returned with the
// canonical validation error body.
func TestI2PValidationDetails(t *testing.T) {
	if got := i2pValidationDetails(nil); len(got) != 0 {
		t.Errorf("i2pValidationDetails(nil) = %+v, want empty", got)
	}

	details := i2pValidationDetails([]config.ValidationError{
		{Field: "virtual_port", Message: "out of range"},
		{Field: "signature_type", Message: "unsupported"},
	})
	if len(details) != 2 {
		t.Fatalf("len(details) = %d, want 2", len(details))
	}
	if details["virtual_port"] != "out of range" {
		t.Errorf("details[virtual_port] = %v, want %q", details["virtual_port"], "out of range")
	}
	if details["signature_type"] != "unsupported" {
		t.Errorf("details[signature_type] = %v, want %q", details["signature_type"], "unsupported")
	}
}

// TestCurrentI2PConfig covers the fallback that keeps the admin page usable
// before the global config has been initialized.
func TestCurrentI2PConfig(t *testing.T) {
	previous := config.GetGlobalConfig()
	t.Cleanup(func() { config.SetGlobalConfig(previous) })

	config.SetGlobalConfig(nil)
	if got := currentI2PConfig(); got.Enabled {
		t.Errorf("currentI2PConfig().Enabled = true, want false (I2P is opt-in)")
	}

	enabled := config.DefaultI2PConfig()
	enabled.Enabled = true
	cfg := &config.AppConfig{}
	cfg.Server.I2P = enabled
	config.SetGlobalConfig(cfg)
	if got := currentI2PConfig(); !got.Enabled {
		t.Errorf("currentI2PConfig().Enabled = false, want true")
	}
}
