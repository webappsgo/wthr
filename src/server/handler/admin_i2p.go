package handler

import (
	"encoding/json"
	"net/http"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/webappsgo/wthr/src/config"
	"github.com/webappsgo/wthr/src/server/middleware"
	"github.com/webappsgo/wthr/src/server/model"
	"github.com/webappsgo/wthr/src/server/service"
)

// i2pInvalidNumber is the sentinel stored for an unparsable numeric form field so the validator reports it as out of range
const i2pInvalidNumber = -1

// I2PAdminHandler handles I2P eepsite administration per AI.md PART 32.2
type I2PAdminHandler struct {
	i2pManager *service.I2PManager
	// settingsModel mirrors the Tor handler constructor shape
	settingsModel *model.SettingsModel
	// dataDir holds the destination key directory root
	dataDir string
}

// NewI2PAdminHandler creates a new I2P admin handler
func NewI2PAdminHandler(i2pManager *service.I2PManager, settingsModel *model.SettingsModel, dataDir string) *I2PAdminHandler {
	return &I2PAdminHandler{
		i2pManager:    i2pManager,
		settingsModel: settingsModel,
		dataDir:       dataDir,
	}
}

// currentI2PConfig returns the persisted I2P settings and falls back to the opt-in defaults when the global config is not yet initialized
func currentI2PConfig() config.I2PConfig {
	if cfg := config.GetGlobalConfig(); cfg != nil {
		return cfg.Server.I2P
	}
	return config.DefaultI2PConfig()
}

// i2pStatusPayload builds the read-only status block and always reports a fully disabled eepsite when I2P is off
func (h *I2PAdminHandler) i2pStatusPayload() map[string]interface{} {
	cfg := currentI2PConfig()
	payload := map[string]interface{}{
		"enabled":        cfg.Enabled,
		"running":        false,
		"status":         "disabled",
		"provider":       "none",
		"address":        "",
		"binary":         "",
		"sam_address":    "",
		"uptime_seconds": int64(0),
		"started_at":     "",
	}
	if h.i2pManager == nil || !cfg.Enabled {
		return payload
	}

	payload["running"] = h.i2pManager.IsRunning()
	payload["status"] = h.i2pManager.Status()
	payload["provider"] = h.i2pManager.Provider()
	payload["address"] = h.i2pManager.EepsiteAddress()
	payload["binary"] = h.i2pManager.BinaryPath()
	payload["sam_address"] = h.i2pManager.SAMAddress()
	payload["uptime_seconds"] = h.i2pManager.UptimeSeconds()
	if startedAt := h.i2pManager.StartedAt(); !startedAt.IsZero() {
		payload["started_at"] = startedAt.UTC().Format(time.RFC3339)
	}
	return payload
}

// i2pValidationDetails converts validator output into the canonical error response details map keyed by field name
func i2pValidationDetails(errs []config.ValidationError) map[string]interface{} {
	details := make(map[string]interface{}, len(errs))
	for _, e := range errs {
		details[e.Field] = e.Message
	}
	return details
}

// i2pValidationFields lists rejected field names for the redirect used by browsers without JavaScript
func i2pValidationFields(errs []config.ValidationError) string {
	fields := make([]string, 0, len(errs))
	for _, e := range errs {
		fields = append(fields, e.Field)
	}
	return strings.Join(fields, ",")
}

// wantsI2PJSON reports whether the request carried a JSON payload rather than an HTML form submission
func wantsI2PJSON(r *http.Request) bool {
	return strings.Contains(r.Header.Get("Content-Type"), "application/json")
}

// i2pPagePath resolves the admin page that a form submission should return to
func i2pPagePath(r *http.Request) string {
	current := r.URL.Path
	for _, action := range []string{"/regenerate", "/restart", "/validate"} {
		if strings.HasSuffix(current, action) {
			return path.Dir(current)
		}
	}
	return current
}

// redirectToI2PPage performs the redirect half of the post redirect get flow used when JavaScript is unavailable
func redirectToI2PPage(w http.ResponseWriter, r *http.Request, status, fields string) {
	target := i2pPagePath(r) + "?status=" + status
	if fields != "" {
		target += "&fields=" + fields
	}
	http.Redirect(w, r, target, http.StatusSeeOther)
}

