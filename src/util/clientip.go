package util

import (
	"net"
	"net/http"
	"net/netip"
	"strings"
)

// maxForwardedHops is how many rightmost chain hops the walk examines (AI.md PART 12).
const maxForwardedHops = 32

// autoClientIPHeaders is the "auto" header order: the first present one is read.
var autoClientIPHeaders = []string{
	"X-Forwarded-For", "Forwarded", "CF-Connecting-IP", "True-Client-IP", "X-Real-IP", "X-Client-IP",
}

// NormalizeClientIP applies the AI.md PART 12 normalization: strip the port,
// brackets and IPv6 zone, parse once, unmap IPv4-mapped IPv6. An invalid value
// returns false.
func NormalizeClientIP(raw string) (netip.Addr, bool) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return netip.Addr{}, false
	}
	if host, _, err := net.SplitHostPort(value); err == nil {
		value = host
	}
	value = strings.TrimSuffix(strings.TrimPrefix(value, "["), "]")
	if i := strings.IndexByte(value, '%'); i >= 0 {
		value = value[:i]
	}
	addr, err := netip.ParseAddr(value)
	if err != nil {
		return netip.Addr{}, false
	}
	return addr.Unmap(), true
}

// forwardedFor returns the "for=" value of every element of every Forwarded
// header instance (RFC 7239), unquoted.
func forwardedFor(values []string) []string {
	var hops []string
	for _, line := range values {
		for _, element := range strings.Split(line, ",") {
			for _, pair := range strings.Split(element, ";") {
				key, val, ok := strings.Cut(strings.TrimSpace(pair), "=")
				if !ok || !strings.EqualFold(strings.TrimSpace(key), "for") {
					continue
				}
				hops = append(hops, strings.Trim(strings.TrimSpace(val), `"`))
			}
		}
	}
	return hops
}

// chainHops joins every instance of a chain header (RFC 9110 5.3) into hops.
func chainHops(h http.Header, name string) []string {
	values := h.Values(name)
	if strings.EqualFold(name, "Forwarded") {
		return forwardedFor(values)
	}
	var hops []string
	for _, line := range values {
		for _, hop := range strings.Split(line, ",") {
			hops = append(hops, strings.TrimSpace(hop))
		}
	}
	return hops
}

// clientIPFromHeaders resolves the client address from proxy headers for a trusted
// peer: pick the header, walk the chain from the right, stop at the first hop that
// is not a trusted proxy (AI.md PART 12). found is false when no header yields a
// valid address, so the caller falls back to the peer.
func clientIPFromHeaders(h http.Header, header string, peer netip.Addr) (netip.Addr, bool) {
	names := autoClientIPHeaders
	if !strings.EqualFold(header, "auto") {
		names = []string{header}
	}
	for _, name := range names {
		hops := chainHops(h, name)
		if len(hops) == 0 {
			continue
		}
		if len(hops) > maxForwardedHops {
			hops = hops[len(hops)-maxForwardedHops:]
		}
		isChain := strings.EqualFold(name, "X-Forwarded-For") || strings.EqualFold(name, "Forwarded")
		if !isChain {
			if addr, ok := NormalizeClientIP(hops[len(hops)-1]); ok {
				return addr, true
			}
			continue
		}
		nearest := peer
		for i := len(hops) - 1; i >= 0; i-- {
			addr, ok := NormalizeClientIP(hops[i])
			if !ok {
				return nearest, true
			}
			if !isTrustedAddr(addr) {
				return addr, true
			}
			nearest = addr
		}
		return nearest, true
	}
	return netip.Addr{}, false
}
