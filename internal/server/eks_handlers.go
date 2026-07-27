package server

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strings"

	"github.com/Kyaxris-Labs/Noctaxris/internal/catalog"
	"github.com/Kyaxris-Labs/Noctaxris/internal/kernel/authn"
	ekssvc "github.com/Kyaxris-Labs/Noctaxris/internal/services/eks"
	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

const (
	eksJSONContentType = "application/json"
	eksEventSource     = "eks.amazonaws.com"
)

func (s *Server) handleEKS(
	w http.ResponseWriter,
	r *http.Request,
	body []byte,
	requestID, eventID, action string,
	verified *authn.Verified,
	readOnly bool,
) {
	if action == "" {
		action = resolveEKSREST(r)
	}
	params := jsonBodyMap(body)
	if name := eksClusterNameFromPath(r.URL.Path); name != "" {
		if _, ok := params["name"]; !ok {
			params["name"] = name
		}
		if _, ok := params["Name"]; !ok {
			params["Name"] = name
		}
	}
	action = eksAction(action)

	switch action {
	case catalog.ActionEKSCreateCluster:
		s.eksCreate(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionEKSDescribeCluster:
		s.eksDescribe(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionEKSListClusters:
		s.eksList(w, r, body, requestID, eventID, verified, readOnly)
	case catalog.ActionEKSDeleteCluster:
		s.eksDelete(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionEKSListNodegroups:
		s.eksListNodegroups(w, r, body, requestID, eventID, verified, readOnly, params)
	default:
		s.writeEKSError(w, r, body, requestID, http.StatusNotImplemented, "InvalidRequestException",
			"This EKS action is not implemented.", readOnly, eventID, verified)
	}
}

func eksAction(action string) string {
	if strings.Contains(action, ":") {
		return action
	}
	switch action {
	case "CreateCluster":
		return catalog.ActionEKSCreateCluster
	case "DescribeCluster":
		return catalog.ActionEKSDescribeCluster
	case "ListClusters":
		return catalog.ActionEKSListClusters
	case "DeleteCluster":
		return catalog.ActionEKSDeleteCluster
	case "ListNodegroups":
		return catalog.ActionEKSListNodegroups
	default:
		return action
	}
}

func isEKSRESTPath(path string) bool {
	p := strings.ToLower(strings.TrimSuffix(path, "/"))
	return p == "/clusters" || strings.HasPrefix(p, "/clusters/")
}

func resolveEKSREST(r *http.Request) string {
	p := strings.TrimSuffix(r.URL.Path, "/")
	lower := strings.ToLower(p)
	method := strings.ToUpper(r.Method)
	parts := strings.Split(strings.Trim(lower, "/"), "/")
	switch {
	case lower == "/clusters" && method == http.MethodPost:
		return catalog.ActionEKSCreateCluster
	case lower == "/clusters" && method == http.MethodGet:
		return catalog.ActionEKSListClusters
	case len(parts) == 2 && parts[0] == "clusters" && method == http.MethodGet:
		return catalog.ActionEKSDescribeCluster
	case len(parts) == 2 && parts[0] == "clusters" && method == http.MethodDelete:
		return catalog.ActionEKSDeleteCluster
	case len(parts) == 3 && parts[0] == "clusters" && parts[2] == "node-groups" && method == http.MethodGet:
		return catalog.ActionEKSListNodegroups
	default:
		return ""
	}
}

func eksClusterNameFromPath(path string) string {
	p := strings.TrimSuffix(path, "/")
	lower := strings.ToLower(p)
	if !strings.HasPrefix(lower, "/clusters/") {
		return ""
	}
	rest := p[len("/clusters/"):]
	if i := strings.Index(rest, "/"); i >= 0 {
		rest = rest[:i]
	}
	decoded, err := url.PathUnescape(rest)
	if err != nil {
		return strings.TrimSpace(rest)
	}
	return strings.TrimSpace(decoded)
}

func (s *Server) eksRegion(verified *authn.Verified) string {
	if verified.Region != "" {
		return verified.Region
	}
	return store.DefaultEKSRegion
}

func (s *Server) eksCreate(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	if !s.authorize(verified, catalog.ActionEKSCreateCluster, "*") {
		s.writeEKSError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform eks:CreateCluster.", readOnly, eventID, verified)
		return
	}
	name, _ := params["name"].(string)
	if name == "" {
		name, _ = params["Name"].(string)
	}
	role, _ := params["roleArn"].(string)
	if role == "" {
		role, _ = params["RoleArn"].(string)
	}
	version, _ := params["version"].(string)
	if version == "" {
		version, _ = params["Version"].(string)
	}
	vpcJSON := "{}"
	if raw, ok := params["resourcesVpcConfig"]; ok {
		b, err := json.Marshal(raw)
		if err == nil {
			vpcJSON = string(b)
		}
	} else if raw, ok := params["ResourcesVpcConfig"]; ok {
		b, err := json.Marshal(raw)
		if err == nil {
			vpcJSON = string(b)
		}
	}
	c, err := s.store.CreateEKSCluster(verified.AccountID, s.eksRegion(verified), name, role, version, vpcJSON)
	if errors.Is(err, store.ErrEKSClusterExists) {
		s.writeEKSError(w, r, body, requestID, http.StatusConflict, "ResourceInUseException",
			"Cluster already exists.", readOnly, eventID, verified)
		return
	}
	if errors.Is(err, store.ErrEKSBadRequest) {
		s.writeEKSError(w, r, body, requestID, http.StatusBadRequest, "InvalidParameterException",
			err.Error(), readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeEKSError(w, r, body, requestID, http.StatusInternalServerError, "ServerException",
			"Unable to create cluster.", readOnly, eventID, verified)
		return
	}
	payload, _ := ekssvc.CreateClusterJSON(c)
	s.writeEKSOK(w, requestID, http.StatusOK, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, eksEventSource, "CreateCluster", readOnly)
}

func (s *Server) eksDescribe(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	if !s.authorize(verified, catalog.ActionEKSDescribeCluster, "*") {
		s.writeEKSError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform eks:DescribeCluster.", readOnly, eventID, verified)
		return
	}
	name, _ := params["name"].(string)
	if name == "" {
		name, _ = params["Name"].(string)
	}
	c, err := s.store.DescribeEKSCluster(verified.AccountID, name)
	if errors.Is(err, store.ErrEKSClusterNotFound) {
		s.writeEKSError(w, r, body, requestID, http.StatusNotFound, "ResourceNotFoundException",
			"No cluster found for name.", readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeEKSError(w, r, body, requestID, http.StatusInternalServerError, "ServerException",
			"Unable to describe cluster.", readOnly, eventID, verified)
		return
	}
	payload, _ := ekssvc.DescribeClusterJSON(c)
	s.writeEKSOK(w, requestID, http.StatusOK, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, eksEventSource, "DescribeCluster", readOnly)
}

func (s *Server) eksList(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool,
) {
	if !s.authorize(verified, catalog.ActionEKSListClusters, "*") {
		s.writeEKSError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform eks:ListClusters.", readOnly, eventID, verified)
		return
	}
	clusters, err := s.store.ListEKSClusters(verified.AccountID)
	if err != nil {
		s.writeEKSError(w, r, body, requestID, http.StatusInternalServerError, "ServerException",
			"Unable to list clusters.", readOnly, eventID, verified)
		return
	}
	payload, _ := ekssvc.ListClustersJSON(clusters)
	s.writeEKSOK(w, requestID, http.StatusOK, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, eksEventSource, "ListClusters", readOnly)
}

func (s *Server) eksDelete(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	if !s.authorize(verified, catalog.ActionEKSDeleteCluster, "*") {
		s.writeEKSError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform eks:DeleteCluster.", readOnly, eventID, verified)
		return
	}
	name, _ := params["name"].(string)
	if name == "" {
		name, _ = params["Name"].(string)
	}
	c, err := s.store.DeleteEKSCluster(verified.AccountID, name)
	if errors.Is(err, store.ErrEKSClusterNotFound) {
		s.writeEKSError(w, r, body, requestID, http.StatusNotFound, "ResourceNotFoundException",
			"No cluster found for name.", readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeEKSError(w, r, body, requestID, http.StatusInternalServerError, "ServerException",
			"Unable to delete cluster.", readOnly, eventID, verified)
		return
	}
	payload, _ := ekssvc.DeleteClusterJSON(c)
	s.writeEKSOK(w, requestID, http.StatusOK, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, eksEventSource, "DeleteCluster", readOnly)
}

func (s *Server) eksListNodegroups(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	if !s.authorize(verified, catalog.ActionEKSListNodegroups, "*") {
		s.writeEKSError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform eks:ListNodegroups.", readOnly, eventID, verified)
		return
	}
	name, _ := params["name"].(string)
	if name == "" {
		name, _ = params["Name"].(string)
	}
	if _, err := s.store.DescribeEKSCluster(verified.AccountID, name); errors.Is(err, store.ErrEKSClusterNotFound) {
		s.writeEKSError(w, r, body, requestID, http.StatusNotFound, "ResourceNotFoundException",
			"No cluster found for name.", readOnly, eventID, verified)
		return
	} else if err != nil {
		s.writeEKSError(w, r, body, requestID, http.StatusInternalServerError, "ServerException",
			"Unable to list nodegroups.", readOnly, eventID, verified)
		return
	}
	payload, _ := ekssvc.ListNodegroupsJSON()
	s.writeEKSOK(w, requestID, http.StatusOK, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, eksEventSource, "ListNodegroups", readOnly)
}

func (s *Server) writeEKSOK(w http.ResponseWriter, requestID string, status int, payload []byte) {
	w.Header().Set("Content-Type", eksJSONContentType)
	w.Header().Set("x-amzn-RequestId", requestID)
	w.WriteHeader(status)
	_, _ = w.Write(payload)
}

func (s *Server) writeEKSError(
	w http.ResponseWriter, r *http.Request, body []byte, requestID string,
	status int, code, message string, readOnly bool, eventID string, verified *authn.Verified,
) {
	w.Header().Set("Content-Type", eksJSONContentType)
	w.Header().Set("x-amzn-RequestId", requestID)
	w.Header().Set("x-amzn-ErrorType", code)
	w.WriteHeader(status)
	raw, _ := json.Marshal(map[string]any{"message": message})
	_, _ = w.Write(raw)
	_ = body
	_ = r
	_ = readOnly
	_ = eventID
	_ = verified
}
