package urlutil

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strings"
	"time"
)

// FetchRemoteImageConfig limits remote image requests and their accepted content.
type FetchRemoteImageConfig struct {
	MaxSize        int64
	Timeout        time.Duration
	AllowedTypes   []string
	AllowedSchemes []string
}

// DefaultFetchRemoteImageConfig returns production-safe image fetch limits.
func DefaultFetchRemoteImageConfig() FetchRemoteImageConfig {
	return FetchRemoteImageConfig{MaxSize: 10 << 20, Timeout: 30 * time.Second, AllowedTypes: []string{"image/png", "image/jpeg", "image/gif", "image/webp", "image/x-icon"}, AllowedSchemes: []string{"https"}}
}

func normalizeConfig(cfg FetchRemoteImageConfig) FetchRemoteImageConfig {
	defaults := DefaultFetchRemoteImageConfig()
	if cfg.MaxSize <= 0 {
		cfg.MaxSize = defaults.MaxSize
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = defaults.Timeout
	}
	if len(cfg.AllowedTypes) == 0 {
		cfg.AllowedTypes = defaults.AllowedTypes
	}
	if len(cfg.AllowedSchemes) == 0 {
		cfg.AllowedSchemes = defaults.AllowedSchemes
	}
	return cfg
}

// ValidateRemoteURL rejects schemes, hosts and addresses that can reach local services.
func ValidateRemoteURL(rawURL string, cfg FetchRemoteImageConfig) error {
	cfg = normalizeConfig(cfg)
	u, err := url.ParseRequestURI(rawURL)
	if err != nil {
		return fmt.Errorf("invalid URL: %w", err)
	}
	if u.User != nil || u.Hostname() == "" {
		return fmt.Errorf("URL must have a host and no user information")
	}
	allowed := false
	for _, scheme := range cfg.AllowedSchemes {
		if strings.EqualFold(scheme, u.Scheme) {
			allowed = true
			break
		}
	}
	if !allowed {
		return fmt.Errorf("scheme not allowed: %s", u.Scheme)
	}
	host := strings.TrimSuffix(strings.ToLower(u.Hostname()), ".")
	if host == "localhost" || strings.HasSuffix(host, ".localhost") || strings.HasSuffix(host, ".local") || strings.HasSuffix(host, ".internal") {
		return fmt.Errorf("local host not allowed")
	}
	ctx, cancel := context.WithTimeout(context.Background(), cfg.Timeout)
	defer cancel()
	ips, err := net.DefaultResolver.LookupNetIP(ctx, "ip", host)
	if err != nil || len(ips) == 0 {
		return fmt.Errorf("DNS lookup failed for host")
	}
	for _, ip := range ips {
		if !publicIP(ip) {
			return fmt.Errorf("non-public address not allowed")
		}
	}
	return nil
}

func publicIP(ip netip.Addr) bool {
	return ip.IsValid() && ip.IsGlobalUnicast() && !ip.IsPrivate() && !ip.IsLoopback() && !ip.IsLinkLocalUnicast() && !ip.IsLinkLocalMulticast() && !ip.IsMulticast() && !ip.IsUnspecified()
}

// FetchRemoteImage safely fetches an image, validating every redirect and resolved address.
func FetchRemoteImage(ctx context.Context, rawURL string, cfg FetchRemoteImageConfig) ([]byte, string, error) {
	cfg = normalizeConfig(cfg)
	if err := ValidateRemoteURL(rawURL, cfg); err != nil {
		return nil, "", fmt.Errorf("URL validation failed: %w", err)
	}
	dialer := &net.Dialer{Timeout: cfg.Timeout, KeepAlive: 30 * time.Second}
	transport := &http.Transport{DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
		host, port, err := net.SplitHostPort(address)
		if err != nil {
			return nil, err
		}
		ips, err := net.DefaultResolver.LookupNetIP(ctx, "ip", host)
		if err != nil {
			return nil, err
		}
		for _, ip := range ips {
			if !publicIP(ip) {
				return nil, fmt.Errorf("non-public address not allowed")
			}
		}
		var lastErr error
		for _, ip := range ips {
			conn, dialErr := dialer.DialContext(ctx, network, net.JoinHostPort(ip.String(), port))
			if dialErr == nil {
				return conn, nil
			}
			lastErr = dialErr
		}
		return nil, lastErr
	}}
	client := &http.Client{Transport: transport, Timeout: cfg.Timeout, CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if len(via) >= 5 {
			return fmt.Errorf("too many redirects")
		}
		return ValidateRemoteURL(req.URL.String(), cfg)
	}}
	defer transport.CloseIdleConnections()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, "", err
	}
	req.Header.Set("User-Agent", "wthr/1.0")
	req.Header.Set("Accept", strings.Join(cfg.AllowedTypes, ", "))
	resp, err := client.Do(req)
	if err != nil {
		return nil, "", fmt.Errorf("fetching URL: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, "", fmt.Errorf("unexpected status: %d", resp.StatusCode)
	}
	if resp.ContentLength > cfg.MaxSize {
		return nil, "", fmt.Errorf("file too large (max %d bytes)", cfg.MaxSize)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, cfg.MaxSize+1))
	if err != nil {
		return nil, "", err
	}
	if int64(len(data)) > cfg.MaxSize {
		return nil, "", fmt.Errorf("file too large (max %d bytes)", cfg.MaxSize)
	}
	contentType := detectedImageContentType(data)
	allowed := false
	for _, candidate := range cfg.AllowedTypes {
		if strings.EqualFold(candidate, contentType) {
			allowed = true
			break
		}
	}
	if !allowed {
		return nil, "", fmt.Errorf("content type not allowed: %s", contentType)
	}
	declared := strings.TrimSpace(strings.SplitN(resp.Header.Get("Content-Type"), ";", 2)[0])
	if declared != "" && !strings.EqualFold(declared, contentType) {
		return nil, "", fmt.Errorf("content type mismatch: declared %s, detected %s", declared, contentType)
	}
	return data, contentType, nil
}

func detectedImageContentType(data []byte) string {
	contentType := strings.SplitN(http.DetectContentType(data), ";", 2)[0]
	if contentType == "image/vnd.microsoft.icon" {
		return "image/x-icon"
	}
	return contentType
}
