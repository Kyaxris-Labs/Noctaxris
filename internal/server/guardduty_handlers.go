package server

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/Kyaxris-Labs/Noctaxris/internal/catalog"
	"github.com/Kyaxris-Labs/Noctaxris/internal/kernel/authn"
	gdsvc "github.com/Kyaxris-Labs/Noctaxris/internal/services/guardduty"
	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

const guarddutyEventSource = "guardduty.amazonaws.com"

func (s *Server) handleGuardDuty(
	w http.ResponseWriter,
	r *http.Request,
	body []byte,
	requestID, eventID, action string,
	verified *authn.Verified,
	readOnly bool,
) {
	params := jsonBodyMap(body)
	action = guarddutyAction(action)

	switch action {
	case catalog.ActionGuardDutyCreateDetector:
		s.gdCreateDetector(w, r, body, requestID, eventID, verified, readOnly)
	case catalog.ActionGuardDutyListDetectors:
		s.gdListDetectors(w, r, body, requestID, eventID, verified, readOnly)
	case catalog.ActionGuardDutyListFindings:
		s.gdListFindings(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionGuardDutyGetFindings:
		s.gdGetFindings(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionGuardDutyInjectFindings:
		s.gdInjectFindings(w, r, body, requestID, eventID, verified, readOnly, params)
	default:
		s.writeGuardDutyError(w, r, body, requestID, http.StatusNotImplemented, "InvalidAction",
			"This GuardDuty action is not implemented.", readOnly, eventID, verified)
	}
}

func guarddutyAction(action string) string {
	if strings.Contains(action, ":") {
		return action
	}
	switch action {
	case "CreateDetector":
		return catalog.ActionGuardDutyCreateDetector
	case "ListDetectors":
		return catalog.ActionGuardDutyListDetectors
	case "ListFindings":
		return catalog.ActionGuardDutyListFindings
	case "GetFindings":
		return catalog.ActionGuardDutyGetFindings
	case "InjectFindings":
		return catalog.ActionGuardDutyInjectFindings
	default:
		return action
	}
}

func (s *Server) gdCreateDetector(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool,
) {
	if !s.authorize(verified, catalog.ActionGuardDutyCreateDetector, "*") {
		s.writeGuardDutyError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform guardduty:CreateDetector.", readOnly, eventID, verified)
		return
	}
	d, err := s.store.CreateGuardDutyDetector(verified.AccountID)
	if err != nil {
		s.writeGuardDutyError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to create detector.", readOnly, eventID, verified)
		return
	}
	payload, _ := gdsvc.CreateDetectorJSON(d.DetectorID)
	s.writeGuardDutyOK(w, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, guarddutyEventSource, "CreateDetector", readOnly)
}

func (s *Server) gdListDetectors(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool,
) {
	if !s.authorize(verified, catalog.ActionGuardDutyListDetectors, "*") {
		s.writeGuardDutyError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform guardduty:ListDetectors.", readOnly, eventID, verified)
		return
	}
	ids, err := s.store.ListGuardDutyDetectors(verified.AccountID)
	if err != nil {
		s.writeGuardDutyError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to list detectors.", readOnly, eventID, verified)
		return
	}
	payload, _ := gdsvc.ListDetectorsJSON(ids)
	s.writeGuardDutyOK(w, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, guarddutyEventSource, "ListDetectors", true)
}

func (s *Server) gdListFindings(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	detectorID, _ := params["DetectorId"].(string)
	if detectorID == "" {
		detectorID, _ = params["detectorId"].(string)
	}
	if !s.authorize(verified, catalog.ActionGuardDutyListFindings, "*") {
		s.writeGuardDutyError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform guardduty:ListFindings.", readOnly, eventID, verified)
		return
	}
	ids, err := s.store.ListGuardDutyFindingIDs(verified.AccountID, detectorID, 50)
	if errors.Is(err, store.ErrGuardDutyNotFound) {
		s.writeGuardDutyError(w, r, body, requestID, http.StatusBadRequest, "BadRequestException",
			"Detector not found.", readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeGuardDutyError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to list findings.", readOnly, eventID, verified)
		return
	}
	payload, _ := gdsvc.ListFindingsJSON(ids)
	s.writeGuardDutyOK(w, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, guarddutyEventSource, "ListFindings", true)
}

func (s *Server) gdGetFindings(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	detectorID, _ := params["DetectorId"].(string)
	if detectorID == "" {
		detectorID, _ = params["detectorId"].(string)
	}
	if !s.authorize(verified, catalog.ActionGuardDutyGetFindings, "*") {
		s.writeGuardDutyError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform guardduty:GetFindings.", readOnly, eventID, verified)
		return
	}
	rawIDs, _ := params["FindingIds"].([]any)
	if rawIDs == nil {
		rawIDs, _ = params["findingIds"].([]any)
	}
	ids := make([]string, 0, len(rawIDs))
	for _, v := range rawIDs {
		if s, ok := v.(string); ok && s != "" {
			ids = append(ids, s)
		}
	}
	findings, err := s.store.GetGuardDutyFindings(verified.AccountID, detectorID, ids)
	if errors.Is(err, store.ErrGuardDutyNotFound) {
		s.writeGuardDutyError(w, r, body, requestID, http.StatusBadRequest, "BadRequestException",
			"Detector not found.", readOnly, eventID, verified)
		return
	}
	if errors.Is(err, store.ErrGuardDutyBadRequest) {
		s.writeGuardDutyError(w, r, body, requestID, http.StatusBadRequest, "BadRequestException",
			err.Error(), readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeGuardDutyError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to get findings.", readOnly, eventID, verified)
		return
	}
	payload, _ := gdsvc.GetFindingsJSON(findings)
	s.writeGuardDutyOK(w, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, guarddutyEventSource, "GetFindings", true)
}

func (s *Server) gdInjectFindings(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	if !s.cfg.GuardDutyInject {
		s.writeGuardDutyError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"guardduty:InjectFindings is disabled. Set NOCTAXRIS_GUARDDUTY_INJECT=1.", readOnly, eventID, verified)
		return
	}
	if !s.authorize(verified, catalog.ActionGuardDutyInjectFindings, "*") {
		s.writeGuardDutyError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform guardduty:InjectFindings.", readOnly, eventID, verified)
		return
	}
	detectorID, _ := params["DetectorId"].(string)
	if detectorID == "" {
		detectorID, _ = params["detectorId"].(string)
	}
	if detectorID == "" {
		var err error
		detectorID, err = s.store.EnsureGuardDutyDetector(verified.AccountID)
		if err != nil {
			s.writeGuardDutyError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
				"Unable to ensure detector.", readOnly, eventID, verified)
			return
		}
	}
	var findings []store.GuardDutyFinding
	if raw, ok := params["Findings"].([]any); ok {
		for _, item := range raw {
			b, _ := json.Marshal(item)
			var f store.GuardDutyFinding
			if err := json.Unmarshal(b, &f); err != nil {
				s.writeGuardDutyError(w, r, body, requestID, http.StatusBadRequest, "BadRequestException",
					"Invalid Findings entry.", readOnly, eventID, verified)
				return
			}
			findings = append(findings, f)
		}
	} else if raw, ok := params["Finding"].(map[string]any); ok {
		b, _ := json.Marshal(raw)
		var f store.GuardDutyFinding
		if err := json.Unmarshal(b, &f); err != nil {
			s.writeGuardDutyError(w, r, body, requestID, http.StatusBadRequest, "BadRequestException",
				"Invalid Finding.", readOnly, eventID, verified)
			return
		}
		findings = append(findings, f)
	}
	region := verified.Region
	ids, err := s.store.InjectGuardDutyFindings(verified.AccountID, detectorID, region, findings)
	if errors.Is(err, store.ErrGuardDutyNotFound) {
		s.writeGuardDutyError(w, r, body, requestID, http.StatusBadRequest, "BadRequestException",
			"Detector not found.", readOnly, eventID, verified)
		return
	}
	if errors.Is(err, store.ErrGuardDutyBadRequest) {
		s.writeGuardDutyError(w, r, body, requestID, http.StatusBadRequest, "BadRequestException",
			err.Error(), readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeGuardDutyError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to inject findings.", readOnly, eventID, verified)
		return
	}
	payload, _ := gdsvc.InjectFindingsJSON(ids)
	s.writeGuardDutyOK(w, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, guarddutyEventSource, "InjectFindings", false,
		WithAuditRequestParameters(map[string]any{"count": len(ids), "detectorId": detectorID}))
}

func (s *Server) writeGuardDutyOK(w http.ResponseWriter, payload []byte) {
	w.Header().Set("Content-Type", "application/x-amz-json-1.1")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(payload)
}

func (s *Server) writeGuardDutyError(
	w http.ResponseWriter, r *http.Request, body []byte, requestID string,
	status int, code, message string, readOnly bool, eventID string, verified *authn.Verified,
) {
	ak, acct := "", ""
	known := false
	if verified != nil {
		ak, acct, known = verified.AccessKeyID, verified.AccountID, true
	}
	s.writeAPIError(w, r, body, requestID, status, code, message, readOnly, eventID, ak, acct, known)
}
