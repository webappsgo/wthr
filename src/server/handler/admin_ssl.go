package handler

import (
	"crypto/tls"
	"database/sql"
	"fmt"
	"log"
	"net/http"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/webappsgo/wthr/src/common/i18n"
	"github.com/webappsgo/wthr/src/config"
	"github.com/webappsgo/wthr/src/server/service"
)

type SSLHandler struct {
	certsDir  string
	leService *service.LetsEncryptService
	db        *sql.DB
	// Runtime-detected HTTPS address (e.g., "localhost:443" or "0.0.0.0:443")
	httpsAddr string
	// challengeSink answers TLS-ALPN-01 validation on the running HTTPS listener
	challengeSink service.ChallengeCertSink
	// onCertificateChange makes the listener serve a certificate that was just obtained or renewed
	onCertificateChange func()
	leMu                sync.Mutex
}

// NewSSLHandler creates a new SSL handler
// httpsAddr should be the local HTTPS address to check certificates (e.g., "127.0.0.1:443")
func NewSSLHandler(certsDir string, db *sql.DB, httpsAddr string) *SSLHandler {
	if httpsAddr == "" {
		httpsAddr = "127.0.0.1:443" // Default for local cert checking
	}
	return &SSLHandler{
		certsDir:  certsDir,
		db:        db,
		httpsAddr: httpsAddr,
	}
}

func saveLetsEncryptSettings(email, challengeType string, staging bool) error {
	cfg := config.GetGlobalConfig()
	if cfg == nil {
		return fmt.Errorf("server configuration is not loaded")
	}
	previous := cfg.Server.SSL.LetsEncrypt
	cfg.Server.SSL.LetsEncrypt.Enabled = true
	cfg.Server.SSL.LetsEncrypt.Email = email
	cfg.Server.SSL.LetsEncrypt.Challenge = challengeType
	cfg.Server.SSL.LetsEncrypt.Staging = staging
	if err := config.SaveConfig(cfg); err != nil {
		cfg.Server.SSL.LetsEncrypt = previous
		return err
	}
	return nil
}

func sslSystemMessage(key string) string {
	translations := i18n.GetGlobalI18n()
	if translations == nil {
		return key
	}
	return translations.T(translations.GetDefaultLanguage(), key)
}

// SetCertificateHooks connects the handler to the HTTPS listener: the sink answers
// TLS-ALPN-01 challenges and onChange runs after a certificate was obtained or renewed.
func (h *SSLHandler) SetCertificateHooks(sink service.ChallengeCertSink, onChange func()) {
	h.challengeSink = sink
	h.onCertificateChange = onChange
}

// certificateChanged tells the listener to pick up the new certificate.
func (h *SSLHandler) certificateChanged() {
	if h.onCertificateChange != nil {
		h.onCertificateChange()
	}
}

// InitLetsEncrypt initializes the Let's Encrypt service
func (h *SSLHandler) InitLetsEncrypt(email string, staging bool) error {
	h.leMu.Lock()
	defer h.leMu.Unlock()
	if h.leService != nil {
		return nil
	}
	leService, err := service.NewLetsEncryptService(email, filepath.Join(h.certsDir, "letsencrypt"), staging)
	if err != nil {
		return fmt.Errorf("failed to initialize Let's Encrypt: %w", err)
	}
	if h.challengeSink != nil {
		leService.SetChallengeCertSink(h.challengeSink)
	}
	h.leService = leService
	return nil
}

func (h *SSLHandler) initConfiguredLetsEncrypt() error {
	cfg := config.GetGlobalConfig()
	if cfg == nil || strings.TrimSpace(cfg.Server.SSL.LetsEncrypt.Email) == "" {
		return fmt.Errorf("%s", sslSystemMessage("errors.admin.ssl.contact_email_not_configured"))
	}
	return h.InitLetsEncrypt(cfg.Server.SSL.LetsEncrypt.Email, cfg.Server.SSL.LetsEncrypt.Staging)
}

type CertificateInfo struct {
	Subject       string    `json:"subject"`
	Issuer        string    `json:"issuer"`
	NotBefore     time.Time `json:"notBefore"`
	NotAfter      time.Time `json:"notAfter"`
	DNSNames      []string  `json:"dnsNames"`
	IsValid       bool      `json:"isValid"`
	DaysRemaining int       `json:"daysRemaining"`
}