// i2pFormNumber reads a numeric form field and keeps the current value when the field was not submitted
func i2pFormNumber(r *http.Request, field string, current int) int {
	if !r.PostForm.Has(field) {
		return current
	}
	value, err := strconv.Atoi(strings.TrimSpace(r.PostFormValue(field)))
	if err != nil {
		return i2pInvalidNumber
	}
	return value
}

// decodeI2PConfig builds the next configuration starting from the persisted values so an omitted field keeps its current value
func (h *I2PAdminHandler) decodeI2PConfig(r *http.Request) (config.I2PConfig, error) {
	next := currentI2PConfig()

	if wantsI2PJSON(r) {
		if err := json.NewDecoder(r.Body).Decode(&next); err != nil {
			return next, err
		}
		return next, nil
	}

	if err := r.ParseForm(); err != nil {
		return next, err
	}

	enabled, err := config.ParseBool(r.PostFormValue("enabled"), false)
	if err != nil {
		enabled = false
	}
	next.Enabled = enabled

	if r.PostForm.Has("binary") {
		next.Binary = strings.TrimSpace(r.PostFormValue("binary"))
	}
	if r.PostForm.Has("sam_address") {
		next.SAMAddress = strings.TrimSpace(r.PostFormValue("sam_address"))
	}

	next.VirtualPort = i2pFormNumber(r, "virtual_port", next.VirtualPort)
	next.InboundLength = i2pFormNumber(r, "inbound_length", next.InboundLength)
	next.OutboundLength = i2pFormNumber(r, "outbound_length", next.OutboundLength)
	next.InboundQuantity = i2pFormNumber(r, "inbound_quantity", next.InboundQuantity)
	next.OutboundQuantity = i2pFormNumber(r, "outbound_quantity", next.OutboundQuantity)
	next.SignatureType = i2pFormNumber(r, "signature_type", next.SignatureType)
	next.BootstrapTimeout = i2pFormNumber(r, "bootstrap_timeout", next.BootstrapTimeout)

	return next, nil
}

// GetStatus returns the I2P eepsite status and configuration
// GET /api/{api_version}/server/{admin_path}/config/i2p
func (h *I2PAdminHandler) GetStatus(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"status": h.i2pStatusPayload(),
		"config": currentI2PConfig(),
	})
}

// UpdateSettings validates, persists and live applies the I2P settings
// PATCH /api/{api_version}/server/{admin_path}/config/i2p
func (h *I2PAdminHandler) UpdateSettings(w http.ResponseWriter, r *http.Request) {
	next, err := h.decodeI2PConfig(r)
	if err != nil {
		if !wantsI2PJSON(r) {
			redirectToI2PPage(w, r, "invalid", "")
			return
		}
		RespondError(w, r, http.StatusBadRequest, ErrBadRequest, Translate(r, "errors.invalid_request"))
		return
	}

	if errs := config.ValidateI2PConfig(&next); len(errs) > 0 {
		if !wantsI2PJSON(r) {
			redirectToI2PPage(w, r, "invalid", i2pValidationFields(errs))
			return
		}
		RespondError(w, r, http.StatusBadRequest, ErrValidationFailed, Translate(r, "errors.validation_failed"), i2pValidationDetails(errs))
		return
	}

	if errs := config.UpdateI2PConfig(next); len(errs) > 0 {
		if !wantsI2PJSON(r) {
			redirectToI2PPage(w, r, "invalid", i2pValidationFields(errs))
			return
		}
		RespondError(w, r, http.StatusBadRequest, ErrValidationFailed, Translate(r, "errors.validation_failed"), i2pValidationDetails(errs))
		return
	}

	if h.i2pManager != nil {
		applied := next
		if applyErr := h.i2pManager.UpdateConfig(&applied); applyErr != nil {
			if !wantsI2PJSON(r) {
				redirectToI2PPage(w, r, "provider-error", "")
				return
			}
			RespondError(w, r, http.StatusInternalServerError, ErrServiceUnavail, applyErr.Error())
			return
		}
	}

	if !wantsI2PJSON(r) {
		redirectToI2PPage(w, r, "saved", "")
		return
	}
	RespondSuccess(w, r, Translate(r, "admin.i2p.settings_saved"), map[string]interface{}{
		"status": h.i2pStatusPayload(),
		"config": currentI2PConfig(),
	})
}

