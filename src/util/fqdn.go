package utils

import (
	"net"
	"net/url"
	"strings"

	"golang.org/x/net/publicsuffix"
)

var devOnlyTLDs = map[string]bool{
	"localhost": true, "test": true, "example": true, "invalid": true,
	"local": true, "lan": true, "internal": true, "home": true,
	"localdomain": true, "home.arpa": true, "intranet": true,
	"corp": true, "private": true,
}

// IsValidHost validates a fully qualified domain name (TEMPLATE.md compliant)
// devMode: true for development mode, false for production
// projectName: project name for dynamic dev TLD validation (e.g., "weather")
func IsValidHost(host string, devMode bool, projectName string) bool {
	lower := strings.ToLower(strings.TrimSpace(host))

	// Remove any protocol prefix
	lower = strings.TrimPrefix(lower, "http://")
	lower = strings.TrimPrefix(lower, "https://")

	// Remove path and query string if present
	if idx := strings.IndexAny(lower, "/?#"); idx != -1 {
		lower = lower[:idx]
	}

	// Remove port if present
	if idx := strings.LastIndex(lower, ":"); idx != -1 {
		lower = lower[:idx]
	}

	// Reject empty
	if lower == "" {
		return false
	}

	// Reject IP addresses always
	if net.ParseIP(lower) != nil {
		return false
	}

	// Handle localhost
	if lower == "localhost" {
		return devMode
	}

	// Must contain at least one dot (except localhost)
	if !strings.Contains(lower, ".") {
		return false
	}

	// Cannot start or end with dot or hyphen
	if strings.HasPrefix(lower, ".") || strings.HasSuffix(lower, ".") ||
		strings.HasPrefix(lower, "-") || strings.HasSuffix(lower, "-") {
		return false
	}

	// Overlay network TLDs - valid but app-managed (not set via DOMAIN)
	if strings.HasSuffix(lower, ".onion") ||
		strings.HasSuffix(lower, ".i2p") ||
		strings.HasSuffix(lower, ".exit") {
		return true
	}

	// Check dynamic project-specific TLD (e.g., app.weather, dev.weather)
	if projectName != "" && strings.HasSuffix(lower, "."+strings.ToLower(projectName)) {
		// Project TLDs only valid in dev mode
		return devMode
	}

	// Get the public suffix (TLD or eTLD like co.uk)
	suffix, icann := publicsuffix.PublicSuffix(lower)

	// Check if it's a dev-only TLD
	if devOnlyTLDs[suffix] {
		// Dev TLDs only valid in dev mode
		return devMode
	}

	// In production, require valid ICANN TLD
	if !devMode && !icann {
		return false
	}

	// Verify we have at least eTLD+1 (not just the suffix itself)
	etldPlusOne, err := publicsuffix.EffectiveTLDPlusOne(lower)
	if err != nil {
		return false
	}

	// Host must be at least eTLD+1 (e.g., "domain.co.uk" not just "co.uk")
	return len(etldPlusOne) > 0
}

// ValidateFQDN validates a domain (backward compatibility wrapper)
// Defaults to production mode with no project name
func ValidateFQDN(domain string) bool {
	return IsValidHost(domain, false, "")
}

// ValidateURL validates a URL and extracts the domain for FQDN validation
func ValidateURL(rawURL string, devMode bool, projectName string) (bool, string, error) {
	// Parse the URL
	parsedURL, err := url.Parse(rawURL)
	if err != nil {
		return false, "", err
	}

	// Must have a scheme
	if parsedURL.Scheme == "" {
		return false, "", nil
	}

	// Must have a host
	if parsedURL.Host == "" {
		return false, "", nil
	}

	// Extract hostname (without port)
	hostname := parsedURL.Hostname()

	// Validate the hostname as FQDN
	valid := IsValidHost(hostname, devMode, projectName)

	return valid, hostname, nil
}

// GetPublicSuffix returns the public suffix (TLD) of a domain
func GetPublicSuffix(domain string) string {
	suffix, _ := publicsuffix.PublicSuffix(domain)
	return suffix
}

// GetEffectiveTLDPlusOne returns the eTLD+1 (e.g., "example.com" from "www.example.com")
func GetEffectiveTLDPlusOne(domain string) string {
	eTLD, err := publicsuffix.EffectiveTLDPlusOne(domain)
	if err != nil {
		return ""
	}
	return eTLD
}