type SSLStatus struct {
	Certificate *CertificateInfo `json:"certificate"`
	NextCheck   string           `json:"nextCheck"`
	NextRenewal string           `json:"nextRenewal"`
	LastRenewal string           `json:"lastRenewal"`
	AutoRenewal bool             `json:"autoRenewal"`
}

// GetStatus returns the current SSL certificate status
func (h *SSLHandler) GetStatus(w http.ResponseWriter, r *http.Request) {
	// Try to load existing certificate
	certInfo, err := h.getCertificateInfo()
	if err != nil {
		// No certificate or error loading
		writeJSON(w, http.StatusOK, SSLStatus{
			Certificate: nil,
			NextCheck:   Translate(r, "admin.ssl.status.not_scheduled"),
			NextRenewal: Translate(r, "admin.ssl.status.no_certificate"),
			LastRenewal: Translate(r, "admin.ssl.status.never"),
			AutoRenewal: false,
		})
		return
	}

	status := SSLStatus{
		Certificate: certInfo,
		NextCheck:   service.NextRenewalCheck(time.Now()).Format("2006-01-02 15:04"),
		NextRenewal: calculateNextRenewal(r, certInfo.NotAfter),
		LastRenewal: Translate(r, "admin.ssl.status.unknown"),
		AutoRenewal: true,
	}

	writeJSON(w, http.StatusOK, status)
}

// ObtainCertificate obtains a new Let's Encrypt certificate
// AI.md PART 15: Full Let's Encrypt integration with all 3 challenge types
func (h *SSLHandler) ObtainCertificate(w http.ResponseWriter, r *http.Request) {
	var request struct {
		Domain   string   `json:"domain" binding:"required"`
		Email    string   `json:"email" binding:"required"`
		AltNames []string `json:"altNames"`
		// http-01, tls-alpn-01, dns-01
		ChallengeType string `json:"challengeType" binding:"required"`
		// Use staging server for testing
		Staging bool `json:"staging"`
	}

	if wantsFormSubmission(r) {
		if err := r.ParseForm(); err != nil {
			redirectAdminForm(w, r, adminPagePath(r, "/ssl"), "success.admin.ssl.certificate_obtained_successfully", "errors.admin.ssl.failed_to_obtain_certificate", err)
			return
		}
		request.Domain = strings.TrimSpace(r.PostForm.Get("domain"))
		request.Email = strings.TrimSpace(r.PostForm.Get("email"))
		request.AltNames = splitSSLFormDomains(r.PostForm.Get("altNames"))
		request.ChallengeType = strings.TrimSpace(r.PostForm.Get("challengeType"))
		request.Staging = r.PostForm.Get("staging") == "true"
		if request.Domain == "" || request.Email == "" || !isSSLChallengeType(request.ChallengeType) {
			redirectAdminForm(w, r, adminPagePath(r, "/ssl"), "success.admin.ssl.certificate_obtained_successfully", "errors.admin.ssl.invalid_challenge_type", fmt.Errorf("domain, email and valid challenge type are required"))
			return
		}
	} else if !DecodeAndValidate(w, r, &request) {
		return
	}

	// Validate challenge type
	validChallenges := map[string]bool{
		"http-01":     true,
		"tls-alpn-01": true,
		"dns-01":      true,
	}
	if !validChallenges[request.ChallengeType] {
		if wantsFormSubmission(r) {
			redirectAdminForm(w, r, adminPagePath(r, "/ssl"), "success.admin.ssl.certificate_obtained_successfully", "errors.admin.ssl.invalid_challenge_type", fmt.Errorf("invalid challenge type"))
			return
		}
		BadRequest(w, r, Translate(r, "errors.admin.ssl.invalid_challenge_type"), map[string]interface{}{
			"validTypes":    []string{"http-01", "tls-alpn-01", "dns-01"},
			"challengeType": request.ChallengeType,
		})
		return
	}
	if err := saveLetsEncryptSettings(request.Email, request.ChallengeType, request.Staging); err != nil {
		log.Printf("ERROR: %s", sslSystemMessage("log.ssl.settings_save_failed"))
		if wantsFormSubmission(r) {
			redirectAdminForm(w, r, adminPagePath(r, "/ssl"), "success.admin.ssl.certificate_obtained_successfully", "errors.admin.ssl.failed_to_initialize_lets_encrypt_service", err)
			return
		}
		InternalError(w, r, Translate(r, "errors.admin.ssl.failed_to_initialize_lets_encrypt_service"))
		return
	}

	// Initialize Let's Encrypt service if not already initialized
	if h.leService == nil {
		if err := h.InitLetsEncrypt(request.Email, request.Staging); err != nil {
			log.Printf("ssl init error: %v", err)
			if wantsFormSubmission(r) {
				redirectAdminForm(w, r, adminPagePath(r, "/ssl"), "success.admin.ssl.certificate_obtained_successfully", "errors.admin.ssl.failed_to_initialize_lets_encrypt_service", err)
				return
			}
			InternalError(w, r, Translate(r, "errors.admin.ssl.failed_to_initialize_lets_encrypt_service"))
			return
		}
	}

	if err := h.applyDNSSolver(request.ChallengeType); err != nil {
		if wantsFormSubmission(r) {
			redirectAdminForm(w, r, adminPagePath(r, "/ssl"), "success.admin.ssl.certificate_obtained_successfully", "errors.admin.ssl.failed_to_obtain_certificate", err)
			return
		}
		failDNSCheck(w, r, err)
		return
	}

	// Obtain certificate
	cert, err := h.leService.ObtainCertificate(request.Domain, request.AltNames, request.ChallengeType)
	if err != nil {
		log.Printf("ssl obtain cert error: %v", err)
		if wantsFormSubmission(r) {
			redirectAdminForm(w, r, adminPagePath(r, "/ssl"), "success.admin.ssl.certificate_obtained_successfully", "errors.admin.ssl.failed_to_obtain_certificate", err)
			return
		}
		InternalError(w, r, Translate(r, "errors.admin.ssl.failed_to_obtain_certificate"))
		return
	}
	h.certificateChanged()
	h.recordDNSValidated(request.ChallengeType)
	if wantsFormSubmission(r) {
		redirectAdminForm(w, r, adminPagePath(r, "/ssl"), "success.admin.ssl.certificate_obtained_successfully", "errors.admin.ssl.failed_to_obtain_certificate", nil)
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"ok":                true,
		"message":           Translate(r, "success.admin.ssl.certificate_obtained_successfully"),
		"domain":            request.Domain,
		"altNames":          request.AltNames,
		"challengeType":     request.ChallengeType,
		"certificate":       string(cert.Certificate),
		"issuerCertificate": string(cert.IssuerCertificate),
	})
}

