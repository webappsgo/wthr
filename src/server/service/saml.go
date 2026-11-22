package service

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/crewjam/saml"
	"github.com/crewjam/saml/samlsp"

	"github.com/webappsgo/wthr/src/common/dbtime"
	"github.com/webappsgo/wthr/src/config"
	"github.com/webappsgo/wthr/src/database"
	models "github.com/webappsgo/wthr/src/server/model"
	"github.com/webappsgo/wthr/src/util"
)

// SAMLProviderConfig contains the operator-controlled settings for one SAML IdP.
type SAMLProviderConfig struct {
	Name                 string              `json:"name" yaml:"name"`
	DisplayName          string              `json:"display_name" yaml:"display_name"`
	IDPMetadataURL       string              `json:"idp_metadata_url" yaml:"idp_metadata_url"`
	IDPMetadataXML       string              `json:"idp_metadata_xml,omitempty" yaml:"idp_metadata_xml,omitempty"`
	SPEntityID           string              `json:"sp_entity_id" yaml:"sp_entity_id"`
	ACSURL               string              `json:"acs_url" yaml:"acs_url"`
	WantAssertionsSigned bool                `json:"want_assertions_signed" yaml:"want_assertions_signed"`
	SignRequests         bool                `json:"sign_requests" yaml:"sign_requests"`
	ClockSkew            string              `json:"clock_skew" yaml:"clock_skew"`
	AutoRegister         bool                `json:"auto_register" yaml:"auto_register"`
	AttributeMapping     SAMLAttributeMap    `json:"attribute_mapping" yaml:"attribute_mapping"`
	UsernameResolution   SAMLUsernameConfig  `json:"username_resolution" yaml:"username_resolution"`
	AdminGroups          []string            `json:"admin_groups" yaml:"admin_groups"`
	RoleMapping          map[string][]string `json:"role_mapping" yaml:"role_mapping"`
	NameIDFormat         string              `json:"nameid_format" yaml:"nameid_format"`
	AutoGenerateCert     bool                `json:"auto_generate_cert" yaml:"auto_generate_cert"`
	SPCertPath           string              `json:"sp_cert_path" yaml:"sp_cert_path"`
	SPKeyPath            string              `json:"sp_key_path" yaml:"sp_key_path"`
	CertExpiryDays       int                 `json:"cert_expiry_days" yaml:"cert_expiry_days"`
	SLOEnabled           bool                `json:"slo_enabled" yaml:"slo_enabled"`
	IDPSLOURL            string              `json:"idp_slo_url" yaml:"idp_slo_url"`
	AllowIDPInitiated    bool                `json:"allow_idp_initiated" yaml:"allow_idp_initiated"`
}

var samlClockSkewMu sync.Mutex

type SAMLAttributeMap struct {
	Username   string `json:"username" yaml:"username"`
	Email      string `json:"email" yaml:"email"`
	Name       string `json:"name" yaml:"name"`
	Groups     string `json:"groups" yaml:"groups"`
	ExternalID string `json:"external_id" yaml:"external_id"`
}

type SAMLUsernameConfig struct {
	Mode                    string `json:"mode" yaml:"mode"`
	AllowCustomOnFirstLogin bool   `json:"allow_custom_on_first_login" yaml:"allow_custom_on_first_login"`
}

// SAMLService resolves named provider configurations and builds validated SPs.
type samlMetadataCacheEntry struct {
	metadata *saml.EntityDescriptor
	expires  time.Time
}

type SAMLService struct {
	settings      *models.SettingsModel
	mu            sync.Mutex
	metadataCache map[string]samlMetadataCacheEntry
}

func NewSAMLService() *SAMLService {
	return &SAMLService{settings: &models.SettingsModel{DB: database.GetServerDB()}, metadataCache: make(map[string]samlMetadataCacheEntry)}
}

func (s *SAMLService) Enabled() bool {
	return s.settings.GetBool("server.auth.saml.enabled", false)
}

func (s *SAMLService) Providers() []SAMLProviderConfig {
	raw := s.settings.GetString("server.auth.saml.providers", "")
	var providers []SAMLProviderConfig
	if raw == "" || json.Unmarshal([]byte(raw), &providers) != nil {
		return nil
	}
	valid := make([]SAMLProviderConfig, 0, len(providers))
	for _, p := range providers {
		if validSAMLProviderName(p.Name) {
			valid = append(valid, p)
		}
	}
	return valid
}

func (s *SAMLService) Provider(name string) (*SAMLProviderConfig, error) {
	for _, p := range s.Providers() {
		if p.Name == name {
			return &p, nil
		}
	}
	return nil, fmt.Errorf("SAML provider %q not found", name)
}

