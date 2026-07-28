package server

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/Kyaxris-Labs/Noctaxris/internal/catalog"
	"github.com/Kyaxris-Labs/Noctaxris/internal/compute"
	"github.com/Kyaxris-Labs/Noctaxris/internal/kernel/authn"
	ssmsvc "github.com/Kyaxris-Labs/Noctaxris/internal/services/ssm"
	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

// ssmSendCommand implements AmazonSSM.SendCommand for AWS-RunShellScript via nested DinD exec.
func (s *Server) ssmSendCommand(
	w http.ResponseWriter,
	r *http.Request,
	body []byte,
	requestID, eventID string,
	verified *authn.Verified,
	readOnly bool,
	params map[string]any,
) {
	if !s.authorizeSSM(verified, catalog.ActionSSMSendCommand, "*") {
		s.writeSSMError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform ssm:SendCommand.", readOnly, eventID, verified)
		return
	}

	documentName, _ := params["DocumentName"].(string)
	documentName = strings.TrimSpace(documentName)
	if documentName == "" {
		s.writeSSMError(w, r, body, requestID, http.StatusBadRequest, "InvalidDocument",
			"DocumentName is required.", readOnly, eventID, verified)
		return
	}
	if documentName != store.SSMDocumentRunShellScript {
		s.writeSSMError(w, r, body, requestID, http.StatusBadRequest, "InvalidDocument",
			"Only document AWS-RunShellScript is supported.", readOnly, eventID, verified)
		return
	}

	instanceIDs := stringSliceParam(params["InstanceIds"])
	if len(instanceIDs) == 0 {
		s.writeSSMError(w, r, body, requestID, http.StatusBadRequest, "InvalidInstanceId",
			"At least one InstanceId is required.", readOnly, eventID, verified)
		return
	}

	region := s.ssmRegion(verified)
	for _, id := range instanceIDs {
		if _, err := s.store.GetEC2Instance(verified.AccountID, region, id); err != nil {
			if errors.Is(err, store.ErrEC2NotFound) {
				s.writeSSMError(w, r, body, requestID, http.StatusBadRequest, "InvalidInstanceId",
					"Instance "+id+" does not exist.", readOnly, eventID, verified)
				return
			}
			s.writeSSMError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
				"Unable to resolve instance.", readOnly, eventID, verified)
			return
		}
	}

	timeoutSeconds := ssmIntParam(params["TimeoutSeconds"], store.SSMDefaultTimeoutSeconds)
	if timeoutSeconds < store.SSMMinTimeoutSeconds {
		s.writeSSMError(w, r, body, requestID, http.StatusBadRequest, "ValidationException",
			fmt.Sprintf("1 validation error detected: Value '%d' at 'timeoutSeconds' failed to satisfy constraint: Member must have value greater than or equal to %d",
				timeoutSeconds, store.SSMMinTimeoutSeconds),
			readOnly, eventID, verified)
		return
	}

	comment, _ := params["Comment"].(string)
	docVersion, _ := params["DocumentVersion"].(string)
	parameters := ssmParametersMap(params["Parameters"])

	cmd, _, err := s.store.CreateSSMCommand(
		verified.AccountID, region, documentName, docVersion, comment, parameters, instanceIDs, timeoutSeconds,
	)
	if err != nil {
		if strings.Contains(err.Error(), "ValidationException") {
			s.writeSSMError(w, r, body, requestID, http.StatusBadRequest, "ValidationException",
				err.Error(), readOnly, eventID, verified)
			return
		}
		s.writeSSMError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to send command.", readOnly, eventID, verified)
		return
	}

	for _, instanceID := range instanceIDs {
		iid := instanceID
		go s.runSSMDirectCommand(verified.AccountID, region, cmd.CommandID, iid, parameters, timeoutSeconds)
	}

	payload, err := ssmsvc.SendCommandJSON(cmd)
	if err != nil {
		s.writeSSMError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to build response.", readOnly, eventID, verified)
		return
	}
	s.writeSSMOK(w, requestID, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, ssmEventSource, "SendCommand", readOnly)
}

