package store

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

// EnvAPIGatewayHTTPProxy enables HTTP_PROXY / VPC_LINK integrations when set to "1".
const EnvAPIGatewayHTTPProxy = "NOCTAXRIS_APIGW_HTTP_PROXY"

// EnvAPIGatewayHTTPProxyAllowlist is a comma-separated list of hosts or URL prefixes
// allowed as HTTP_PROXY / VPC_LINK IntegrationUri destinations when proxy is on.
const EnvAPIGatewayHTTPProxyAllowlist = "NOCTAXRIS_APIGW_HTTP_PROXY_ALLOWLIST"

const (
	APIGatewayIntegrationHTTPProxy = "HTTP_PROXY"
	APIGatewayIntegrationVPCLink   = "VPC_LINK"
)

const apigwHTTPProxyMaxBody = 1 << 20 // 1 MiB lab cap

// APIGatewayHTTPProxyEnabled reports whether opt-in HTTP_PROXY / VPC_LINK is on.
func APIGatewayHTTPProxyEnabled() bool {
	return strings.TrimSpace(os.Getenv(EnvAPIGatewayHTTPProxy)) == "1"
}

// ValidateAPIGatewayHTTPProxyURI checks IntegrationUri for HTTP_PROXY / VPC_LINK.
// Fail-closed when env unset; when on, destination must match allowlist.
// Link-local / metadata / loopback / private hosts are rejected unless the matching
// allowlist entry itself names that unsafe host (or URL prefix to it).
// Shared by HTTP API and REST PutIntegration callers.
func ValidateAPIGatewayHTTPProxyURI(integrationURI string) error {
	if !APIGatewayHTTPProxyEnabled() {
		return fmt.Errorf("%w: HTTP_PROXY / VPC_LINK disabled (set %s=1 and %s)",
			ErrAPIGatewayBadRequest, EnvAPIGatewayHTTPProxy, EnvAPIGatewayHTTPProxyAllowlist)
	}
	uri := strings.TrimSpace(integrationURI)
	if uri == "" {
		return fmt.Errorf("%w: IntegrationUri required for HTTP_PROXY / VPC_LINK", ErrAPIGatewayBadRequest)
	}
	u, err := url.Parse(uri)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return fmt.Errorf("%w: IntegrationUri must be an absolute http(s) URL", ErrAPIGatewayBadRequest)
	}
	scheme := strings.ToLower(u.Scheme)
	if scheme != "http" && scheme != "https" {
		return fmt.Errorf("%w: IntegrationUri scheme must be http or https", ErrAPIGatewayBadRequest)
	}
	entry, ok := matchAPIGatewayHTTPProxyAllowlist(uri, u)
	if !ok {
		return fmt.Errorf("%w: IntegrationUri not in %s", ErrAPIGatewayBadRequest, EnvAPIGatewayHTTPProxyAllowlist)
	}
	host := u.Hostname()
	// Name/IP-literal unsafe hosts need an allowlist entry that itself names that host.
	// Public hostnames are checked again at dial time against DNS rebinding.
	if isAPIGatewayHTTPProxyUnsafeHost(host) && !apigwHTTPProxyEntryAllowsUnsafe(entry) {
		return fmt.Errorf("%w: link-local, metadata, loopback, and private IntegrationUri hosts require an explicit allowlist entry naming that host",
			ErrAPIGatewayBadRequest)
	}
	return nil
}

func matchAPIGatewayHTTPProxyAllowlist(uri string, u *url.URL) (entry string, ok bool) {
	raw := strings.TrimSpace(os.Getenv(EnvAPIGatewayHTTPProxyAllowlist))
	if raw == "" {
		return "", false
	}
	want := strings.TrimSpace(uri)
	host := strings.ToLower(u.Hostname())
	hostPort := strings.ToLower(u.Host)
	for _, part := range strings.Split(raw, ",") {
		entry = strings.TrimSpace(part)
		if entry == "" {
			continue
		}
		if strings.Contains(entry, "://") {
			if strings.HasPrefix(want, entry) {
				return entry, true
			}
			continue
		}
		e := strings.ToLower(entry)
		if e == host || e == hostPort {
			return entry, true
		}
	}
	return "", false
}

func apigwHTTPProxyEntryAllowsUnsafe(entry string) bool {
	entry = strings.TrimSpace(entry)
	if entry == "" {
		return false
	}
	if strings.Contains(entry, "://") {
		eu, err := url.Parse(entry)
		if err != nil || eu.Hostname() == "" {
			return false
		}
		return isAPIGatewayHTTPProxyUnsafeHost(eu.Hostname())
	}
	host := entry
	if h, _, err := net.SplitHostPort(entry); err == nil {
		host = h
	}
	return isAPIGatewayHTTPProxyUnsafeHost(host)
}

func isAPIGatewayHTTPProxyUnsafeHost(host string) bool {
	host = strings.TrimSpace(strings.ToLower(host))
	if host == "" {
		return true
	}
	if host == "localhost" || strings.HasSuffix(host, ".localhost") {
		return true
	}
	if host == "metadata.google.internal" || strings.Contains(host, "metadata") {
		return true
	}
	if ip := net.ParseIP(host); ip != nil {
		return rejectAPIGatewayHTTPProxyUnsafeIP(ip) != nil
	}
	return false
}

