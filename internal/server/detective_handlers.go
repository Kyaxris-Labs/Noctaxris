package server

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/Kyaxris-Labs/Noctaxris/internal/catalog"
	"github.com/Kyaxris-Labs/Noctaxris/internal/kernel/authn"
	detsvc "github.com/Kyaxris-Labs/Noctaxris/internal/services/detective"
	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

const detectiveEventSource = "detective.amazonaws.com"

func (s *Server) handleDetective(
	w http.ResponseWriter,
	r *http.Request,
	body []byte,
	requestID, eventID, action string,
	verified *authn.Verified,
	readOnly bool,
) {
	params := jsonBodyMap(body)
	action = detectiveAction(action)

	switch action {
	case catalog.ActionDetectiveCreateGraph:
		s.detCreateGraph(w, r, body, requestID, eventID, verified, readOnly)
	case catalog.ActionDetectiveListGraphs:
		s.detListGraphs(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionDetectiveAcceptInvitation:
		s.detAcceptInvitation(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionDetectiveSearchGraph:
		s.detSearchGraph(w, r, body, requestID, eventID, verified, readOnly, params)
	default:
		s.writeDetectiveError(w, r, body, requestID, http.StatusNotImplemented, "InvalidAction",
			"This Detective action is not implemented.", readOnly, eventID, verified)
	}
}

func detectiveAction(action string) string {
	if strings.Contains(action, ":") {
		return action
	}
	switch action {
	case "CreateGraph":
		return catalog.ActionDetectiveCreateGraph
	case "ListGraphs":
		return catalog.ActionDetectiveListGraphs
	case "AcceptInvitation":
		return catalog.ActionDetectiveAcceptInvitation
	case "SearchGraph":
		return catalog.ActionDetectiveSearchGraph
	default:
		return action
	}
}

func (s *Server) detCreateGraph(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool,
) {
	if !s.authorize(verified, catalog.ActionDetectiveCreateGraph, "*") {
		s.writeDetectiveError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform detective:CreateGraph.", readOnly, eventID, verified)
		return
	}
	region := verified.Region
	if region == "" {
		region = store.DefaultDetectiveRegion
	}
	g, err := s.store.CreateDetectiveGraph(verified.AccountID, region)
	if err != nil {
		s.writeDetectiveError(w, r, body, requestID, http.StatusInternalServerError, "InternalServerException",
			"Unable to create graph.", readOnly, eventID, verified)
		return
	}
	payload, _ := detsvc.CreateGraphJSON(g.GraphARN)
	s.writeDetectiveOK(w, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, detectiveEventSource, "CreateGraph", false)
}

func (s *Server) detListGraphs(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	if !s.authorize(verified, catalog.ActionDetectiveListGraphs, "*") {
		s.writeDetectiveError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform detective:ListGraphs.", readOnly, eventID, verified)
		return
	}
	max := intFromParam(params, "MaxResults", 50)
	region := verified.Region
	graphs, err := s.store.ListDetectiveGraphs(verified.AccountID, region, max)
	if err != nil {
		s.writeDetectiveError(w, r, body, requestID, http.StatusInternalServerError, "InternalServerException",
			"Unable to list graphs.", readOnly, eventID, verified)
		return
	}
	payload, _ := detsvc.ListGraphsJSON(graphs)
	s.writeDetectiveOK(w, payload)
}

func (s *Server) detAcceptInvitation(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	if !s.authorize(verified, catalog.ActionDetectiveAcceptInvitation, "*") {
		s.writeDetectiveError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform detective:AcceptInvitation.", readOnly, eventID, verified)
		return
	}
	graphARN, _ := params["GraphArn"].(string)
	if strings.TrimSpace(graphARN) == "" {
		s.writeDetectiveError(w, r, body, requestID, http.StatusBadRequest, "ValidationException",
			"GraphArn is required.", readOnly, eventID, verified)
		return
	}
	// Lab noop: accept when the graph exists for this account (member simulation).
	if _, err := s.store.GetDetectiveGraphByARN(verified.AccountID, graphARN); err != nil {
		if errors.Is(err, store.ErrDetectiveNotFound) {
			s.writeDetectiveError(w, r, body, requestID, http.StatusNotFound, "ResourceNotFoundException",
				"Graph not found.", readOnly, eventID, verified)
			return
		}
		if errors.Is(err, store.ErrDetectiveBadRequest) {
			s.writeDetectiveError(w, r, body, requestID, http.StatusBadRequest, "ValidationException",
				err.Error(), readOnly, eventID, verified)
			return
		}
		s.writeDetectiveError(w, r, body, requestID, http.StatusInternalServerError, "InternalServerException",
			"Unable to accept invitation.", readOnly, eventID, verified)
		return
	}
	payload, _ := detsvc.AcceptInvitationJSON()
	s.writeDetectiveOK(w, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, detectiveEventSource, "AcceptInvitation", false)
}

func (s *Server) detSearchGraph(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	if !s.authorize(verified, catalog.ActionDetectiveSearchGraph, "*") {
		s.writeDetectiveError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform detective:SearchGraph.", readOnly, eventID, verified)
		return
	}
	graphARN, _ := params["GraphArn"].(string)
	resourceArn, _ := params["ResourceArn"].(string)
	ip, _ := params["IpAddress"].(string)
	max := intFromParam(params, "MaxResults", 25)
	result, err := s.store.SearchDetectiveGraph(verified.AccountID, s.store.DataRoot(), graphARN, store.DetectiveSearchFilter{
		ResourceArn: resourceArn,
		IpAddress:   ip,
		MaxResults:  max,
	})
	if errors.Is(err, store.ErrDetectiveNotFound) {
		s.writeDetectiveError(w, r, body, requestID, http.StatusNotFound, "ResourceNotFoundException",
			"Graph not found.", readOnly, eventID, verified)
		return
	}
	if errors.Is(err, store.ErrDetectiveBadRequest) {
		s.writeDetectiveError(w, r, body, requestID, http.StatusBadRequest, "ValidationException",
			err.Error(), readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeDetectiveError(w, r, body, requestID, http.StatusInternalServerError, "InternalServerException",
			"Unable to search graph.", readOnly, eventID, verified)
		return
	}
	payload, err := detsvc.SearchGraphJSON(result)
	if err != nil {
		s.writeDetectiveError(w, r, body, requestID, http.StatusInternalServerError, "InternalServerException",
			"Unable to encode search results.", readOnly, eventID, verified)
		return
	}
	s.writeDetectiveOK(w, payload)
}

func intFromParam(params map[string]any, key string, def int) int {
	if params == nil {
		return def
	}
	v, ok := params[key]
	if !ok {
		return def
	}
	switch n := v.(type) {
	case float64:
		return int(n)
	case int:
		return n
	case json.Number:
		i, err := n.Int64()
		if err != nil {
			return def
		}
		return int(i)
	default:
		return def
	}
}

func (s *Server) writeDetectiveOK(w http.ResponseWriter, payload []byte) {
	w.Header().Set("Content-Type", "application/x-amz-json-1.1")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(payload)
}

func (s *Server) writeDetectiveError(
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
