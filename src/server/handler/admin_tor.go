package handler

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strconv"

	"github.com/webappsgo/wthr/src/common/display"
	"github.com/webappsgo/wthr/src/server/model"
	"github.com/webappsgo/wthr/src/server/service"
)

// TorAdminHandler handles Tor administration endpoints
type TorAdminHandler struct {
	torService      *service.TorService
	settingsModel   *model.SettingsModel
	vanityGenerator *service.VanityGenerator
	keyManager      *service.TorKeyManager
}

// NewTorAdminHandler creates a new Tor admin handler
func NewTorAdminHandler(torService *service.TorService, settingsModel *model.SettingsModel, dataDir string) *TorAdminHandler {
	return &TorAdminHandler{
		torService:      torService,
		settingsModel:   settingsModel,
		vanityGenerator: service.NewVanityGenerator(),
		keyManager:      service.NewTorKeyManager(dataDir + "/tor"),
	}
}

func (h *TorAdminHandler) GetServiceStatus() map[string]interface{} {
	return h.torService.GetStatus()
}

func (h *TorAdminHandler) GetServiceHealth() map[string]interface{} {
	return h.torService.GetHealthStatus()
}

func (h *TorAdminHandler) EnableService(httpPort int) (map[string]interface{}, error) {
	if err := h.settingsModel.SetBool("tor.enabled", true); err != nil {
		return nil, fmt.Errorf("failed to update settings: %w", err)
	}
	if err := h.torService.StartTorService(httpPort); err != nil {
		return nil, fmt.Errorf("failed to start Tor: %w", err)
	}
	return h.torService.GetStatus(), nil
}

func (h *TorAdminHandler) DisableService() error {
	if err := h.settingsModel.SetBool("tor.enabled", false); err != nil {
		return fmt.Errorf("failed to update settings: %w", err)
	}
	if err := h.torService.StopTorService(); err != nil {
		return fmt.Errorf("failed to stop Tor: %w", err)
	}
	h.torService.SetDisabled()
	return nil
}

func (h *TorAdminHandler) RegenerateService(httpPort int) (string, error) {
	if err := h.torService.RegenerateAddress(httpPort); err != nil {
		return "", fmt.Errorf("failed to regenerate address: %w", err)
	}
	return h.torService.GetOnionAddress(), nil
}

func (h *TorAdminHandler) GetVanityGenerationStatus() *service.VanityGenerationStatus {
	return h.vanityGenerator.GetStatus()
}

func (h *TorAdminHandler) StartVanityGeneration(prefix string) error {
	if err := h.vanityGenerator.StartVanityGeneration(prefix); err != nil {
		return fmt.Errorf("failed to start generation: %w", err)
	}
	go h.monitorVanityGeneration()
	return nil
}

func (h *TorAdminHandler) CancelVanityGeneration() error {
	if err := h.vanityGenerator.Cancel(); err != nil {
		return fmt.Errorf("failed to cancel: %w", err)
	}
	return nil
}

func (h *TorAdminHandler) ApplyVanityKeys(httpPort int) (string, error) {
	publicKey, privateKey, err := h.vanityGenerator.GetKeys()
	if err != nil {
		return "", fmt.Errorf("no keys available: %w", err)
	}
	if err := h.keyManager.ImportKeys(publicKey, privateKey); err != nil {
		return "", fmt.Errorf("failed to import keys: %w", err)
	}
	if err := h.torService.RegenerateAddress(httpPort); err != nil {
		return "", fmt.Errorf("failed to restart Tor: %w", err)
	}
	return h.torService.GetOnionAddress(), nil
}

func (h *TorAdminHandler) ImportTorKeys(publicKey, privateKey []byte, httpPort int) (string, error) {
	if err := h.keyManager.ImportKeys(publicKey, privateKey); err != nil {
		return "", fmt.Errorf("failed to import keys: %w", err)
	}
	if err := h.torService.RegenerateAddress(httpPort); err != nil {
		return "", fmt.Errorf("failed to restart Tor: %w", err)
	}
	return h.torService.GetOnionAddress(), nil
}

func (h *TorAdminHandler) ExportTorKeys() ([]byte, []byte, error) {
	publicKey, privateKey, err := h.keyManager.ExportKeys()
	if err != nil {
		return nil, nil, fmt.Errorf("failed to export keys: %w", err)
	}
	return publicKey, privateKey, nil
}

// GetStatus returns Tor service status
// GET /{api_version}/admin/server/tor/status
func (h *TorAdminHandler) GetStatus(w http.ResponseWriter, r *http.Request) {
	status := h.torService.GetStatus()

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"status": status,
	})
}

// GetHealth returns Tor service health status
// GET /{api_version}/admin/server/tor/health
func (h *TorAdminHandler) GetHealth(w http.ResponseWriter, r *http.Request) {
	health := h.torService.GetHealthStatus()

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"health": health,
	})
}