func rejectAPIGatewayHTTPProxyUnsafeHost(host string) error {
	host = strings.TrimSpace(host)
	lower := strings.ToLower(host)
	if lower == "localhost" || strings.HasSuffix(lower, ".localhost") {
		return fmt.Errorf("unsafe HTTP_PROXY host")
	}
	if lower == "metadata.google.internal" || strings.Contains(lower, "metadata") {
		return fmt.Errorf("unsafe HTTP_PROXY host")
	}
	ips, err := net.LookupIP(host)
	if err != nil {
		if ip := net.ParseIP(host); ip != nil {
			return rejectAPIGatewayHTTPProxyUnsafeIP(ip)
		}
		return fmt.Errorf("resolve HTTP_PROXY host: %v", err)
	}
	if len(ips) == 0 {
		return fmt.Errorf("unsafe HTTP_PROXY host")
	}
	for _, ip := range ips {
		if err := rejectAPIGatewayHTTPProxyUnsafeIP(ip); err != nil {
			return err
		}
	}
	return nil
}

func rejectAPIGatewayHTTPProxyUnsafeIP(ip net.IP) error {
	if ip == nil {
		return fmt.Errorf("unsafe HTTP_PROXY host")
	}
	if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() ||
		ip.IsMulticast() || ip.IsUnspecified() {
		return fmt.Errorf("unsafe HTTP_PROXY host")
	}
	if ip4 := ip.To4(); ip4 != nil && ip4[0] == 169 && ip4[1] == 254 {
		return fmt.Errorf("unsafe HTTP_PROXY host")
	}
	return nil
}

// FetchAPIGatewayHTTPProxy performs a lab HTTP_PROXY fetch (no redirects).
func FetchAPIGatewayHTTPProxy(ctx context.Context, method, integrationURI string, body []byte, header http.Header) (status int, respBody []byte, respHeader http.Header, err error) {
	if err := ValidateAPIGatewayHTTPProxyURI(integrationURI); err != nil {
		return 0, nil, nil, err
	}
	u, err := url.Parse(strings.TrimSpace(integrationURI))
	if err != nil {
		return 0, nil, nil, err
	}
	entry, _ := matchAPIGatewayHTTPProxyAllowlist(strings.TrimSpace(integrationURI), u)
	allowUnsafeDial := apigwHTTPProxyEntryAllowsUnsafe(entry)

	method = strings.ToUpper(strings.TrimSpace(method))
	if method == "" {
		method = http.MethodGet
	}
	var reader io.Reader
	if len(body) > 0 && method != http.MethodGet && method != http.MethodHead {
		reader = strings.NewReader(string(body))
	}
	req, err := http.NewRequestWithContext(ctx, method, integrationURI, reader)
	if err != nil {
		return 0, nil, nil, err
	}
	for k, vals := range header {
		lk := strings.ToLower(k)
		if lk == "host" || lk == "connection" || lk == "content-length" || lk == "transfer-encoding" {
			continue
		}
		for _, v := range vals {
			req.Header.Add(k, v)
		}
	}
	client := apigwHTTPProxyClient(allowUnsafeDial)
	resp, err := client.Do(req)
	if err != nil {
		return 0, nil, nil, err
	}
	defer resp.Body.Close()
	limited := io.LimitReader(resp.Body, apigwHTTPProxyMaxBody+1)
	raw, err := io.ReadAll(limited)
	if err != nil {
		return 0, nil, nil, err
	}
	if len(raw) > apigwHTTPProxyMaxBody {
		return 0, nil, nil, fmt.Errorf("HTTP_PROXY response exceeds lab size cap")
	}
	return resp.StatusCode, raw, resp.Header.Clone(), nil
}

func apigwHTTPProxyClient(allowUnsafeDial bool) *http.Client {
	base, ok := http.DefaultTransport.(*http.Transport)
	var transport *http.Transport
	if ok {
		transport = base.Clone()
	} else {
		transport = &http.Transport{}
	}
	transport.DialContext = pinnedAPIGatewayHTTPProxyDialContext(allowUnsafeDial)
	return &http.Client{
		Timeout:   5 * time.Second,
		Transport: transport,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return fmt.Errorf("apigateway http_proxy: redirects are not allowed")
		},
	}
}

// PinnedAPIGatewayHTTPProxyDialContext is exported for dialer unit tests (safe public dial).
func PinnedAPIGatewayHTTPProxyDialContext(ctx context.Context, network, addr string) (net.Conn, error) {
	return pinnedAPIGatewayHTTPProxyDialContext(false)(ctx, network, addr)
}

func pinnedAPIGatewayHTTPProxyDialContext(allowUnsafe bool) func(context.Context, string, string) (net.Conn, error) {
	return func(ctx context.Context, network, addr string) (net.Conn, error) {
		host, port, err := net.SplitHostPort(addr)
		if err != nil {
			return nil, fmt.Errorf("apigateway http_proxy: dial addr: %w", err)
		}
		ips, err := net.LookupIP(host)
		if err != nil {
			if ip := net.ParseIP(host); ip != nil {
				ips = []net.IP{ip}
			} else {
				return nil, fmt.Errorf("apigateway http_proxy: resolve dial host: %w", err)
			}
		}
		if len(ips) == 0 {
			return nil, fmt.Errorf("apigateway http_proxy: dial host resolved to no addresses")
		}
		var dialer net.Dialer
		var lastErr error
		for _, ip := range ips {
			if !allowUnsafe {
				if err := rejectAPIGatewayHTTPProxyUnsafeIP(ip); err != nil {
					lastErr = err
					continue
				}
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
		return nil, fmt.Errorf("apigateway http_proxy: no safe dial target")
	}
}