func validSAMLProviderName(name string) bool {
	if len(name) == 0 || len(name) > 48 || name[0] < 'a' || name[0] > 'z' {
		return false
	}
	for _, c := range name {
		switch {
		case c >= 'a' && c <= 'z', c >= '0' && c <= '9', c == '-', c == '_':
		default:
			return false
		}
	}
	return true
}

// ServiceProvider returns a library-backed SP configured to reject unsolicited
// responses unless this provider explicitly allows them.
func (s *SAMLService) ServiceProvider(ctx context.Context, p *SAMLProviderConfig, requestURL string) (*saml.ServiceProvider, error) {
	if p == nil {
		return nil, errors.New("SAML provider is required")
	}
	if strings.TrimSpace(p.SPCertPath) != "" || strings.TrimSpace(p.SPKeyPath) != "" {
		if _, _, err := LoadSAMLSPKeyPairFiles(p.SPCertPath, p.SPKeyPath); err != nil {
			return nil, err
		}
	} else if p.AutoGenerateCert {
		if err := s.ensureSAMLSPCertificate(p); err != nil {
			return nil, err
		}
	}
	metadata, err := s.idpMetadata(ctx, p)
	if err != nil {
		return nil, err
	}
	metadataURL, err := url.Parse(strings.TrimRight(requestURL, "/") + "/server/auth/saml/" + p.Name + "/metadata")
	if err != nil {
		return nil, err
	}
	sloURL, err := url.Parse(strings.TrimRight(requestURL, "/") + "/server/auth/saml/" + p.Name + "/slo")
	if err != nil {
		return nil, err
	}
	acsURL := p.ACSURL
	if acsURL == "" {
		acsURL = strings.TrimRight(requestURL, "/") + "/server/auth/saml/" + p.Name + "/acs"
	}
	acs, err := url.Parse(acsURL)
	if err != nil || !validSAMLHTTPURL(acsURL) {
		return nil, errors.New("SAML ACS URL must be an HTTP or HTTPS URL")
	}
	entity := p.SPEntityID
	if entity == "" {
		entity = metadataURL.String()
	}
	if !validSAMLHTTPURL(entity) {
		return nil, errors.New("SAML SP entity ID must be an HTTP or HTTPS URL")
	}
	var signer *rsa.PrivateKey
	var cert *x509.Certificate
	if strings.TrimSpace(p.SPCertPath) != "" || strings.TrimSpace(p.SPKeyPath) != "" {
		signer, cert, err = LoadSAMLSPKeyPairFiles(p.SPCertPath, p.SPKeyPath)
		if err != nil {
			return nil, err
		}
	} else {
		keyPEM, readErr := config.ReadSecret("server.auth.saml.providers." + p.Name + ".sp_private_key")
		if readErr != nil || keyPEM == "" {
			return nil, errors.New("SAML SP signing key is not configured")
		}
		cfg := config.GetGlobalConfig()
		if cfg == nil || strings.TrimSpace(cfg.Server.Security.EncryptionKey) == "" {
			return nil, errors.New("SAML private-key encryption is not configured")
		}
		keyPEM, err = util.DecryptAtRest(cfg.Server.Security.EncryptionKey, keyPEM)
		if err != nil {
			return nil, errors.New("SAML SP signing key could not be decrypted")
		}
		signer, err = parseSAMLPrivateKey([]byte(keyPEM))
		if err != nil {
			return nil, err
		}
		certPEM, readErr := config.ReadSecret("server.auth.saml.providers." + p.Name + ".sp_certificate")
		if readErr != nil || certPEM == "" {
			return nil, errors.New("SAML SP certificate is not configured")
		}
		cert, err = parseSAMLSPCertificate([]byte(certPEM))
		if err != nil {
			return nil, err
		}
		if err := validateSAMLKeyPair(signer, cert); err != nil {
			return nil, err
		}
	}
	sp := &saml.ServiceProvider{
		EntityID: entity, Key: signer, Certificate: cert, MetadataURL: *metadataURL, SloURL: *sloURL,
		AcsURL: *acs, IDPMetadata: metadata, AllowIDPInitiated: p.AllowIDPInitiated,
		AuthnNameIDFormat:                saml.NameIDFormat(p.NameIDFormat),
		LogoutSignatureMethod:            "http://www.w3.org/2001/04/xmldsig-more#rsa-sha256",
		LogoutBindings:                   []string{saml.HTTPRedirectBinding, saml.HTTPPostBinding},
		AssertionConsumerServiceBindings: []string{saml.HTTPPostBinding},
		ReplayCache:                      samlReplayCache{},
	}
	if p.SignRequests {
		sp.SignatureMethod = "http://www.w3.org/2001/04/xmldsig-more#rsa-sha256"
	}
	return sp, nil
}

