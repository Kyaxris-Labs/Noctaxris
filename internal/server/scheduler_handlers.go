package server

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/Kyaxris-Labs/Noctaxris/internal/catalog"
	"github.com/Kyaxris-Labs/Noctaxris/internal/kernel/authn"
	"github.com/Kyaxris-Labs/Noctaxris/internal/kernel/authz"
	"github.com/Kyaxris-Labs/Noctaxris/internal/kernel/sts"
	schedsvc "github.com/Kyaxris-Labs/Noctaxris/internal/services/scheduler"
	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

const (
	schedulerJSONContentType = "application/x-amz-json-1.1"
	schedulerEventSource     = "scheduler.amazonaws.com"
)

func (s *Server) handleScheduler(
	w http.ResponseWriter,
	r *http.Request,
	body []byte,
	requestID, eventID, action string,
	verified *authn.Verified,
	readOnly bool,
) {
	params := jsonBodyMap(body)
	action = schedulerAction(action)

	switch action {
	case catalog.ActionSchedulerCreateSchedule:
		s.schedulerCreate(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionSchedulerGetSchedule:
		s.schedulerGet(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionSchedulerUpdateSchedule:
		s.schedulerUpdate(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionSchedulerDeleteSchedule:
		s.schedulerDelete(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionSchedulerListSchedules:
		s.schedulerList(w, r, body, requestID, eventID, verified, readOnly, params)
	default:
		s.writeSchedulerError(w, r, body, requestID, http.StatusNotImplemented, "InternalFailure",
			"This Scheduler action is not implemented.", readOnly, eventID, verified)
	}
}

func schedulerAction(action string) string {
	if strings.Contains(action, ":") {
		return action
	}
	switch action {
	case "CreateSchedule":
		return catalog.ActionSchedulerCreateSchedule
	case "GetSchedule":
		return catalog.ActionSchedulerGetSchedule
	case "UpdateSchedule":
		return catalog.ActionSchedulerUpdateSchedule
	case "DeleteSchedule":
		return catalog.ActionSchedulerDeleteSchedule
	case "ListSchedules":
		return catalog.ActionSchedulerListSchedules
	default:
		return action
	}
}

func (s *Server) schedulerRegion(verified *authn.Verified) string {
	if verified != nil && verified.Region != "" {
		return verified.Region
	}
	return store.DefaultSchedulerRegion
}

func (s *Server) schedulerARN(verified *authn.Verified, group, name string) string {
	return store.ScheduleARN(s.schedulerRegion(verified), verified.AccountID, group, name)
}

func (s *Server) checkSchedulerPassRole(verified *authn.Verified, roleARN string) error {
	accountID, roleName, ok := sts.ParseRoleARN(roleARN)
	if !ok {
		return errors.New("RoleArn must be a valid IAM role ARN")
	}
	if accountID != verified.AccountID {
		return errors.New("RoleArn must be in the same account")
	}
	storedARN, trust, err := s.store.GetRole(accountID, roleName)
	if err != nil {
		return errors.New("RoleArn not found")
	}
	if storedARN != "" {
		roleARN = storedARN
	}
	in, ok := s.evalInputs(verified)
	if !ok {
		return errors.New("not authorized to pass role to Scheduler")
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
		ServicePrincipal: authz.ServicePrincipalScheduler,
	})
	if decision != authz.Allow {
		return errors.New("not authorized to pass role to Scheduler")
	}
	return nil
}

func parseSchedulerTarget(params map[string]any) (arn, roleARN, input string) {
	raw, ok := params["Target"].(map[string]any)
	if !ok {
		return "", "", ""
	}
	return stringParam(raw["Arn"]), stringParam(raw["RoleArn"]), stringParam(raw["Input"])
}

func (s *Server) schedulerCreate(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	name := stringParam(params["Name"])
	if name == "" {
		s.writeSchedulerError(w, r, body, requestID, http.StatusBadRequest, "ValidationException",
			"Name is required.", readOnly, eventID, verified)
		return
	}
	group := stringParam(params["GroupName"])
	expr := stringParam(params["ScheduleExpression"])
	state := stringParam(params["State"])
	targetARN, roleARN, input := parseSchedulerTarget(params)
	resource := s.schedulerARN(verified, group, name)
	if !s.authorize(verified, catalog.ActionSchedulerCreateSchedule, resource) {
		s.writeSchedulerError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform scheduler:CreateSchedule.", readOnly, eventID, verified)
		return
	}
	if roleARN != "" {
		if err := s.checkSchedulerPassRole(verified, roleARN); err != nil {
			s.writeSchedulerError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
				err.Error(), readOnly, eventID, verified)
			return
		}
	}
	sch, err := s.store.CreateSchedule(verified.AccountID, s.schedulerRegion(verified), store.CreateScheduleInput{
		Name:       name,
		GroupName:  group,
		Expression: expr,
		State:      state,
		TargetARN:  targetARN,
		RoleARN:    roleARN,
		Input:      input,
	}, s.now().UTC())
	if errors.Is(err, store.ErrScheduleAlreadyExists) {
		s.writeSchedulerError(w, r, body, requestID, http.StatusConflict, "ConflictException",
			"Schedule already exists.", readOnly, eventID, verified)
		return
	}
	if errors.Is(err, store.ErrInvalidScheduleExpr) || errors.Is(err, store.ErrInvalidScheduleTarget) {
		s.writeSchedulerError(w, r, body, requestID, http.StatusBadRequest, "ValidationException",
			err.Error(), readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeSchedulerError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to create schedule.", readOnly, eventID, verified)
		return
	}
	payload, err := schedsvc.CreateScheduleJSON(sch)
	if err != nil {
		s.writeSchedulerError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to build response.", readOnly, eventID, verified)
		return
	}
	s.writeSchedulerOK(w, requestID, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, schedulerEventSource, "CreateSchedule", readOnly)
}

func (s *Server) schedulerGet(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	name := stringParam(params["Name"])
	group := stringParam(params["GroupName"])
	resource := s.schedulerARN(verified, group, name)
	if !s.authorize(verified, catalog.ActionSchedulerGetSchedule, resource) {
		s.writeSchedulerError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform scheduler:GetSchedule.", readOnly, eventID, verified)
		return
	}
	sch, err := s.store.GetSchedule(verified.AccountID, group, name)
	if errors.Is(err, store.ErrNoSuchSchedule) {
		s.writeSchedulerError(w, r, body, requestID, http.StatusNotFound, "ResourceNotFoundException",
			"Schedule does not exist.", readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeSchedulerError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to get schedule.", readOnly, eventID, verified)
		return
	}
	payload, err := schedsvc.GetScheduleJSON(sch)
	if err != nil {
		s.writeSchedulerError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to build response.", readOnly, eventID, verified)
		return
	}
	s.writeSchedulerOK(w, requestID, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, schedulerEventSource, "GetSchedule", readOnly)
}

func (s *Server) schedulerUpdate(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	name := stringParam(params["Name"])
	group := stringParam(params["GroupName"])
	resource := s.schedulerARN(verified, group, name)
	if !s.authorize(verified, catalog.ActionSchedulerUpdateSchedule, resource) {
		s.writeSchedulerError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform scheduler:UpdateSchedule.", readOnly, eventID, verified)
		return
	}
	in := store.UpdateScheduleInput{}
	if v, ok := params["ScheduleExpression"].(string); ok {
		in.Expression = &v
	}
	if v, ok := params["State"].(string); ok {
		in.State = &v
	}
	if _, ok := params["Target"].(map[string]any); ok {
		targetARN, roleARN, input := parseSchedulerTarget(params)
		in.TargetARN = &targetARN
		in.RoleARN = &roleARN
		in.Input = &input
		if roleARN != "" {
			if err := s.checkSchedulerPassRole(verified, roleARN); err != nil {
				s.writeSchedulerError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
					err.Error(), readOnly, eventID, verified)
				return
			}
		}
	}
	sch, err := s.store.UpdateSchedule(verified.AccountID, group, name, in, s.now().UTC())
	if errors.Is(err, store.ErrNoSuchSchedule) {
		s.writeSchedulerError(w, r, body, requestID, http.StatusNotFound, "ResourceNotFoundException",
			"Schedule does not exist.", readOnly, eventID, verified)
		return
	}
	if errors.Is(err, store.ErrInvalidScheduleExpr) || errors.Is(err, store.ErrInvalidScheduleTarget) {
		s.writeSchedulerError(w, r, body, requestID, http.StatusBadRequest, "ValidationException",
			err.Error(), readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeSchedulerError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to update schedule.", readOnly, eventID, verified)
		return
	}
	payload, err := schedsvc.UpdateScheduleJSON(sch)
	if err != nil {
		s.writeSchedulerError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to build response.", readOnly, eventID, verified)
		return
	}
	s.writeSchedulerOK(w, requestID, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, schedulerEventSource, "UpdateSchedule", readOnly)
}

func (s *Server) schedulerDelete(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	name := stringParam(params["Name"])
	group := stringParam(params["GroupName"])
	resource := s.schedulerARN(verified, group, name)
	if !s.authorize(verified, catalog.ActionSchedulerDeleteSchedule, resource) {
		s.writeSchedulerError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform scheduler:DeleteSchedule.", readOnly, eventID, verified)
		return
	}
	if err := s.store.DeleteSchedule(verified.AccountID, group, name); errors.Is(err, store.ErrNoSuchSchedule) {
		s.writeSchedulerError(w, r, body, requestID, http.StatusNotFound, "ResourceNotFoundException",
			"Schedule does not exist.", readOnly, eventID, verified)
		return
	} else if err != nil {
		s.writeSchedulerError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to delete schedule.", readOnly, eventID, verified)
		return
	}
	payload, err := schedsvc.DeleteScheduleJSON()
	if err != nil {
		s.writeSchedulerError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to build response.", readOnly, eventID, verified)
		return
	}
	s.writeSchedulerOK(w, requestID, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, schedulerEventSource, "DeleteSchedule", readOnly)
}

func (s *Server) schedulerList(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	if !s.authorize(verified, catalog.ActionSchedulerListSchedules, "*") {
		s.writeSchedulerError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform scheduler:ListSchedules.", readOnly, eventID, verified)
		return
	}
	group := stringParam(params["GroupName"])
	list, err := s.store.ListSchedules(verified.AccountID, group)
	if err != nil {
		s.writeSchedulerError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to list schedules.", readOnly, eventID, verified)
		return
	}
	payload, err := schedsvc.ListSchedulesJSON(list)
	if err != nil {
		s.writeSchedulerError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to build response.", readOnly, eventID, verified)
		return
	}
	s.writeSchedulerOK(w, requestID, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, schedulerEventSource, "ListSchedules", readOnly)
}

func (s *Server) writeSchedulerOK(w http.ResponseWriter, requestID string, payload []byte) {
	w.Header().Set("Content-Type", schedulerJSONContentType)
	w.Header().Set(requestIDHeader, requestID)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(payload)
}

func (s *Server) writeSchedulerError(
	w http.ResponseWriter, r *http.Request, body []byte, requestID string, status int, code, message string,
	readOnly bool, eventID string, verified *authn.Verified,
) {
	w.Header().Set("Content-Type", schedulerJSONContentType)
	w.Header().Set("x-amzn-ErrorType", code)
	w.Header().Set(requestIDHeader, requestID)
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"__type": code, "message": message})
	_ = body
	s.writeSuccessAudit(r, requestID, eventID, verified, schedulerEventSource, code, readOnly)
}
