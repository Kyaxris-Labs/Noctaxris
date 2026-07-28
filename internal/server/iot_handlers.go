package server

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strings"

	"github.com/Kyaxris-Labs/Noctaxris/internal/catalog"
	"github.com/Kyaxris-Labs/Noctaxris/internal/kernel/authn"
	iotsvc "github.com/Kyaxris-Labs/Noctaxris/internal/services/iot"
	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

const (
	iotJSONContentType = "application/x-amz-json-1.1"
	iotEventSource     = "iot.amazonaws.com"
	iotDataEventSource = "iotdata.amazonaws.com"
)

func (s *Server) handleIoT(
	w http.ResponseWriter,
	r *http.Request,
	body []byte,
	requestID, eventID, action string,
	verified *authn.Verified,
	readOnly bool,
) {
	params := jsonBodyMap(body)
	action = iotAction(action)

	switch action {
	case catalog.ActionIoTCreateThing:
		s.iotCreateThing(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionIoTDescribeThing:
		s.iotDescribeThing(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionIoTListThings:
		s.iotListThings(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionIoTUpdateThing:
		s.iotUpdateThing(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionIoTDeleteThing:
		s.iotDeleteThing(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionIoTCreateKeysAndCertificate:
		s.iotCreateKeysAndCertificate(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionIoTDescribeCertificate:
		s.iotDescribeCertificate(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionIoTListCertificates:
		s.iotListCertificates(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionIoTUpdateCertificate:
		s.iotUpdateCertificate(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionIoTDeleteCertificate:
		s.iotDeleteCertificate(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionIoTCreatePolicy:
		s.iotCreatePolicy(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionIoTGetPolicy:
		s.iotGetPolicy(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionIoTListPolicies:
		s.iotListPolicies(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionIoTDeletePolicy:
		s.iotDeletePolicy(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionIoTAttachPolicy:
		s.iotAttachPolicy(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionIoTDetachPolicy:
		s.iotDetachPolicy(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionIoTAttachThingPrincipal:
		s.iotAttachThingPrincipal(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionIoTListThingPrincipals:
		s.iotListThingPrincipals(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionIoTDataUpdateThingShadow:
		s.iotUpdateThingShadow(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionIoTDataGetThingShadow:
		s.iotGetThingShadow(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionIoTDataDeleteThingShadow:
		s.iotDeleteThingShadow(w, r, body, requestID, eventID, verified, readOnly, params)
	default:
		s.writeIoTError(w, r, body, requestID, http.StatusNotImplemented, "InvalidAction",
			"This IoT action is not implemented.", readOnly, eventID, verified)
	}
}

func iotAction(action string) string {
	if strings.Contains(action, ":") {
		return action
	}
	switch action {
	case "CreateThing":
		return catalog.ActionIoTCreateThing
	case "DescribeThing":
		return catalog.ActionIoTDescribeThing
	case "ListThings":
		return catalog.ActionIoTListThings
	case "UpdateThing":
		return catalog.ActionIoTUpdateThing
	case "DeleteThing":
		return catalog.ActionIoTDeleteThing
	case "CreateKeysAndCertificate":
		return catalog.ActionIoTCreateKeysAndCertificate
	case "DescribeCertificate":
		return catalog.ActionIoTDescribeCertificate
	case "ListCertificates":
		return catalog.ActionIoTListCertificates
	case "UpdateCertificate":
		return catalog.ActionIoTUpdateCertificate
	case "DeleteCertificate":
		return catalog.ActionIoTDeleteCertificate
	case "CreatePolicy":
		return catalog.ActionIoTCreatePolicy
	case "GetPolicy":
		return catalog.ActionIoTGetPolicy
	case "ListPolicies":
		return catalog.ActionIoTListPolicies
	case "DeletePolicy":
		return catalog.ActionIoTDeletePolicy
	case "AttachPolicy":
		return catalog.ActionIoTAttachPolicy
	case "DetachPolicy":
		return catalog.ActionIoTDetachPolicy
	case "AttachThingPrincipal":
		return catalog.ActionIoTAttachThingPrincipal
	case "ListThingPrincipals":
		return catalog.ActionIoTListThingPrincipals
	case "UpdateThingShadow":
		return catalog.ActionIoTDataUpdateThingShadow
	case "GetThingShadow":
		return catalog.ActionIoTDataGetThingShadow
	case "DeleteThingShadow":
		return catalog.ActionIoTDataDeleteThingShadow
	default:
		return action
	}
}

func (s *Server) iotRegion(verified *authn.Verified) string {
	if verified.Region != "" {
		return verified.Region
	}
	return store.DefaultIoTRegion
}

func stringMapFromAny(v any) map[string]string {
	m, ok := v.(map[string]any)
	if !ok {
		return map[string]string{}
	}
	out := make(map[string]string, len(m))
	for k, raw := range m {
		if s, ok := raw.(string); ok {
			out[k] = s
		}
	}
	return out
}

func (s *Server) iotCreateThing(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	if !s.authorize(verified, catalog.ActionIoTCreateThing, "*") {
		s.writeIoTError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform iot:CreateThing.", readOnly, eventID, verified)
		return
	}
	name, _ := params["thingName"].(string)
	if name == "" {
		name, _ = params["ThingName"].(string)
	}
	attrs := map[string]string{}
	if ap, ok := params["attributePayload"].(map[string]any); ok {
		attrs = stringMapFromAny(ap["attributes"])
		if len(attrs) == 0 {
			attrs = stringMapFromAny(ap["Attributes"])
		}
	}
	if ap, ok := params["AttributePayload"].(map[string]any); ok {
		attrs = stringMapFromAny(ap["attributes"])
		if len(attrs) == 0 {
			attrs = stringMapFromAny(ap["Attributes"])
		}
	}
	thing, err := s.store.CreateIoTThing(verified.AccountID, s.iotRegion(verified), name, attrs)
	if errors.Is(err, store.ErrIoTConflict) {
		s.writeIoTError(w, r, body, requestID, http.StatusConflict, "ResourceAlreadyExistsException",
			"Thing already exists with different attributes.", readOnly, eventID, verified)
		return
	}
	if errors.Is(err, store.ErrIoTBadRequest) {
		s.writeIoTError(w, r, body, requestID, http.StatusBadRequest, "InvalidRequestException",
			err.Error(), readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeIoTError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailureException",
			"Unable to create thing.", readOnly, eventID, verified)
		return
	}
	payload, _ := iotsvc.CreateThingJSON(thing)
	s.writeIoTOK(w, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, iotEventSource, "CreateThing", readOnly)
}

func (s *Server) iotDescribeThing(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	if !s.authorize(verified, catalog.ActionIoTDescribeThing, "*") {
		s.writeIoTError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform iot:DescribeThing.", readOnly, eventID, verified)
		return
	}
	name, _ := params["thingName"].(string)
	if name == "" {
		name, _ = params["ThingName"].(string)
	}
	thing, err := s.store.DescribeIoTThing(verified.AccountID, s.iotRegion(verified), name)
	if errors.Is(err, store.ErrIoTNotFound) {
		s.writeIoTError(w, r, body, requestID, http.StatusNotFound, "ResourceNotFoundException",
			"Thing not found.", readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeIoTError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailureException",
			"Unable to describe thing.", readOnly, eventID, verified)
		return
	}
	payload, _ := iotsvc.DescribeThingJSON(thing)
	s.writeIoTOK(w, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, iotEventSource, "DescribeThing", readOnly)
}

func (s *Server) iotListThings(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	_ = params
	if !s.authorize(verified, catalog.ActionIoTListThings, "*") {
		s.writeIoTError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform iot:ListThings.", readOnly, eventID, verified)
		return
	}
	list, err := s.store.ListIoTThings(verified.AccountID, s.iotRegion(verified))
	if err != nil {
		s.writeIoTError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailureException",
			"Unable to list things.", readOnly, eventID, verified)
		return
	}
	payload, _ := iotsvc.ListThingsJSON(list)
	s.writeIoTOK(w, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, iotEventSource, "ListThings", readOnly)
}

func (s *Server) iotUpdateThing(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	if !s.authorize(verified, catalog.ActionIoTUpdateThing, "*") {
		s.writeIoTError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform iot:UpdateThing.", readOnly, eventID, verified)
		return
	}
	name, _ := params["thingName"].(string)
	if name == "" {
		name, _ = params["ThingName"].(string)
	}
	attrs := map[string]string{}
	if ap, ok := params["attributePayload"].(map[string]any); ok {
		attrs = stringMapFromAny(ap["attributes"])
	}
	if ap, ok := params["AttributePayload"].(map[string]any); ok {
		attrs = stringMapFromAny(ap["attributes"])
		if len(attrs) == 0 {
			attrs = stringMapFromAny(ap["Attributes"])
		}
	}
	var expected *int64
	if v, ok := params["expectedVersion"].(float64); ok {
		n := int64(v)
		expected = &n
	}
	thing, err := s.store.UpdateIoTThing(verified.AccountID, s.iotRegion(verified), name, attrs, expected)
	if errors.Is(err, store.ErrIoTNotFound) {
		s.writeIoTError(w, r, body, requestID, http.StatusNotFound, "ResourceNotFoundException",
			"Thing not found.", readOnly, eventID, verified)
		return
	}
	if errors.Is(err, store.ErrIoTVersionConflict) {
		s.writeIoTError(w, r, body, requestID, http.StatusConflict, "VersionConflictException",
			"Expected version mismatch.", readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeIoTError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailureException",
			"Unable to update thing.", readOnly, eventID, verified)
		return
	}
	payload, _ := iotsvc.UpdateThingJSON(thing)
	s.writeIoTOK(w, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, iotEventSource, "UpdateThing", readOnly)
}

func (s *Server) iotDeleteThing(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	if !s.authorize(verified, catalog.ActionIoTDeleteThing, "*") {
		s.writeIoTError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform iot:DeleteThing.", readOnly, eventID, verified)
		return
	}
	name, _ := params["thingName"].(string)
	if name == "" {
		name, _ = params["ThingName"].(string)
	}
	err := s.store.DeleteIoTThing(verified.AccountID, s.iotRegion(verified), name)
	if errors.Is(err, store.ErrIoTNotFound) {
		s.writeIoTError(w, r, body, requestID, http.StatusNotFound, "ResourceNotFoundException",
			"Thing not found.", readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeIoTError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailureException",
			"Unable to delete thing.", readOnly, eventID, verified)
		return
	}
	payload, _ := iotsvc.EmptyJSON()
	s.writeIoTOK(w, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, iotEventSource, "DeleteThing", readOnly)
}

func (s *Server) iotCreateKeysAndCertificate(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	if !s.authorize(verified, catalog.ActionIoTCreateKeysAndCertificate, "*") {
		s.writeIoTError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform iot:CreateKeysAndCertificate.", readOnly, eventID, verified)
		return
	}
	setActive := false
	if v, ok := params["setAsActive"].(bool); ok {
		setActive = v
	}
	if v, ok := params["SetAsActive"].(bool); ok {
		setActive = v
	}
	cert, err := s.store.CreateIoTKeysAndCertificate(verified.AccountID, s.iotRegion(verified), setActive)
	if err != nil {
		s.writeIoTError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailureException",
			"Unable to create keys and certificate.", readOnly, eventID, verified)
		return
	}
	if s.cfg.SharedMQTT {
		if err := tryEnsureSharedMQTT(s); err != nil {
			log.Printf("shared mqtt ensure: %v", err)
		}
	}
	payload, _ := iotsvc.CreateKeysAndCertificateJSON(cert)
	s.writeIoTOK(w, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, iotEventSource, "CreateKeysAndCertificate", readOnly)
}

func (s *Server) iotDescribeCertificate(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	if !s.authorize(verified, catalog.ActionIoTDescribeCertificate, "*") {
		s.writeIoTError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform iot:DescribeCertificate.", readOnly, eventID, verified)
		return
	}
	id, _ := params["certificateId"].(string)
	if id == "" {
		id, _ = params["CertificateId"].(string)
	}
	cert, err := s.store.DescribeIoTCertificate(verified.AccountID, s.iotRegion(verified), id)
	if errors.Is(err, store.ErrIoTNotFound) {
		s.writeIoTError(w, r, body, requestID, http.StatusNotFound, "ResourceNotFoundException",
			"Certificate not found.", readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeIoTError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailureException",
			"Unable to describe certificate.", readOnly, eventID, verified)
		return
	}
	payload, _ := iotsvc.DescribeCertificateJSON(cert)
	s.writeIoTOK(w, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, iotEventSource, "DescribeCertificate", readOnly)
}

func (s *Server) iotListCertificates(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	_ = params
	if !s.authorize(verified, catalog.ActionIoTListCertificates, "*") {
		s.writeIoTError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform iot:ListCertificates.", readOnly, eventID, verified)
		return
	}
	list, err := s.store.ListIoTCertificates(verified.AccountID, s.iotRegion(verified))
	if err != nil {
		s.writeIoTError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailureException",
			"Unable to list certificates.", readOnly, eventID, verified)
		return
	}
	payload, _ := iotsvc.ListCertificatesJSON(list)
	s.writeIoTOK(w, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, iotEventSource, "ListCertificates", readOnly)
}

func (s *Server) iotUpdateCertificate(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	if !s.authorize(verified, catalog.ActionIoTUpdateCertificate, "*") {
		s.writeIoTError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform iot:UpdateCertificate.", readOnly, eventID, verified)
		return
	}
	id, _ := params["certificateId"].(string)
	if id == "" {
		id, _ = params["CertificateId"].(string)
	}
	status, _ := params["newStatus"].(string)
	if status == "" {
		status, _ = params["NewStatus"].(string)
	}
	err := s.store.UpdateIoTCertificate(verified.AccountID, s.iotRegion(verified), id, status)
	if errors.Is(err, store.ErrIoTNotFound) {
		s.writeIoTError(w, r, body, requestID, http.StatusNotFound, "ResourceNotFoundException",
			"Certificate not found.", readOnly, eventID, verified)
		return
	}
	if errors.Is(err, store.ErrIoTBadRequest) {
		s.writeIoTError(w, r, body, requestID, http.StatusBadRequest, "InvalidRequestException",
			err.Error(), readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeIoTError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailureException",
			"Unable to update certificate.", readOnly, eventID, verified)
		return
	}
	payload, _ := iotsvc.EmptyJSON()
	s.writeIoTOK(w, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, iotEventSource, "UpdateCertificate", readOnly)
}

func (s *Server) iotDeleteCertificate(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	if !s.authorize(verified, catalog.ActionIoTDeleteCertificate, "*") {
		s.writeIoTError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform iot:DeleteCertificate.", readOnly, eventID, verified)
		return
	}
	id, _ := params["certificateId"].(string)
	if id == "" {
		id, _ = params["CertificateId"].(string)
	}
	err := s.store.DeleteIoTCertificate(verified.AccountID, s.iotRegion(verified), id)
	if errors.Is(err, store.ErrIoTNotFound) {
		s.writeIoTError(w, r, body, requestID, http.StatusNotFound, "ResourceNotFoundException",
			"Certificate not found.", readOnly, eventID, verified)
		return
	}
	if errors.Is(err, store.ErrIoTDeleteConflict) {
		s.writeIoTError(w, r, body, requestID, http.StatusConflict, "DeleteConflictException",
			err.Error(), readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeIoTError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailureException",
			"Unable to delete certificate.", readOnly, eventID, verified)
		return
	}
	payload, _ := iotsvc.EmptyJSON()
	s.writeIoTOK(w, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, iotEventSource, "DeleteCertificate", readOnly)
}

func (s *Server) iotCreatePolicy(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	if !s.authorize(verified, catalog.ActionIoTCreatePolicy, "*") {
		s.writeIoTError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform iot:CreatePolicy.", readOnly, eventID, verified)
		return
	}
	name, _ := params["policyName"].(string)
	if name == "" {
		name, _ = params["PolicyName"].(string)
	}
	doc, _ := params["policyDocument"].(string)
	if doc == "" {
		doc, _ = params["PolicyDocument"].(string)
	}
	pol, err := s.store.CreateIoTPolicy(verified.AccountID, s.iotRegion(verified), name, doc)
	if errors.Is(err, store.ErrIoTConflict) {
		s.writeIoTError(w, r, body, requestID, http.StatusConflict, "ResourceAlreadyExistsException",
			"Policy already exists.", readOnly, eventID, verified)
		return
	}
	if errors.Is(err, store.ErrIoTBadRequest) {
		s.writeIoTError(w, r, body, requestID, http.StatusBadRequest, "InvalidRequestException",
			err.Error(), readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeIoTError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailureException",
			"Unable to create policy.", readOnly, eventID, verified)
		return
	}
	payload, _ := iotsvc.CreatePolicyJSON(pol)
	s.writeIoTOK(w, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, iotEventSource, "CreatePolicy", readOnly)
}

func (s *Server) iotGetPolicy(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	if !s.authorize(verified, catalog.ActionIoTGetPolicy, "*") {
		s.writeIoTError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform iot:GetPolicy.", readOnly, eventID, verified)
		return
	}
	name, _ := params["policyName"].(string)
	if name == "" {
		name, _ = params["PolicyName"].(string)
	}
	pol, err := s.store.GetIoTPolicy(verified.AccountID, s.iotRegion(verified), name)
	if errors.Is(err, store.ErrIoTNotFound) {
		s.writeIoTError(w, r, body, requestID, http.StatusNotFound, "ResourceNotFoundException",
			"Policy not found.", readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeIoTError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailureException",
			"Unable to get policy.", readOnly, eventID, verified)
		return
	}
	payload, _ := iotsvc.GetPolicyJSON(pol)
	s.writeIoTOK(w, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, iotEventSource, "GetPolicy", readOnly)
}

func (s *Server) iotListPolicies(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	_ = params
	if !s.authorize(verified, catalog.ActionIoTListPolicies, "*") {
		s.writeIoTError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform iot:ListPolicies.", readOnly, eventID, verified)
		return
	}
	list, err := s.store.ListIoTPolicies(verified.AccountID, s.iotRegion(verified))
	if err != nil {
		s.writeIoTError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailureException",
			"Unable to list policies.", readOnly, eventID, verified)
		return
	}
	payload, _ := iotsvc.ListPoliciesJSON(list)
	s.writeIoTOK(w, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, iotEventSource, "ListPolicies", readOnly)
}

func (s *Server) iotDeletePolicy(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	if !s.authorize(verified, catalog.ActionIoTDeletePolicy, "*") {
		s.writeIoTError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform iot:DeletePolicy.", readOnly, eventID, verified)
		return
	}
	name, _ := params["policyName"].(string)
	if name == "" {
		name, _ = params["PolicyName"].(string)
	}
	err := s.store.DeleteIoTPolicy(verified.AccountID, s.iotRegion(verified), name)
	if errors.Is(err, store.ErrIoTNotFound) {
		s.writeIoTError(w, r, body, requestID, http.StatusNotFound, "ResourceNotFoundException",
			"Policy not found.", readOnly, eventID, verified)
		return
	}
	if errors.Is(err, store.ErrIoTDeleteConflict) {
		s.writeIoTError(w, r, body, requestID, http.StatusConflict, "DeleteConflictException",
			err.Error(), readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeIoTError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailureException",
			"Unable to delete policy.", readOnly, eventID, verified)
		return
	}
	payload, _ := iotsvc.EmptyJSON()
	s.writeIoTOK(w, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, iotEventSource, "DeletePolicy", readOnly)
}

func (s *Server) iotAttachPolicy(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	if !s.authorize(verified, catalog.ActionIoTAttachPolicy, "*") {
		s.writeIoTError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform iot:AttachPolicy.", readOnly, eventID, verified)
		return
	}
	name, _ := params["policyName"].(string)
	if name == "" {
		name, _ = params["PolicyName"].(string)
	}
	target, _ := params["target"].(string)
	if target == "" {
		target, _ = params["Target"].(string)
	}
	err := s.store.AttachIoTPolicy(verified.AccountID, s.iotRegion(verified), name, target)
	if errors.Is(err, store.ErrIoTNotFound) {
		s.writeIoTError(w, r, body, requestID, http.StatusNotFound, "ResourceNotFoundException",
			"Policy not found.", readOnly, eventID, verified)
		return
	}
	if errors.Is(err, store.ErrIoTBadRequest) {
		s.writeIoTError(w, r, body, requestID, http.StatusBadRequest, "InvalidRequestException",
			err.Error(), readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeIoTError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailureException",
			"Unable to attach policy.", readOnly, eventID, verified)
		return
	}
	payload, _ := iotsvc.EmptyJSON()
	s.writeIoTOK(w, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, iotEventSource, "AttachPolicy", readOnly)
}

func (s *Server) iotDetachPolicy(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	if !s.authorize(verified, catalog.ActionIoTDetachPolicy, "*") {
		s.writeIoTError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform iot:DetachPolicy.", readOnly, eventID, verified)
		return
	}
	name, _ := params["policyName"].(string)
	if name == "" {
		name, _ = params["PolicyName"].(string)
	}
	target, _ := params["target"].(string)
	if target == "" {
		target, _ = params["Target"].(string)
	}
	if err := s.store.DetachIoTPolicy(verified.AccountID, s.iotRegion(verified), name, target); err != nil {
		s.writeIoTError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailureException",
			"Unable to detach policy.", readOnly, eventID, verified)
		return
	}
	payload, _ := iotsvc.EmptyJSON()
	s.writeIoTOK(w, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, iotEventSource, "DetachPolicy", readOnly)
}

func (s *Server) iotAttachThingPrincipal(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	if !s.authorize(verified, catalog.ActionIoTAttachThingPrincipal, "*") {
		s.writeIoTError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform iot:AttachThingPrincipal.", readOnly, eventID, verified)
		return
	}
	name, _ := params["thingName"].(string)
	if name == "" {
		name, _ = params["ThingName"].(string)
	}
	principal, _ := params["principal"].(string)
	if principal == "" {
		principal, _ = params["Principal"].(string)
	}
	err := s.store.AttachIoTThingPrincipal(verified.AccountID, s.iotRegion(verified), name, principal)
	if errors.Is(err, store.ErrIoTNotFound) {
		s.writeIoTError(w, r, body, requestID, http.StatusNotFound, "ResourceNotFoundException",
			"Thing not found.", readOnly, eventID, verified)
		return
	}
	if errors.Is(err, store.ErrIoTBadRequest) {
		s.writeIoTError(w, r, body, requestID, http.StatusBadRequest, "InvalidRequestException",
			err.Error(), readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeIoTError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailureException",
			"Unable to attach thing principal.", readOnly, eventID, verified)
		return
	}
	payload, _ := iotsvc.EmptyJSON()
	s.writeIoTOK(w, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, iotEventSource, "AttachThingPrincipal", readOnly)
}

func (s *Server) iotListThingPrincipals(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	if !s.authorize(verified, catalog.ActionIoTListThingPrincipals, "*") {
		s.writeIoTError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform iot:ListThingPrincipals.", readOnly, eventID, verified)
		return
	}
	name, _ := params["thingName"].(string)
	if name == "" {
		name, _ = params["ThingName"].(string)
	}
	list, err := s.store.ListIoTThingPrincipals(verified.AccountID, s.iotRegion(verified), name)
	if errors.Is(err, store.ErrIoTNotFound) {
		s.writeIoTError(w, r, body, requestID, http.StatusNotFound, "ResourceNotFoundException",
			"Thing not found.", readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeIoTError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailureException",
			"Unable to list thing principals.", readOnly, eventID, verified)
		return
	}
	payload, _ := iotsvc.ListThingPrincipalsJSON(list)
	s.writeIoTOK(w, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, iotEventSource, "ListThingPrincipals", readOnly)
}

func (s *Server) iotUpdateThingShadow(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	if !s.authorize(verified, catalog.ActionIoTDataUpdateThingShadow, "*") {
		s.writeIoTError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform iot:UpdateThingShadow.", readOnly, eventID, verified)
		return
	}
	name, _ := params["thingName"].(string)
	if name == "" {
		name, _ = params["ThingName"].(string)
	}
	shadowName, _ := params["shadowName"].(string)
	if shadowName == "" {
		shadowName, _ = params["ShadowName"].(string)
	}
	payloadMap := params
	if raw, ok := params["payload"].(string); ok && raw != "" {
		_ = json.Unmarshal([]byte(raw), &payloadMap)
	} else if nested, ok := params["payload"].(map[string]any); ok {
		payloadMap = nested
	}
	sh, err := s.store.UpdateIoTThingShadow(verified.AccountID, s.iotRegion(verified), name, shadowName, payloadMap)
	if errors.Is(err, store.ErrIoTNotFound) {
		s.writeIoTError(w, r, body, requestID, http.StatusNotFound, "ResourceNotFoundException",
			"Thing not found.", readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeIoTError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailureException",
			"Unable to update thing shadow.", readOnly, eventID, verified)
		return
	}
	s.writeIoTOK(w, []byte(sh.PayloadJSON))
	s.writeSuccessAudit(r, requestID, eventID, verified, iotDataEventSource, "UpdateThingShadow", readOnly)
}

func (s *Server) iotGetThingShadow(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	if !s.authorize(verified, catalog.ActionIoTDataGetThingShadow, "*") {
		s.writeIoTError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform iot:GetThingShadow.", readOnly, eventID, verified)
		return
	}
	name, _ := params["thingName"].(string)
	if name == "" {
		name, _ = params["ThingName"].(string)
	}
	shadowName, _ := params["shadowName"].(string)
	if shadowName == "" {
		shadowName, _ = params["ShadowName"].(string)
	}
	sh, err := s.store.GetIoTThingShadow(verified.AccountID, s.iotRegion(verified), name, shadowName)
	if errors.Is(err, store.ErrIoTNotFound) {
		s.writeIoTError(w, r, body, requestID, http.StatusNotFound, "ResourceNotFoundException",
			"Shadow not found.", readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeIoTError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailureException",
			"Unable to get thing shadow.", readOnly, eventID, verified)
		return
	}
	s.writeIoTOK(w, []byte(sh.PayloadJSON))
	s.writeSuccessAudit(r, requestID, eventID, verified, iotDataEventSource, "GetThingShadow", readOnly)
}

func (s *Server) iotDeleteThingShadow(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	if !s.authorize(verified, catalog.ActionIoTDataDeleteThingShadow, "*") {
		s.writeIoTError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform iot:DeleteThingShadow.", readOnly, eventID, verified)
		return
	}
	name, _ := params["thingName"].(string)
	if name == "" {
		name, _ = params["ThingName"].(string)
	}
	shadowName, _ := params["shadowName"].(string)
	if shadowName == "" {
		shadowName, _ = params["ShadowName"].(string)
	}
	err := s.store.DeleteIoTThingShadow(verified.AccountID, s.iotRegion(verified), name, shadowName)
	if errors.Is(err, store.ErrIoTNotFound) {
		s.writeIoTError(w, r, body, requestID, http.StatusNotFound, "ResourceNotFoundException",
			"Shadow not found.", readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeIoTError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailureException",
			"Unable to delete thing shadow.", readOnly, eventID, verified)
		return
	}
	payload, _ := iotsvc.EmptyJSON()
	s.writeIoTOK(w, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, iotDataEventSource, "DeleteThingShadow", readOnly)
}

func (s *Server) writeIoTOK(w http.ResponseWriter, payload []byte) {
	w.Header().Set("Content-Type", iotJSONContentType)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(payload)
}

func (s *Server) writeIoTError(
	w http.ResponseWriter, r *http.Request, body []byte, requestID string, status int, code, message string,
	readOnly bool, eventID string, verified *authn.Verified,
) {
	w.Header().Set("Content-Type", iotJSONContentType)
	w.Header().Set("x-amzn-ErrorType", code)
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"__type": code, "message": message})
	_ = body
	s.writeSuccessAudit(r, requestID, eventID, verified, iotEventSource, code, readOnly)
}
