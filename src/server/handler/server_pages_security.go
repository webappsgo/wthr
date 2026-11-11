package handler

import (
	"net/http"
	"strings"

	"github.com/webappsgo/wthr/src/config"
	"github.com/webappsgo/wthr/src/database"
	"github.com/webappsgo/wthr/src/server/model"
	"github.com/webappsgo/wthr/src/util"
)

// securityOverview builds the data behind /server/security and its JSON mirror: the contact
// channels in preference order, the security.txt expiry, the PGP key state and the policy and
// hall-of-fame links. Every URL is resolved per request.
func securityOverview(r *http.Request, cfg *config.AppConfig) map[string]interface{} {
	contacts := []map[string]string{{"type": "form", "url": util.BuildURL(r, "/server/contact")}}
	if cfg != nil {
		if email := strings.TrimSpace(cfg.Server.Contact.ResolveContactEmail("security")); email != "" {
			contacts = append(contacts, map[string]string{"type": "email", "address": email})
		}
	}
	expires := ""
	if db := database.GetServerDB(); db != nil {
		expires = (&model.SettingsModel{DB: db}).GetString("security.expires", "")
	}
	return map[string]interface{}{
		"contacts":    contacts,
		"expires":     expires,
		"pgp":         map[string]interface{}{"available": false, "url": "", "fingerprint": ""},
		"policy_url":  util.BuildURL(r, "/server/security/policy"),
		"thanks_url":  util.BuildURL(r, "/server/security/thanks"),
		"security_tx": util.BuildURL(r, "/.well-known/security.txt"),
	}
}

// securityPolicyText returns the operator-edited disclosure policy (Markdown), or "" for the default.
func securityPolicyText() string {
	if db := database.GetServerDB(); db != nil {
		return (&model.SettingsModel{DB: db}).GetString("security.policy", "")
	}
	return ""
}

// ShowSecurityPage renders the human-readable security overview.
// GET /server/security
func ShowSecurityPage(cfg *config.AppConfig) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		overview := securityOverview(r, cfg)
		data := map[string]interface{}{
			"page":     "security",
			"server":   pageServerData(r, map[string]interface{}{"Title": cfg.Server.Branding.Title}),
			"HostInfo": util.GetHostInfo(r),
			"Overview": overview,
		}
		NegotiateResponse(w, r, "page/security.tmpl", data)
	}
}

// GetSecurityAPI is the JSON mirror of /server/security.
// GET /api/{api_version}/server/security
func GetSecurityAPI(cfg *config.AppConfig) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		overview := securityOverview(r, cfg)
		delete(overview, "security_tx")
		RespondNegotiatedData(w, r, http.StatusOK, overview)
	}
}

// ShowSecurityPolicyPage renders the coordinated-disclosure policy.
// GET /server/security/policy
func ShowSecurityPolicyPage(cfg *config.AppConfig) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		data := map[string]interface{}{
			"page":     "security_policy",
			"server":   pageServerData(r, map[string]interface{}{"Title": cfg.Server.Branding.Title}),
			"HostInfo": util.GetHostInfo(r),
			"Policy":   securityPolicyText(),
		}
		NegotiateResponse(w, r, "page/security_policy.tmpl", data)
	}
}

// GetSecurityPolicyAPI is the JSON mirror of the disclosure policy.
// GET /api/{api_version}/server/security/policy
func GetSecurityPolicyAPI() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		RespondNegotiatedData(w, r, http.StatusOK, map[string]interface{}{"policy": securityPolicyText()})
	}
}

// ShowSecurityThanksPage renders the acknowledgments page.
// GET /server/security/thanks
func ShowSecurityThanksPage(cfg *config.AppConfig) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		data := map[string]interface{}{
			"page":     "security_thanks",
			"server":   pageServerData(r, map[string]interface{}{"Title": cfg.Server.Branding.Title}),
			"HostInfo": util.GetHostInfo(r),
			"Entries":  []map[string]string{},
		}
		NegotiateResponse(w, r, "page/security_thanks.tmpl", data)
	}
}

// GetSecurityThanksAPI is the JSON mirror of the acknowledgments page.
// GET /api/{api_version}/server/security/thanks
func GetSecurityThanksAPI() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		RespondNegotiatedData(w, r, http.StatusOK, map[string]interface{}{"entries": []map[string]string{}})
	}
}
