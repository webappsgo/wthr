package util

import (
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/webappsgo/wthr/src/config"
)

// alwaysTrustedCIDRs are the private/link-local ranges AI.md PART 12 marks as
// trusted with no config: loopback, RFC 1918 IPv4, RFC 4193 IPv6 ULA, and
// link-local (IPv4 + IPv6).
var alwaysTrustedCIDRs = mustParseCIDRs(
	"127.0.0.0/8",
	"10.0.0.0/8",
	"172.16.0.0/12",
	"192.168.0.0/16",
	"fc00::/7",
	"169.254.0.0/16",
	"fe80::/10",
)

func mustParseCIDRs(cidrs ...string) []*net.IPNet {
	nets := make([]*net.IPNet, 0, len(cidrs))
	for _, c := range cidrs {
		_, n, err := net.ParseCIDR(c)
		if err != nil {
			// Constant list, verified at compile time by tests; a bad entry
			// here is a programmer error, not a runtime condition.
			panic("util: invalid trusted-proxy CIDR literal: " + c)
		}
		nets = append(nets, n)
	}
	return nets
}

// dnsTrustEntry caches the resolved IPs for a DNS-name entry in the
// `trusted_proxies.additional` allow-list, refreshed every 5 minutes per
// AI.md PART 12 ("resolved at startup and refreshed every 5 minutes").
type dnsTrustEntry struct {
	ips      []net.IP
	resolved time.Time
}

var (
	dnsTrustCacheMu sync.Mutex
	dnsTrustCache   = map[string]*dnsTrustEntry{}
)

const dnsTrustRefresh = 5 * time.Minute

// resolveDNSTrustEntry returns the cached (or freshly resolved) IPs for a
// DNS-name entry in the additional allow-list.
func resolveDNSTrustEntry(name string) []net.IP {
	dnsTrustCacheMu.Lock()
	entry, ok := dnsTrustCache[name]
	if ok && time.Since(entry.resolved) < dnsTrustRefresh {
		ips := entry.ips
		dnsTrustCacheMu.Unlock()
		return ips
	}
	dnsTrustCacheMu.Unlock()

	addrs, err := net.LookupIP(name)
	if err != nil {
		addrs = nil
	}

	dnsTrustCacheMu.Lock()
	dnsTrustCache[name] = &dnsTrustEntry{ips: addrs, resolved: time.Now()}
	dnsTrustCacheMu.Unlock()

	return addrs
}

// sameSlash24 reports whether ip is in the same IPv4 /24 as listenAddr. Used
// for the containerized reverse-proxy-sidecar pattern (AI.md PART 12). A
// wildcard/unspecified or non-IPv4 listen address makes this check a no-op.
func sameSlash24(ip net.IP, listenAddr string) bool {
	ip4 := ip.To4()
	if ip4 == nil {
		return false
	}

	host := listenAddr
	if h, _, err := net.SplitHostPort(listenAddr); err == nil {
		host = h
	}

	listenIP := net.ParseIP(host)
	if listenIP == nil {
		return false
	}
	listenIP4 := listenIP.To4()
	if listenIP4 == nil || listenIP.IsUnspecified() {
		return false
	}

	return ip4[0] == listenIP4[0] && ip4[1] == listenIP4[1] && ip4[2] == listenIP4[2]
}

// isTrustedPeer reports whether remoteAddr (the original, unrewritten TCP
// peer — see AI.md PART 12 "Middleware ordering") is allowed to set
// forwarded/real-IP headers: always-trusted private/link-local ranges, the
// same /24 as the configured listen address, or an entry in
// server.trusted_proxies.additional (IP, CIDR, or DNS name).
func isTrustedPeer(remoteAddr string) bool {
	host := remoteAddr
	if h, _, err := net.SplitHostPort(remoteAddr); err == nil {
		host = h
	}

	ip := net.ParseIP(host)
	if ip == nil {
		return false
	}

	if ip.IsLoopback() {
		return true
	}

	for _, n := range alwaysTrustedCIDRs {
		if n.Contains(ip) {
			return true
		}
	}

	cfg := config.GetGlobalConfig()
	if cfg == nil {
		return false
	}

	if cfg.Server.Address != "" && sameSlash24(ip, cfg.Server.Address) {
		return true
	}

	for _, entry := range cfg.Server.TrustedProxies.Additional {
		if entry == "" {
			continue
		}

		// IP literal.
		if entryIP := net.ParseIP(entry); entryIP != nil {
			if entryIP.Equal(ip) {
				return true
			}
			continue
		}

		// CIDR.
		if _, n, err := net.ParseCIDR(entry); err == nil {
			if n.Contains(ip) {
				return true
			}
			continue
		}

		// DNS name, resolved and cached (refreshed every 5 minutes).
		for _, resolvedIP := range resolveDNSTrustEntry(entry) {
			if resolvedIP.Equal(ip) {
				return true
			}
		}
	}

	return false
}

// TrustedGetClientIP resolves the real client IP, honoring
// X-Forwarded-*/real-IP headers only when the immediate TCP peer
// (r.RemoteAddr) passes the trusted_proxies gate (AI.md PART 5/12).
// Untrusted peers' forwarded headers are dropped and r.RemoteAddr is used
// directly — an attacker reaching the binary without going through a
// trusted proxy cannot forge their apparent client IP.
func TrustedGetClientIP(r *http.Request) string {
	if !isTrustedPeer(r.RemoteAddr) {
		if host, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
			return host
		}
		return r.RemoteAddr
	}

	return GetClientIP(r)
}
