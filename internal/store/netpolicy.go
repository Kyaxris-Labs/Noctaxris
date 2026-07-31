package store

import "net"

// LabForwardIPDenied blocks unspecified and link-local addresses (incl. cloud metadata).
// Loopback stays allowed for same-host lab backends when the API is loopback-bound.
// RFC1918 private addresses stay allowed so nested DinD instance IPs can be targets.
func LabForwardIPDenied(ip net.IP) bool {
	if ip == nil {
		return true
	}
	return ip.IsUnspecified() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast()
}
