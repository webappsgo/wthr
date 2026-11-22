package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/webappsgo/wthr/src/config"
	"github.com/webappsgo/wthr/src/database"
	"github.com/webappsgo/wthr/src/server/middleware"
	models "github.com/webappsgo/wthr/src/server/model"
	"github.com/webappsgo/wthr/src/server/service"
	"gopkg.in/yaml.v3"
)

// AdminAuthSettingsHandler handles authentication settings
type AdminAuthSettingsHandler struct {
	ConfigPath string
}

// ShowAuthSettings displays authentication settings page
func (h *AdminAuthSettingsHandler) ShowAuthSettings(w http.ResponseWriter, r *http.Request) {
	settings := readAuthSettings(h.ConfigPath)
	providers := settings.OIDCProviders
	providers = append(providers, OIDCProvider{})
	samlProviders := make([]samlProviderForm, 0, len(settings.SAMLProviders)+1)
	for _, provider := range settings.SAMLProviders {
		form := makeSAMLProviderForm(provider)
		if cert, err := service.GetSAMLSPCertificateInfoForConfig(&provider); err == nil {
			form.CertificateFingerprint = cert.Fingerprint
			form.CertificateExpiry = cert.ExpiresAt.UTC().Format(time.RFC3339)
		}
		samlProviders = append(samlProviders, form)
	}
	samlProviders = append(samlProviders, makeSAMLProviderForm(service.SAMLProviderConfig{}))
	data := map[string]interface{}{
		"title":           "Authentication Settings",
		"OIDCEnabled":     settings.OIDCEnabled,
		"OIDCProviders":   providers,
		"SAMLEnabled":     settings.SAMLEnabled,
		"SAMLProviders":   samlProviders,
		"LDAPEnabled":     settings.LDAPEnabled,
		"LDAPServer":      settings.LDAPServer,
		"LDAPPort":        settings.LDAPPort,
		"LDAPBindDN":      settings.LDAPBindDN,
		"LDAPBaseDN":      settings.LDAPBaseDN,
		"LDAPUserFilter":  settings.LDAPUserFilter,
		"TOTPEnabled":     settings.TOTPEnabled,
		"TOTPIssuer":      settings.TOTPIssuer,
		"TOTPDigits":      settings.TOTPDigits,
		"TOTPPeriod":      settings.TOTPPeriod,
		"PasskeysEnabled": settings.PasskeysEnabled,
		"PasskeysRPID":    settings.PasskeysRPID,
		"PasskeysRPName":  settings.PasskeysRPName,
		"flash":           TakeFlash(w, r),
	}
	middleware.RenderHTML(w, r, http.StatusOK, "admin/admin_auth_settings.tmpl", AdminTemplateData(r, data))
}

