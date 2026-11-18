package handler

import (
	"errors"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/webappsgo/wthr/src/config"
	"github.com/webappsgo/wthr/src/server/service"
	"github.com/webappsgo/wthr/src/util"
)

// dnsSettingsView is the admin view of the DNS-01 settings. Credential values never appear: each
// stored name is shown with the mask (AI.md PART 15 "Encryption at rest").
type dnsSettingsView struct {
	Providers   []string          `json:"providers"`
	Provider    string            `json:"provider"`
	Credentials map[string]string `json:"credentials"`
	ValidatedAt string            `json:"validatedAt"`
}

// dnsSettingsRequest is the body of a save or check call. A credential whose value is empty or the
// mask keeps the stored value.
type dnsSettingsRequest struct {
	Provider    string            `json:"provider"`
	Credentials map[string]string `json:"credentials"`
}

// dnsView builds the masked view of the live DNS-01 settings.
func (h *SSLHandler) dnsView(settings config.LetsEncryptConfig) dnsSettingsView {
	view := dnsSettingsView{
		Providers:   service.DNSProviderNames(),
		Provider:    settings.DNSProvider,
		Credentials: util.MaskDNSCredentials(settings.DNSCredentials),
	}
	if validated, ok := service.DNSValidatedAt(h.db); ok {
		view.ValidatedAt = validated.UTC().Format(time.RFC3339)
	}
	return view
}

// GetDNSSettings returns the provider list, the selected provider and the masked credentials.
func (h *SSLHandler) GetDNSSettings(w http.ResponseWriter, r *http.Request) {
	cfg := config.GetGlobalConfig()
	if cfg == nil {
		InternalError(w, r, Translate(r, "errors.admin.ssl.dns_save_failed"))
		return
	}
	writeJSON(w, http.StatusOK, h.dnsView(cfg.Server.SSL.LetsEncrypt))
}

// mergeDNSCredentials returns the plaintext credential map a request describes: a value that is
// empty or the mask keeps the stored value of the same provider, and a name the request omits is
// dropped. A provider change starts from nothing.
func mergeDNSCredentials(key string, current config.LetsEncryptConfig, request dnsSettingsRequest) (map[string]string, error) {
	var stored map[string]string
	if request.Provider == current.DNSProvider {
		opened, err := util.OpenDNSCredentials(key, current.DNSCredentials)
		if err != nil {
			return nil, err
		}
		stored = opened
	}
	merged := make(map[string]string, len(request.Credentials))
	for name, value := range request.Credentials {
		name = strings.TrimSpace(name)
		value = strings.TrimSpace(value)
		if value == "" || value == util.DNSCredentialMask {
			if kept, ok := stored[name]; ok {
				merged[name] = kept
			}
			continue
		}
		merged[name] = value
	}
	return merged, nil
}

// failDNSCheck answers a provider construction failure with the variable names only.
func failDNSCheck(w http.ResponseWriter, r *http.Request, err error) {
	var providerErr *service.DNSProviderError
	if errors.As(err, &providerErr) {
		BadRequest(w, r, Translate(r, "errors.admin.ssl.dns_provider_check_failed"), map[string]interface{}{
			"provider":  providerErr.Provider,
			"variables": providerErr.Variables,
		})
		return
	}
	BadRequest(w, r, Translate(r, "errors.admin.ssl.dns_credential_name_invalid"))
}

// CheckDNSSettings builds the provider from the submitted credentials without storing anything,
// so the form can learn which variables a provider needs. No DNS record is touched.
func (h *SSLHandler) CheckDNSSettings(w http.ResponseWriter, r *http.Request) {
	var request dnsSettingsRequest
	if !DecodeAndValidate(w, r, &request) {
		return
	}
	request.Provider = strings.TrimSpace(request.Provider)
	if !service.IsDNSProviderName(request.Provider) {
		BadRequest(w, r, Translate(r, "errors.admin.ssl.dns_provider_unsupported"))
		return
	}
	cfg := config.GetGlobalConfig()
	if cfg == nil || cfg.Server.Security.EncryptionKey == "" {
		InternalError(w, r, Translate(r, "errors.admin.ssl.dns_encryption_unavailable"))
		return
	}
	credentials, err := mergeDNSCredentials(cfg.Server.Security.EncryptionKey, cfg.Server.SSL.LetsEncrypt, request)
	if err != nil {
		InternalError(w, r, Translate(r, "errors.admin.ssl.dns_save_failed"))
		return
	}
	if _, err := service.NewDNSProvider(request.Provider, credentials); err != nil {
		failDNSCheck(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"ok":      true,
		"message": Translate(r, "success.admin.ssl.dns_checked"),
	})
}

