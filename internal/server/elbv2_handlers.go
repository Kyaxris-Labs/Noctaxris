package server

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/Kyaxris-Labs/Noctaxris/internal/catalog"
	"github.com/Kyaxris-Labs/Noctaxris/internal/kernel/authn"
	elbsvc "github.com/Kyaxris-Labs/Noctaxris/internal/services/elbv2"
	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

const (
	elbv2JSONContentType = "application/x-amz-json-1.1"
	elbv2EventSource     = "elasticloadbalancing.amazonaws.com"
)

func (s *Server) handleELBv2(
	w http.ResponseWriter,
	r *http.Request,
	body []byte,
	requestID, eventID, action string,
	verified *authn.Verified,
	readOnly bool,
) {
	params := jsonBodyMap(body)
	action = elbv2Action(action)

	switch action {
	case catalog.ActionELBv2CreateLoadBalancer:
		s.elbCreateLB(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionELBv2DescribeLoadBalancers:
		s.elbDescribeLBs(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionELBv2DeleteLoadBalancer:
		s.elbDeleteLB(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionELBv2CreateTargetGroup:
		s.elbCreateTG(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionELBv2DescribeTargetGroups:
		s.elbDescribeTGs(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionELBv2DeleteTargetGroup:
		s.elbDeleteTG(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionELBv2CreateListener:
		s.elbCreateListener(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionELBv2DescribeListeners:
		s.elbDescribeListeners(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionELBv2DeleteListener:
		s.elbDeleteListener(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionELBv2RegisterTargets:
		s.elbRegisterTargets(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionELBv2DescribeTargetHealth:
		s.elbDescribeTargetHealth(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionELBv2CreateRule:
		s.elbCreateRule(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionELBv2DescribeRules:
		s.elbDescribeRules(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionELBv2DeleteRule:
		s.elbDeleteRule(w, r, body, requestID, eventID, verified, readOnly, params)
	case "elasticloadbalancing:ModifyLoadBalancerAttributes", "ModifyLoadBalancerAttributes":
		s.elbModifyLoadBalancerAttributes(w, r, body, requestID, eventID, verified, readOnly, params)
	case "elasticloadbalancing:DescribeLoadBalancerAttributes", "DescribeLoadBalancerAttributes":
		s.elbDescribeLoadBalancerAttributes(w, r, body, requestID, eventID, verified, readOnly, params)
	default:
		s.writeELBv2Error(w, r, body, requestID, http.StatusNotImplemented, "InvalidAction",
			"This Elastic Load Balancing action is not implemented.", readOnly, eventID, verified)
	}
}

func elbv2Action(action string) string {
	if strings.Contains(action, ":") {
		return action
	}
	switch action {
	case "CreateLoadBalancer":
		return catalog.ActionELBv2CreateLoadBalancer
	case "DescribeLoadBalancers":
		return catalog.ActionELBv2DescribeLoadBalancers
	case "DeleteLoadBalancer":
		return catalog.ActionELBv2DeleteLoadBalancer
	case "CreateTargetGroup":
		return catalog.ActionELBv2CreateTargetGroup
	case "DescribeTargetGroups":
		return catalog.ActionELBv2DescribeTargetGroups
	case "DeleteTargetGroup":
		return catalog.ActionELBv2DeleteTargetGroup
	case "CreateListener":
		return catalog.ActionELBv2CreateListener
	case "DescribeListeners":
		return catalog.ActionELBv2DescribeListeners
	case "DeleteListener":
		return catalog.ActionELBv2DeleteListener
	case "RegisterTargets":
		return catalog.ActionELBv2RegisterTargets
	case "DescribeTargetHealth":
		return catalog.ActionELBv2DescribeTargetHealth
	case "CreateRule":
		return catalog.ActionELBv2CreateRule
	case "DescribeRules":
		return catalog.ActionELBv2DescribeRules
	case "DeleteRule":
		return catalog.ActionELBv2DeleteRule
	case "ModifyLoadBalancerAttributes":
		return "elasticloadbalancing:ModifyLoadBalancerAttributes"
	case "DescribeLoadBalancerAttributes":
		return "elasticloadbalancing:DescribeLoadBalancerAttributes"
	default:
		return action
	}
}

func elbStringSliceParam(params map[string]any, key string) []string {
	raw, _ := params[key].([]any)
	var out []string
	for _, v := range raw {
		if s, ok := v.(string); ok {
			out = append(out, s)
		}
	}
	return out
}

func (s *Server) elbCreateLB(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	name, _ := params["Name"].(string)
	scheme, _ := params["Scheme"].(string)
	lbType, _ := params["Type"].(string)
	lbType = strings.ToLower(strings.TrimSpace(lbType))
	if lbType == "" {
		lbType = "application"
	}
	if lbType != "application" {
		s.writeELBv2Error(w, r, body, requestID, http.StatusBadRequest, "ValidationError",
			"Type must be application (network load balancers are not implemented).", readOnly, eventID, verified)
		return
	}
	if !s.authorize(verified, catalog.ActionELBv2CreateLoadBalancer, "*") {
		s.writeELBv2Error(w, r, body, requestID, http.StatusForbidden, "AccessDenied",
			"User is not authorized to perform elasticloadbalancing:CreateLoadBalancer.", readOnly, eventID, verified)
		return
	}
	region := verified.Region
	if region == "" {
		region = store.DefaultELBv2Region
	}
	lb, err := s.store.CreateELBv2LoadBalancer(verified.AccountID, region, name, scheme)
	if errors.Is(err, store.ErrELBv2BadRequest) {
		s.writeELBv2Error(w, r, body, requestID, http.StatusBadRequest, "ValidationError",
			err.Error(), readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeELBv2Error(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to create load balancer.", readOnly, eventID, verified)
		return
	}
	payload, _ := elbsvc.CreateLoadBalancerJSON(lb)
	s.writeELBv2OK(w, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, elbv2EventSource, "CreateLoadBalancer", readOnly)
}

func (s *Server) elbDescribeLBs(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	if !s.authorize(verified, catalog.ActionELBv2DescribeLoadBalancers, "*") {
		s.writeELBv2Error(w, r, body, requestID, http.StatusForbidden, "AccessDenied",
			"User is not authorized to perform elasticloadbalancing:DescribeLoadBalancers.", readOnly, eventID, verified)
		return
	}
	lbs, err := s.store.DescribeELBv2LoadBalancers(verified.AccountID, elbStringSliceParam(params, "LoadBalancerArns"))
	if err != nil {
		s.writeELBv2Error(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to describe load balancers.", readOnly, eventID, verified)
		return
	}
	payload, _ := elbsvc.DescribeLoadBalancersJSON(lbs)
	s.writeELBv2OK(w, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, elbv2EventSource, "DescribeLoadBalancers", readOnly)
}

func (s *Server) elbDeleteLB(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	arn, _ := params["LoadBalancerArn"].(string)
	if !s.authorize(verified, catalog.ActionELBv2DeleteLoadBalancer, arn) {
		s.writeELBv2Error(w, r, body, requestID, http.StatusForbidden, "AccessDenied",
			"User is not authorized to perform elasticloadbalancing:DeleteLoadBalancer.", readOnly, eventID, verified)
		return
	}
	err := s.store.DeleteELBv2LoadBalancer(verified.AccountID, arn)
	if errors.Is(err, store.ErrELBv2NotFound) {
		s.writeELBv2Error(w, r, body, requestID, http.StatusBadRequest, "LoadBalancerNotFound",
			"Load balancer not found.", readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeELBv2Error(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to delete load balancer.", readOnly, eventID, verified)
		return
	}
	payload, _ := elbsvc.DeleteOKJSON()
	s.writeELBv2OK(w, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, elbv2EventSource, "DeleteLoadBalancer", readOnly)
}

func (s *Server) elbCreateTG(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	name, _ := params["Name"].(string)
	targetType, _ := params["TargetType"].(string)
	protocol, _ := params["Protocol"].(string)
	port := 80
	if p, ok := params["Port"].(float64); ok {
		port = int(p)
	}
	if !s.authorize(verified, catalog.ActionELBv2CreateTargetGroup, "*") {
		s.writeELBv2Error(w, r, body, requestID, http.StatusForbidden, "AccessDenied",
			"User is not authorized to perform elasticloadbalancing:CreateTargetGroup.", readOnly, eventID, verified)
		return
	}
	region := verified.Region
	if region == "" {
		region = store.DefaultELBv2Region
	}
	tg, err := s.store.CreateELBv2TargetGroup(verified.AccountID, region, name, targetType, protocol, port)
	if errors.Is(err, store.ErrELBv2BadRequest) {
		s.writeELBv2Error(w, r, body, requestID, http.StatusBadRequest, "ValidationError",
			err.Error(), readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeELBv2Error(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to create target group.", readOnly, eventID, verified)
		return
	}
	payload, _ := elbsvc.CreateTargetGroupJSON(tg)
	s.writeELBv2OK(w, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, elbv2EventSource, "CreateTargetGroup", readOnly)
}

func (s *Server) elbDescribeTGs(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	if !s.authorize(verified, catalog.ActionELBv2DescribeTargetGroups, "*") {
		s.writeELBv2Error(w, r, body, requestID, http.StatusForbidden, "AccessDenied",
			"User is not authorized to perform elasticloadbalancing:DescribeTargetGroups.", readOnly, eventID, verified)
		return
	}
	tgs, err := s.store.DescribeELBv2TargetGroups(verified.AccountID, elbStringSliceParam(params, "TargetGroupArns"))
	if err != nil {
		s.writeELBv2Error(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to describe target groups.", readOnly, eventID, verified)
		return
	}
	payload, _ := elbsvc.DescribeTargetGroupsJSON(tgs)
	s.writeELBv2OK(w, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, elbv2EventSource, "DescribeTargetGroups", readOnly)
}

func (s *Server) elbDeleteTG(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	arn, _ := params["TargetGroupArn"].(string)
	if !s.authorize(verified, catalog.ActionELBv2DeleteTargetGroup, arn) {
		s.writeELBv2Error(w, r, body, requestID, http.StatusForbidden, "AccessDenied",
			"User is not authorized to perform elasticloadbalancing:DeleteTargetGroup.", readOnly, eventID, verified)
		return
	}
	err := s.store.DeleteELBv2TargetGroup(verified.AccountID, arn)
	if errors.Is(err, store.ErrELBv2TGNotFound) {
		s.writeELBv2Error(w, r, body, requestID, http.StatusBadRequest, "TargetGroupNotFound",
			"Target group not found.", readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeELBv2Error(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to delete target group.", readOnly, eventID, verified)
		return
	}
	payload, _ := elbsvc.DeleteOKJSON()
	s.writeELBv2OK(w, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, elbv2EventSource, "DeleteTargetGroup", readOnly)
}

func (s *Server) elbCreateListener(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	lbARN, _ := params["LoadBalancerArn"].(string)
	protocol, _ := params["Protocol"].(string)
	port := 80
	if p, ok := params["Port"].(float64); ok {
		port = int(p)
	}
	tgARN := ""
	if actions, ok := params["DefaultActions"].([]any); ok && len(actions) > 0 {
		if m, ok := actions[0].(map[string]any); ok {
			tgARN, _ = m["TargetGroupArn"].(string)
		}
	}
	if tgARN == "" {
		tgARN, _ = params["TargetGroupArn"].(string)
	}
	if !s.authorize(verified, catalog.ActionELBv2CreateListener, lbARN) {
		s.writeELBv2Error(w, r, body, requestID, http.StatusForbidden, "AccessDenied",
			"User is not authorized to perform elasticloadbalancing:CreateListener.", readOnly, eventID, verified)
		return
	}
	region := verified.Region
	if region == "" {
		region = store.DefaultELBv2Region
	}
	l, err := s.store.CreateELBv2Listener(verified.AccountID, region, lbARN, tgARN, protocol, port)
	if errors.Is(err, store.ErrELBv2NotFound) {
		s.writeELBv2Error(w, r, body, requestID, http.StatusBadRequest, "LoadBalancerNotFound",
			"Load balancer not found.", readOnly, eventID, verified)
		return
	}
	if errors.Is(err, store.ErrELBv2TGNotFound) {
		s.writeELBv2Error(w, r, body, requestID, http.StatusBadRequest, "TargetGroupNotFound",
			"Target group not found.", readOnly, eventID, verified)
		return
	}
	if errors.Is(err, store.ErrELBv2BadRequest) {
		s.writeELBv2Error(w, r, body, requestID, http.StatusBadRequest, "ValidationError",
			err.Error(), readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeELBv2Error(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to create listener.", readOnly, eventID, verified)
		return
	}
	payload, _ := elbsvc.CreateListenerJSON(l)
	s.writeELBv2OK(w, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, elbv2EventSource, "CreateListener", readOnly)
}

func (s *Server) elbDescribeListeners(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	lbARN, _ := params["LoadBalancerArn"].(string)
	if !s.authorize(verified, catalog.ActionELBv2DescribeListeners, lbARN) {
		s.writeELBv2Error(w, r, body, requestID, http.StatusForbidden, "AccessDenied",
			"User is not authorized to perform elasticloadbalancing:DescribeListeners.", readOnly, eventID, verified)
		return
	}
	ls, err := s.store.DescribeELBv2Listeners(verified.AccountID, lbARN)
	if err != nil {
		s.writeELBv2Error(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to describe listeners.", readOnly, eventID, verified)
		return
	}
	payload, _ := elbsvc.DescribeListenersJSON(ls)
	s.writeELBv2OK(w, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, elbv2EventSource, "DescribeListeners", readOnly)
}

func (s *Server) elbDeleteListener(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	arn, _ := params["ListenerArn"].(string)
	if !s.authorize(verified, catalog.ActionELBv2DeleteListener, arn) {
		s.writeELBv2Error(w, r, body, requestID, http.StatusForbidden, "AccessDenied",
			"User is not authorized to perform elasticloadbalancing:DeleteListener.", readOnly, eventID, verified)
		return
	}
	err := s.store.DeleteELBv2Listener(verified.AccountID, arn)
	if errors.Is(err, store.ErrELBv2NotFound) {
		s.writeELBv2Error(w, r, body, requestID, http.StatusBadRequest, "ListenerNotFound",
			"Listener not found.", readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeELBv2Error(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to delete listener.", readOnly, eventID, verified)
		return
	}
	payload, _ := elbsvc.DeleteOKJSON()
	s.writeELBv2OK(w, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, elbv2EventSource, "DeleteListener", readOnly)
}

func (s *Server) elbRegisterTargets(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	tgARN, _ := params["TargetGroupArn"].(string)
	if !s.authorize(verified, catalog.ActionELBv2RegisterTargets, tgARN) {
		s.writeELBv2Error(w, r, body, requestID, http.StatusForbidden, "AccessDenied",
			"User is not authorized to perform elasticloadbalancing:RegisterTargets.", readOnly, eventID, verified)
		return
	}
	raw, _ := params["Targets"].([]any)
	var targets []store.ELBv2Target
	for _, item := range raw {
		m, _ := item.(map[string]any)
		id, _ := m["Id"].(string)
		port := 0
		if p, ok := m["Port"].(float64); ok {
			port = int(p)
		}
		targets = append(targets, store.ELBv2Target{ID: id, Port: port})
	}
	err := s.store.RegisterELBv2Targets(verified.AccountID, tgARN, targets)
	if errors.Is(err, store.ErrELBv2TGNotFound) {
		s.writeELBv2Error(w, r, body, requestID, http.StatusBadRequest, "TargetGroupNotFound",
			"Target group not found.", readOnly, eventID, verified)
		return
	}
	if errors.Is(err, store.ErrELBv2BadRequest) {
		s.writeELBv2Error(w, r, body, requestID, http.StatusBadRequest, "ValidationError",
			err.Error(), readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeELBv2Error(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to register targets.", readOnly, eventID, verified)
		return
	}
	payload, _ := elbsvc.RegisterTargetsJSON()
	s.writeELBv2OK(w, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, elbv2EventSource, "RegisterTargets", readOnly)
}

func (s *Server) elbDescribeTargetHealth(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	tgARN, _ := params["TargetGroupArn"].(string)
	if !s.authorize(verified, catalog.ActionELBv2DescribeTargetHealth, tgARN) {
		s.writeELBv2Error(w, r, body, requestID, http.StatusForbidden, "AccessDenied",
			"User is not authorized to perform elasticloadbalancing:DescribeTargetHealth.", readOnly, eventID, verified)
		return
	}
	descs, err := s.store.DescribeELBv2TargetHealth(verified.AccountID, tgARN)
	if errors.Is(err, store.ErrELBv2TGNotFound) {
		s.writeELBv2Error(w, r, body, requestID, http.StatusBadRequest, "TargetGroupNotFound",
			"Target group not found.", readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeELBv2Error(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to describe target health.", readOnly, eventID, verified)
		return
	}
	payload, _ := elbsvc.DescribeTargetHealthJSON(descs)
	s.writeELBv2OK(w, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, elbv2EventSource, "DescribeTargetHealth", readOnly)
}

func elbPathPatternsFromConditions(params map[string]any) []string {
	raw, _ := params["Conditions"].([]any)
	var out []string
	for _, item := range raw {
		m, _ := item.(map[string]any)
		field, _ := m["Field"].(string)
		if field != "" && !strings.EqualFold(field, "path-pattern") {
			continue
		}
		if vals, ok := m["Values"].([]any); ok {
			for _, v := range vals {
				if s, ok := v.(string); ok {
					out = append(out, s)
				}
			}
		}
		if cfg, ok := m["PathPatternConfig"].(map[string]any); ok {
			if vals, ok := cfg["Values"].([]any); ok {
				for _, v := range vals {
					if s, ok := v.(string); ok {
						out = append(out, s)
					}
				}
			}
		}
	}
	return out
}

func elbHostHeadersFromConditions(params map[string]any) []string {
	raw, _ := params["Conditions"].([]any)
	var out []string
	for _, item := range raw {
		m, _ := item.(map[string]any)
		field, _ := m["Field"].(string)
		if !strings.EqualFold(field, "host-header") {
			continue
		}
		if vals, ok := m["Values"].([]any); ok {
			for _, v := range vals {
				if s, ok := v.(string); ok {
					out = append(out, s)
				}
			}
		}
		if cfg, ok := m["HostHeaderConfig"].(map[string]any); ok {
			if vals, ok := cfg["Values"].([]any); ok {
				for _, v := range vals {
					if s, ok := v.(string); ok {
						out = append(out, s)
					}
				}
			}
		}
	}
	return out
}

func elbForwardTargetGroupFromActions(params map[string]any) string {
	raw, _ := params["Actions"].([]any)
	for _, item := range raw {
		m, _ := item.(map[string]any)
		typ, _ := m["Type"].(string)
		if typ != "" && !strings.EqualFold(typ, "forward") {
			continue
		}
		if tg, ok := m["TargetGroupArn"].(string); ok && strings.TrimSpace(tg) != "" {
			return tg
		}
	}
	tg, _ := params["TargetGroupArn"].(string)
	return tg
}

func elbRuleJSON(rule store.ELBv2Rule) map[string]any {
	conds := make([]map[string]any, 0, 2)
	if len(rule.HostHeaders) > 0 {
		conds = append(conds, map[string]any{
			"Field":  "host-header",
			"Values": rule.HostHeaders,
		})
	}
	if len(rule.PathPatterns) > 0 {
		conds = append(conds, map[string]any{
			"Field":  "path-pattern",
			"Values": rule.PathPatterns,
		})
	}
	return map[string]any{
		"RuleArn":    rule.RuleARN,
		"Priority":   strconv.Itoa(rule.Priority),
		"IsDefault":  false,
		"Conditions": conds,
		"Actions": []map[string]any{{
			"Type":           "forward",
			"TargetGroupArn": rule.TargetGroupARN,
		}},
	}
}

func (s *Server) elbCreateRule(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	listenerARN, _ := params["ListenerArn"].(string)
	priority := 0
	switch p := params["Priority"].(type) {
	case float64:
		priority = int(p)
	case string:
		priority, _ = strconv.Atoi(p)
	}
	tgARN := elbForwardTargetGroupFromActions(params)
	patterns := elbPathPatternsFromConditions(params)
	hosts := elbHostHeadersFromConditions(params)
	if !s.authorize(verified, catalog.ActionELBv2CreateRule, listenerARN) {
		s.writeELBv2Error(w, r, body, requestID, http.StatusForbidden, "AccessDenied",
			"User is not authorized to perform elasticloadbalancing:CreateRule.", readOnly, eventID, verified)
		return
	}
	region := verified.Region
	if region == "" {
		region = store.DefaultELBv2Region
	}
	rule, err := s.store.CreateELBv2Rule(verified.AccountID, region, listenerARN, tgARN, priority, patterns, hosts)
	if errors.Is(err, store.ErrELBv2ListenerNotFound) {
		s.writeELBv2Error(w, r, body, requestID, http.StatusBadRequest, "ListenerNotFound",
			"Listener not found.", readOnly, eventID, verified)
		return
	}
	if errors.Is(err, store.ErrELBv2TGNotFound) {
		s.writeELBv2Error(w, r, body, requestID, http.StatusBadRequest, "TargetGroupNotFound",
			"Target group not found.", readOnly, eventID, verified)
		return
	}
	if errors.Is(err, store.ErrELBv2PriorityInUse) {
		s.writeELBv2Error(w, r, body, requestID, http.StatusBadRequest, "PriorityInUse",
			"The specified priority is in use.", readOnly, eventID, verified)
		return
	}
	if errors.Is(err, store.ErrELBv2BadRequest) {
		s.writeELBv2Error(w, r, body, requestID, http.StatusBadRequest, "ValidationError",
			err.Error(), readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeELBv2Error(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to create rule.", readOnly, eventID, verified)
		return
	}
	payload, _ := json.Marshal(map[string]any{"Rules": []map[string]any{elbRuleJSON(rule)}})
	s.writeELBv2OK(w, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, elbv2EventSource, "CreateRule", readOnly)
}

func (s *Server) elbDescribeRules(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	listenerARN, _ := params["ListenerArn"].(string)
	ruleARNs := elbStringSliceParam(params, "RuleArns")
	if !s.authorize(verified, catalog.ActionELBv2DescribeRules, listenerARN) {
		s.writeELBv2Error(w, r, body, requestID, http.StatusForbidden, "AccessDenied",
			"User is not authorized to perform elasticloadbalancing:DescribeRules.", readOnly, eventID, verified)
		return
	}
	rules, err := s.store.DescribeELBv2Rules(verified.AccountID, listenerARN, ruleARNs)
	if errors.Is(err, store.ErrELBv2ListenerNotFound) {
		s.writeELBv2Error(w, r, body, requestID, http.StatusBadRequest, "ListenerNotFound",
			"Listener not found.", readOnly, eventID, verified)
		return
	}
	if errors.Is(err, store.ErrELBv2RuleNotFound) {
		s.writeELBv2Error(w, r, body, requestID, http.StatusBadRequest, "RuleNotFound",
			"Rule not found.", readOnly, eventID, verified)
		return
	}
	if errors.Is(err, store.ErrELBv2BadRequest) {
		s.writeELBv2Error(w, r, body, requestID, http.StatusBadRequest, "ValidationError",
			err.Error(), readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeELBv2Error(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to describe rules.", readOnly, eventID, verified)
		return
	}
	items := make([]map[string]any, 0, len(rules)+1)
	for _, rule := range rules {
		items = append(items, elbRuleJSON(rule))
	}
	if listenerARN != "" {
		if l, err := s.store.GetELBv2ListenerByARN(verified.AccountID, listenerARN); err == nil {
			items = append(items, map[string]any{
				"RuleArn":    l.ListenerARN + "/default",
				"Priority":   "default",
				"IsDefault":  true,
				"Conditions": []any{},
				"Actions": []map[string]any{{
					"Type": "forward", "TargetGroupArn": l.TargetGroupARN,
				}},
			})
		}
	}
	payload, _ := json.Marshal(map[string]any{"Rules": items})
	s.writeELBv2OK(w, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, elbv2EventSource, "DescribeRules", readOnly)
}

func (s *Server) elbDeleteRule(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	arn, _ := params["RuleArn"].(string)
	if !s.authorize(verified, catalog.ActionELBv2DeleteRule, arn) {
		s.writeELBv2Error(w, r, body, requestID, http.StatusForbidden, "AccessDenied",
			"User is not authorized to perform elasticloadbalancing:DeleteRule.", readOnly, eventID, verified)
		return
	}
	err := s.store.DeleteELBv2Rule(verified.AccountID, arn)
	if errors.Is(err, store.ErrELBv2RuleNotFound) {
		s.writeELBv2Error(w, r, body, requestID, http.StatusBadRequest, "RuleNotFound",
			"Rule not found.", readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeELBv2Error(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to delete rule.", readOnly, eventID, verified)
		return
	}
	payload, _ := elbsvc.DeleteOKJSON()
	s.writeELBv2OK(w, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, elbv2EventSource, "DeleteRule", readOnly)
}

func elbAttributesFromParams(params map[string]any) map[string]string {
	raw, _ := params["Attributes"].([]any)
	out := map[string]string{}
	for _, item := range raw {
		m, _ := item.(map[string]any)
		k, _ := m["Key"].(string)
		v, _ := m["Value"].(string)
		if strings.TrimSpace(k) != "" {
			out[k] = v
		}
	}
	return out
}

func (s *Server) elbModifyLoadBalancerAttributes(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	lbARN, _ := params["LoadBalancerArn"].(string)
	if !s.authorize(verified, "elasticloadbalancing:ModifyLoadBalancerAttributes", lbARN) {
		s.writeELBv2Error(w, r, body, requestID, http.StatusForbidden, "AccessDenied",
			"User is not authorized to perform elasticloadbalancing:ModifyLoadBalancerAttributes.", readOnly, eventID, verified)
		return
	}
	attrs := elbAttributesFromParams(params)
	if len(attrs) == 0 {
		s.writeELBv2Error(w, r, body, requestID, http.StatusBadRequest, "ValidationError",
			"Attributes required.", readOnly, eventID, verified)
		return
	}
	err := s.store.SetELBv2LoadBalancerAttributes(verified.AccountID, lbARN, attrs)
	if errors.Is(err, store.ErrELBv2NotFound) {
		s.writeELBv2Error(w, r, body, requestID, http.StatusBadRequest, "LoadBalancerNotFound",
			"Load balancer not found.", readOnly, eventID, verified)
		return
	}
	if errors.Is(err, store.ErrELBv2BadRequest) {
		s.writeELBv2Error(w, r, body, requestID, http.StatusBadRequest, "ValidationError",
			err.Error(), readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeELBv2Error(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to modify load balancer attributes.", readOnly, eventID, verified)
		return
	}
	desc, err := s.store.DescribeELBv2LoadBalancerAttributes(verified.AccountID, lbARN)
	if err != nil {
		s.writeELBv2Error(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to describe load balancer attributes.", readOnly, eventID, verified)
		return
	}
	payload, _ := json.Marshal(map[string]any{"Attributes": desc})
	s.writeELBv2OK(w, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, elbv2EventSource, "ModifyLoadBalancerAttributes", readOnly)
}

func (s *Server) elbDescribeLoadBalancerAttributes(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	lbARN, _ := params["LoadBalancerArn"].(string)
	if !s.authorize(verified, "elasticloadbalancing:DescribeLoadBalancerAttributes", lbARN) {
		s.writeELBv2Error(w, r, body, requestID, http.StatusForbidden, "AccessDenied",
			"User is not authorized to perform elasticloadbalancing:DescribeLoadBalancerAttributes.", readOnly, eventID, verified)
		return
	}
	desc, err := s.store.DescribeELBv2LoadBalancerAttributes(verified.AccountID, lbARN)
	if errors.Is(err, store.ErrELBv2NotFound) {
		s.writeELBv2Error(w, r, body, requestID, http.StatusBadRequest, "LoadBalancerNotFound",
			"Load balancer not found.", readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeELBv2Error(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to describe load balancer attributes.", readOnly, eventID, verified)
		return
	}
	payload, _ := json.Marshal(map[string]any{"Attributes": desc})
	s.writeELBv2OK(w, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, elbv2EventSource, "DescribeLoadBalancerAttributes", readOnly)
}

func (s *Server) writeELBv2OK(w http.ResponseWriter, payload []byte) {
	w.Header().Set("Content-Type", elbv2JSONContentType)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(payload)
}

func (s *Server) writeELBv2Error(
	w http.ResponseWriter, r *http.Request, body []byte, requestID string, status int, code, message string,
	readOnly bool, eventID string, verified *authn.Verified,
) {
	w.Header().Set("Content-Type", elbv2JSONContentType)
	w.Header().Set("x-amzn-ErrorType", code)
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"__type": code, "message": message})
	_ = body
	s.writeSuccessAudit(r, requestID, eventID, verified, elbv2EventSource, code, readOnly)
}

