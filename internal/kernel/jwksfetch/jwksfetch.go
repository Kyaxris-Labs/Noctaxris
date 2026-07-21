// Package jwksfetch loads JWKS for JWT authorizers with fail-closed SSRF controls.
package jwksfetch

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

const (
	// EnvAllowRemote enables optional remote JWKS fetch (default off).
	EnvAllowRemote = "NOCTAXRIS_ALLOW_REMOTE_JWKS"
	// EnvHostAllowlist is a comma-separated host allowlist used only when remote JWKS is enabled.
	// Entries are host or host:port (no scheme). Empty allowlist means no remote hosts are accepted.
	EnvHostAllowlist = "NOCTAXRIS_JWKS_HOST_ALLOWLIST"

	labCognitoIssuerPrefix = "http://127.0.0.1:4566/cognito-idp/"
)

// ParseLabCognitoIssuer returns region and pool ID when issuer is a lab Cognito URL or path.
func ParseLabCognitoIssuer(issuer string) (region, poolID string, ok bool) {
	issuer = strings.TrimRight(strings.TrimSpace(issuer), "/")
	if strings.HasPrefix(issuer, labCognitoIssuerPrefix) {
		rest := strings.TrimPrefix(issuer, labCognitoIssuerPrefix)
		parts := strings.Split(rest, "/")
		if len(parts) == 2 && parts[0] != "" && parts[1] != "" {
			return parts[0], parts[1], true
		}
		return "", "", false
	}
	const pathPrefix = "/cognito-idp/"
	if strings.HasPrefix(issuer, pathPrefix) {
		rest := strings.TrimPrefix(issuer, pathPrefix)
		parts := strings.Split(rest, "/")
		if len(parts) == 2 && parts[0] != "" && parts[1] != "" {
			return parts[0], parts[1], true
		}
	}
	return "", "", false
}

// IsLabCognitoIssuer reports whether issuer is an in-process lab Cognito shape.
func IsLabCognitoIssuer(issuer string) bool {
	_, _, ok := ParseLabCognitoIssuer(issuer)
	return ok
}

// RemoteAllowed reports whether NOCTAXRIS_ALLOW_REMOTE_JWKS enables remote fetch.
func RemoteAllowed() bool {
	v := strings.TrimSpace(os.Getenv(EnvAllowRemote))
	return v == "1" || strings.EqualFold(v, "true") || strings.EqualFold(v, "yes")
}

// ValidateIssuerForConfig rejects non-lab issuers unless remote JWKS is explicitly enabled
// and the issuer host passes the remote allowlist (no private/link-local/metadata).
func ValidateIssuerForConfig(issuer string) error {
	issuer = strings.TrimRight(strings.TrimSpace(issuer), "/")
	if issuer == "" {
		return fmt.Errorf("jwksfetch: issuer required")
	}
	if IsLabCognitoIssuer(issuer) {
		return nil
	}
	if !RemoteAllowed() {
		return fmt.Errorf("jwksfetch: remote JWKS issuer rejected (set %s=1 and %s for lab escape hatch)", EnvAllowRemote, EnvHostAllowlist)
	}
	return validateRemoteIssuerURL(issuer)
}

// FetchRemoteJWKS GETs issuer/.well-known/jwks.json under the remote allowlist.
// Callers must use in-process Cognito JWKS for lab issuers instead of this function.
func FetchRemoteJWKS(issuer string, client *http.Client) ([]byte, error) {
	issuer = strings.TrimRight(strings.TrimSpace(issuer), "/")
	if err := ValidateIssuerForConfig(issuer); err != nil {
		return nil, err
	}
	if IsLabCognitoIssuer(issuer) {
		return nil, fmt.Errorf("jwksfetch: lab Cognito issuer must use in-process JWKS")
	}
	if client == nil {
		client = secureJWKSClient()
	}
	jwksURL := issuer + "/.well-known/jwks.json"
	resp, err := client.Get(jwksURL)
	if err != nil {
		return nil, fmt.Errorf("jwksfetch: jwks unreachable: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("jwksfetch: jwks status %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, fmt.Errorf("jwksfetch: jwks read: %w", err)
	}
	return body, nil
}

func secureJWKSClient() *http.Client {
	base, ok := http.DefaultTransport.(*http.Transport)
	var transport *http.Transport
	if ok {
		transport = base.Clone()
	} else {
		transport = &http.Transport{}
	}
	transport.DialContext = pinnedSafeDialContext
	return &http.Client{
		Timeout:   5 * time.Second,
		Transport: transport,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return fmt.Errorf("jwksfetch: redirects are not allowed")
		},
	}
}