// RenewCertificate renews an existing certificate
// AI.md PART 15: Auto-renewal support
func (h *SSLHandler) RenewCertificate(w http.ResponseWriter, r *http.Request) {
	var request struct {
		Domain        string `json:"domain" binding:"required"`
		ChallengeType string `json:"challengeType" binding:"required"`
		// Force renewal even if not needed
		Force bool `json:"force"`
	}

	if wantsFormSubmission(r) {
		if err := r.ParseForm(); err != nil {
			redirectAdminForm(w, r, adminPagePath(r, "/ssl"), "success.admin.ssl.certificate_renewed_successfully", "errors.admin.ssl.failed_to_renew_certificate", err)
			return
		}
		request.Domain = strings.TrimSpace(r.PostForm.Get("domain"))
		request.ChallengeType = strings.TrimSpace(r.PostForm.Get("challengeType"))
		request.Force = r.PostForm.Get("force") == "true"
		if request.Domain == "" || !isSSLChallengeType(request.ChallengeType) {
			redirectAdminForm(w, r, adminPagePath(r, "/ssl"), "success.admin.ssl.certificate_renewed_successfully", "errors.admin.ssl.invalid_challenge_type", fmt.Errorf("domain and valid challenge type are required"))
			return
		}
	} else if !DecodeAndValidate(w, r, &request) {
		return
	}

	// Check if Let's Encrypt service is initialized
	if h.leService == nil {
		if err := h.initConfiguredLetsEncrypt(); err != nil {
			if wantsFormSubmission(r) {
				redirectAdminForm(w, r, adminPagePath(r, "/ssl"), "success.admin.ssl.certificate_renewed_successfully", "errors.admin.ssl.failed_to_initialize_lets_encrypt_service", err)
				return
			}
			InternalError(w, r, Translate(r, "errors.admin.ssl.failed_to_initialize_lets_encrypt_service"))
			return
		}
	}

	// Check if renewal is needed
	if !request.Force {
		needsRenewal, daysRemaining, err := h.leService.CheckRenewal(request.Domain)
		if err != nil {
			log.Printf("ssl check renewal error: %v", err)
			if wantsFormSubmission(r) {
				redirectAdminForm(w, r, adminPagePath(r, "/ssl"), "success.admin.ssl.certificate_renewed_successfully", "errors.admin.ssl.failed_to_check_renewal_status", err)
				return
			}
			InternalError(w, r, Translate(r, "errors.admin.ssl.failed_to_check_renewal_status"))
			return
		}

		if !needsRenewal {
			if wantsFormSubmission(r) {
				SetFlash(w, r, "info", "success.admin.ssl.certificate_does_not_need_renewal_yet")
				http.Redirect(w, r, adminPagePath(r, "/ssl"), http.StatusSeeOther)
				return
			}
			writeJSON(w, http.StatusOK, map[string]interface{}{
				"message":       Translate(r, "success.admin.ssl.certificate_does_not_need_renewal_yet"),
				"daysRemaining": daysRemaining,
				"needsRenewal":  false,
			})
			return
		}
	}

	if err := h.applyDNSSolver(request.ChallengeType); err != nil {
		if wantsFormSubmission(r) {
			redirectAdminForm(w, r, adminPagePath(r, "/ssl"), "success.admin.ssl.certificate_renewed_successfully", "errors.admin.ssl.failed_to_renew_certificate", err)
			return
		}
		failDNSCheck(w, r, err)
		return
	}

	// Renew certificate
	cert, err := h.leService.RenewCertificate(request.Domain, request.ChallengeType)
	if err != nil {
		log.Printf("ssl renew error: %v", err)
		if wantsFormSubmission(r) {
			redirectAdminForm(w, r, adminPagePath(r, "/ssl"), "success.admin.ssl.certificate_renewed_successfully", "errors.admin.ssl.failed_to_renew_certificate", err)
			return
		}
		InternalError(w, r, Translate(r, "errors.admin.ssl.failed_to_renew_certificate"))
		return
	}
	h.certificateChanged()
	h.recordDNSValidated(request.ChallengeType)
	if wantsFormSubmission(r) {
		redirectAdminForm(w, r, adminPagePath(r, "/ssl"), "success.admin.ssl.certificate_renewed_successfully", "errors.admin.ssl.failed_to_renew_certificate", nil)
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"ok":          true,
		"message":     Translate(r, "success.admin.ssl.certificate_renewed_successfully"),
		"domain":      request.Domain,
		"certificate": string(cert.Certificate),
	})
}

