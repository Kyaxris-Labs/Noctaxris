package server

import (
	"errors"
	"net/http"
	"strings"

	"github.com/Kyaxris-Labs/Noctaxris/internal/catalog"
	"github.com/Kyaxris-Labs/Noctaxris/internal/kernel/authn"
	athenasvc "github.com/Kyaxris-Labs/Noctaxris/internal/services/athena"
	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

const (
	athenaJSONContentType = "application/x-amz-json-1.1"
	athenaEventSource     = "athena.amazonaws.com"
)

func (s *Server) handleAthena(
	w http.ResponseWriter,
	r *http.Request,
	body []byte,
	requestID, eventID, action string,
	verified *authn.Verified,
	readOnly bool,
) {
	params := jsonBodyMap(body)
	action = athenaAction(action)

	switch action {
	case catalog.ActionAthenaStartQueryExecution:
		s.athenaStart(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionAthenaGetQueryExecution:
		s.athenaGetExecution(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionAthenaGetQueryResults:
		s.athenaGetResults(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionAthenaStopQueryExecution:
		s.athenaStop(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionAthenaCreateWorkGroup:
		s.athenaCreateWorkGroup(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionAthenaGetWorkGroup:
		s.athenaGetWorkGroup(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionAthenaListWorkGroups:
		s.athenaListWorkGroups(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionAthenaDeleteWorkGroup:
		s.athenaDeleteWorkGroup(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionAthenaUpdateWorkGroup:
		s.athenaUpdateWorkGroup(w, r, body, requestID, eventID, verified, readOnly, params)
	default:
		s.writeAthenaError(w, r, body, requestID, http.StatusNotImplemented, "InternalFailure",
			"This Athena action is not implemented.", readOnly, eventID, verified)
	}
}

func athenaAction(action string) string {
	if strings.Contains(action, ":") {
		return action
	}
	switch action {
	case "StartQueryExecution":
		return catalog.ActionAthenaStartQueryExecution
	case "GetQueryExecution":
		return catalog.ActionAthenaGetQueryExecution
	case "GetQueryResults":
		return catalog.ActionAthenaGetQueryResults
	case "StopQueryExecution":
		return catalog.ActionAthenaStopQueryExecution
	case "CreateWorkGroup":
		return catalog.ActionAthenaCreateWorkGroup
	case "GetWorkGroup":
		return catalog.ActionAthenaGetWorkGroup
	case "ListWorkGroups":
		return catalog.ActionAthenaListWorkGroups
	case "DeleteWorkGroup":
		return catalog.ActionAthenaDeleteWorkGroup
	case "UpdateWorkGroup":
		return catalog.ActionAthenaUpdateWorkGroup
	default:
		return action
	}
}

func (s *Server) athenaStart(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	if !s.authorize(verified, catalog.ActionAthenaStartQueryExecution, "*") {
		s.writeAthenaError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform athena:StartQueryExecution.", readOnly, eventID, verified)
		return
	}
	q, _ := params["QueryString"].(string)
	in := store.AthenaStartInput{QueryString: q}
	if ctx, ok := params["QueryExecutionContext"].(map[string]any); ok {
		in.Database, _ = ctx["Database"].(string)
		in.Catalog, _ = ctx["Catalog"].(string)
	}
	if rc, ok := params["ResultConfiguration"].(map[string]any); ok {
		in.OutputLocation, _ = rc["OutputLocation"].(string)
	}
	if wg, ok := params["WorkGroup"].(string); ok {
		in.WorkGroup = wg
	}
	var exec store.AthenaQueryExecution
	var err error
	if duckExec, used, duckErr := s.tryStartAthenaDuck(verified.AccountID, in); used {
		exec, err = duckExec, duckErr
	} else {
		exec, err = s.store.StartAthenaQueryExecution(verified.AccountID, in)
	}
	if errors.Is(err, store.ErrAthenaBadRequest) {
		s.writeAthenaError(w, r, body, requestID, http.StatusBadRequest, "InvalidRequestException",
			err.Error(), readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeAthenaError(w, r, body, requestID, http.StatusInternalServerError, "InternalServerException",
			"Unable to start query execution.", readOnly, eventID, verified)
		return
	}
	payload, _ := athenasvc.StartQueryExecutionJSON(exec.QueryExecutionID)
	s.writeAthenaOK(w, requestID, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, athenaEventSource, "StartQueryExecution", readOnly)
}

func (s *Server) athenaGetExecution(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	if !s.authorize(verified, catalog.ActionAthenaGetQueryExecution, "*") {
		s.writeAthenaError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform athena:GetQueryExecution.", readOnly, eventID, verified)
		return
	}
	id, _ := params["QueryExecutionId"].(string)
	exec, err := s.store.GetAthenaQueryExecution(verified.AccountID, id)
	if errors.Is(err, store.ErrAthenaNotFound) {
		s.writeAthenaError(w, r, body, requestID, http.StatusBadRequest, "InvalidRequestException",
			"QueryExecution was not found.", readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeAthenaError(w, r, body, requestID, http.StatusInternalServerError, "InternalServerException",
			"Unable to get query execution.", readOnly, eventID, verified)
		return
	}
	payload, _ := athenasvc.GetQueryExecutionJSON(exec)
	s.writeAthenaOK(w, requestID, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, athenaEventSource, "GetQueryExecution", readOnly)
}

func (s *Server) athenaGetResults(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	if !s.authorize(verified, catalog.ActionAthenaGetQueryResults, "*") {
		s.writeAthenaError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform athena:GetQueryResults.", readOnly, eventID, verified)
		return
	}
	id, _ := params["QueryExecutionId"].(string)
	exec, err := s.store.GetAthenaQueryResults(verified.AccountID, id)
	if errors.Is(err, store.ErrAthenaNotFound) {
		s.writeAthenaError(w, r, body, requestID, http.StatusBadRequest, "InvalidRequestException",
			"QueryExecution was not found.", readOnly, eventID, verified)
		return
	}
	if errors.Is(err, store.ErrAthenaBadRequest) {
		s.writeAthenaError(w, r, body, requestID, http.StatusBadRequest, "InvalidRequestException",
			err.Error(), readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeAthenaError(w, r, body, requestID, http.StatusInternalServerError, "InternalServerException",
			"Unable to get query results.", readOnly, eventID, verified)
		return
	}
	payload, _ := athenasvc.GetQueryResultsJSON(exec)
	s.writeAthenaOK(w, requestID, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, athenaEventSource, "GetQueryResults", readOnly)
}

func (s *Server) athenaStop(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	if !s.authorize(verified, catalog.ActionAthenaStopQueryExecution, "*") {
		s.writeAthenaError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform athena:StopQueryExecution.", readOnly, eventID, verified)
		return
	}
	id, _ := params["QueryExecutionId"].(string)
	err := s.store.StopAthenaQueryExecution(verified.AccountID, id)
	if errors.Is(err, store.ErrAthenaNotFound) {
		s.writeAthenaError(w, r, body, requestID, http.StatusBadRequest, "InvalidRequestException",
			"QueryExecution was not found.", readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeAthenaError(w, r, body, requestID, http.StatusInternalServerError, "InternalServerException",
			"Unable to stop query execution.", readOnly, eventID, verified)
		return
	}
	payload, _ := athenasvc.StopQueryExecutionJSON()
	s.writeAthenaOK(w, requestID, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, athenaEventSource, "StopQueryExecution", readOnly)
}

func (s *Server) athenaCreateWorkGroup(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	if !s.authorize(verified, catalog.ActionAthenaCreateWorkGroup, "*") {
		s.writeAthenaError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform athena:CreateWorkGroup.", readOnly, eventID, verified)
		return
	}
	in := store.AthenaWorkGroupCreate{}
	in.Name, _ = params["Name"].(string)
	in.Description, _ = params["Description"].(string)
	if cfg, ok := params["Configuration"].(map[string]any); ok {
		if v, ok := cfg["EnforceWorkGroupConfiguration"].(bool); ok {
			in.EnforceWorkGroupConfig = v
		}
		if rc, ok := cfg["ResultConfiguration"].(map[string]any); ok {
			in.OutputLocation, _ = rc["OutputLocation"].(string)
		}
	}
	_, err := s.store.CreateAthenaWorkGroup(verified.AccountID, in)
	if errors.Is(err, store.ErrAthenaBadRequest) {
		s.writeAthenaError(w, r, body, requestID, http.StatusBadRequest, "InvalidRequestException",
			err.Error(), readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeAthenaError(w, r, body, requestID, http.StatusInternalServerError, "InternalServerException",
			"Unable to create workgroup.", readOnly, eventID, verified)
		return
	}
	payload, _ := athenasvc.CreateWorkGroupJSON()
	s.writeAthenaOK(w, requestID, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, athenaEventSource, "CreateWorkGroup", readOnly)
}

func (s *Server) athenaGetWorkGroup(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	if !s.authorize(verified, catalog.ActionAthenaGetWorkGroup, "*") {
		s.writeAthenaError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform athena:GetWorkGroup.", readOnly, eventID, verified)
		return
	}
	name, _ := params["WorkGroup"].(string)
	wg, err := s.store.GetAthenaWorkGroup(verified.AccountID, name)
	if errors.Is(err, store.ErrAthenaNotFound) {
		s.writeAthenaError(w, r, body, requestID, http.StatusBadRequest, "InvalidRequestException",
			err.Error(), readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeAthenaError(w, r, body, requestID, http.StatusInternalServerError, "InternalServerException",
			"Unable to get workgroup.", readOnly, eventID, verified)
		return
	}
	payload, _ := athenasvc.GetWorkGroupJSON(wg)
	s.writeAthenaOK(w, requestID, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, athenaEventSource, "GetWorkGroup", readOnly)
}

func (s *Server) athenaListWorkGroups(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	if !s.authorize(verified, catalog.ActionAthenaListWorkGroups, "*") {
		s.writeAthenaError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform athena:ListWorkGroups.", readOnly, eventID, verified)
		return
	}
	_ = params
	groups, err := s.store.ListAthenaWorkGroups(verified.AccountID)
	if err != nil {
		s.writeAthenaError(w, r, body, requestID, http.StatusInternalServerError, "InternalServerException",
			"Unable to list workgroups.", readOnly, eventID, verified)
		return
	}
	payload, _ := athenasvc.ListWorkGroupsJSON(groups)
	s.writeAthenaOK(w, requestID, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, athenaEventSource, "ListWorkGroups", readOnly)
}

func (s *Server) athenaDeleteWorkGroup(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	if !s.authorize(verified, catalog.ActionAthenaDeleteWorkGroup, "*") {
		s.writeAthenaError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform athena:DeleteWorkGroup.", readOnly, eventID, verified)
		return
	}
	name, _ := params["WorkGroup"].(string)
	err := s.store.DeleteAthenaWorkGroup(verified.AccountID, name)
	if errors.Is(err, store.ErrAthenaNotFound) || errors.Is(err, store.ErrAthenaBadRequest) {
		s.writeAthenaError(w, r, body, requestID, http.StatusBadRequest, "InvalidRequestException",
			err.Error(), readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeAthenaError(w, r, body, requestID, http.StatusInternalServerError, "InternalServerException",
			"Unable to delete workgroup.", readOnly, eventID, verified)
		return
	}
	payload, _ := athenasvc.DeleteWorkGroupJSON()
	s.writeAthenaOK(w, requestID, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, athenaEventSource, "DeleteWorkGroup", readOnly)
}

func (s *Server) athenaUpdateWorkGroup(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	if !s.authorize(verified, catalog.ActionAthenaUpdateWorkGroup, "*") {
		s.writeAthenaError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform athena:UpdateWorkGroup.", readOnly, eventID, verified)
		return
	}
	name, _ := params["WorkGroup"].(string)
	in := store.AthenaWorkGroupUpdate{}
	if desc, ok := params["Description"].(string); ok {
		in.Description = &desc
	}
	if st, ok := params["State"].(string); ok {
		in.State = &st
	}
	if cfg, ok := params["ConfigurationUpdates"].(map[string]any); ok {
		if v, ok := cfg["EnforceWorkGroupConfiguration"].(bool); ok {
			in.EnforceWorkGroupConfig = &v
		}
		if rc, ok := cfg["ResultConfigurationUpdates"].(map[string]any); ok {
			if rem, ok := rc["RemoveOutputLocation"].(bool); ok && rem {
				in.RemoveOutputLocation = true
			} else if out, ok := rc["OutputLocation"].(string); ok {
				in.OutputLocation = &out
			}
		}
	}
	_, err := s.store.UpdateAthenaWorkGroup(verified.AccountID, name, in)
	if errors.Is(err, store.ErrAthenaNotFound) || errors.Is(err, store.ErrAthenaBadRequest) {
		s.writeAthenaError(w, r, body, requestID, http.StatusBadRequest, "InvalidRequestException",
			err.Error(), readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeAthenaError(w, r, body, requestID, http.StatusInternalServerError, "InternalServerException",
			"Unable to update workgroup.", readOnly, eventID, verified)
		return
	}
	payload, _ := athenasvc.UpdateWorkGroupJSON()
	s.writeAthenaOK(w, requestID, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, athenaEventSource, "UpdateWorkGroup", readOnly)
}

func (s *Server) writeAthenaOK(w http.ResponseWriter, requestID string, payload []byte) {
	w.Header().Set("Content-Type", athenaJSONContentType)
	w.Header().Set("x-amzn-RequestId", requestID)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(payload)
}

func (s *Server) writeAthenaError(
	w http.ResponseWriter, r *http.Request, body []byte, requestID string,
	status int, code, message string, readOnly bool, eventID string, verified *authn.Verified,
) {
	w.Header().Set("Content-Type", athenaJSONContentType)
	w.Header().Set("x-amzn-RequestId", requestID)
	w.Header().Set("x-amzn-ErrorType", code)
	w.WriteHeader(status)
	escaped := strings.ReplaceAll(message, `"`, `'`)
	_, _ = w.Write([]byte(`{"__type":"` + code + `","message":"` + escaped + `"}`))
	_ = body
	_ = r
	_ = readOnly
	_ = eventID
	_ = verified
}
