package util

import (
	"fmt"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
)

// defaultHTTPPorts are the ports never included in a built URL: the scheme's
// own well-known port carries no information for the client (AI.md PART 12).
var defaultHTTPPorts = map[string]string{
	"http":  "80",
	"https": "443",
}

// GetURLVars returns the resolved proto, fqdn, and port for a request, honoring
// reverse-proxy headers first and gated by the trusted_proxies allow-list
// (AI.md PART 12 → "Resolution Order"). Resolution order is:
//
//  1. Overlay network host (Tor/I2P) if the request arrived over one
//  2. X-Forwarded-Proto / X-Forwarded-Host / X-Forwarded-Port, when the
//     immediate TCP peer is a trusted proxy
//  3. The request's own TLS state, Host header, and listener address
//
// The returned port is "" when it is the scheme's default (80 for http, 443
// for https), so callers never emit a redundant ":80" or ":443". Both proto and
// fqdn are lowercased: Host headers are case-insensitive and a mixed-case value
// would otherwise render a URL the client treats as a different origin.
//
// These values are resolved per request and must never be cached at startup:
// the URL a client sees has to match the Host/proto that client actually used,
// or rendered links disagree with what the client can reach (AI.md PART 12).
func GetURLVars(r *http.Request) (proto, fqdn, port string) {
	proto = "http"
	if r != nil && TrustedIsHTTPS(r) {
		proto = "https"
	}

	fqdn = ""
	if r != nil {
		fqdn = TrustedGetHostFromRequest(r)
	}
	if fqdn == "" {
		fqdn = GetFQDN()
	}
	fqdn = strings.ToLower(strings.TrimSpace(fqdn))

	// A Host header may carry an explicit port ("example.com:8080"). Split it
	// off so the hostname and the port are decided separately - an IPv6 literal
	// has colons of its own and must not be split naively.
	if h, p, err := net.SplitHostPort(fqdn); err == nil {
		fqdn, port = strings.ToLower(strings.TrimSpace(h)), p
	}
	// Strip the brackets from an IPv6 literal: they belong in URLs, not in the
	// fqdn a caller may interpolate into other contexts.
	fqdn = strings.TrimSuffix(strings.TrimSuffix(fqdn, "]"), "[")

	// Never build security-sensitive absolute URLs from an arbitrary Host header.
	// Even forwarded hosts are accepted only when the peer is trusted, and must
	// still match a configured canonical hostname exactly.
	if !isConfiguredURLHost(fqdn) {
		fqdn = strings.ToLower(strings.TrimSuffix(strings.TrimSpace(GetFQDN()), "."))
		port = ""
	}

	// A trusted proxy's X-Forwarded-Port wins over anything derived from Host:
	// a proxy on :443 forwarding to a listener on :8080 sends Host without a
	// port, so the listener port would otherwise be reported to the client.
	if r != nil && isTrustedPeer(r.RemoteAddr) {
		if p := strings.TrimSpace(r.Header.Get("X-Forwarded-Port")); p != "" {
			port = p
		}
	}

	if port == defaultHTTPPorts[proto] {
		port = ""
	}
	if portSeparatorFor(fqdn, port) == "" {
		port = ""
	}
	return proto, fqdn, port
}

// isConfiguredURLHost accepts only canonical hosts from DOMAIN (including its
// comma-separated aliases), the configured machine hostname, or an IP literal
// explicitly present in one of those configured values. Arbitrary Host-header
// IPs are rejected to prevent host-header injection.
func isConfiguredURLHost(host string) bool {
	host = strings.ToLower(strings.TrimSuffix(strings.TrimSpace(host), "."))
	if host == "" {
		return false
	}
	candidates := []string{os.Getenv("DOMAIN"), os.Getenv("HOSTNAME")}
	if machine, err := os.Hostname(); err == nil {
		candidates = append(candidates, machine)
	}
	for _, raw := range candidates {
		for _, candidate := range strings.Split(raw, ",") {
			candidate = strings.ToLower(strings.TrimSuffix(strings.TrimSpace(candidate), "."))
			if candidate != "" && candidate == host {
				return true
			}
		}
	}
	return false
}

// BuildURL constructs an absolute URL for path using the proto, host, and port
// this request actually arrived on, with the scheme's default port stripped
// (AI.md PART 12). Use it for every user-visible or outbound URL - email
// verification links, OAuth callbacks, rendered templates, Swagger servers,
// GraphQL URLs, and the generated well-known files - so those URLs can never
// disagree with what the requesting client can reach.
//
// Bare paths are correct only for internal router registration (AI.md PART 12
// → "Exception - Internal routing only").
func BuildURL(r *http.Request, path string) string {
	proto, fqdn, port := GetURLVars(r)
	if !strings.HasPrefix(path, "/") && path != "" {
		path = "/" + path
	}
	if port == "" {
		return fmt.Sprintf("%s://%s%s", proto, fqdn, path)
	}
	return fmt.Sprintf("%s://%s%s%s", proto, fqdn, portSeparatorFor(fqdn, port), path)
}

// portSeparatorFor renders the ":port" suffix for a host, bracketing the host
// when it is an IPv6 literal, since a bare "::1:8080" is not parseable as
// host:port in a URL. Returns "" when port is empty or not a plain number: a
// malformed X-Forwarded-Port must never be interpolated into a URL, since it
// could inject a "?" or "/" and rewrite the whole URL.
func portSeparatorFor(fqdn, port string) string {
	if port == "" {
		return ""
	}
	if _, err := strconv.Atoi(port); err != nil {
		return ""
	}
	if ip := net.ParseIP(strings.Trim(fqdn, "[]")); ip != nil && ip.To4() == nil {
		return ":" + port
	}
	return net.JoinHostPort(fqdn, port)[len(fqdn):]
}