// VerifyCertificate verifies the current certificate
func (h *SSLHandler) VerifyCertificate(w http.ResponseWriter, r *http.Request) {
	certInfo, err := h.getCertificateInfo()
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"valid": false,
			"error": Translate(r, "errors.admin.ssl.no_certificate_found_or_unable_to_load"),
		})
		return
	}

	// Check if certificate is expired
	if time.Now().After(certInfo.NotAfter) {
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"valid": false,
			"error": Translate(r, "errors.admin.ssl.certificate_has_expired"),
		})
		return
	}

	// Check if certificate is not yet valid
	if time.Now().Before(certInfo.NotBefore) {
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"valid": false,
			"error": Translate(r, "errors.admin.ssl.certificate_is_not_yet_valid"),
		})
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"valid":         true,
		"message":       Translate(r, "success.admin.ssl.certificate_is_valid"),
		"subject":       certInfo.Subject,
		"issuer":        certInfo.Issuer,
		"notAfter":      certInfo.NotAfter,
		"daysRemaining": certInfo.DaysRemaining,
	})
}

// RevokeCertificate revokes the current certificate
func (h *SSLHandler) RevokeCertificate(w http.ResponseWriter, r *http.Request) {
	var request struct {
		Domain string `json:"domain" binding:"required"`
		Reason string `json:"reason"`
	}

	if wantsFormSubmission(r) {
		if err := r.ParseForm(); err != nil {
			redirectAdminForm(w, r, adminPagePath(r, "/ssl"), "success.admin.ssl.certificate_revoked_successfully", "errors.admin.ssl.failed_to_revoke_certificate", err)
			return
		}
		request.Domain = strings.TrimSpace(r.PostForm.Get("domain"))
		request.Reason = strings.TrimSpace(r.PostForm.Get("reason"))
		if request.Domain == "" {
			redirectAdminForm(w, r, adminPagePath(r, "/ssl"), "success.admin.ssl.certificate_revoked_successfully", "errors.admin.ssl.failed_to_revoke_certificate", fmt.Errorf("domain is required"))
			return
		}
	} else if !DecodeAndValidate(w, r, &request) {
		return
	}

	// Check if Let's Encrypt service is initialized
	if h.leService == nil {
		if err := h.initConfiguredLetsEncrypt(); err != nil {
			if wantsFormSubmission(r) {
				redirectAdminForm(w, r, adminPagePath(r, "/ssl"), "success.admin.ssl.certificate_revoked_successfully", "errors.admin.ssl.failed_to_initialize_lets_encrypt_service", err)
				return
			}
			InternalError(w, r, Translate(r, "errors.admin.ssl.failed_to_initialize_lets_encrypt_service"))
			return
		}
	}

	// Revoke certificate
	if err := h.leService.RevokeCertificate(request.Domain); err != nil {
		log.Printf("ssl revoke error: %v", err)
		if wantsFormSubmission(r) {
			redirectAdminForm(w, r, adminPagePath(r, "/ssl"), "success.admin.ssl.certificate_revoked_successfully", "errors.admin.ssl.failed_to_revoke_certificate", err)
			return
		}
		InternalError(w, r, Translate(r, "errors.admin.ssl.failed_to_revoke_certificate"))
		return
	}
	if wantsFormSubmission(r) {
		redirectAdminForm(w, r, adminPagePath(r, "/ssl"), "success.admin.ssl.certificate_revoked_successfully", "errors.admin.ssl.failed_to_revoke_certificate", nil)
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"ok":      true,
		"message": Translate(r, "success.admin.ssl.certificate_revoked_successfully"),
		"domain":  request.Domain,
		"reason":  request.Reason,
	})
}

