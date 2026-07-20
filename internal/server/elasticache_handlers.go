package server

import (
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/Kyaxris-Labs/Noctaxris/internal/catalog"
	"github.com/Kyaxris-Labs/Noctaxris/internal/kernel/authn"
	ecsvc "github.com/Kyaxris-Labs/Noctaxris/internal/services/elasticache"
	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

const elasticacheEventSource = "elasticache.amazonaws.com"

func (s *Server) handleElastiCache(
	w http.ResponseWriter,
	r *http.Request,
	body []byte,
	requestID, eventID, action string,
	verified *authn.Verified,
	readOnly bool,
) {
	params := formParams(r, body)
	action = elasticacheAction(action)

	switch action {
	case catalog.ActionElastiCacheCreateCacheCluster:
		s.elasticacheCreate(w, r, requestID, eventID, verified, readOnly, params)
	case catalog.ActionElastiCacheDescribeCacheClusters:
		s.elasticacheDescribe(w, r, requestID, eventID, verified, readOnly, params)
	case catalog.ActionElastiCacheDeleteCacheCluster:
		s.elasticacheDelete(w, r, requestID, eventID, verified, readOnly, params)
	default:
		s.writeElastiCacheError(w, r, requestID, http.StatusBadRequest, "InvalidAction",
			"This ElastiCache action is not implemented.", readOnly, eventID, verified)
	}
}

func elasticacheAction(action string) string {
	if strings.Contains(action, ":") {
		return action
	}
	switch action {
	case "CreateCacheCluster":
		return catalog.ActionElastiCacheCreateCacheCluster
	case "DescribeCacheClusters":
		return catalog.ActionElastiCacheDescribeCacheClusters
	case "DeleteCacheCluster":
		return catalog.ActionElastiCacheDeleteCacheCluster
	default:
		return action
	}
}

func (s *Server) elasticacheRegion(verified *authn.Verified) string {
	if verified.Region != "" {
		return verified.Region
	}
	return store.DefaultElastiCacheRegion
}

func (s *Server) elasticacheCreate(
	w http.ResponseWriter, r *http.Request, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params url.Values,
) {
	id := strings.TrimSpace(params.Get("CacheClusterId"))
	engine := strings.TrimSpace(params.Get("Engine"))
	version := strings.TrimSpace(params.Get("EngineVersion"))
	nodeType := strings.TrimSpace(params.Get("CacheNodeType"))
	numNodes := 1
	if n := strings.TrimSpace(params.Get("NumCacheNodes")); n != "" {
		if v, err := strconv.Atoi(n); err == nil {
			numNodes = v
		}
	}
	if !s.authorize(verified, catalog.ActionElastiCacheCreateCacheCluster, "*") {
		s.writeElastiCacheError(w, r, requestID, http.StatusForbidden, "AccessDenied",
			"User is not authorized to perform elasticache:CreateCacheCluster.", readOnly, eventID, verified)
		return
	}
	c, err := s.store.CreateElastiCacheCluster(verified.AccountID, s.elasticacheRegion(verified), id, engine, version, nodeType, numNodes)
	if errors.Is(err, store.ErrElastiCacheClusterExists) {
		s.writeElastiCacheError(w, r, requestID, http.StatusBadRequest, "CacheClusterAlreadyExists",
			"Cache cluster already exists.", readOnly, eventID, verified)
		return
	}
	if errors.Is(err, store.ErrElastiCacheBadRequest) {
		s.writeElastiCacheError(w, r, requestID, http.StatusBadRequest, "InvalidParameterValue",
			err.Error(), readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeElastiCacheError(w, r, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to create cache cluster.", readOnly, eventID, verified)
		return
	}
	_, _ = s.store.EnsureDataPlaneMasterSecret(
		verified.AccountID, s.elasticacheRegion(verified),
		store.DataPlaneSecretElastiCache, c.CacheClusterID, "default", "noctaxris-cache-lab",
	)
	// Nested Valkey/Redis lab image has no AUTH by default. Secret is for control-plane labs.
	_ = tryStartNestedDataEngine(s, verified.AccountID, "elasticache", c.CacheClusterID, nil)
	if updated, err := s.store.DescribeElastiCacheCluster(verified.AccountID, c.CacheClusterID); err == nil {
		c = updated
	}
	payload, _ := ecsvc.CreateCacheClusterXML(c, requestID)
	s.writeElastiCacheOK(w, requestID, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, elasticacheEventSource, "CreateCacheCluster", readOnly)
}

func (s *Server) elasticacheDescribe(
	w http.ResponseWriter, r *http.Request, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params url.Values,
) {
	id := strings.TrimSpace(params.Get("CacheClusterId"))
	if !s.authorize(verified, catalog.ActionElastiCacheDescribeCacheClusters, "*") {
		s.writeElastiCacheError(w, r, requestID, http.StatusForbidden, "AccessDenied",
			"User is not authorized to perform elasticache:DescribeCacheClusters.", readOnly, eventID, verified)
		return
	}
	clusters, err := s.store.DescribeElastiCacheClusters(verified.AccountID, id)
	if err != nil {
		s.writeElastiCacheError(w, r, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to describe cache clusters.", readOnly, eventID, verified)
		return
	}
	if id != "" && len(clusters) == 0 {
		s.writeElastiCacheError(w, r, requestID, http.StatusBadRequest, "CacheClusterNotFound",
			"Cache cluster not found.", readOnly, eventID, verified)
		return
	}
	payload, _ := ecsvc.DescribeCacheClustersXML(clusters, requestID)
	s.writeElastiCacheOK(w, requestID, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, elasticacheEventSource, "DescribeCacheClusters", readOnly)
}

func (s *Server) elasticacheDelete(
	w http.ResponseWriter, r *http.Request, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params url.Values,
) {
	id := strings.TrimSpace(params.Get("CacheClusterId"))
	if !s.authorize(verified, catalog.ActionElastiCacheDeleteCacheCluster, "*") {
		s.writeElastiCacheError(w, r, requestID, http.StatusForbidden, "AccessDenied",
			"User is not authorized to perform elasticache:DeleteCacheCluster.", readOnly, eventID, verified)
		return
	}
	existing, err := s.store.DescribeElastiCacheCluster(verified.AccountID, id)
	if errors.Is(err, store.ErrElastiCacheClusterNotFound) {
		s.writeElastiCacheError(w, r, requestID, http.StatusBadRequest, "CacheClusterNotFound",
			"Cache cluster not found.", readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeElastiCacheError(w, r, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to delete cache cluster.", readOnly, eventID, verified)
		return
	}
	containerID, err := s.store.DeleteElastiCacheCluster(verified.AccountID, id)
	if err != nil {
		s.writeElastiCacheError(w, r, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to delete cache cluster.", readOnly, eventID, verified)
		return
	}
	_ = tryStopNestedDataEngine(s, containerID)
	payload, _ := ecsvc.DeleteCacheClusterXML(existing, requestID)
	s.writeElastiCacheOK(w, requestID, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, elasticacheEventSource, "DeleteCacheCluster", readOnly)
}

func (s *Server) writeElastiCacheOK(w http.ResponseWriter, requestID string, payload []byte) {
	w.Header().Set(requestIDHeader, requestID)
	w.Header().Set("Content-Type", "text/xml")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(payload)
}

func (s *Server) writeElastiCacheError(
	w http.ResponseWriter, r *http.Request, requestID string,
	status int, code, message string, readOnly bool, eventID string, verified *authn.Verified,
) {
	payload, _ := ecsvc.ErrorXML(code, message, requestID)
	w.Header().Set(requestIDHeader, requestID)
	w.Header().Set("Content-Type", "text/xml")
	w.WriteHeader(status)
	_, _ = w.Write(payload)
	_ = r
	_ = readOnly
	_ = eventID
	_ = verified
}