func (s *Server) ssmGetCommandInvocation(
	w http.ResponseWriter,
	r *http.Request,
	body []byte,
	requestID, eventID string,
	verified *authn.Verified,
	readOnly bool,
	params map[string]any,
) {
	if !s.authorizeSSM(verified, catalog.ActionSSMGetCommandInvocation, "*") {
		s.writeSSMError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform ssm:GetCommandInvocation.", readOnly, eventID, verified)
		return
	}

	commandID, _ := params["CommandId"].(string)
	instanceID, _ := params["InstanceId"].(string)
	if strings.TrimSpace(commandID) == "" || strings.TrimSpace(instanceID) == "" {
		s.writeSSMError(w, r, body, requestID, http.StatusBadRequest, "ValidationException",
			"CommandId and InstanceId are required.", readOnly, eventID, verified)
		return
	}

	inv, err := s.store.GetSSMCommandInvocation(verified.AccountID, s.ssmRegion(verified), commandID, instanceID)
	if errors.Is(err, store.ErrSSMInvocationNotFound) {
		s.writeSSMError(w, r, body, requestID, http.StatusBadRequest, "InvocationDoesNotExist",
			"Command "+commandID+" on instance "+instanceID+" does not exist.", readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeSSMError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to get command invocation.", readOnly, eventID, verified)
		return
	}

	payload, err := ssmsvc.GetCommandInvocationJSON(inv)
	if err != nil {
		s.writeSSMError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to build response.", readOnly, eventID, verified)
		return
	}
	s.writeSSMOK(w, requestID, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, ssmEventSource, "GetCommandInvocation", readOnly)
}

func (s *Server) ssmListCommandInvocations(
	w http.ResponseWriter,
	r *http.Request,
	body []byte,
	requestID, eventID string,
	verified *authn.Verified,
	readOnly bool,
	params map[string]any,
) {
	if !s.authorizeSSM(verified, catalog.ActionSSMListCommandInvocations, "*") {
		s.writeSSMError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform ssm:ListCommandInvocations.", readOnly, eventID, verified)
		return
	}

	commandID, _ := params["CommandId"].(string)
	instanceID, _ := params["InstanceId"].(string)
	list, err := s.store.ListSSMCommandInvocations(verified.AccountID, s.ssmRegion(verified), commandID, instanceID)
	if err != nil {
		s.writeSSMError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to list command invocations.", readOnly, eventID, verified)
		return
	}

	payload, err := ssmsvc.ListCommandInvocationsJSON(list)
	if err != nil {
		s.writeSSMError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to build response.", readOnly, eventID, verified)
		return
	}
	s.writeSSMOK(w, requestID, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, ssmEventSource, "ListCommandInvocations", readOnly)
}

func (s *Server) runSSMDirectCommand(
	accountID, region, commandID, instanceID string,
	parameters map[string][]string,
	timeoutSeconds int,
) {
	start := time.Now().UTC().Format(time.RFC3339)
	_ = s.store.MarkSSMCommandInvocationInProgress(accountID, region, commandID, instanceID, start)

	commands := parameters["commands"]
	result, status, errMsg := s.execSSMShellOnInstance(accountID, region, instanceID, commands, timeoutSeconds)
	end := time.Now().UTC().Format(time.RFC3339)

	stdout := result.Stdout
	stderr := result.Stderr
	code := result.ExitCode
	if errMsg != "" && stderr == "" {
		stderr = errMsg
	}
	if status == "" {
		if code == 0 {
			status = store.SSMCommandStatusSuccess
		} else {
			status = store.SSMCommandStatusFailed
		}
	}
	_ = s.store.UpdateSSMCommandInvocationStatus(
		accountID, region, commandID, instanceID, status, status, stdout, stderr, code, start, end,
	)
}

func (s *Server) execSSMShellOnInstance(
	accountID, region, instanceID string,
	commands []string,
	timeoutSeconds int,
) (compute.ExecResult, string, string) {
	inst, err := s.store.GetEC2Instance(accountID, region, instanceID)
	if err != nil {
		return compute.ExecResult{ExitCode: 1}, store.SSMCommandStatusFailed, "instance not found"
	}
	if strings.TrimSpace(inst.ContainerID) == "" {
		return compute.ExecResult{ExitCode: 1}, store.SSMCommandStatusFailed,
			"instance has no nested container; start the instance before SendCommand"
	}
	if inst.StateName != store.EC2StateRunning {
		return compute.ExecResult{ExitCode: 1}, store.SSMCommandStatusFailed,
			"instance is not running (state=" + inst.StateName + ")"
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeoutSeconds+5)*time.Second)
	defer cancel()

	if s.ssmExecHook != nil {
		res, err := s.ssmExecHook(ctx, inst.ContainerID, commands, timeoutSeconds)
		if err != nil {
			if errors.Is(err, context.DeadlineExceeded) || errors.Is(ctx.Err(), context.DeadlineExceeded) {
				return res, store.SSMCommandStatusTimedOut, "Timed out after " + fmt.Sprintf("%d", timeoutSeconds) + "s"
			}
			return compute.ExecResult{ExitCode: 1, Stderr: err.Error()}, store.SSMCommandStatusFailed, err.Error()
		}
		return res, "", ""
	}

	cli, err := s.computeClient()
	if err != nil || cli == nil {
		msg := "nested compute unavailable"
		if err != nil {
			msg = err.Error()
		}
		return compute.ExecResult{ExitCode: 1}, store.SSMCommandStatusFailed, msg
	}

	script := strings.Join(commands, "\n")
	cmd := []string{"sh", "-c", script}
	if script == "" {
		cmd = []string{"sh", "-c", "true"}
	}

	execCtx, execCancel := context.WithTimeout(ctx, time.Duration(timeoutSeconds)*time.Second)
	defer execCancel()

	res, err := cli.Exec(execCtx, compute.ExecOpts{
		ContainerID: inst.ContainerID,
		Cmd:         cmd,
	})
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(execCtx.Err(), context.DeadlineExceeded) {
			return res, store.SSMCommandStatusTimedOut, "Timed out after " + fmt.Sprintf("%d", timeoutSeconds) + "s"
		}
		return compute.ExecResult{ExitCode: 1, Stderr: err.Error()}, store.SSMCommandStatusFailed, err.Error()
	}
	return res, "", ""
}

func ssmParametersMap(v any) map[string][]string {
	out := map[string][]string{}
	m, ok := v.(map[string]any)
	if !ok || m == nil {
		return out
	}
	for k, raw := range m {
		out[k] = stringSliceParam(raw)
		if len(out[k]) == 0 {
			if s, ok := raw.(string); ok && s != "" {
				out[k] = []string{s}
			}
		}
	}
	return out
}

func ssmIntParam(v any, def int) int {
	switch t := v.(type) {
	case float64:
		return int(t)
	case int:
		return t
	case int64:
		return int(t)
	default:
		return def
	}
}
