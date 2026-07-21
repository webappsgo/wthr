// Package service provides OIDC authentication per AI.md PART 34.
package service

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	oidc "github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"

	models "github.com/webappsgo/wthr/src/server/model"
)

// OIDCProviderConfig holds configuration for one OIDC provider.
type OIDCProviderConfig struct {
	Name          string   `json:"name"`
	DisplayName   string   `json:"display_name"`
	Issuer        string   `json:"issuer"`
	ClientID      string   `json:"client_id"`
	ClientSecret  string   `json:"client_secret"`
	Scopes        []string `json:"scopes"`
	AutoRegister  bool     `json:"auto_register"`
	AdminGroups   []string `json:"admin_groups"`
	GroupsClaim   string   `json:"groups_claim"`
	UsernameClaim string   `json:"username_claim"`
	EmailClaim    string   `json:"email_claim"`
	NameClaim     string   `json:"name_claim"`
}

// OIDCClaims holds the user claims extracted from an OIDC ID token.
type OIDCClaims struct {
	Sub               string
	Email             string
	EmailVerified     bool
	Name              string
	PreferredUsername string
	Groups            []string
	RawClaims         map[string]interface{}
}

// OIDCService manages OIDC provider configuration and authentication flows.
type OIDCService struct {
	settings     *models.SettingsModel
	mu           sync.Mutex
	providerCache map[string]*oidc.Provider
	cacheExpiry  map[string]time.Time
}

// NewOIDCService creates a new OIDCService backed by the given DB.
func NewOIDCService(db *sql.DB) *OIDCService {
	return &OIDCService{
		settings:     &models.SettingsModel{DB: db},
		providerCache: make(map[string]*oidc.Provider),
		cacheExpiry:  make(map[string]time.Time),
	}
}

// Enabled returns true if OIDC authentication is enabled.
func (s *OIDCService) Enabled() bool {
	return s.settings.GetBool("server.auth.oidc.enabled", false)
}

// GetProviderConfigs reads all configured OIDC providers from server_config.
func (s *OIDCService) GetProviderConfigs() []OIDCProviderConfig {
	raw := s.settings.GetString("server.auth.oidc.providers", "")
	if raw == "" {
		return nil
	}
	var providers []OIDCProviderConfig
	if err := json.Unmarshal([]byte(raw), &providers); err != nil {
		return nil
	}
	return providers
}

// GetProviderConfig returns the config for a single named provider.
func (s *OIDCService) GetProviderConfig(name string) (*OIDCProviderConfig, error) {
	for _, p := range s.GetProviderConfigs() {
		if p.Name == name {
			cfg := p
			return &cfg, nil
		}
	}
	return nil, fmt.Errorf("OIDC provider %q not found", name)
}

// getOIDCProvider returns (and caches) the go-oidc Provider for the given issuer.
// Cache TTL is 5 minutes to pick up issuer metadata refreshes.
func (s *OIDCService) getOIDCProvider(ctx context.Context, issuer string) (*oidc.Provider, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if p, ok := s.providerCache[issuer]; ok {
		if time.Now().Before(s.cacheExpiry[issuer]) {
			return p, nil
		}
	}
	p, err := oidc.NewProvider(ctx, issuer)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize OIDC provider for %q: %w", issuer, err)
	}
	s.providerCache[issuer] = p
	s.cacheExpiry[issuer] = time.Now().Add(5 * time.Minute)
	return p, nil
}

// oauth2Config builds an oauth2.Config for the given provider.
func (s *OIDCService) oauth2Config(cfg *OIDCProviderConfig, oidcProvider *oidc.Provider, redirectURL string) *oauth2.Config {
	scopes := cfg.Scopes
	if len(scopes) == 0 {
		scopes = []string{oidc.ScopeOpenID, "profile", "email"}
	}
	return &oauth2.Config{
		ClientID:     cfg.ClientID,
		ClientSecret: cfg.ClientSecret,
		RedirectURL:  redirectURL,
		Endpoint:     oidcProvider.Endpoint(),
		Scopes:       scopes,
	}
}

