package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/crewjam/saml"
	"github.com/go-chi/chi/v5"
	"github.com/webappsgo/wthr/src/config"
	"github.com/webappsgo/wthr/src/database"
	models "github.com/webappsgo/wthr/src/server/model"
	"github.com/webappsgo/wthr/src/server/service"
	"github.com/webappsgo/wthr/src/util"
)

func (h *AdminAuthSettingsHandler) ListSAMLProvidersAPI(w http.ResponseWriter, r *http.Request) {
	settings := readAuthSettings(h.ConfigPath)
	writeJSON(w, http.StatusOK, map[string]interface{}{"ok": true, "data": map[string]interface{}{"providers": settings.SAMLProviders}})
}

func (h *AdminAuthSettingsHandler) GetSAMLProviderAPI(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "provider")
	for _, provider := range readAuthSettings(h.ConfigPath).SAMLProviders {
		if provider.Name == name {
			writeJSON(w, http.StatusOK, map[string]interface{}{"ok": true, "data": provider})
			return
		}
	}
	writeJSON(w, http.StatusNotFound, map[string]interface{}{"ok": false, "error": "NOT_FOUND", "message": "SAML provider not found"})
}

func (h *AdminAuthSettingsHandler) CreateSAMLProviderAPI(w http.ResponseWriter, r *http.Request) {
	var provider service.SAMLProviderConfig
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&provider); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{"ok": false, "error": "INVALID_JSON", "message": "Invalid SAML provider"})
		return
	}
	provider.Name = strings.ToLower(strings.TrimSpace(provider.Name))
	if !validOIDCProviderName(provider.Name) || !validSAMLProviderInput(provider) {
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{"ok": false, "error": "VALIDATION_FAILED", "message": "SAML provider name and valid IdP metadata are required"})
		return
	}
	settings := readAuthSettings(h.ConfigPath)
	for _, existing := range settings.SAMLProviders {
		if existing.Name == provider.Name {
			writeJSON(w, http.StatusConflict, map[string]interface{}{"ok": false, "error": "CONFLICT", "message": "SAML provider already exists"})
			return
		}
	}
	if provider.CertExpiryDays == 0 {
		provider.CertExpiryDays = 365
	}
	if !provider.AutoGenerateCert && provider.SPCertPath == "" && provider.SPKeyPath == "" {
		provider.AutoGenerateCert = true
	}
	settings.SAMLProviders = append(settings.SAMLProviders, provider)
	if err := h.storeSAMLSettings(r.Context(), settings.SAMLEnabled, settings.SAMLProviders); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"ok": false, "error": "INTERNAL_ERROR", "message": "SAML provider could not be saved"})
		return
	}
	writeJSON(w, http.StatusCreated, map[string]interface{}{"ok": true, "data": provider})
}

func (h *AdminAuthSettingsHandler) UpdateSAMLProviderAPI(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "provider")
	var provider service.SAMLProviderConfig
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&provider); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{"ok": false, "error": "INVALID_JSON", "message": "Invalid SAML provider"})
		return
	}
	provider.Name = name
	if !validSAMLProviderInput(provider) {
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{"ok": false, "error": "VALIDATION_FAILED", "message": "SAML provider metadata is invalid"})
		return
	}
	settings := readAuthSettings(h.ConfigPath)
	found := false
	for i := range settings.SAMLProviders {
		if settings.SAMLProviders[i].Name == name {
			settings.SAMLProviders[i] = provider
			found = true
			break
		}
	}
	if !found {
		writeJSON(w, http.StatusNotFound, map[string]interface{}{"ok": false, "error": "NOT_FOUND", "message": "SAML provider not found"})
		return
	}
	if err := h.storeSAMLSettings(r.Context(), settings.SAMLEnabled, settings.SAMLProviders); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"ok": false, "error": "INTERNAL_ERROR", "message": "SAML provider could not be saved"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"ok": true, "data": provider})
}

func validSAMLProviderInput(provider service.SAMLProviderConfig) bool {
	return provider.Name != "" && len(provider.Name) <= 64 && validSAMLMetadataURL(provider.IDPMetadataURL) && len(provider.IDPMetadataURL) <= 2048 && len(provider.SPEntityID) <= 2048 && len(provider.ACSURL) <= 2048 && (provider.SPEntityID == "" || validProviderURL(provider.SPEntityID)) && (provider.ACSURL == "" || validProviderURL(provider.ACSURL)) && ((provider.SPCertPath == "") == (provider.SPKeyPath == ""))
}

func (h *AdminAuthSettingsHandler) SAMLProviderMetadataAPI(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "provider")
	provider, err := service.NewSAMLService().Provider(name)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]interface{}{"ok": false, "error": "NOT_FOUND", "message": "SAML provider not found"})
		return
	}
	sp, err := service.NewSAMLService().ServiceProvider(r.Context(), provider, util.BuildURL(r, "/"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{"ok": false, "error": "VALIDATION_FAILED", "message": "SAML service provider metadata could not be built"})
		return
	}
	metadata, err := service.MarshalSAMLMetadata(sp)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"ok": false, "error": "INTERNAL_ERROR", "message": "SAML metadata could not be generated"})
		return
	}
	w.Header().Set("Content-Type", "application/samlmetadata+xml; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="`+name+`-metadata.xml"`)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(metadata)
}

