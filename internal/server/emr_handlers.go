package server

import (
	"errors"
	"net/http"
	"strings"

	"github.com/Kyaxris-Labs/Noctaxris/internal/catalog"
	"github.com/Kyaxris-Labs/Noctaxris/internal/kernel/authn"
	emrsvc "github.com/Kyaxris-Labs/Noctaxris/internal/services/emr"
	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

const (
	emrJSONContentType = "application/x-amz-json-1.1"
	emrEventSource     = "elasticmapreduce.amazonaws.com"
)

func (s *Server) handleEMR(
	w http.ResponseWriter,
	r *http.Request,
	body []byte,
	requestID, eventID, action string,
	verified *authn.Verified,
	readOnly bool,
) {
	params := jsonBodyMap(body)
	action = emrAction(action)

	switch action {
	case catalog.ActionEMRRunJobFlow:
		s.emrRunJobFlow(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionEMRDescribeCluster:
		s.emrDescribe(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionEMRListClusters:
		s.emrList(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionEMRTerminateJobFlows:
		s.emrTerminate(w, r, body, requestID, eventID, verified, readOnly, params)
	default:
		s.writeEMRError(w, requestID, http.StatusNotImplemented, "InternalFailure",
			"This EMR action is not implemented.")
	}
}

func emrAction(action string) string {
	if strings.Contains(action, ":") {
		return action
	}
	switch action {
	case "RunJobFlow":
		return catalog.ActionEMRRunJobFlow
	case "DescribeCluster":
		return catalog.ActionEMRDescribeCluster
	case "ListClusters":
		return catalog.ActionEMRListClusters
	case "TerminateJobFlows":
		return catalog.ActionEMRTerminateJobFlows
	default:
		return action
	}
}

func (s *Server) emrRegion(verified *authn.Verified) string {
	if verified.Region != "" {
		return verified.Region
	}
	return store.DefaultEMRRegion
}

func (s *Server) emrRunJobFlow(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	if !s.authorize(verified, catalog.ActionEMRRunJobFlow, "*") {
		s.writeEMRError(w, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform elasticmapreduce:RunJobFlow.")
		return
	}
	name, _ := params["Name"].(string)
	release, _ := params["ReleaseLabel"].(string)
	logURI, _ := params["LogUri"].(string)
	c, err := s.store.RunEMRJobFlow(verified.AccountID, s.emrRegion(verified), name, release, logURI)
	if errors.Is(err, store.ErrEMRValidation) {
		s.writeEMRError(w, requestID, http.StatusBadRequest, "ValidationException", err.Error())
		return
	}
	if err != nil {
		s.writeEMRError(w, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to run job flow.")
		return
	}
	payload, _ := emrsvc.RunJobFlowJSON(c)
	s.writeEMROK(w, requestID, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, emrEventSource, "RunJobFlow", readOnly)
}

func (s *Server) emrDescribe(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	if !s.authorize(verified, catalog.ActionEMRDescribeCluster, "*") {
		s.writeEMRError(w, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform elasticmapreduce:DescribeCluster.")
		return
	}
	id, _ := params["ClusterId"].(string)
	c, err := s.store.DescribeEMRCluster(verified.AccountID, id)
	if errors.Is(err, store.ErrEMRNotFound) {
		s.writeEMRError(w, requestID, http.StatusBadRequest, "InvalidRequestException",
			"Cluster not found.")
		return
	}
	if err != nil {
		s.writeEMRError(w, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to describe cluster.")
		return
	}
	payload, _ := emrsvc.DescribeClusterJSON(c)
	s.writeEMROK(w, requestID, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, emrEventSource, "DescribeCluster", readOnly)
}

func (s *Server) emrList(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	_ = params
	if !s.authorize(verified, catalog.ActionEMRListClusters, "*") {
		s.writeEMRError(w, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform elasticmapreduce:ListClusters.")
		return
	}
	clusters, err := s.store.ListEMRClusters(verified.AccountID)
	if err != nil {
		s.writeEMRError(w, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to list clusters.")
		return
	}
	payload, _ := emrsvc.ListClustersJSON(clusters)
	s.writeEMROK(w, requestID, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, emrEventSource, "ListClusters", readOnly)
}

func (s *Server) emrTerminate(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	if !s.authorize(verified, catalog.ActionEMRTerminateJobFlows, "*") {
		s.writeEMRError(w, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform elasticmapreduce:TerminateJobFlows.")
		return
	}
	var ids []string
	if raw, ok := params["JobFlowIds"].([]any); ok {
		for _, v := range raw {
			if s, ok := v.(string); ok {
				ids = append(ids, s)
			}
		}
	}
	err := s.store.TerminateEMRJobFlows(verified.AccountID, ids)
	if errors.Is(err, store.ErrEMRValidation) {
		s.writeEMRError(w, requestID, http.StatusBadRequest, "ValidationException", err.Error())
		return
	}
	if errors.Is(err, store.ErrEMRNotFound) {
		s.writeEMRError(w, requestID, http.StatusBadRequest, "InvalidRequestException", err.Error())
		return
	}
	if err != nil {
		s.writeEMRError(w, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to terminate job flows.")
		return
	}
	w.Header().Set("Content-Type", emrJSONContentType)
	w.Header().Set("x-amzn-RequestId", requestID)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{}`))
	s.writeSuccessAudit(r, requestID, eventID, verified, emrEventSource, "TerminateJobFlows", readOnly)
}

func (s *Server) writeEMROK(w http.ResponseWriter, requestID string, payload []byte) {
	w.Header().Set("Content-Type", emrJSONContentType)
	w.Header().Set("x-amzn-RequestId", requestID)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(payload)
}

func (s *Server) writeEMRError(w http.ResponseWriter, requestID string, status int, code, message string) {
	w.Header().Set("Content-Type", emrJSONContentType)
	w.Header().Set("x-amzn-RequestId", requestID)
	w.Header().Set("x-amzn-ErrorType", code)
	w.WriteHeader(status)
	_, _ = w.Write([]byte(`{"__type":"` + code + `","message":"` + message + `"}`))
}
