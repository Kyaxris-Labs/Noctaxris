package server

import (
	"errors"
	"net/http"
	"net/url"
	"strings"

	"github.com/Kyaxris-Labs/Noctaxris/internal/catalog"
	"github.com/Kyaxris-Labs/Noctaxris/internal/kernel/authn"
	"github.com/Kyaxris-Labs/Noctaxris/internal/kernel/authz"
	"github.com/Kyaxris-Labs/Noctaxris/internal/kernel/sts"
	cfnsvc "github.com/Kyaxris-Labs/Noctaxris/internal/services/cloudformation"
	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

const cfnEventSource = "cloudformation.amazonaws.com"

func (s *Server) handleCloudFormation(
	w http.ResponseWriter,
	r *http.Request,
	body []byte,
	requestID, eventID, action string,
	verified *authn.Verified,
	readOnly bool,
) {
	params := formParams(r, body)
	action = cfnAction(action)

	switch action {
	case catalog.ActionCFNCreateStack:
		s.cfnCreateStack(w, r, requestID, eventID, verified, readOnly, params)
	case catalog.ActionCFNDescribeStacks:
		s.cfnDescribeStacks(w, r, requestID, eventID, verified, readOnly, params)
	case catalog.ActionCFNDeleteStack:
		s.cfnDeleteStack(w, r, requestID, eventID, verified, readOnly, params)
	case catalog.ActionCFNListStacks:
		s.cfnListStacks(w, r, requestID, eventID, verified, readOnly)
	default:
		s.writeCFNError(w, r, requestID, http.StatusBadRequest, "InvalidAction",
			"This CloudFormation action is not implemented.", readOnly, eventID, verified)
	}
}

func cfnAction(action string) string {
	if strings.Contains(action, ":") {
		return action
	}
	switch action {
	case "CreateStack":
		return catalog.ActionCFNCreateStack
	case "DescribeStacks":
		return catalog.ActionCFNDescribeStacks
	case "DeleteStack":
		return catalog.ActionCFNDeleteStack
	case "ListStacks":
		return catalog.ActionCFNListStacks
	default:
		return action
	}
}

func (s *Server) cfnRegion(verified *authn.Verified) string {
	if verified.Region != "" {
		return verified.Region
	}
	return store.DefaultCFNRegion
}

func (s *Server) checkCFNPassRole(verified *authn.Verified, roleARN string) error {
	accountID, roleName, ok := sts.ParseRoleARN(roleARN)
	if !ok {
		return errors.New("RoleARN must be a valid IAM role ARN")
	}
	if accountID != verified.AccountID {
		return errors.New("RoleARN must be in the same account")
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
		return errors.New("not authorized to pass role to CloudFormation")
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
		ServicePrincipal: authz.ServicePrincipalCloudFormation,
	})
	if decision != authz.Allow {
		return errors.New("not authorized to pass role to CloudFormation")
	}
	return nil
}

