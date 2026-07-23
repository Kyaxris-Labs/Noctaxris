package server

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/Kyaxris-Labs/Noctaxris/internal/catalog"
	"github.com/Kyaxris-Labs/Noctaxris/internal/kernel/authn"
	lambdasvc "github.com/Kyaxris-Labs/Noctaxris/internal/services/lambda"
	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

var (
	esmPollerOnce     sync.Once
	esmPollerStop     chan struct{}
	esmPollerStopOnce sync.Once
)

func (s *Server) ensureESMPoller() {
	esmPollerOnce.Do(func() {
		esmPollerStop = make(chan struct{})
		go s.runESMPoller(esmPollerStop)
	})
}

// StopESMPoller stops the in-process ESM poller if running.
func (s *Server) StopESMPoller() {
	esmPollerStopOnce.Do(func() {
		if esmPollerStop != nil {
			close(esmPollerStop)
		}
	})
}

func (s *Server) runESMPoller(stop <-chan struct{}) {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-stop:
			return
		case <-ticker.C:
			s.pollAllEventSourceMappings()
		}
	}
}

func (s *Server) pollAllEventSourceMappings() {
	mappings, err := s.store.ListEnabledEventSourceMappings()
	if err != nil || len(mappings) == 0 {
		return
	}
	for _, m := range mappings {
		_ = s.store.PollEventSourceMappingOnce(m.UUID, func(accountID, functionName, qualifier, eventJSON string) (string, error) {
			if strings.TrimSpace(qualifier) == "" {
				qualifier = "$LATEST"
			}
			fn, executedVersion, err := s.store.ResolveFunction(accountID, functionName, qualifier)
			if err != nil {
				return "", err
			}
			out, err := s.executeLambdaInvoke(context.Background(), accountID, functionName, fn, executedVersion, eventJSON)
			return string(out), err
		})
	}
}

