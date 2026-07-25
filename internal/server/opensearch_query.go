package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/Kyaxris-Labs/Noctaxris/internal/catalog"
	"github.com/Kyaxris-Labs/Noctaxris/internal/kernel/authn"
	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

const (
	openSearchLabHostPrefix     = "noctaxris-opensearch-"
	openSearchLabHostPrefixAlt  = "noctaxris-data-opensearch-"
	openSearchLabTimeout        = 10 * time.Second
)

// openSearchLabTransport is overridden in unit tests (mock RoundTripper).
// When nil, the default HTTP transport is used.
var openSearchLabTransport http.RoundTripper

// isOpenSearchLabQueryPath reports lab index/search paths on :4566.
// PUT  /opensearch/{domain}/lab/{index}/_doc/{id}
// POST /opensearch/{domain}/lab/{index}/_search
func isOpenSearchLabQueryPath(path string) bool {
	_, _, _, _, ok := parseOpenSearchLabPath(path)
	return ok
}

type openSearchLabOp int

const (
	openSearchLabOpDoc openSearchLabOp = iota
	openSearchLabOpSearch
)

func parseOpenSearchLabPath(path string) (domain, index, docID string, op openSearchLabOp, ok bool) {
	path = strings.TrimSuffix(path, "/")
	parts := strings.Split(strings.TrimPrefix(path, "/"), "/")
	// opensearch / {domain} / lab / {index} / _doc / {id}
	// opensearch / {domain} / lab / {index} / _search
	if len(parts) < 5 || parts[0] != "opensearch" || parts[2] != "lab" {
		return "", "", "", 0, false
	}
	domain = parts[1]
	index = parts[3]
	if domain == "" || index == "" || !safeOpenSearchPathSegment(domain) || !safeOpenSearchPathSegment(index) {
		return "", "", "", 0, false
	}
	switch {
	case len(parts) == 6 && parts[4] == "_doc":
		docID = parts[5]
		if docID == "" || !safeOpenSearchPathSegment(docID) {
			return "", "", "", 0, false
		}
		return domain, index, docID, openSearchLabOpDoc, true
	case len(parts) == 5 && parts[4] == "_search":
		return domain, index, "", openSearchLabOpSearch, true
	default:
		return "", "", "", 0, false
	}
}

