package server

import (
	"errors"
	"net/http"
	"strings"

	"github.com/Kyaxris-Labs/Noctaxris/internal/catalog"
	"github.com/Kyaxris-Labs/Noctaxris/internal/kernel/authn"
	ddbstreams "github.com/Kyaxris-Labs/Noctaxris/internal/services/dynamodbstreams"
	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

const (
	dynamoStreamsJSONContentType = "application/x-amz-json-1.0"
	dynamoStreamsEventSource     = "dynamodb.amazonaws.com"
)

func (s *Server) handleDynamoDBStreams(
	w http.ResponseWriter,
	r *http.Request,
	body []byte,
	requestID, eventID, action string,
	verified *authn.Verified,
	readOnly bool,
) {
	params := jsonBodyMap(body)
	action = dynamodbstreamsAction(action)

	switch action {
	case catalog.ActionDynamoDBStreamsListStreams:
		s.ddbStreamsList(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionDynamoDBStreamsDescribeStream:
		s.ddbStreamsDescribe(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionDynamoDBStreamsGetShardIterator:
		s.ddbStreamsGetIterator(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionDynamoDBStreamsGetRecords:
		s.ddbStreamsGetRecords(w, r, body, requestID, eventID, verified, readOnly, params)
	default:
		s.writeDynamoStreamsError(w, r, body, requestID, http.StatusNotImplemented, "InternalFailure",
			"This DynamoDB Streams action is not implemented.", readOnly, eventID, verified)
	}
}

func dynamodbstreamsAction(action string) string {
	if strings.Contains(action, ":") {
		return action
	}
	switch action {
	case "ListStreams":
		return catalog.ActionDynamoDBStreamsListStreams
	case "DescribeStream":
		return catalog.ActionDynamoDBStreamsDescribeStream
	case "GetShardIterator":
		return catalog.ActionDynamoDBStreamsGetShardIterator
	case "GetRecords":
		return catalog.ActionDynamoDBStreamsGetRecords
	default:
		return action
	}
}

func (s *Server) dynamoStreamsRegion(verified *authn.Verified) string {
	if verified.Region != "" {
		return verified.Region
	}
	return store.DefaultDynamoRegion
}

func (s *Server) ddbStreamsList(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	if !s.authorize(verified, catalog.ActionDynamoDBStreamsListStreams, "*") {
		s.writeDynamoStreamsError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform dynamodbstreams:ListStreams.", readOnly, eventID, verified)
		return
	}
	tableName, _ := params["TableName"].(string)
	tables, err := s.store.ListDynamoStreams(verified.AccountID, tableName)
	if err != nil {
		s.writeDynamoStreamsError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to list streams.", readOnly, eventID, verified)
		return
	}
	payload, err := ddbstreams.ListStreamsJSON(tables, s.dynamoStreamsRegion(verified))
	if err != nil {
		s.writeDynamoStreamsError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to build response.", readOnly, eventID, verified)
		return
	}
	s.writeDynamoStreamsOK(w, requestID, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, dynamoStreamsEventSource, "ListStreams", readOnly)
}

func (s *Server) ddbStreamsDescribe(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	streamARN, _ := params["StreamArn"].(string)
	accountID, tableName, label, ok := store.ParseDynamoStreamARN(streamARN)
	if !ok {
		s.writeDynamoStreamsError(w, r, body, requestID, http.StatusBadRequest, "ValidationException",
			"StreamArn is required.", readOnly, eventID, verified)
		return
	}
	if accountID != verified.AccountID {
		s.writeDynamoStreamsError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform dynamodbstreams:DescribeStream.", readOnly, eventID, verified)
		return
	}
	if !s.authorize(verified, catalog.ActionDynamoDBStreamsDescribeStream, streamARN) {
		s.writeDynamoStreamsError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform dynamodbstreams:DescribeStream.", readOnly, eventID, verified)
		return
	}
	table, err := s.store.DescribeDynamoStream(accountID, tableName, label)
	if errors.Is(err, store.ErrDynamoStreamNotFound) || errors.Is(err, store.ErrNoSuchTable) {
		s.writeDynamoStreamsError(w, r, body, requestID, http.StatusBadRequest, "ResourceNotFoundException",
			"Stream not found.", readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeDynamoStreamsError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to describe stream.", readOnly, eventID, verified)
		return
	}
	payload, err := ddbstreams.DescribeStreamJSON(table, s.dynamoStreamsRegion(verified))
	if err != nil {
		s.writeDynamoStreamsError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to build response.", readOnly, eventID, verified)
		return
	}
	s.writeDynamoStreamsOK(w, requestID, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, dynamoStreamsEventSource, "DescribeStream", readOnly)
}

func (s *Server) ddbStreamsGetIterator(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	streamARN, _ := params["StreamArn"].(string)
	accountID, tableName, label, ok := store.ParseDynamoStreamARN(streamARN)
	if !ok {
		s.writeDynamoStreamsError(w, r, body, requestID, http.StatusBadRequest, "ValidationException",
			"StreamArn is required.", readOnly, eventID, verified)
		return
	}
	if accountID != verified.AccountID {
		s.writeDynamoStreamsError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform dynamodbstreams:GetShardIterator.", readOnly, eventID, verified)
		return
	}
	if !s.authorize(verified, catalog.ActionDynamoDBStreamsGetShardIterator, streamARN) {
		s.writeDynamoStreamsError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform dynamodbstreams:GetShardIterator.", readOnly, eventID, verified)
		return
	}
	if _, err := s.store.DescribeDynamoStream(accountID, tableName, label); err != nil {
		s.writeDynamoStreamsError(w, r, body, requestID, http.StatusBadRequest, "ResourceNotFoundException",
			"Stream not found.", readOnly, eventID, verified)
		return
	}
	shardID, _ := params["ShardId"].(string)
	iterType, _ := params["ShardIteratorType"].(string)
	startSeq, _ := params["SequenceNumber"].(string)
	iterator, err := s.store.GetDynamoStreamShardIterator(accountID, tableName, shardID, iterType, startSeq)
	if errors.Is(err, store.ErrDynamoStreamInvalidShard) {
		s.writeDynamoStreamsError(w, r, body, requestID, http.StatusBadRequest, "ValidationException",
			"Invalid shard or sequence number.", readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeDynamoStreamsError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to get shard iterator.", readOnly, eventID, verified)
		return
	}
	payload, _ := ddbstreams.GetShardIteratorJSON(iterator)
	s.writeDynamoStreamsOK(w, requestID, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, dynamoStreamsEventSource, "GetShardIterator", readOnly)
}

func (s *Server) ddbStreamsGetRecords(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	if !s.authorize(verified, catalog.ActionDynamoDBStreamsGetRecords, "*") {
		s.writeDynamoStreamsError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform dynamodbstreams:GetRecords.", readOnly, eventID, verified)
		return
	}
	iterator, _ := params["ShardIterator"].(string)
	limit := 0
	if v, ok := params["Limit"].(float64); ok {
		limit = int(v)
	}
	records, next, err := s.store.GetDynamoStreamRecords(iterator, limit)
	if errors.Is(err, store.ErrDynamoStreamExpiredIter) {
		s.writeDynamoStreamsError(w, r, body, requestID, http.StatusBadRequest, "ExpiredIteratorException",
			"Shard iterator has expired.", readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeDynamoStreamsError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to get records.", readOnly, eventID, verified)
		return
	}
	payload, err := ddbstreams.GetRecordsJSON(records, next)
	if err != nil {
		s.writeDynamoStreamsError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to build response.", readOnly, eventID, verified)
		return
	}
	s.writeDynamoStreamsOK(w, requestID, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, dynamoStreamsEventSource, "GetRecords", readOnly)
}

func (s *Server) writeDynamoStreamsOK(w http.ResponseWriter, requestID string, payload []byte) {
	w.Header().Set("Content-Type", dynamoStreamsJSONContentType)
	w.Header().Set("x-amzn-RequestId", requestID)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(payload)
}

func (s *Server) writeDynamoStreamsError(
	w http.ResponseWriter, r *http.Request, body []byte, requestID string,
	status int, code, message string, readOnly bool, eventID string, verified *authn.Verified,
) {
	w.Header().Set("Content-Type", dynamoStreamsJSONContentType)
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