// UpdateAuthSettings updates authentication settings in server.yml
func (h *AdminAuthSettingsHandler) UpdateAuthSettings(w http.ResponseWriter, r *http.Request) {
	var req struct {
		OIDCEnabled      bool                         `json:"oidc_enabled"`
		OIDCProviders    []OIDCProvider               `json:"oidc_providers"`
		SAMLEnabled      bool                         `json:"saml_enabled"`
		SAMLProviders    []service.SAMLProviderConfig `json:"saml_providers"`
		LDAPEnabled      bool                         `json:"ldap_enabled"`
		LDAPServer       string                       `json:"ldap_server"`
		LDAPPort         int                          `json:"ldap_port"`
		LDAPBindDN       string                       `json:"ldap_bind_dn"`
		LDAPBindPassword string                       `json:"ldap_bind_password"`
		LDAPBaseDN       string                       `json:"ldap_base_dn"`
		LDAPUserFilter   string                       `json:"ldap_user_filter"`
		TOTPEnabled      bool                         `json:"totp_enabled"`
		TOTPIssuer       string                       `json:"totp_issuer"`
		TOTPDigits       int                          `json:"totp_digits"`
		TOTPPeriod       int                          `json:"totp_period"`
		PasskeysEnabled  bool                         `json:"passkeys_enabled"`
		PasskeysRPID     string                       `json:"passkeys_rp_id"`
		PasskeysRPName   string                       `json:"passkeys_rp_name"`
	}

	if wantsFormSubmission(r) {
		if err := r.ParseForm(); err != nil {
			redirectAdminForm(w, r, adminPagePath(r, "/security/auth"), "flash.settings_saved", "flash.settings_save_failed", err)
			return
		}
		req.OIDCEnabled = r.PostForm.Get("oidc_enabled") != ""
		req.LDAPEnabled = r.PostForm.Get("ldap_enabled") != ""
		req.LDAPServer = r.PostForm.Get("ldap_server")
		req.LDAPBindDN = r.PostForm.Get("ldap_bind_dn")
		req.LDAPBindPassword = r.PostForm.Get("ldap_bind_password")
		req.LDAPBaseDN = r.PostForm.Get("ldap_base_dn")
		req.LDAPUserFilter = r.PostForm.Get("ldap_user_filter")
		req.TOTPEnabled = r.PostForm.Get("totp_enabled") != ""
		req.TOTPIssuer = r.PostForm.Get("totp_issuer")
		req.PasskeysEnabled = r.PostForm.Get("passkeys_enabled") != ""
		req.PasskeysRPID = r.PostForm.Get("passkeys_rp_id")
		req.PasskeysRPName = r.PostForm.Get("passkeys_rp_name")
		var err error
		req.LDAPPort, err = parseAuthFormInt(r.PostForm.Get("ldap_port"), 389)
		if err == nil {
			req.TOTPDigits, err = parseAuthFormInt(r.PostForm.Get("totp_digits"), 6)
		}
		if err == nil {
			req.TOTPPeriod, err = parseAuthFormInt(r.PostForm.Get("totp_period"), 30)
		}
		if err != nil || req.LDAPPort < 1 || req.LDAPPort > 65535 || req.TOTPDigits < 6 || req.TOTPDigits > 8 || req.TOTPPeriod < 15 || req.TOTPPeriod > 120 {
			redirectAdminForm(w, r, adminPagePath(r, "/security/auth"), "flash.settings_saved", "flash.settings_save_failed", fmt.Errorf("invalid authentication setting"))
			return
		}
		req.OIDCProviders = make([]OIDCProvider, 0, len(r.PostForm["oidc_provider_name"]))
		req.SAMLEnabled = r.PostForm.Get("saml_enabled") != ""
		removeSAML := make(map[string]bool)
		for _, name := range r.PostForm["saml_provider_remove"] {
			removeSAML[name] = true
		}
		for i, name := range r.PostForm["saml_provider_name"] {
			autoRegister := false
			for _, selected := range r.PostForm["saml_provider_auto_register"] {
				if selected == name {
					autoRegister = true
				}
			}
			if name == "" {
				for _, selected := range r.PostForm["saml_provider_auto_register"] {
					if selected == "true" {
						autoRegister = true
					}
				}
			}
			certPath := strings.TrimSpace(formValueAt(r, "saml_provider_certificate_path", i))
			keyPath := strings.TrimSpace(formValueAt(r, "saml_provider_private_key_path", i))
			p := service.SAMLProviderConfig{Name: strings.ToLower(strings.TrimSpace(name)), DisplayName: strings.TrimSpace(formValueAt(r, "saml_provider_display_name", i)), IDPMetadataURL: strings.TrimSpace(formValueAt(r, "saml_provider_metadata_url", i)), SPEntityID: strings.TrimSpace(formValueAt(r, "saml_provider_entity_id", i)), ACSURL: strings.TrimSpace(formValueAt(r, "saml_provider_acs_url", i)), SPCertPath: certPath, SPKeyPath: keyPath, AutoRegister: autoRegister, AutoGenerateCert: certPath == "" && keyPath == "", WantAssertionsSigned: true, SignRequests: true, AttributeMapping: service.SAMLAttributeMap{Username: strings.TrimSpace(formValueAt(r, "saml_provider_username_attribute", i)), Email: strings.TrimSpace(formValueAt(r, "saml_provider_email_attribute", i)), Name: strings.TrimSpace(formValueAt(r, "saml_provider_name_attribute", i)), Groups: strings.TrimSpace(formValueAt(r, "saml_provider_groups_attribute", i)), ExternalID: strings.TrimSpace(formValueAt(r, "saml_provider_external_id_attribute", i))}, UsernameResolution: service.SAMLUsernameConfig{Mode: "prompt_on_first_login", AllowCustomOnFirstLogin: true}}
			p.AdminGroups = splitAuthList(formValueAt(r, "saml_provider_admin_groups", i))
			p.RoleMapping = map[string][]string{"admin": splitAuthList(formValueAt(r, "saml_provider_role_admin", i)), "moderator": splitAuthList(formValueAt(r, "saml_provider_role_moderator", i)), "user": splitAuthList(formValueAt(r, "saml_provider_role_user", i))}
			if removeSAML[p.Name] {
				continue
			}
			if p.Name == "" && p.IDPMetadataURL == "" && p.SPEntityID == "" && p.ACSURL == "" {
				continue
			}
			if p.Name == "" || p.IDPMetadataURL == "" || !validOIDCProviderName(p.Name) || !validSAMLMetadataURL(p.IDPMetadataURL) || len(p.IDPMetadataURL) > 2048 || len(p.SPEntityID) > 2048 || len(p.ACSURL) > 2048 || (p.SPEntityID != "" && !validProviderURL(p.SPEntityID)) || (p.ACSURL != "" && !validProviderURL(p.ACSURL)) || ((p.SPCertPath == "") != (p.SPKeyPath == "")) {
				redirectAdminForm(w, r, adminPagePath(r, "/security/auth"), "flash.settings_saved", "flash.settings_save_failed", fmt.Errorf("each SAML provider needs a valid name and HTTPS IdP metadata URL"))
				return
			}
			req.SAMLProviders = append(req.SAMLProviders, p)
		}
		remove := make(map[string]bool)
		for _, name := range r.PostForm["oidc_provider_remove"] {
			remove[name] = true
		}
		for i, name := range r.PostForm["oidc_provider_name"] {
			provider := OIDCProvider{Name: name}
			provider.ClientID = formValueAt(r, "oidc_provider_client_id", i)
			provider.ClientSecret = formValueAt(r, "oidc_provider_client_secret", i)
			provider.IssuerURL = formValueAt(r, "oidc_provider_issuer_url", i)
			provider.RedirectURL = formValueAt(r, "oidc_provider_redirect_url", i)
			if remove[name] {
				continue
			}
			if provider.Name == "" && provider.ClientID == "" && provider.IssuerURL == "" && provider.RedirectURL == "" {
				continue
			}
			if provider.Name == "" || provider.ClientID == "" || provider.IssuerURL == "" || provider.RedirectURL == "" {
				redirectAdminForm(w, r, adminPagePath(r, "/security/auth"), "flash.settings_saved", "flash.settings_save_failed", fmt.Errorf("each OIDC provider needs a name, client ID, issuer, and redirect URL"))
				return
			}
			req.OIDCProviders = append(req.OIDCProviders, provider)
		}
	} else if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{"ok": false, "error": "VALIDATION_FAILED", "message": Translate(r, "errors.admin.auth_settings.invalid_request")})
		return
	}
	if !wantsFormSubmission(r) && req.SAMLProviders == nil {
		currentSAML := readAuthSettings(h.ConfigPath)
		req.SAMLProviders = currentSAML.SAMLProviders
		req.SAMLEnabled = currentSAML.SAMLEnabled
	}
	if len(req.SAMLProviders) > 20 {
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{"ok": false, "error": "VALIDATION_FAILED", "message": Translate(r, "errors.admin.auth_settings.invalid_request")})
		return
	}
	seenSAML := make(map[string]bool)
	for i := range req.SAMLProviders {
		p := &req.SAMLProviders[i]
		p.Name = strings.ToLower(strings.TrimSpace(p.Name))
		if !validOIDCProviderName(p.Name) || seenSAML[p.Name] || len(p.IDPMetadataURL) > 2048 || !validSAMLMetadataURL(p.IDPMetadataURL) || ((p.SPCertPath == "") != (p.SPKeyPath == "")) {
			if wantsFormSubmission(r) {
				redirectAdminForm(w, r, adminPagePath(r, "/security/auth"), "flash.settings_saved", "flash.settings_save_failed", fmt.Errorf("invalid SAML provider configuration"))
				return
			}
			writeJSON(w, http.StatusBadRequest, map[string]interface{}{"ok": false, "error": "VALIDATION_FAILED", "message": Translate(r, "errors.admin.auth_settings.invalid_request")})
			return
		}
		if p.SPCertPath != "" {
			if _, _, err := service.LoadSAMLSPKeyPairFiles(p.SPCertPath, p.SPKeyPath); err != nil {
				if wantsFormSubmission(r) {
					redirectAdminForm(w, r, adminPagePath(r, "/security/auth"), "flash.settings_saved", "flash.settings_save_failed", fmt.Errorf("invalid SAML SP certificate or private-key files"))
					return
				}
				writeJSON(w, http.StatusBadRequest, map[string]interface{}{"ok": false, "error": "VALIDATION_FAILED", "message": Translate(r, "errors.admin.auth_settings.invalid_request")})
				return
			}
			p.AutoGenerateCert = false
		}
		seenSAML[p.Name] = true
	}
	for _, p := range req.SAMLProviders {
		if p.SPCertPath != "" {
			continue
		}
		key, keyErr := config.ReadSecret("server.auth.saml.providers." + p.Name + ".sp_private_key")
		if keyErr != nil {
			if wantsFormSubmission(r) {
				redirectAdminForm(w, r, adminPagePath(r, "/security/auth"), "flash.settings_saved", "flash.settings_save_failed", keyErr)
				return
			}
			writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"ok": false, "error": "INTERNAL_ERROR", "message": Translate(r, "errors.admin.auth_settings.failed_to_update_settings")})
			return
		}
		if key == "" && p.AutoGenerateCert {
			if err := service.GenerateSAMLSPKeyPair(p.Name, p.CertExpiryDays); err != nil {
				if wantsFormSubmission(r) {
					redirectAdminForm(w, r, adminPagePath(r, "/security/auth"), "flash.settings_saved", "flash.settings_save_failed", err)
					return
				}
				writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"ok": false, "error": "INTERNAL_ERROR", "message": Translate(r, "errors.admin.auth_settings.failed_to_update_settings")})
				return
			}
		}
	}

	providerCount := 0
	for _, provider := range req.OIDCProviders {
		if strings.TrimSpace(provider.Name) != "" || strings.TrimSpace(provider.ClientID) != "" || strings.TrimSpace(provider.ClientSecret) != "" || strings.TrimSpace(provider.IssuerURL) != "" || strings.TrimSpace(provider.RedirectURL) != "" {
			providerCount++
		}
	}
	if providerCount > 20 {
		if wantsFormSubmission(r) {
			redirectAdminForm(w, r, adminPagePath(r, "/security/auth"), "flash.settings_saved", "flash.settings_save_failed", fmt.Errorf("at most 20 OIDC providers are allowed"))
			return
		}
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{"ok": false, "error": "VALIDATION_FAILED", "message": Translate(r, "errors.admin.auth_settings.invalid_request")})
		return
	}
	current := readAuthSettings(h.ConfigPath)
	currentProviders := make(map[string]OIDCProvider, len(current.OIDCProviders))
	for _, provider := range current.OIDCProviders {
		currentProviders[provider.Name] = provider
	}
	secretWrites := make(map[string]string)
	seenProviderNames := make(map[string]bool, len(req.OIDCProviders))
	providers := make([]OIDCProvider, 0, len(req.OIDCProviders))
	for i := range req.OIDCProviders {
		provider := &req.OIDCProviders[i]
		provider.Name = strings.ToLower(strings.TrimSpace(provider.Name))
		provider.ClientID = strings.TrimSpace(provider.ClientID)
		provider.IssuerURL = strings.TrimSpace(provider.IssuerURL)
		provider.RedirectURL = strings.TrimSpace(provider.RedirectURL)
		if provider.Name == "" && provider.ClientID == "" && provider.IssuerURL == "" && provider.RedirectURL == "" && provider.ClientSecret == "" {
			continue
		}
		if !validOIDCProviderName(provider.Name) || seenProviderNames[provider.Name] || provider.ClientID == "" || len(provider.ClientID) > 512 || len(provider.IssuerURL) > 2048 || len(provider.RedirectURL) > 2048 || !validProviderURL(provider.IssuerURL) || !validProviderURL(provider.RedirectURL) {
			if wantsFormSubmission(r) {
				redirectAdminForm(w, r, adminPagePath(r, "/security/auth"), "flash.settings_saved", "flash.settings_save_failed", fmt.Errorf("invalid OIDC provider name or configuration"))
				return
			}
			writeJSON(w, http.StatusBadRequest, map[string]interface{}{"ok": false, "error": "VALIDATION_FAILED", "message": Translate(r, "errors.admin.auth_settings.invalid_request")})
			return
		}
		seenProviderNames[provider.Name] = true
		secretKey := "server.auth.oidc.providers." + provider.Name + ".client_secret"
		secret := provider.ClientSecret
		if len(secret) > 4096 {
			if wantsFormSubmission(r) {
				redirectAdminForm(w, r, adminPagePath(r, "/security/auth"), "flash.settings_saved", "flash.settings_save_failed", fmt.Errorf("OIDC client secret is too long"))
				return
			}
			writeJSON(w, http.StatusBadRequest, map[string]interface{}{"ok": false, "error": "VALIDATION_FAILED", "message": Translate(r, "errors.admin.auth_settings.invalid_request")})
			return
		}
		if secret == "" {
			var err error
			secret, err = config.ReadSecret(secretKey)
			if err != nil {
				if wantsFormSubmission(r) {
					redirectAdminForm(w, r, adminPagePath(r, "/security/auth"), "flash.settings_saved", "flash.settings_save_failed", err)
					return
				}
				writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"ok": false, "error": "INTERNAL_ERROR", "message": Translate(r, "errors.admin.auth_settings.failed_to_update_settings")})
				return
			}
		}
		if secret == "" {
			secret = currentProviders[provider.Name].ClientSecret
		}
		if secret == "" {
			if wantsFormSubmission(r) {
				redirectAdminForm(w, r, adminPagePath(r, "/security/auth"), "flash.settings_saved", "flash.settings_save_failed", fmt.Errorf("client secret is required for a new OIDC provider"))
				return
			}
			writeJSON(w, http.StatusBadRequest, map[string]interface{}{"ok": false, "error": "VALIDATION_FAILED", "message": Translate(r, "errors.admin.auth_settings.invalid_request")})
			return
		}
		secretWrites[secretKey] = secret
		provider.ClientSecret = ""
		providers = append(providers, *provider)
	}
	req.OIDCProviders = providers
	req.LDAPServer = strings.TrimSpace(req.LDAPServer)
	req.LDAPBindDN = strings.TrimSpace(req.LDAPBindDN)
	req.LDAPBaseDN = strings.TrimSpace(req.LDAPBaseDN)
	req.LDAPUserFilter = strings.TrimSpace(req.LDAPUserFilter)
	ldapSecret := strings.TrimSpace(req.LDAPBindPassword)
	if ldapSecret == "" {
		var err error
		ldapSecret, err = config.ReadSecret("server.auth.ldap.bind_password")
		if err != nil {
			if wantsFormSubmission(r) {
				redirectAdminForm(w, r, adminPagePath(r, "/security/auth"), "flash.settings_saved", "flash.settings_save_failed", err)
				return
			}
			writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"ok": false, "error": "INTERNAL_ERROR", "message": Translate(r, "errors.admin.auth_settings.failed_to_update_settings")})
			return
		}
	}
	if ldapSecret == "" {
		ldapSecret = current.LDAPBindPassword
	}
	if req.LDAPPort == 0 {
		req.LDAPPort = 389
	}
	if req.LDAPEnabled && (req.LDAPServer == "" || req.LDAPBaseDN == "" || req.LDAPBindDN == "" || ldapSecret == "") {
		if wantsFormSubmission(r) {
			redirectAdminForm(w, r, adminPagePath(r, "/security/auth"), "flash.settings_saved", "flash.settings_save_failed", fmt.Errorf("LDAP requires a server, bind DN, base DN, and bind password"))
			return
		}
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{"ok": false, "error": "VALIDATION_FAILED", "message": Translate(r, "errors.admin.auth_settings.invalid_request")})
		return
	}
	if len(req.LDAPServer) > 255 || strings.ContainsAny(req.LDAPServer, " \t\r\n/") || len(req.LDAPBindDN) > 2048 || strings.ContainsAny(req.LDAPBindDN+req.LDAPBaseDN, "\r\n") || len(req.LDAPBaseDN) > 2048 || len(ldapSecret) > 4096 {
		if wantsFormSubmission(r) {
			redirectAdminForm(w, r, adminPagePath(r, "/security/auth"), "flash.settings_saved", "flash.settings_save_failed", fmt.Errorf("invalid LDAP connection settings"))
			return
		}
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{"ok": false, "error": "VALIDATION_FAILED", "message": Translate(r, "errors.admin.auth_settings.invalid_request")})
		return
	}
	if req.LDAPPort < 1 || req.LDAPPort > 65535 {
		if wantsFormSubmission(r) {
			redirectAdminForm(w, r, adminPagePath(r, "/security/auth"), "flash.settings_saved", "flash.settings_save_failed", fmt.Errorf("LDAP port must be between 1 and 65535"))
			return
		}
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{"ok": false, "error": "VALIDATION_FAILED", "message": Translate(r, "errors.admin.auth_settings.invalid_request")})
		return
	}
	if req.LDAPUserFilter == "" {
		req.LDAPUserFilter = "(uid=%s)"
	}
	filterRemainder := strings.Replace(req.LDAPUserFilter, "%s", "", 1)
	if len(req.LDAPUserFilter) > 512 || strings.ContainsAny(req.LDAPUserFilter, "\r\n") || strings.Count(req.LDAPUserFilter, "%s") != 1 || strings.Contains(filterRemainder, "%") {
		if wantsFormSubmission(r) {
			redirectAdminForm(w, r, adminPagePath(r, "/security/auth"), "flash.settings_saved", "flash.settings_save_failed", fmt.Errorf("LDAP user filter must contain exactly one %%s placeholder"))
			return
		}
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{"ok": false, "error": "VALIDATION_FAILED", "message": Translate(r, "errors.admin.auth_settings.invalid_request")})
		return
	}
	if ldapSecret != "" {
		secretWrites["server.auth.ldap.bind_password"] = ldapSecret
	}
	if err := validateAuthSettingsPath(h.ConfigPath); err != nil {
		if wantsFormSubmission(r) {
			redirectAdminForm(w, r, adminPagePath(r, "/security/auth"), "flash.settings_saved", "flash.settings_save_failed", err)
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"ok": false, "error": "INTERNAL_ERROR", "message": Translate(r, "errors.admin.auth_settings.failed_to_update_settings")})
		return
	}
	for key, value := range secretWrites {
		if err := config.WriteSecret(key, value); err != nil {
			if wantsFormSubmission(r) {
				redirectAdminForm(w, r, adminPagePath(r, "/security/auth"), "flash.settings_saved", "flash.settings_save_failed", err)
				return
			}
			writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"ok": false, "error": "INTERNAL_ERROR", "message": Translate(r, "errors.admin.auth_settings.failed_to_update_settings")})
			return
		}
	}

	updates := map[string]interface{}{
		"server.auth.saml.enabled":       req.SAMLEnabled,
		"server.auth.saml.providers":     req.SAMLProviders,
		"server.auth.oidc.enabled":       req.OIDCEnabled,
		"server.auth.oidc.providers":     req.OIDCProviders,
		"server.auth.ldap.enabled":       req.LDAPEnabled,
		"server.auth.ldap.server":        req.LDAPServer,
		"server.auth.ldap.port":          req.LDAPPort,
		"server.auth.ldap.bind_dn":       req.LDAPBindDN,
		"server.auth.ldap.bind_password": "",
		"server.auth.ldap.base_dn":       req.LDAPBaseDN,
		"server.auth.ldap.user_filter":   req.LDAPUserFilter,
		"server.auth.totp.enabled":       req.TOTPEnabled,
		"server.auth.totp.issuer":        req.TOTPIssuer,
		"server.auth.totp.digits":        req.TOTPDigits,
		"server.auth.totp.period":        req.TOTPPeriod,
		"server.auth.passkeys.enabled":   req.PasskeysEnabled,
		"server.auth.passkeys.rp_id":     req.PasskeysRPID,
		"server.auth.passkeys.rp_name":   req.PasskeysRPName,
	}

	delete(updates, "server.auth.ldap.bind_password")
	for key, value := range updates {
		if err := config.WriteSetting(key, value); err != nil {
			if wantsFormSubmission(r) {
				redirectAdminForm(w, r, adminPagePath(r, "/security/auth"), "flash.settings_saved", "flash.settings_save_failed", err)
				return
			}
			writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"ok": false, "error": "INTERNAL_ERROR", "message": Translate(r, "errors.admin.auth_settings.failed_to_update_settings")})
			return
		}
	}
	settingsModel := &models.SettingsModel{DB: database.GetServerDB()}
	settingsErr := settingsModel.SetBool("server.auth.saml.enabled", req.SAMLEnabled)
	if settingsErr == nil {
		settingsErr = settingsModel.SetJSON("server.auth.saml.providers", req.SAMLProviders)
	}
	if settingsErr != nil {
		if wantsFormSubmission(r) {
			redirectAdminForm(w, r, adminPagePath(r, "/security/auth"), "flash.settings_saved", "flash.settings_save_failed", settingsErr)
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"ok": false, "error": "INTERNAL_ERROR", "message": Translate(r, "errors.admin.auth_settings.failed_to_update_settings")})
		return
	}

	if wantsFormSubmission(r) {
		redirectAdminForm(w, r, adminPagePath(r, "/security/auth"), "flash.settings_saved", "flash.settings_save_failed", nil)
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"ok": true, "data": map[string]interface{}{}})
}