func safeOpenSearchPathSegment(s string) bool {
	if s == "" || s == "." || s == ".." {
		return false
	}
	if strings.ContainsAny(s, `/\`) {
		return false
	}
	return true
}

// validateNestedOpenSearchHost allows only DinD nested OpenSearch hostnames.
// Loopback, wildcards, and raw IPs are rejected so the query facade never dials an
// operator-published or open network endpoint.
func validateNestedOpenSearchHost(host string) error {
	host = strings.TrimSpace(host)
	if host == "" {
		return fmt.Errorf("nested opensearch endpoint is empty")
	}
	lower := strings.ToLower(host)
	switch lower {
	case "localhost", "127.0.0.1", "::1", "0.0.0.0", "*", "host.docker.internal":
		return fmt.Errorf("refusing non-nested opensearch host %q", host)
	}
	if strings.Contains(host, "/") || strings.Contains(host, "\\") {
		return fmt.Errorf("invalid nested opensearch host")
	}
	if ip := net.ParseIP(host); ip != nil {
		return fmt.Errorf("refusing IP opensearch host %q (nested container DNS name required)", host)
	}
	if !strings.HasPrefix(lower, openSearchLabHostPrefix) && !strings.HasPrefix(lower, openSearchLabHostPrefixAlt) {
		return fmt.Errorf("nested opensearch host %q is not a data-plane endpoint", host)
	}
	suffix := strings.TrimPrefix(lower, openSearchLabHostPrefix)
	if strings.HasPrefix(lower, openSearchLabHostPrefixAlt) {
		suffix = strings.TrimPrefix(lower, openSearchLabHostPrefixAlt)
	}
	if suffix == "" {
		return fmt.Errorf("nested opensearch host %q is missing a domain suffix", host)
	}
	return nil
}

// validateOpenSearchLabSearchBody allowlists query.match, query.match_all, and size only.
func validateOpenSearchLabSearchBody(body []byte) error {
	trimmed := strings.TrimSpace(string(body))
	if trimmed == "" {
		return nil
	}
	var m map[string]any
	if err := json.Unmarshal([]byte(trimmed), &m); err != nil {
		return fmt.Errorf("search body must be JSON: %w", err)
	}
	for k := range m {
		switch k {
		case "query", "size":
		default:
			return fmt.Errorf("unsupported search key %q (allowed: query, size)", k)
		}
	}
	if raw, ok := m["query"]; ok {
		qm, ok := raw.(map[string]any)
		if !ok {
			return fmt.Errorf("query must be an object")
		}
		if len(qm) == 0 {
			return fmt.Errorf("query object is empty")
		}
		for k := range qm {
			switch k {
			case "match", "match_all":
			default:
				return fmt.Errorf("unsupported query type %q (allowed: match, match_all)", k)
			}
		}
	}
	if raw, ok := m["size"]; ok {
		switch raw.(type) {
		case float64, json.Number:
		default:
			return fmt.Errorf("size must be a number")
		}
	}
	return nil
}

func openSearchLabHTTPClient() *http.Client {
	tr := openSearchLabTransport
	return &http.Client{
		Timeout:   openSearchLabTimeout,
		Transport: tr,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 3 {
				return fmt.Errorf("stopped after %d redirects", len(via))
			}
			host := req.URL.Hostname()
			if err := validateNestedOpenSearchHost(host); err != nil {
				return fmt.Errorf("refusing redirect off nested network: %w", err)
			}
			port := req.URL.Port()
			if port == "" {
				port = "80"
				if req.URL.Scheme == "https" {
					port = "443"
				}
			}
			p, err := strconv.Atoi(port)
			if err != nil || p != store.OpenSearchNestedPort {
				return fmt.Errorf("refusing redirect to non-nested opensearch port %q", port)
			}
			return nil
		},
	}
}

// handleOpenSearchLabQuery proxies allowlisted index/search calls to the nested OpenSearch engine.
// verified may be nil in unit tests (uses cfg.AccountID); ServeHTTP always passes a verified identity.
func (s *Server) handleOpenSearchLabQuery(
	w http.ResponseWriter,
	r *http.Request,
	body []byte,
	requestID, eventID string,
	verified *authn.Verified,
	readOnly bool,
) {
	_ = eventID
	_ = readOnly
	domain, index, docID, op, ok := parseOpenSearchLabPath(r.URL.Path)
	if !ok {
		s.writeOpenSearchLabError(w, requestID, http.StatusNotFound, "ResourceNotFoundException", "OpenSearch lab path not found.")
		return
	}

	switch op {
	case openSearchLabOpDoc:
		if r.Method != http.MethodPut {
			s.writeOpenSearchLabError(w, requestID, http.StatusMethodNotAllowed, "MethodNotAllowed", "Use PUT for lab _doc index.")
			return
		}
	case openSearchLabOpSearch:
		if r.Method != http.MethodPost {
			s.writeOpenSearchLabError(w, requestID, http.StatusMethodNotAllowed, "MethodNotAllowed", "Use POST for lab _search.")
			return
		}
		if err := validateOpenSearchLabSearchBody(body); err != nil {
			s.writeOpenSearchLabError(w, requestID, http.StatusBadRequest, "ValidationException", err.Error())
			return
		}
	default:
		s.writeOpenSearchLabError(w, requestID, http.StatusNotFound, "ResourceNotFoundException", "OpenSearch lab path not found.")
		return
	}

	accountID := strings.TrimSpace(s.cfg.AccountID)
	if verified != nil {
		if !strings.EqualFold(verified.Service, "es") && !strings.EqualFold(verified.Service, "opensearch") {
			s.writeOpenSearchLabError(w, requestID, http.StatusForbidden, "AccessDeniedException", "SigV4 service must be es.")
			return
		}
		if !s.authorize(verified, catalog.ActionOpenSearchDescribeDomain, "*") {
			s.writeOpenSearchLabError(w, requestID, http.StatusForbidden, "AccessDeniedException",
				"User is not authorized to perform es:DescribeDomain.")
			return
		}
		accountID = verified.AccountID
	}
	if accountID == "" {
		s.writeOpenSearchLabError(w, requestID, http.StatusInternalServerError, "InternalFailure", "Account ID is not configured.")
		return
	}

	targetURL, err := s.resolveOpenSearchLabTargetURL(accountID, domain, index, docID, op)
	if err != nil {
		status, code, msg := openSearchLabResolveError(err)
		s.writeOpenSearchLabError(w, requestID, status, code, msg)
		return
	}

	req, err := http.NewRequestWithContext(r.Context(), r.Method, targetURL, strings.NewReader(string(body)))
	if err != nil {
		s.writeOpenSearchLabError(w, requestID, http.StatusInternalServerError, "InternalFailure", "Unable to build nested request.")
		return
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := openSearchLabHTTPClient().Do(req)
	if err != nil {
		s.writeOpenSearchLabError(w, requestID, http.StatusBadGateway, "InternalFailure", "Nested OpenSearch request failed.")
		return
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		s.writeOpenSearchLabError(w, requestID, http.StatusBadGateway, "InternalFailure", "Unable to read nested OpenSearch response.")
		return
	}
	if ct := resp.Header.Get("Content-Type"); ct != "" {
		w.Header().Set("Content-Type", ct)
	} else {
		w.Header().Set("Content-Type", "application/json")
	}
	w.Header().Set("x-amzn-RequestId", requestID)
	w.WriteHeader(resp.StatusCode)
	_, _ = w.Write(respBody)
}

func (s *Server) resolveOpenSearchLabTargetURL(accountID, domain, index, docID string, op openSearchLabOp) (string, error) {
	d, err := s.store.DescribeOpenSearchDomain(accountID, domain)
	if err != nil {
		return "", err
	}
	if d.DomainStatus != store.OpenSearchDomainStatusActive {
		return "", errOpenSearchLabNotReady
	}
	endpoint := strings.TrimSpace(d.StubEndpoint)
	if endpoint == "" || strings.HasPrefix(endpoint, "stub://") {
		return "", errOpenSearchLabNotReady
	}
	host, port, err := parseNestedOpenSearchEndpoint(endpoint)
	if err != nil {
		return "", err
	}
	if err := validateNestedOpenSearchHost(host); err != nil {
		return "", errOpenSearchLabBadEndpoint
	}
	if port != store.OpenSearchNestedPort {
		return "", errOpenSearchLabBadEndpoint
	}

	var path string
	switch op {
	case openSearchLabOpDoc:
		path = "/" + url.PathEscape(index) + "/_doc/" + url.PathEscape(docID)
	case openSearchLabOpSearch:
		path = "/" + url.PathEscape(index) + "/_search"
	default:
		return "", fmt.Errorf("unknown lab op")
	}
	return fmt.Sprintf("http://%s%s", net.JoinHostPort(host, strconv.Itoa(port)), path), nil
}

var (
	errOpenSearchLabNotReady    = fmt.Errorf("opensearch lab domain not ready")
	errOpenSearchLabBadEndpoint = fmt.Errorf("opensearch lab endpoint refused")
)

func openSearchLabResolveError(err error) (status int, code, msg string) {
	if err == nil {
		return http.StatusInternalServerError, "InternalFailure", "Unknown error."
	}
	switch {
	case errors.Is(err, store.ErrOpenSearchDomainNotFound):
		return http.StatusNotFound, "ResourceNotFoundException", "Domain not found."
	case errors.Is(err, errOpenSearchLabNotReady):
		return http.StatusConflict, "ResourceNotFoundException", "Domain is not Active or has a stub endpoint."
	case errors.Is(err, errOpenSearchLabBadEndpoint):
		return http.StatusConflict, "ValidationException", "Domain endpoint is not a nested OpenSearch host."
	case errors.Is(err, store.ErrOpenSearchBadRequest):
		return http.StatusBadRequest, "ValidationException", err.Error()
	default:
		if strings.Contains(err.Error(), "nested opensearch") || strings.Contains(err.Error(), "refusing") {
			return http.StatusConflict, "ValidationException", "Domain endpoint is not a nested OpenSearch host."
		}
		return http.StatusInternalServerError, "InternalFailure", "Unable to resolve OpenSearch domain."
	}
}

func parseNestedOpenSearchEndpoint(endpoint string) (host string, port int, err error) {
	endpoint = strings.TrimSpace(endpoint)
	if endpoint == "" {
		return "", 0, errOpenSearchLabBadEndpoint
	}
	if strings.Contains(endpoint, "://") {
		u, perr := url.Parse(endpoint)
		if perr != nil || u.Host == "" {
			return "", 0, errOpenSearchLabBadEndpoint
		}
		endpoint = u.Host
	}
	h, p, perr := net.SplitHostPort(endpoint)
	if perr != nil {
		// host without port
		if validateNestedOpenSearchHost(endpoint) == nil {
			return endpoint, store.OpenSearchNestedPort, nil
		}
		return "", 0, errOpenSearchLabBadEndpoint
	}
	port, aerr := strconv.Atoi(p)
	if aerr != nil {
		return "", 0, errOpenSearchLabBadEndpoint
	}
	return h, port, nil
}

func (s *Server) writeOpenSearchLabError(w http.ResponseWriter, requestID string, status int, code, message string) {
	w.Header().Set("Content-Type", opensearchJSONContentType)
	w.Header().Set("x-amzn-RequestId", requestID)
	w.Header().Set("x-amzn-ErrorType", code)
	w.WriteHeader(status)
	escaped := strings.ReplaceAll(message, `"`, `'`)
	_, _ = w.Write([]byte(`{"__type":"` + code + `","message":"` + escaped + `"}`))
}
