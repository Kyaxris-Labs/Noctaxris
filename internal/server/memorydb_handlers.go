package server

import (
	"errors"
	"net/http"
	"strings"

	"github.com/Kyaxris-Labs/Noctaxris/internal/catalog"
	"github.com/Kyaxris-Labs/Noctaxris/internal/kernel/authn"
	mdbsvc "github.com/Kyaxris-Labs/Noctaxris/internal/services/memorydb"
	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

const (
	memorydbJSONContentType = "application/x-amz-json-1.1"
	memorydbEventSource     = "memorydb.amazonaws.com"
)

func (s *Server) handleMemoryDB(
	w http.ResponseWriter,
	r *http.Request,
	body []byte,
	requestID, eventID, action string,
	verified *authn.Verified,
	readOnly bool,
) {
	params := jsonBodyMap(body)
	action = memorydbAction(action)

	switch action {
	case catalog.ActionMemoryDBCreateCluster:
		s.memorydbCreate(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionMemoryDBDescribeClusters:
		s.memorydbDescribe(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionMemoryDBDeleteCluster:
		s.memorydbDelete(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionMemoryDBDescribeUsers:
		s.memorydbDescribeUsers(w, r, body, requestID, eventID, verified, readOnly)
	case catalog.ActionMemoryDBDescribeACLs:
		s.memorydbDescribeACLs(w, r, body, requestID, eventID, verified, readOnly)
	default:
		s.writeMemoryDBError(w, r, body, requestID, http.StatusBadRequest, "InvalidAction",
			"This MemoryDB action is not implemented.", readOnly, eventID, verified)
	}
}

func memorydbAction(action string) string {
	if strings.Contains(action, ":") {
		return action
	}
	switch action {
	case "CreateCluster":
		return catalog.ActionMemoryDBCreateCluster
	case "DescribeClusters":
		return catalog.ActionMemoryDBDescribeClusters
	case "DeleteCluster":
		return catalog.ActionMemoryDBDeleteCluster
	case "DescribeUsers":
		return catalog.ActionMemoryDBDescribeUsers
	case "DescribeACLs":
		return catalog.ActionMemoryDBDescribeACLs
	default:
		return action
	}
}

func (s *Server) memorydbRegion(verified *authn.Verified) string {
	if verified.Region != "" {
		return verified.Region
	}
	return store.DefaultMemoryDBRegion
}

func (s *Server) memorydbCreate(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	name, _ := params["ClusterName"].(string)
	engine, _ := params["Engine"].(string)
	version, _ := params["EngineVersion"].(string)
	nodeType, _ := params["NodeType"].(string)
	aclName, _ := params["ACLName"].(string)
	numShards := 1
	switch v := params["NumShards"].(type) {
	case float64:
		numShards = int(v)
	case int:
		numShards = v
	}
	if !s.authorize(verified, catalog.ActionMemoryDBCreateCluster, "*") {
		s.writeMemoryDBError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform memorydb:CreateCluster.", readOnly, eventID, verified)
		return
	}
	c, err := s.store.CreateMemoryDBCluster(
		verified.AccountID, s.memorydbRegion(verified), name, engine, version, nodeType, aclName, numShards,
	)
	if errors.Is(err, store.ErrMemoryDBClusterExists) {
		s.writeMemoryDBError(w, r, body, requestID, http.StatusBadRequest, "ClusterAlreadyExistsFault",
			"Cluster already exists.", readOnly, eventID, verified)
		return
	}
	if errors.Is(err, store.ErrMemoryDBBadRequest) {
		s.writeMemoryDBError(w, r, body, requestID, http.StatusBadRequest, "InvalidParameterValueException",
			err.Error(), readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeMemoryDBError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to create cluster.", readOnly, eventID, verified)
		return
	}
	_, _ = s.store.EnsureDataPlaneMasterSecret(
		verified.AccountID, s.memorydbRegion(verified),
		store.DataPlaneSecretMemoryDB, c.Name, "default", "noctaxris-memorydb-lab",
	)
	// Nested Valkey/Redis lab image has no AUTH by default. Secret is for control-plane labs.
	_ = tryStartNestedDataEngine(s, verified.AccountID, "memorydb", c.Name, nil)
	if updated, err := s.store.DescribeMemoryDBCluster(verified.AccountID, s.memorydbRegion(verified), c.Name); err == nil {
		c = updated
	}
	payload, _ := mdbsvc.CreateClusterJSON(c)
	s.writeMemoryDBOK(w, requestID, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, memorydbEventSource, "CreateCluster", readOnly)
}

func (s *Server) memorydbDescribe(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	name, _ := params["ClusterName"].(string)
	if !s.authorize(verified, catalog.ActionMemoryDBDescribeClusters, "*") {
		s.writeMemoryDBError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform memorydb:DescribeClusters.", readOnly, eventID, verified)
		return
	}
	clusters, err := s.store.DescribeMemoryDBClusters(verified.AccountID, s.memorydbRegion(verified), name)
	if err != nil {
		s.writeMemoryDBError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to describe clusters.", readOnly, eventID, verified)
		return
	}
	if name != "" && len(clusters) == 0 {
		s.writeMemoryDBError(w, r, body, requestID, http.StatusBadRequest, "ClusterNotFoundFault",
			"Cluster not found.", readOnly, eventID, verified)
		return
	}
	payload, _ := mdbsvc.DescribeClustersJSON(clusters)
	s.writeMemoryDBOK(w, requestID, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, memorydbEventSource, "DescribeClusters", readOnly)
}

func (s *Server) memorydbDelete(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	name, _ := params["ClusterName"].(string)
	if !s.authorize(verified, catalog.ActionMemoryDBDeleteCluster, "*") {
		s.writeMemoryDBError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform memorydb:DeleteCluster.", readOnly, eventID, verified)
		return
	}
	existing, err := s.store.DescribeMemoryDBCluster(verified.AccountID, s.memorydbRegion(verified), name)
	if errors.Is(err, store.ErrMemoryDBClusterNotFound) {
		s.writeMemoryDBError(w, r, body, requestID, http.StatusBadRequest, "ClusterNotFoundFault",
			"Cluster not found.", readOnly, eventID, verified)
		return
	}
	if errors.Is(err, store.ErrMemoryDBBadRequest) {
		s.writeMemoryDBError(w, r, body, requestID, http.StatusBadRequest, "InvalidParameterValueException",
			err.Error(), readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeMemoryDBError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to delete cluster.", readOnly, eventID, verified)
		return
	}
	containerID, err := s.store.DeleteMemoryDBCluster(verified.AccountID, name)
	if err != nil {
		s.writeMemoryDBError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to delete cluster.", readOnly, eventID, verified)
		return
	}
	_ = tryStopNestedDataEngine(s, containerID)
	payload, _ := mdbsvc.DeleteClusterJSON(existing)
	s.writeMemoryDBOK(w, requestID, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, memorydbEventSource, "DeleteCluster", readOnly)
}

func (s *Server) memorydbDescribeUsers(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool,
) {
	if !s.authorize(verified, catalog.ActionMemoryDBDescribeUsers, "*") {
		s.writeMemoryDBError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform memorydb:DescribeUsers.", readOnly, eventID, verified)
		return
	}
	payload, _ := mdbsvc.DescribeUsersJSON()
	s.writeMemoryDBOK(w, requestID, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, memorydbEventSource, "DescribeUsers", readOnly)
}

func (s *Server) memorydbDescribeACLs(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool,
) {
	if !s.authorize(verified, catalog.ActionMemoryDBDescribeACLs, "*") {
		s.writeMemoryDBError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform memorydb:DescribeACLs.", readOnly, eventID, verified)
		return
	}
	payload, _ := mdbsvc.DescribeACLsJSON()
	s.writeMemoryDBOK(w, requestID, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, memorydbEventSource, "DescribeACLs", readOnly)
}

func (s *Server) writeMemoryDBOK(w http.ResponseWriter, requestID string, payload []byte) {
	w.Header().Set("Content-Type", memorydbJSONContentType)
	w.Header().Set("x-amzn-RequestId", requestID)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(payload)
}

func (s *Server) writeMemoryDBError(
	w http.ResponseWriter, r *http.Request, body []byte, requestID string,
	status int, code, message string, readOnly bool, eventID string, verified *authn.Verified,
) {
	w.Header().Set("Content-Type", memorydbJSONContentType)
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