// ParseResponse applies the configured assertion clock skew while validating
// a response. crewjam/saml exposes this setting as a package global, so the
// lock keeps provider-specific values from racing across concurrent logins.
func (s *SAMLService) ParseResponse(sp *saml.ServiceProvider, provider *SAMLProviderConfig, request *http.Request, requestIDs []string) (*saml.Assertion, error) {
	skew := 2 * time.Minute
	if provider.ClockSkew != "" {
		parsed, err := time.ParseDuration(provider.ClockSkew)
		if err != nil || parsed < 0 {
			return nil, fmt.Errorf("invalid SAML clock_skew %q", provider.ClockSkew)
		}
		skew = parsed
	}
	samlClockSkewMu.Lock()
	defer samlClockSkewMu.Unlock()
	previous := saml.MaxClockSkew
	saml.MaxClockSkew = skew
	defer func() { saml.MaxClockSkew = previous }()
	return sp.ParseResponse(request, requestIDs)
}

// ensureSAMLSPCertificate rotates generated keys before they expire. It keeps
// concurrent metadata and login requests from replacing the same keypair.
func (s *SAMLService) ensureSAMLSPCertificate(p *SAMLProviderConfig) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	days := p.CertExpiryDays
	if days <= 0 || days > 36500 {
		days = 3650
	}
	rotateBefore := 30 * 24 * time.Hour
	if configured := time.Duration(days) * 24 * time.Hour / 3; configured < rotateBefore {
		rotateBefore = configured
	}
	if rotateBefore < time.Hour {
		rotateBefore = time.Hour
	}
	info, err := GetSAMLSPCertificateInfo(p.Name)
	if err == nil && time.Until(info.ExpiresAt) > rotateBefore {
		return nil
	}
	return GenerateSAMLSPKeyPair(p.Name, days)
}

func validSAMLHTTPURL(raw string) bool {
	u, err := url.ParseRequestURI(raw)
	return err == nil && (u.Scheme == "http" || u.Scheme == "https") && u.Host != "" && u.User == nil && u.Fragment == ""
}

// LoadSAMLSPKeyPairFiles loads operator-supplied signing material. The private
// key must be a regular, non-symlink file with owner-read-only permissions;
// the public certificate may be readable by other users but must not be
// writable by group or others.
func LoadSAMLSPKeyPairFiles(certPath, keyPath string) (*rsa.PrivateKey, *x509.Certificate, error) {
	if strings.TrimSpace(certPath) == "" || strings.TrimSpace(keyPath) == "" {
		return nil, nil, errors.New("both SP certificate and private-key paths are required")
	}
	certPEM, err := readSAMLSPFile(certPath, false)
	if err != nil {
		return nil, nil, fmt.Errorf("invalid SAML SP certificate file")
	}
	keyPEM, err := readSAMLSPFile(keyPath, true)
	if err != nil {
		return nil, nil, fmt.Errorf("invalid SAML SP private-key file")
	}
	key, err := parseSAMLPrivateKey(keyPEM)
	if err != nil {
		return nil, nil, err
	}
	cert, err := parseSAMLSPCertificate(certPEM)
	if err != nil {
		return nil, nil, err
	}
	if err := validateSAMLKeyPair(key, cert); err != nil {
		return nil, nil, err
	}
	return key, cert, nil
}

func readSAMLSPFile(path string, private bool) ([]byte, error) {
	if !filepath.IsAbs(path) || filepath.Clean(path) != path {
		return nil, errors.New("SAML key paths must be absolute and clean")
	}
	linkInfo, err := os.Lstat(path)
	if err != nil || !linkInfo.Mode().IsRegular() {
		return nil, errors.New("SAML key material must be a regular file")
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = file.Close() }()
	info, err := file.Stat()
	if err != nil || !os.SameFile(linkInfo, info) || !info.Mode().IsRegular() {
		return nil, errors.New("SAML key material changed while opening")
	}
	if private {
		if info.Mode().Perm() != 0400 && info.Mode().Perm() != 0600 {
			return nil, errors.New("SAML private key must have mode 0400 or 0600")
		}
	} else if info.Mode().Perm()&0022 != 0 {
		return nil, errors.New("SAML certificate file must not be group or world writable")
	}
	if info.Size() <= 0 || info.Size() > 1<<20 {
		return nil, errors.New("SAML key material file size is invalid")
	}
	data := make([]byte, info.Size())
	if _, err := io.ReadFull(file, data); err != nil {
		return nil, err
	}
	return data, nil
}

