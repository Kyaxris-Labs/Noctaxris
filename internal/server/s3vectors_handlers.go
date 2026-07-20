package server

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/Kyaxris-Labs/Noctaxris/internal/catalog"
	"github.com/Kyaxris-Labs/Noctaxris/internal/kernel/authn"
	s3vsvc "github.com/Kyaxris-Labs/Noctaxris/internal/services/s3vectors"
	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

const (
	s3vectorsJSONContentType = "application/x-amz-json-1.1"
	s3vectorsEventSource     = "s3vectors.amazonaws.com"
)

func (s *Server) handleS3Vectors(
	w http.ResponseWriter,
	r *http.Request,
	body []byte,
	requestID, eventID, action string,
	verified *authn.Verified,
	readOnly bool,
) {
	params := jsonBodyMap(body)
	action = s3vectorsAction(action)

	switch action {
	case catalog.ActionS3VectorsCreateVectorBucket:
		s.s3vCreateBucket(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionS3VectorsListVectorBuckets:
		s.s3vListBuckets(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionS3VectorsDeleteVectorBucket:
		s.s3vDeleteBucket(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionS3VectorsCreateIndex:
		s.s3vCreateIndex(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionS3VectorsListIndexes:
		s.s3vListIndexes(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionS3VectorsDeleteIndex:
		s.s3vDeleteIndex(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionS3VectorsPutVectors:
		s.s3vPutVectors(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionS3VectorsQueryVectors:
		s.s3vQueryVectors(w, r, body, requestID, eventID, verified, readOnly, params)
	default:
		s.writeS3VectorsError(w, r, body, requestID, http.StatusNotImplemented, "InvalidAction",
			"This S3 Vectors action is not implemented.", readOnly, eventID, verified)
	}
}

func s3vectorsAction(action string) string {
	if strings.Contains(action, ":") {
		return action
	}
	switch action {
	case "CreateVectorBucket":
		return catalog.ActionS3VectorsCreateVectorBucket
	case "ListVectorBuckets":
		return catalog.ActionS3VectorsListVectorBuckets
	case "DeleteVectorBucket":
		return catalog.ActionS3VectorsDeleteVectorBucket
	case "CreateIndex":
		return catalog.ActionS3VectorsCreateIndex
	case "ListIndexes":
		return catalog.ActionS3VectorsListIndexes
	case "DeleteIndex":
		return catalog.ActionS3VectorsDeleteIndex
	case "PutVectors":
		return catalog.ActionS3VectorsPutVectors
	case "QueryVectors":
		return catalog.ActionS3VectorsQueryVectors
	default:
		return action
	}
}

func (s *Server) s3vCreateBucket(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	name, _ := params["vectorBucketName"].(string)
	if name == "" {
		name, _ = params["VectorBucketName"].(string)
	}
	if !s.authorize(verified, catalog.ActionS3VectorsCreateVectorBucket, "*") {
		s.writeS3VectorsError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform s3vectors:CreateVectorBucket.", readOnly, eventID, verified)
		return
	}
	b, err := s.store.CreateS3VectorBucket(verified.AccountID, name)
	if errors.Is(err, store.ErrS3VectorsExists) {
		s.writeS3VectorsError(w, r, body, requestID, http.StatusConflict, "ConflictException",
			"Vector bucket already exists.", readOnly, eventID, verified)
		return
	}
	if errors.Is(err, store.ErrS3VectorsBadRequest) {
		s.writeS3VectorsError(w, r, body, requestID, http.StatusBadRequest, "ValidationException",
			err.Error(), readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeS3VectorsError(w, r, body, requestID, http.StatusInternalServerError, "InternalServerException",
			"Unable to create vector bucket.", readOnly, eventID, verified)
		return
	}
	payload, _ := s3vsvc.CreateVectorBucketJSON(b)
	s.writeS3VectorsOK(w, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, s3vectorsEventSource, "CreateVectorBucket", readOnly)
}

func (s *Server) s3vListBuckets(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	_ = params
	if !s.authorize(verified, catalog.ActionS3VectorsListVectorBuckets, "*") {
		s.writeS3VectorsError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform s3vectors:ListVectorBuckets.", readOnly, eventID, verified)
		return
	}
	buckets, err := s.store.ListS3VectorBuckets(verified.AccountID)
	if err != nil {
		s.writeS3VectorsError(w, r, body, requestID, http.StatusInternalServerError, "InternalServerException",
			"Unable to list vector buckets.", readOnly, eventID, verified)
		return
	}
	payload, _ := s3vsvc.ListVectorBucketsJSON(buckets)
	s.writeS3VectorsOK(w, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, s3vectorsEventSource, "ListVectorBuckets", readOnly)
}

func (s *Server) s3vDeleteBucket(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	name, _ := params["vectorBucketName"].(string)
	if name == "" {
		name, _ = params["VectorBucketName"].(string)
	}
	if !s.authorize(verified, catalog.ActionS3VectorsDeleteVectorBucket, "*") {
		s.writeS3VectorsError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform s3vectors:DeleteVectorBucket.", readOnly, eventID, verified)
		return
	}
	err := s.store.DeleteS3VectorBucket(verified.AccountID, name)
	if errors.Is(err, store.ErrS3VectorsNotFound) {
		s.writeS3VectorsError(w, r, body, requestID, http.StatusNotFound, "NotFoundException",
			"Vector bucket not found.", readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeS3VectorsError(w, r, body, requestID, http.StatusInternalServerError, "InternalServerException",
			"Unable to delete vector bucket.", readOnly, eventID, verified)
		return
	}
	payload, _ := s3vsvc.DeleteOKJSON()
	s.writeS3VectorsOK(w, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, s3vectorsEventSource, "DeleteVectorBucket", readOnly)
}

func (s *Server) s3vCreateIndex(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	bucket, _ := params["vectorBucketName"].(string)
	if bucket == "" {
		bucket, _ = params["VectorBucketName"].(string)
	}
	indexName, _ := params["indexName"].(string)
	if indexName == "" {
		indexName, _ = params["IndexName"].(string)
	}
	metric, _ := params["distanceMetric"].(string)
	if metric == "" {
		metric, _ = params["DistanceMetric"].(string)
	}
	dim := 0
	switch d := params["dimension"].(type) {
	case float64:
		dim = int(d)
	default:
		if d2, ok := params["Dimension"].(float64); ok {
			dim = int(d2)
		}
	}
	if !s.authorize(verified, catalog.ActionS3VectorsCreateIndex, "*") {
		s.writeS3VectorsError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform s3vectors:CreateIndex.", readOnly, eventID, verified)
		return
	}
	idx, err := s.store.CreateS3VectorIndex(verified.AccountID, bucket, indexName, metric, dim)
	if errors.Is(err, store.ErrS3VectorsNotFound) {
		s.writeS3VectorsError(w, r, body, requestID, http.StatusNotFound, "NotFoundException",
			"Vector bucket not found.", readOnly, eventID, verified)
		return
	}
	if errors.Is(err, store.ErrS3VectorsExists) {
		s.writeS3VectorsError(w, r, body, requestID, http.StatusConflict, "ConflictException",
			"Index already exists.", readOnly, eventID, verified)
		return
	}
	if errors.Is(err, store.ErrS3VectorsBadRequest) {
		s.writeS3VectorsError(w, r, body, requestID, http.StatusBadRequest, "ValidationException",
			err.Error(), readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeS3VectorsError(w, r, body, requestID, http.StatusInternalServerError, "InternalServerException",
			"Unable to create index.", readOnly, eventID, verified)
		return
	}
	payload, _ := s3vsvc.CreateIndexJSON(idx)
	s.writeS3VectorsOK(w, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, s3vectorsEventSource, "CreateIndex", readOnly)
}

func (s *Server) s3vListIndexes(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	bucket, _ := params["vectorBucketName"].(string)
	if bucket == "" {
		bucket, _ = params["VectorBucketName"].(string)
	}
	if !s.authorize(verified, catalog.ActionS3VectorsListIndexes, "*") {
		s.writeS3VectorsError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform s3vectors:ListIndexes.", readOnly, eventID, verified)
		return
	}
	indexes, err := s.store.ListS3VectorIndexes(verified.AccountID, bucket)
	if err != nil {
		s.writeS3VectorsError(w, r, body, requestID, http.StatusInternalServerError, "InternalServerException",
			"Unable to list indexes.", readOnly, eventID, verified)
		return
	}
	payload, _ := s3vsvc.ListIndexesJSON(indexes)
	s.writeS3VectorsOK(w, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, s3vectorsEventSource, "ListIndexes", readOnly)
}

func (s *Server) s3vDeleteIndex(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	bucket, _ := params["vectorBucketName"].(string)
	if bucket == "" {
		bucket, _ = params["VectorBucketName"].(string)
	}
	indexName, _ := params["indexName"].(string)
	if indexName == "" {
		indexName, _ = params["IndexName"].(string)
	}
	if !s.authorize(verified, catalog.ActionS3VectorsDeleteIndex, "*") {
		s.writeS3VectorsError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform s3vectors:DeleteIndex.", readOnly, eventID, verified)
		return
	}
	err := s.store.DeleteS3VectorIndex(verified.AccountID, bucket, indexName)
	if errors.Is(err, store.ErrS3VectorsNotFound) {
		s.writeS3VectorsError(w, r, body, requestID, http.StatusNotFound, "NotFoundException",
			"Index not found.", readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeS3VectorsError(w, r, body, requestID, http.StatusInternalServerError, "InternalServerException",
			"Unable to delete index.", readOnly, eventID, verified)
		return
	}
	payload, _ := s3vsvc.DeleteOKJSON()
	s.writeS3VectorsOK(w, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, s3vectorsEventSource, "DeleteIndex", readOnly)
}

func parseFloat32Vector(data map[string]any) []float64 {
	raw, _ := data["float32"].([]any)
	out := make([]float64, 0, len(raw))
	for _, v := range raw {
		switch n := v.(type) {
		case float64:
			out = append(out, n)
		}
	}
	return out
}

func (s *Server) s3vPutVectors(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	bucket, _ := params["vectorBucketName"].(string)
	if bucket == "" {
		bucket, _ = params["VectorBucketName"].(string)
	}
	indexName, _ := params["indexName"].(string)
	if indexName == "" {
		indexName, _ = params["IndexName"].(string)
	}
	if !s.authorize(verified, catalog.ActionS3VectorsPutVectors, "*") {
		s.writeS3VectorsError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform s3vectors:PutVectors.", readOnly, eventID, verified)
		return
	}
	raw, _ := params["vectors"].([]any)
	if raw == nil {
		raw, _ = params["Vectors"].([]any)
	}
	var vectors []store.S3Vector
	for _, item := range raw {
		m, _ := item.(map[string]any)
		key, _ := m["key"].(string)
		if key == "" {
			key, _ = m["Key"].(string)
		}
		dataMap, _ := m["data"].(map[string]any)
		if dataMap == nil {
			dataMap, _ = m["Data"].(map[string]any)
		}
		metaJSON := "{}"
		if meta, ok := m["metadata"]; ok {
			b, _ := json.Marshal(meta)
			metaJSON = string(b)
		}
		vectors = append(vectors, store.S3Vector{
			Key: key, Data: parseFloat32Vector(dataMap), MetadataJSON: metaJSON,
		})
	}
	err := s.store.PutS3Vectors(verified.AccountID, bucket, indexName, vectors)
	if errors.Is(err, store.ErrS3VectorsNotFound) {
		s.writeS3VectorsError(w, r, body, requestID, http.StatusNotFound, "NotFoundException",
			"Index not found.", readOnly, eventID, verified)
		return
	}
	if errors.Is(err, store.ErrS3VectorsBadRequest) {
		s.writeS3VectorsError(w, r, body, requestID, http.StatusBadRequest, "ValidationException",
			err.Error(), readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeS3VectorsError(w, r, body, requestID, http.StatusInternalServerError, "InternalServerException",
			"Unable to put vectors.", readOnly, eventID, verified)
		return
	}
	payload, _ := s3vsvc.PutVectorsJSON()
	s.writeS3VectorsOK(w, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, s3vectorsEventSource, "PutVectors", readOnly)
}

func (s *Server) s3vQueryVectors(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	bucket, _ := params["vectorBucketName"].(string)
	if bucket == "" {
		bucket, _ = params["VectorBucketName"].(string)
	}
	indexName, _ := params["indexName"].(string)
	if indexName == "" {
		indexName, _ = params["IndexName"].(string)
	}
	topK := 10
	if t, ok := params["topK"].(float64); ok {
		topK = int(t)
	} else if t, ok := params["TopK"].(float64); ok {
		topK = int(t)
	}
	queryMap, _ := params["queryVector"].(map[string]any)
	if queryMap == nil {
		queryMap, _ = params["QueryVector"].(map[string]any)
	}
	if !s.authorize(verified, catalog.ActionS3VectorsQueryVectors, "*") {
		s.writeS3VectorsError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform s3vectors:QueryVectors.", readOnly, eventID, verified)
		return
	}
	matches, err := s.store.QueryS3Vectors(verified.AccountID, bucket, indexName, parseFloat32Vector(queryMap), topK)
	if errors.Is(err, store.ErrS3VectorsNotFound) {
		s.writeS3VectorsError(w, r, body, requestID, http.StatusNotFound, "NotFoundException",
			"Index not found.", readOnly, eventID, verified)
		return
	}
	if errors.Is(err, store.ErrS3VectorsBadRequest) {
		s.writeS3VectorsError(w, r, body, requestID, http.StatusBadRequest, "ValidationException",
			err.Error(), readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeS3VectorsError(w, r, body, requestID, http.StatusInternalServerError, "InternalServerException",
			"Unable to query vectors.", readOnly, eventID, verified)
		return
	}
	payload, _ := s3vsvc.QueryVectorsJSON(matches)
	s.writeS3VectorsOK(w, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, s3vectorsEventSource, "QueryVectors", readOnly)
}

func (s *Server) writeS3VectorsOK(w http.ResponseWriter, payload []byte) {
	w.Header().Set("Content-Type", s3vectorsJSONContentType)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(payload)
}

func (s *Server) writeS3VectorsError(
	w http.ResponseWriter, r *http.Request, body []byte, requestID string, status int, code, message string,
	readOnly bool, eventID string, verified *authn.Verified,
) {
	w.Header().Set("Content-Type", s3vectorsJSONContentType)
	w.Header().Set("x-amzn-ErrorType", code)
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"__type": code, "message": message})
	_ = body
	s.writeSuccessAudit(r, requestID, eventID, verified, s3vectorsEventSource, code, readOnly)
}
