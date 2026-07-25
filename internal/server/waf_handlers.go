package server

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/Kyaxris-Labs/Noctaxris/internal/catalog"
	"github.com/Kyaxris-Labs/Noctaxris/internal/kernel/authn"
	wafsvc "github.com/Kyaxris-Labs/Noctaxris/internal/services/wafv2"
	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

const (
	wafJSONContentType = "application/x-amz-json-1.1"
	wafEventSource     = "wafv2.amazonaws.com"
)

func (s *Server) handleWAFv2(
	w http.ResponseWriter,
	r *http.Request,
	body []byte,
	requestID, eventID, action string,
	verified *authn.Verified,
	readOnly bool,
) {
	params := jsonBodyMap(body)
	action = wafAction(action)

	switch action {
	case catalog.ActionWAFCreateWebACL:
		s.wafCreateWebACL(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionWAFUpdateWebACL:
		s.wafUpdateWebACL(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionWAFGetWebACL:
		s.wafGetWebACL(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionWAFListWebACLs:
		s.wafListWebACLs(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionWAFCreateRuleGroup:
		s.wafCreateRuleGroup(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionWAFAssociateWebACL:
		s.wafAssociate(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionWAFEvaluate:
		s.wafEvaluate(w, r, body, requestID, eventID, verified, readOnly, params)
	default:
		s.writeWAFError(w, r, body, requestID, http.StatusNotImplemented, "WAFInternalErrorException",
			"This WAFv2 action is not implemented.", readOnly, eventID, verified)
	}
}

func wafAction(action string) string {
	if strings.Contains(action, ":") {
		return action
	}
	switch action {
	case "CreateWebACL":
		return catalog.ActionWAFCreateWebACL
	case "UpdateWebACL":
		return catalog.ActionWAFUpdateWebACL
	case "GetWebACL":
		return catalog.ActionWAFGetWebACL
	case "ListWebACLs":
		return catalog.ActionWAFListWebACLs
	case "CreateRuleGroup":
		return catalog.ActionWAFCreateRuleGroup
	case "AssociateWebACL":
		return catalog.ActionWAFAssociateWebACL
	case "Evaluate":
		return catalog.ActionWAFEvaluate
	default:
		return action
	}
}

func parseWAFRules(params map[string]any) []store.WAFRule {
	raw, _ := params["Rules"].([]any)
	var rules []store.WAFRule
	for _, r := range raw {
		m, _ := r.(map[string]any)
		name, _ := m["Name"].(string)
		label, _ := m["Label"].(string)
		action := wafRuleActionFromMap(m)
		prio := 0
		switch p := m["Priority"].(type) {
		case float64:
			prio = int(p)
		}
		rule := store.WAFRule{Name: name, Priority: prio, Action: action, Label: label}
		if bm := parseWAFByteMatchStatement(m); bm != nil {
			rule.ByteMatchStatement = bm
			rule.Label = ""
			rules = append(rules, rule)
			continue
		}
		if sc := parseWAFSizeConstraintStatement(m); sc != nil {
			rule.SizeConstraintStatement = sc
			rule.Label = ""
			rules = append(rules, rule)
			continue
		}
		if ip := parseWAFIPSetReferenceStatement(m); ip != nil {
			rule.IPSetReferenceStatement = ip
			rule.Label = ""
			rules = append(rules, rule)
			continue
		}
		rules = append(rules, rule)
	}
	return rules
}

func wafRuleActionFromMap(m map[string]any) string {
	action := "Allow"
	if a, ok := m["Action"].(map[string]any); ok {
		if _, ok := a["Block"]; ok {
			action = "Block"
		}
	} else if as, ok := m["Action"].(string); ok {
		action = as
	}
	return action
}

func parseWAFStatementMap(rule map[string]any, key string) map[string]any {
	if stmt, ok := rule["Statement"].(map[string]any); ok {
		if m, ok := stmt[key].(map[string]any); ok {
			return m
		}
	}
	if m, ok := rule[key].(map[string]any); ok {
		return m
	}
	return nil
}

func parseWAFByteMatchStatement(rule map[string]any) *store.WAFByteMatchStatement {
	bm := parseWAFStatementMap(rule, "ByteMatchStatement")
	if bm == nil {
		return nil
	}
	search, _ := bm["SearchString"].(string)
	constraint, _ := bm["PositionalConstraint"].(string)
	ftm, _ := bm["FieldToMatch"].(map[string]any)
	if ftm == nil {
		return nil
	}
	out := &store.WAFByteMatchStatement{
		SearchString:         search,
		PositionalConstraint: constraint,
	}
	if _, ok := ftm["UriPath"]; ok {
		out.FieldToMatchType = "UriPath"
		return out
	}
	if sh, ok := ftm["SingleHeader"].(map[string]any); ok {
		out.FieldToMatchType = "SingleHeader"
		out.HeaderName, _ = sh["Name"].(string)
		return out
	}
	return nil
}

func parseWAFSizeConstraintStatement(rule map[string]any) *store.WAFSizeConstraintStatement {
	sc := parseWAFStatementMap(rule, "SizeConstraintStatement")
	if sc == nil {
		return nil
	}
	op, _ := sc["ComparisonOperator"].(string)
	ftm, _ := sc["FieldToMatch"].(map[string]any)
	if ftm == nil {
		return nil
	}
	var size int64
	switch v := sc["Size"].(type) {
	case float64:
		size = int64(v)
	case int:
		size = int64(v)
	case int64:
		size = v
	case json.Number:
		n, err := v.Int64()
		if err != nil {
			return nil
		}
		size = n
	default:
		return nil
	}
	out := &store.WAFSizeConstraintStatement{
		ComparisonOperator: op,
		Size:               size,
	}
	if _, ok := ftm["UriPath"]; ok {
		out.FieldToMatchType = "UriPath"
		return out
	}
	if sh, ok := ftm["SingleHeader"].(map[string]any); ok {
		out.FieldToMatchType = "SingleHeader"
		out.HeaderName, _ = sh["Name"].(string)
		return out
	}
	return nil
}

func parseWAFIPSetReferenceStatement(rule map[string]any) *store.WAFIPSetReferenceStatement {
	ip := parseWAFStatementMap(rule, "IPSetReferenceStatement")
	if ip == nil {
		return nil
	}
	raw, _ := ip["Addresses"].([]any)
	if len(raw) == 0 {
		return nil
	}
	addrs := make([]string, 0, len(raw))
	for _, a := range raw {
		s, ok := a.(string)
		if !ok {
			continue
		}
		s = strings.TrimSpace(s)
		if s != "" {
			addrs = append(addrs, s)
		}
	}
	if len(addrs) == 0 {
		return nil
	}
	return &store.WAFIPSetReferenceStatement{Addresses: addrs}
}

func parseWAFDefaultAction(params map[string]any) string {
	if da, ok := params["DefaultAction"].(map[string]any); ok {
		if _, ok := da["Block"]; ok {
			return "Block"
		}
		return "Allow"
	}
	if s, ok := params["DefaultAction"].(string); ok {
		return s
	}
	return "Allow"
}

func (s *Server) wafCreateWebACL(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	name, _ := params["Name"].(string)
	scope, _ := params["Scope"].(string)
	desc, _ := params["Description"].(string)
	if !s.authorize(verified, catalog.ActionWAFCreateWebACL, "*") {
		s.writeWAFError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform wafv2:CreateWebACL.", readOnly, eventID, verified)
		return
	}
	region := verified.Region
	if region == "" {
		region = store.DefaultWAFRegion
	}
	acl, err := s.store.CreateWAFWebACL(verified.AccountID, region, name, scope, desc, parseWAFDefaultAction(params), parseWAFRules(params))
	if errors.Is(err, store.ErrWAFAlreadyExists) {
		s.writeWAFError(w, r, body, requestID, http.StatusBadRequest, "WAFDuplicateItemException",
			"WebACL already exists.", readOnly, eventID, verified)
		return
	}
	if errors.Is(err, store.ErrWAFBadRequest) {
		s.writeWAFError(w, r, body, requestID, http.StatusBadRequest, "WAFInvalidParameterException",
			err.Error(), readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeWAFError(w, r, body, requestID, http.StatusInternalServerError, "WAFInternalErrorException",
			"Unable to create WebACL.", readOnly, eventID, verified)
		return
	}
	payload, _ := wafsvc.CreateWebACLJSON(acl)
	s.writeWAFOK(w, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, wafEventSource, "CreateWebACL", readOnly)
}

func (s *Server) wafUpdateWebACL(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	name, _ := params["Name"].(string)
	scope, _ := params["Scope"].(string)
	lock, _ := params["LockToken"].(string)
	if !s.authorize(verified, catalog.ActionWAFUpdateWebACL, "*") {
		s.writeWAFError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform wafv2:UpdateWebACL.", readOnly, eventID, verified)
		return
	}
	acl, err := s.store.UpdateWAFWebACL(verified.AccountID, name, scope, lock, parseWAFDefaultAction(params), parseWAFRules(params))
	if errors.Is(err, store.ErrWAFNotFound) {
		s.writeWAFError(w, r, body, requestID, http.StatusBadRequest, "WAFNonexistentItemException",
			"WebACL not found.", readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeWAFError(w, r, body, requestID, http.StatusBadRequest, "WAFInvalidParameterException",
			err.Error(), readOnly, eventID, verified)
		return
	}
	payload, _ := wafsvc.UpdateWebACLJSON(acl)
	s.writeWAFOK(w, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, wafEventSource, "UpdateWebACL", readOnly)
}

func (s *Server) wafGetWebACL(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	name, _ := params["Name"].(string)
	scope, _ := params["Scope"].(string)
	id, _ := params["Id"].(string)
	if !s.authorize(verified, catalog.ActionWAFGetWebACL, "*") {
		s.writeWAFError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform wafv2:GetWebACL.", readOnly, eventID, verified)
		return
	}
	acl, err := s.store.GetWAFWebACL(verified.AccountID, name, scope, id)
	if errors.Is(err, store.ErrWAFNotFound) {
		s.writeWAFError(w, r, body, requestID, http.StatusBadRequest, "WAFNonexistentItemException",
			"WebACL not found.", readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeWAFError(w, r, body, requestID, http.StatusInternalServerError, "WAFInternalErrorException",
			"Unable to get WebACL.", readOnly, eventID, verified)
		return
	}
	payload, _ := wafsvc.GetWebACLJSON(acl)
	s.writeWAFOK(w, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, wafEventSource, "GetWebACL", readOnly)
}

func (s *Server) wafListWebACLs(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	scope, _ := params["Scope"].(string)
	if !s.authorize(verified, catalog.ActionWAFListWebACLs, "*") {
		s.writeWAFError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform wafv2:ListWebACLs.", readOnly, eventID, verified)
		return
	}
	acls, err := s.store.ListWAFWebACLs(verified.AccountID, scope)
	if err != nil {
		s.writeWAFError(w, r, body, requestID, http.StatusInternalServerError, "WAFInternalErrorException",
			"Unable to list WebACLs.", readOnly, eventID, verified)
		return
	}
	payload, _ := wafsvc.ListWebACLsJSON(acls)
	s.writeWAFOK(w, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, wafEventSource, "ListWebACLs", readOnly)
}

func (s *Server) wafCreateRuleGroup(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	name, _ := params["Name"].(string)
	scope, _ := params["Scope"].(string)
	capacity := 1
	if c, ok := params["Capacity"].(float64); ok {
		capacity = int(c)
	}
	if !s.authorize(verified, catalog.ActionWAFCreateRuleGroup, "*") {
		s.writeWAFError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform wafv2:CreateRuleGroup.", readOnly, eventID, verified)
		return
	}
	region := verified.Region
	if region == "" {
		region = store.DefaultWAFRegion
	}
	g, err := s.store.CreateWAFRuleGroup(verified.AccountID, region, name, scope, capacity, parseWAFRules(params))
	if errors.Is(err, store.ErrWAFAlreadyExists) {
		s.writeWAFError(w, r, body, requestID, http.StatusBadRequest, "WAFDuplicateItemException",
			"Rule group already exists.", readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeWAFError(w, r, body, requestID, http.StatusBadRequest, "WAFInvalidParameterException",
			err.Error(), readOnly, eventID, verified)
		return
	}
	payload, _ := wafsvc.CreateRuleGroupJSON(g)
	s.writeWAFOK(w, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, wafEventSource, "CreateRuleGroup", readOnly)
}

func (s *Server) wafAssociate(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	webARN, _ := params["WebACLArn"].(string)
	resARN, _ := params["ResourceArn"].(string)
	if !s.authorize(verified, catalog.ActionWAFAssociateWebACL, "*") {
		s.writeWAFError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform wafv2:AssociateWebACL.", readOnly, eventID, verified)
		return
	}
	err := s.store.AssociateWAFWebACL(verified.AccountID, webARN, resARN)
	if err != nil {
		if errors.Is(err, store.ErrWAFNotFound) {
			s.writeWAFError(w, r, body, requestID, http.StatusBadRequest, "WAFNonexistentItemException",
				"WebACL not found.", readOnly, eventID, verified)
			return
		}
		s.writeWAFError(w, r, body, requestID, http.StatusBadRequest, "WAFInvalidParameterException",
			err.Error(), readOnly, eventID, verified)
		return
	}
	payload, _ := wafsvc.AssociateWebACLJSON()
	s.writeWAFOK(w, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, wafEventSource, "AssociateWebACL", readOnly)
}

func (s *Server) wafEvaluate(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	webARN, _ := params["WebACLArn"].(string)
	label, _ := params["Label"].(string)
	view := wafRequestViewFromEvaluateParams(params)
	if !s.authorize(verified, catalog.ActionWAFEvaluate, "*") {
		s.writeWAFError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform wafv2:Evaluate.", readOnly, eventID, verified)
		return
	}
	action, err := s.store.EvaluateWAFRequestWithView(verified.AccountID, webARN, label, view)
	if errors.Is(err, store.ErrWAFNotFound) {
		s.writeWAFError(w, r, body, requestID, http.StatusBadRequest, "WAFNonexistentItemException",
			"WebACL not found.", readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeWAFError(w, r, body, requestID, http.StatusInternalServerError, "WAFInternalErrorException",
			"Unable to evaluate.", readOnly, eventID, verified)
		return
	}
	payload, _ := wafsvc.EvaluateJSON(action)
	s.writeWAFOK(w, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, wafEventSource, "Evaluate", readOnly)
}

func (s *Server) writeWAFOK(w http.ResponseWriter, payload []byte) {
	w.Header().Set("Content-Type", wafJSONContentType)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(payload)
}

func (s *Server) writeWAFError(
	w http.ResponseWriter, r *http.Request, body []byte, requestID string, status int, code, message string,
	readOnly bool, eventID string, verified *authn.Verified,
) {
	w.Header().Set("Content-Type", wafJSONContentType)
	w.Header().Set("x-amzn-ErrorType", code)
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"__type": code, "message": message})
	_ = body
	s.writeSuccessAudit(r, requestID, eventID, verified, wafEventSource, code, readOnly)
}
