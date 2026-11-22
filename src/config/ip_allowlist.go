package config

import "net/netip"

// IsIPAllowlisted reports whether ip matches any configured server security
// allowlist entry.
func IsIPAllowlisted(ip string) bool {
	address, err := netip.ParseAddr(ip)
	if err != nil {
		return false
	}
	address = address.Unmap()
	cfg := GetGlobalConfig()
	if cfg == nil {
		return false
	}
	for _, entry := range cfg.Server.Security.Allowlist {
		prefix, err := netip.ParsePrefix(entry.CIDR)
		if err != nil {
			if single, parseErr := netip.ParseAddr(entry.CIDR); parseErr == nil {
				prefix = netip.PrefixFrom(single.Unmap(), single.Unmap().BitLen())
			} else {
				continue
			}
		}
		if prefix.Contains(address) {
			return true
		}
	}
	return false
}
