package server

import (
	"net"
	"net/http"
	"strings"

	"github.com/Kyaxris-Labs/Noctaxris/internal/config"
	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

// enforceAssociatedWAF blocks the request with 403 when a Web ACL associated to
// any candidate ARN evaluates to Block. Missing associations are a no-op.
// Evaluation errors with an association present fail closed (403).
func (s *Server) enforceAssociatedWAF(w http.ResponseWriter, r *http.Request, accountID string, candidateARNs []string) bool {
	view := s.wafRequestViewFromHTTP(r)
	action, associated, err := s.store.EvaluateAssociatedWAFWithView(accountID, candidateARNs, "", view)
	if err != nil {
		if associated {
			http.Error(w, "Forbidden", http.StatusForbidden)
			return false
		}
		// Lookup/store failure before any association match: fail closed.
		http.Error(w, "Forbidden", http.StatusForbidden)
		return false
	}
	if !associated {
		return true
	}
	if strings.EqualFold(action, "Block") {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return false
	}
	return true
}

var wafEnforceHeaderNames = []string{"Host", "User-Agent", "X-Forwarded-For"}

func (s *Server) wafRequestViewFromHTTP(r *http.Request) *store.WAFRequestView {
	if r == nil || r.URL == nil {
		return nil
	}
	headers := make(map[string]string, len(wafEnforceHeaderNames))
	for _, name := range wafEnforceHeaderNames {
		if v := r.Header.Get(name); v != "" {
			headers[name] = v
		}
	}
	var trusted []*net.IPNet
	if s != nil {
		trusted = s.cfg.TrustedProxies
	}
	return &store.WAFRequestView{
		URI:      r.URL.Path,
		Headers:  headers,
		SourceIP: wafSourceIPFromHTTP(r, trusted),
	}
}

// wafSourceIPFromHTTP picks the client IP for IPSet matching.
// X-Forwarded-For is used only when the TCP peer is in TrustedProxies;
// otherwise RemoteAddr (host only). Invalid XFF falls back to RemoteAddr.
func wafSourceIPFromHTTP(r *http.Request, trusted []*net.IPNet) string {
	if r == nil {
		return ""
	}
	peer := peerHostOnly(r)
	if config.PeerInTrustedProxies(peer, trusted) {
		if xff := strings.TrimSpace(r.Header.Get("X-Forwarded-For")); xff != "" {
			first := strings.TrimSpace(strings.Split(xff, ",")[0])
			if ip := net.ParseIP(first); ip != nil {
				return ip.String()
			}
		}
	}
	return peer
}

func peerHostOnly(r *http.Request) string {
	if r == nil {
		return ""
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err == nil && host != "" {
		if ip := net.ParseIP(host); ip != nil {
			return ip.String()
		}
		return host
	}
	if ip := net.ParseIP(r.RemoteAddr); ip != nil {
		return ip.String()
	}
	return strings.TrimSpace(r.RemoteAddr)
}

func wafRequestViewFromEvaluateParams(params map[string]any) *store.WAFRequestView {
	if params == nil {
		return nil
	}
	uri, _ := params["URI"].(string)
	if uri == "" {
		uri, _ = params["Uri"].(string)
	}
	sourceIP, _ := params["SourceIP"].(string)
	rawHeaders, _ := params["Headers"].(map[string]any)
	if uri == "" && sourceIP == "" && len(rawHeaders) == 0 {
		return nil
	}
	headers := map[string]string{}
	for k, v := range rawHeaders {
		if s, ok := v.(string); ok {
			headers[k] = s
		}
	}
	if sourceIP == "" {
		if xff := wafHeaderValueMap(headers, "X-Forwarded-For"); xff != "" {
			first := strings.TrimSpace(strings.Split(xff, ",")[0])
			if ip := net.ParseIP(first); ip != nil {
				sourceIP = ip.String()
			}
		}
	}
	return &store.WAFRequestView{URI: uri, Headers: headers, SourceIP: sourceIP}
}

func wafHeaderValueMap(headers map[string]string, name string) string {
	if headers == nil {
		return ""
	}
	if v, ok := headers[name]; ok {
		return v
	}
	lower := strings.ToLower(name)
	for k, v := range headers {
		if strings.ToLower(k) == lower {
			return v
		}
	}
	return ""
}

func httpAPIWAFCandidateARNs(region, accountID, apiID, stage string) []string {
	if region == "" {
		region = store.DefaultAPIGatewayRegion
	}
	return []string{
		"arn:aws:apigateway:" + region + "::/apis/" + apiID + "/stages/" + stage,
		"arn:aws:apigateway:" + region + "::/apis/" + apiID,
		"arn:aws:execute-api:" + region + ":" + accountID + ":" + apiID + "/" + stage,
		"arn:aws:execute-api:" + region + ":" + accountID + ":" + apiID,
	}
}

func appSyncWAFCandidateARNs(region, accountID, apiID string) []string {
	if region == "" {
		region = store.DefaultAppSyncRegion
	}
	return []string{
		"arn:aws:appsync:" + region + ":" + accountID + ":apis/" + apiID,
	}
}
