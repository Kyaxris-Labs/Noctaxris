package server

import (
	"errors"
	"net/http"
	"strings"

	"github.com/Kyaxris-Labs/Noctaxris/internal/catalog"
	"github.com/Kyaxris-Labs/Noctaxris/internal/kernel/authn"
	"github.com/Kyaxris-Labs/Noctaxris/internal/kernel/authz"
	"github.com/Kyaxris-Labs/Noctaxris/internal/kernel/sts"
	pipessvc "github.com/Kyaxris-Labs/Noctaxris/internal/services/pipes"
	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

const (
	pipesJSONContentType = "application/x-amz-json-1.0"
	pipesEventSource     = "pipes.amazonaws.com"
)

func (s *Server) handlePipes(
	w http.ResponseWriter,
	r *http.Request,
	body []byte,
	requestID, eventID, action string,
	verified *authn.Verified,
	readOnly bool,
) {
	params := jsonBodyMap(body)
	action = pipesAction(action)

	switch action {
	case catalog.ActionPipesCreatePipe:
		s.pipesCreate(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionPipesDescribePipe:
		s.pipesDescribe(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionPipesDeletePipe:
		s.pipesDelete(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionPipesListPipes:
		s.pipesList(w, r, body, requestID, eventID, verified, readOnly, params)
	default:
		s.writePipesError(w, r, body, requestID, http.StatusNotImplemented, "InternalFailure",
			"This Pipes action is not implemented.", readOnly, eventID, verified)
	}
}

func pipesAction(action string) string {
	if strings.Contains(action, ":") {
		return action
	}
	switch action {
	case "CreatePipe":
		return catalog.ActionPipesCreatePipe
	case "DescribePipe":
		return catalog.ActionPipesDescribePipe
	case "DeletePipe":
		return catalog.ActionPipesDeletePipe
	case "ListPipes":
		return catalog.ActionPipesListPipes
	default:
		return action
	}
}

func (s *Server) checkPipesPassRole(verified *authn.Verified, roleARN string) error {
	accountID, roleName, ok := sts.ParseRoleARN(roleARN)
	if !ok {
		return errors.New("roleARN must be a valid IAM role ARN")
	}
	if accountID != verified.AccountID {
		return errors.New("roleARN must be in the same account")
	}
	storedARN, trust, err := s.store.GetRole(accountID, roleName)
	if err != nil {
		return errors.New("Role not found")
	}
	if storedARN != "" {
		roleARN = storedARN
	}
	in, ok := s.evalInputs(verified)
	if !ok {
		return errors.New("not authorized to pass role to Pipes")
	}
	decision := authz.CheckPassRole(authz.PassRoleRequest{
		EvalInputs:       in,
		RoleARN:          roleARN,
		TrustPolicyDoc:   trust,
		ServicePrincipal: authz.ServicePrincipalPipes,
	})
	if decision != authz.Allow {
		return errors.New("not authorized to pass role to Pipes")
	}
	return nil
}

func (s *Server) pipesRegion(verified *authn.Verified) string {
	if verified.Region != "" {
		return verified.Region
	}
	return store.DefaultPipesRegion
}

func (s *Server) pipesCreate(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	name, _ := params["Name"].(string)
	source, _ := params["Source"].(string)
	target, _ := params["Target"].(string)
	roleARN, _ := params["RoleArn"].(string)
	description, _ := params["Description"].(string)
	desired, _ := params["DesiredState"].(string)
	arn := store.PipeARN(s.pipesRegion(verified), verified.AccountID, name)
	if !s.authorize(verified, catalog.ActionPipesCreatePipe, arn) {
		s.writePipesError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform pipes:CreatePipe.", readOnly, eventID, verified)
		return
	}
	if roleARN != "" {
		if err := s.checkPipesPassRole(verified, roleARN); err != nil {
			s.writePipesError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
				err.Error(), readOnly, eventID, verified)
			return
		}
	}
	enrichment, _ := params["Enrichment"].(string)
	p, err := s.store.CreatePipeWithEnrichment(verified.AccountID, s.pipesRegion(verified), name, description, source, target, roleARN, enrichment, desired)
	if errors.Is(err, store.ErrPipeExists) {
		s.writePipesError(w, r, body, requestID, http.StatusConflict, "ConflictException",
			"Pipe already exists.", readOnly, eventID, verified)
		return
	}
	if errors.Is(err, store.ErrPipeBadReq) {
		s.writePipesError(w, r, body, requestID, http.StatusBadRequest, "ValidationException",
			err.Error(), readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writePipesError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to create pipe.", readOnly, eventID, verified)
		return
	}
	s.StartPipesTicker()
	payload, _ := pipessvc.CreatePipeJSON(p)
	s.writePipesOK(w, requestID, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, pipesEventSource, "CreatePipe", readOnly)
}

func (s *Server) pipesDescribe(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	name, _ := params["Name"].(string)
	arn := store.PipeARN(s.pipesRegion(verified), verified.AccountID, name)
	if !s.authorize(verified, catalog.ActionPipesDescribePipe, arn) {
		s.writePipesError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform pipes:DescribePipe.", readOnly, eventID, verified)
		return
	}
	p, err := s.store.DescribePipe(verified.AccountID, name)
	if errors.Is(err, store.ErrPipeNotFound) {
		s.writePipesError(w, r, body, requestID, http.StatusBadRequest, "NotFoundException",
			"Pipe not found.", readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writePipesError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to describe pipe.", readOnly, eventID, verified)
		return
	}
	payload, _ := pipessvc.DescribePipeJSON(p)
	s.writePipesOK(w, requestID, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, pipesEventSource, "DescribePipe", readOnly)
}

func (s *Server) pipesDelete(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	name, _ := params["Name"].(string)
	arn := store.PipeARN(s.pipesRegion(verified), verified.AccountID, name)
	if !s.authorize(verified, catalog.ActionPipesDeletePipe, arn) {
		s.writePipesError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform pipes:DeletePipe.", readOnly, eventID, verified)
		return
	}
	err := s.store.DeletePipe(verified.AccountID, name)
	if errors.Is(err, store.ErrPipeNotFound) {
		s.writePipesError(w, r, body, requestID, http.StatusBadRequest, "NotFoundException",
			"Pipe not found.", readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writePipesError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to delete pipe.", readOnly, eventID, verified)
		return
	}
	w.Header().Set("Content-Type", pipesJSONContentType)
	w.Header().Set("x-amzn-RequestId", requestID)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{}`))
	s.writeSuccessAudit(r, requestID, eventID, verified, pipesEventSource, "DeletePipe", readOnly)
}

func (s *Server) pipesList(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	_ = params
	if !s.authorize(verified, catalog.ActionPipesListPipes, "*") {
		s.writePipesError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform pipes:ListPipes.", readOnly, eventID, verified)
		return
	}
	pipes, err := s.store.ListPipes(verified.AccountID)
	if err != nil {
		s.writePipesError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to list pipes.", readOnly, eventID, verified)
		return
	}
	payload, _ := pipessvc.ListPipesJSON(pipes)
	s.writePipesOK(w, requestID, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, pipesEventSource, "ListPipes", readOnly)
}

func (s *Server) writePipesOK(w http.ResponseWriter, requestID string, payload []byte) {
	w.Header().Set("Content-Type", pipesJSONContentType)
	w.Header().Set("x-amzn-RequestId", requestID)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(payload)
}

func (s *Server) writePipesError(
	w http.ResponseWriter, r *http.Request, body []byte, requestID string,
	status int, code, message string, readOnly bool, eventID string, verified *authn.Verified,
) {
	w.Header().Set("Content-Type", pipesJSONContentType)
	w.Header().Set("x-amzn-RequestId", requestID)
	w.Header().Set("x-amzn-ErrorType", code)
	w.WriteHeader(status)
	_, _ = w.Write([]byte(`{"__type":"` + code + `","message":"` + message + `"}`))
	_ = body
	_ = r
	_ = readOnly
	_ = eventID
	_ = verified
}
