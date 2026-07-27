package server

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/Kyaxris-Labs/Noctaxris/internal/catalog"
	"github.com/Kyaxris-Labs/Noctaxris/internal/compute"
	"github.com/Kyaxris-Labs/Noctaxris/internal/kernel/authn"
	msksvc "github.com/Kyaxris-Labs/Noctaxris/internal/services/msk"
	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

const (
	mskJSONContentType = "application/x-amz-json-1.1"
	mskEventSource     = "kafka.amazonaws.com"
)

func (s *Server) handleMSK(
	w http.ResponseWriter,
	r *http.Request,
	body []byte,
	requestID, eventID, action string,
	verified *authn.Verified,
	readOnly bool,
) {
	if action == "" {
		action = resolveMSKREST(r)
	}
	params := jsonBodyMap(body)
	// REST path may encode ClusterArn; merge into params.
	if arn := mskClusterARNFromPath(r.URL.Path); arn != "" {
		if _, ok := params["ClusterArn"]; !ok {
			params["ClusterArn"] = arn
		}
	}
	action = mskAction(action)

	switch action {
	case catalog.ActionMSKCreateCluster:
		s.mskCreate(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionMSKDescribeCluster:
		s.mskDescribe(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionMSKListClusters:
		s.mskList(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionMSKDeleteCluster:
		s.mskDelete(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionMSKGetBootstrapBrokers:
		s.mskBootstrap(w, r, body, requestID, eventID, verified, readOnly, params)
	default:
		s.writeMSKError(w, r, body, requestID, http.StatusNotImplemented, "InternalFailure",
			"This MSK action is not implemented.", readOnly, eventID, verified)
	}
}

func mskAction(action string) string {
	if strings.Contains(action, ":") {
		return action
	}
	switch action {
	case "CreateCluster":
		return catalog.ActionMSKCreateCluster
	case "DescribeCluster":
		return catalog.ActionMSKDescribeCluster
	case "ListClusters":
		return catalog.ActionMSKListClusters
	case "DeleteCluster":
		return catalog.ActionMSKDeleteCluster
	case "GetBootstrapBrokers":
		return catalog.ActionMSKGetBootstrapBrokers
	default:
		return action
	}
}

func isMSKRESTPath(path string) bool {
	p := strings.ToLower(path)
	return strings.HasPrefix(p, "/v1/clusters") || strings.HasPrefix(p, "/api/v2/clusters")
}

func resolveMSKREST(r *http.Request) string {
	p := strings.TrimSuffix(r.URL.Path, "/")
	lower := strings.ToLower(p)
	method := strings.ToUpper(r.Method)
	switch {
	case (lower == "/v1/clusters" || lower == "/api/v2/clusters") && method == http.MethodPost:
		return catalog.ActionMSKCreateCluster
	case (lower == "/v1/clusters" || lower == "/api/v2/clusters") && method == http.MethodGet:
		return catalog.ActionMSKListClusters
	case strings.HasSuffix(lower, "/bootstrap-brokers") && method == http.MethodGet:
		return catalog.ActionMSKGetBootstrapBrokers
	case (strings.HasPrefix(lower, "/v1/clusters/") || strings.HasPrefix(lower, "/api/v2/clusters/")) && method == http.MethodGet:
		return catalog.ActionMSKDescribeCluster
	case (strings.HasPrefix(lower, "/v1/clusters/") || strings.HasPrefix(lower, "/api/v2/clusters/")) && method == http.MethodDelete:
		return catalog.ActionMSKDeleteCluster
	default:
		return ""
	}
}

func mskClusterARNFromPath(path string) string {
	p := strings.TrimSuffix(path, "/")
	lower := strings.ToLower(p)
	var rest string
	switch {
	case strings.HasPrefix(lower, "/v1/clusters/"):
		rest = p[len("/v1/clusters/"):]
	case strings.HasPrefix(lower, "/api/v2/clusters/"):
		rest = p[len("/api/v2/clusters/"):]
	default:
		return ""
	}
	if i := strings.Index(strings.ToLower(rest), "/bootstrap-brokers"); i >= 0 {
		rest = rest[:i]
	}
	decoded, err := url.PathUnescape(rest)
	if err != nil {
		return strings.TrimSpace(rest)
	}
	return strings.TrimSpace(decoded)
}

func (s *Server) mskRegion(verified *authn.Verified) string {
	if verified.Region != "" {
		return verified.Region
	}
	return store.DefaultMSKRegion
}

func (s *Server) mskCreate(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	name, _ := params["ClusterName"].(string)
	if name == "" {
		name, _ = params["clusterName"].(string)
	}
	version, _ := params["KafkaVersion"].(string)
	if version == "" {
		version, _ = params["kafkaVersion"].(string)
	}
	nodes := 1
	switch v := params["NumberOfBrokerNodes"].(type) {
	case float64:
		nodes = int(v)
	case int:
		nodes = v
	case json.Number:
		if n, err := v.Int64(); err == nil {
			nodes = int(n)
		}
	case string:
		if n, err := strconv.Atoi(strings.TrimSpace(v)); err == nil {
			nodes = n
		}
	default:
		if v, ok := params["numberOfBrokerNodes"].(float64); ok {
			nodes = int(v)
		}
	}
	if !s.authorize(verified, catalog.ActionMSKCreateCluster, "*") {
		s.writeMSKError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform kafka:CreateCluster.", readOnly, eventID, verified)
		return
	}
	c, err := s.store.CreateMSKCluster(verified.AccountID, s.mskRegion(verified), name, version, nodes)
	if errors.Is(err, store.ErrMSKClusterExists) {
		s.writeMSKError(w, r, body, requestID, http.StatusConflict, "ConflictException",
			"Cluster already exists.", readOnly, eventID, verified)
		return
	}
	if errors.Is(err, store.ErrMSKBadRequest) {
		s.writeMSKError(w, r, body, requestID, http.StatusBadRequest, "BadRequestException",
			err.Error(), readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeMSKError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to create cluster.", readOnly, eventID, verified)
		return
	}
	_ = tryStartNestedMSK(s, verified.AccountID, c.ClusterName)
	if updated, err := s.store.DescribeMSKClusterByName(verified.AccountID, c.ClusterName); err == nil {
		c = updated
	}
	payload, _ := msksvc.CreateClusterJSON(c)
	s.writeMSKOK(w, requestID, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, mskEventSource, "CreateCluster", readOnly)
}

func (s *Server) mskDescribe(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	arn, _ := params["ClusterArn"].(string)
	if arn == "" {
		arn, _ = params["clusterArn"].(string)
	}
	if !s.authorize(verified, catalog.ActionMSKDescribeCluster, "*") {
		s.writeMSKError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform kafka:DescribeCluster.", readOnly, eventID, verified)
		return
	}
	c, err := s.store.DescribeMSKClusterByARN(verified.AccountID, arn)
	if errors.Is(err, store.ErrMSKClusterNotFound) {
		s.writeMSKError(w, r, body, requestID, http.StatusBadRequest, "NotFoundException",
			"Cluster not found.", readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeMSKError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to describe cluster.", readOnly, eventID, verified)
		return
	}
	payload, _ := msksvc.DescribeClusterJSON(c)
	s.writeMSKOK(w, requestID, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, mskEventSource, "DescribeCluster", readOnly)
}

func (s *Server) mskList(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	_ = params
	if !s.authorize(verified, catalog.ActionMSKListClusters, "*") {
		s.writeMSKError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform kafka:ListClusters.", readOnly, eventID, verified)
		return
	}
	clusters, err := s.store.ListMSKClusters(verified.AccountID)
	if err != nil {
		s.writeMSKError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to list clusters.", readOnly, eventID, verified)
		return
	}
	payload, _ := msksvc.ListClustersJSON(clusters)
	s.writeMSKOK(w, requestID, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, mskEventSource, "ListClusters", readOnly)
}

func (s *Server) mskDelete(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	arn, _ := params["ClusterArn"].(string)
	if arn == "" {
		arn, _ = params["clusterArn"].(string)
	}
	if !s.authorize(verified, catalog.ActionMSKDeleteCluster, "*") {
		s.writeMSKError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform kafka:DeleteCluster.", readOnly, eventID, verified)
		return
	}
	containerID, err := s.store.DeleteMSKCluster(verified.AccountID, arn)
	if errors.Is(err, store.ErrMSKClusterNotFound) {
		s.writeMSKError(w, r, body, requestID, http.StatusBadRequest, "NotFoundException",
			"Cluster not found.", readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeMSKError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to delete cluster.", readOnly, eventID, verified)
		return
	}
	_ = tryStopNestedDataEngine(s, containerID)
	payload, _ := msksvc.DeleteClusterJSON(arn)
	s.writeMSKOK(w, requestID, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, mskEventSource, "DeleteCluster", readOnly)
}

func (s *Server) mskBootstrap(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	arn, _ := params["ClusterArn"].(string)
	if arn == "" {
		arn, _ = params["clusterArn"].(string)
	}
	if !s.authorize(verified, catalog.ActionMSKGetBootstrapBrokers, "*") {
		s.writeMSKError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform kafka:GetBootstrapBrokers.", readOnly, eventID, verified)
		return
	}
	c, err := s.store.DescribeMSKClusterByARN(verified.AccountID, arn)
	if errors.Is(err, store.ErrMSKClusterNotFound) {
		s.writeMSKError(w, r, body, requestID, http.StatusBadRequest, "NotFoundException",
			"Cluster not found.", readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeMSKError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to get bootstrap brokers.", readOnly, eventID, verified)
		return
	}
	payload, _ := msksvc.GetBootstrapBrokersJSON(c)
	s.writeMSKOK(w, requestID, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, mskEventSource, "GetBootstrapBrokers", readOnly)
}

func (s *Server) writeMSKOK(w http.ResponseWriter, requestID string, payload []byte) {
	w.Header().Set("Content-Type", mskJSONContentType)
	w.Header().Set("x-amzn-RequestId", requestID)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(payload)
}

func (s *Server) writeMSKError(
	w http.ResponseWriter, r *http.Request, body []byte, requestID string,
	status int, code, message string, readOnly bool, eventID string, verified *authn.Verified,
) {
	w.Header().Set("Content-Type", mskJSONContentType)
	w.Header().Set("x-amzn-RequestId", requestID)
	w.Header().Set("x-amzn-ErrorType", code)
	w.WriteHeader(status)
	_, _ = w.Write([]byte(`{"__type":"` + code + `","message":"` + message + `"}`))
	_ = body
	_ = r
	_ = readOnly
	_ = eventID
	_ = verified
}

// tryStartNestedMSK starts nested Redpanda with advertise address for the cluster name.
func tryStartNestedMSK(s *Server, accountID, clusterName string) error {
	name := strings.ToLower(strings.TrimSpace(clusterName))
	containerName := "noctaxris-msk-" + name
	return tryStartNestedDataEngineWithOpts(s, accountID, "msk", name, nil, compute.DefaultDataPlaneImage(compute.DataKindMSK), compute.RedpandaStartCmd(containerName), containerName)
}
