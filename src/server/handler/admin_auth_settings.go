package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/webappsgo/wthr/src/util"
)

// AdminAuthSettingsHandler handles authentication settings
type AdminAuthSettingsHandler struct {
	ConfigPath string
}

// ShowAuthSettings displays authentication settings page
func (h *AdminAuthSettingsHandler) ShowAuthSettings(c *gin.Context) {
	c.HTML(http.StatusOK, "admin/admin_auth_settings.tmpl", util.TemplateData(c, gin.H{
		"title": "Authentication Settings",
	}))
}

// UpdateAuthSettings updates authentication settings in server.yml
func (h *AdminAuthSettingsHandler) UpdateAuthSettings(c *gin.Context) {
	var req struct {
		OIDCEnabled      bool           `json:"oidc_enabled"`
		OIDCProviders    []OIDCProvider `json:"oidc_providers"`
		LDAPEnabled      bool           `json:"ldap_enabled"`
		LDAPServer       string         `json:"ldap_server"`
		LDAPPort         int            `json:"ldap_port"`
		LDAPBindDN       string         `json:"ldap_bind_dn"`
		LDAPBindPassword string         `json:"ldap_bind_password"`
		LDAPBaseDN       string         `json:"ldap_base_dn"`
		LDAPUserFilter   string         `json:"ldap_user_filter"`
		TOTPEnabled      bool           `json:"totp_enabled"`
		TOTPIssuer       string         `json:"totp_issuer"`
		TOTPDigits       int            `json:"totp_digits"`
		TOTPPeriod       int            `json:"totp_period"`
		PasskeysEnabled  bool           `json:"passkeys_enabled"`
		PasskeysRPID     string         `json:"passkeys_rp_id"`
		PasskeysRPName   string         `json:"passkeys_rp_name"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"ok": false, "error": "VALIDATION_FAILED", "message": err.Error()})
		return
	}

	updates := map[string]interface{}{
		"server.auth.oidc.enabled":       req.OIDCEnabled,
		"server.auth.oidc.providers":     req.OIDCProviders,
		"server.auth.ldap.enabled":       req.LDAPEnabled,
		"server.auth.ldap.server":        req.LDAPServer,
		"server.auth.ldap.port":          req.LDAPPort,
		"server.auth.ldap.bind_dn":       req.LDAPBindDN,
		"server.auth.ldap.bind_password": req.LDAPBindPassword,
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

	if err := util.UpdateYAMLConfig(h.ConfigPath, updates); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"ok": false, "error": "INTERNAL_ERROR", "message": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"ok": true, "data": gin.H{}})
}

type OIDCProvider struct {
	Name         string `json:"name" yaml:"name"`
	ClientID     string `json:"client_id" yaml:"client_id"`
	ClientSecret string `json:"client_secret" yaml:"client_secret"`
	IssuerURL    string `json:"issuer_url" yaml:"issuer_url"`
	RedirectURL  string `json:"redirect_url" yaml:"redirect_url"`
}
