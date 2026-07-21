package config

import (
	"fmt"
	"net"
	"os"
	"strings"
)

const (
	// EnvAllowNonLoopbackListen permits binding non-loopback addresses without TLS
	// (Compose container bind with host publish restricted to 127.0.0.1).
	EnvAllowNonLoopbackListen = "NOCTAXRIS_ALLOW_NONLOOPBACK_LISTEN"
	// EnvAllowOpenDataPlane permits Function URL / HTTP API AuthorizationType NONE
	// when listen is non-loopback. Loopback listen allows NONE without this env.
	EnvAllowOpenDataPlane = "NOCTAXRIS_ALLOW_OPEN_DATA_PLANE"
)

// ListenIsLoopback reports whether addr binds only loopback (or is empty / port-only).
func ListenIsLoopback(addr string) bool {
	addr = strings.TrimSpace(addr)
	if addr == "" {
		return true
	}
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		// ":4566" or bare hostname
		if strings.HasPrefix(addr, ":") {
			return true
		}
		host = addr
	}
	host = strings.Trim(host, "[]")
	if host == "" || strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	if ip == nil {
		return false
	}
	return ip.IsLoopback()
}

// TLSEnabled reports whether both TLS PEM paths are set.
func (c Config) TLSEnabled() bool {
	return strings.TrimSpace(c.TLSCertFile) != "" && strings.TrimSpace(c.TLSKeyFile) != ""
}

// ValidateListenSecurity fails closed when listen is a concrete non-loopback
// address without TLS, unless NOCTAXRIS_ALLOW_NONLOOPBACK_LISTEN=1.
// Wildcard binds (0.0.0.0 / ::) also require TLS or that opt-in (Compose sets it).
func ValidateListenSecurity(c Config) error {
	if ListenIsLoopback(c.ListenAddr) {
		return nil
	}
	if c.TLSEnabled() {
		return nil
	}
	if strings.TrimSpace(os.Getenv(EnvAllowNonLoopbackListen)) == "1" {
		return nil
	}
	return fmt.Errorf("NOCTAXRIS_LISTEN %q is non-loopback without TLS; set NOCTAXRIS_TLS_CERT and NOCTAXRIS_TLS_KEY, or %s=1 when host publish stays loopback (Compose)",
		c.ListenAddr, EnvAllowNonLoopbackListen)
}

// OpenDataPlaneAllowed reports whether AuthType/AuthorizationType NONE is permitted.
// Loopback listen allows NONE by default. Non-loopback requires NOCTAXRIS_ALLOW_OPEN_DATA_PLANE=1.
func (c Config) OpenDataPlaneAllowed() bool {
	if strings.TrimSpace(os.Getenv(EnvAllowOpenDataPlane)) == "1" {
		return true
	}
	return ListenIsLoopback(c.ListenAddr)
}
