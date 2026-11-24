package handler

import (
	"fmt"
	"net/http"
	"net/mail"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/webappsgo/wthr/src/config"
	"github.com/webappsgo/wthr/src/server/middleware"
	"github.com/webappsgo/wthr/src/server/service"
	"github.com/webappsgo/wthr/src/util"
)

var contactWebhookTransports = []string{"telegram", "discord", "slack", "mattermost", "pushover", "gotify", "generic"}

type contactWebhookFormRole struct {
	Key        string
	TitleKey   string
	Email      string
	Transports []contactWebhookFormTransport
}

type contactWebhookFormTransport struct {
	Name string
	URL  string
}

// ShowAdminContactNotifications renders private role-based contact delivery settings.
func ShowAdminContactNotifications(w http.ResponseWriter, r *http.Request) {
	cfg := config.GetGlobalConfig()
	if cfg == nil {
		http.Error(w, Translate(r, "errors.internal"), http.StatusInternalServerError)
		return
	}
	roles := []contactWebhookFormRole{
		{Key: "admin", TitleKey: "admin.notify.admin_role", Email: cfg.Server.Contact.Admin.Email, Transports: contactWebhookTransportsFor(cfg.Server.Contact.Admin)},
		{Key: "security", TitleKey: "admin.notify.security_role", Email: cfg.Server.Contact.Security.Email, Transports: contactWebhookTransportsFor(cfg.Server.Contact.Security)},
		{Key: "abuse", TitleKey: "admin.notify.abuse_role", Email: cfg.Server.Contact.Abuse.Email, Transports: contactWebhookTransportsFor(cfg.Server.Contact.Abuse)},
		{Key: "general", TitleKey: "admin.notify.general_role", Email: cfg.Server.Contact.General.Email, Transports: contactWebhookTransportsFor(cfg.Server.Contact.General)},
	}
	middleware.RenderHTML(w, r, http.StatusOK, "admin/admin_notify.tmpl", AdminTemplateData(r, map[string]interface{}{
		"title": Translate(r, "admin.notify.title"), "page": "notify", "roles": roles, "flash": TakeFlash(w, r),
	}))
}

func contactWebhookTransportsFor(role config.ContactRoleConfig) []contactWebhookFormTransport {
	transports := make([]contactWebhookFormTransport, 0, len(contactWebhookTransports))
	for _, name := range contactWebhookTransports {
		transports = append(transports, contactWebhookFormTransport{Name: name, URL: role.Webhooks[name]})
	}
	sort.Slice(transports, func(i, j int) bool { return transports[i].Name < transports[j].Name })
	return transports
}

