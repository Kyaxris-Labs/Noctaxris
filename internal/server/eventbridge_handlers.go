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
	eb "github.com/Kyaxris-Labs/Noctaxris/internal/services/events"
	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

const eventsEventSource = "events.amazonaws.com"

func (s *Server) handleEventBridge(
	w http.ResponseWriter,
	r *http.Request,
	body []byte,
	requestID, eventID, action string,
	verified *authn.Verified,
	readOnly bool,
) {
	params := jsonBodyMap(body)
	action = eventsAction(action)

	switch action {
	case catalog.ActionEventsPutEvents:
		s.eventsPutEvents(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionEventsCreateEventBus:
		s.eventsCreateEventBus(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionEventsDeleteEventBus:
		s.eventsDeleteEventBus(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionEventsDescribeEventBus:
		s.eventsDescribeEventBus(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionEventsListEventBuses:
		s.eventsListEventBuses(w, r, body, requestID, eventID, verified, readOnly)
	case catalog.ActionEventsPutRule:
		s.eventsPutRule(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionEventsDescribeRule:
		s.eventsDescribeRule(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionEventsListRules:
		s.eventsListRules(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionEventsDeleteRule:
		s.eventsDeleteRule(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionEventsEnableRule:
		s.eventsEnableRule(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionEventsDisableRule:
		s.eventsDisableRule(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionEventsPutTargets:
		s.eventsPutTargets(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionEventsRemoveTargets:
		s.eventsRemoveTargets(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionEventsListTargetsByRule:
		s.eventsListTargetsByRule(w, r, body, requestID, eventID, verified, readOnly, params)
	default:
		s.writeEventsError(w, r, body, requestID, http.StatusNotImplemented, "InternalFailure",
			"This EventBridge action is not implemented.", readOnly, eventID, verified)
	}
}

func eventsAction(action string) string {
	if strings.Contains(action, ":") {
		return action
	}
	switch action {
	case "PutEvents":
		return catalog.ActionEventsPutEvents
	case "CreateEventBus":
		return catalog.ActionEventsCreateEventBus
	case "DeleteEventBus":
		return catalog.ActionEventsDeleteEventBus
	case "DescribeEventBus":
		return catalog.ActionEventsDescribeEventBus
	case "ListEventBuses":
		return catalog.ActionEventsListEventBuses
	case "PutRule":
		return catalog.ActionEventsPutRule
	case "DescribeRule":
		return catalog.ActionEventsDescribeRule
	case "ListRules":
		return catalog.ActionEventsListRules
	case "DeleteRule":
		return catalog.ActionEventsDeleteRule
	case "EnableRule":
		return catalog.ActionEventsEnableRule
	case "DisableRule":
		return catalog.ActionEventsDisableRule
	case "PutTargets":
		return catalog.ActionEventsPutTargets
	case "RemoveTargets":
		return catalog.ActionEventsRemoveTargets
	case "ListTargetsByRule":
		return catalog.ActionEventsListTargetsByRule
	default:
		return action
	}
}

func (s *Server) eventsRegion(verified *authn.Verified) string {
	if verified.Region != "" {
		return verified.Region
	}
	return store.DefaultEventsRegion
}

func (s *Server) authorizeEvents(verified *authn.Verified, action, resource string) bool {
	return s.authorize(verified, action, resource)
}

func (s *Server) checkEventsPassRole(verified *authn.Verified, roleARN string) error {
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
		return errors.New("not authorized to pass role to EventBridge")
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
		ServicePrincipal: authz.ServicePrincipalEvents,
	})
	if decision != authz.Allow {
		return errors.New("not authorized to pass role to EventBridge")
	}
	return nil
}

func (s *Server) eventsBusARN(verified *authn.Verified, busName string) string {
	return store.EventBusARN(s.eventsRegion(verified), verified.AccountID, busName)
}

func (s *Server) eventsRuleARN(verified *authn.Verified, busName, ruleName string) string {
	return store.EventRuleARN(s.eventsRegion(verified), verified.AccountID, busName, ruleName)
}

func (s *Server) eventsPutEvents(
	w http.ResponseWriter,
	r *http.Request,
	body []byte,
	requestID, eventID string,
	verified *authn.Verified,
	readOnly bool,
	params map[string]any,
) {
	entriesRaw, ok := params["Entries"].([]any)
	if !ok || len(entriesRaw) == 0 {
		s.writeEventsError(w, r, body, requestID, http.StatusBadRequest, "ValidationException",
			"Entries is required.", readOnly, eventID, verified)
		return
	}

	busName := store.DefaultEventBusName
	entries := make([]store.PutEventsEntry, 0, len(entriesRaw))
	for _, raw := range entriesRaw {
		m, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		entry := store.PutEventsEntry{
			Source:     stringParam(m["Source"]),
			DetailType: stringParam(m["DetailType"]),
			Detail:     stringParam(m["Detail"]),
		}
		if v := stringParam(m["EventBusName"]); v != "" {
			entry.EventBusName = v
			busName = v
		}
		if v := stringParam(m["Time"]); v != "" {
			entry.Time = v
		}
		entries = append(entries, entry)
	}
	if len(entries) == 0 {
		s.writeEventsError(w, r, body, requestID, http.StatusBadRequest, "ValidationException",
			"Entries is required.", readOnly, eventID, verified)
		return
	}

	resource := s.eventsBusARN(verified, busName)
	if !s.authorizeEvents(verified, catalog.ActionEventsPutEvents, resource) {
		s.writeEventsError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform events:PutEvents.", readOnly, eventID, verified)
		return
	}

	result, err := s.store.PutEvents(verified.AccountID, entries)
	if err != nil {
		s.writeEventsError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to put events.", readOnly, eventID, verified)
		return
	}

	payload, err := eb.PutEventsJSON(result)
	if err != nil {
		s.writeEventsError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to build response.", readOnly, eventID, verified)
		return
	}
	s.writeEventsOK(w, requestID, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, eventsEventSource, "PutEvents", readOnly)
}

func (s *Server) eventsPutRule(
	w http.ResponseWriter,
	r *http.Request,
	body []byte,
	requestID, eventID string,
	verified *authn.Verified,
	readOnly bool,
	params map[string]any,
) {
	name := stringParam(params["Name"])
	if name == "" {
		s.writeEventsError(w, r, body, requestID, http.StatusBadRequest, "ValidationException",
			"Name is required.", readOnly, eventID, verified)
		return
	}
	busName := stringParam(params["EventBusName"])
	pattern := stringParam(params["EventPattern"])
	description := stringParam(params["Description"])
	state := stringParam(params["State"])

	resource := s.eventsRuleARN(verified, busName, name)
	if !s.authorizeEvents(verified, catalog.ActionEventsPutRule, resource) {
		s.writeEventsError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform events:PutRule.", readOnly, eventID, verified)
		return
	}

	rule, err := s.store.PutRule(verified.AccountID, s.eventsRegion(verified), busName, name, pattern, description, state)
	if err != nil {
		s.writeEventsError(w, r, body, requestID, http.StatusBadRequest, "ValidationException",
			err.Error(), readOnly, eventID, verified)
		return
	}

	payload, err := eb.PutRuleJSON(rule)
	if err != nil {
		s.writeEventsError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to build response.", readOnly, eventID, verified)
		return
	}
	s.writeEventsOK(w, requestID, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, eventsEventSource, "PutRule", readOnly)
}

func (s *Server) eventsPutTargets(
	w http.ResponseWriter,
	r *http.Request,
	body []byte,
	requestID, eventID string,
	verified *authn.Verified,
	readOnly bool,
	params map[string]any,
) {
	ruleName := stringParam(params["Rule"])
	if ruleName == "" {
		s.writeEventsError(w, r, body, requestID, http.StatusBadRequest, "ValidationException",
			"Rule is required.", readOnly, eventID, verified)
		return
	}
	busName := stringParam(params["EventBusName"])
	targetsRaw, ok := params["Targets"].([]any)
	if !ok || len(targetsRaw) == 0 {
		s.writeEventsError(w, r, body, requestID, http.StatusBadRequest, "ValidationException",
			"Targets is required.", readOnly, eventID, verified)
		return
	}

	resource := s.eventsRuleARN(verified, busName, ruleName)
	if !s.authorizeEvents(verified, catalog.ActionEventsPutTargets, resource) {
		s.writeEventsError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform events:PutTargets.", readOnly, eventID, verified)
		return
	}

	inputs := make([]store.EventTargetInput, 0, len(targetsRaw))
	for _, raw := range targetsRaw {
		m, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		roleARN := stringParam(m["RoleArn"])
		if roleARN != "" {
			if err := s.checkEventsPassRole(verified, roleARN); err != nil {
				s.writeEventsError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
					err.Error(), readOnly, eventID, verified)
				return
			}
		}
		inputs = append(inputs, store.EventTargetInput{
			ID:        stringParam(m["Id"]),
			ARN:       stringParam(m["Arn"]),
			RoleARN:   roleARN,
			Input:     stringParam(m["Input"]),
			InputPath: stringParam(m["InputPath"]),
		})
	}
	if len(inputs) == 0 {
		s.writeEventsError(w, r, body, requestID, http.StatusBadRequest, "ValidationException",
			"Targets is required.", readOnly, eventID, verified)
		return
	}

	if err := s.store.PutTargets(verified.AccountID, busName, ruleName, inputs); err != nil {
		if errors.Is(err, store.ErrNoSuchEventRule) {
			s.writeEventsError(w, r, body, requestID, http.StatusNotFound, "ResourceNotFoundException",
				"Rule does not exist.", readOnly, eventID, verified)
			return
		}
		s.writeEventsError(w, r, body, requestID, http.StatusBadRequest, "ValidationException",
			err.Error(), readOnly, eventID, verified)
		return
	}

	payload, err := eb.PutTargetsJSON(nil)
	if err != nil {
		s.writeEventsError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to build response.", readOnly, eventID, verified)
		return
	}
	s.writeEventsOK(w, requestID, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, eventsEventSource, "PutTargets", readOnly)
}

func (s *Server) eventsCreateEventBus(
	w http.ResponseWriter,
	r *http.Request,
	body []byte,
	requestID, eventID string,
	verified *authn.Verified,
	readOnly bool,
	params map[string]any,
) {
	name := stringParam(params["Name"])
	if name == "" {
		s.writeEventsError(w, r, body, requestID, http.StatusBadRequest, "ValidationException",
			"Name is required.", readOnly, eventID, verified)
		return
	}
	resource := s.eventsBusARN(verified, name)
	if !s.authorizeEvents(verified, catalog.ActionEventsCreateEventBus, resource) {
		s.writeEventsError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform events:CreateEventBus.", readOnly, eventID, verified)
		return
	}
	bus, err := s.store.CreateEventBus(verified.AccountID, s.eventsRegion(verified), name)
	if errors.Is(err, store.ErrEventBusAlreadyExists) {
		s.writeEventsError(w, r, body, requestID, http.StatusBadRequest, "ResourceAlreadyExistsException",
			"Event bus already exists.", readOnly, eventID, verified)
		return
	} else if err != nil {
		s.writeEventsError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to create event bus.", readOnly, eventID, verified)
		return
	}
	payload, err := eb.CreateEventBusJSON(bus)
	if err != nil {
		s.writeEventsError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to build response.", readOnly, eventID, verified)
		return
	}
	s.writeEventsOK(w, requestID, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, eventsEventSource, "CreateEventBus", readOnly)
}

func (s *Server) eventsDeleteEventBus(
	w http.ResponseWriter,
	r *http.Request,
	body []byte,
	requestID, eventID string,
	verified *authn.Verified,
	readOnly bool,
	params map[string]any,
) {
	name := stringParam(params["Name"])
	if name == "" {
		s.writeEventsError(w, r, body, requestID, http.StatusBadRequest, "ValidationException",
			"Name is required.", readOnly, eventID, verified)
		return
	}
	resource := s.eventsBusARN(verified, name)
	if !s.authorizeEvents(verified, catalog.ActionEventsDeleteEventBus, resource) {
		s.writeEventsError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform events:DeleteEventBus.", readOnly, eventID, verified)
		return
	}
	if err := s.store.DeleteEventBus(verified.AccountID, name); err != nil {
		if errors.Is(err, store.ErrCannotDeleteDefaultEventBus) {
			s.writeEventsError(w, r, body, requestID, http.StatusBadRequest, "InvalidOperationException",
				"Cannot delete default event bus.", readOnly, eventID, verified)
			return
		}
		if errors.Is(err, store.ErrNoSuchEventBus) {
			s.writeEventsError(w, r, body, requestID, http.StatusNotFound, "ResourceNotFoundException",
				"Event bus does not exist.", readOnly, eventID, verified)
			return
		}
		s.writeEventsError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to delete event bus.", readOnly, eventID, verified)
		return
	}
	payload, err := eb.EmptyOKJSON()
	if err != nil {
		s.writeEventsError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to build response.", readOnly, eventID, verified)
		return
	}
	s.writeEventsOK(w, requestID, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, eventsEventSource, "DeleteEventBus", readOnly)
}

func (s *Server) eventsDescribeEventBus(
	w http.ResponseWriter,
	r *http.Request,
	body []byte,
	requestID, eventID string,
	verified *authn.Verified,
	readOnly bool,
	params map[string]any,
) {
	name := stringParam(params["Name"])
	resource := s.eventsBusARN(verified, name)
	if !s.authorizeEvents(verified, catalog.ActionEventsDescribeEventBus, resource) {
		s.writeEventsError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform events:DescribeEventBus.", readOnly, eventID, verified)
		return
	}
	bus, err := s.store.DescribeEventBus(verified.AccountID, name)
	if errors.Is(err, store.ErrNoSuchEventBus) {
		s.writeEventsError(w, r, body, requestID, http.StatusNotFound, "ResourceNotFoundException",
			"Event bus does not exist.", readOnly, eventID, verified)
		return
	} else if err != nil {
		s.writeEventsError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to describe event bus.", readOnly, eventID, verified)
		return
	}
	payload, err := eb.DescribeEventBusJSON(bus)
	if err != nil {
		s.writeEventsError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to build response.", readOnly, eventID, verified)
		return
	}
	s.writeEventsOK(w, requestID, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, eventsEventSource, "DescribeEventBus", readOnly)
}

func (s *Server) eventsListEventBuses(
	w http.ResponseWriter,
	r *http.Request,
	body []byte,
	requestID, eventID string,
	verified *authn.Verified,
	readOnly bool,
) {
	if !s.authorizeEvents(verified, catalog.ActionEventsListEventBuses, "*") {
		s.writeEventsError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform events:ListEventBuses.", readOnly, eventID, verified)
		return
	}
	buses, err := s.store.ListEventBuses(verified.AccountID)
	if err != nil {
		s.writeEventsError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to list event buses.", readOnly, eventID, verified)
		return
	}
	payload, err := eb.ListEventBusesJSON(buses)
	if err != nil {
		s.writeEventsError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to build response.", readOnly, eventID, verified)
		return
	}
	s.writeEventsOK(w, requestID, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, eventsEventSource, "ListEventBuses", readOnly)
}

func (s *Server) eventsDescribeRule(
	w http.ResponseWriter,
	r *http.Request,
	body []byte,
	requestID, eventID string,
	verified *authn.Verified,
	readOnly bool,
	params map[string]any,
) {
	ruleName := stringParam(params["Name"])
	if ruleName == "" {
		s.writeEventsError(w, r, body, requestID, http.StatusBadRequest, "ValidationException",
			"Name is required.", readOnly, eventID, verified)
		return
	}
	busName := stringParam(params["EventBusName"])
	resource := s.eventsRuleARN(verified, busName, ruleName)
	if !s.authorizeEvents(verified, catalog.ActionEventsDescribeRule, resource) {
		s.writeEventsError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform events:DescribeRule.", readOnly, eventID, verified)
		return
	}
	rule, err := s.store.DescribeRule(verified.AccountID, busName, ruleName)
	if errors.Is(err, store.ErrNoSuchEventRule) {
		s.writeEventsError(w, r, body, requestID, http.StatusNotFound, "ResourceNotFoundException",
			"Rule does not exist.", readOnly, eventID, verified)
		return
	} else if err != nil {
		s.writeEventsError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to describe rule.", readOnly, eventID, verified)
		return
	}
	payload, err := eb.DescribeRuleJSON(rule)
	if err != nil {
		s.writeEventsError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to build response.", readOnly, eventID, verified)
		return
	}
	s.writeEventsOK(w, requestID, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, eventsEventSource, "DescribeRule", readOnly)
}

func (s *Server) eventsListRules(
	w http.ResponseWriter,
	r *http.Request,
	body []byte,
	requestID, eventID string,
	verified *authn.Verified,
	readOnly bool,
	params map[string]any,
) {
	busName := stringParam(params["EventBusName"])
	resource := s.eventsBusARN(verified, busName)
	if !s.authorizeEvents(verified, catalog.ActionEventsListRules, resource) {
		s.writeEventsError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform events:ListRules.", readOnly, eventID, verified)
		return
	}
	rules, err := s.store.ListRules(verified.AccountID, busName)
	if err != nil {
		s.writeEventsError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to list rules.", readOnly, eventID, verified)
		return
	}
	payload, err := eb.ListRulesJSON(rules)
	if err != nil {
		s.writeEventsError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to build response.", readOnly, eventID, verified)
		return
	}
	s.writeEventsOK(w, requestID, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, eventsEventSource, "ListRules", readOnly)
}

func (s *Server) eventsDeleteRule(
	w http.ResponseWriter,
	r *http.Request,
	body []byte,
	requestID, eventID string,
	verified *authn.Verified,
	readOnly bool,
	params map[string]any,
) {
	ruleName := stringParam(params["Name"])
	if ruleName == "" {
		s.writeEventsError(w, r, body, requestID, http.StatusBadRequest, "ValidationException",
			"Name is required.", readOnly, eventID, verified)
		return
	}
	busName := stringParam(params["EventBusName"])
	resource := s.eventsRuleARN(verified, busName, ruleName)
	if !s.authorizeEvents(verified, catalog.ActionEventsDeleteRule, resource) {
		s.writeEventsError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform events:DeleteRule.", readOnly, eventID, verified)
		return
	}
	if err := s.store.DeleteRule(verified.AccountID, busName, ruleName); errors.Is(err, store.ErrNoSuchEventRule) {
		s.writeEventsError(w, r, body, requestID, http.StatusNotFound, "ResourceNotFoundException",
			"Rule does not exist.", readOnly, eventID, verified)
		return
	} else if err != nil {
		s.writeEventsError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to delete rule.", readOnly, eventID, verified)
		return
	}
	payload, err := eb.EmptyOKJSON()
	if err != nil {
		s.writeEventsError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to build response.", readOnly, eventID, verified)
		return
	}
	s.writeEventsOK(w, requestID, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, eventsEventSource, "DeleteRule", readOnly)
}

func (s *Server) eventsEnableRule(
	w http.ResponseWriter,
	r *http.Request,
	body []byte,
	requestID, eventID string,
	verified *authn.Verified,
	readOnly bool,
	params map[string]any,
) {
	s.eventsSetRuleState(w, r, body, requestID, eventID, verified, readOnly, params, catalog.ActionEventsEnableRule, "EnableRule", true)
}

func (s *Server) eventsDisableRule(
	w http.ResponseWriter,
	r *http.Request,
	body []byte,
	requestID, eventID string,
	verified *authn.Verified,
	readOnly bool,
	params map[string]any,
) {
	s.eventsSetRuleState(w, r, body, requestID, eventID, verified, readOnly, params, catalog.ActionEventsDisableRule, "DisableRule", false)
}

func (s *Server) eventsSetRuleState(
	w http.ResponseWriter,
	r *http.Request,
	body []byte,
	requestID, eventID string,
	verified *authn.Verified,
	readOnly bool,
	params map[string]any,
	action, auditName string,
	enable bool,
) {
	ruleName := stringParam(params["Name"])
	if ruleName == "" {
		s.writeEventsError(w, r, body, requestID, http.StatusBadRequest, "ValidationException",
			"Name is required.", readOnly, eventID, verified)
		return
	}
	busName := stringParam(params["EventBusName"])
	resource := s.eventsRuleARN(verified, busName, ruleName)
	if !s.authorizeEvents(verified, action, resource) {
		s.writeEventsError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform "+action+".", readOnly, eventID, verified)
		return
	}
	var err error
	if enable {
		err = s.store.EnableRule(verified.AccountID, busName, ruleName)
	} else {
		err = s.store.DisableRule(verified.AccountID, busName, ruleName)
	}
	if errors.Is(err, store.ErrNoSuchEventRule) {
		s.writeEventsError(w, r, body, requestID, http.StatusNotFound, "ResourceNotFoundException",
			"Rule does not exist.", readOnly, eventID, verified)
		return
	} else if err != nil {
		s.writeEventsError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to update rule state.", readOnly, eventID, verified)
		return
	}
	payload, err := eb.EmptyOKJSON()
	if err != nil {
		s.writeEventsError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to build response.", readOnly, eventID, verified)
		return
	}
	s.writeEventsOK(w, requestID, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, eventsEventSource, auditName, readOnly)
}

func (s *Server) eventsRemoveTargets(
	w http.ResponseWriter,
	r *http.Request,
	body []byte,
	requestID, eventID string,
	verified *authn.Verified,
	readOnly bool,
	params map[string]any,
) {
	ruleName := stringParam(params["Rule"])
	if ruleName == "" {
		s.writeEventsError(w, r, body, requestID, http.StatusBadRequest, "ValidationException",
			"Rule is required.", readOnly, eventID, verified)
		return
	}
	busName := stringParam(params["EventBusName"])
	ids := stringSliceParam(params["Ids"])
	resource := s.eventsRuleARN(verified, busName, ruleName)
	if !s.authorizeEvents(verified, catalog.ActionEventsRemoveTargets, resource) {
		s.writeEventsError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform events:RemoveTargets.", readOnly, eventID, verified)
		return
	}
	if err := s.store.RemoveTargets(verified.AccountID, busName, ruleName, ids); errors.Is(err, store.ErrNoSuchEventRule) {
		s.writeEventsError(w, r, body, requestID, http.StatusNotFound, "ResourceNotFoundException",
			"Rule does not exist.", readOnly, eventID, verified)
		return
	} else if errors.Is(err, store.ErrNoSuchEventTarget) {
		s.writeEventsError(w, r, body, requestID, http.StatusNotFound, "ResourceNotFoundException",
			"Target does not exist.", readOnly, eventID, verified)
		return
	} else if err != nil {
		s.writeEventsError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to remove targets.", readOnly, eventID, verified)
		return
	}
	payload, err := eb.EmptyOKJSON()
	if err != nil {
		s.writeEventsError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to build response.", readOnly, eventID, verified)
		return
	}
	s.writeEventsOK(w, requestID, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, eventsEventSource, "RemoveTargets", readOnly)
}

func (s *Server) eventsListTargetsByRule(
	w http.ResponseWriter,
	r *http.Request,
	body []byte,
	requestID, eventID string,
	verified *authn.Verified,
	readOnly bool,
	params map[string]any,
) {
	ruleName := stringParam(params["Rule"])
	if ruleName == "" {
		s.writeEventsError(w, r, body, requestID, http.StatusBadRequest, "ValidationException",
			"Rule is required.", readOnly, eventID, verified)
		return
	}
	busName := stringParam(params["EventBusName"])
	resource := s.eventsRuleARN(verified, busName, ruleName)
	if !s.authorizeEvents(verified, catalog.ActionEventsListTargetsByRule, resource) {
		s.writeEventsError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform events:ListTargetsByRule.", readOnly, eventID, verified)
		return
	}
	targets, err := s.store.ListTargetsByRule(verified.AccountID, busName, ruleName)
	if errors.Is(err, store.ErrNoSuchEventRule) {
		s.writeEventsError(w, r, body, requestID, http.StatusNotFound, "ResourceNotFoundException",
			"Rule does not exist.", readOnly, eventID, verified)
		return
	} else if err != nil {
		s.writeEventsError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to list targets.", readOnly, eventID, verified)
		return
	}
	payload, err := eb.ListTargetsByRuleJSON(targets)
	if err != nil {
		s.writeEventsError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to build response.", readOnly, eventID, verified)
		return
	}
	s.writeEventsOK(w, requestID, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, eventsEventSource, "ListTargetsByRule", readOnly)
}

func stringParam(v any) string {
	s, _ := v.(string)
	return strings.TrimSpace(s)
}

func (s *Server) writeEventsOK(w http.ResponseWriter, requestID string, payload []byte) {
	w.Header().Set(requestIDHeader, requestID)
	w.Header().Set("Content-Type", eb.JSONContentType())
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(payload)
}

func (s *Server) writeEventsError(
	w http.ResponseWriter,
	r *http.Request,
	body []byte,
	requestID string,
	status int,
	code, message string,
	readOnly bool,
	eventID string,
	verified *authn.Verified,
) {
	_ = body
	w.Header().Set(requestIDHeader, requestID)
	w.Header().Set("Content-Type", eb.JSONContentType())
	w.WriteHeader(status)
	payload, _ := json.Marshal(map[string]string{
		"__type":  code,
		"message": message,
	})
	_, _ = w.Write(payload)

	accessKeyID := ""
	accountID := ""
	if verified != nil {
		accessKeyID = verified.AccessKeyID
		accountID = verified.AccountID
	}
	s.auditAPIError(r, requestID, eventID, code, message, readOnly, accessKeyID, accountID, verified != nil)
}