func (s *Server) lambdaCreateEventSourceMapping(
	w http.ResponseWriter,
	r *http.Request,
	body []byte,
	requestID, eventID string,
	verified *authn.Verified,
	readOnly bool,
	params map[string]any,
) {
	s.ensureESMPoller()
	fnName, _ := params["FunctionName"].(string)
	fnName = strings.TrimSpace(fnName)
	if fnName == "" {
		s.writeLambdaError(w, r, body, requestID, http.StatusBadRequest, "ValidationException",
			"FunctionName is required.", readOnly, eventID, verified)
		return
	}
	eventSourceARN, _ := params["EventSourceArn"].(string)
	resource := "arn:aws:lambda:" + store.DefaultLambdaRegion + ":" + verified.AccountID + ":event-source-mapping"
	if !s.authorizeLambda(verified, catalog.ActionLambdaCreateEventSourceMapping, resource, "") {
		s.writeLambdaError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform lambda:CreateEventSourceMapping.", readOnly, eventID, verified)
		return
	}
	batchSize := 0
	switch v := params["BatchSize"].(type) {
	case float64:
		batchSize = int(v)
	case int:
		batchSize = v
	}
	var enabled *bool
	if raw, ok := params["Enabled"]; ok {
		switch t := raw.(type) {
		case bool:
			enabled = &t
		}
	}
	filterJSON := ""
	if rawFC, ok := params["FilterCriteria"]; ok && rawFC != nil {
		b, mErr := json.Marshal(rawFC)
		if mErr != nil {
			s.writeLambdaError(w, r, body, requestID, http.StatusBadRequest, "InvalidParameterValueException",
				"FilterCriteria must be an object.", readOnly, eventID, verified)
			return
		}
		filterJSON = string(b)
	}
	respTypesJSON := ""
	if rawRT, ok := params["FunctionResponseTypes"]; ok && rawRT != nil {
		b, mErr := json.Marshal(rawRT)
		if mErr != nil {
			s.writeLambdaError(w, r, body, requestID, http.StatusBadRequest, "InvalidParameterValueException",
				"FunctionResponseTypes must be an array.", readOnly, eventID, verified)
			return
		}
		respTypesJSON = string(b)
	}
	m, err := s.store.CreateEventSourceMapping(store.CreateEventSourceMappingInput{
		AccountID:                 verified.AccountID,
		FunctionName:              fnName,
		EventSourceARN:            eventSourceARN,
		BatchSize:                 batchSize,
		Enabled:                   enabled,
		FilterCriteriaJSON:        filterJSON,
		FunctionResponseTypesJSON: respTypesJSON,
	})
	if errors.Is(err, store.ErrNoSuchFunction) || errors.Is(err, store.ErrInvalidFunctionName) {
		s.writeLambdaError(w, r, body, requestID, http.StatusNotFound, "ResourceNotFoundException",
			"Function not found.", readOnly, eventID, verified)
		return
	}
	if errors.Is(err, store.ErrNoSuchQueue) || errors.Is(err, store.ErrInvalidEventSourceARN) ||
		errors.Is(err, store.ErrInvalidESMBatchSize) || errors.Is(err, store.ErrESMSourceAuthz) {
		s.writeLambdaError(w, r, body, requestID, http.StatusBadRequest, "InvalidParameterValueException",
			err.Error(), readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeLambdaError(w, r, body, requestID, http.StatusInternalServerError, "ServiceException",
			"Unable to create event source mapping.", readOnly, eventID, verified)
		return
	}
	// Unit tests leave DockerHost empty: run one poll inline so Receive→Invoke→Delete is deterministic.
	if strings.TrimSpace(s.cfg.DockerHost) == "" && m.Enabled {
		_ = s.store.PollEventSourceMappingOnce(m.UUID, func(accountID, functionName, qualifier, eventJSON string) (string, error) {
			fn, executedVersion, err := s.store.ResolveFunction(accountID, functionName, qualifier)
			if err != nil {
				return "", err
			}
			out, err := s.executeLambdaInvoke(r.Context(), accountID, functionName, fn, executedVersion, eventJSON)
			return string(out), err
		})
	}
	payload, err := lambdasvc.EventSourceMappingJSON(m)
	if err != nil {
		s.writeLambdaError(w, r, body, requestID, http.StatusInternalServerError, "ServiceException",
			"Unable to build response.", readOnly, eventID, verified)
		return
	}
	s.writeLambdaOK(w, requestID, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, lambdaEventSource, "CreateEventSourceMapping", readOnly)
}

func (s *Server) lambdaGetEventSourceMapping(
	w http.ResponseWriter,
	r *http.Request,
	body []byte,
	requestID, eventID string,
	verified *authn.Verified,
	readOnly bool,
	params map[string]any,
) {
	uuidStr, _ := params["UUID"].(string)
	resource := "arn:aws:lambda:" + store.DefaultLambdaRegion + ":" + verified.AccountID + ":event-source-mapping/" + strings.TrimSpace(uuidStr)
	if !s.authorizeLambda(verified, catalog.ActionLambdaGetEventSourceMapping, resource, "") {
		s.writeLambdaError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform lambda:GetEventSourceMapping.", readOnly, eventID, verified)
		return
	}
	m, err := s.store.GetEventSourceMapping(verified.AccountID, uuidStr)
	if errors.Is(err, store.ErrNoSuchEventSourceMapping) {
		s.writeLambdaError(w, r, body, requestID, http.StatusNotFound, "ResourceNotFoundException",
			"Event source mapping not found.", readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeLambdaError(w, r, body, requestID, http.StatusInternalServerError, "ServiceException",
			"Unable to get event source mapping.", readOnly, eventID, verified)
		return
	}
	payload, err := lambdasvc.EventSourceMappingJSON(m)
	if err != nil {
		s.writeLambdaError(w, r, body, requestID, http.StatusInternalServerError, "ServiceException",
			"Unable to build response.", readOnly, eventID, verified)
		return
	}
	s.writeLambdaOK(w, requestID, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, lambdaEventSource, "GetEventSourceMapping", readOnly)
}

func (s *Server) lambdaListEventSourceMappings(
	w http.ResponseWriter,
	r *http.Request,
	body []byte,
	requestID, eventID string,
	verified *authn.Verified,
	readOnly bool,
	params map[string]any,
) {
	resource := "arn:aws:lambda:" + store.DefaultLambdaRegion + ":" + verified.AccountID + ":event-source-mapping"
	if !s.authorizeLambda(verified, catalog.ActionLambdaListEventSourceMappings, resource, "") {
		s.writeLambdaError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform lambda:ListEventSourceMappings.", readOnly, eventID, verified)
		return
	}
	fnName, _ := params["FunctionName"].(string)
	mappings, err := s.store.ListEventSourceMappings(verified.AccountID, fnName)
	if err != nil {
		s.writeLambdaError(w, r, body, requestID, http.StatusInternalServerError, "ServiceException",
			"Unable to list event source mappings.", readOnly, eventID, verified)
		return
	}
	payload, err := lambdasvc.ListEventSourceMappingsJSON(mappings)
	if err != nil {
		s.writeLambdaError(w, r, body, requestID, http.StatusInternalServerError, "ServiceException",
			"Unable to build response.", readOnly, eventID, verified)
		return
	}
	s.writeLambdaOK(w, requestID, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, lambdaEventSource, "ListEventSourceMappings", readOnly)
}

func (s *Server) lambdaUpdateEventSourceMapping(
	w http.ResponseWriter,
	r *http.Request,
	body []byte,
	requestID, eventID string,
	verified *authn.Verified,
	readOnly bool,
	params map[string]any,
) {
	uuidStr, _ := params["UUID"].(string)
	resource := "arn:aws:lambda:" + store.DefaultLambdaRegion + ":" + verified.AccountID + ":event-source-mapping/" + strings.TrimSpace(uuidStr)
	if !s.authorizeLambda(verified, catalog.ActionLambdaUpdateEventSourceMapping, resource, "") {
		s.writeLambdaError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform lambda:UpdateEventSourceMapping.", readOnly, eventID, verified)
		return
	}
	in := store.UpdateEventSourceMappingInput{
		AccountID: verified.AccountID,
		UUID:      uuidStr,
	}
	if raw, ok := params["BatchSize"]; ok {
		switch v := raw.(type) {
		case float64:
			n := int(v)
			in.BatchSize = &n
		case int:
			in.BatchSize = &v
		}
	}
	if raw, ok := params["Enabled"].(bool); ok {
		in.Enabled = &raw
	}
	if fn, ok := params["FunctionName"].(string); ok {
		in.Function = fn
	}
	if rawFC, ok := params["FilterCriteria"]; ok && rawFC != nil {
		b, mErr := json.Marshal(rawFC)
		if mErr != nil {
			s.writeLambdaError(w, r, body, requestID, http.StatusBadRequest, "InvalidParameterValueException",
				"FilterCriteria must be an object.", readOnly, eventID, verified)
			return
		}
		fc := string(b)
		in.FilterCriteriaJSON = &fc
	}
	if rawRT, ok := params["FunctionResponseTypes"]; ok && rawRT != nil {
		b, mErr := json.Marshal(rawRT)
		if mErr != nil {
			s.writeLambdaError(w, r, body, requestID, http.StatusBadRequest, "InvalidParameterValueException",
				"FunctionResponseTypes must be an array.", readOnly, eventID, verified)
			return
		}
		rt := string(b)
		in.FunctionResponseTypesJSON = &rt
	}
	m, err := s.store.UpdateEventSourceMapping(in)
	if errors.Is(err, store.ErrNoSuchEventSourceMapping) {
		s.writeLambdaError(w, r, body, requestID, http.StatusNotFound, "ResourceNotFoundException",
			"Event source mapping not found.", readOnly, eventID, verified)
		return
	}
	if errors.Is(err, store.ErrInvalidESMBatchSize) {
		s.writeLambdaError(w, r, body, requestID, http.StatusBadRequest, "InvalidParameterValueException",
			err.Error(), readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeLambdaError(w, r, body, requestID, http.StatusInternalServerError, "ServiceException",
			"Unable to update event source mapping.", readOnly, eventID, verified)
		return
	}
	payload, err := lambdasvc.EventSourceMappingJSON(m)
	if err != nil {
		s.writeLambdaError(w, r, body, requestID, http.StatusInternalServerError, "ServiceException",
			"Unable to build response.", readOnly, eventID, verified)
		return
	}
	s.writeLambdaOK(w, requestID, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, lambdaEventSource, "UpdateEventSourceMapping", readOnly)
}

func (s *Server) lambdaDeleteEventSourceMapping(
	w http.ResponseWriter,
	r *http.Request,
	body []byte,
	requestID, eventID string,
	verified *authn.Verified,
	readOnly bool,
	params map[string]any,
) {
	uuidStr, _ := params["UUID"].(string)
	resource := "arn:aws:lambda:" + store.DefaultLambdaRegion + ":" + verified.AccountID + ":event-source-mapping/" + strings.TrimSpace(uuidStr)
	if !s.authorizeLambda(verified, catalog.ActionLambdaDeleteEventSourceMapping, resource, "") {
		s.writeLambdaError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform lambda:DeleteEventSourceMapping.", readOnly, eventID, verified)
		return
	}
	m, err := s.store.GetEventSourceMapping(verified.AccountID, uuidStr)
	if errors.Is(err, store.ErrNoSuchEventSourceMapping) {
		s.writeLambdaError(w, r, body, requestID, http.StatusNotFound, "ResourceNotFoundException",
			"Event source mapping not found.", readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeLambdaError(w, r, body, requestID, http.StatusInternalServerError, "ServiceException",
			"Unable to delete event source mapping.", readOnly, eventID, verified)
		return
	}
	if err := s.store.DeleteEventSourceMapping(verified.AccountID, uuidStr); err != nil {
		s.writeLambdaError(w, r, body, requestID, http.StatusInternalServerError, "ServiceException",
			"Unable to delete event source mapping.", readOnly, eventID, verified)
		return
	}
	payload, err := lambdasvc.EventSourceMappingJSON(m)
	if err != nil {
		s.writeLambdaError(w, r, body, requestID, http.StatusInternalServerError, "ServiceException",
			"Unable to build response.", readOnly, eventID, verified)
		return
	}
	s.writeLambdaOK(w, requestID, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, lambdaEventSource, "DeleteEventSourceMapping", readOnly)
}