// SaveAdminContactNotifications updates one role and keeps endpoint URLs in secrets.yml.
func SaveAdminContactNotifications(w http.ResponseWriter, r *http.Request) {
	target := adminPagePath(r, "/notify")
	r.Body = http.MaxBytesReader(w, r.Body, 1<<16)
	if err := r.ParseForm(); err != nil {
		SetFlash(w, r, "error", "flash.settings_save_failed")
		http.Redirect(w, r, target, http.StatusSeeOther)
		return
	}
	roleName := strings.ToLower(strings.TrimSpace(r.PostFormValue("role")))
	if !validContactRole(roleName) {
		SetFlash(w, r, "error", "flash.settings_save_failed")
		http.Redirect(w, r, target, http.StatusSeeOther)
		return
	}
	current := config.GetGlobalConfig()
	if current == nil {
		SetFlash(w, r, "error", "flash.settings_save_failed")
		http.Redirect(w, r, target, http.StatusSeeOther)
		return
	}
	encoded, err := yaml.Marshal(current)
	var next config.AppConfig
	if err == nil {
		err = yaml.Unmarshal(encoded, &next)
	}
	if err != nil {
		SetFlash(w, r, "error", "flash.settings_save_failed")
		http.Redirect(w, r, target, http.StatusSeeOther)
		return
	}
	contactRole := contactRoleConfig(&next.Server.Contact, roleName)
	if contactRole == nil {
		SetFlash(w, r, "error", "flash.settings_save_failed")
		http.Redirect(w, r, target, http.StatusSeeOther)
		return
	}
	email := strings.TrimSpace(r.PostFormValue("email"))
	if email != "" {
		parsed, parseErr := mail.ParseAddress(email)
		if parseErr != nil || parsed.Address != email || strings.ContainsAny(email, "\r\n") {
			SetFlash(w, r, "error", "flash.settings_save_failed")
			http.Redirect(w, r, target, http.StatusSeeOther)
			return
		}
	}
	contactRole.Email = email
	if contactRole.Webhooks == nil {
		contactRole.Webhooks = make(map[string]string)
	}
	var removed []string
	for _, transport := range contactWebhookTransports {
		if !r.PostForm.Has("webhook_" + transport) {
			continue
		}
		endpoint := strings.TrimSpace(r.PostFormValue("webhook_" + transport))
		if err := service.ValidateContactWebhookURL(endpoint); err != nil {
			SetFlash(w, r, "error", "flash.settings_save_failed")
			http.Redirect(w, r, target, http.StatusSeeOther)
			return
		}
		urlKey := fmt.Sprintf("server.contact.%s.webhooks.%s", roleName, transport)
		secretKey := urlKey + "_secret"
		if endpoint == "" {
			delete(contactRole.Webhooks, transport)
			removed = append(removed, urlKey, secretKey)
			continue
		}
		if err := config.WriteSecret(urlKey, endpoint); err != nil {
			SetFlash(w, r, "error", "flash.settings_save_failed")
			http.Redirect(w, r, target, http.StatusSeeOther)
			return
		}
		contactRole.Webhooks[transport] = endpoint
	}
	if err := config.SaveConfig(&next); err != nil {
		SetFlash(w, r, "error", "flash.settings_save_failed")
		http.Redirect(w, r, target, http.StatusSeeOther)
		return
	}
	for _, key := range removed {
		if err := config.DeleteSecret(key); err != nil {
			SetFlash(w, r, "error", "flash.settings_save_failed")
			http.Redirect(w, r, target, http.StatusSeeOther)
			return
		}
	}
	if err := config.EnsureContactWebhookSecrets(&next); err != nil {
		SetFlash(w, r, "error", "flash.settings_save_failed")
		http.Redirect(w, r, target, http.StatusSeeOther)
		return
	}
	reloaded, err := config.LoadConfig()
	if err != nil {
		SetFlash(w, r, "error", "flash.settings_save_failed")
		http.Redirect(w, r, target, http.StatusSeeOther)
		return
	}
	config.SetGlobalConfig(reloaded)
	SetFlash(w, r, "success", "flash.settings_saved")
	http.Redirect(w, r, target, http.StatusSeeOther)
}

// TestAdminContactWebhooks sends a sample event through every configured transport for a role.
func TestAdminContactWebhooks(w http.ResponseWriter, r *http.Request) {
	role := strings.ToLower(strings.TrimSpace(r.FormValue("role")))
	cfg := config.GetGlobalConfig()
	if cfg == nil || !validContactRole(role) {
		SetFlash(w, r, "error", "flash.settings_save_failed")
		http.Redirect(w, r, adminPagePath(r, "/notify"), http.StatusSeeOther)
		return
	}
	message := service.ContactWebhookMessage{
		Event: "notify.webhook_test", Subject: Translate(r, "admin.notify.test_action"),
		Body: Translate(r, "admin.notify.test_action"), Severity: "info",
		ProjectName: "wthr", ProjectVersion: Version, AppURL: util.BuildURL(r, "/"),
	}
	count, err := service.DispatchContactWebhooks(r.Context(), cfg.Server.Contact, role, message)
	if err != nil || count == 0 {
		SetFlash(w, r, "error", "flash.settings_save_failed")
	} else {
		SetFlash(w, r, "success", "flash.settings_saved")
	}
	http.Redirect(w, r, adminPagePath(r, "/notify"), http.StatusSeeOther)
}

func validContactRole(role string) bool {
	switch role {
	case "admin", "security", "abuse", "general":
		return true
	default:
		return false
	}
}

func contactRoleConfig(contact *config.ContactConfig, role string) *config.ContactRoleConfig {
	switch role {
	case "admin":
		return &contact.Admin
	case "security":
		return &contact.Security
	case "abuse":
		return &contact.Abuse
	case "general":
		return &contact.General
	default:
		return nil
	}
}