func (s *Server) cfnCreateStack(
	w http.ResponseWriter, r *http.Request, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params url.Values,
) {
	name := strings.TrimSpace(params.Get("StackName"))
	template := params.Get("TemplateBody")
	roleARN := strings.TrimSpace(params.Get("RoleARN"))
	if name == "" || strings.TrimSpace(template) == "" {
		s.writeCFNError(w, r, requestID, http.StatusBadRequest, "ValidationError",
			"StackName and TemplateBody are required.", readOnly, eventID, verified)
		return
	}
	if !s.authorize(verified, catalog.ActionCFNCreateStack, "*") {
		s.writeCFNError(w, r, requestID, http.StatusForbidden, "AccessDenied",
			"User is not authorized to perform cloudformation:CreateStack.", readOnly, eventID, verified)
		return
	}
	if roleARN != "" {
		if err := s.checkCFNPassRole(verified, roleARN); err != nil {
			s.writeCFNError(w, r, requestID, http.StatusForbidden, "AccessDenied",
				err.Error(), readOnly, eventID, verified)
			return
		}
	}
	st, err := s.store.CreateCFNStack(verified.AccountID, s.cfnRegion(verified), name, template, roleARN)
	if errors.Is(err, store.ErrCFNStackExists) {
		s.writeCFNError(w, r, requestID, http.StatusBadRequest, "AlreadyExistsException",
			"Stack already exists.", readOnly, eventID, verified)
		return
	}
	if errors.Is(err, store.ErrCFNBadTemplate) {
		s.writeCFNError(w, r, requestID, http.StatusBadRequest, "ValidationError",
			err.Error(), readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeCFNError(w, r, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to create stack.", readOnly, eventID, verified)
		return
	}
	payload, _ := cfnsvc.CreateStackXML(st.StackID, requestID)
	s.writeCFNOK(w, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, cfnEventSource, "CreateStack", readOnly)
}

func (s *Server) cfnDescribeStacks(
	w http.ResponseWriter, r *http.Request, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params url.Values,
) {
	if !s.authorize(verified, catalog.ActionCFNDescribeStacks, "*") {
		s.writeCFNError(w, r, requestID, http.StatusForbidden, "AccessDenied",
			"User is not authorized to perform cloudformation:DescribeStacks.", readOnly, eventID, verified)
		return
	}
	name := strings.TrimSpace(params.Get("StackName"))
	stacks, err := s.store.DescribeCFNStacks(verified.AccountID, name)
	if errors.Is(err, store.ErrCFNStackNotFound) {
		s.writeCFNError(w, r, requestID, http.StatusBadRequest, "ValidationError",
			"Stack does not exist.", readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeCFNError(w, r, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to describe stacks.", readOnly, eventID, verified)
		return
	}
	payload, _ := cfnsvc.DescribeStacksXML(stacks, requestID)
	s.writeCFNOK(w, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, cfnEventSource, "DescribeStacks", readOnly)
}

func (s *Server) cfnDeleteStack(
	w http.ResponseWriter, r *http.Request, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params url.Values,
) {
	name := strings.TrimSpace(params.Get("StackName"))
	if name == "" {
		s.writeCFNError(w, r, requestID, http.StatusBadRequest, "ValidationError",
			"StackName is required.", readOnly, eventID, verified)
		return
	}
	if !s.authorize(verified, catalog.ActionCFNDeleteStack, "*") {
		s.writeCFNError(w, r, requestID, http.StatusForbidden, "AccessDenied",
			"User is not authorized to perform cloudformation:DeleteStack.", readOnly, eventID, verified)
		return
	}
	err := s.store.DeleteCFNStack(verified.AccountID, name)
	if errors.Is(err, store.ErrCFNStackNotFound) {
		s.writeCFNError(w, r, requestID, http.StatusBadRequest, "ValidationError",
			"Stack does not exist.", readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeCFNError(w, r, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to delete stack.", readOnly, eventID, verified)
		return
	}
	payload, _ := cfnsvc.DeleteStackXML(requestID)
	s.writeCFNOK(w, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, cfnEventSource, "DeleteStack", readOnly)
}

func (s *Server) cfnListStacks(
	w http.ResponseWriter, r *http.Request, requestID, eventID string,
	verified *authn.Verified, readOnly bool,
) {
	if !s.authorize(verified, catalog.ActionCFNListStacks, "*") {
		s.writeCFNError(w, r, requestID, http.StatusForbidden, "AccessDenied",
			"User is not authorized to perform cloudformation:ListStacks.", readOnly, eventID, verified)
		return
	}
	stacks, err := s.store.ListCFNStacks(verified.AccountID)
	if err != nil {
		s.writeCFNError(w, r, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to list stacks.", readOnly, eventID, verified)
		return
	}
	payload, _ := cfnsvc.ListStacksXML(stacks, requestID)
	s.writeCFNOK(w, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, cfnEventSource, "ListStacks", readOnly)
}

func (s *Server) writeCFNOK(w http.ResponseWriter, payload []byte) {
	w.Header().Set("Content-Type", "text/xml; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(payload)
}

func (s *Server) writeCFNError(
	w http.ResponseWriter, r *http.Request, requestID string, status int, code, message string,
	readOnly bool, eventID string, verified *authn.Verified,
) {
	s.writeAWSError(w, requestID, status, code, message, readOnly, r, eventID, verified.AccessKeyID, verified.AccountID, true)
}