func validOIDCProviderName(name string) bool {
	if len(name) < 1 || len(name) > 48 || name[0] < 'a' || name[0] > 'z' {
		return false
	}
	for _, char := range name {
		switch {
		case char >= 'a' && char <= 'z', char >= '0' && char <= '9', char == '-', char == '_':
		default:
			return false
		}
	}
	return true
}

func validProviderURL(raw string) bool {
	parsed, err := url.ParseRequestURI(raw)
	return err == nil && parsed.Host != "" && parsed.User == nil && parsed.Fragment == "" && (parsed.Scheme == "https" || parsed.Scheme == "http")
}

func validSAMLMetadataURL(raw string) bool {
	parsed, err := url.ParseRequestURI(raw)
	return err == nil && parsed.Scheme == "https" && parsed.Host != "" && parsed.User == nil && parsed.Fragment == ""
}

func validateAuthSettingsPath(path string) error {
	if path == "" {
		return fmt.Errorf("authentication configuration path is not set")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read authentication configuration: %w", err)
	}
	var root map[string]interface{}
	if err := yaml.Unmarshal(data, &root); err != nil {
		return fmt.Errorf("parse authentication configuration: %w", err)
	}
	return nil
}

type OIDCProvider struct {
	Name         string `json:"name" yaml:"name"`
	ClientID     string `json:"client_id" yaml:"client_id"`
	ClientSecret string `json:"client_secret" yaml:"client_secret,omitempty"`
	IssuerURL    string `json:"issuer_url" yaml:"issuer_url"`
	RedirectURL  string `json:"redirect_url" yaml:"redirect_url"`
}

