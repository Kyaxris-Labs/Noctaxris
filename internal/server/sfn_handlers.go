package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/Kyaxris-Labs/Noctaxris/internal/catalog"
	"github.com/Kyaxris-Labs/Noctaxris/internal/kernel/authn"
	"github.com/Kyaxris-Labs/Noctaxris/internal/kernel/authz"
	sfnsvc "github.com/Kyaxris-Labs/Noctaxris/internal/services/sfn"
	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
	"github.com/Kyaxris-Labs/Noctaxris/internal/kernel/sts"
)

const (
	sfnJSONContentType = "application/x-amz-json-1.0"
	sfnEventSource     = "states.amazonaws.com"
)

func (s *Server) handleSFN(
	w http.ResponseWriter,
	r *http.Request,
	body []byte,
	requestID, eventID, action string,
	verified *authn.Verified,
	readOnly bool,
) {
	params := jsonBodyMap(body)
	action = sfnAction(action)

	switch action {
	case catalog.ActionSFNCreateStateMachine:
		s.sfnCreateStateMachine(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionSFNDeleteStateMachine:
		s.sfnDeleteStateMachine(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionSFNDescribeStateMachine:
		s.sfnDescribeStateMachine(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionSFNListStateMachines:
		s.sfnListStateMachines(w, r, body, requestID, eventID, verified, readOnly)
	case catalog.ActionSFNStartExecution:
		s.sfnStartExecution(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionSFNDescribeExecution:
		s.sfnDescribeExecution(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionSFNGetExecutionHistory:
		s.sfnGetExecutionHistory(w, r, body, requestID, eventID, verified, readOnly, params)
	default:
		s.writeSFNError(w, r, body, requestID, http.StatusNotImplemented, "InternalFailure",
			"This Step Functions action is not implemented.", readOnly, eventID, verified)
	}
}

func sfnAction(action string) string {
	if strings.Contains(action, ":") {
		return action
	}
	switch action {
	case "CreateStateMachine":
		return catalog.ActionSFNCreateStateMachine
	case "DeleteStateMachine":
		return catalog.ActionSFNDeleteStateMachine
	case "DescribeStateMachine":
		return catalog.ActionSFNDescribeStateMachine
	case "ListStateMachines":
		return catalog.ActionSFNListStateMachines
	case "StartExecution":
		return catalog.ActionSFNStartExecution
	case "DescribeExecution":
		return catalog.ActionSFNDescribeExecution
	case "GetExecutionHistory":
		return catalog.ActionSFNGetExecutionHistory
	default:
		return action
	}
}

func (s *Server) sfnRegion(verified *authn.Verified) string {
	if verified.Region != "" {
		return verified.Region
	}
	return store.DefaultSFNRegion
}

func (s *Server) checkSFNPassRole(verified *authn.Verified, roleARN string) error {
	accountID, roleName, ok := sts.ParseRoleARN(roleARN)
	if !ok {
		return errors.New("RoleArn must be a valid IAM role ARN")
	}
	if accountID != verified.AccountID {
		return errors.New("RoleArn must be in the same account")
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
		return errors.New("not authorized to pass role to Step Functions")
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
		ServicePrincipal: authz.ServicePrincipalStates,
	})
	if decision != authz.Allow {
		return errors.New("not authorized to pass role to Step Functions")
	}
	return nil
}

func (s *Server) sfnCreateStateMachine(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	name, _ := params["name"].(string)
	definition, _ := params["definition"].(string)
	roleARN, _ := params["roleArn"].(string)
	if strings.TrimSpace(name) == "" || strings.TrimSpace(definition) == "" {
		s.writeSFNError(w, r, body, requestID, http.StatusBadRequest, "InvalidDefinition",
			"name and definition are required.", readOnly, eventID, verified)
		return
	}
	arn := store.SFNStateMachineARN(s.sfnRegion(verified), verified.AccountID, name)
	if !s.authorize(verified, catalog.ActionSFNCreateStateMachine, arn) {
		s.writeSFNError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform states:CreateStateMachine.", readOnly, eventID, verified)
		return
	}
	if strings.TrimSpace(roleARN) != "" {
		if err := s.checkSFNPassRole(verified, roleARN); err != nil {
			s.writeSFNError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
				err.Error(), readOnly, eventID, verified)
			return
		}
	}
	sm, err := s.store.CreateSFNStateMachine(verified.AccountID, s.sfnRegion(verified), name, definition, roleARN)
	if errors.Is(err, store.ErrSFNStateMachineExists) {
		s.writeSFNError(w, r, body, requestID, http.StatusBadRequest, "StateMachineAlreadyExists",
			"State machine already exists.", readOnly, eventID, verified)
		return
	}
	if errors.Is(err, store.ErrSFNInvalidDefinition) || (err != nil && strings.Contains(err.Error(), "InvalidDefinition")) {
		s.writeSFNError(w, r, body, requestID, http.StatusBadRequest, "InvalidDefinition",
			err.Error(), readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeSFNError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to create state machine.", readOnly, eventID, verified)
		return
	}
	payload, _ := sfnsvc.CreateStateMachineJSON(sm)
	s.writeSFNOK(w, requestID, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, sfnEventSource, "CreateStateMachine", readOnly)
}

func (s *Server) sfnDeleteStateMachine(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	arn, _ := params["stateMachineArn"].(string)
	if strings.TrimSpace(arn) == "" {
		s.writeSFNError(w, r, body, requestID, http.StatusBadRequest, "InvalidArn",
			"stateMachineArn is required.", readOnly, eventID, verified)
		return
	}
	if !s.authorize(verified, catalog.ActionSFNDeleteStateMachine, arn) {
		s.writeSFNError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform states:DeleteStateMachine.", readOnly, eventID, verified)
		return
	}
	err := s.store.DeleteSFNStateMachine(verified.AccountID, arn)
	if errors.Is(err, store.ErrSFNStateMachineNotFound) {
		s.writeSFNError(w, r, body, requestID, http.StatusBadRequest, "StateMachineDoesNotExist",
			"State machine does not exist.", readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeSFNError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to delete state machine.", readOnly, eventID, verified)
		return
	}
	payload, _ := sfnsvc.DeleteStateMachineJSON()
	s.writeSFNOK(w, requestID, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, sfnEventSource, "DeleteStateMachine", readOnly)
}

func (s *Server) sfnDescribeStateMachine(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	arn, _ := params["stateMachineArn"].(string)
	if strings.TrimSpace(arn) == "" {
		s.writeSFNError(w, r, body, requestID, http.StatusBadRequest, "InvalidArn",
			"stateMachineArn is required.", readOnly, eventID, verified)
		return
	}
	if !s.authorize(verified, catalog.ActionSFNDescribeStateMachine, arn) {
		s.writeSFNError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform states:DescribeStateMachine.", readOnly, eventID, verified)
		return
	}
	sm, err := s.store.GetSFNStateMachine(verified.AccountID, arn)
	if errors.Is(err, store.ErrSFNStateMachineNotFound) {
		s.writeSFNError(w, r, body, requestID, http.StatusBadRequest, "StateMachineDoesNotExist",
			"State machine does not exist.", readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeSFNError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to describe state machine.", readOnly, eventID, verified)
		return
	}
	payload, _ := sfnsvc.DescribeStateMachineJSON(sm)
	s.writeSFNOK(w, requestID, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, sfnEventSource, "DescribeStateMachine", readOnly)
}

func (s *Server) sfnListStateMachines(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool,
) {
	if !s.authorize(verified, catalog.ActionSFNListStateMachines, "*") {
		s.writeSFNError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform states:ListStateMachines.", readOnly, eventID, verified)
		return
	}
	machines, err := s.store.ListSFNStateMachines(verified.AccountID)
	if err != nil {
		s.writeSFNError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to list state machines.", readOnly, eventID, verified)
		return
	}
	payload, _ := sfnsvc.ListStateMachinesJSON(machines)
	s.writeSFNOK(w, requestID, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, sfnEventSource, "ListStateMachines", readOnly)
}

func (s *Server) sfnStartExecution(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	smARN, _ := params["stateMachineArn"].(string)
	name, _ := params["name"].(string)
	input, _ := params["input"].(string)
	if strings.TrimSpace(smARN) == "" {
		s.writeSFNError(w, r, body, requestID, http.StatusBadRequest, "InvalidArn",
			"stateMachineArn is required.", readOnly, eventID, verified)
		return
	}
	if !s.authorize(verified, catalog.ActionSFNStartExecution, smARN) {
		s.writeSFNError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform states:StartExecution.", readOnly, eventID, verified)
		return
	}
	invoke := func(resourceARN, inputJSON string) (string, error) {
		return s.sfnInvokeLambdaTask(r, verified, resourceARN, inputJSON)
	}
	exec, err := s.store.StartSFNExecution(verified.AccountID, s.sfnRegion(verified), smARN, name, input, invoke)
	if errors.Is(err, store.ErrSFNStateMachineNotFound) {
		s.writeSFNError(w, r, body, requestID, http.StatusBadRequest, "StateMachineDoesNotExist",
			"State machine does not exist.", readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeSFNError(w, r, body, requestID, http.StatusBadRequest, "InvalidExecutionInput",
			err.Error(), readOnly, eventID, verified)
		return
	}
	payload, _ := sfnsvc.StartExecutionJSON(exec)
	s.writeSFNOK(w, requestID, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, sfnEventSource, "StartExecution", readOnly)
}

func (s *Server) sfnInvokeLambdaTask(r *http.Request, verified *authn.Verified, resourceARN, inputJSON string) (string, error) {
	accountID, functionName, ok := store.ParseLambdaARNFromSFNResource(resourceARN)
	if !ok {
		return "", fmt.Errorf("Task Resource must be a Lambda function ARN or name (SQS/SNS/EventBridge bus Tasks are handled in-store)")
	}
	if accountID == "" {
		accountID = verified.AccountID
	}
	fn, executedVersion, err := s.store.ResolveFunction(accountID, functionName, "$LATEST")
	if err != nil {
		return "", fmt.Errorf("resolve function: %w", err)
	}
	if !s.authorizeLambda(verified, catalog.ActionLambdaInvoke, fn.FunctionARN, fn.ResourcePolicy) {
		return "", fmt.Errorf("not authorized to invoke %s", fn.FunctionARN)
	}
	result, err := s.executeLambdaInvoke(r.Context(), accountID, functionName, fn, executedVersion, inputJSON)
	if err != nil {
		return "", err
	}
	return string(result), nil
}

func (s *Server) sfnDescribeExecution(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	arn, _ := params["executionArn"].(string)
	if strings.TrimSpace(arn) == "" {
		s.writeSFNError(w, r, body, requestID, http.StatusBadRequest, "InvalidArn",
			"executionArn is required.", readOnly, eventID, verified)
		return
	}
	if !s.authorize(verified, catalog.ActionSFNDescribeExecution, arn) {
		s.writeSFNError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform states:DescribeExecution.", readOnly, eventID, verified)
		return
	}
	exec, err := s.store.DescribeSFNExecution(arn)
	if errors.Is(err, store.ErrSFNExecutionNotFound) {
		s.writeSFNError(w, r, body, requestID, http.StatusBadRequest, "ExecutionDoesNotExist",
			"Execution does not exist.", readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeSFNError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to describe execution.", readOnly, eventID, verified)
		return
	}
	payload, _ := sfnsvc.DescribeExecutionJSON(exec)
	s.writeSFNOK(w, requestID, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, sfnEventSource, "DescribeExecution", readOnly)
}

func (s *Server) sfnGetExecutionHistory(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	arn, _ := params["executionArn"].(string)
	if strings.TrimSpace(arn) == "" {
		s.writeSFNError(w, r, body, requestID, http.StatusBadRequest, "InvalidArn",
			"executionArn is required.", readOnly, eventID, verified)
		return
	}
	if !s.authorize(verified, catalog.ActionSFNGetExecutionHistory, arn) {
		s.writeSFNError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform states:GetExecutionHistory.", readOnly, eventID, verified)
		return
	}
	events, err := s.store.GetSFNExecutionHistory(arn)
	if errors.Is(err, store.ErrSFNExecutionNotFound) {
		s.writeSFNError(w, r, body, requestID, http.StatusBadRequest, "ExecutionDoesNotExist",
			"Execution does not exist.", readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeSFNError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to get execution history.", readOnly, eventID, verified)
		return
	}
	payload, _ := sfnsvc.GetExecutionHistoryJSON(events)
	s.writeSFNOK(w, requestID, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, sfnEventSource, "GetExecutionHistory", readOnly)
}

func (s *Server) writeSFNOK(w http.ResponseWriter, requestID string, payload []byte) {
	w.Header().Set(requestIDHeader, requestID)
	w.Header().Set("Content-Type", sfnJSONContentType)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(payload)
}

func (s *Server) writeSFNError(
	w http.ResponseWriter, r *http.Request, body []byte, requestID string,
	status int, code, message string, readOnly bool, eventID string, verified *authn.Verified,
) {
	_ = body
	w.Header().Set(requestIDHeader, requestID)
	w.Header().Set("Content-Type", sfnJSONContentType)
	w.WriteHeader(status)
	payload, _ := json.Marshal(map[string]string{"__type": code, "message": message})
	_, _ = w.Write(payload)
	accessKeyID := ""
	accountID := ""
	if verified != nil {
		accessKeyID = verified.AccessKeyID
		accountID = verified.AccountID
	}
	s.auditAPIError(r, requestID, eventID, code, message, readOnly, accessKeyID, accountID, verified != nil)
}
