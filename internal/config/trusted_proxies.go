package config

import (
	"fmt"
	"net"
	"strings"
)

// EnvTrustedProxies is a comma-separated list of CIDRs (or single IPs) that may
// supply a trustworthy X-Forwarded-For hop for WAF SourceIP and (with
// NOCTAXRIS_CLOUDTRAIL_TRUST_XFF) audit sourceIPAddress.
const EnvTrustedProxies = "NOCTAXRIS_TRUSTED_PROXIES"

// ParseTrustedProxies parses comma-separated CIDRs or bare IPs into IPNets.
// Empty input returns nil (no trusted proxies; XFF ignored for enforcement).
func ParseTrustedProxies(raw string) ([]*net.IPNet, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	parts := strings.Split(raw, ",")
	out := make([]*net.IPNet, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		if !strings.Contains(p, "/") {
			ip := net.ParseIP(p)
			if ip == nil {
				return nil, fmt.Errorf("invalid trusted proxy IP %q", p)
			}
			bits := 128
			if ip.To4() != nil {
				bits = 32
			}
			p = fmt.Sprintf("%s/%d", ip.String(), bits)
		}
		_, n, err := net.ParseCIDR(p)
		if err != nil {
			return nil, fmt.Errorf("invalid trusted proxy CIDR %q: %w", p, err)
		}
		out = append(out, n)
	}
	return out, nil
}

// PeerInTrustedProxies reports whether hostIP is inside any trusted CIDR.
// Empty trusted list is never trusted.
func PeerInTrustedProxies(hostIP string, trusted []*net.IPNet) bool {
	if len(trusted) == 0 {
		return false
	}
	ip := net.ParseIP(strings.TrimSpace(hostIP))
	if ip == nil {
		return false
	}
	for _, n := range trusted {
		if n != nil && n.Contains(ip) {
			return true
		}
	}
	return false
}