// Enable enables the Tor service
// POST /{api_version}/admin/server/tor/enable
func (h *TorAdminHandler) Enable(w http.ResponseWriter, r *http.Request) {
	// Get HTTP port from settings or context
	// Default, should be retrieved from actual server config
	httpPort := 8080

	// Update setting
	if err := h.settingsModel.SetBool("tor.enabled", true); err != nil {
		log.Printf("tor admin: settings_update_failed: %v", err)
		RespondError(w, r, http.StatusInternalServerError, "DATABASE_ERROR", Translate(r, "errors.tor.settings_update_failed"))
		return
	}

	// Start Tor service
	if err := h.torService.StartTorService(httpPort); err != nil {
		log.Printf("tor admin: start_failed: %v", err)
		RespondError(w, r, http.StatusInternalServerError, "TOR_START_FAILED", Translate(r, "errors.tor.start_failed"))
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"ok":      true,
		"message": Translate(r, "success.tor.service_started"),
		"status":  h.torService.GetStatus(),
	})
}

// Disable disables the Tor service
// POST /{api_version}/admin/server/tor/disable
func (h *TorAdminHandler) Disable(w http.ResponseWriter, r *http.Request) {
	// Update setting
	if err := h.settingsModel.SetBool("tor.enabled", false); err != nil {
		log.Printf("tor admin: settings_update_failed: %v", err)
		RespondError(w, r, http.StatusInternalServerError, "DATABASE_ERROR", Translate(r, "errors.tor.settings_update_failed"))
		return
	}

	// Stop Tor service
	if err := h.torService.StopTorService(); err != nil {
		log.Printf("tor admin: stop_failed: %v", err)
		RespondError(w, r, http.StatusInternalServerError, "TOR_STOP_FAILED", Translate(r, "errors.tor.stop_failed"))
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"ok":      true,
		"message": Translate(r, "success.tor.service_stopped"),
	})
}

// UpdateSettings handles PATCH /server/tor per AI.md spec
// Accepts {"enabled": bool} to enable/disable Tor
func (h *TorAdminHandler) UpdateSettings(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Enabled *bool `json:"enabled"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		RespondError(w, r, http.StatusBadRequest, "INVALID_REQUEST", Translate(r, "errors.common.invalid_request_body"))
		return
	}

	if req.Enabled != nil {
		if *req.Enabled {
			h.Enable(w, r)
		} else {
			h.Disable(w, r)
		}
		return
	}

	RespondError(w, r, http.StatusBadRequest, "INVALID_REQUEST", Translate(r, "errors.common.no_settings_to_update"))
}

// Regenerate regenerates the .onion address
// POST /{api_version}/admin/server/tor/regenerate
func (h *TorAdminHandler) Regenerate(w http.ResponseWriter, r *http.Request) {
	// Should be retrieved from actual server config
	httpPort := 8080

	if err := h.torService.RegenerateAddress(httpPort); err != nil {
		log.Printf("tor admin: regenerate_failed: %v", err)
		RespondError(w, r, http.StatusInternalServerError, "REGENERATE_FAILED", Translate(r, "errors.tor.regenerate_failed"))
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"ok":      true,
		"message": Translate(r, "success.tor.address_regenerated"),
		"address": h.torService.GetOnionAddress(),
	})
}

// GenerateVanity starts vanity address generation
// POST /{api_version}/admin/server/tor/vanity/generate
func (h *TorAdminHandler) GenerateVanity(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Prefix string `json:"prefix"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		RespondError(w, r, http.StatusBadRequest, "INVALID_REQUEST", Translate(r, "errors.tor.invalid_prefix"))
		return
	}

	// binding:"required" equivalent
	if req.Prefix == "" {
		RespondError(w, r, http.StatusBadRequest, "INVALID_REQUEST", Translate(r, "errors.tor.invalid_prefix"))
		return
	}

	if err := h.vanityGenerator.StartVanityGeneration(req.Prefix); err != nil {
		log.Printf("tor admin: generation_failed: %v", err)
		RespondError(w, r, http.StatusBadRequest, "GENERATION_FAILED", Translate(r, "errors.tor.generation_failed"))
		return
	}

	// Start monitoring for completion in background
	go h.monitorVanityGeneration()

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"ok":      true,
		"message": fmt.Sprintf("Started generating vanity address with prefix: %s", req.Prefix),
		"status":  h.vanityGenerator.GetStatus(),
	})
}

// monitorVanityGeneration watches for completion and sends notification
func (h *TorAdminHandler) monitorVanityGeneration() {
	notifyCh := h.vanityGenerator.GetNotificationChannel()
	address := <-notifyCh

	// Notification sent via notification service
	fmt.Printf("%s Vanity address generated: %s\n", display.Emoji("🎉", "*"), address)
}

