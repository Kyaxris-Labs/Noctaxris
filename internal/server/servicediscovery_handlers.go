package server

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/Kyaxris-Labs/Noctaxris/internal/catalog"
	"github.com/Kyaxris-Labs/Noctaxris/internal/kernel/authn"
	sdsvc "github.com/Kyaxris-Labs/Noctaxris/internal/services/servicediscovery"
	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

const (
	sdJSONContentType = "application/x-amz-json-1.1"
	sdEventSource     = "servicediscovery.amazonaws.com"
)

func (s *Server) handleServiceDiscovery(
	w http.ResponseWriter,
	r *http.Request,
	body []byte,
	requestID, eventID, action string,
	verified *authn.Verified,
	readOnly bool,
) {
	params := jsonBodyMap(body)
	action = serviceDiscoveryAction(action)

	switch action {
	case catalog.ActionSDCreatePrivateDnsNamespace:
		s.sdCreateNamespace(w, r, body, requestID, eventID, verified, readOnly, params, "DNS_PRIVATE")
	case catalog.ActionSDCreateHttpNamespace:
		s.sdCreateNamespace(w, r, body, requestID, eventID, verified, readOnly, params, "HTTP")
	case catalog.ActionSDCreateService:
		s.sdCreateService(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionSDRegisterInstance:
		s.sdRegisterInstance(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionSDDeregisterInstance:
		s.sdDeregisterInstance(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionSDDiscoverInstances:
		s.sdDiscoverInstances(w, r, body, requestID, eventID, verified, readOnly, params)
	default:
		s.writeSDError(w, r, body, requestID, http.StatusNotImplemented, "InvalidAction",
			"This Cloud Map action is not implemented.", readOnly, eventID, verified)
	}
}

func serviceDiscoveryAction(action string) string {
	if strings.Contains(action, ":") {
		return action
	}
	switch action {
	case "CreatePrivateDnsNamespace":
		return catalog.ActionSDCreatePrivateDnsNamespace
	case "CreateHttpNamespace":
		return catalog.ActionSDCreateHttpNamespace
	case "CreateService":
		return catalog.ActionSDCreateService
	case "RegisterInstance":
		return catalog.ActionSDRegisterInstance
	case "DeregisterInstance":
		return catalog.ActionSDDeregisterInstance
	case "DiscoverInstances":
		return catalog.ActionSDDiscoverInstances
	default:
		return action
	}
}

func (s *Server) sdCreateNamespace(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any, nsType string,
) {
	name, _ := params["Name"].(string)
	desc, _ := params["Description"].(string)
	action := catalog.ActionSDCreatePrivateDnsNamespace
	if nsType == "HTTP" {
		action = catalog.ActionSDCreateHttpNamespace
	}
	if !s.authorize(verified, action, "*") {
		s.writeSDError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to create namespace.", readOnly, eventID, verified)
		return
	}
	region := verified.Region
	if region == "" {
		region = store.DefaultServiceDiscoveryRegion
	}
	ns, err := s.store.CreateSDNamespace(verified.AccountID, region, name, nsType, desc)
	if errors.Is(err, store.ErrServiceDiscoveryExists) {
		s.writeSDError(w, r, body, requestID, http.StatusBadRequest, "NamespaceAlreadyExists",
			"Namespace already exists.", readOnly, eventID, verified)
		return
	}
	if errors.Is(err, store.ErrServiceDiscoveryBadRequest) {
		s.writeSDError(w, r, body, requestID, http.StatusBadRequest, "InvalidInput",
			err.Error(), readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeSDError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to create namespace.", readOnly, eventID, verified)
		return
	}
	payload, _ := sdsvc.CreatePrivateDnsNamespaceJSON(ns)
	s.writeSDOK(w, payload)
	eventName := "CreatePrivateDnsNamespace"
	if nsType == "HTTP" {
		eventName = "CreateHttpNamespace"
	}
	s.writeSuccessAudit(r, requestID, eventID, verified, sdEventSource, eventName, readOnly)
}

func (s *Server) sdCreateService(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	name, _ := params["Name"].(string)
	nsID, _ := params["NamespaceId"].(string)
	desc, _ := params["Description"].(string)
	if !s.authorize(verified, catalog.ActionSDCreateService, "*") {
		s.writeSDError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform servicediscovery:CreateService.", readOnly, eventID, verified)
		return
	}
	region := verified.Region
	if region == "" {
		region = store.DefaultServiceDiscoveryRegion
	}
	svc, err := s.store.CreateSDService(verified.AccountID, region, nsID, name, desc)
	if errors.Is(err, store.ErrServiceDiscoveryNotFound) {
		s.writeSDError(w, r, body, requestID, http.StatusBadRequest, "NamespaceNotFound",
			"Namespace not found.", readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeSDError(w, r, body, requestID, http.StatusBadRequest, "InvalidInput",
			err.Error(), readOnly, eventID, verified)
		return
	}
	payload, _ := sdsvc.CreateServiceJSON(svc)
	s.writeSDOK(w, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, sdEventSource, "CreateService", readOnly)
}

func (s *Server) sdRegisterInstance(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	svcID, _ := params["ServiceId"].(string)
	instID, _ := params["InstanceId"].(string)
	attrs := map[string]string{}
	if raw, ok := params["Attributes"].(map[string]any); ok {
		for k, v := range raw {
			if s, ok := v.(string); ok {
				attrs[k] = s
			}
		}
	}
	if !s.authorize(verified, catalog.ActionSDRegisterInstance, "*") {
		s.writeSDError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform servicediscovery:RegisterInstance.", readOnly, eventID, verified)
		return
	}
	inst, err := s.store.RegisterSDInstance(verified.AccountID, svcID, instID, attrs)
	if errors.Is(err, store.ErrServiceDiscoveryNotFound) {
		s.writeSDError(w, r, body, requestID, http.StatusBadRequest, "ServiceNotFound",
			"Service not found.", readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeSDError(w, r, body, requestID, http.StatusBadRequest, "InvalidInput",
			err.Error(), readOnly, eventID, verified)
		return
	}
	payload, _ := sdsvc.RegisterInstanceJSON(inst)
	s.writeSDOK(w, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, sdEventSource, "RegisterInstance", readOnly)
}

func (s *Server) sdDeregisterInstance(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	svcID, _ := params["ServiceId"].(string)
	instID, _ := params["InstanceId"].(string)
	if !s.authorize(verified, catalog.ActionSDDeregisterInstance, "*") {
		s.writeSDError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform servicediscovery:DeregisterInstance.", readOnly, eventID, verified)
		return
	}
	err := s.store.DeregisterSDInstance(verified.AccountID, svcID, instID)
	if errors.Is(err, store.ErrServiceDiscoveryNotFound) {
		s.writeSDError(w, r, body, requestID, http.StatusBadRequest, "InstanceNotFound",
			"Instance not found.", readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeSDError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to deregister instance.", readOnly, eventID, verified)
		return
	}
	payload, _ := sdsvc.DeregisterInstanceJSON()
	s.writeSDOK(w, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, sdEventSource, "DeregisterInstance", readOnly)
}

func (s *Server) sdDiscoverInstances(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	nsName, _ := params["NamespaceName"].(string)
	svcName, _ := params["ServiceName"].(string)
	if !s.authorize(verified, catalog.ActionSDDiscoverInstances, "*") {
		s.writeSDError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform servicediscovery:DiscoverInstances.", readOnly, eventID, verified)
		return
	}
	instances, err := s.store.DiscoverSDInstances(verified.AccountID, nsName, svcName)
	if err != nil {
		s.writeSDError(w, r, body, requestID, http.StatusBadRequest, "InvalidInput",
			err.Error(), readOnly, eventID, verified)
		return
	}
	payload, _ := sdsvc.DiscoverInstancesJSON(instances)
	s.writeSDOK(w, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, sdEventSource, "DiscoverInstances", readOnly)
}

func (s *Server) writeSDOK(w http.ResponseWriter, payload []byte) {
	w.Header().Set("Content-Type", sdJSONContentType)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(payload)
}

func (s *Server) writeSDError(
	w http.ResponseWriter, r *http.Request, body []byte, requestID string, status int, code, message string,
	readOnly bool, eventID string, verified *authn.Verified,
) {
	w.Header().Set("Content-Type", sdJSONContentType)
	w.Header().Set("x-amzn-ErrorType", code)
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"__type": code, "message": message})
	_ = body
	s.writeSuccessAudit(r, requestID, eventID, verified, sdEventSource, code, readOnly)
}