// GenerateState returns a cryptographically random hex state string.
func GenerateState() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// GenerateCodeVerifier returns a PKCE code_verifier (43–128 chars, Base64URL).
func GenerateCodeVerifier() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// CodeChallenge computes the PKCE S256 code_challenge from a verifier.
func CodeChallenge(verifier string) string {
	h := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(h[:])
}

// AuthURL builds the provider's authorization URL.
// state and codeVerifier must be stored (e.g., in signed cookies) by the caller.
func (s *OIDCService) AuthURL(ctx context.Context, providerName, redirectURL, state, codeVerifier string) (string, error) {
	cfg, err := s.GetProviderConfig(providerName)
	if err != nil {
		return "", err
	}
	oidcProvider, err := s.getOIDCProvider(ctx, cfg.Issuer)
	if err != nil {
		return "", err
	}
	o2cfg := s.oauth2Config(cfg, oidcProvider, redirectURL)
	challenge := CodeChallenge(codeVerifier)
	return o2cfg.AuthCodeURL(
		state,
		oauth2.S256ChallengeOption(challenge),
	), nil
}

// ExchangeAndVerify exchanges the authorization code for tokens, verifies the ID token,
// and returns the extracted claims.
func (s *OIDCService) ExchangeAndVerify(ctx context.Context, providerName, redirectURL, code, codeVerifier string) (*OIDCClaims, error) {
	cfg, err := s.GetProviderConfig(providerName)
	if err != nil {
		return nil, err
	}
	oidcProvider, err := s.getOIDCProvider(ctx, cfg.Issuer)
	if err != nil {
		return nil, err
	}
	o2cfg := s.oauth2Config(cfg, oidcProvider, redirectURL)

	ctx2, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	token, err := o2cfg.Exchange(ctx2, code, oauth2.VerifierOption(codeVerifier))
	if err != nil {
		return nil, fmt.Errorf("code exchange failed: %w", err)
	}

	rawIDToken, ok := token.Extra("id_token").(string)
	if !ok {
		return nil, fmt.Errorf("no id_token in response")
	}

	verifier := oidcProvider.Verifier(&oidc.Config{ClientID: cfg.ClientID})
	idToken, err := verifier.Verify(ctx2, rawIDToken)
	if err != nil {
		return nil, fmt.Errorf("id_token verification failed: %w", err)
	}

	var raw map[string]interface{}
	if err := idToken.Claims(&raw); err != nil {
		return nil, fmt.Errorf("failed to extract claims: %w", err)
	}

	claims := &OIDCClaims{
		Sub:       idToken.Subject,
		RawClaims: raw,
	}

	usernameClaim := cfg.UsernameClaim
	if usernameClaim == "" {
		usernameClaim = "preferred_username"
	}
	emailClaim := cfg.EmailClaim
	if emailClaim == "" {
		emailClaim = "email"
	}
	nameClaim := cfg.NameClaim
	if nameClaim == "" {
		nameClaim = "name"
	}
	groupsClaim := cfg.GroupsClaim
	if groupsClaim == "" {
		groupsClaim = "groups"
	}

	if v, ok := raw[emailClaim].(string); ok {
		claims.Email = v
	}
	if v, ok := raw["email_verified"].(bool); ok {
		claims.EmailVerified = v
	}
	if v, ok := raw[nameClaim].(string); ok {
		claims.Name = v
	}
	if v, ok := raw[usernameClaim].(string); ok {
		claims.PreferredUsername = v
	}
	if v, ok := raw[groupsClaim]; ok {
		switch g := v.(type) {
		case []interface{}:
			for _, item := range g {
				if s, ok := item.(string); ok {
					claims.Groups = append(claims.Groups, s)
				}
			}
		case []string:
			claims.Groups = g
		}
	}

	return claims, nil
}

// IsAdminGroup returns true if any of the user's groups match an admin group in the provider config.
func (s *OIDCService) IsAdminGroup(cfg *OIDCProviderConfig, userGroups []string) bool {
	for _, adminGroup := range cfg.AdminGroups {
		for _, userGroup := range userGroups {
			if strings.EqualFold(adminGroup, userGroup) {
				return true
			}
		}
	}
	return false
}
