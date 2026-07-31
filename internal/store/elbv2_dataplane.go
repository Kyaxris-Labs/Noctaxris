package store

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const elbv2LabForwardMaxBody = 1 << 20 // 1 MiB lab cap

// ELBv2NLBForwardURL builds an http URL for the first registered target on a network LB
// lab mux path. Only explicitly registered target IDs are used (no open proxy).
func (s *Store) ELBv2NLBForwardURL(accountID string, tg ELBv2TargetGroup, target ELBv2Target, routePath, rawQuery string) (string, error) {
	host, port, err := s.resolveELBv2NLBTargetHostPort(accountID, tg, target)
	if err != nil {
		return "", err
	}
	if routePath == "" {
		routePath = "/"
	}
	u := url.URL{
		Scheme: "http",
		Host:   net.JoinHostPort(host, strconv.Itoa(port)),
		Path:   routePath,
	}
	if rawQuery != "" {
		u.RawQuery = rawQuery
	}
	return u.String(), nil
}

func (s *Store) resolveELBv2NLBTargetHostPort(accountID string, tg ELBv2TargetGroup, target ELBv2Target) (host string, port int, err error) {
	port = tg.Port
	if target.Port > 0 {
		port = target.Port
	}
	if port <= 0 {
		port = 80
	}
	switch tg.TargetType {
	case "ip":
		host = strings.TrimSpace(target.ID)
		if h, p, splitErr := net.SplitHostPort(host); splitErr == nil {
			host = h
			if target.Port <= 0 && p != "" {
				if n, convErr := strconv.Atoi(p); convErr == nil && n > 0 {
					port = n
				}
			}
		}
		if net.ParseIP(host) == nil {
			return "", 0, fmt.Errorf("%w: invalid ip target", ErrELBv2BadRequest)
		}
		if LabForwardIPDenied(net.ParseIP(host)) {
			return "", 0, fmt.Errorf("%w: ip target not allowed for lab nlb forward", ErrELBv2BadRequest)
		}
	case "instance":
		inst, instErr := s.GetEC2Instance(accountID, DefaultELBv2Region, strings.TrimSpace(target.ID))
		if instErr != nil || strings.TrimSpace(inst.PrivateIP) == "" {
			return "", 0, fmt.Errorf("instance target not reachable (no private IP)")
		}
		host = strings.TrimSpace(inst.PrivateIP)
		if LabForwardIPDenied(net.ParseIP(host)) {
			return "", 0, fmt.Errorf("instance target not reachable (blocked address)")
		}
	default:
		return "", 0, fmt.Errorf("%w: lab nlb dataplane supports ip and instance targets only", ErrELBv2BadRequest)
	}
	return host, port, nil
}

// FetchELBv2LabHTTPForward performs a lab HTTP forward to a URL built from registered NLB targets.
// The dial is pinned to the URL hostname to limit SSRF when the host is a public DNS name;
// NLB lab URLs use literal IPs from registration.
func FetchELBv2LabHTTPForward(ctx context.Context, targetURL, method string, body []byte, header http.Header) (status int, respBody []byte, respHeader http.Header, err error) {
	u, err := url.Parse(strings.TrimSpace(targetURL))
	if err != nil || u.Scheme != "http" || u.Host == "" {
		return 0, nil, nil, fmt.Errorf("invalid target url")
	}
	pinnedHost := u.Hostname()
	if net.ParseIP(pinnedHost) == nil {
		return 0, nil, nil, fmt.Errorf("lab nlb forward requires ip host")
	}

	method = strings.ToUpper(strings.TrimSpace(method))
	if method == "" {
		method = http.MethodGet
	}
	var reader io.Reader
	if len(body) > 0 && method != http.MethodGet && method != http.MethodHead {
		reader = strings.NewReader(string(body))
	}
	req, err := http.NewRequestWithContext(ctx, method, targetURL, reader)
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
	client := elbv2LabForwardHTTPClient(pinnedHost)
	resp, err := client.Do(req)
	if err != nil {
		return 0, nil, nil, err
	}
	defer resp.Body.Close()
	limited := io.LimitReader(resp.Body, elbv2LabForwardMaxBody+1)
	raw, err := io.ReadAll(limited)
	if err != nil {
		return 0, nil, nil, err
	}
	if len(raw) > elbv2LabForwardMaxBody {
		return 0, nil, nil, fmt.Errorf("nlb forward response exceeds lab size cap")
	}
	return resp.StatusCode, raw, resp.Header.Clone(), nil
}

func elbv2LabForwardHTTPClient(pinnedHost string) *http.Client {
	base, ok := http.DefaultTransport.(*http.Transport)
	var transport *http.Transport
	if ok {
		transport = base.Clone()
	} else {
		transport = &http.Transport{}
	}
	transport.DialContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
		_, port, err := net.SplitHostPort(addr)
		if err != nil {
			return nil, err
		}
		d := net.Dialer{Timeout: 5 * time.Second}
		return d.DialContext(ctx, network, net.JoinHostPort(pinnedHost, port))
	}
	return &http.Client{
		Timeout:   5 * time.Second,
		Transport: transport,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return fmt.Errorf("nlb lab forward: redirects are not allowed")
		},
	}
}
