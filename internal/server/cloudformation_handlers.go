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
	case catalog.ActionCFNUpdateStack:
		s.cfnUpdateStack(w, r, requestID, eventID, verified, readOnly, params)
	case catalog.ActionCFNCreateChangeSet:
		s.cfnCreateChangeSet(w, r, requestID, eventID, verified, readOnly, params)
	case catalog.ActionCFNDescribeChangeSet:
		s.cfnDescribeChangeSet(w, r, requestID, eventID, verified, readOnly, params)
	case catalog.ActionCFNExecuteChangeSet:
		s.cfnExecuteChangeSet(w, r, requestID, eventID, verified, readOnly, params)
	case catalog.ActionCFNDetectStackDrift:
		s.cfnDetectStackDrift(w, r, requestID, eventID, verified, readOnly, params)
	case catalog.ActionCFNDescribeStackDriftDetectionStatus:
		s.cfnDescribeStackDriftDetectionStatus(w, r, requestID, eventID, verified, readOnly, params)
	case catalog.ActionCFNDescribeStackResourceDrifts:
		s.cfnDescribeStackResourceDrifts(w, r, requestID, eventID, verified, readOnly, params)
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
	case "UpdateStack":
		return catalog.ActionCFNUpdateStack
	case "CreateChangeSet":
		return catalog.ActionCFNCreateChangeSet
	case "DescribeChangeSet":
		return catalog.ActionCFNDescribeChangeSet
	case "ExecuteChangeSet":
		return catalog.ActionCFNExecuteChangeSet
	case "DetectStackDrift":
		return catalog.ActionCFNDetectStackDrift
	case "DescribeStackDriftDetectionStatus":
		return catalog.ActionCFNDescribeStackDriftDetectionStatus
	case "DescribeStackResourceDrifts":
		return catalog.ActionCFNDescribeStackResourceDrifts
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
	caps := cfnCapabilitiesFromParams(params)
	authz, err := s.newCFNAuthorizer(verified, roleARN)
	if err != nil {
		s.writeCFNError(w, r, requestID, http.StatusForbidden, "AccessDenied",
			err.Error(), readOnly, eventID, verified)
		return
	}
	st, err := s.store.CreateCFNStackAuthorized(verified.AccountID, s.cfnRegion(verified), name, template, roleARN, caps, authz)
	if errors.Is(err, store.ErrCFNStackExists) {
		s.writeCFNError(w, r, requestID, http.StatusBadRequest, "AlreadyExistsException",
			"Stack already exists.", readOnly, eventID, verified)
		return
	}
	if errors.Is(err, store.ErrCFNInsufficientCapabilities) {
		s.writeCFNError(w, r, requestID, http.StatusBadRequest, "InsufficientCapabilitiesException",
			err.Error(), readOnly, eventID, verified)
		return
	}
	if errors.Is(err, store.ErrCFNAccessDenied) {
		s.writeCFNError(w, r, requestID, http.StatusForbidden, "AccessDenied",
			err.Error(), readOnly, eventID, verified)
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

func (s *Server) cfnUpdateStack(
	w http.ResponseWriter, r *http.Request, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params url.Values,
) {
	name := strings.TrimSpace(params.Get("StackName"))
	template := strings.TrimSpace(params.Get("TemplateBody"))
	if name == "" || template == "" {
		s.writeCFNError(w, r, requestID, http.StatusBadRequest, "ValidationError",
			"StackName and TemplateBody are required.", readOnly, eventID, verified)
		return
	}
	if !s.authorize(verified, catalog.ActionCFNUpdateStack, "*") {
		s.writeCFNError(w, r, requestID, http.StatusForbidden, "AccessDenied",
			"User is not authorized to perform cloudformation:UpdateStack.", readOnly, eventID, verified)
		return
	}
	caps := cfnCapabilitiesFromParams(params)
	authz, aerr := s.newCFNAuthorizer(verified, "")
	if aerr != nil {
		s.writeCFNError(w, r, requestID, http.StatusForbidden, "AccessDenied",
			aerr.Error(), readOnly, eventID, verified)
		return
	}
	st, err := s.store.UpdateCFNStackAuthorized(verified.AccountID, s.cfnRegion(verified), name, template, caps, authz)
	if errors.Is(err, store.ErrCFNStackNotFound) {
		s.writeCFNError(w, r, requestID, http.StatusBadRequest, "ValidationError", "Stack does not exist.", readOnly, eventID, verified)
		return
	}
	if errors.Is(err, store.ErrCFNInsufficientCapabilities) {
		s.writeCFNError(w, r, requestID, http.StatusBadRequest, "InsufficientCapabilitiesException", err.Error(), readOnly, eventID, verified)
		return
	}
	if errors.Is(err, store.ErrCFNAccessDenied) {
		s.writeCFNError(w, r, requestID, http.StatusForbidden, "AccessDenied", err.Error(), readOnly, eventID, verified)
		return
	}
	if errors.Is(err, store.ErrCFNBadTemplate) {
		s.writeCFNError(w, r, requestID, http.StatusBadRequest, "ValidationError", err.Error(), readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeCFNError(w, r, requestID, http.StatusInternalServerError, "InternalFailure", "Unable to update stack.", readOnly, eventID, verified)
		return
	}
	payload, _ := cfnsvc.UpdateStackXML(st.StackID, requestID)
	s.writeCFNOK(w, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, cfnEventSource, "UpdateStack", readOnly)
}

func (s *Server) cfnCreateChangeSet(
	w http.ResponseWriter, r *http.Request, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params url.Values,
) {
	name := strings.TrimSpace(params.Get("StackName"))
	csName := strings.TrimSpace(params.Get("ChangeSetName"))
	template := strings.TrimSpace(params.Get("TemplateBody"))
	if name == "" || csName == "" || template == "" {
		s.writeCFNError(w, r, requestID, http.StatusBadRequest, "ValidationError",
			"StackName, ChangeSetName, and TemplateBody are required.", readOnly, eventID, verified)
		return
	}
	if !s.authorize(verified, catalog.ActionCFNCreateChangeSet, "*") {
		s.writeCFNError(w, r, requestID, http.StatusForbidden, "AccessDenied",
			"User is not authorized to perform cloudformation:CreateChangeSet.", readOnly, eventID, verified)
		return
	}
	cs, err := s.store.CreateCFNChangeSet(verified.AccountID, s.cfnRegion(verified), name, csName, template)
	if errors.Is(err, store.ErrCFNStackNotFound) {
		s.writeCFNError(w, r, requestID, http.StatusBadRequest, "ValidationError", "Stack does not exist.", readOnly, eventID, verified)
		return
	}
	if errors.Is(err, store.ErrCFNBadTemplate) {
		s.writeCFNError(w, r, requestID, http.StatusBadRequest, "ValidationError", err.Error(), readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeCFNError(w, r, requestID, http.StatusInternalServerError, "InternalFailure", "Unable to create change set.", readOnly, eventID, verified)
		return
	}
	payload, _ := cfnsvc.CreateChangeSetXML(cs.ChangeSetID, cs.StackID, requestID)
	s.writeCFNOK(w, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, cfnEventSource, "CreateChangeSet", readOnly)
}

func (s *Server) cfnDescribeChangeSet(
	w http.ResponseWriter, r *http.Request, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params url.Values,
) {
	if !s.authorize(verified, catalog.ActionCFNDescribeChangeSet, "*") {
		s.writeCFNError(w, r, requestID, http.StatusForbidden, "AccessDenied",
			"User is not authorized to perform cloudformation:DescribeChangeSet.", readOnly, eventID, verified)
		return
	}
	cs, err := s.store.DescribeCFNChangeSet(verified.AccountID, params.Get("ChangeSetName"), params.Get("StackName"))
	if errors.Is(err, store.ErrCFNChangeSetNotFound) {
		s.writeCFNError(w, r, requestID, http.StatusBadRequest, "ChangeSetNotFound", "Change set does not exist.", readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeCFNError(w, r, requestID, http.StatusInternalServerError, "InternalFailure", "Unable to describe change set.", readOnly, eventID, verified)
		return
	}
	payload, _ := cfnsvc.DescribeChangeSetXML(cs, requestID)
	s.writeCFNOK(w, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, cfnEventSource, "DescribeChangeSet", readOnly)
}

func (s *Server) cfnExecuteChangeSet(
	w http.ResponseWriter, r *http.Request, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params url.Values,
) {
	if !s.authorize(verified, catalog.ActionCFNExecuteChangeSet, "*") {
		s.writeCFNError(w, r, requestID, http.StatusForbidden, "AccessDenied",
			"User is not authorized to perform cloudformation:ExecuteChangeSet.", readOnly, eventID, verified)
		return
	}
	caps := cfnCapabilitiesFromParams(params)
	authz, aerr := s.newCFNAuthorizer(verified, "")
	if aerr != nil {
		s.writeCFNError(w, r, requestID, http.StatusForbidden, "AccessDenied",
			aerr.Error(), readOnly, eventID, verified)
		return
	}
	_, err := s.store.ExecuteCFNChangeSetAuthorized(verified.AccountID, s.cfnRegion(verified), params.Get("ChangeSetName"), params.Get("StackName"), caps, authz)
	if errors.Is(err, store.ErrCFNChangeSetNotFound) {
		s.writeCFNError(w, r, requestID, http.StatusBadRequest, "ChangeSetNotFound", "Change set does not exist.", readOnly, eventID, verified)
		return
	}
	if errors.Is(err, store.ErrCFNInsufficientCapabilities) {
		s.writeCFNError(w, r, requestID, http.StatusBadRequest, "InsufficientCapabilitiesException", err.Error(), readOnly, eventID, verified)
		return
	}
	if errors.Is(err, store.ErrCFNAccessDenied) {
		s.writeCFNError(w, r, requestID, http.StatusForbidden, "AccessDenied", err.Error(), readOnly, eventID, verified)
		return
	}
	if errors.Is(err, store.ErrCFNBadTemplate) {
		s.writeCFNError(w, r, requestID, http.StatusBadRequest, "ValidationError", err.Error(), readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeCFNError(w, r, requestID, http.StatusInternalServerError, "InternalFailure", "Unable to execute change set.", readOnly, eventID, verified)
		return
	}
	payload, _ := cfnsvc.ExecuteChangeSetXML(requestID)
	s.writeCFNOK(w, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, cfnEventSource, "ExecuteChangeSet", readOnly)
}

func (s *Server) cfnDetectStackDrift(
	w http.ResponseWriter, r *http.Request, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params url.Values,
) {
	if !s.authorize(verified, catalog.ActionCFNDetectStackDrift, "*") {
		s.writeCFNError(w, r, requestID, http.StatusForbidden, "AccessDenied",
			"User is not authorized to perform cloudformation:DetectStackDrift.", readOnly, eventID, verified)
		return
	}
	det, err := s.store.DetectCFNStackDrift(verified.AccountID, params.Get("StackName"))
	if errors.Is(err, store.ErrCFNStackNotFound) {
		s.writeCFNError(w, r, requestID, http.StatusBadRequest, "ValidationError", "Stack does not exist.", readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeCFNError(w, r, requestID, http.StatusInternalServerError, "InternalFailure", "Unable to detect drift.", readOnly, eventID, verified)
		return
	}
	payload, _ := cfnsvc.DetectStackDriftXML(det.StackDriftDetectionID, requestID)
	s.writeCFNOK(w, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, cfnEventSource, "DetectStackDrift", readOnly)
}

func (s *Server) cfnDescribeStackDriftDetectionStatus(
	w http.ResponseWriter, r *http.Request, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params url.Values,
) {
	if !s.authorize(verified, catalog.ActionCFNDescribeStackDriftDetectionStatus, "*") {
		s.writeCFNError(w, r, requestID, http.StatusForbidden, "AccessDenied",
			"User is not authorized to perform cloudformation:DescribeStackDriftDetectionStatus.", readOnly, eventID, verified)
		return
	}
	det, err := s.store.DescribeCFNStackDriftDetectionStatus(verified.AccountID, params.Get("StackDriftDetectionId"))
	if err != nil {
		s.writeCFNError(w, r, requestID, http.StatusBadRequest, "ValidationError", "Drift detection not found.", readOnly, eventID, verified)
		return
	}
	payload, _ := cfnsvc.DetectStackDriftXML(det.StackDriftDetectionID, requestID)
	s.writeCFNOK(w, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, cfnEventSource, "DescribeStackDriftDetectionStatus", readOnly)
}

func (s *Server) cfnDescribeStackResourceDrifts(
	w http.ResponseWriter, r *http.Request, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params url.Values,
) {
	if !s.authorize(verified, catalog.ActionCFNDescribeStackResourceDrifts, "*") {
		s.writeCFNError(w, r, requestID, http.StatusForbidden, "AccessDenied",
			"User is not authorized to perform cloudformation:DescribeStackResourceDrifts.", readOnly, eventID, verified)
		return
	}
	_, err := s.store.DescribeCFNStackResourceDrifts(verified.AccountID, params.Get("StackName"))
	if errors.Is(err, store.ErrCFNStackNotFound) {
		s.writeCFNError(w, r, requestID, http.StatusBadRequest, "ValidationError", "Stack does not exist.", readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeCFNError(w, r, requestID, http.StatusInternalServerError, "InternalFailure", "Unable to describe resource drifts.", readOnly, eventID, verified)
		return
	}
	// Reuse DetectStackDriftXML shape for lab smoke; clients should prefer store/unit coverage.
	payload, _ := cfnsvc.DetectStackDriftXML(params.Get("StackName"), requestID)
	s.writeCFNOK(w, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, cfnEventSource, "DescribeStackResourceDrifts", readOnly)
}
