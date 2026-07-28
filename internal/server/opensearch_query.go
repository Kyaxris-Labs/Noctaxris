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
	openSearchLabTimeout = 10 * time.Second
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
// Delegates to store.ValidateNestedOpenSearchHost (rejects dotted suffixes after prefix).
func validateNestedOpenSearchHost(host string) error {
	return store.ValidateNestedOpenSearchHost(host)
}

// validateOpenSearchLabSearchBody allowlists a lite _search DSL:
// top-level query/size/aggs|aggregations/sort; query.match|match_all|term|range|bool;
// bool must/should/must_not/filter with nested match|match_all|term (no nested bool);
// aggs terms|value_count lite; sort field+order only. Fail-closed otherwise.
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
		case "query", "size", "aggs", "aggregations", "sort":
		default:
			return fmt.Errorf("unsupported search key %q (allowed: query, size, aggs, aggregations, sort)", k)
		}
	}
	if raw, ok := m["query"]; ok {
		if err := validateOpenSearchLabQuery(raw); err != nil {
			return err
		}
	}
	if raw, ok := m["size"]; ok {
		switch raw.(type) {
		case float64, json.Number:
		default:
			return fmt.Errorf("size must be a number")
		}
	}
	if raw, ok := m["aggs"]; ok {
		if err := validateOpenSearchLabAggs(raw); err != nil {
			return err
		}
	}
	if raw, ok := m["aggregations"]; ok {
		if err := validateOpenSearchLabAggs(raw); err != nil {
			return err
		}
	}
	if raw, ok := m["sort"]; ok {
		if err := validateOpenSearchLabSort(raw); err != nil {
			return err
		}
	}
	return nil
}

func validateOpenSearchLabQuery(raw any) error {
	qm, ok := raw.(map[string]any)
	if !ok {
		return fmt.Errorf("query must be an object")
	}
	if len(qm) == 0 {
		return fmt.Errorf("query object is empty")
	}
	for k, v := range qm {
		switch k {
		case "match", "match_all":
		case "term":
			if err := validateOpenSearchLabTerm(v); err != nil {
				return err
			}
		case "range":
			if err := validateOpenSearchLabRange(v); err != nil {
				return err
			}
		case "bool":
			if err := validateOpenSearchLabBool(v); err != nil {
				return err
			}
		default:
			return fmt.Errorf("unsupported query type %q (allowed: match, match_all, term, range, bool)", k)
		}
	}
	return nil
}

func validateOpenSearchLabBool(raw any) error {
	bm, ok := raw.(map[string]any)
	if !ok {
		return fmt.Errorf("bool must be an object")
	}
	if len(bm) == 0 {
		return fmt.Errorf("bool object is empty")
	}
	for k, v := range bm {
		switch k {
		case "must", "should", "must_not", "filter":
			if err := validateOpenSearchLabBoolClauses(k, v); err != nil {
				return err
			}
		default:
			return fmt.Errorf("unsupported bool key %q (allowed: must, should, must_not, filter)", k)
		}
	}
	return nil
}

func validateOpenSearchLabBoolClauses(clause string, raw any) error {
	switch v := raw.(type) {
	case map[string]any:
		return validateOpenSearchLabLeafQuery(clause, v)
	case []any:
		for i, item := range v {
			qm, ok := item.(map[string]any)
			if !ok {
				return fmt.Errorf("bool.%s[%d] must be an object", clause, i)
			}
			if err := validateOpenSearchLabLeafQuery(clause, qm); err != nil {
				return err
			}
		}
		return nil
	default:
		return fmt.Errorf("bool.%s must be an object or array", clause)
	}
}

// validateOpenSearchLabLeafQuery allowlists match/match_all/term under bool clauses (no nested bool/range).
func validateOpenSearchLabLeafQuery(clause string, qm map[string]any) error {
	if len(qm) == 0 {
		return fmt.Errorf("bool.%s query object is empty", clause)
	}
	for k, v := range qm {
		switch k {
		case "match", "match_all":
		case "term":
			if err := validateOpenSearchLabTerm(v); err != nil {
				return fmt.Errorf("bool.%s: %w", clause, err)
			}
		default:
			return fmt.Errorf("unsupported bool.%s query type %q (allowed: match, match_all, term)", clause, k)
		}
	}
	return nil
}

// validateOpenSearchLabTerm allowlists {"field": value} or {"field": {"value": ...}} lite shapes.
func validateOpenSearchLabTerm(raw any) error {
	tm, ok := raw.(map[string]any)
	if !ok {
		return fmt.Errorf("term must be an object")
	}
	if len(tm) == 0 {
		return fmt.Errorf("term object is empty")
	}
	if len(tm) != 1 {
		return fmt.Errorf("term must have exactly one field")
	}
	for field, spec := range tm {
		if field == "" {
			return fmt.Errorf("term field must be non-empty")
		}
		switch s := spec.(type) {
		case map[string]any:
			if len(s) == 0 {
				return fmt.Errorf("term.%s object is empty", field)
			}
			for k := range s {
				if k != "value" {
					return fmt.Errorf("unsupported term key %q on %q (allowed: value)", k, field)
				}
			}
			if _, ok := s["value"]; !ok {
				return fmt.Errorf("term.%s requires value", field)
			}
		case string, float64, bool, json.Number:
			// scalar term value
		case nil:
			return fmt.Errorf("term.%s value must not be null", field)
		default:
			return fmt.Errorf("term.%s value must be a scalar or {\"value\":...}", field)
		}
	}
	return nil
}

