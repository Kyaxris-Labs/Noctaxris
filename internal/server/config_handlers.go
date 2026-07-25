package server

import (
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/Kyaxris-Labs/Noctaxris/internal/catalog"
	"github.com/Kyaxris-Labs/Noctaxris/internal/kernel/authn"
	"github.com/Kyaxris-Labs/Noctaxris/internal/kernel/authz"
	"github.com/Kyaxris-Labs/Noctaxris/internal/kernel/sts"
	configsvc "github.com/Kyaxris-Labs/Noctaxris/internal/services/config"
	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

const configEventSource = "config.amazonaws.com"

func (s *Server) handleConfig(
	w http.ResponseWriter,
	r *http.Request,
	body []byte,
	requestID, eventID, action string,
	verified *authn.Verified,
	readOnly bool,
) {
	params := formParams(r, body)
	action = configAction(action)

	switch action {
	case catalog.ActionConfigPutConfigurationRecorder:
		s.cfgPutRecorder(w, r, requestID, eventID, verified, readOnly, params)
	case catalog.ActionConfigPutDeliveryChannel:
		s.cfgPutDelivery(w, r, requestID, eventID, verified, readOnly, params)
	case catalog.ActionConfigStartConfigurationRecorder:
		s.cfgStartRecorder(w, r, requestID, eventID, verified, readOnly, params)
	case catalog.ActionConfigDescribeComplianceByConfigRule:
		s.cfgDescribeCompliance(w, r, requestID, eventID, verified, readOnly, params)
	case catalog.ActionConfigGetResourceConfigHistory:
		s.cfgGetResourceConfigHistory(w, r, requestID, eventID, verified, readOnly, params)
	default:
		s.writeConfigError(w, r, requestID, http.StatusBadRequest, "InvalidAction",
			"This Config action is not implemented.", readOnly, eventID, verified)
	}
}

func configAction(action string) string {
	if strings.Contains(action, ":") {
		return action
	}
	switch action {
	case "PutConfigurationRecorder":
		return catalog.ActionConfigPutConfigurationRecorder
	case "PutDeliveryChannel":
		return catalog.ActionConfigPutDeliveryChannel
	case "StartConfigurationRecorder":
		return catalog.ActionConfigStartConfigurationRecorder
	case "DescribeComplianceByConfigRule":
		return catalog.ActionConfigDescribeComplianceByConfigRule
	case "GetResourceConfigHistory":
		return catalog.ActionConfigGetResourceConfigHistory
	default:
		return action
	}
}

func (s *Server) checkConfigPassRole(verified *authn.Verified, roleARN string) error {
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
		return errors.New("not authorized to pass role to Config")
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
		ServicePrincipal: authz.ServicePrincipalConfig,
	})
	if decision != authz.Allow {
		return errors.New("not authorized to pass role to Config")
	}
	return nil
}