func (h *AdminAuthSettingsHandler) DeleteSAMLProvider(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "provider")
	settings := readAuthSettings(h.ConfigPath)
	providers := make([]service.SAMLProviderConfig, 0, len(settings.SAMLProviders))
	found := false
	for _, provider := range settings.SAMLProviders {
		if provider.Name == name {
			found = true
			continue
		}
		providers = append(providers, provider)
	}
	if !found {
		if wantsFormSubmission(r) {
			redirectAdminForm(w, r, adminPagePath(r, "/security/auth/saml"), "flash.settings_saved", "flash.settings_save_failed", fmt.Errorf("SAML provider not found"))
			return
		}
		writeJSON(w, http.StatusNotFound, map[string]interface{}{"ok": false, "error": "NOT_FOUND", "message": "SAML provider not found"})
		return
	}
	if err := h.storeSAMLSettings(r.Context(), settings.SAMLEnabled, providers); err != nil {
		if wantsFormSubmission(r) {
			redirectAdminForm(w, r, adminPagePath(r, "/security/auth/saml"), "flash.settings_saved", "flash.settings_save_failed", err)
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"ok": false, "error": "INTERNAL_ERROR", "message": Translate(r, "errors.admin.auth_settings.failed_to_update_settings")})
		return
	}
	if err := revokeSAMLProviderState(r.Context(), name); err != nil {
		if wantsFormSubmission(r) {
			redirectAdminForm(w, r, adminPagePath(r, "/security/auth/saml"), "flash.settings_saved", "flash.settings_save_failed", err)
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"ok": false, "error": "INTERNAL_ERROR", "message": Translate(r, "errors.admin.auth_settings.failed_to_update_settings")})
		return
	}
	if wantsFormSubmission(r) {
		redirectAdminForm(w, r, adminPagePath(r, "/security/auth/saml"), "flash.settings_saved", "flash.settings_save_failed", nil)
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"ok": true, "data": map[string]interface{}{}})
}

func revokeSAMLProviderState(ctx context.Context, provider string) error {
	usersDB, serverDB := database.GetUsersStore(), database.GetServerStore()
	userRows, err := database.QueryContext(ctx, usersDB, database.TimeoutSimpleSelect, `SELECT session_id FROM user_saml_sessions WHERE provider_name=?`, provider)
	if err != nil {
		return err
	}
	var userSessionIDs []int64
	for userRows.Next() {
		var id int64
		if err := userRows.Scan(&id); err != nil {
			_ = userRows.Close()
			return err
		}
		userSessionIDs = append(userSessionIDs, id)
	}
	if err := userRows.Err(); err != nil {
		_ = userRows.Close()
		return err
	}
	if err := userRows.Close(); err != nil {
		return err
	}
	adminRows, err := database.QueryContext(ctx, serverDB, database.TimeoutSimpleSelect, `SELECT session_id FROM admin_saml_sessions WHERE provider_name=?`, provider)
	if err != nil {
		return err
	}
	var adminSessionIDs []string
	for adminRows.Next() {
		var id string
		if err := adminRows.Scan(&id); err != nil {
			_ = adminRows.Close()
			return err
		}
		adminSessionIDs = append(adminSessionIDs, id)
	}
	if err := adminRows.Err(); err != nil {
		_ = adminRows.Close()
		return err
	}
	if err := adminRows.Close(); err != nil {
		return err
	}
	for _, id := range userSessionIDs {
		if _, err := database.ExecContext(ctx, usersDB, database.TimeoutWrite, `DELETE FROM user_sessions WHERE id=?`, id); err != nil {
			return err
		}
	}
	for _, id := range adminSessionIDs {
		if _, err := database.ExecContext(ctx, serverDB, database.TimeoutWrite, `DELETE FROM admin_sessions WHERE id=?`, id); err != nil {
			return err
		}
	}
	if _, err := database.ExecContext(ctx, usersDB, database.TimeoutWrite, `DELETE FROM user_saml_mappings WHERE provider_name=?`, provider); err != nil {
		return err
	}
	if _, err := database.ExecContext(ctx, usersDB, database.TimeoutWrite, `DELETE FROM admin_saml_mappings WHERE provider_name=?`, provider); err != nil {
		return err
	}
	if _, err := database.ExecContext(ctx, usersDB, database.TimeoutWrite, `DELETE FROM user_saml_sessions WHERE provider_name=?`, provider); err != nil {
		return err
	}
	if _, err := database.ExecContext(ctx, serverDB, database.TimeoutWrite, `DELETE FROM admin_saml_sessions WHERE provider_name=?`, provider); err != nil {
		return err
	}
	if _, err := database.ExecContext(ctx, serverDB, database.TimeoutWrite, `DELETE FROM saml_logout_requests WHERE provider_name=?`, provider); err != nil {
		return err
	}
	if _, err := database.ExecContext(ctx, serverDB, database.TimeoutWrite, `DELETE FROM saml_api_requests WHERE provider_name=?`, provider); err != nil {
		return err
	}
	prefix := "server.auth.saml.providers." + provider + "."
	if err := config.DeleteSecret(prefix + "sp_private_key"); err != nil {
		return err
	}
	if err := config.DeleteSecret(prefix + "sp_certificate"); err != nil {
		return err
	}
	return nil
}

