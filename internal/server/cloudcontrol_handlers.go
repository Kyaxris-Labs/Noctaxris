package server

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/Kyaxris-Labs/Noctaxris/internal/catalog"
	"github.com/Kyaxris-Labs/Noctaxris/internal/kernel/authn"
	ccsvc "github.com/Kyaxris-Labs/Noctaxris/internal/services/cloudcontrol"
	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

const (
	cloudControlJSONContentType = "application/x-amz-json-1.0"
	cloudControlEventSource     = "cloudcontrol.amazonaws.com"
)

func (s *Server) handleCloudControl(
	w http.ResponseWriter,
	r *http.Request,
	body []byte,
	requestID, eventID, action string,
	verified *authn.Verified,
	readOnly bool,
) {
	params := jsonBodyMap(body)
	action = cloudControlAction(action)

	switch action {
	case catalog.ActionCloudControlCreateResource:
		s.ccCreateResource(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionCloudControlGetResource:
		s.ccGetResource(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionCloudControlListResources:
		s.ccListResources(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionCloudControlDeleteResource:
		s.ccDeleteResource(w, r, body, requestID, eventID, verified, readOnly, params)
	default:
		s.writeCloudControlError(w, r, body, requestID, http.StatusNotImplemented, "InvalidAction",
			"This Cloud Control action is not implemented.", readOnly, eventID, verified)
	}
}

func cloudControlAction(action string) string {
	if strings.Contains(action, ":") {
		return action
	}
	switch action {
	case "CreateResource":
		return catalog.ActionCloudControlCreateResource
	case "GetResource":
		return catalog.ActionCloudControlGetResource
	case "ListResources":
		return catalog.ActionCloudControlListResources
	case "DeleteResource":
		return catalog.ActionCloudControlDeleteResource
	default:
		return action
	}
}

func (s *Server) ccCreateResource(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	typeName, _ := params["TypeName"].(string)
	desired, _ := params["DesiredState"].(string)
	if !s.authorize(verified, catalog.ActionCloudControlCreateResource, "*") {
		s.writeCloudControlError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform cloudcontrol:CreateResource.", readOnly, eventID, verified)
		return
	}
	res, err := s.store.CloudControlCreateResource(verified.AccountID, typeName, desired)
	if errors.Is(err, store.ErrCloudControlTypeUnsupported) {
		s.writeCloudControlError(w, r, body, requestID, http.StatusBadRequest, "UnsupportedActionException",
			err.Error(), readOnly, eventID, verified)
		return
	}
	if errors.Is(err, store.ErrCloudControlAlreadyExists) {
		s.writeCloudControlError(w, r, body, requestID, http.StatusBadRequest, "AlreadyExistsException",
			err.Error(), readOnly, eventID, verified)
		return
	}
	if errors.Is(err, store.ErrCloudControlBadRequest) {
		s.writeCloudControlError(w, r, body, requestID, http.StatusBadRequest, "InvalidRequestException",
			err.Error(), readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeCloudControlError(w, r, body, requestID, http.StatusInternalServerError, "HandlerInternalFailureException",
			"Unable to create resource.", readOnly, eventID, verified)
		return
	}
	payload, _ := ccsvc.ProgressEventJSON("CREATE", res.TypeName, res.Identifier, store.CloudControlRequestToken(), res.Properties)
	s.writeCloudControlOK(w, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, cloudControlEventSource, "CreateResource", readOnly)
}

func (s *Server) ccGetResource(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	typeName, _ := params["TypeName"].(string)
	identifier, _ := params["Identifier"].(string)
	if !s.authorize(verified, catalog.ActionCloudControlGetResource, "*") {
		s.writeCloudControlError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform cloudcontrol:GetResource.", readOnly, eventID, verified)
		return
	}
	res, err := s.store.CloudControlGetResource(verified.AccountID, typeName, identifier)
	if errors.Is(err, store.ErrCloudControlTypeUnsupported) {
		s.writeCloudControlError(w, r, body, requestID, http.StatusBadRequest, "UnsupportedActionException",
			err.Error(), readOnly, eventID, verified)
		return
	}
	if errors.Is(err, store.ErrCloudControlNotFound) {
		s.writeCloudControlError(w, r, body, requestID, http.StatusBadRequest, "ResourceNotFoundException",
			"Resource not found.", readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeCloudControlError(w, r, body, requestID, http.StatusInternalServerError, "HandlerInternalFailureException",
			"Unable to get resource.", readOnly, eventID, verified)
		return
	}
	payload, _ := ccsvc.GetResourceJSON(res)
	s.writeCloudControlOK(w, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, cloudControlEventSource, "GetResource", readOnly)
}

func (s *Server) ccListResources(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	typeName, _ := params["TypeName"].(string)
	if !s.authorize(verified, catalog.ActionCloudControlListResources, "*") {
		s.writeCloudControlError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform cloudcontrol:ListResources.", readOnly, eventID, verified)
		return
	}
	resources, err := s.store.CloudControlListResources(verified.AccountID, typeName)
	if errors.Is(err, store.ErrCloudControlTypeUnsupported) {
		s.writeCloudControlError(w, r, body, requestID, http.StatusBadRequest, "UnsupportedActionException",
			err.Error(), readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeCloudControlError(w, r, body, requestID, http.StatusInternalServerError, "HandlerInternalFailureException",
			"Unable to list resources.", readOnly, eventID, verified)
		return
	}
	payload, _ := ccsvc.ListResourcesJSON(typeName, resources)
	s.writeCloudControlOK(w, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, cloudControlEventSource, "ListResources", readOnly)
}

func (s *Server) ccDeleteResource(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	typeName, _ := params["TypeName"].(string)
	identifier, _ := params["Identifier"].(string)
	if !s.authorize(verified, catalog.ActionCloudControlDeleteResource, "*") {
		s.writeCloudControlError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform cloudcontrol:DeleteResource.", readOnly, eventID, verified)
		return
	}
	err := s.store.CloudControlDeleteResource(verified.AccountID, typeName, identifier)
	if errors.Is(err, store.ErrCloudControlTypeUnsupported) {
		s.writeCloudControlError(w, r, body, requestID, http.StatusBadRequest, "UnsupportedActionException",
			err.Error(), readOnly, eventID, verified)
		return
	}
	if errors.Is(err, store.ErrCloudControlNotFound) || errors.Is(err, store.ErrCloudControlBadRequest) {
		s.writeCloudControlError(w, r, body, requestID, http.StatusBadRequest, "ResourceNotFoundException",
			err.Error(), readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeCloudControlError(w, r, body, requestID, http.StatusInternalServerError, "HandlerInternalFailureException",
			"Unable to delete resource.", readOnly, eventID, verified)
		return
	}
	payload, _ := ccsvc.ProgressEventJSON("DELETE", typeName, identifier, store.CloudControlRequestToken(), "")
	s.writeCloudControlOK(w, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, cloudControlEventSource, "DeleteResource", readOnly)
}

func (s *Server) writeCloudControlOK(w http.ResponseWriter, payload []byte) {
	w.Header().Set("Content-Type", cloudControlJSONContentType)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(payload)
}

func (s *Server) writeCloudControlError(
	w http.ResponseWriter, r *http.Request, body []byte, requestID string, status int, code, message string,
	readOnly bool, eventID string, verified *authn.Verified,
) {
	w.Header().Set("Content-Type", cloudControlJSONContentType)
	w.Header().Set("x-amzn-ErrorType", code)
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"__type": code, "message": message})
	_ = body
	s.writeSuccessAudit(r, requestID, eventID, verified, cloudControlEventSource, code, readOnly)
}