// validateOpenSearchLabRange allowlists {"field": {"gte"|"gt"|"lte"|"lt": scalar}} lite shapes.
func validateOpenSearchLabRange(raw any) error {
	rm, ok := raw.(map[string]any)
	if !ok {
		return fmt.Errorf("range must be an object")
	}
	if len(rm) == 0 {
		return fmt.Errorf("range object is empty")
	}
	if len(rm) != 1 {
		return fmt.Errorf("range must have exactly one field")
	}
	for field, spec := range rm {
		if field == "" {
			return fmt.Errorf("range field must be non-empty")
		}
		sm, ok := spec.(map[string]any)
		if !ok {
			return fmt.Errorf("range.%s must be an object", field)
		}
		if len(sm) == 0 {
			return fmt.Errorf("range.%s object is empty", field)
		}
		for k, v := range sm {
			switch k {
			case "gte", "gt", "lte", "lt":
				switch v.(type) {
				case string, float64, bool, json.Number:
				default:
					return fmt.Errorf("range.%s.%s must be a scalar", field, k)
				}
			default:
				return fmt.Errorf("unsupported range key %q on %q (allowed: gte, gt, lte, lt)", k, field)
			}
		}
	}
	return nil
}

func validateOpenSearchLabAggs(raw any) error {
	am, ok := raw.(map[string]any)
	if !ok {
		return fmt.Errorf("aggs must be an object")
	}
	for name, def := range am {
		dm, ok := def.(map[string]any)
		if !ok {
			return fmt.Errorf("agg %q must be an object", name)
		}
		if len(dm) == 0 {
			return fmt.Errorf("agg %q object is empty", name)
		}
		for k, v := range dm {
			switch k {
			case "terms":
				if err := validateOpenSearchLabTermsAgg(name, v); err != nil {
					return err
				}
			case "value_count":
				if err := validateOpenSearchLabValueCountAgg(name, v); err != nil {
					return err
				}
			default:
				return fmt.Errorf("unsupported agg type %q on %q (allowed: terms, value_count)", k, name)
			}
		}
	}
	return nil
}

func validateOpenSearchLabTermsAgg(name string, raw any) error {
	tm, ok := raw.(map[string]any)
	if !ok {
		return fmt.Errorf("agg %q terms must be an object", name)
	}
	if len(tm) == 0 {
		return fmt.Errorf("agg %q terms object is empty", name)
	}
	for k, v := range tm {
		switch k {
		case "field":
			if _, ok := v.(string); !ok {
				return fmt.Errorf("agg %q terms.field must be a string", name)
			}
		case "size":
			switch v.(type) {
			case float64, json.Number:
			default:
				return fmt.Errorf("agg %q terms.size must be a number", name)
			}
		default:
			return fmt.Errorf("unsupported terms key %q on agg %q (allowed: field, size)", k, name)
		}
	}
	if _, ok := tm["field"]; !ok {
		return fmt.Errorf("agg %q terms requires field", name)
	}
	return nil
}

func validateOpenSearchLabValueCountAgg(name string, raw any) error {
	vm, ok := raw.(map[string]any)
	if !ok {
		return fmt.Errorf("agg %q value_count must be an object", name)
	}
	if len(vm) == 0 {
		return fmt.Errorf("agg %q value_count object is empty", name)
	}
	for k, v := range vm {
		switch k {
		case "field":
			if _, ok := v.(string); !ok {
				return fmt.Errorf("agg %q value_count.field must be a string", name)
			}
		default:
			return fmt.Errorf("unsupported value_count key %q on agg %q (allowed: field)", k, name)
		}
	}
	if _, ok := vm["field"]; !ok {
		return fmt.Errorf("agg %q value_count requires field", name)
	}
	return nil
}

func validateOpenSearchLabSort(raw any) error {
	switch v := raw.(type) {
	case []any:
		for i, item := range v {
			if err := validateOpenSearchLabSortClause(item); err != nil {
				return fmt.Errorf("sort[%d]: %w", i, err)
			}
		}
		return nil
	case map[string]any:
		return validateOpenSearchLabSortClause(v)
	default:
		return fmt.Errorf("sort must be an object or array")
	}
}

func validateOpenSearchLabSortClause(raw any) error {
	sm, ok := raw.(map[string]any)
	if !ok {
		return fmt.Errorf("sort clause must be an object")
	}
	if len(sm) != 1 {
		return fmt.Errorf("sort clause must have exactly one field")
	}
	for field, spec := range sm {
		if field == "" {
			return fmt.Errorf("sort field must be non-empty")
		}
		switch s := spec.(type) {
		case map[string]any:
			if len(s) == 0 {
				return fmt.Errorf("sort.%s object is empty", field)
			}
			for k, v := range s {
				if k != "order" {
					return fmt.Errorf("unsupported sort key %q on %q (allowed: order)", k, field)
				}
				order, ok := v.(string)
				if !ok {
					return fmt.Errorf("sort.%s.order must be a string", field)
				}
				if order != "asc" && order != "desc" {
					return fmt.Errorf("sort.%s.order must be asc or desc", field)
				}
			}
		default:
			return fmt.Errorf("sort.%s must be an object with order", field)
		}
	}
	return nil
}

func openSearchLabHTTPClient() *http.Client {
	checkRedirect := func(req *http.Request, via []*http.Request) error {
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
	}
	if openSearchLabTransport != nil {
		return &http.Client{
			Timeout:       openSearchLabTimeout,
			Transport:     openSearchLabTransport,
			CheckRedirect: checkRedirect,
		}
	}
	return store.NestedOpenSearchHTTPClient(openSearchLabTimeout, checkRedirect)
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
