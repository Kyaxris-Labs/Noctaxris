package server

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/Kyaxris-Labs/Noctaxris/internal/catalog"
	"github.com/Kyaxris-Labs/Noctaxris/internal/kernel/authn"
	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

// Lab CloudFront fake-edge path: /cloudfront/{distributionId}/{objectKey...}
// SigV4 required (no anonymous). First origin only. Origins resolve in-store only
// (S3 GetObject or internal HTTP API invoke). Never dials arbitrary hosts.

func isCloudFrontEdgePath(path string) bool {
	path = strings.TrimSuffix(path, "/")
	parts := strings.Split(strings.TrimPrefix(path, "/"), "/")
	return len(parts) >= 2 && parts[0] == "cloudfront" && parts[1] != ""
}

func parseCloudFrontEdgePath(path string) (distributionID, objectKey string, ok bool) {
	path = strings.TrimSuffix(path, "/")
	parts := strings.Split(strings.TrimPrefix(path, "/"), "/")
	if len(parts) < 2 || parts[0] != "cloudfront" || parts[1] == "" {
		return "", "", false
	}
	distributionID = parts[1]
	if len(parts) == 2 {
		return distributionID, "", true
	}
	return distributionID, strings.Join(parts[2:], "/"), true
}

func cloudFrontWAFCandidateARNs(distARN string) []string {
	distARN = strings.TrimSpace(distARN)
	if distARN == "" {
		return nil
	}
	return []string{distARN}
}

// CloudFrontEdgeHandler exposes the lab fake-edge for direct unit tests (self-verifies SigV4).
func (s *Server) CloudFrontEdgeHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestID := newRequestID()
		eventID := newRequestID()
		readOnly := r.Method == http.MethodGet || r.Method == http.MethodHead
		body, err := readBody(r, bodyLimitForRequest(r))
		if err != nil {
			http.Error(w, "Unable to read request body.", http.StatusBadRequest)
			return
		}
		s.handleCloudFrontEdge(w, r, body, requestID, eventID, readOnly)
	})
}

// handleCloudFrontEdge verifies SigV4 then serves the fake-edge path.
func (s *Server) handleCloudFrontEdge(w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string, readOnly bool) {
	verified, err := authn.Verify(r, body, s.now(), sigv4Skew, s.lookupKey)
	if err != nil {
		code := authn.Code(err)
		if code == "" {
			code = authn.CodeMissingAuthenticationToken
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("x-amzn-ErrorType", code)
		w.WriteHeader(http.StatusForbidden)
		_ = json.NewEncoder(w).Encode(map[string]string{"__type": code, "message": defaultAuthnMessage(code)})
		return
	}
	verified.SourceIP = clientIP(r)
	s.handleCloudFrontEdgeAfterAuth(w, r, body, requestID, eventID, verified, readOnly)
}

// handleCloudFrontEdgeAfterAuth serves GET /cloudfront/{distributionId}/{objectKey...}
// after ServeHTTP has already verified SigV4.
func (s *Server) handleCloudFrontEdgeAfterAuth(
	w http.ResponseWriter, r *http.Request, body []byte,
	requestID, eventID string, verified *authn.Verified, readOnly bool,
) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	distributionID, objectKey, ok := parseCloudFrontEdgePath(r.URL.Path)
	if !ok {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	if !strings.EqualFold(verified.Service, "cloudfront") {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}

	d, err := s.store.GetCloudFrontDistribution(verified.AccountID, distributionID)
	if errors.Is(err, store.ErrCloudFrontNotFound) {
		http.Error(w, "distribution not found", http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	if !s.authorize(verified, catalog.ActionCloudFrontGetDistribution, d.ARN) {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}
	if !d.Enabled {
		http.Error(w, "distribution disabled", http.StatusForbidden)
		return
	}
	if !s.enforceAssociatedWAF(w, r, verified.AccountID, cloudFrontWAFCandidateARNs(d.ARN)) {
		return
	}

	var origins []store.CloudFrontOrigin
	if err := json.Unmarshal([]byte(d.OriginsJSON), &origins); err != nil || len(origins) == 0 {
		http.Error(w, "invalid origin configuration", http.StatusBadGateway)
		return
	}
	origin := origins[0]

	switch origin.OriginType {
	case "s3":
		s.cloudFrontEdgeFetchS3(w, r, verified, requestID, eventID, readOnly, origin.DomainName, objectKey)
	case "apigateway":
		s.cloudFrontEdgeFetchAPIGateway(w, r, body, requestID, eventID, readOnly, origin.DomainName, objectKey)
	default:
		http.Error(w, "unsupported origin type", http.StatusBadGateway)
	}
}

func (s *Server) cloudFrontEdgeFetchS3(
	w http.ResponseWriter, r *http.Request, verified *authn.Verified,
	requestID, eventID string, readOnly bool, bucket, key string,
) {
	if strings.TrimSpace(key) == "" {
		http.Error(w, "object key required", http.StatusNotFound)
		return
	}
	meta, data, err := s.store.GetObject(verified.AccountID, bucket, key)
	if errors.Is(err, store.ErrNoSuchKey) || errors.Is(err, store.ErrInvalidObjectKey) {
		http.Error(w, "NoSuchKey", http.StatusNotFound)
		return
	}
	if errors.Is(err, store.ErrNoSuchBucket) {
		http.Error(w, "origin bucket not found", http.StatusBadGateway)
		return
	}
	if err != nil {
		http.Error(w, "origin fetch failed", http.StatusBadGateway)
		return
	}
	if meta.ContentType != "" {
		w.Header().Set("Content-Type", meta.ContentType)
	} else {
		w.Header().Set("Content-Type", "application/octet-stream")
	}
	if meta.ETag != "" {
		w.Header().Set("ETag", `"`+meta.ETag+`"`)
	}
	w.WriteHeader(http.StatusOK)
	if r.Method != http.MethodHead {
		_, _ = w.Write(data)
	}
	s.writeSuccessAudit(r, requestID, eventID, verified, cloudfrontEventSource, "EdgeGetObject", readOnly)
}

func (s *Server) cloudFrontEdgeFetchAPIGateway(
	w http.ResponseWriter, r *http.Request, body []byte,
	requestID, eventID string, readOnly bool, apiID, routePath string,
) {
	apiID = strings.TrimSpace(apiID)
	if apiID == "" {
		http.Error(w, "invalid apigateway origin", http.StatusBadGateway)
		return
	}
	if routePath == "" {
		routePath = "/"
	} else if !strings.HasPrefix(routePath, "/") {
		routePath = "/" + routePath
	}
	// Internal invoke only: rewrite to lab HTTP API path. Never dial origin DomainName.
	cloned := r.Clone(r.Context())
	u := *r.URL
	u.Path = "/http-api/" + apiID + "/$default" + routePath
	u.RawPath = ""
	cloned.URL = &u
	s.handleHTTPAPIInvoke(w, cloned, body, requestID, eventID, readOnly)
}
