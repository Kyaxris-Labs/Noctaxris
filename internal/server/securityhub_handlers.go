package server

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/Kyaxris-Labs/Noctaxris/internal/catalog"
	"github.com/Kyaxris-Labs/Noctaxris/internal/kernel/authn"
	shsvc "github.com/Kyaxris-Labs/Noctaxris/internal/services/securityhub"
	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

const securityHubEventSource = "securityhub.amazonaws.com"

func (s *Server) handleSecurityHub(
	w http.ResponseWriter,
	r *http.Request,
	body []byte,
	requestID, eventID, action string,
	verified *authn.Verified,
	readOnly bool,
) {
	params := jsonBodyMap(body)
	action = securityHubAction(action)

	switch action {
	case catalog.ActionSecurityHubBatchImportFindings:
		s.shBatchImportFindings(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionSecurityHubGetFindings:
		s.shGetFindings(w, r, body, requestID, eventID, verified, readOnly, params)
	default:
		s.writeSecurityHubError(w, r, body, requestID, http.StatusNotImplemented, "InvalidAction",
			"This Security Hub action is not implemented.", readOnly, eventID, verified)
	}
}

func securityHubAction(action string) string {
	if strings.Contains(action, ":") {
		return action
	}
	switch action {
	case "BatchImportFindings":
		return catalog.ActionSecurityHubBatchImportFindings
	case "GetFindings":
		return catalog.ActionSecurityHubGetFindings
	default:
		return action
	}
}

func (s *Server) shBatchImportFindings(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	if !s.authorize(verified, catalog.ActionSecurityHubBatchImportFindings, "*") {
		s.writeSecurityHubError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform securityhub:BatchImportFindings.", readOnly, eventID, verified)
		return
	}
	raw, _ := params["Findings"].([]any)
	findings := make([]store.SecurityHubFinding, 0, len(raw))
	for _, item := range raw {
		b, _ := json.Marshal(item)
		var f store.SecurityHubFinding
		if err := json.Unmarshal(b, &f); err != nil {
			s.writeSecurityHubError(w, r, body, requestID, http.StatusBadRequest, "InvalidInputException",
				"Invalid Findings entry.", readOnly, eventID, verified)
			return
		}
		findings = append(findings, f)
	}
	succeeded, failed, err := s.store.BatchImportSecurityHubFindings(verified.AccountID, verified.Region, findings)
	if errors.Is(err, store.ErrSecurityHubBadRequest) {
		s.writeSecurityHubError(w, r, body, requestID, http.StatusBadRequest, "InvalidInputException",
			err.Error(), readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeSecurityHubError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to import findings.", readOnly, eventID, verified)
		return
	}
	payload, _ := shsvc.BatchImportFindingsJSON(succeeded, failed)
	s.writeSecurityHubOK(w, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, securityHubEventSource, "BatchImportFindings", false,
		WithAuditRequestParameters(map[string]any{"successCount": len(succeeded), "failedCount": len(failed)}))
}

func (s *Server) shGetFindings(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	if !s.authorize(verified, catalog.ActionSecurityHubGetFindings, "*") {
		s.writeSecurityHubError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform securityhub:GetFindings.", readOnly, eventID, verified)
		return
	}
	filter := store.SecurityHubFindingsFilter{}
	if filters, ok := params["Filters"].(map[string]any); ok {
		filter.ProductArn = firstStringFilter(filters, "ProductArn")
		filter.GeneratorId = firstStringFilter(filters, "GeneratorId")
		filter.SeverityLabel = firstStringFilter(filters, "SeverityLabel")
		filter.ResourceType = firstStringFilter(filters, "ResourceType")
	}
	// Lab also accepts flat keys for convenience.
	if v, _ := params["ProductArn"].(string); v != "" {
		filter.ProductArn = v
	}
	if v, _ := params["GeneratorId"].(string); v != "" {
		filter.GeneratorId = v
	}
	if v, _ := params["SeverityLabel"].(string); v != "" {
		filter.SeverityLabel = v
	}
	if v, _ := params["ResourceType"].(string); v != "" {
		filter.ResourceType = v
	}
	findings, err := s.store.GetSecurityHubFindings(verified.AccountID, filter)
	if err != nil {
		s.writeSecurityHubError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to get findings.", readOnly, eventID, verified)
		return
	}
	payload, _ := shsvc.GetFindingsJSON(findings)
	s.writeSecurityHubOK(w, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, securityHubEventSource, "GetFindings", true)
}

func firstStringFilter(filters map[string]any, key string) string {
	raw, ok := filters[key]
	if !ok {
		return ""
	}
	switch v := raw.(type) {
	case string:
		return v
	case map[string]any:
		if eq, ok := v["Eq"].([]any); ok && len(eq) > 0 {
			if s, ok := eq[0].(string); ok {
				return s
			}
		}
		if s, ok := v["Value"].(string); ok {
			return s
		}
	case []any:
		if len(v) > 0 {
			if s, ok := v[0].(string); ok {
				return s
			}
		}
	}
	return ""
}

func (s *Server) writeSecurityHubOK(w http.ResponseWriter, payload []byte) {
	w.Header().Set("Content-Type", "application/x-amz-json-1.1")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(payload)
}

func (s *Server) writeSecurityHubError(
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