func (h *AdminAuthSettingsHandler) TestSAMLProvider(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "provider")
	samlService := service.NewSAMLService()
	provider, err := samlService.Provider(name)
	if err == nil {
		var sp *saml.ServiceProvider
		sp, err = samlService.ServiceProvider(r.Context(), provider, util.BuildURL(r, "/"))
		if err == nil {
			location := sp.GetSSOBindingLocation(saml.HTTPRedirectBinding)
			if location == "" {
				err = fmt.Errorf("provider metadata has no Redirect SSO service")
			} else {
				var request *saml.AuthnRequest
				request, err = sp.MakeAuthenticationRequest(location, saml.HTTPRedirectBinding, saml.HTTPPostBinding)
				if err == nil {
					_, err = request.Redirect("saml-provider-test", sp)
				}
			}
		}
	}
	if err == nil {
		if wantsFormSubmission(r) {
			redirectAdminForm(w, r, adminPagePath(r, "/security/auth/saml"), "flash.settings_saved", "flash.settings_save_failed", nil)
			return
		}
		writeJSON(w, http.StatusOK, map[string]interface{}{"ok": true, "data": map[string]interface{}{"provider": provider.Name, "metadata": "valid", "authn_request": "valid"}})
		return
	}
	if wantsFormSubmission(r) {
		redirectAdminForm(w, r, adminPagePath(r, "/security/auth/saml"), "flash.settings_saved", "flash.settings_save_failed", fmt.Errorf("SAML provider validation failed"))
		return
	}
	writeJSON(w, http.StatusBadRequest, map[string]interface{}{"ok": false, "error": "VALIDATION_FAILED", "message": "SAML provider metadata could not be validated"})
}

func (h *AdminAuthSettingsHandler) RegenerateSAMLSPCertificate(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "provider")
	settings := readAuthSettings(h.ConfigPath)
	for _, provider := range settings.SAMLProviders {
		if provider.Name != name {
			continue
		}
		if strings.TrimSpace(provider.SPCertPath) != "" || strings.TrimSpace(provider.SPKeyPath) != "" {
			if wantsFormSubmission(r) {
				redirectAdminForm(w, r, adminPagePath(r, "/security/auth/saml"), "flash.settings_saved", "flash.settings_save_failed", fmt.Errorf("operator-supplied SAML certificates must be rotated at their configured file paths"))
				return
			}
			writeJSON(w, http.StatusConflict, map[string]interface{}{"ok": false, "error": "CONFLICT", "message": "Operator-supplied SAML certificates must be rotated at their configured file paths"})
			return
		}
		if err := service.GenerateSAMLSPKeyPair(name, provider.CertExpiryDays); err != nil {
			if wantsFormSubmission(r) {
				redirectAdminForm(w, r, adminPagePath(r, "/security/auth/saml"), "flash.settings_saved", "flash.settings_save_failed", err)
				return
			}
			writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"ok": false, "error": "INTERNAL_ERROR", "message": "SAML signing certificate could not be regenerated"})
			return
		}
		if wantsFormSubmission(r) {
			redirectAdminForm(w, r, adminPagePath(r, "/security/auth/saml"), "flash.settings_saved", "flash.settings_save_failed", nil)
			return
		}
		writeJSON(w, http.StatusOK, map[string]interface{}{"ok": true, "data": map[string]interface{}{}})
		return
	}
	writeJSON(w, http.StatusNotFound, map[string]interface{}{"ok": false, "error": "NOT_FOUND", "message": "SAML provider not found"})
}

func (h *AdminAuthSettingsHandler) storeSAMLSettings(ctx context.Context, enabled bool, providers []service.SAMLProviderConfig) error {
	if err := validateAuthSettingsPath(h.ConfigPath); err != nil {
		return err
	}
	if err := config.WriteSetting("server.auth.saml.enabled", enabled); err != nil {
		return err
	}
	if err := config.WriteSetting("server.auth.saml.providers", providers); err != nil {
		return err
	}
	model := &models.SettingsModel{Store: database.GetServerStore()}
	if err := model.SetBool("server.auth.saml.enabled", enabled); err != nil {
		return err
	}
	return model.SetJSON("server.auth.saml.providers", providers)
}