func parseSAMLPrivateKey(data []byte) (*rsa.PrivateKey, error) {
	block, rest := pem.Decode(data)
	if block == nil || len(strings.TrimSpace(string(rest))) != 0 {
		return nil, errors.New("invalid SAML SP signing key PEM")
	}
	var parsed interface{}
	var err error
	switch block.Type {
	case "PRIVATE KEY":
		parsed, err = x509.ParsePKCS8PrivateKey(block.Bytes)
	case "RSA PRIVATE KEY":
		parsed, err = x509.ParsePKCS1PrivateKey(block.Bytes)
	default:
		return nil, errors.New("SAML SP key must be an unencrypted RSA private key")
	}
	if err != nil {
		return nil, errors.New("invalid SAML SP signing key")
	}
	key, ok := parsed.(*rsa.PrivateKey)
	if !ok || key.N.BitLen() < 2048 {
		return nil, errors.New("SAML SP signing key must be RSA with at least 2048 bits")
	}
	if err := key.Validate(); err != nil {
		return nil, errors.New("invalid SAML SP signing key")
	}
	return key, nil
}

func parseSAMLSPCertificate(data []byte) (*x509.Certificate, error) {
	block, rest := pem.Decode(data)
	if block == nil || block.Type != "CERTIFICATE" || len(strings.TrimSpace(string(rest))) != 0 {
		return nil, errors.New("invalid SAML SP certificate PEM")
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return nil, errors.New("invalid SAML SP certificate")
	}
	return cert, nil
}

func validateSAMLKeyPair(key *rsa.PrivateKey, cert *x509.Certificate) error {
	if err := validateSAMLKeyPairMaterial(key, cert); err != nil {
		return err
	}
	now := time.Now()
	if now.Before(cert.NotBefore) || !now.Before(cert.NotAfter) {
		return errors.New("SAML SP certificate is outside its validity period")
	}
	return nil
}

func validateSAMLKeyPairMaterial(key *rsa.PrivateKey, cert *x509.Certificate) error {
	if key == nil || cert == nil {
		return errors.New("SAML SP signing key and certificate are required")
	}
	publicKey, ok := cert.PublicKey.(*rsa.PublicKey)
	if !ok || key.N.Cmp(publicKey.N) != 0 || key.E != publicKey.E {
		return errors.New("SAML SP certificate does not match its private key")
	}
	if cert.KeyUsage != 0 && cert.KeyUsage&x509.KeyUsageDigitalSignature == 0 {
		return errors.New("SAML SP certificate does not permit digital signatures")
	}
	return nil
}

type samlReplayCache struct{}

func (samlReplayCache) Consume(assertionID string, expiresAt time.Time) error {
	if assertionID == "" {
		return errors.New("empty SAML assertion ID")
	}
	db := database.GetServerDB()
	if db == nil {
		return errors.New("SAML replay store is unavailable")
	}
	now := dbtime.FormatSQLTimestamp(time.Now())
	_, _ = db.Exec(`DELETE FROM saml_replay_cache WHERE expires_at <= ?`, now)
	_, err := db.Exec(`INSERT INTO saml_replay_cache (assertion_id, expires_at) VALUES (?, ?)`, assertionID, dbtime.FormatSQLTimestamp(expiresAt))
	if err != nil {
		return fmt.Errorf("record SAML assertion replay key: %w", err)
	}
	return nil
}

func (s *SAMLService) idpMetadata(ctx context.Context, p *SAMLProviderConfig) (*saml.EntityDescriptor, error) {
	cacheKey := p.Name + "\x00" + p.IDPMetadataURL + "\x00" + p.IDPMetadataXML
	s.mu.Lock()
	if entry, ok := s.metadataCache[cacheKey]; ok && time.Now().Before(entry.expires) {
		s.mu.Unlock()
		return entry.metadata, nil
	}
	s.mu.Unlock()
	var metadata *saml.EntityDescriptor
	var err error
	if strings.TrimSpace(p.IDPMetadataXML) != "" {
		metadata, err = samlsp.ParseMetadata([]byte(p.IDPMetadataXML))
	} else if p.IDPMetadataURL == "" {
		return nil, errors.New("SAML IdP metadata URL or XML is required")
	} else {
		u, parseErr := url.Parse(p.IDPMetadataURL)
		if parseErr != nil || u.Scheme != "https" || u.Host == "" {
			return nil, errors.New("SAML metadata URL must be a valid HTTPS URL")
		}
		client := &http.Client{Timeout: 10 * time.Second}
		metadata, err = samlsp.FetchMetadata(ctx, client, *u)
	}
	if err != nil {
		return nil, err
	}
	s.mu.Lock()
	if s.metadataCache == nil {
		s.metadataCache = make(map[string]samlMetadataCacheEntry)
	}
	s.metadataCache[cacheKey] = samlMetadataCacheEntry{metadata: metadata, expires: time.Now().Add(time.Hour)}
	s.mu.Unlock()
	return metadata, nil
}

