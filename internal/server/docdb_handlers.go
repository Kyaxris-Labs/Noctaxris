package server

import (
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/Kyaxris-Labs/Noctaxris/internal/catalog"
	"github.com/Kyaxris-Labs/Noctaxris/internal/kernel/authn"
	docdbsvc "github.com/Kyaxris-Labs/Noctaxris/internal/services/docdb"
	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

const docdbEventSource = "rds.amazonaws.com"

func (s *Server) handleDocDB(
	w http.ResponseWriter,
	r *http.Request,
	body []byte,
	requestID, eventID, action string,
	verified *authn.Verified,
	readOnly bool,
) {
	params := formParams(r, body)
	action = docdbAction(action)

	switch action {
	case catalog.ActionDocDBCreateDBCluster:
		s.docdbCreate(w, r, requestID, eventID, verified, readOnly, params)
	case catalog.ActionDocDBDescribeDBClusters:
		s.docdbDescribe(w, r, requestID, eventID, verified, readOnly, params)
	case catalog.ActionDocDBDeleteDBCluster:
		s.docdbDelete(w, r, requestID, eventID, verified, readOnly, params)
	default:
		s.writeDocDBError(w, r, requestID, http.StatusBadRequest, "InvalidAction",
			"This DocumentDB action is not implemented.", readOnly, eventID, verified)
	}
}

func docdbAction(action string) string {
	if strings.Contains(action, ":") {
		return action
	}
	switch action {
	case "CreateDBCluster":
		return catalog.ActionDocDBCreateDBCluster
	case "DescribeDBClusters":
		return catalog.ActionDocDBDescribeDBClusters
	case "DeleteDBCluster":
		return catalog.ActionDocDBDeleteDBCluster
	default:
		return action
	}
}

func isDocDBControlPlaneAction(action string) bool {
	a := docdbAction(action)
	switch a {
	case catalog.ActionDocDBCreateDBCluster,
		catalog.ActionDocDBDescribeDBClusters,
		catalog.ActionDocDBDeleteDBCluster,
		"CreateDBCluster", "DescribeDBClusters", "DeleteDBCluster":
		return true
	default:
		return false
	}
}

func (s *Server) docdbRegion(verified *authn.Verified) string {
	if verified.Region != "" {
		return verified.Region
	}
	return store.DefaultDocDBRegion
}

func (s *Server) docdbCreate(
	w http.ResponseWriter, r *http.Request, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params url.Values,
) {
	id := strings.TrimSpace(params.Get("DBClusterIdentifier"))
	engine := strings.TrimSpace(params.Get("Engine"))
	version := strings.TrimSpace(params.Get("EngineVersion"))
	master := strings.TrimSpace(params.Get("MasterUsername"))
	port := 0
	if p := strings.TrimSpace(params.Get("Port")); p != "" {
		if v, err := strconv.Atoi(p); err == nil {
			port = v
		}
	}
	if !s.authorize(verified, catalog.ActionDocDBCreateDBCluster, "*") {
		s.writeDocDBError(w, r, requestID, http.StatusForbidden, "AccessDenied",
			"User is not authorized to perform rds:CreateDBCluster.", readOnly, eventID, verified)
		return
	}
	c, err := s.store.CreateDocDBCluster(verified.AccountID, s.docdbRegion(verified), id, engine, version, master, port)
	if errors.Is(err, store.ErrDocDBClusterExists) {
		s.writeDocDBError(w, r, requestID, http.StatusBadRequest, "DBClusterAlreadyExistsFault",
			"DB cluster already exists.", readOnly, eventID, verified)
		return
	}
	if errors.Is(err, store.ErrDocDBBadRequest) {
		s.writeDocDBError(w, r, requestID, http.StatusBadRequest, "InvalidParameterValue",
			err.Error(), readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeDocDBError(w, r, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to create DB cluster.", readOnly, eventID, verified)
		return
	}
	user := c.MasterUsername
	if user == "" {
		user = "docdbadmin"
	}
	pass := strings.TrimSpace(params.Get("MasterUserPassword"))
	if pass == "" {
		pass = "noctaxris-docdb-lab"
	}
	_, _ = s.store.EnsureDataPlaneMasterSecret(
		verified.AccountID, s.docdbRegion(verified),
		store.DataPlaneSecretDocDB, c.DBClusterIdentifier, user, pass,
	)
	_ = tryStartNestedDataEngine(s, verified.AccountID, "docdb", c.DBClusterIdentifier, map[string]string{
		"MONGO_INITDB_ROOT_USERNAME": user,
		"MONGO_INITDB_ROOT_PASSWORD": pass,
	})
	if updated, err := s.store.DescribeDocDBCluster(verified.AccountID, c.DBClusterIdentifier); err == nil {
		c = updated
	}
	payload, _ := docdbsvc.CreateDBClusterXML(c, requestID)
	s.writeDocDBOK(w, requestID, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, docdbEventSource, "CreateDBCluster", readOnly)
}

func (s *Server) docdbDescribe(
	w http.ResponseWriter, r *http.Request, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params url.Values,
) {
	id := strings.TrimSpace(params.Get("DBClusterIdentifier"))
	if !s.authorize(verified, catalog.ActionDocDBDescribeDBClusters, "*") {
		s.writeDocDBError(w, r, requestID, http.StatusForbidden, "AccessDenied",
			"User is not authorized to perform rds:DescribeDBClusters.", readOnly, eventID, verified)
		return
	}
	clusters, err := s.store.DescribeDocDBClusters(verified.AccountID, id)
	if err != nil {
		s.writeDocDBError(w, r, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to describe DB clusters.", readOnly, eventID, verified)
		return
	}
	if id != "" && len(clusters) == 0 {
		s.writeDocDBError(w, r, requestID, http.StatusBadRequest, "DBClusterNotFoundFault",
			"DB cluster not found.", readOnly, eventID, verified)
		return
	}
	payload, _ := docdbsvc.DescribeDBClustersXML(clusters, requestID)
	s.writeDocDBOK(w, requestID, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, docdbEventSource, "DescribeDBClusters", readOnly)
}

func (s *Server) docdbDelete(
	w http.ResponseWriter, r *http.Request, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params url.Values,
) {
	id := strings.TrimSpace(params.Get("DBClusterIdentifier"))
	if !s.authorize(verified, catalog.ActionDocDBDeleteDBCluster, "*") {
		s.writeDocDBError(w, r, requestID, http.StatusForbidden, "AccessDenied",
			"User is not authorized to perform rds:DeleteDBCluster.", readOnly, eventID, verified)
		return
	}
	existing, err := s.store.DescribeDocDBCluster(verified.AccountID, id)
	if errors.Is(err, store.ErrDocDBClusterNotFound) {
		s.writeDocDBError(w, r, requestID, http.StatusBadRequest, "DBClusterNotFoundFault",
			"DB cluster not found.", readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeDocDBError(w, r, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to delete DB cluster.", readOnly, eventID, verified)
		return
	}
	containerID, err := s.store.DeleteDocDBCluster(verified.AccountID, id)
	if err != nil {
		s.writeDocDBError(w, r, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to delete DB cluster.", readOnly, eventID, verified)
		return
	}
	_ = tryStopNestedDataEngine(s, containerID)
	payload, _ := docdbsvc.DeleteDBClusterXML(existing, requestID)
	s.writeDocDBOK(w, requestID, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, docdbEventSource, "DeleteDBCluster", readOnly)
}

func (s *Server) writeDocDBOK(w http.ResponseWriter, requestID string, payload []byte) {
	w.Header().Set(requestIDHeader, requestID)
	w.Header().Set("Content-Type", "text/xml")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(payload)
}

func (s *Server) writeDocDBError(
	w http.ResponseWriter, r *http.Request, requestID string,
	status int, code, message string, readOnly bool, eventID string, verified *authn.Verified,
) {
	payload, _ := docdbsvc.ErrorXML(code, message, requestID)
	w.Header().Set(requestIDHeader, requestID)
	w.Header().Set("Content-Type", "text/xml")
	w.WriteHeader(status)
	_, _ = w.Write(payload)
	_ = r
	_ = readOnly
	_ = eventID
	_ = verified
}
