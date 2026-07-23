package server

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/Kyaxris-Labs/Noctaxris/internal/catalog"
	"github.com/Kyaxris-Labs/Noctaxris/internal/kernel/authn"
	"github.com/Kyaxris-Labs/Noctaxris/internal/kernel/authz"
	"github.com/Kyaxris-Labs/Noctaxris/internal/kernel/sts"
	cpsvc "github.com/Kyaxris-Labs/Noctaxris/internal/services/codepipeline"
	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

const (
	codepipelineJSONContentType = "application/x-amz-json-1.1"
	codepipelineEventSource     = "codepipeline.amazonaws.com"
)

func (s *Server) handleCodePipeline(
	w http.ResponseWriter,
	r *http.Request,
	body []byte,
	requestID, eventID, action string,
	verified *authn.Verified,
	readOnly bool,
) {
	params := jsonBodyMap(body)
	action = codepipelineAction(action)

	switch action {
	case catalog.ActionCodePipelineCreatePipeline:
		s.cpCreatePipeline(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionCodePipelineGetPipeline:
		s.cpGetPipeline(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionCodePipelineDeletePipeline:
		s.cpDeletePipeline(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionCodePipelineStartPipelineExecution:
		s.cpStartExecution(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionCodePipelineGetPipelineState:
		s.cpGetState(w, r, body, requestID, eventID, verified, readOnly, params)
	default:
		s.writeCodePipelineError(w, r, body, requestID, http.StatusNotImplemented, "InternalFailure",
			"This CodePipeline action is not implemented.", readOnly, eventID, verified)
	}
}

func codepipelineAction(action string) string {
	if strings.Contains(action, ":") {
		return action
	}
	switch action {
	case "CreatePipeline":
		return catalog.ActionCodePipelineCreatePipeline
	case "GetPipeline":
		return catalog.ActionCodePipelineGetPipeline
	case "DeletePipeline":
		return catalog.ActionCodePipelineDeletePipeline
	case "StartPipelineExecution":
		return catalog.ActionCodePipelineStartPipelineExecution
	case "GetPipelineState":
		return catalog.ActionCodePipelineGetPipelineState
	default:
		return action
	}
}

func (s *Server) checkCodePipelinePassRole(verified *authn.Verified, roleARN string) error {
	accountID, roleName, ok := sts.ParseRoleARN(roleARN)
	if !ok {
		return errors.New("roleArn must be a valid IAM role ARN")
	}
	if accountID != verified.AccountID {
		return errors.New("roleArn must be in the same account")
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
		return errors.New("not authorized to pass role to CodePipeline")
	}
	decision := authz.CheckPassRole(authz.PassRoleRequest{
		Caller: authz.RequestContext{
			Principal:     verified.Principal,
			Resource:      roleARN,
			Region:        verified.Region,
			ConditionKeys: s.conditionKeys(verified),
		},
		EvalInputs:       in,
		RoleARN:          roleARN,
		TrustPolicyDoc:   trust,
		ServicePrincipal: authz.ServicePrincipalCodePipeline,
	})
	if decision != authz.Allow {
		return errors.New("not authorized to pass role to CodePipeline")
	}
	return nil
}

func (s *Server) cpCreatePipeline(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	pipeRaw, _ := json.Marshal(params["pipeline"])
	var decl store.CodePipelineDeclaration
	if err := json.Unmarshal(pipeRaw, &decl); err != nil || strings.TrimSpace(decl.Name) == "" {
		s.writeCodePipelineError(w, r, body, requestID, http.StatusBadRequest, "InvalidStructureException",
			"pipeline with name is required.", readOnly, eventID, verified)
		return
	}
	if !s.authorize(verified, catalog.ActionCodePipelineCreatePipeline, "*") {
		s.writeCodePipelineError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform codepipeline:CreatePipeline.", readOnly, eventID, verified)
		return
	}
	if strings.TrimSpace(decl.RoleARN) != "" {
		if err := s.checkCodePipelinePassRole(verified, decl.RoleARN); err != nil {
			s.writeCodePipelineError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
				err.Error(), readOnly, eventID, verified)
			return
		}
	}
	region := verified.Region
	if region == "" {
		region = store.DefaultCodePipelineRegion
	}
	p, err := s.store.CreateCodePipeline(verified.AccountID, region, decl)
	if errors.Is(err, store.ErrCodePipelineExists) {
		s.writeCodePipelineError(w, r, body, requestID, http.StatusBadRequest, "PipelineNameInUseException",
			"Pipeline already exists.", readOnly, eventID, verified)
		return
	}
	if errors.Is(err, store.ErrCodePipelineBadReq) {
		s.writeCodePipelineError(w, r, body, requestID, http.StatusBadRequest, "InvalidStructureException",
			err.Error(), readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeCodePipelineError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to create pipeline.", readOnly, eventID, verified)
		return
	}
	payload, _ := cpsvc.CreatePipelineJSON(p)
	s.writeCodePipelineOK(w, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, codepipelineEventSource, "CreatePipeline", readOnly)
}

func (s *Server) cpGetPipeline(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	name, _ := params["name"].(string)
	if strings.TrimSpace(name) == "" {
		s.writeCodePipelineError(w, r, body, requestID, http.StatusBadRequest, "ValidationException",
			"name is required.", readOnly, eventID, verified)
		return
	}
	if !s.authorize(verified, catalog.ActionCodePipelineGetPipeline, "*") {
		s.writeCodePipelineError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform codepipeline:GetPipeline.", readOnly, eventID, verified)
		return
	}
	p, err := s.store.GetCodePipeline(verified.AccountID, name)
	if errors.Is(err, store.ErrCodePipelineNotFound) {
		s.writeCodePipelineError(w, r, body, requestID, http.StatusBadRequest, "PipelineNotFoundException",
			"Pipeline not found.", readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeCodePipelineError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to get pipeline.", readOnly, eventID, verified)
		return
	}
	payload, _ := cpsvc.GetPipelineJSON(p)
	s.writeCodePipelineOK(w, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, codepipelineEventSource, "GetPipeline", readOnly)
}

func (s *Server) cpDeletePipeline(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	name, _ := params["name"].(string)
	if strings.TrimSpace(name) == "" {
		s.writeCodePipelineError(w, r, body, requestID, http.StatusBadRequest, "ValidationException",
			"name is required.", readOnly, eventID, verified)
		return
	}
	if !s.authorize(verified, catalog.ActionCodePipelineDeletePipeline, "*") {
		s.writeCodePipelineError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform codepipeline:DeletePipeline.", readOnly, eventID, verified)
		return
	}
	err := s.store.DeleteCodePipeline(verified.AccountID, name)
	if errors.Is(err, store.ErrCodePipelineNotFound) {
		s.writeCodePipelineError(w, r, body, requestID, http.StatusBadRequest, "PipelineNotFoundException",
			"Pipeline not found.", readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeCodePipelineError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to delete pipeline.", readOnly, eventID, verified)
		return
	}
	payload, _ := cpsvc.DeletePipelineJSON()
	s.writeCodePipelineOK(w, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, codepipelineEventSource, "DeletePipeline", readOnly)
}

func (s *Server) cpStartExecution(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	name, _ := params["name"].(string)
	if strings.TrimSpace(name) == "" {
		s.writeCodePipelineError(w, r, body, requestID, http.StatusBadRequest, "ValidationException",
			"name is required.", readOnly, eventID, verified)
		return
	}
	if !s.authorize(verified, catalog.ActionCodePipelineStartPipelineExecution, "*") {
		s.writeCodePipelineError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform codepipeline:StartPipelineExecution.", readOnly, eventID, verified)
		return
	}
	region := verified.Region
	if region == "" {
		region = store.DefaultCodeBuildRegion
	}
	runBuild := func(projectName string) (buildID, status string, err error) {
		b, err := s.store.StartCodeBuildBuild(verified.AccountID, region, projectName, "")
		if err != nil {
			return "", "Failed", err
		}
		if strings.TrimSpace(s.cfg.DockerHost) == "" {
			end := time.Now().UTC().Format(time.RFC3339)
			_ = s.store.SetCodeBuildBuildRuntime(verified.AccountID, b.ID, "", store.CodeBuildStatusFailed, end)
			return b.ID, store.CodeBuildStatusFailed, errors.New("compute unavailable")
		}
		if err := s.startCodeBuildContainer(r.Context(), verified.AccountID, b); err != nil {
			end := time.Now().UTC().Format(time.RFC3339)
			_ = s.store.SetCodeBuildBuildRuntime(verified.AccountID, b.ID, "", store.CodeBuildStatusFailed, end)
			return b.ID, store.CodeBuildStatusFailed, err
		}
		builds, getErr := s.store.BatchGetCodeBuildBuilds(verified.AccountID, []string{b.ID})
		if getErr != nil || len(builds) == 0 {
			return b.ID, store.CodeBuildStatusSucceeded, nil
		}
		return builds[0].ID, builds[0].BuildStatus, nil
	}
	e, err := s.store.StartCodePipelineExecution(verified.AccountID, name, runBuild)
	if errors.Is(err, store.ErrCodePipelineNotFound) {
		s.writeCodePipelineError(w, r, body, requestID, http.StatusBadRequest, "PipelineNotFoundException",
			"Pipeline not found.", readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeCodePipelineError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to start execution.", readOnly, eventID, verified)
		return
	}
	payload, _ := cpsvc.StartPipelineExecutionJSON(e)
	s.writeCodePipelineOK(w, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, codepipelineEventSource, "StartPipelineExecution", readOnly)
}

func (s *Server) cpGetState(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	name, _ := params["name"].(string)
	if strings.TrimSpace(name) == "" {
		s.writeCodePipelineError(w, r, body, requestID, http.StatusBadRequest, "ValidationException",
			"name is required.", readOnly, eventID, verified)
		return
	}
	if !s.authorize(verified, catalog.ActionCodePipelineGetPipelineState, "*") {
		s.writeCodePipelineError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform codepipeline:GetPipelineState.", readOnly, eventID, verified)
		return
	}
	e, err := s.store.GetCodePipelineState(verified.AccountID, name)
	if errors.Is(err, store.ErrCodePipelineNotFound) {
		s.writeCodePipelineError(w, r, body, requestID, http.StatusBadRequest, "PipelineNotFoundException",
			"Pipeline not found.", readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeCodePipelineError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to get pipeline state.", readOnly, eventID, verified)
		return
	}
	payload, _ := cpsvc.GetPipelineStateJSON(name, e)
	s.writeCodePipelineOK(w, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, codepipelineEventSource, "GetPipelineState", readOnly)
}

func (s *Server) writeCodePipelineOK(w http.ResponseWriter, payload []byte) {
	w.Header().Set("Content-Type", codepipelineJSONContentType)
	w.Header().Set("x-amzn-RequestId", "req")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(payload)
}

func (s *Server) writeCodePipelineError(
	w http.ResponseWriter, r *http.Request, body []byte, requestID string, status int, code, message string,
	readOnly bool, eventID string, verified *authn.Verified,
) {
	w.Header().Set("Content-Type", codepipelineJSONContentType)
	w.Header().Set("x-amzn-ErrorType", code)
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"__type": code, "message": message})
	_ = body
	s.writeSuccessAudit(r, requestID, eventID, verified, codepipelineEventSource, code, readOnly)
}
