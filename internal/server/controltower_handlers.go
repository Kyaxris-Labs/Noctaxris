package server

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/Kyaxris-Labs/Noctaxris/internal/catalog"
	"github.com/Kyaxris-Labs/Noctaxris/internal/kernel/authn"
)

const (
	controlTowerJSONContentType = "application/x-amz-json-1.1"
	controlTowerEventSource     = "controltower.amazonaws.com"
)

func (s *Server) handleControlTower(
	w http.ResponseWriter,
	r *http.Request,
	body []byte,
	requestID, eventID, action string,
	verified *authn.Verified,
	readOnly bool,
) {
	params := jsonBodyMap(body)
	action = controlTowerAction(action)

	switch action {
	case catalog.ActionControlTowerListLandingZones:
		s.ctListLandingZones(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionControlTowerGetLandingZone:
		s.ctGetLandingZone(w, r, body, requestID, eventID, verified, readOnly, params)
	default:
		s.writeControlTowerError(w, r, body, requestID, http.StatusNotImplemented, "UnknownOperationException",
			"This Control Tower action is not implemented.", readOnly, eventID, verified)
	}
}

func controlTowerAction(action string) string {
	if strings.Contains(action, ":") {
		return action
	}
	switch action {
	case "ListLandingZones":
		return catalog.ActionControlTowerListLandingZones
	case "GetLandingZone":
		return catalog.ActionControlTowerGetLandingZone
	default:
		return action
	}
}

func isControlTowerAction(action, service string) bool {
	if strings.EqualFold(service, "controltower") || strings.HasPrefix(action, "controltower:") {
		switch controlTowerAction(action) {
		case catalog.ActionControlTowerListLandingZones, catalog.ActionControlTowerGetLandingZone:
			return true
		}
	}
	return false
}

func (s *Server) ctListLandingZones(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	_ = params
	if !s.authorize(verified, catalog.ActionControlTowerListLandingZones, "*") {
		s.writeControlTowerError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform controltower:ListLandingZones.", readOnly, eventID, verified)
		return
	}
	payload, _ := json.Marshal(map[string]any{"landingZones": []any{}})
	s.writeControlTowerOK(w, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, controlTowerEventSource, "ListLandingZones", readOnly)
}

func (s *Server) ctGetLandingZone(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	_ = params
	if !s.authorize(verified, catalog.ActionControlTowerGetLandingZone, "*") {
		s.writeControlTowerError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform controltower:GetLandingZone.", readOnly, eventID, verified)
		return
	}
	s.writeControlTowerError(w, r, body, requestID, http.StatusBadRequest, "ResourceNotFoundException",
		"Landing zone not found.", readOnly, eventID, verified)
}

func (s *Server) writeControlTowerOK(w http.ResponseWriter, payload []byte) {
	w.Header().Set("Content-Type", controlTowerJSONContentType)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(payload)
}

func (s *Server) writeControlTowerError(
	w http.ResponseWriter, r *http.Request, body []byte, requestID string, status int, code, message string,
	readOnly bool, eventID string, verified *authn.Verified,
) {
	w.Header().Set("Content-Type", controlTowerJSONContentType)
	w.Header().Set("x-amzn-ErrorType", code)
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"__type": code, "message": message})
	_ = body
	s.writeSuccessAudit(r, requestID, eventID, verified, controlTowerEventSource, code, readOnly)
}
