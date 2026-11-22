package handler

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/netip"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/webappsgo/wthr/src/config"
	"github.com/webappsgo/wthr/src/database"
	"github.com/webappsgo/wthr/src/server/middleware"
)

type allowlistView struct {
	ID          string `json:"id"`
	CIDR        string `json:"cidr"`
	Description string `json:"description"`
}

// ShowFirewall renders the active allowlist as a usable admin page.
func ShowFirewall(w http.ResponseWriter, r *http.Request) {
	entries := currentAllowlistViews()
	blocks, err := readIPBlocks(r.Context(), database.GetServerDB(), false)
	if err != nil {
		InternalError(w, r, err.Error())
		return
	}
	middleware.RenderHTML(w, r, http.StatusOK, "admin/admin_firewall.tmpl", AdminTemplateData(r, map[string]interface{}{
		"Title":   Translate(r, "admin.firewall.title"),
		"Entries": entries,
		"Blocks":  blocks,
		"flash":   TakeFlash(w, r),
	}))
}

// ListAllowlist returns configured trusted IPs and CIDRs.
func ListAllowlist(w http.ResponseWriter, r *http.Request) {
	RespondData(w, r, currentAllowlistViews())
}

// AddAllowlist accepts a browser form or a JSON API request.
func AddAllowlist(w http.ResponseWriter, r *http.Request) {
	var cidr, description string
	if wantsFormSubmission(r) {
		if err := r.ParseForm(); err != nil {
			redirectAdminForm(w, r, adminPagePath(r, "/security/firewall"), "flash.settings_saved", "flash.settings_save_failed", err)
			return
		}
		cidr, description = r.PostForm.Get("cidr"), r.PostForm.Get("description")
	} else {
		var request struct {
			CIDR        string `json:"cidr"`
			Description string `json:"description"`
		}
		if !DecodeAndValidate(w, r, &request) {
			return
		}
		cidr, description = request.CIDR, request.Description
	}
	cidr = strings.TrimSpace(cidr)
	description = strings.TrimSpace(description)
	err := validateAllowlistEntry(cidr, description)
	if err == nil {
		cfg := config.GetGlobalConfig()
		if cfg == nil {
			err = fmt.Errorf("configuration is not initialized")
		} else {
			entries := append([]config.IPAllowlistEntry(nil), cfg.Server.Security.Allowlist...)
			for _, existing := range entries {
				if existing.CIDR == cidr {
					err = fmt.Errorf("IP or CIDR is already allowlisted")
					break
				}
			}
			if err == nil {
				entries = append(entries, config.IPAllowlistEntry{CIDR: cidr, Description: description})
				err = config.SaveIPAllowlistArea(entries)
				if err == nil {
					cfg.Server.Security.Allowlist = entries
					config.SetGlobalConfig(cfg)
					logSecurityAudit(r, "security.allowlist_added", cidr, "")
				}
			}
		}
	}
	if wantsFormSubmission(r) {
		redirectAdminForm(w, r, adminPagePath(r, "/security/firewall"), "flash.settings_saved", "flash.settings_save_failed", err)
		return
	}
	if err != nil {
		RespondError(w, r, http.StatusBadRequest, "ALLOWLIST_INVALID", err.Error())
		return
	}
	RespondCreated(w, r, "Allowlist entry added", allowlistID(cidr))
}

// RemoveAllowlist removes one entry by its stable identifier.
func RemoveAllowlist(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var err error
	cfg := config.GetGlobalConfig()
	if cfg == nil {
		err = fmt.Errorf("allowlist entry not found")
	} else {
		entries := append([]config.IPAllowlistEntry(nil), cfg.Server.Security.Allowlist...)
		found := -1
		for index, entry := range entries {
			if allowlistID(entry.CIDR) == id {
				found = index
				break
			}
		}
		if found < 0 {
			err = fmt.Errorf("allowlist entry not found")
		} else {
			removed := entries[found]
			entries = append(entries[:found], entries[found+1:]...)
			err = config.SaveIPAllowlistArea(entries)
			if err == nil {
				cfg.Server.Security.Allowlist = entries
				config.SetGlobalConfig(cfg)
				logSecurityAudit(r, "security.allowlist_removed", removed.CIDR, "")
			}
		}
	}
	if wantsFormSubmission(r) {
		redirectAdminForm(w, r, adminPagePath(r, "/security/firewall"), "flash.settings_saved", "flash.settings_save_failed", err)
		return
	}
	if err != nil {
		RespondError(w, r, http.StatusNotFound, "ALLOWLIST_NOT_FOUND", err.Error())
		return
	}
	RespondSuccess(w, r, "Allowlist entry removed")
}

// CheckAllowlist reports whether an IP is covered by the configured entries.
func CheckAllowlist(w http.ResponseWriter, r *http.Request) {
	ip := strings.TrimSpace(chi.URLParam(r, "ip"))
	if parsed, err := netip.ParseAddr(ip); err == nil {
		ip = parsed.String()
	} else {
		RespondError(w, r, http.StatusBadRequest, "INVALID_IP", "A valid IP address is required")
		return
	}
	RespondData(w, r, map[string]interface{}{"ip": ip, "allowlisted": config.IsIPAllowlisted(ip)})
}

func currentAllowlistViews() []allowlistView {
	cfg := config.GetGlobalConfig()
	if cfg == nil {
		return []allowlistView{}
	}
	entries := make([]allowlistView, 0, len(cfg.Server.Security.Allowlist))
	for _, entry := range cfg.Server.Security.Allowlist {
		entries = append(entries, allowlistView{ID: allowlistID(entry.CIDR), CIDR: entry.CIDR, Description: entry.Description})
	}
	return entries
}

func allowlistID(cidr string) string {
	digest := sha256.Sum256([]byte(cidr))
	return hex.EncodeToString(digest[:])
}

func validateAllowlistEntry(cidr, description string) error {
	if strings.TrimSpace(description) == "" || len(description) > 120 {
		return fmt.Errorf("description is required and must be 120 characters or fewer")
	}
	if prefix, err := netip.ParsePrefix(cidr); err == nil {
		if prefix != prefix.Masked() {
			return fmt.Errorf("CIDR must be a network prefix with host bits cleared")
		}
		return nil
	}
	if _, err := netip.ParseAddr(cidr); err != nil {
		return fmt.Errorf("a valid IP address or CIDR is required")
	}
	return nil
}
