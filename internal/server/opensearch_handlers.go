package server

import (
	"errors"
	"net/http"
	"strings"

	"github.com/Kyaxris-Labs/Noctaxris/internal/catalog"
	"github.com/Kyaxris-Labs/Noctaxris/internal/kernel/authn"
	ossvc "github.com/Kyaxris-Labs/Noctaxris/internal/services/opensearch"
	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

const (
	opensearchJSONContentType = "application/x-amz-json-1.0"
	opensearchEventSource     = "es.amazonaws.com"
)

func (s *Server) handleOpenSearch(
	w http.ResponseWriter,
	r *http.Request,
	body []byte,
	requestID, eventID, action string,
	verified *authn.Verified,
	readOnly bool,
) {
	params := jsonBodyMap(body)
	action = opensearchAction(action)

	switch action {
	case catalog.ActionOpenSearchCreateDomain:
		s.opensearchCreate(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionOpenSearchDescribeDomain:
		s.opensearchDescribe(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionOpenSearchListDomainNames:
		s.opensearchList(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionOpenSearchDeleteDomain:
		s.opensearchDelete(w, r, body, requestID, eventID, verified, readOnly, params)
	default:
		s.writeOpenSearchError(w, r, body, requestID, http.StatusNotImplemented, "InternalFailure",
			"This OpenSearch action is not implemented.", readOnly, eventID, verified)
	}
}

func opensearchAction(action string) string {
	if strings.Contains(action, ":") {
		return action
	}
	switch action {
	case "CreateDomain":
		return catalog.ActionOpenSearchCreateDomain
	case "DescribeDomain":
		return catalog.ActionOpenSearchDescribeDomain
	case "ListDomainNames":
		return catalog.ActionOpenSearchListDomainNames
	case "DeleteDomain":
		return catalog.ActionOpenSearchDeleteDomain
	default:
		return action
	}
}

func (s *Server) opensearchRegion(verified *authn.Verified) string {
	if verified.Region != "" {
		return verified.Region
	}
	return store.DefaultOpenSearchRegion
}

func (s *Server) opensearchCreate(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	name, _ := params["DomainName"].(string)
	version, _ := params["EngineVersion"].(string)
	if !s.authorize(verified, catalog.ActionOpenSearchCreateDomain, "*") {
		s.writeOpenSearchError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform es:CreateDomain.", readOnly, eventID, verified)
		return
	}
	d, err := s.store.CreateOpenSearchDomain(verified.AccountID, s.opensearchRegion(verified), name, version)
	if errors.Is(err, store.ErrOpenSearchDomainExists) {
		s.writeOpenSearchError(w, r, body, requestID, http.StatusConflict, "ResourceAlreadyExistsException",
			"Domain already exists.", readOnly, eventID, verified)
		return
	}
	if errors.Is(err, store.ErrOpenSearchBadRequest) {
		s.writeOpenSearchError(w, r, body, requestID, http.StatusBadRequest, "ValidationException",
			err.Error(), readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeOpenSearchError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to create domain.", readOnly, eventID, verified)
		return
	}
	// Nested OpenSearch on Internal noctaxris-data; Active only after healthy wait (never Active on stub://).
	_ = tryStartNestedDataEngine(s, verified.AccountID, "opensearch", d.DomainName, map[string]string{
		"discovery.type":              "single-node",
		"DISABLE_SECURITY_PLUGIN":     "true",
		"DISABLE_INSTALL_DEMO_CONFIG": "true",
		"bootstrap.memory_lock":       "false",
		"OPENSEARCH_JAVA_OPTS":        "-Xms512m -Xmx512m",
	})
	if updated, err := s.store.DescribeOpenSearchDomain(verified.AccountID, d.DomainName); err == nil {
		d = updated
	}
	payload, _ := ossvc.CreateDomainJSON(d)
	s.writeOpenSearchOK(w, requestID, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, opensearchEventSource, "CreateDomain", readOnly)
}

func (s *Server) opensearchDescribe(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	name, _ := params["DomainName"].(string)
	if !s.authorize(verified, catalog.ActionOpenSearchDescribeDomain, "*") {
		s.writeOpenSearchError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform es:DescribeDomain.", readOnly, eventID, verified)
		return
	}
	d, err := s.store.DescribeOpenSearchDomain(verified.AccountID, name)
	if errors.Is(err, store.ErrOpenSearchDomainNotFound) {
		s.writeOpenSearchError(w, r, body, requestID, http.StatusNotFound, "ResourceNotFoundException",
			"Domain not found.", readOnly, eventID, verified)
		return
	}
	if errors.Is(err, store.ErrOpenSearchBadRequest) {
		s.writeOpenSearchError(w, r, body, requestID, http.StatusBadRequest, "ValidationException",
			err.Error(), readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeOpenSearchError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to describe domain.", readOnly, eventID, verified)
		return
	}
	payload, _ := ossvc.DescribeDomainJSON(d)
	s.writeOpenSearchOK(w, requestID, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, opensearchEventSource, "DescribeDomain", readOnly)
}

func (s *Server) opensearchList(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	_ = params
	if !s.authorize(verified, catalog.ActionOpenSearchListDomainNames, "*") {
		s.writeOpenSearchError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform es:ListDomainNames.", readOnly, eventID, verified)
		return
	}
	domains, err := s.store.ListOpenSearchDomainNames(verified.AccountID)
	if err != nil {
		s.writeOpenSearchError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to list domains.", readOnly, eventID, verified)
		return
	}
	payload, _ := ossvc.ListDomainNamesJSON(domains)
	s.writeOpenSearchOK(w, requestID, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, opensearchEventSource, "ListDomainNames", readOnly)
}

func (s *Server) opensearchDelete(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	name, _ := params["DomainName"].(string)
	if !s.authorize(verified, catalog.ActionOpenSearchDeleteDomain, "*") {
		s.writeOpenSearchError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform es:DeleteDomain.", readOnly, eventID, verified)
		return
	}
	d, err := s.store.DescribeOpenSearchDomain(verified.AccountID, name)
	if errors.Is(err, store.ErrOpenSearchDomainNotFound) {
		s.writeOpenSearchError(w, r, body, requestID, http.StatusNotFound, "ResourceNotFoundException",
			"Domain not found.", readOnly, eventID, verified)
		return
	}
	if err != nil && !errors.Is(err, store.ErrOpenSearchBadRequest) {
		s.writeOpenSearchError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to delete domain.", readOnly, eventID, verified)
		return
	}
	if errors.Is(err, store.ErrOpenSearchBadRequest) {
		s.writeOpenSearchError(w, r, body, requestID, http.StatusBadRequest, "ValidationException",
			err.Error(), readOnly, eventID, verified)
		return
	}
	containerID, err := s.store.DeleteOpenSearchDomain(verified.AccountID, name)
	if err != nil {
		s.writeOpenSearchError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to delete domain.", readOnly, eventID, verified)
		return
	}
	_ = tryStopNestedDataEngine(s, containerID)
	payload, _ := ossvc.DeleteDomainJSON(d)
	s.writeOpenSearchOK(w, requestID, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, opensearchEventSource, "DeleteDomain", readOnly)
}

func (s *Server) writeOpenSearchOK(w http.ResponseWriter, requestID string, payload []byte) {
	w.Header().Set("Content-Type", opensearchJSONContentType)
	w.Header().Set("x-amzn-RequestId", requestID)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(payload)
}

func (s *Server) writeOpenSearchError(
	w http.ResponseWriter, r *http.Request, body []byte, requestID string,
	status int, code, message string, readOnly bool, eventID string, verified *authn.Verified,
) {
	w.Header().Set("Content-Type", opensearchJSONContentType)
	w.Header().Set("x-amzn-RequestId", requestID)
	w.Header().Set("x-amzn-ErrorType", code)
	w.WriteHeader(status)
	escaped := strings.ReplaceAll(message, `"`, `'`)
	_, _ = w.Write([]byte(`{"__type":"` + code + `","message":"` + escaped + `"}`))
	_ = body
	_ = r
	_ = readOnly
	_ = eventID
	_ = verified
}
