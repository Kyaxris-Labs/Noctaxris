package server

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/Kyaxris-Labs/Noctaxris/internal/catalog"
	"github.com/Kyaxris-Labs/Noctaxris/internal/kernel/authn"
	cesvc "github.com/Kyaxris-Labs/Noctaxris/internal/services/costexplorer"
)

const (
	ceJSONContentType = "application/x-amz-json-1.1"
	ceEventSource     = "ce.amazonaws.com"
)

func (s *Server) handleCostExplorer(
	w http.ResponseWriter,
	r *http.Request,
	body []byte,
	requestID, eventID, action string,
	verified *authn.Verified,
	readOnly bool,
) {
	params := jsonBodyMap(body)
	action = costExplorerAction(action)

	switch action {
	case catalog.ActionCEGetCostAndUsage:
		s.ceGetCostAndUsage(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionCEGetCostForecast:
		s.ceGetCostForecast(w, r, body, requestID, eventID, verified, readOnly, params)
	default:
		s.writeCEError(w, r, body, requestID, http.StatusNotImplemented, "InvalidAction",
			"This Cost Explorer action is not implemented.", readOnly, eventID, verified)
	}
}

func costExplorerAction(action string) string {
	if strings.Contains(action, ":") {
		return action
	}
	switch action {
	case "GetCostAndUsage":
		return catalog.ActionCEGetCostAndUsage
	case "GetCostForecast":
		return catalog.ActionCEGetCostForecast
	default:
		return action
	}
}

func (s *Server) ceGetCostAndUsage(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	if !s.authorize(verified, catalog.ActionCEGetCostAndUsage, "*") {
		s.writeCEError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform ce:GetCostAndUsage.", readOnly, eventID, verified)
		return
	}
	granularity, _ := params["Granularity"].(string)
	var metrics []string
	if raw, ok := params["Metrics"].([]any); ok {
		for _, item := range raw {
			if m, ok := item.(string); ok {
				metrics = append(metrics, m)
			}
		}
	}
	rows, err := s.store.CostExplorerGetCostAndUsage(granularity, metrics)
	if err != nil {
		s.writeCEError(w, r, body, requestID, http.StatusBadRequest, "ValidationException",
			err.Error(), readOnly, eventID, verified)
		return
	}
	payload, _ := cesvc.GetCostAndUsageJSON(rows)
	s.writeCEOK(w, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, ceEventSource, "GetCostAndUsage", readOnly)
}

func (s *Server) ceGetCostForecast(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	if !s.authorize(verified, catalog.ActionCEGetCostForecast, "*") {
		s.writeCEError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform ce:GetCostForecast.", readOnly, eventID, verified)
		return
	}
	metric, _ := params["Metric"].(string)
	total, start, end, err := s.store.CostExplorerGetCostForecast(metric)
	if err != nil {
		s.writeCEError(w, r, body, requestID, http.StatusBadRequest, "ValidationException",
			err.Error(), readOnly, eventID, verified)
		return
	}
	payload, _ := cesvc.GetCostForecastJSON(total, start, end)
	s.writeCEOK(w, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, ceEventSource, "GetCostForecast", readOnly)
}

func (s *Server) writeCEOK(w http.ResponseWriter, payload []byte) {
	w.Header().Set("Content-Type", ceJSONContentType)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(payload)
}

func (s *Server) writeCEError(
	w http.ResponseWriter, r *http.Request, body []byte, requestID string, status int, code, message string,
	readOnly bool, eventID string, verified *authn.Verified,
) {
	w.Header().Set("Content-Type", ceJSONContentType)
	w.Header().Set("x-amzn-ErrorType", code)
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"__type": code, "message": message})
	_ = body
	s.writeSuccessAudit(r, requestID, eventID, verified, ceEventSource, code, readOnly)
}
