package server

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/Kyaxris-Labs/Noctaxris/internal/catalog"
	"github.com/Kyaxris-Labs/Noctaxris/internal/kernel/authn"
	lightsailsvc "github.com/Kyaxris-Labs/Noctaxris/internal/services/lightsail"
	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

const (
	lightsailJSONContentType = "application/x-amz-json-1.1"
	lightsailEventSource     = "lightsail.amazonaws.com"
)

func (s *Server) handleLightsail(
	w http.ResponseWriter,
	r *http.Request,
	body []byte,
	requestID, eventID, action string,
	verified *authn.Verified,
	readOnly bool,
) {
	params := jsonBodyMap(body)
	action = lightsailAction(action)

	switch action {
	case catalog.ActionLightsailGetBlueprints:
		s.lightsailGetBlueprints(w, r, body, requestID, eventID, verified, readOnly)
	case catalog.ActionLightsailGetBundles:
		s.lightsailGetBundles(w, r, body, requestID, eventID, verified, readOnly)
	case catalog.ActionLightsailCreateInstances:
		s.lightsailCreateInstances(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionLightsailGetInstance:
		s.lightsailGetInstance(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionLightsailGetInstances:
		s.lightsailGetInstances(w, r, body, requestID, eventID, verified, readOnly)
	case catalog.ActionLightsailStartInstance:
		s.lightsailMutateInstance(w, r, body, requestID, eventID, verified, readOnly, params, "start")
	case catalog.ActionLightsailStopInstance:
		s.lightsailMutateInstance(w, r, body, requestID, eventID, verified, readOnly, params, "stop")
	case catalog.ActionLightsailRebootInstance:
		s.lightsailMutateInstance(w, r, body, requestID, eventID, verified, readOnly, params, "reboot")
	case catalog.ActionLightsailDeleteInstance:
		s.lightsailMutateInstance(w, r, body, requestID, eventID, verified, readOnly, params, "delete")
	default:
		s.writeLightsailError(w, r, body, requestID, http.StatusNotImplemented, "UnsupportedOperationException",
			"This Lightsail action is not implemented.", readOnly, eventID, verified)
	}
}

func lightsailAction(action string) string {
	if strings.Contains(action, ":") {
		return action
	}
	switch action {
	case "GetBlueprints":
		return catalog.ActionLightsailGetBlueprints
	case "GetBundles":
		return catalog.ActionLightsailGetBundles
	case "CreateInstances":
		return catalog.ActionLightsailCreateInstances
	case "GetInstance":
		return catalog.ActionLightsailGetInstance
	case "GetInstances":
		return catalog.ActionLightsailGetInstances
	case "StartInstance":
		return catalog.ActionLightsailStartInstance
	case "StopInstance":
		return catalog.ActionLightsailStopInstance
	case "RebootInstance":
		return catalog.ActionLightsailRebootInstance
	case "DeleteInstance":
		return catalog.ActionLightsailDeleteInstance
	default:
		return action
	}
}

func (s *Server) lightsailRegion(verified *authn.Verified) string {
	if verified.Region != "" {
		return verified.Region
	}
	return store.DefaultLightsailRegion
}

func lightsailStringList(params map[string]any, keys ...string) []string {
	for _, key := range keys {
		if raw, ok := params[key]; ok {
			switch v := raw.(type) {
			case []any:
				out := make([]string, 0, len(v))
				for _, item := range v {
					if s, ok := item.(string); ok && strings.TrimSpace(s) != "" {
						out = append(out, s)
					}
				}
				return out
			case []string:
				return v
			}
		}
	}
	return nil
}

func lightsailString(params map[string]any, keys ...string) string {
	for _, key := range keys {
		if v, ok := params[key].(string); ok && strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

func (s *Server) lightsailGetBlueprints(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool,
) {
	if !s.authorize(verified, catalog.ActionLightsailGetBlueprints, "*") {
		s.writeLightsailError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform lightsail:GetBlueprints.", readOnly, eventID, verified)
		return
	}
	payload, _ := lightsailsvc.GetBlueprintsJSON()
	s.writeLightsailOK(w, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, lightsailEventSource, "GetBlueprints", readOnly)
}

func (s *Server) lightsailGetBundles(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool,
) {
	if !s.authorize(verified, catalog.ActionLightsailGetBundles, "*") {
		s.writeLightsailError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform lightsail:GetBundles.", readOnly, eventID, verified)
		return
	}
	payload, _ := lightsailsvc.GetBundlesJSON()
	s.writeLightsailOK(w, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, lightsailEventSource, "GetBundles", readOnly)
}

func (s *Server) lightsailCreateInstances(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	if !s.authorize(verified, catalog.ActionLightsailCreateInstances, "*") {
		s.writeLightsailError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform lightsail:CreateInstances.", readOnly, eventID, verified)
		return
	}
	names := lightsailStringList(params, "instanceNames", "InstanceNames")
	az := lightsailString(params, "availabilityZone", "AvailabilityZone")
	blueprintID := lightsailString(params, "blueprintId", "BlueprintId")
	bundleID := lightsailString(params, "bundleId", "BundleId")
	instances, err := s.store.CreateLightsailInstances(verified.AccountID, s.lightsailRegion(verified), names, az, blueprintID, bundleID)
	if errors.Is(err, store.ErrLightsailExists) {
		s.writeLightsailError(w, r, body, requestID, http.StatusBadRequest, "InvalidResourceNameException",
			err.Error(), readOnly, eventID, verified)
		return
	}
	if errors.Is(err, store.ErrLightsailBadRequest) {
		s.writeLightsailError(w, r, body, requestID, http.StatusBadRequest, "InvalidInputException",
			err.Error(), readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeLightsailError(w, r, body, requestID, http.StatusInternalServerError, "ServiceException",
			"Unable to create instances.", readOnly, eventID, verified)
		return
	}
	payload, _ := lightsailsvc.CreateInstancesJSON(instances)
	s.writeLightsailOK(w, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, lightsailEventSource, "CreateInstances", readOnly)
}

func (s *Server) lightsailGetInstance(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	name := lightsailString(params, "instanceName", "InstanceName")
	if !s.authorize(verified, catalog.ActionLightsailGetInstance, "*") {
		s.writeLightsailError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform lightsail:GetInstance.", readOnly, eventID, verified)
		return
	}
	inst, err := s.store.GetLightsailInstance(verified.AccountID, s.lightsailRegion(verified), name)
	if errors.Is(err, store.ErrLightsailNotFound) {
		s.writeLightsailError(w, r, body, requestID, http.StatusBadRequest, "NotFoundException",
			"Instance not found.", readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeLightsailError(w, r, body, requestID, http.StatusInternalServerError, "ServiceException",
			"Unable to get instance.", readOnly, eventID, verified)
		return
	}
	payload, _ := lightsailsvc.GetInstanceJSON(inst)
	s.writeLightsailOK(w, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, lightsailEventSource, "GetInstance", readOnly)
}

func (s *Server) lightsailGetInstances(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool,
) {
	if !s.authorize(verified, catalog.ActionLightsailGetInstances, "*") {
		s.writeLightsailError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform lightsail:GetInstances.", readOnly, eventID, verified)
		return
	}
	list, err := s.store.GetLightsailInstances(verified.AccountID, s.lightsailRegion(verified))
	if err != nil {
		s.writeLightsailError(w, r, body, requestID, http.StatusInternalServerError, "ServiceException",
			"Unable to list instances.", readOnly, eventID, verified)
		return
	}
	payload, _ := lightsailsvc.GetInstancesJSON(list)
	s.writeLightsailOK(w, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, lightsailEventSource, "GetInstances", readOnly)
}

func (s *Server) lightsailMutateInstance(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any, op string,
) {
	name := lightsailString(params, "instanceName", "InstanceName")
	var (
		action string
		opType string
		err    error
		inst   store.LightsailInstance
	)
	switch op {
	case "start":
		action, opType = catalog.ActionLightsailStartInstance, "StartInstance"
		if !s.authorize(verified, action, "*") {
			s.writeLightsailError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
				"User is not authorized to perform lightsail:StartInstance.", readOnly, eventID, verified)
			return
		}
		inst, err = s.store.StartLightsailInstance(verified.AccountID, s.lightsailRegion(verified), name)
	case "stop":
		action, opType = catalog.ActionLightsailStopInstance, "StopInstance"
		if !s.authorize(verified, action, "*") {
			s.writeLightsailError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
				"User is not authorized to perform lightsail:StopInstance.", readOnly, eventID, verified)
			return
		}
		inst, err = s.store.StopLightsailInstance(verified.AccountID, s.lightsailRegion(verified), name)
	case "reboot":
		action, opType = catalog.ActionLightsailRebootInstance, "RebootInstance"
		if !s.authorize(verified, action, "*") {
			s.writeLightsailError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
				"User is not authorized to perform lightsail:RebootInstance.", readOnly, eventID, verified)
			return
		}
		inst, err = s.store.RebootLightsailInstance(verified.AccountID, s.lightsailRegion(verified), name)
	case "delete":
		action, opType = catalog.ActionLightsailDeleteInstance, "DeleteInstance"
		if !s.authorize(verified, action, "*") {
			s.writeLightsailError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
				"User is not authorized to perform lightsail:DeleteInstance.", readOnly, eventID, verified)
			return
		}
		inst, err = s.store.GetLightsailInstance(verified.AccountID, s.lightsailRegion(verified), name)
		if err == nil {
			err = s.store.DeleteLightsailInstance(verified.AccountID, s.lightsailRegion(verified), name)
		}
	default:
		s.writeLightsailError(w, r, body, requestID, http.StatusBadRequest, "InvalidInputException",
			"Unknown instance operation.", readOnly, eventID, verified)
		return
	}
	if errors.Is(err, store.ErrLightsailNotFound) {
		s.writeLightsailError(w, r, body, requestID, http.StatusBadRequest, "NotFoundException",
			"Instance not found.", readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeLightsailError(w, r, body, requestID, http.StatusInternalServerError, "ServiceException",
			"Unable to update instance.", readOnly, eventID, verified)
		return
	}
	payload, _ := lightsailsvc.InstanceOperationJSON(inst, opType)
	s.writeLightsailOK(w, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, lightsailEventSource, opType, readOnly)
}

func (s *Server) writeLightsailOK(w http.ResponseWriter, payload []byte) {
	w.Header().Set("Content-Type", lightsailJSONContentType)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(payload)
}

func (s *Server) writeLightsailError(
	w http.ResponseWriter, r *http.Request, body []byte, requestID string, status int, code, message string,
	readOnly bool, eventID string, verified *authn.Verified,
) {
	w.Header().Set("Content-Type", lightsailJSONContentType)
	w.Header().Set("x-amzn-ErrorType", code)
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"__type": code, "message": message})
	_ = body
	s.writeSuccessAudit(r, requestID, eventID, verified, lightsailEventSource, code, readOnly)
}
