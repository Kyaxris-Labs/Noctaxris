package server

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/Kyaxris-Labs/Noctaxris/internal/catalog"
	"github.com/Kyaxris-Labs/Noctaxris/internal/kernel/authn"
	taggingsvc "github.com/Kyaxris-Labs/Noctaxris/internal/services/tagging"
	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

const (
	taggingJSONContentType = "application/x-amz-json-1.1"
	taggingEventSource     = "tagging.amazonaws.com"
)

func (s *Server) handleTagging(
	w http.ResponseWriter,
	r *http.Request,
	body []byte,
	requestID, eventID, action string,
	verified *authn.Verified,
	readOnly bool,
) {
	params := jsonBodyMap(body)
	action = taggingAction(action)

	switch action {
	case catalog.ActionTaggingTagResources:
		s.taggingTagResources(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionTaggingUntagResources:
		s.taggingUntagResources(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionTaggingGetResources:
		s.taggingGetResources(w, r, body, requestID, eventID, verified, readOnly, params)
	default:
		s.writeTaggingError(w, r, body, requestID, http.StatusNotImplemented, "InternalServiceException",
			"This Resource Groups Tagging API action is not implemented.", readOnly, eventID, verified)
	}
}

func taggingAction(action string) string {
	if strings.Contains(action, ":") {
		return action
	}
	switch action {
	case "TagResources":
		return catalog.ActionTaggingTagResources
	case "UntagResources":
		return catalog.ActionTaggingUntagResources
	case "GetResources":
		return catalog.ActionTaggingGetResources
	default:
		return action
	}
}

func (s *Server) taggingTagResources(
	w http.ResponseWriter,
	r *http.Request,
	body []byte,
	requestID, eventID string,
	verified *authn.Verified,
	readOnly bool,
	params map[string]any,
) {
	if !s.authorize(verified, catalog.ActionTaggingTagResources, "*") {
		s.writeTaggingError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform tag:TagResources.", readOnly, eventID, verified)
		return
	}
	arns := stringSliceParam(params["ResourceARNList"])
	tags := parseResourceGroupTagMap(params["Tags"])
	if len(arns) == 0 || len(tags) == 0 {
		s.writeTaggingError(w, r, body, requestID, http.StatusBadRequest, "InvalidParameterException",
			"ResourceARNList and Tags are required.", readOnly, eventID, verified)
		return
	}
	failed, err := s.store.TagResources(verified.AccountID, arns, tags)
	if err != nil {
		s.writeTaggingError(w, r, body, requestID, http.StatusInternalServerError, "InternalServiceException",
			"Unable to tag resources.", readOnly, eventID, verified)
		return
	}
	payload, err := taggingsvc.TagResourcesJSON(failed)
	if err != nil {
		s.writeTaggingError(w, r, body, requestID, http.StatusInternalServerError, "InternalServiceException",
			"Unable to build response.", readOnly, eventID, verified)
		return
	}
	s.writeTaggingOK(w, requestID, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, taggingEventSource, "TagResources", readOnly)
}

func (s *Server) taggingUntagResources(
	w http.ResponseWriter,
	r *http.Request,
	body []byte,
	requestID, eventID string,
	verified *authn.Verified,
	readOnly bool,
	params map[string]any,
) {
	if !s.authorize(verified, catalog.ActionTaggingUntagResources, "*") {
		s.writeTaggingError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform tag:UntagResources.", readOnly, eventID, verified)
		return
	}
	arns := stringSliceParam(params["ResourceARNList"])
	keys := stringSliceParam(params["TagKeys"])
	if len(arns) == 0 || len(keys) == 0 {
		s.writeTaggingError(w, r, body, requestID, http.StatusBadRequest, "InvalidParameterException",
			"ResourceARNList and TagKeys are required.", readOnly, eventID, verified)
		return
	}
	failed, err := s.store.UntagResources(verified.AccountID, arns, keys)
	if err != nil {
		s.writeTaggingError(w, r, body, requestID, http.StatusInternalServerError, "InternalServiceException",
			"Unable to untag resources.", readOnly, eventID, verified)
		return
	}
	payload, err := taggingsvc.UntagResourcesJSON(failed)
	if err != nil {
		s.writeTaggingError(w, r, body, requestID, http.StatusInternalServerError, "InternalServiceException",
			"Unable to build response.", readOnly, eventID, verified)
		return
	}
	s.writeTaggingOK(w, requestID, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, taggingEventSource, "UntagResources", readOnly)
}

func (s *Server) taggingGetResources(
	w http.ResponseWriter,
	r *http.Request,
	body []byte,
	requestID, eventID string,
	verified *authn.Verified,
	readOnly bool,
	params map[string]any,
) {
	if !s.authorize(verified, catalog.ActionTaggingGetResources, "*") {
		s.writeTaggingError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform tag:GetResources.", readOnly, eventID, verified)
		return
	}
	var filters []store.TagFilter
	if raw, ok := params["TagFilters"].([]any); ok {
		for _, item := range raw {
			m, _ := item.(map[string]any)
			if m == nil {
				continue
			}
			key, _ := m["Key"].(string)
			filters = append(filters, store.TagFilter{
				Key:    key,
				Values: stringSliceParam(m["Values"]),
			})
		}
	}
	typeFilters := stringSliceParam(params["ResourceTypeFilters"])
	resources, err := s.store.GetResources(verified.AccountID, filters, typeFilters)
	if err != nil {
		s.writeTaggingError(w, r, body, requestID, http.StatusInternalServerError, "InternalServiceException",
			"Unable to get resources.", readOnly, eventID, verified)
		return
	}
	payload, err := taggingsvc.GetResourcesJSON(resources)
	if err != nil {
		s.writeTaggingError(w, r, body, requestID, http.StatusInternalServerError, "InternalServiceException",
			"Unable to build response.", readOnly, eventID, verified)
		return
	}
	s.writeTaggingOK(w, requestID, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, taggingEventSource, "GetResources", readOnly)
}

func anyToString(v any) string {
	switch t := v.(type) {
	case string:
		return t
	default:
		return ""
	}
}

func (s *Server) writeTaggingOK(w http.ResponseWriter, requestID string, payload []byte) {
	w.Header().Set(requestIDHeader, requestID)
	w.Header().Set("Content-Type", taggingJSONContentType)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(payload)
}

func (s *Server) writeTaggingError(
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
	w.Header().Set("Content-Type", taggingJSONContentType)
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