// GenerateSAMLSPKeyPair creates a self-signed SP certificate and persists both
// PEM values in the secrets store, never in ordinary YAML settings.
func GenerateSAMLSPKeyPair(provider string, expiryDays int) error {
	if !validSAMLProviderName(provider) {
		return errors.New("invalid SAML provider name")
	}
	if expiryDays <= 0 || expiryDays > 36500 {
		expiryDays = 3650
	}
	key, err := rsa.GenerateKey(rand.Reader, 3072)
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return err
	}
	template := x509.Certificate{SerialNumber: serial, Subject: pkix.Name{CommonName: "wthr SAML SP " + provider}, NotBefore: now.Add(-5 * time.Minute), NotAfter: now.Add(time.Duration(expiryDays) * 24 * time.Hour), KeyUsage: x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth}, BasicConstraintsValid: true}
	der, err := x509.CreateCertificate(rand.Reader, &template, &template, &key.PublicKey, key)
	if err != nil {
		return err
	}
	keyDER, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		return err
	}
	prefix := "server.auth.saml.providers." + provider + "."
	cfg := config.GetGlobalConfig()
	if cfg == nil || strings.TrimSpace(cfg.Server.Security.EncryptionKey) == "" {
		return errors.New("SAML private-key encryption is not configured")
	}
	sealedKey, err := util.EncryptAtRest(cfg.Server.Security.EncryptionKey, string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER})))
	if err != nil {
		return err
	}
	if err := config.WriteSecret(prefix+"sp_private_key", sealedKey); err != nil {
		return err
	}
	return config.WriteSecret(prefix+"sp_certificate", string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})))
}

type SAMLSPCertificateInfo struct {
	Fingerprint string
	ExpiresAt   time.Time
}

func GetSAMLSPCertificateInfo(provider string) (*SAMLSPCertificateInfo, error) {
	certPEM, err := config.ReadSecret("server.auth.saml.providers." + provider + ".sp_certificate")
	if err != nil {
		return nil, err
	}
	cert, err := parseSAMLSPCertificate([]byte(certPEM))
	if err != nil {
		return nil, err
	}
	return samlSPCertificateInfo(cert), nil
}

func GetSAMLSPCertificateInfoForConfig(provider *SAMLProviderConfig) (*SAMLSPCertificateInfo, error) {
	if provider == nil {
		return nil, errors.New("SAML provider is required")
	}
	if strings.TrimSpace(provider.SPCertPath) != "" || strings.TrimSpace(provider.SPKeyPath) != "" {
		certPEM, err := readSAMLSPFile(provider.SPCertPath, false)
		if err != nil {
			return nil, err
		}
		keyPEM, err := readSAMLSPFile(provider.SPKeyPath, true)
		if err != nil {
			return nil, err
		}
		key, err := parseSAMLPrivateKey(keyPEM)
		if err != nil {
			return nil, err
		}
		cert, err := parseSAMLSPCertificate(certPEM)
		if err != nil {
			return nil, err
		}
		if err := validateSAMLKeyPairMaterial(key, cert); err != nil {
			return nil, err
		}
		return samlSPCertificateInfo(cert), nil
	}
	return GetSAMLSPCertificateInfo(provider.Name)
}

func samlSPCertificateInfo(cert *x509.Certificate) *SAMLSPCertificateInfo {
	fingerprint := sha256.Sum256(cert.Raw)
	encoded := strings.ToUpper(hex.EncodeToString(fingerprint[:]))
	var spaced strings.Builder
	for i := 0; i < len(encoded); i += 2 {
		if spaced.Len() > 0 {
			spaced.WriteByte(':')
		}
		spaced.WriteString(encoded[i : i+2])
	}
	return &SAMLSPCertificateInfo{Fingerprint: spaced.String(), ExpiresAt: cert.NotAfter}
}

func MarshalSAMLMetadata(sp *saml.ServiceProvider) ([]byte, error) {
	if sp == nil {
		return nil, errors.New("SAML service provider is required")
	}
	return xml.MarshalIndent(sp.Metadata(), "", "  ")
}
