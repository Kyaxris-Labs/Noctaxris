package server

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/Kyaxris-Labs/Noctaxris/internal/catalog"
	"github.com/Kyaxris-Labs/Noctaxris/internal/kernel/authn"
	kinesissvc "github.com/Kyaxris-Labs/Noctaxris/internal/services/kinesis"
	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

const (
	kinesisJSONContentType = "application/x-amz-json-1.1"
	kinesisEventSource     = "kinesis.amazonaws.com"
)

func (s *Server) handleKinesis(
	w http.ResponseWriter,
	r *http.Request,
	body []byte,
	requestID, eventID, action string,
	verified *authn.Verified,
	readOnly bool,
) {
	params := jsonBodyMap(body)
	action = kinesisAction(action)

	switch action {
	case catalog.ActionKinesisCreateStream:
		s.kinesisCreateStream(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionKinesisDeleteStream:
		s.kinesisDeleteStream(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionKinesisDescribeStream:
		s.kinesisDescribeStream(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionKinesisListStreams:
		s.kinesisListStreams(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionKinesisPutRecord:
		s.kinesisPutRecord(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionKinesisPutRecords:
		s.kinesisPutRecords(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionKinesisGetShardIterator:
		s.kinesisGetShardIterator(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionKinesisGetRecords:
		s.kinesisGetRecords(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionKinesisPutResourcePolicy:
		s.kinesisPutResourcePolicy(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionKinesisGetResourcePolicy:
		s.kinesisGetResourcePolicy(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionKinesisDeleteResourcePolicy:
		s.kinesisDeleteResourcePolicy(w, r, body, requestID, eventID, verified, readOnly, params)
	default:
		s.writeKinesisError(w, r, body, requestID, http.StatusNotImplemented, "InternalFailure",
			"This Kinesis action is not implemented.", readOnly, eventID, verified)
	}
}

func kinesisAction(action string) string {
	if strings.Contains(action, ":") {
		return action
	}
	switch action {
	case "CreateStream":
		return catalog.ActionKinesisCreateStream
	case "DeleteStream":
		return catalog.ActionKinesisDeleteStream
	case "DescribeStream":
		return catalog.ActionKinesisDescribeStream
	case "ListStreams":
		return catalog.ActionKinesisListStreams
	case "PutRecord":
		return catalog.ActionKinesisPutRecord
	case "PutRecords":
		return catalog.ActionKinesisPutRecords
	case "GetShardIterator":
		return catalog.ActionKinesisGetShardIterator
	case "GetRecords":
		return catalog.ActionKinesisGetRecords
	case "PutResourcePolicy":
		return catalog.ActionKinesisPutResourcePolicy
	case "GetResourcePolicy":
		return catalog.ActionKinesisGetResourcePolicy
	case "DeleteResourcePolicy":
		return catalog.ActionKinesisDeleteResourcePolicy
	default:
		return action
	}
}

func (s *Server) kinesisRegion(verified *authn.Verified) string {
	if verified.Region != "" {
		return verified.Region
	}
	return store.DefaultKinesisRegion
}

func (s *Server) kinesisCreateStream(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	name, _ := params["StreamName"].(string)
	if strings.TrimSpace(name) == "" {
		s.writeKinesisError(w, r, body, requestID, http.StatusBadRequest, "InvalidArgumentException",
			"StreamName is required.", readOnly, eventID, verified)
		return
	}
	arn := store.KinesisStreamARN(s.kinesisRegion(verified), verified.AccountID, name)
	if !s.authorize(verified, catalog.ActionKinesisCreateStream, arn) {
		s.writeKinesisError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform kinesis:CreateStream.", readOnly, eventID, verified)
		return
	}
	shardCount := 1
	if v, ok := params["ShardCount"].(float64); ok && v > 0 {
		shardCount = int(v)
	}
	_, err := s.store.CreateKinesisStream(verified.AccountID, s.kinesisRegion(verified), name, shardCount)
	if errors.Is(err, store.ErrKinesisStreamExists) {
		s.writeKinesisError(w, r, body, requestID, http.StatusBadRequest, "ResourceInUseException",
			"Stream already exists.", readOnly, eventID, verified)
		return
	}
	if errors.Is(err, store.ErrKinesisInvalidShard) {
		s.writeKinesisError(w, r, body, requestID, http.StatusBadRequest, "InvalidArgumentException",
			"ShardCount must be between 1 and 4.", readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeKinesisError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to create stream.", readOnly, eventID, verified)
		return
	}
	payload, _ := kinesissvc.EmptyOKJSON()
	s.writeKinesisOK(w, requestID, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, kinesisEventSource, "CreateStream", readOnly)
}

func (s *Server) kinesisDeleteStream(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	name, _ := params["StreamName"].(string)
	if strings.TrimSpace(name) == "" {
		s.writeKinesisError(w, r, body, requestID, http.StatusBadRequest, "InvalidArgumentException",
			"StreamName is required.", readOnly, eventID, verified)
		return
	}
	arn := store.KinesisStreamARN(s.kinesisRegion(verified), verified.AccountID, name)
	if !s.authorize(verified, catalog.ActionKinesisDeleteStream, arn) {
		s.writeKinesisError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform kinesis:DeleteStream.", readOnly, eventID, verified)
		return
	}
	err := s.store.DeleteKinesisStream(verified.AccountID, name)
	if errors.Is(err, store.ErrKinesisStreamNotFound) {
		s.writeKinesisError(w, r, body, requestID, http.StatusBadRequest, "ResourceNotFoundException",
			"Stream not found.", readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeKinesisError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to delete stream.", readOnly, eventID, verified)
		return
	}
	payload, _ := kinesissvc.EmptyOKJSON()
	s.writeKinesisOK(w, requestID, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, kinesisEventSource, "DeleteStream", readOnly)
}

func (s *Server) kinesisDescribeStream(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	name, _ := params["StreamName"].(string)
	if strings.TrimSpace(name) == "" {
		s.writeKinesisError(w, r, body, requestID, http.StatusBadRequest, "InvalidArgumentException",
			"StreamName is required.", readOnly, eventID, verified)
		return
	}
	arn := store.KinesisStreamARN(s.kinesisRegion(verified), verified.AccountID, name)
	if !s.authorize(verified, catalog.ActionKinesisDescribeStream, arn) {
		s.writeKinesisError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform kinesis:DescribeStream.", readOnly, eventID, verified)
		return
	}
	st, err := s.store.DescribeKinesisStream(verified.AccountID, name)
	if errors.Is(err, store.ErrKinesisStreamNotFound) {
		s.writeKinesisError(w, r, body, requestID, http.StatusBadRequest, "ResourceNotFoundException",
			"Stream not found.", readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeKinesisError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to describe stream.", readOnly, eventID, verified)
		return
	}
	payload, err := kinesissvc.DescribeStreamJSON(st)
	if err != nil {
		s.writeKinesisError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to build response.", readOnly, eventID, verified)
		return
	}
	s.writeKinesisOK(w, requestID, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, kinesisEventSource, "DescribeStream", readOnly)
}

func (s *Server) kinesisListStreams(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	if !s.authorize(verified, catalog.ActionKinesisListStreams, "*") {
		s.writeKinesisError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform kinesis:ListStreams.", readOnly, eventID, verified)
		return
	}
	_ = params
	names, err := s.store.ListKinesisStreams(verified.AccountID, "")
	if err != nil {
		s.writeKinesisError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to list streams.", readOnly, eventID, verified)
		return
	}
	payload, err := kinesissvc.ListStreamsJSON(names)
	if err != nil {
		s.writeKinesisError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to build response.", readOnly, eventID, verified)
		return
	}
	s.writeKinesisOK(w, requestID, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, kinesisEventSource, "ListStreams", readOnly)
}

func (s *Server) kinesisPutRecord(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	name, _ := params["StreamName"].(string)
	pk, _ := params["PartitionKey"].(string)
	if strings.TrimSpace(name) == "" || strings.TrimSpace(pk) == "" {
		s.writeKinesisError(w, r, body, requestID, http.StatusBadRequest, "InvalidArgumentException",
			"StreamName and PartitionKey are required.", readOnly, eventID, verified)
		return
	}
	arn := store.KinesisStreamARN(s.kinesisRegion(verified), verified.AccountID, name)
	if !s.authorize(verified, catalog.ActionKinesisPutRecord, arn) {
		s.writeKinesisError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform kinesis:PutRecord.", readOnly, eventID, verified)
		return
	}
	data, err := store.DecodeKinesisData(params["Data"])
	if err != nil {
		s.writeKinesisError(w, r, body, requestID, http.StatusBadRequest, "InvalidArgumentException",
			"Data must be base64.", readOnly, eventID, verified)
		return
	}
	seq, shard, err := s.store.PutKinesisRecord(verified.AccountID, name, pk, data)
	if errors.Is(err, store.ErrKinesisStreamNotFound) {
		s.writeKinesisError(w, r, body, requestID, http.StatusBadRequest, "ResourceNotFoundException",
			"Stream not found.", readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeKinesisError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to put record.", readOnly, eventID, verified)
		return
	}
	payload, err := kinesissvc.PutRecordJSON(seq, shard)
	if err != nil {
		s.writeKinesisError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to build response.", readOnly, eventID, verified)
		return
	}
	s.writeKinesisOK(w, requestID, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, kinesisEventSource, "PutRecord", readOnly)
}

func (s *Server) kinesisPutRecords(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	name, _ := params["StreamName"].(string)
	if strings.TrimSpace(name) == "" {
		s.writeKinesisError(w, r, body, requestID, http.StatusBadRequest, "InvalidArgumentException",
			"StreamName is required.", readOnly, eventID, verified)
		return
	}
	arn := store.KinesisStreamARN(s.kinesisRegion(verified), verified.AccountID, name)
	if !s.authorize(verified, catalog.ActionKinesisPutRecords, arn) {
		s.writeKinesisError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform kinesis:PutRecords.", readOnly, eventID, verified)
		return
	}
	raw, _ := params["Records"].([]any)
	entries := make([]store.PutKinesisRecordsEntry, 0, len(raw))
	for _, item := range raw {
		m, _ := item.(map[string]any)
		if m == nil {
			continue
		}
		pk, _ := m["PartitionKey"].(string)
		data, _ := store.DecodeKinesisData(m["Data"])
		entries = append(entries, store.PutKinesisRecordsEntry{PartitionKey: pk, Data: data})
	}
	results, failed, err := s.store.PutKinesisRecords(verified.AccountID, name, entries)
	if errors.Is(err, store.ErrKinesisStreamNotFound) {
		s.writeKinesisError(w, r, body, requestID, http.StatusBadRequest, "ResourceNotFoundException",
			"Stream not found.", readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeKinesisError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to put records.", readOnly, eventID, verified)
		return
	}
	payload, err := kinesissvc.PutRecordsJSON(results, failed)
	if err != nil {
		s.writeKinesisError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to build response.", readOnly, eventID, verified)
		return
	}
	s.writeKinesisOK(w, requestID, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, kinesisEventSource, "PutRecords", readOnly)
}

func (s *Server) kinesisGetShardIterator(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	name, _ := params["StreamName"].(string)
	shardID, _ := params["ShardId"].(string)
	itType, _ := params["ShardIteratorType"].(string)
	startSeq, _ := params["StartingSequenceNumber"].(string)
	if strings.TrimSpace(name) == "" {
		s.writeKinesisError(w, r, body, requestID, http.StatusBadRequest, "InvalidArgumentException",
			"StreamName is required.", readOnly, eventID, verified)
		return
	}
	arn := store.KinesisStreamARN(s.kinesisRegion(verified), verified.AccountID, name)
	if !s.authorize(verified, catalog.ActionKinesisGetShardIterator, arn) {
		s.writeKinesisError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform kinesis:GetShardIterator.", readOnly, eventID, verified)
		return
	}
	it, err := s.store.GetKinesisShardIterator(verified.AccountID, name, shardID, itType, startSeq)
	if errors.Is(err, store.ErrKinesisStreamNotFound) {
		s.writeKinesisError(w, r, body, requestID, http.StatusBadRequest, "ResourceNotFoundException",
			"Stream not found.", readOnly, eventID, verified)
		return
	}
	if errors.Is(err, store.ErrKinesisInvalidShard) {
		s.writeKinesisError(w, r, body, requestID, http.StatusBadRequest, "InvalidArgumentException",
			"Invalid shard or sequence.", readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeKinesisError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to get shard iterator.", readOnly, eventID, verified)
		return
	}
	payload, err := kinesissvc.GetShardIteratorJSON(it)
	if err != nil {
		s.writeKinesisError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to build response.", readOnly, eventID, verified)
		return
	}
	s.writeKinesisOK(w, requestID, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, kinesisEventSource, "GetShardIterator", readOnly)
}

func (s *Server) kinesisGetRecords(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	iterator, _ := params["ShardIterator"].(string)
	if strings.TrimSpace(iterator) == "" {
		s.writeKinesisError(w, r, body, requestID, http.StatusBadRequest, "InvalidArgumentException",
			"ShardIterator is required.", readOnly, eventID, verified)
		return
	}
	if !s.authorize(verified, catalog.ActionKinesisGetRecords, "*") {
		s.writeKinesisError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform kinesis:GetRecords.", readOnly, eventID, verified)
		return
	}
	limit := 0
	if v, ok := params["Limit"].(float64); ok {
		limit = int(v)
	}
	recs, next, err := s.store.GetKinesisRecords(iterator, limit)
	if errors.Is(err, store.ErrKinesisExpiredIterator) {
		s.writeKinesisError(w, r, body, requestID, http.StatusBadRequest, "ExpiredIteratorException",
			"Iterator expired or invalid.", readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeKinesisError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to get records.", readOnly, eventID, verified)
		return
	}
	payload, err := kinesissvc.GetRecordsJSON(recs, next)
	if err != nil {
		s.writeKinesisError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to build response.", readOnly, eventID, verified)
		return
	}
	s.writeKinesisOK(w, requestID, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, kinesisEventSource, "GetRecords", readOnly)
}

func (s *Server) kinesisPutResourcePolicy(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	resourceARN, _ := params["ResourceArn"].(string)
	if strings.TrimSpace(resourceARN) == "" {
		resourceARN, _ = params["StreamName"].(string)
	}
	policy, _ := params["Policy"].(string)
	if strings.TrimSpace(resourceARN) == "" || strings.TrimSpace(policy) == "" {
		s.writeKinesisError(w, r, body, requestID, http.StatusBadRequest, "InvalidArgumentException",
			"ResourceArn and Policy are required.", readOnly, eventID, verified)
		return
	}
	arn := resourceARN
	if !strings.Contains(resourceARN, ":stream/") {
		arn = store.KinesisStreamARN(s.kinesisRegion(verified), verified.AccountID, resourceARN)
	}
	if !s.authorize(verified, catalog.ActionKinesisPutResourcePolicy, arn) {
		s.writeKinesisError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform kinesis:PutResourcePolicy.", readOnly, eventID, verified)
		return
	}
	if err := s.store.PutKinesisResourcePolicy(verified.AccountID, resourceARN, policy); err != nil {
		if errors.Is(err, store.ErrKinesisStreamNotFound) {
			s.writeKinesisError(w, r, body, requestID, http.StatusBadRequest, "ResourceNotFoundException",
				"Stream not found.", readOnly, eventID, verified)
			return
		}
		s.writeKinesisError(w, r, body, requestID, http.StatusBadRequest, "InvalidArgumentException",
			err.Error(), readOnly, eventID, verified)
		return
	}
	s.writeKinesisOK(w, requestID, []byte(`{}`))
	s.writeSuccessAudit(r, requestID, eventID, verified, kinesisEventSource, "PutResourcePolicy", readOnly)
}

func (s *Server) kinesisGetResourcePolicy(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	resourceARN, _ := params["ResourceArn"].(string)
	if strings.TrimSpace(resourceARN) == "" {
		resourceARN, _ = params["StreamName"].(string)
	}
	if strings.TrimSpace(resourceARN) == "" {
		s.writeKinesisError(w, r, body, requestID, http.StatusBadRequest, "InvalidArgumentException",
			"ResourceArn is required.", readOnly, eventID, verified)
		return
	}
	arn := resourceARN
	if !strings.Contains(resourceARN, ":stream/") {
		arn = store.KinesisStreamARN(s.kinesisRegion(verified), verified.AccountID, resourceARN)
	}
	if !s.authorize(verified, catalog.ActionKinesisGetResourcePolicy, arn) {
		s.writeKinesisError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform kinesis:GetResourcePolicy.", readOnly, eventID, verified)
		return
	}
	policy, err := s.store.GetKinesisResourcePolicy(verified.AccountID, resourceARN)
	if errors.Is(err, store.ErrKinesisStreamNotFound) {
		s.writeKinesisError(w, r, body, requestID, http.StatusBadRequest, "ResourceNotFoundException",
			"Stream not found.", readOnly, eventID, verified)
		return
	}
	if errors.Is(err, store.ErrNoSuchResourcePolicy) {
		s.writeKinesisError(w, r, body, requestID, http.StatusBadRequest, "ResourceNotFoundException",
			"Resource policy not found.", readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeKinesisError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to get resource policy.", readOnly, eventID, verified)
		return
	}
	payload, _ := json.Marshal(map[string]string{"Policy": policy})
	s.writeKinesisOK(w, requestID, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, kinesisEventSource, "GetResourcePolicy", readOnly)
}

func (s *Server) kinesisDeleteResourcePolicy(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	resourceARN, _ := params["ResourceArn"].(string)
	if strings.TrimSpace(resourceARN) == "" {
		resourceARN, _ = params["StreamName"].(string)
	}
	if strings.TrimSpace(resourceARN) == "" {
		s.writeKinesisError(w, r, body, requestID, http.StatusBadRequest, "InvalidArgumentException",
			"ResourceArn is required.", readOnly, eventID, verified)
		return
	}
	arn := resourceARN
	if !strings.Contains(resourceARN, ":stream/") {
		arn = store.KinesisStreamARN(s.kinesisRegion(verified), verified.AccountID, resourceARN)
	}
	if !s.authorize(verified, catalog.ActionKinesisDeleteResourcePolicy, arn) {
		s.writeKinesisError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform kinesis:DeleteResourcePolicy.", readOnly, eventID, verified)
		return
	}
	if err := s.store.DeleteKinesisResourcePolicy(verified.AccountID, resourceARN); err != nil {
		if errors.Is(err, store.ErrKinesisStreamNotFound) {
			s.writeKinesisError(w, r, body, requestID, http.StatusBadRequest, "ResourceNotFoundException",
				"Stream not found.", readOnly, eventID, verified)
			return
		}
		s.writeKinesisError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to delete resource policy.", readOnly, eventID, verified)
		return
	}
	s.writeKinesisOK(w, requestID, []byte(`{}`))
	s.writeSuccessAudit(r, requestID, eventID, verified, kinesisEventSource, "DeleteResourcePolicy", readOnly)
}

func (s *Server) writeKinesisOK(w http.ResponseWriter, requestID string, payload []byte) {
	w.Header().Set(requestIDHeader, requestID)
	w.Header().Set("Content-Type", kinesisJSONContentType)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(payload)
}

func (s *Server) writeKinesisError(
	w http.ResponseWriter, r *http.Request, body []byte, requestID string,
	status int, code, message string, readOnly bool, eventID string, verified *authn.Verified,
) {
	_ = body
	w.Header().Set(requestIDHeader, requestID)
	w.Header().Set("Content-Type", kinesisJSONContentType)
	w.WriteHeader(status)
	payload, _ := json.Marshal(map[string]string{"__type": code, "message": message})
	_, _ = w.Write(payload)
	accessKeyID := ""
	accountID := ""
	if verified != nil {
		accessKeyID = verified.AccessKeyID
		accountID = verified.AccountID
	}
	s.auditAPIError(r, requestID, eventID, code, message, readOnly, accessKeyID, accountID, verified != nil)
}