type samlProviderForm struct {
	service.SAMLProviderConfig
	AdminGroupsCSV         string
	RoleAdminCSV           string
	RoleModeratorCSV       string
	RoleUserCSV            string
	CertificateFingerprint string
	CertificateExpiry      string
}

func makeSAMLProviderForm(provider service.SAMLProviderConfig) samlProviderForm {
	return samlProviderForm{SAMLProviderConfig: provider, AdminGroupsCSV: strings.Join(provider.AdminGroups, ", "), RoleAdminCSV: strings.Join(provider.RoleMapping["admin"], ", "), RoleModeratorCSV: strings.Join(provider.RoleMapping["moderator"], ", "), RoleUserCSV: strings.Join(provider.RoleMapping["user"], ", ")}
}

type authSettingsPageConfig struct {
	OIDCEnabled      bool                         `yaml:"-"`
	OIDCProviders    []OIDCProvider               `yaml:"-"`
	SAMLEnabled      bool                         `yaml:"-"`
	SAMLProviders    []service.SAMLProviderConfig `yaml:"-"`
	LDAPEnabled      bool                         `yaml:"-"`
	LDAPServer       string                       `yaml:"-"`
	LDAPPort         int                          `yaml:"-"`
	LDAPBindDN       string                       `yaml:"-"`
	LDAPBindPassword string                       `yaml:"-"`
	LDAPBaseDN       string                       `yaml:"-"`
	LDAPUserFilter   string                       `yaml:"-"`
	TOTPEnabled      bool                         `yaml:"-"`
	TOTPIssuer       string                       `yaml:"-"`
	TOTPDigits       int                          `yaml:"-"`
	TOTPPeriod       int                          `yaml:"-"`
	PasskeysEnabled  bool                         `yaml:"-"`
	PasskeysRPID     string                       `yaml:"-"`
	PasskeysRPName   string                       `yaml:"-"`
}

