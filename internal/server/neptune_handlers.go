package server

import (
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/Kyaxris-Labs/Noctaxris/internal/catalog"
	"github.com/Kyaxris-Labs/Noctaxris/internal/kernel/authn"
	neptunesvc "github.com/Kyaxris-Labs/Noctaxris/internal/services/neptune"
	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

const neptuneEventSource = "rds.amazonaws.com"

func (s *Server) handleNeptune(
	w http.ResponseWriter,
	r *http.Request,
	body []byte,
	requestID, eventID, action string,
	verified *authn.Verified,
	readOnly bool,
) {
	params := formParams(r, body)
	action = neptuneAction(action)

	switch action {
	case catalog.ActionNeptuneCreateDBCluster:
		s.neptuneCreate(w, r, requestID, eventID, verified, readOnly, params)
	case catalog.ActionNeptuneDescribeDBClusters:
		s.neptuneDescribe(w, r, requestID, eventID, verified, readOnly, params)
	case catalog.ActionNeptuneDeleteDBCluster:
		s.neptuneDelete(w, r, requestID, eventID, verified, readOnly, params)
	default:
		s.writeNeptuneError(w, r, requestID, http.StatusBadRequest, "InvalidAction",
			"This Neptune action is not implemented.", readOnly, eventID, verified)
	}
}

func neptuneAction(action string) string {
	if strings.Contains(action, ":") {
		return action
	}
	switch action {
	case "CreateDBCluster":
		return catalog.ActionNeptuneCreateDBCluster
	case "DescribeDBClusters":
		return catalog.ActionNeptuneDescribeDBClusters
	case "DeleteDBCluster":
		return catalog.ActionNeptuneDeleteDBCluster
	default:
		return action
	}
}

func isNeptuneControlPlaneAction(action string) bool {
	a := neptuneAction(action)
	switch a {
	case catalog.ActionNeptuneCreateDBCluster,
		catalog.ActionNeptuneDescribeDBClusters,
		catalog.ActionNeptuneDeleteDBCluster,
		"CreateDBCluster", "DescribeDBClusters", "DeleteDBCluster":
		return true
	default:
		return false
	}
}

func (s *Server) neptuneRegion(verified *authn.Verified) string {
	if verified.Region != "" {
		return verified.Region
	}
	return store.DefaultNeptuneRegion
}

func (s *Server) neptuneCreate(
	w http.ResponseWriter, r *http.Request, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params url.Values,
) {
	id := strings.TrimSpace(params.Get("DBClusterIdentifier"))
	engine := strings.TrimSpace(params.Get("Engine"))
	version := strings.TrimSpace(params.Get("EngineVersion"))
	port := 0
	if p := strings.TrimSpace(params.Get("Port")); p != "" {
		if v, err := strconv.Atoi(p); err == nil {
			port = v
		}
	}
	if !s.authorize(verified, catalog.ActionDocDBCreateDBCluster, "*") {
		s.writeNeptuneError(w, r, requestID, http.StatusForbidden, "AccessDenied",
			"User is not authorized to perform rds:CreateDBCluster.", readOnly, eventID, verified)
		return
	}
	c, err := s.store.CreateNeptuneCluster(verified.AccountID, s.neptuneRegion(verified), id, engine, version, port)
	if errors.Is(err, store.ErrNeptuneClusterExists) {
		s.writeNeptuneError(w, r, requestID, http.StatusBadRequest, "DBClusterAlreadyExistsFault",
			"DB cluster already exists.", readOnly, eventID, verified)
		return
	}
	if errors.Is(err, store.ErrNeptuneBadRequest) {
		s.writeNeptuneError(w, r, requestID, http.StatusBadRequest, "InvalidParameterValue",
			err.Error(), readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeNeptuneError(w, r, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to create DB cluster.", readOnly, eventID, verified)
		return
	}
	_ = tryStartNestedDataEngine(s, verified.AccountID, "neptune", c.DBClusterIdentifier, nil)
	if updated, err := s.store.DescribeNeptuneCluster(verified.AccountID, c.DBClusterIdentifier); err == nil {
		c = updated
	}
	payload, _ := neptunesvc.CreateDBClusterXML(c, requestID)
	s.writeNeptuneOK(w, requestID, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, neptuneEventSource, "CreateDBCluster", readOnly)
}

func (s *Server) neptuneDescribe(
	w http.ResponseWriter, r *http.Request, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params url.Values,
) {
	id := strings.TrimSpace(params.Get("DBClusterIdentifier"))
	if !s.authorize(verified, catalog.ActionDocDBDescribeDBClusters, "*") {
		s.writeNeptuneError(w, r, requestID, http.StatusForbidden, "AccessDenied",
			"User is not authorized to perform rds:DescribeDBClusters.", readOnly, eventID, verified)
		return
	}
	clusters, err := s.store.DescribeNeptuneClusters(verified.AccountID, id)
	if err != nil {
		s.writeNeptuneError(w, r, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to describe DB clusters.", readOnly, eventID, verified)
		return
	}
	if id != "" && len(clusters) == 0 {
		s.writeNeptuneError(w, r, requestID, http.StatusBadRequest, "DBClusterNotFoundFault",
			"DB cluster not found.", readOnly, eventID, verified)
		return
	}
	payload, _ := neptunesvc.DescribeDBClustersXML(clusters, requestID)
	s.writeNeptuneOK(w, requestID, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, neptuneEventSource, "DescribeDBClusters", readOnly)
}

func (s *Server) neptuneDelete(
	w http.ResponseWriter, r *http.Request, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params url.Values,
) {
	id := strings.TrimSpace(params.Get("DBClusterIdentifier"))
	if !s.authorize(verified, catalog.ActionDocDBDeleteDBCluster, "*") {
		s.writeNeptuneError(w, r, requestID, http.StatusForbidden, "AccessDenied",
			"User is not authorized to perform rds:DeleteDBCluster.", readOnly, eventID, verified)
		return
	}
	existing, err := s.store.DescribeNeptuneCluster(verified.AccountID, id)
	if errors.Is(err, store.ErrNeptuneClusterNotFound) {
		s.writeNeptuneError(w, r, requestID, http.StatusBadRequest, "DBClusterNotFoundFault",
			"DB cluster not found.", readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeNeptuneError(w, r, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to delete DB cluster.", readOnly, eventID, verified)
		return
	}
	containerID, err := s.store.DeleteNeptuneCluster(verified.AccountID, id)
	if err != nil {
		s.writeNeptuneError(w, r, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to delete DB cluster.", readOnly, eventID, verified)
		return
	}
	_ = tryStopNestedDataEngine(s, containerID)
	payload, _ := neptunesvc.DeleteDBClusterXML(existing, requestID)
	s.writeNeptuneOK(w, requestID, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, neptuneEventSource, "DeleteDBCluster", readOnly)
}

func (s *Server) writeNeptuneOK(w http.ResponseWriter, requestID string, payload []byte) {
	w.Header().Set(requestIDHeader, requestID)
	w.Header().Set("Content-Type", "text/xml")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(payload)
}

func (s *Server) writeNeptuneError(
	w http.ResponseWriter, r *http.Request, requestID string,
	status int, code, message string, readOnly bool, eventID string, verified *authn.Verified,
) {
	payload, _ := neptunesvc.ErrorXML(code, message, requestID)
	w.Header().Set(requestIDHeader, requestID)
	w.Header().Set("Content-Type", "text/xml")
	w.WriteHeader(status)
	_, _ = w.Write(payload)
	_ = r
	_ = readOnly
	_ = eventID
	_ = verified
}