// StartAutoRenewal starts automatic certificate renewal
// AI.md PART 15: Auto-renewal must be available
func (h *SSLHandler) StartAutoRenewal(w http.ResponseWriter, r *http.Request) {
	var request struct {
		Domains       []string `json:"domains" binding:"required"`
		ChallengeType string   `json:"challengeType" binding:"required"`
	}

	if wantsFormSubmission(r) {
		if err := r.ParseForm(); err != nil {
			redirectAdminForm(w, r, adminPagePath(r, "/ssl"), "success.admin.ssl.auto_renewal_started", "errors.admin.ssl.failed_to_check_renewal_status", err)
			return
		}
		request.Domains = splitSSLFormDomains(r.PostForm.Get("domains"))
		request.ChallengeType = strings.TrimSpace(r.PostForm.Get("challengeType"))
		if len(request.Domains) == 0 || !isSSLChallengeType(request.ChallengeType) {
			redirectAdminForm(w, r, adminPagePath(r, "/ssl"), "success.admin.ssl.auto_renewal_started", "errors.admin.ssl.invalid_challenge_type", fmt.Errorf("at least one domain and a valid challenge type are required"))
			return
		}
	} else if !DecodeAndValidate(w, r, &request) {
		return
	}

	// Check if Let's Encrypt service is initialized
	if h.leService == nil {
		cfg := config.GetGlobalConfig()
		if cfg == nil || strings.TrimSpace(cfg.Server.SSL.LetsEncrypt.Email) == "" {
			if wantsFormSubmission(r) {
				redirectAdminForm(w, r, adminPagePath(r, "/ssl"), "success.admin.ssl.auto_renewal_started", "errors.admin.ssl.contact_email_not_configured", fmt.Errorf("let's Encrypt contact email is not configured"))
				return
			}
			BadRequest(w, r, Translate(r, "errors.admin.ssl.contact_email_not_configured"))
			return
		}
		if err := h.InitLetsEncrypt(cfg.Server.SSL.LetsEncrypt.Email, cfg.Server.SSL.LetsEncrypt.Staging); err != nil {
			if wantsFormSubmission(r) {
				redirectAdminForm(w, r, adminPagePath(r, "/ssl"), "success.admin.ssl.auto_renewal_started", "errors.admin.ssl.failed_to_initialize_lets_encrypt_service", err)
				return
			}
			InternalError(w, r, Translate(r, "errors.admin.ssl.failed_to_initialize_lets_encrypt_service"))
			return
		}
	}
	if err := h.applyDNSSolver(request.ChallengeType); err != nil {
		if wantsFormSubmission(r) {
			redirectAdminForm(w, r, adminPagePath(r, "/ssl"), "success.admin.ssl.auto_renewal_started", "errors.admin.ssl.dns_provider_check_failed", err)
			return
		}
		failDNSCheck(w, r, err)
		return
	}

	// Start auto-renewal in background
	h.leService.StartAutoRenewal(request.Domains, request.ChallengeType)
	if wantsFormSubmission(r) {
		redirectAdminForm(w, r, adminPagePath(r, "/ssl"), "success.admin.ssl.auto_renewal_started", "errors.admin.ssl.failed_to_check_renewal_status", nil)
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"ok":            true,
		"message":       Translate(r, "success.admin.ssl.auto_renewal_started"),
		"domains":       request.Domains,
		"checkInterval": "daily at 03:00",
	})
}