func (s *Server) cfgPutRecorder(
	w http.ResponseWriter, r *http.Request, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params url.Values,
) {
	name := strings.TrimSpace(params.Get("ConfigurationRecorder.Name"))
	if name == "" {
		name = strings.TrimSpace(params.Get("ConfigurationRecorder.name"))
	}
	roleARN := strings.TrimSpace(params.Get("ConfigurationRecorder.roleARN"))
	if roleARN == "" {
		roleARN = strings.TrimSpace(params.Get("ConfigurationRecorder.RoleARN"))
	}
	if name == "" {
		s.writeConfigError(w, r, requestID, http.StatusBadRequest, "InvalidParameterValueException",
			"ConfigurationRecorder.Name is required.", readOnly, eventID, verified)
		return
	}
	if !s.authorize(verified, catalog.ActionConfigPutConfigurationRecorder, "*") {
		s.writeConfigError(w, r, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform config:PutConfigurationRecorder.", readOnly, eventID, verified)
		return
	}
	if roleARN != "" {
		if err := s.checkConfigPassRole(verified, roleARN); err != nil {
			s.writeConfigError(w, r, requestID, http.StatusForbidden, "AccessDeniedException",
				err.Error(), readOnly, eventID, verified)
			return
		}
	}
	_, err := s.store.PutConfigRecorder(verified.AccountID, name, roleARN, "ALL")
	if err != nil {
		s.writeConfigError(w, r, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to put configuration recorder.", readOnly, eventID, verified)
		return
	}
	payload, _ := configsvc.PutConfigurationRecorderXML(requestID)
	s.writeConfigOK(w, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, configEventSource, "PutConfigurationRecorder", readOnly)
}

func (s *Server) cfgPutDelivery(
	w http.ResponseWriter, r *http.Request, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params url.Values,
) {
	name := strings.TrimSpace(params.Get("DeliveryChannel.Name"))
	bucket := strings.TrimSpace(params.Get("DeliveryChannel.s3BucketName"))
	if bucket == "" {
		bucket = strings.TrimSpace(params.Get("DeliveryChannel.S3BucketName"))
	}
	prefix := strings.TrimSpace(params.Get("DeliveryChannel.s3KeyPrefix"))
	sns := strings.TrimSpace(params.Get("DeliveryChannel.snsTopicARN"))
	if name == "" || bucket == "" {
		s.writeConfigError(w, r, requestID, http.StatusBadRequest, "InvalidParameterValueException",
			"DeliveryChannel.Name and s3BucketName are required.", readOnly, eventID, verified)
		return
	}
	if !s.authorize(verified, catalog.ActionConfigPutDeliveryChannel, "*") {
		s.writeConfigError(w, r, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform config:PutDeliveryChannel.", readOnly, eventID, verified)
		return
	}
	_, err := s.store.PutConfigDeliveryChannel(verified.AccountID, name, bucket, prefix, sns)
	if errors.Is(err, store.ErrConfigBadRequest) {
		s.writeConfigError(w, r, requestID, http.StatusBadRequest, "InvalidParameterValueException",
			err.Error(), readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeConfigError(w, r, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to put delivery channel.", readOnly, eventID, verified)
		return
	}
	payload, _ := configsvc.PutDeliveryChannelXML(requestID)
	s.writeConfigOK(w, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, configEventSource, "PutDeliveryChannel", readOnly)
}

func (s *Server) cfgStartRecorder(
	w http.ResponseWriter, r *http.Request, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params url.Values,
) {
	name := strings.TrimSpace(params.Get("ConfigurationRecorderName"))
	if name == "" {
		s.writeConfigError(w, r, requestID, http.StatusBadRequest, "InvalidParameterValueException",
			"ConfigurationRecorderName is required.", readOnly, eventID, verified)
		return
	}
	if !s.authorize(verified, catalog.ActionConfigStartConfigurationRecorder, "*") {
		s.writeConfigError(w, r, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform config:StartConfigurationRecorder.", readOnly, eventID, verified)
		return
	}
	err := s.store.StartConfigRecorder(verified.AccountID, name)
	if errors.Is(err, store.ErrConfigNotFound) {
		s.writeConfigError(w, r, requestID, http.StatusBadRequest, "NoSuchConfigurationRecorderException",
			"Configuration recorder not found.", readOnly, eventID, verified)
		return
	}
	if errors.Is(err, store.ErrConfigBadRequest) {
		s.writeConfigError(w, r, requestID, http.StatusBadRequest, "InvalidParameterValueException",
			err.Error(), readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeConfigError(w, r, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to start configuration recorder.", readOnly, eventID, verified)
		return
	}
	s.store.NotifyConfigDeliveryChannelsSNS(verified.AccountID, name)
	payload, _ := configsvc.StartConfigurationRecorderXML(requestID)
	s.writeConfigOK(w, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, configEventSource, "StartConfigurationRecorder", readOnly)
}

func (s *Server) cfgDescribeCompliance(
	w http.ResponseWriter, r *http.Request, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params url.Values,
) {
	ruleName := strings.TrimSpace(params.Get("ConfigRuleNames.member.1"))
	if ruleName == "" {
		ruleName = strings.TrimSpace(params.Get("ConfigRuleName"))
	}
	if !s.authorize(verified, catalog.ActionConfigDescribeComplianceByConfigRule, "*") {
		s.writeConfigError(w, r, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform config:DescribeComplianceByConfigRule.", readOnly, eventID, verified)
		return
	}
	results, err := s.store.DescribeConfigComplianceByRule(verified.AccountID, ruleName)
	if err != nil {
		s.writeConfigError(w, r, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to describe compliance.", readOnly, eventID, verified)
		return
	}
	payload, _ := configsvc.DescribeComplianceByConfigRuleXML(results, requestID)
	s.writeConfigOK(w, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, configEventSource, "DescribeComplianceByConfigRule", readOnly)
}

func (s *Server) cfgGetResourceConfigHistory(
	w http.ResponseWriter, r *http.Request, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params url.Values,
) {
	resourceType := strings.TrimSpace(params.Get("resourceType"))
	resourceID := strings.TrimSpace(params.Get("resourceId"))
	if resourceID == "" {
		resourceID = strings.TrimSpace(params.Get("resourceID"))
	}
	if resourceType == "" || resourceID == "" {
		s.writeConfigError(w, r, requestID, http.StatusBadRequest, "InvalidParameterValueException",
			"resourceType and resourceId are required.", readOnly, eventID, verified)
		return
	}
	if !s.authorize(verified, catalog.ActionConfigGetResourceConfigHistory, "*") {
		s.writeConfigError(w, r, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform config:GetResourceConfigHistory.", readOnly, eventID, verified)
		return
	}
	limit := 100
	if v := strings.TrimSpace(params.Get("limit")); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			limit = n
		}
	}
	items, err := s.store.GetResourceConfigHistory(verified.AccountID, resourceType, resourceID, limit)
	if errors.Is(err, store.ErrConfigBadRequest) {
		s.writeConfigError(w, r, requestID, http.StatusBadRequest, "InvalidParameterValueException",
			err.Error(), readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeConfigError(w, r, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to get resource config history.", readOnly, eventID, verified)
		return
	}
	payload, _ := configsvc.GetResourceConfigHistoryXML(items, requestID)
	s.writeConfigOK(w, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, configEventSource, "GetResourceConfigHistory", readOnly)
}

func (s *Server) writeConfigOK(w http.ResponseWriter, payload []byte) {
	w.Header().Set("Content-Type", "text/xml; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(payload)
}

func (s *Server) writeConfigError(
	w http.ResponseWriter, r *http.Request, requestID string, status int, code, message string,
	readOnly bool, eventID string, verified *authn.Verified,
) {
	s.writeAWSError(w, requestID, status, code, message, readOnly, r, eventID, verified.AccessKeyID, verified.AccountID, true)
}