// SaveDNSSettings validates the provider, seals the credentials, writes server.yml and clears
// dns_validated_at when the provider or any credential value changed (AI.md PART 15).
func (h *SSLHandler) SaveDNSSettings(w http.ResponseWriter, r *http.Request) {
	var request dnsSettingsRequest
	if !DecodeAndValidate(w, r, &request) {
		return
	}
	request.Provider = strings.TrimSpace(request.Provider)
	cfg := config.GetGlobalConfig()
	if cfg == nil || cfg.Server.Security.EncryptionKey == "" {
		InternalError(w, r, Translate(r, "errors.admin.ssl.dns_encryption_unavailable"))
		return
	}
	key := cfg.Server.Security.EncryptionKey
	current := cfg.Server.SSL.LetsEncrypt

	if request.Provider == "" {
		h.storeDNSSettings(w, r, cfg, "", nil, current)
		return
	}
	if !service.IsDNSProviderName(request.Provider) {
		BadRequest(w, r, Translate(r, "errors.admin.ssl.dns_provider_unsupported"))
		return
	}
	credentials, err := mergeDNSCredentials(key, current, request)
	if err != nil {
		InternalError(w, r, Translate(r, "errors.admin.ssl.dns_save_failed"))
		return
	}
	if _, err := service.NewDNSProvider(request.Provider, credentials); err != nil {
		failDNSCheck(w, r, err)
		return
	}
	h.storeDNSSettings(w, r, cfg, request.Provider, credentials, current)
}

// storeDNSSettings seals and persists the validated settings.
func (h *SSLHandler) storeDNSSettings(w http.ResponseWriter, r *http.Request, cfg *config.AppConfig, provider string, credentials map[string]string, current config.LetsEncryptConfig) {
	key := cfg.Server.Security.EncryptionKey
	changed := provider != current.DNSProvider || len(credentials) != len(current.DNSCredentials)
	if !changed {
		opened, err := util.OpenDNSCredentials(key, current.DNSCredentials)
		if err != nil {
			changed = true
		} else {
			for name, value := range credentials {
				if opened[name] != value {
					changed = true
					break
				}
			}
		}
	}

	sealed := make(map[string]string, len(credentials))
	for name, value := range credentials {
		if !changed {
			sealed[name] = current.DNSCredentials[name]
			continue
		}
		stored, err := util.SealDNSCredential(key, value)
		if err != nil {
			log.Printf("ERROR: dns credential could not be encrypted: %v", err)
			InternalError(w, r, Translate(r, "errors.admin.ssl.dns_save_failed"))
			return
		}
		sealed[name] = stored
	}

	cfg.Server.SSL.LetsEncrypt.DNSProvider = provider
	cfg.Server.SSL.LetsEncrypt.DNSCredentials = sealed
	if err := config.SaveConfig(cfg); err != nil {
		cfg.Server.SSL.LetsEncrypt = current
		log.Printf("ERROR: dns provider settings could not be written: %v", err)
		InternalError(w, r, Translate(r, "errors.admin.ssl.dns_save_failed"))
		return
	}
	if changed {
		if err := service.ClearDNSValidated(h.db); err != nil {
			log.Printf("WARNING: dns_validated_at was not cleared: %v", err)
		}
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"ok":       true,
		"message":  Translate(r, "success.admin.ssl.dns_saved"),
		"settings": h.dnsView(cfg.Server.SSL.LetsEncrypt),
	})
}

// applyDNSSolver gives a dns-01 request the lego provider built from the stored credentials. With
// no provider stored the manual TXT-record mode stays in place. A failure fails the request and
// names the variable only (AI.md PART 15 "Validation").
func (h *SSLHandler) applyDNSSolver(challengeType string) error {
	if challengeType != "dns-01" || h.leService == nil {
		return nil
	}
	cfg := config.GetGlobalConfig()
	if cfg == nil {
		return nil
	}
	solver, err := service.ConfiguredDNSSolver(cfg.Server.SSL.LetsEncrypt, cfg.Server.Security.EncryptionKey)
	if err != nil {
		log.Printf("ERROR: dns-01 provider check failed: %v", err)
		return err
	}
	h.leService.SetDNSSolver(solver)
	return nil
}

// recordDNSValidated stores the validation time after a dns-01 issuance through a provider.
func (h *SSLHandler) recordDNSValidated(challengeType string) {
	if challengeType != "dns-01" {
		return
	}
	cfg := config.GetGlobalConfig()
	if cfg == nil || cfg.Server.SSL.LetsEncrypt.DNSProvider == "" {
		return
	}
	if err := service.MarkDNSValidated(h.db); err != nil {
		log.Printf("WARNING: dns_validated_at was not recorded: %v", err)
	}
}