// Validate checks the submitted settings without saving them
// POST /api/{api_version}/server/{admin_path}/config/i2p/validate
func (h *I2PAdminHandler) Validate(w http.ResponseWriter, r *http.Request) {
	next, err := h.decodeI2PConfig(r)
	if err != nil {
		RespondError(w, r, http.StatusBadRequest, ErrBadRequest, Translate(r, "errors.invalid_request"))
		return
	}

	if errs := config.ValidateI2PConfig(&next); len(errs) > 0 {
		RespondError(w, r, http.StatusBadRequest, ErrValidationFailed, Translate(r, "errors.validation_failed"), i2pValidationDetails(errs))
		return
	}

	RespondSuccess(w, r, Translate(r, "admin.i2p.settings_valid"), map[string]interface{}{
		"config": next,
	})
}

// Regenerate creates a brand new .b32.i2p destination
// POST /api/{api_version}/server/{admin_path}/config/i2p/regenerate
func (h *I2PAdminHandler) Regenerate(w http.ResponseWriter, r *http.Request) {
	if h.i2pManager == nil || !currentI2PConfig().Enabled {
		if !wantsI2PJSON(r) {
			redirectToI2PPage(w, r, "disabled", "")
			return
		}
		RespondError(w, r, http.StatusBadRequest, ErrBadRequest, Translate(r, "admin.i2p.error_disabled"))
		return
	}

	address, err := h.i2pManager.RegenerateAddress()
	if err != nil {
		if !wantsI2PJSON(r) {
			redirectToI2PPage(w, r, "provider-error", "")
			return
		}
		RespondError(w, r, http.StatusInternalServerError, ErrServiceUnavail, err.Error())
		return
	}

	if !wantsI2PJSON(r) {
		redirectToI2PPage(w, r, "regenerated", "")
		return
	}
	RespondSuccess(w, r, Translate(r, "admin.i2p.address_regenerated"), map[string]interface{}{
		"address": address,
		"status":  h.i2pStatusPayload(),
	})
}

// Restart restarts the I2P provider with the persisted configuration
// POST /api/{api_version}/server/{admin_path}/config/i2p/restart
func (h *I2PAdminHandler) Restart(w http.ResponseWriter, r *http.Request) {
	if h.i2pManager == nil || !currentI2PConfig().Enabled {
		if !wantsI2PJSON(r) {
			redirectToI2PPage(w, r, "disabled", "")
			return
		}
		RespondError(w, r, http.StatusBadRequest, ErrBadRequest, Translate(r, "admin.i2p.error_disabled"))
		return
	}

	current := currentI2PConfig()
	if err := h.i2pManager.UpdateConfig(&current); err != nil {
		if !wantsI2PJSON(r) {
			redirectToI2PPage(w, r, "provider-error", "")
			return
		}
		RespondError(w, r, http.StatusInternalServerError, ErrServiceUnavail, err.Error())
		return
	}

	if !wantsI2PJSON(r) {
		redirectToI2PPage(w, r, "restarted", "")
		return
	}
	RespondSuccess(w, r, Translate(r, "admin.i2p.provider_restarted"), map[string]interface{}{
		"status": h.i2pStatusPayload(),
	})
}

// ShowI2PSettings renders the I2P eepsite admin page
// GET /server/{admin_path}/config/network/i2p
func (h *I2PAdminHandler) ShowI2PSettings(w http.ResponseWriter, r *http.Request) {
	middleware.RenderHTML(w, r, http.StatusOK, "admin/admin_i2p.tmpl", AdminTemplateData(r, map[string]interface{}{
		"title":          Translate(r, "admin.i2p.title"),
		"page":           "i2p",
		"breadcrumb":     Translate(r, "admin.i2p.title"),
		"I2PStatus":      h.i2pStatusPayload(),
		"I2PConfig":      currentI2PConfig(),
		"form_status":    r.URL.Query().Get("status"),
		"invalid_fields": r.URL.Query().Get("fields"),
	}))
}