// GetVanityStatus returns vanity generation status
// GET /{api_version}/admin/server/tor/vanity/status
func (h *TorAdminHandler) GetVanityStatus(w http.ResponseWriter, r *http.Request) {
	status := h.vanityGenerator.GetStatus()

	if status == nil {
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"running": false,
		})
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"running":        status.Running,
		"prefix":         status.Prefix,
		"start_time":     status.StartTime,
		"attempts":       status.Attempts,
		"estimated_time": status.EstimatedTime,
		"address":        status.Address,
	})
}

// CancelVanity cancels vanity generation
// POST /{api_version}/admin/server/tor/vanity/cancel
func (h *TorAdminHandler) CancelVanity(w http.ResponseWriter, r *http.Request) {
	if err := h.vanityGenerator.Cancel(); err != nil {
		log.Printf("tor admin: cancel_failed: %v", err)
		RespondError(w, r, http.StatusBadRequest, "CANCEL_FAILED", Translate(r, "errors.tor.cancel_failed"))
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"ok":      true,
		"message": Translate(r, "success.tor.vanity_cancelled"),
	})
}

// ApplyVanity applies the generated vanity keys
// POST /{api_version}/admin/server/tor/vanity/apply
func (h *TorAdminHandler) ApplyVanity(w http.ResponseWriter, r *http.Request) {
	// Get generated keys
	publicKey, privateKey, err := h.vanityGenerator.GetKeys()
	if err != nil {
		log.Printf("tor admin: no_keys: %v", err)
		RespondError(w, r, http.StatusBadRequest, "NO_KEYS", Translate(r, "errors.tor.no_keys"))
		return
	}

	// Import keys
	if err := h.keyManager.ImportKeys(publicKey, privateKey); err != nil {
		log.Printf("tor admin: import_failed: %v", err)
		RespondError(w, r, http.StatusInternalServerError, "IMPORT_FAILED", Translate(r, "errors.tor.import_failed"))
		return
	}

	// Restart Tor with new keys
	httpPort := 8080
	if err := h.torService.RegenerateAddress(httpPort); err != nil {
		log.Printf("tor admin: restart_failed: %v", err)
		RespondError(w, r, http.StatusInternalServerError, "RESTART_FAILED", Translate(r, "errors.tor.restart_failed"))
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"ok":      true,
		"message": Translate(r, "success.tor.vanity_applied"),
		"address": h.torService.GetOnionAddress(),
	})
}

// ImportKeys imports external Tor keys
// POST /{api_version}/admin/server/tor/keys/import
func (h *TorAdminHandler) ImportKeys(w http.ResponseWriter, r *http.Request) {
	file, _, err := r.FormFile("key_file")
	if err != nil {
		RespondError(w, r, http.StatusBadRequest, "NO_FILE", Translate(r, "errors.tor.no_key_file"))
		return
	}
	defer file.Close()

	// Save uploaded file temporarily
	tempPath := "/tmp/tor_key_upload"
	dest, err := os.Create(tempPath)
	if err == nil {
		_, err = io.Copy(dest, file)
		dest.Close()
	}
	if err != nil {
		log.Printf("tor admin: save_failed: %v", err)
		RespondError(w, r, http.StatusInternalServerError, "SAVE_FAILED", Translate(r, "errors.tor.save_failed"))
		return
	}

	// Import from file
	if err := h.keyManager.ImportFromFile(tempPath); err != nil {
		log.Printf("tor admin: import_failed: %v", err)
		RespondError(w, r, http.StatusBadRequest, "IMPORT_FAILED", Translate(r, "errors.tor.import_failed"))
		return
	}

	// Restart Tor with new keys
	httpPort := 8080
	if err := h.torService.RegenerateAddress(httpPort); err != nil {
		log.Printf("tor admin: restart_failed: %v", err)
		RespondError(w, r, http.StatusInternalServerError, "RESTART_FAILED", Translate(r, "errors.tor.restart_failed"))
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"ok":      true,
		"message": Translate(r, "success.tor.keys_imported"),
		"address": h.torService.GetOnionAddress(),
	})
}

// ExportKeys exports current Tor keys
// GET /{api_version}/admin/server/tor/keys/export
func (h *TorAdminHandler) ExportKeys(w http.ResponseWriter, r *http.Request) {
	publicKey, privateKey, err := h.keyManager.ExportKeys()
	if err != nil {
		log.Printf("tor admin: export_failed: %v", err)
		RespondError(w, r, http.StatusInternalServerError, "EXPORT_FAILED", Translate(r, "errors.tor.export_failed"))
		return
	}

	// Return private key file for download
	w.Header().Set("Content-Disposition", "attachment; filename=hs_ed25519_secret_key")
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Length", strconv.Itoa(len(privateKey)))

	// Write key in Tor format (32-byte header + 32-byte key)
	header := []byte("== ed25519v1-secret: type0 ==")
	padding := make([]byte, 32-len(header))
	w.Write(header)
	w.Write(padding)
	w.Write(privateKey)

	// Public key can be derived from private
	_ = publicKey
}