func readAuthSettings(path string) authSettingsPageConfig {
	defaults := authSettingsPageConfig{LDAPPort: 389, LDAPUserFilter: "(uid=%s)", TOTPIssuer: "Weather", TOTPDigits: 6, TOTPPeriod: 30, PasskeysRPName: "Weather"}
	if path == "" {
		return defaults
	}
	if _, err := os.Stat(path); err != nil {
		return defaults
	}
	load := func(key string, target any) {
		value, exists, err := config.ReadSetting(key)
		if err != nil || !exists {
			return
		}
		encoded, err := json.Marshal(value)
		if err == nil {
			_ = json.Unmarshal(encoded, target)
		}
	}
	load("server.auth.oidc.enabled", &defaults.OIDCEnabled)
	load("server.auth.oidc.providers", &defaults.OIDCProviders)
	load("server.auth.saml.enabled", &defaults.SAMLEnabled)
	load("server.auth.saml.providers", &defaults.SAMLProviders)
	load("server.auth.ldap.enabled", &defaults.LDAPEnabled)
	load("server.auth.ldap.server", &defaults.LDAPServer)
	load("server.auth.ldap.port", &defaults.LDAPPort)
	load("server.auth.ldap.bind_dn", &defaults.LDAPBindDN)
	load("server.auth.ldap.bind_password", &defaults.LDAPBindPassword)
	load("server.auth.ldap.base_dn", &defaults.LDAPBaseDN)
	load("server.auth.ldap.user_filter", &defaults.LDAPUserFilter)
	load("server.auth.totp.enabled", &defaults.TOTPEnabled)
	load("server.auth.totp.issuer", &defaults.TOTPIssuer)
	load("server.auth.totp.digits", &defaults.TOTPDigits)
	load("server.auth.totp.period", &defaults.TOTPPeriod)
	load("server.auth.passkeys.enabled", &defaults.PasskeysEnabled)
	load("server.auth.passkeys.rp_id", &defaults.PasskeysRPID)
	load("server.auth.passkeys.rp_name", &defaults.PasskeysRPName)
	return defaults
}

func parseAuthFormInt(value string, fallback int) (int, error) {
	if value == "" {
		return fallback, nil
	}
	return strconv.Atoi(value)
}

func formValueAt(r *http.Request, key string, index int) string {
	values := r.PostForm[key]
	if index >= len(values) {
		return ""
	}
	return values[index]
}

func splitAuthList(value string) []string {
	items := strings.Split(value, ",")
	result := make([]string, 0, len(items))
	for _, item := range items {
		item = strings.TrimSpace(item)
		if item != "" {
			result = append(result, item)
		}
	}
	return result
}
