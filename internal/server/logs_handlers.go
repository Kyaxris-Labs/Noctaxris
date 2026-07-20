package server

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/Kyaxris-Labs/Noctaxris/internal/catalog"
	"github.com/Kyaxris-Labs/Noctaxris/internal/kernel/authn"
	logssvc "github.com/Kyaxris-Labs/Noctaxris/internal/services/logs"
	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

const (
	logsJSONContentType = "application/x-amz-json-1.1"
	logsEventSource     = "logs.amazonaws.com"
)

func (s *Server) handleLogs(
	w http.ResponseWriter,
	r *http.Request,
	body []byte,
	requestID, eventID, action string,
	verified *authn.Verified,
	readOnly bool,
) {
	params := jsonBodyMap(body)
	action = logsAction(action)

	switch action {
	case catalog.ActionLogsCreateLogGroup:
		s.logsCreateLogGroup(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionLogsCreateLogStream:
		s.logsCreateLogStream(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionLogsDeleteLogGroup:
		s.logsDeleteLogGroup(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionLogsDeleteLogStream:
		s.logsDeleteLogStream(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionLogsDescribeLogStreams:
		s.logsDescribeLogStreams(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionLogsPutLogEvents:
		s.logsPutLogEvents(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionLogsGetLogEvents:
		s.logsGetLogEvents(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionLogsDescribeLogGroups:
		s.logsDescribeLogGroups(w, r, body, requestID, eventID, verified, readOnly, params)
	default:
		s.writeLogsError(w, r, body, requestID, http.StatusNotImplemented, "InternalFailure",
			"This CloudWatch Logs action is not implemented.", readOnly, eventID, verified)
	}
}

func logsAction(action string) string {
	if strings.Contains(action, ":") {
		return action
	}
	switch action {
	case "CreateLogGroup":
		return catalog.ActionLogsCreateLogGroup
	case "CreateLogStream":
		return catalog.ActionLogsCreateLogStream
	case "DeleteLogGroup":
		return catalog.ActionLogsDeleteLogGroup
	case "DeleteLogStream":
		return catalog.ActionLogsDeleteLogStream
	case "DescribeLogStreams":
		return catalog.ActionLogsDescribeLogStreams
	case "PutLogEvents":
		return catalog.ActionLogsPutLogEvents
	case "GetLogEvents":
		return catalog.ActionLogsGetLogEvents
	case "DescribeLogGroups":
		return catalog.ActionLogsDescribeLogGroups
	default:
		return action
	}
}

func (s *Server) logsRegion(verified *authn.Verified) string {
	if verified.Region != "" {
		return verified.Region
	}
	return store.DefaultSSMRegion
}

func (s *Server) logsCreateLogGroup(
	w http.ResponseWriter,
	r *http.Request,
	body []byte,
	requestID, eventID string,
	verified *authn.Verified,
	readOnly bool,
	params map[string]any,
) {
	name, _ := params["logGroupName"].(string)
	if strings.TrimSpace(name) == "" {
		s.writeLogsError(w, r, body, requestID, http.StatusBadRequest, "InvalidParameterException",
			"logGroupName is required.", readOnly, eventID, verified)
		return
	}
	arn := store.LogGroupARN(s.logsRegion(verified), verified.AccountID, name)
	if !s.authorize(verified, catalog.ActionLogsCreateLogGroup, arn) {
		s.writeLogsError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform logs:CreateLogGroup.", readOnly, eventID, verified)
		return
	}
	_, err := s.store.CreateLogGroup(verified.AccountID, s.logsRegion(verified), name)
	if errors.Is(err, store.ErrLogGroupAlreadyExists) {
		s.writeLogsError(w, r, body, requestID, http.StatusBadRequest, "ResourceAlreadyExistsException",
			"The specified log group already exists.", readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeLogsError(w, r, body, requestID, http.StatusInternalServerError, "ServiceUnavailableException",
			"Unable to create log group.", readOnly, eventID, verified)
		return
	}
	payload, _ := logssvc.EmptyOKJSON()
	s.writeLogsOK(w, requestID, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, logsEventSource, "CreateLogGroup", readOnly)
}

func (s *Server) logsCreateLogStream(
	w http.ResponseWriter,
	r *http.Request,
	body []byte,
	requestID, eventID string,
	verified *authn.Verified,
	readOnly bool,
	params map[string]any,
) {
	group, _ := params["logGroupName"].(string)
	stream, _ := params["logStreamName"].(string)
	if strings.TrimSpace(group) == "" || strings.TrimSpace(stream) == "" {
		s.writeLogsError(w, r, body, requestID, http.StatusBadRequest, "InvalidParameterException",
			"logGroupName and logStreamName are required.", readOnly, eventID, verified)
		return
	}
	arn := store.LogStreamARN(s.logsRegion(verified), verified.AccountID, group, stream)
	if !s.authorize(verified, catalog.ActionLogsCreateLogStream, arn) {
		s.writeLogsError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform logs:CreateLogStream.", readOnly, eventID, verified)
		return
	}
	_, err := s.store.CreateLogStream(verified.AccountID, s.logsRegion(verified), group, stream)
	if errors.Is(err, store.ErrLogGroupNotFound) {
		s.writeLogsError(w, r, body, requestID, http.StatusBadRequest, "ResourceNotFoundException",
			"The specified log group does not exist.", readOnly, eventID, verified)
		return
	}
	if errors.Is(err, store.ErrLogStreamAlreadyExists) {
		s.writeLogsError(w, r, body, requestID, http.StatusBadRequest, "ResourceAlreadyExistsException",
			"The specified log stream already exists.", readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeLogsError(w, r, body, requestID, http.StatusInternalServerError, "ServiceUnavailableException",
			"Unable to create log stream.", readOnly, eventID, verified)
		return
	}
	payload, _ := logssvc.EmptyOKJSON()
	s.writeLogsOK(w, requestID, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, logsEventSource, "CreateLogStream", readOnly)
}

func (s *Server) logsDeleteLogGroup(
	w http.ResponseWriter,
	r *http.Request,
	body []byte,
	requestID, eventID string,
	verified *authn.Verified,
	readOnly bool,
	params map[string]any,
) {
	name, _ := params["logGroupName"].(string)
	if strings.TrimSpace(name) == "" {
		s.writeLogsError(w, r, body, requestID, http.StatusBadRequest, "InvalidParameterException",
			"logGroupName is required.", readOnly, eventID, verified)
		return
	}
	arn := store.LogGroupARN(s.logsRegion(verified), verified.AccountID, name)
	if !s.authorize(verified, catalog.ActionLogsDeleteLogGroup, arn) {
		s.writeLogsError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform logs:DeleteLogGroup.", readOnly, eventID, verified)
		return
	}
	err := s.store.DeleteLogGroup(verified.AccountID, name)
	if errors.Is(err, store.ErrLogGroupNotFound) {
		s.writeLogsError(w, r, body, requestID, http.StatusBadRequest, "ResourceNotFoundException",
			"The specified log group does not exist.", readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeLogsError(w, r, body, requestID, http.StatusInternalServerError, "ServiceUnavailableException",
			"Unable to delete log group.", readOnly, eventID, verified)
		return
	}
	payload, _ := logssvc.EmptyOKJSON()
	s.writeLogsOK(w, requestID, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, logsEventSource, "DeleteLogGroup", readOnly)
}

func (s *Server) logsDeleteLogStream(
	w http.ResponseWriter,
	r *http.Request,
	body []byte,
	requestID, eventID string,
	verified *authn.Verified,
	readOnly bool,
	params map[string]any,
) {
	group, _ := params["logGroupName"].(string)
	stream, _ := params["logStreamName"].(string)
	if strings.TrimSpace(group) == "" || strings.TrimSpace(stream) == "" {
		s.writeLogsError(w, r, body, requestID, http.StatusBadRequest, "InvalidParameterException",
			"logGroupName and logStreamName are required.", readOnly, eventID, verified)
		return
	}
	arn := store.LogStreamARN(s.logsRegion(verified), verified.AccountID, group, stream)
	if !s.authorize(verified, catalog.ActionLogsDeleteLogStream, arn) {
		s.writeLogsError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform logs:DeleteLogStream.", readOnly, eventID, verified)
		return
	}
	err := s.store.DeleteLogStream(verified.AccountID, group, stream)
	if errors.Is(err, store.ErrLogStreamNotFound) || errors.Is(err, store.ErrLogGroupNotFound) {
		s.writeLogsError(w, r, body, requestID, http.StatusBadRequest, "ResourceNotFoundException",
			"The specified log stream does not exist.", readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeLogsError(w, r, body, requestID, http.StatusInternalServerError, "ServiceUnavailableException",
			"Unable to delete log stream.", readOnly, eventID, verified)
		return
	}
	payload, _ := logssvc.EmptyOKJSON()
	s.writeLogsOK(w, requestID, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, logsEventSource, "DeleteLogStream", readOnly)
}

func (s *Server) logsDescribeLogStreams(
	w http.ResponseWriter,
	r *http.Request,
	body []byte,
	requestID, eventID string,
	verified *authn.Verified,
	readOnly bool,
	params map[string]any,
) {
	group, _ := params["logGroupName"].(string)
	if strings.TrimSpace(group) == "" {
		s.writeLogsError(w, r, body, requestID, http.StatusBadRequest, "InvalidParameterException",
			"logGroupName is required.", readOnly, eventID, verified)
		return
	}
	arn := store.LogGroupARN(s.logsRegion(verified), verified.AccountID, group)
	if !s.authorize(verified, catalog.ActionLogsDescribeLogStreams, arn) {
		s.writeLogsError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform logs:DescribeLogStreams.", readOnly, eventID, verified)
		return
	}
	prefix, _ := params["logStreamNamePrefix"].(string)
	streams, err := s.store.DescribeLogStreams(verified.AccountID, group, prefix)
	if errors.Is(err, store.ErrLogGroupNotFound) {
		s.writeLogsError(w, r, body, requestID, http.StatusBadRequest, "ResourceNotFoundException",
			"The specified log group does not exist.", readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeLogsError(w, r, body, requestID, http.StatusInternalServerError, "ServiceUnavailableException",
			"Unable to describe log streams.", readOnly, eventID, verified)
		return
	}
	payload, err := logssvc.DescribeLogStreamsJSON(streams)
	if err != nil {
		s.writeLogsError(w, r, body, requestID, http.StatusInternalServerError, "ServiceUnavailableException",
			"Unable to build response.", readOnly, eventID, verified)
		return
	}
	s.writeLogsOK(w, requestID, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, logsEventSource, "DescribeLogStreams", readOnly)
}

func (s *Server) logsPutLogEvents(
	w http.ResponseWriter,
	r *http.Request,
	body []byte,
	requestID, eventID string,
	verified *authn.Verified,
	readOnly bool,
	params map[string]any,
) {
	group, _ := params["logGroupName"].(string)
	stream, _ := params["logStreamName"].(string)
	seq, _ := params["sequenceToken"].(string)
	if strings.TrimSpace(group) == "" || strings.TrimSpace(stream) == "" {
		s.writeLogsError(w, r, body, requestID, http.StatusBadRequest, "InvalidParameterException",
			"logGroupName and logStreamName are required.", readOnly, eventID, verified)
		return
	}
	arn := store.LogStreamARN(s.logsRegion(verified), verified.AccountID, group, stream)
	if !s.authorize(verified, catalog.ActionLogsPutLogEvents, arn) {
		s.writeLogsError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform logs:PutLogEvents.", readOnly, eventID, verified)
		return
	}

	rawEvents, _ := params["logEvents"].([]any)
	events := make([]store.LogEvent, 0, len(rawEvents))
	for _, item := range rawEvents {
		m, _ := item.(map[string]any)
		if m == nil {
			continue
		}
		msg, _ := m["message"].(string)
		ts := int64(0)
		switch v := m["timestamp"].(type) {
		case float64:
			ts = int64(v)
		}
		events = append(events, store.LogEvent{Timestamp: ts, Message: msg})
	}

	next, _, err := s.store.PutLogEvents(verified.AccountID, group, stream, seq, events)
	if errors.Is(err, store.ErrLogStreamNotFound) || errors.Is(err, store.ErrLogGroupNotFound) {
		s.writeLogsError(w, r, body, requestID, http.StatusBadRequest, "ResourceNotFoundException",
			"The specified log stream does not exist.", readOnly, eventID, verified)
		return
	}
	if errors.Is(err, store.ErrInvalidSequenceToken) {
		s.writeLogsError(w, r, body, requestID, http.StatusBadRequest, "InvalidSequenceTokenException",
			"The given sequenceToken is invalid.", readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeLogsError(w, r, body, requestID, http.StatusInternalServerError, "ServiceUnavailableException",
			"Unable to put log events.", readOnly, eventID, verified)
		return
	}
	payload, err := logssvc.PutLogEventsJSON(next)
	if err != nil {
		s.writeLogsError(w, r, body, requestID, http.StatusInternalServerError, "ServiceUnavailableException",
			"Unable to build response.", readOnly, eventID, verified)
		return
	}
	s.writeLogsOK(w, requestID, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, logsEventSource, "PutLogEvents", readOnly)
}

func (s *Server) logsGetLogEvents(
	w http.ResponseWriter,
	r *http.Request,
	body []byte,
	requestID, eventID string,
	verified *authn.Verified,
	readOnly bool,
	params map[string]any,
) {
	group, _ := params["logGroupName"].(string)
	stream, _ := params["logStreamName"].(string)
	if strings.TrimSpace(group) == "" || strings.TrimSpace(stream) == "" {
		s.writeLogsError(w, r, body, requestID, http.StatusBadRequest, "InvalidParameterException",
			"logGroupName and logStreamName are required.", readOnly, eventID, verified)
		return
	}
	arn := store.LogStreamARN(s.logsRegion(verified), verified.AccountID, group, stream)
	if !s.authorize(verified, catalog.ActionLogsGetLogEvents, arn) {
		s.writeLogsError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform logs:GetLogEvents.", readOnly, eventID, verified)
		return
	}
	startFromHead := true
	if b, ok := params["startFromHead"].(bool); ok {
		startFromHead = b
	}
	startTime := logsInt64Param(params["startTime"])
	endTime := logsInt64Param(params["endTime"])
	limit := int(logsInt64Param(params["limit"]))

	events, err := s.store.GetLogEvents(verified.AccountID, group, stream, startTime, endTime, startFromHead, limit)
	if errors.Is(err, store.ErrLogStreamNotFound) || errors.Is(err, store.ErrLogGroupNotFound) {
		s.writeLogsError(w, r, body, requestID, http.StatusBadRequest, "ResourceNotFoundException",
			"The specified log stream does not exist.", readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeLogsError(w, r, body, requestID, http.StatusInternalServerError, "ServiceUnavailableException",
			"Unable to get log events.", readOnly, eventID, verified)
		return
	}
	payload, err := logssvc.GetLogEventsJSON(events)
	if err != nil {
		s.writeLogsError(w, r, body, requestID, http.StatusInternalServerError, "ServiceUnavailableException",
			"Unable to build response.", readOnly, eventID, verified)
		return
	}
	s.writeLogsOK(w, requestID, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, logsEventSource, "GetLogEvents", readOnly)
}

func (s *Server) logsDescribeLogGroups(
	w http.ResponseWriter,
	r *http.Request,
	body []byte,
	requestID, eventID string,
	verified *authn.Verified,
	readOnly bool,
	params map[string]any,
) {
	if !s.authorize(verified, catalog.ActionLogsDescribeLogGroups, "*") {
		s.writeLogsError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform logs:DescribeLogGroups.", readOnly, eventID, verified)
		return
	}
	prefix, _ := params["logGroupNamePrefix"].(string)
	groups, err := s.store.DescribeLogGroups(verified.AccountID, prefix)
	if err != nil {
		s.writeLogsError(w, r, body, requestID, http.StatusInternalServerError, "ServiceUnavailableException",
			"Unable to describe log groups.", readOnly, eventID, verified)
		return
	}
	payload, err := logssvc.DescribeLogGroupsJSON(groups)
	if err != nil {
		s.writeLogsError(w, r, body, requestID, http.StatusInternalServerError, "ServiceUnavailableException",
			"Unable to build response.", readOnly, eventID, verified)
		return
	}
	s.writeLogsOK(w, requestID, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, logsEventSource, "DescribeLogGroups", readOnly)
}

func logsInt64Param(v any) int64 {
	switch n := v.(type) {
	case float64:
		return int64(n)
	case int64:
		return n
	case int:
		return int64(n)
	default:
		return 0
	}
}

func (s *Server) writeLogsOK(w http.ResponseWriter, requestID string, payload []byte) {
	w.Header().Set(requestIDHeader, requestID)
	w.Header().Set("Content-Type", logsJSONContentType)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(payload)
}

func (s *Server) writeLogsError(
	w http.ResponseWriter,
	r *http.Request,
	body []byte,
	requestID string,
	status int,
	code, message string,
	readOnly bool,
	eventID string,
	verified *authn.Verified,
) {
	_ = body
	w.Header().Set(requestIDHeader, requestID)
	w.Header().Set("Content-Type", logsJSONContentType)
	w.WriteHeader(status)
	payload, _ := json.Marshal(map[string]string{
		"__type":  code,
		"message": message,
	})
	_, _ = w.Write(payload)

	accessKeyID := ""
	accountID := ""
	if verified != nil {
		accessKeyID = verified.AccessKeyID
		accountID = verified.AccountID
	}
	s.auditAPIError(r, requestID, eventID, code, message, readOnly, accessKeyID, accountID, verified != nil)
}