func splitSSLFormDomains(value string) []string {
	parts := strings.FieldsFunc(value, func(r rune) bool { return r == ',' || r == '\n' || r == '\r' || r == ' ' || r == '\t' })
	domains := make([]string, 0, len(parts))
	for _, part := range parts {
		if domain := strings.TrimSpace(part); domain != "" {
			domains = append(domains, domain)
		}
	}
	return domains
}

func isSSLChallengeType(challenge string) bool {
	switch challenge {
	case "http-01", "tls-alpn-01", "dns-01":
		return true
	default:
		return false
	}
}

// GetDNSRecords returns DNS records needed for DNS-01 challenge
// AI.md PART 15: DNS-01 challenge support
func (h *SSLHandler) GetDNSRecords(w http.ResponseWriter, r *http.Request) {
	// Check if Let's Encrypt service is initialized
	if h.leService == nil {
		BadRequest(w, r, Translate(r, "errors.admin.ssl.let_s_encrypt_service_not_initialized"))
		return
	}

	records := h.leService.GetPendingDNSRecords()

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"ok":      true,
		"message": Translate(r, "success.admin.ssl.dns_records_for_manual_configuration"),
		"note":    Translate(r, "admin.ssl.add_these_txt_records_to_your_dns_before_requesting_certificate_with_dns_01_challenge"),
		"records": records,
	})
}

// Helper: Get certificate information
func (h *SSLHandler) getCertificateInfo() (*CertificateInfo, error) {
	// Connect to the local HTTPS server to read the active certificate
	conn, err := tls.Dial("tcp", h.httpsAddr, &tls.Config{
		InsecureSkipVerify: true,
	})
	if err != nil {
		return nil, fmt.Errorf("no certificate available")
	}
	defer func() { _ = conn.Close() }()

	certs := conn.ConnectionState().PeerCertificates
	if len(certs) == 0 {
		return nil, fmt.Errorf("no certificate found")
	}

	cert := certs[0]
	daysRemaining := int(time.Until(cert.NotAfter).Hours() / 24)

	return &CertificateInfo{
		Subject:       cert.Subject.CommonName,
		Issuer:        cert.Issuer.CommonName,
		NotBefore:     cert.NotBefore,
		NotAfter:      cert.NotAfter,
		DNSNames:      cert.DNSNames,
		IsValid:       time.Now().After(cert.NotBefore) && time.Now().Before(cert.NotAfter),
		DaysRemaining: daysRemaining,
	}, nil
}

// Helper: Calculate next renewal date
func calculateNextRenewal(r *http.Request, notAfter time.Time) string {
	// Renew 30 days before expiry
	renewalDate := notAfter.Add(-30 * 24 * time.Hour)
	if time.Now().After(renewalDate) {
		return Translate(r, "admin.ssl.status.now")
	}
	return renewalDate.Format("2006-01-02 15:04")
}