// pinnedSafeDialContext resolves addr, rejects unsafe IPs at dial time, and connects
// only to a validated address (mitigates DNS rebinding between check and connect).
func pinnedSafeDialContext(ctx context.Context, network, addr string) (net.Conn, error) {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return nil, fmt.Errorf("jwksfetch: dial addr: %w", err)
	}
	ips, err := net.LookupIP(host)
	if err != nil {
		if ip := net.ParseIP(host); ip != nil {
			ips = []net.IP{ip}
		} else {
			return nil, fmt.Errorf("jwksfetch: resolve dial host: %w", err)
		}
	}
	if len(ips) == 0 {
		return nil, fmt.Errorf("jwksfetch: dial host resolved to no addresses")
	}
	var dialer net.Dialer
	var lastErr error
	for _, ip := range ips {
		if err := rejectUnsafeIP(ip); err != nil {
			lastErr = err
			continue
		}
		conn, err := dialer.DialContext(ctx, network, net.JoinHostPort(ip.String(), port))
		if err == nil {
			return conn, nil
		}
		lastErr = err
	}
	if lastErr != nil {
		return nil, lastErr
	}
	return nil, fmt.Errorf("jwksfetch: no safe dial target")
}

func validateRemoteIssuerURL(issuer string) error {
	u, err := url.Parse(issuer)
	if err != nil {
		return fmt.Errorf("jwksfetch: invalid issuer URL: %w", err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("jwksfetch: issuer scheme must be http or https")
	}
	if u.Host == "" {
		return fmt.Errorf("jwksfetch: issuer host required")
	}
	if u.User != nil {
		return fmt.Errorf("jwksfetch: issuer must not include userinfo")
	}
	host := u.Hostname()
	if host == "" {
		return fmt.Errorf("jwksfetch: issuer host required")
	}
	if !hostOnAllowlist(u.Host) {
		return fmt.Errorf("jwksfetch: issuer host %q not on %s", u.Host, EnvHostAllowlist)
	}
	if err := rejectUnsafeHost(host); err != nil {
		return err
	}
	return nil
}

func hostOnAllowlist(hostPort string) bool {
	raw := strings.TrimSpace(os.Getenv(EnvHostAllowlist))
	if raw == "" {
		return false
	}
	want := strings.ToLower(strings.TrimSpace(hostPort))
	for _, part := range strings.Split(raw, ",") {
		entry := strings.ToLower(strings.TrimSpace(part))
		if entry == "" {
			continue
		}
		if entry == want {
			return true
		}
		// Allow matching hostname when allowlist entry omits an explicit default port.
		if h, _, err := net.SplitHostPort(want); err == nil && strings.EqualFold(h, entry) {
			return true
		}
	}
	return false
}

func rejectUnsafeHost(host string) error {
	host = strings.TrimSpace(host)
	lower := strings.ToLower(host)
	if lower == "localhost" || strings.HasSuffix(lower, ".localhost") {
		return fmt.Errorf("jwksfetch: localhost JWKS hosts are not allowed for remote fetch")
	}
	if lower == "metadata.google.internal" || strings.Contains(lower, "metadata") {
		return fmt.Errorf("jwksfetch: metadata hosts are not allowed")
	}
	ips, err := net.LookupIP(host)
	if err != nil {
		// Literal IP parse fallback when DNS fails (offline tests use literals).
		if ip := net.ParseIP(host); ip != nil {
			return rejectUnsafeIP(ip)
		}
		return fmt.Errorf("jwksfetch: resolve issuer host: %w", err)
	}
	if len(ips) == 0 {
		return fmt.Errorf("jwksfetch: issuer host resolved to no addresses")
	}
	for _, ip := range ips {
		if err := rejectUnsafeIP(ip); err != nil {
			return err
		}
	}
	return nil
}

func rejectUnsafeIP(ip net.IP) error {
	if ip == nil {
		return fmt.Errorf("jwksfetch: invalid IP")
	}
	if ip.IsLoopback() {
		return fmt.Errorf("jwksfetch: loopback JWKS targets are not allowed for remote fetch")
	}
	if ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsMulticast() || ip.IsUnspecified() {
		return fmt.Errorf("jwksfetch: private or link-local JWKS targets are not allowed")
	}
	// Cloud metadata link-local (covered by IsLinkLocalUnicast for 169.254.0.0/16).
	if ip4 := ip.To4(); ip4 != nil && ip4[0] == 169 && ip4[1] == 254 {
		return fmt.Errorf("jwksfetch: link-local JWKS targets are not allowed")
	}
	return nil
}
