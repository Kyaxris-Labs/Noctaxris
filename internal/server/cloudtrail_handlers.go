package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/Kyaxris-Labs/Noctaxris/internal/catalog"
	"github.com/Kyaxris-Labs/Noctaxris/internal/kernel/audit"
	"github.com/Kyaxris-Labs/Noctaxris/internal/kernel/authn"
	"github.com/Kyaxris-Labs/Noctaxris/internal/kernel/authz"
	"github.com/Kyaxris-Labs/Noctaxris/internal/kernel/sts"
	cloudtrailsvc "github.com/Kyaxris-Labs/Noctaxris/internal/services/cloudtrail"
	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

var errCloudTrailBadTime = errors.New("invalid time")

const (
	cloudtrailJSONContentType = "application/x-amz-json-1.1"
	cloudtrailEventSource     = "cloudtrail.amazonaws.com"
)

func (s *Server) handleCloudTrail(
	w http.ResponseWriter,
	r *http.Request,
	body []byte,
	requestID, eventID, action string,
	verified *authn.Verified,
	readOnly bool,
) {
	params := jsonBodyMap(body)
	action = cloudtrailAction(action)

	switch action {
	case catalog.ActionCloudTrailLookupEvents:
		s.cloudtrailLookupEvents(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionCloudTrailCreateTrail:
		s.cloudtrailCreateTrail(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionCloudTrailDescribeTrails:
		s.cloudtrailDescribeTrails(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionCloudTrailDeleteTrail:
		s.cloudtrailDeleteTrail(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionCloudTrailStartLogging:
		s.cloudtrailStartLogging(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionCloudTrailStopLogging:
		s.cloudtrailStopLogging(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionCloudTrailInjectEvents:
		s.cloudtrailInjectEvents(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionCloudTrailInjectInsightsEvents:
		s.cloudtrailInjectInsightsEvents(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionCloudTrailPutEventSelectors:
		s.cloudtrailPutEventSelectors(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionCloudTrailGetEventSelectors:
		s.cloudtrailGetEventSelectors(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionCloudTrailValidateLogs:
		s.cloudtrailValidateLogs(w, r, body, requestID, eventID, verified, readOnly, params)
	default:
		s.writeCloudTrailError(w, r, body, requestID, http.StatusNotImplemented, "InternalFailure",
			"This CloudTrail action is not implemented.", readOnly, eventID, verified)
	}
}

func cloudtrailAction(action string) string {
	if strings.Contains(action, ":") {
		return action
	}
	switch action {
	case "LookupEvents":
		return catalog.ActionCloudTrailLookupEvents
	case "CreateTrail":
		return catalog.ActionCloudTrailCreateTrail
	case "DescribeTrails":
		return catalog.ActionCloudTrailDescribeTrails
	case "DeleteTrail":
		return catalog.ActionCloudTrailDeleteTrail
	case "StartLogging":
		return catalog.ActionCloudTrailStartLogging
	case "StopLogging":
		return catalog.ActionCloudTrailStopLogging
	case "InjectEvents":
		return catalog.ActionCloudTrailInjectEvents
	case "InjectInsightsEvents":
		return catalog.ActionCloudTrailInjectInsightsEvents
	case "PutEventSelectors":
		return catalog.ActionCloudTrailPutEventSelectors
	case "GetEventSelectors":
		return catalog.ActionCloudTrailGetEventSelectors
	case "ValidateLogs":
		return catalog.ActionCloudTrailValidateLogs
	default:
		return action
	}
}

func (s *Server) cloudtrailRegion(verified *authn.Verified) string {
	if verified != nil && verified.Region != "" {
		return verified.Region
	}
	return store.DefaultCloudTrailRegion
}

func (s *Server) checkCloudTrailPassRole(verified *authn.Verified, roleARN, sourceARN string) error {
	accountID, roleName, ok := sts.ParseRoleARN(roleARN)
	if !ok {
		return errors.New("Role must be a valid IAM role ARN")
	}
	if accountID != verified.AccountID {
		return errors.New("Role must be in the same account")
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
		return errors.New("not authorized to pass role to CloudTrail")
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
		ServicePrincipal: cloudtrailEventSource,
		SourceArn:        sourceARN,
	})
	if decision != authz.Allow {
		return errors.New("not authorized to pass role to CloudTrail")
	}
	return nil
}

func (s *Server) cloudtrailCreateTrail(
	w http.ResponseWriter,
	r *http.Request,
	body []byte,
	requestID, eventID string,
	verified *authn.Verified,
	readOnly bool,
	params map[string]any,
) {
	if !s.authorize(verified, catalog.ActionCloudTrailCreateTrail, "*") {
		s.writeCloudTrailError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform cloudtrail:CreateTrail.", readOnly, eventID, verified)
		return
	}
	name, _ := params["Name"].(string)
	bucket, _ := params["S3BucketName"].(string)
	prefix, _ := params["S3KeyPrefix"].(string)
	cwGroup, _ := params["CloudWatchLogsLogGroupArn"].(string)
	cwRole, _ := params["CloudWatchLogsRoleArn"].(string)
	name = strings.TrimSpace(name)
	bucket = strings.TrimSpace(bucket)
	if name == "" || bucket == "" {
		s.writeCloudTrailError(w, r, body, requestID, http.StatusBadRequest, "InvalidParameterException",
			"Name and S3BucketName are required.", readOnly, eventID, verified)
		return
	}
	isOrgTrail := cloudtrailBoolParam(params["IsOrganizationTrail"])
	if isOrgTrail && !s.store.IsManagementAccount(verified.AccountID) {
		s.writeCloudTrailError(w, r, body, requestID, http.StatusBadRequest, "InvalidParameterException",
			"Organization trails can only be created in the management account.", readOnly, eventID, verified)
		return
	}
	region := s.cloudtrailRegion(verified)
	trailARN := store.CloudTrailTrailARNFor(region, verified.AccountID, name, isOrgTrail)
	if strings.TrimSpace(cwRole) != "" {
		if err := s.checkCloudTrailPassRole(verified, strings.TrimSpace(cwRole), trailARN); err != nil {
			s.writeCloudTrailError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
				err.Error(), readOnly, eventID, verified)
			return
		}
	}
	trail, err := s.store.CreateCloudTrailTrail(verified.AccountID, store.CloudTrailTrail{
		Name:                      name,
		S3BucketName:              bucket,
		S3KeyPrefix:               prefix,
		CloudWatchLogsLogGroupArn: cwGroup,
		CloudWatchLogsRoleArn:     cwRole,
		HomeRegion:                region,
		IsOrganizationTrail:       isOrgTrail,
	})
	if errors.Is(err, store.ErrCloudTrailAlreadyExists) {
		s.writeCloudTrailError(w, r, body, requestID, http.StatusBadRequest, "TrailAlreadyExistsException",
			"A trail with that name already exists.", readOnly, eventID, verified)
		return
	}
	if errors.Is(err, store.ErrCloudTrailS3Bucket) {
		s.writeCloudTrailError(w, r, body, requestID, http.StatusBadRequest, "S3BucketDoesNotExistException",
			err.Error(), readOnly, eventID, verified)
		return
	}
	if errors.Is(err, store.ErrCloudTrailBadRequest) {
		s.writeCloudTrailError(w, r, body, requestID, http.StatusBadRequest, "InvalidParameterException",
			err.Error(), readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeCloudTrailError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to create trail.", readOnly, eventID, verified)
		return
	}
	payload, _ := json.Marshal(cloudtrailTrailResponse(verified.AccountID, trail))
	s.writeCloudTrailOK(w, requestID, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, cloudtrailEventSource, "CreateTrail", readOnly)
}

func (s *Server) cloudtrailDescribeTrails(
	w http.ResponseWriter,
	r *http.Request,
	body []byte,
	requestID, eventID string,
	verified *authn.Verified,
	readOnly bool,
	params map[string]any,
) {
	if !s.authorize(verified, catalog.ActionCloudTrailDescribeTrails, "*") {
		s.writeCloudTrailError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform cloudtrail:DescribeTrails.", readOnly, eventID, verified)
		return
	}
	var names []string
	if raw, ok := params["trailNameList"].([]any); ok {
		for _, item := range raw {
			if s, ok := item.(string); ok {
				names = append(names, s)
			}
		}
	}
	trails, err := s.store.DescribeCloudTrailTrails(verified.AccountID, names)
	if err != nil {
		s.writeCloudTrailError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to describe trails.", readOnly, eventID, verified)
		return
	}
	list := make([]map[string]any, 0, len(trails))
	for _, t := range trails {
		list = append(list, cloudtrailTrailDescribeEntry(verified.AccountID, t))
	}
	payload, _ := json.Marshal(map[string]any{"trailList": list})
	s.writeCloudTrailOK(w, requestID, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, cloudtrailEventSource, "DescribeTrails", readOnly)
}

func (s *Server) cloudtrailDeleteTrail(
	w http.ResponseWriter,
	r *http.Request,
	body []byte,
	requestID, eventID string,
	verified *authn.Verified,
	readOnly bool,
	params map[string]any,
) {
	if !s.authorize(verified, catalog.ActionCloudTrailDeleteTrail, "*") {
		s.writeCloudTrailError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform cloudtrail:DeleteTrail.", readOnly, eventID, verified)
		return
	}
	name, _ := params["Name"].(string)
	name = strings.TrimSpace(name)
	if name == "" {
		s.writeCloudTrailError(w, r, body, requestID, http.StatusBadRequest, "InvalidTrailNameException",
			"Name is required.", readOnly, eventID, verified)
		return
	}
	err := s.store.DeleteCloudTrailTrail(verified.AccountID, name)
	if errors.Is(err, store.ErrCloudTrailNotFound) {
		s.writeCloudTrailError(w, r, body, requestID, http.StatusBadRequest, "TrailNotFoundException",
			"Trail not found.", readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeCloudTrailError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to delete trail.", readOnly, eventID, verified)
		return
	}
	s.writeCloudTrailOK(w, requestID, []byte(`{}`))
	s.writeSuccessAudit(r, requestID, eventID, verified, cloudtrailEventSource, "DeleteTrail", readOnly)
}

func (s *Server) cloudtrailStartLogging(
	w http.ResponseWriter,
	r *http.Request,
	body []byte,
	requestID, eventID string,
	verified *authn.Verified,
	readOnly bool,
	params map[string]any,
) {
	if !s.authorize(verified, catalog.ActionCloudTrailStartLogging, "*") {
		s.writeCloudTrailError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform cloudtrail:StartLogging.", readOnly, eventID, verified)
		return
	}
	name, _ := params["Name"].(string)
	name = strings.TrimSpace(name)
	if name == "" {
		s.writeCloudTrailError(w, r, body, requestID, http.StatusBadRequest, "InvalidTrailNameException",
			"Name is required.", readOnly, eventID, verified)
		return
	}
	err := s.store.StartCloudTrailLogging(verified.AccountID, name, s.cfg.DataRoot)
	if errors.Is(err, store.ErrCloudTrailNotFound) {
		s.writeCloudTrailError(w, r, body, requestID, http.StatusBadRequest, "TrailNotFoundException",
			"Trail not found.", readOnly, eventID, verified)
		return
	}
	if errors.Is(err, store.ErrCloudTrailS3Bucket) || errors.Is(err, store.ErrCloudTrailBadRequest) {
		s.writeCloudTrailError(w, r, body, requestID, http.StatusBadRequest, "InvalidParameterException",
			err.Error(), readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeCloudTrailError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to start logging.", readOnly, eventID, verified)
		return
	}
	s.writeCloudTrailOK(w, requestID, []byte(`{}`))
	s.writeSuccessAudit(r, requestID, eventID, verified, cloudtrailEventSource, "StartLogging", readOnly)
}

func (s *Server) cloudtrailStopLogging(
	w http.ResponseWriter,
	r *http.Request,
	body []byte,
	requestID, eventID string,
	verified *authn.Verified,
	readOnly bool,
	params map[string]any,
) {
	if !s.authorize(verified, catalog.ActionCloudTrailStopLogging, "*") {
		s.writeCloudTrailError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform cloudtrail:StopLogging.", readOnly, eventID, verified)
		return
	}
	name, _ := params["Name"].(string)
	name = strings.TrimSpace(name)
	if name == "" {
		s.writeCloudTrailError(w, r, body, requestID, http.StatusBadRequest, "InvalidTrailNameException",
			"Name is required.", readOnly, eventID, verified)
		return
	}
	err := s.store.StopCloudTrailLogging(verified.AccountID, name)
	if errors.Is(err, store.ErrCloudTrailNotFound) {
		s.writeCloudTrailError(w, r, body, requestID, http.StatusBadRequest, "TrailNotFoundException",
			"Trail not found.", readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeCloudTrailError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to stop logging.", readOnly, eventID, verified)
		return
	}
	s.writeCloudTrailOK(w, requestID, []byte(`{}`))
	s.writeSuccessAudit(r, requestID, eventID, verified, cloudtrailEventSource, "StopLogging", readOnly)
}

const (
	cloudTrailInjectBatchCap   = 50
	cloudTrailInjectMaxDepth   = 8
	cloudTrailInjectMaxEntries = 64
	cloudTrailInjectMaxStrLen  = 4096
)

func (s *Server) cloudtrailInjectEvents(
	w http.ResponseWriter,
	r *http.Request,
	body []byte,
	requestID, eventID string,
	verified *authn.Verified,
	readOnly bool,
	params map[string]any,
) {
	if !s.cfg.CloudTrailInject {
		s.writeCloudTrailError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"CloudTrail lab inject is disabled. Set NOCTAXRIS_CLOUDTRAIL_INJECT=1 to enable.", readOnly, eventID, verified)
		return
	}
	if !s.authorize(verified, catalog.ActionCloudTrailInjectEvents, "*") {
		s.writeCloudTrailError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform cloudtrail:InjectEvents.", readOnly, eventID, verified)
		return
	}

	rawEvents, err := cloudtrailInjectEventList(params)
	if err != nil {
		s.writeCloudTrailError(w, r, body, requestID, http.StatusBadRequest, "InvalidParameterException",
			err.Error(), readOnly, eventID, verified)
		return
	}
	if len(rawEvents) == 0 {
		s.writeCloudTrailError(w, r, body, requestID, http.StatusBadRequest, "InvalidParameterException",
			"Events is required (single Event object or Events array).", readOnly, eventID, verified)
		return
	}
	if len(rawEvents) > cloudTrailInjectBatchCap {
		s.writeCloudTrailError(w, r, body, requestID, http.StatusBadRequest, "InvalidParameterException",
			fmt.Sprintf("Events batch exceeds cap of %d.", cloudTrailInjectBatchCap), readOnly, eventID, verified)
		return
	}

	writtenIDs := make([]string, 0, len(rawEvents))
	for i, raw := range rawEvents {
		ev, buildErr := buildInjectedAuditEvent(raw, verified, requestID, s.cloudtrailRegion(verified), s.now().UTC())
		if buildErr != nil {
			s.writeCloudTrailError(w, r, body, requestID, http.StatusBadRequest, "InvalidParameterException",
				fmt.Sprintf("Events[%d]: %s", i, buildErr.Error()), readOnly, eventID, verified)
			return
		}
		if err := s.audit.Write(r.Context(), ev); err != nil {
			s.writeCloudTrailError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
				"Unable to write injected event.", readOnly, eventID, verified)
			return
		}
		writtenIDs = append(writtenIDs, ev.EventID)
	}

	payload, _ := json.Marshal(map[string]any{
		"EventCount": len(writtenIDs),
		"EventIDs":   writtenIDs,
	})
	s.writeCloudTrailOK(w, requestID, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, cloudtrailEventSource, "InjectEvents", readOnly,
		WithAuditRequestParameters(map[string]any{"eventCount": len(writtenIDs)}))
}

func (s *Server) cloudtrailInjectInsightsEvents(
	w http.ResponseWriter,
	r *http.Request,
	body []byte,
	requestID, eventID string,
	verified *authn.Verified,
	readOnly bool,
	params map[string]any,
) {
	if !s.cfg.CloudTrailInject {
		s.writeCloudTrailError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"CloudTrail lab inject is disabled. Set NOCTAXRIS_CLOUDTRAIL_INJECT=1 to enable.", readOnly, eventID, verified)
		return
	}
	if !s.authorize(verified, catalog.ActionCloudTrailInjectInsightsEvents, "*") {
		s.writeCloudTrailError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform cloudtrail:InjectInsightsEvents.", readOnly, eventID, verified)
		return
	}

	rawEvents, err := cloudtrailInjectInsightsEventList(params)
	if err != nil {
		s.writeCloudTrailError(w, r, body, requestID, http.StatusBadRequest, "InvalidParameterException",
			err.Error(), readOnly, eventID, verified)
		return
	}
	if len(rawEvents) == 0 {
		s.writeCloudTrailError(w, r, body, requestID, http.StatusBadRequest, "InvalidParameterException",
			"Events is required (single Event object or Events array).", readOnly, eventID, verified)
		return
	}
	if len(rawEvents) > cloudTrailInjectBatchCap {
		s.writeCloudTrailError(w, r, body, requestID, http.StatusBadRequest, "InvalidParameterException",
			fmt.Sprintf("Events batch exceeds cap of %d.", cloudTrailInjectBatchCap), readOnly, eventID, verified)
		return
	}

	writtenIDs := make([]string, 0, len(rawEvents))
	for i, raw := range rawEvents {
		ev, buildErr := buildInjectedInsightAuditEvent(raw, verified, requestID, s.cloudtrailRegion(verified), s.now().UTC())
		if buildErr != nil {
			s.writeCloudTrailError(w, r, body, requestID, http.StatusBadRequest, "InvalidParameterException",
				fmt.Sprintf("Events[%d]: %s", i, buildErr.Error()), readOnly, eventID, verified)
			return
		}
		if err := s.audit.Write(r.Context(), ev); err != nil {
			s.writeCloudTrailError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
				"Unable to write injected insight event.", readOnly, eventID, verified)
			return
		}
		writtenIDs = append(writtenIDs, ev.EventID)
	}

	payload, _ := json.Marshal(map[string]any{
		"EventCount": len(writtenIDs),
		"EventIDs":   writtenIDs,
	})
	s.writeCloudTrailOK(w, requestID, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, cloudtrailEventSource, "InjectInsightsEvents", readOnly,
		WithAuditRequestParameters(map[string]any{"eventCount": len(writtenIDs)}))
}

func cloudtrailInjectEventList(params map[string]any) ([]map[string]any, error) {
	if raw, ok := params["Events"].([]any); ok {
		out := make([]map[string]any, 0, len(raw))
		for _, item := range raw {
			m, ok := item.(map[string]any)
			if !ok {
				return nil, errors.New("Events entries must be objects")
			}
			out = append(out, m)
		}
		return out, nil
	}
	if raw, ok := params["Event"].(map[string]any); ok {
		return []map[string]any{raw}, nil
	}
	if _, ok := params["eventName"].(string); ok {
		return []map[string]any{params}, nil
	}
	if _, ok := params["EventName"].(string); ok {
		return []map[string]any{params}, nil
	}
	return nil, nil
}

func buildInjectedAuditEvent(
	raw map[string]any,
	verified *authn.Verified,
	requestID, defaultRegion string,
	now time.Time,
) (audit.Event, error) {
	eventName := cloudtrailInjectString(raw, "eventName", "EventName")
	eventSource := cloudtrailInjectString(raw, "eventSource", "EventSource")
	if eventName == "" || eventSource == "" {
		return audit.Event{}, errors.New("eventName and eventSource are required")
	}

	eventTime := cloudtrailInjectString(raw, "eventTime", "EventTime")
	if eventTime == "" {
		eventTime = now.Format(time.RFC3339)
	} else if _, err := time.Parse(time.RFC3339, eventTime); err != nil {
		if _, err2 := time.Parse(time.RFC3339Nano, eventTime); err2 != nil {
			return audit.Event{}, errors.New("eventTime must be RFC3339")
		}
	}

	eventID := cloudtrailInjectString(raw, "eventID", "EventID", "eventId")
	if eventID == "" {
		eventID = newRequestID()
	}

	region := cloudtrailInjectString(raw, "awsRegion", "AWSRegion")
	if region == "" {
		region = defaultRegion
	}

	readOnly := false
	if v, ok := raw["readOnly"]; ok {
		switch t := v.(type) {
		case bool:
			readOnly = t
		case string:
			readOnly = strings.EqualFold(t, "true")
		}
	}

	uid := cloudtrailInjectUserIdentity(raw, verified)
	resources := cloudtrailInjectResources(raw)

	var reqParams map[string]any
	if v, ok := raw["requestParameters"].(map[string]any); ok {
		reqParams = redactCloudTrailInjectMap(v, cloudTrailInjectMaxDepth)
	}
	var respElems map[string]any
	if v, ok := raw["responseElements"].(map[string]any); ok {
		respElems = redactCloudTrailInjectMap(v, cloudTrailInjectMaxDepth)
	}

	var managementEvent *bool
	if v, ok := raw["managementEvent"]; ok {
		managementEvent = cloudtrailInjectBoolPtr(v)
	} else if v, ok := raw["ManagementEvent"]; ok {
		managementEvent = cloudtrailInjectBoolPtr(v)
	}

	return audit.Event{
		EventVersion:       eventVersion,
		UserIdentity:       uid,
		EventTime:          eventTime,
		EventSource:        eventSource,
		EventName:          eventName,
		AWSRegion:          region,
		SourceIPAddress:    cloudtrailInjectString(raw, "sourceIPAddress", "SourceIPAddress"),
		UserAgent:          cloudtrailInjectString(raw, "userAgent", "UserAgent"),
		RequestParameters:  reqParams,
		ResponseElements:   respElems,
		Resources:          resources,
		ErrorCode:          cloudtrailInjectString(raw, "errorCode", "ErrorCode"),
		ErrorMessage:       cloudtrailInjectString(raw, "errorMessage", "ErrorMessage"),
		RequestID:          requestID,
		EventID:            eventID,
		EventType:          "AwsApiCall",
		RecipientAccountID: verified.AccountID,
		ReadOnly:           readOnly,
		EventCategory:      cloudtrailInjectString(raw, "eventCategory", "EventCategory"),
		ManagementEvent:    managementEvent,
	}, nil
}

func cloudtrailInjectInsightsEventList(params map[string]any) ([]map[string]any, error) {
	if raw, ok := params["Events"].([]any); ok {
		out := make([]map[string]any, 0, len(raw))
		for _, item := range raw {
			m, ok := item.(map[string]any)
			if !ok {
				return nil, errors.New("Events entries must be objects")
			}
			out = append(out, m)
		}
		return out, nil
	}
	if raw, ok := params["Event"].(map[string]any); ok {
		return []map[string]any{raw}, nil
	}
	if _, ok := params["insightDetails"].(map[string]any); ok {
		return []map[string]any{params}, nil
	}
	if _, ok := params["InsightDetails"].(map[string]any); ok {
		return []map[string]any{params}, nil
	}
	return nil, nil
}

func buildInjectedInsightAuditEvent(
	raw map[string]any,
	verified *authn.Verified,
	requestID, defaultRegion string,
	now time.Time,
) (audit.Event, error) {
	details, _ := raw["insightDetails"].(map[string]any)
	if details == nil {
		details, _ = raw["InsightDetails"].(map[string]any)
	}
	if details == nil {
		return audit.Event{}, errors.New("insightDetails is required")
	}

	eventName := cloudtrailInjectString(raw, "eventName", "EventName")
	if eventName == "" {
		if v, _ := details["eventName"].(string); v != "" {
			eventName = v
		}
	}
	eventSource := cloudtrailInjectString(raw, "eventSource", "EventSource")
	if eventSource == "" {
		if v, _ := details["eventSource"].(string); v != "" {
			eventSource = v
		}
	}
	if eventName == "" || eventSource == "" {
		return audit.Event{}, errors.New("insightDetails.eventName and insightDetails.eventSource are required")
	}
	if _, ok := details["eventName"]; !ok {
		details["eventName"] = eventName
	}
	if _, ok := details["eventSource"]; !ok {
		details["eventSource"] = eventSource
	}
	if _, ok := details["insightType"]; !ok {
		if v := cloudtrailInjectString(raw, "insightType", "InsightType"); v != "" {
			details["insightType"] = v
		} else {
			details["insightType"] = "ApiCallRateInsight"
		}
	}
	if _, ok := details["state"]; !ok {
		details["state"] = "Start"
	}
	if _, ok := details["sourceEventCategory"]; !ok {
		details["sourceEventCategory"] = "Management"
	}

	eventTime := cloudtrailInjectString(raw, "eventTime", "EventTime")
	if eventTime == "" {
		eventTime = now.Format(time.RFC3339)
	} else if _, err := time.Parse(time.RFC3339, eventTime); err != nil {
		if _, err2 := time.Parse(time.RFC3339Nano, eventTime); err2 != nil {
			return audit.Event{}, errors.New("eventTime must be RFC3339")
		}
	}

	eventID := cloudtrailInjectString(raw, "eventID", "EventID", "eventId")
	if eventID == "" {
		eventID = newRequestID()
	}
	sharedID := cloudtrailInjectString(raw, "sharedEventID", "SharedEventID")
	if sharedID == "" {
		sharedID = newRequestID()
	}

	region := cloudtrailInjectString(raw, "awsRegion", "AWSRegion")
	if region == "" {
		region = defaultRegion
	}

	return audit.Event{
		EventVersion:       "1.09",
		EventTime:          eventTime,
		EventSource:        eventSource,
		EventName:          eventName,
		AWSRegion:          region,
		RequestID:          requestID,
		EventID:            eventID,
		EventType:          "AwsCloudTrailInsight",
		RecipientAccountID: verified.AccountID,
		EventCategory:      "Insight",
		SharedEventID:      sharedID,
		InsightDetails:     redactCloudTrailInjectMap(details, cloudTrailInjectMaxDepth),
	}, nil
}

func cloudtrailInjectSensitiveKey(key string) bool {
	k := strings.ToLower(strings.TrimSpace(key))
	switch k {
	case "secretstring", "secretbinary", "secretaccesskey",
		"password", "masteruserpassword", "temporarypassword",
		"sessiontoken", "idtoken", "accesstoken", "refreshtoken",
		"clientsecret", "privatekey", "credentials", "authorization":
		return true
	case "secretid", "secretarn", "secretname":
		return false
	}
	if strings.HasSuffix(k, "password") {
		return true
	}
	if strings.HasPrefix(k, "secret") {
		return true
	}
	if strings.HasSuffix(k, "token") {
		return true
	}
	return false
}

func redactCloudTrailInjectMap(m map[string]any, depth int) map[string]any {
	if m == nil {
		return nil
	}
	if depth <= 0 {
		return map[string]any{"_redacted": "depth"}
	}
	out := make(map[string]any)
	count := 0
	for k, v := range m {
		if count >= cloudTrailInjectMaxEntries {
			out["_truncated"] = true
			break
		}
		count++
		if cloudtrailInjectSensitiveKey(k) {
			out[k] = "[REDACTED]"
			continue
		}
		out[k] = redactCloudTrailInjectValue(v, depth-1)
	}
	return out
}

func redactCloudTrailInjectValue(v any, depth int) any {
	if depth <= 0 {
		return "[redacted:depth]"
	}
	switch t := v.(type) {
	case map[string]any:
		return redactCloudTrailInjectMap(t, depth)
	case []any:
		n := len(t)
		if n > cloudTrailInjectMaxEntries {
			n = cloudTrailInjectMaxEntries
		}
		out := make([]any, 0, n)
		for i := 0; i < n; i++ {
			out = append(out, redactCloudTrailInjectValue(t[i], depth-1))
		}
		return out
	case string:
		if len(t) > cloudTrailInjectMaxStrLen {
			return t[:cloudTrailInjectMaxStrLen] + "..."
		}
		return t
	default:
		return t
	}
}

func cloudtrailInjectString(raw map[string]any, keys ...string) string {
	for _, k := range keys {
		if v, ok := raw[k].(string); ok {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

func cloudtrailInjectUserIdentity(raw map[string]any, verified *authn.Verified) map[string]any {
	uid := map[string]any{}
	if v, ok := raw["userIdentity"].(map[string]any); ok {
		for k, val := range v {
			uid[k] = val
		}
	}
	if sc, ok := raw["sessionContext"].(map[string]any); ok && len(sc) > 0 {
		uid["sessionContext"] = sc
	}
	if _, ok := uid["accountId"]; !ok {
		uid["accountId"] = verified.AccountID
	}
	if _, ok := uid["type"]; !ok {
		uid["type"] = "IAMUser"
	}
	return uid
}

func cloudtrailInjectBoolPtr(v any) *bool {
	switch t := v.(type) {
	case bool:
		b := t
		return &b
	case string:
		b := strings.EqualFold(t, "true")
		return &b
	default:
		return nil
	}
}

func cloudtrailInjectResources(raw map[string]any) []audit.Resource {
	list, ok := raw["resources"].([]any)
	if !ok {
		return nil
	}
	out := make([]audit.Resource, 0, len(list))
	for _, item := range list {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		res := audit.Resource{
			AccountID: cloudtrailInjectString(m, "accountId", "AccountId"),
			Type:      cloudtrailInjectString(m, "type", "Type"),
			ARN:       cloudtrailInjectString(m, "ARN", "arn", "Arn"),
		}
		if res.ARN == "" && res.Type == "" {
			continue
		}
		out = append(out, res)
	}
	return out
}

func cloudtrailTrailResponse(accountID string, t store.CloudTrailTrail) map[string]any {
	out := map[string]any{
		"Name":         t.Name,
		"S3BucketName": t.S3BucketName,
		"TrailARN":     store.CloudTrailTrailARNFor(t.HomeRegion, accountID, t.Name, t.IsOrganizationTrail),
	}
	if t.S3KeyPrefix != "" {
		out["S3KeyPrefix"] = t.S3KeyPrefix
	}
	if t.CloudWatchLogsLogGroupArn != "" {
		out["CloudWatchLogsLogGroupArn"] = t.CloudWatchLogsLogGroupArn
	}
	if t.CloudWatchLogsRoleArn != "" {
		out["CloudWatchLogsRoleArn"] = t.CloudWatchLogsRoleArn
	}
	if t.IsOrganizationTrail {
		out["IsOrganizationTrail"] = true
	}
	return out
}

func cloudtrailTrailDescribeEntry(accountID string, t store.CloudTrailTrail) map[string]any {
	out := cloudtrailTrailResponse(accountID, t)
	out["HomeRegion"] = t.HomeRegion
	out["IsLogging"] = t.IsLogging
	return out
}

func (s *Server) cloudtrailLookupEvents(
	w http.ResponseWriter,
	r *http.Request,
	body []byte,
	requestID, eventID string,
	verified *authn.Verified,
	readOnly bool,
	params map[string]any,
) {
	if !s.authorize(verified, catalog.ActionCloudTrailLookupEvents, "*") {
		s.writeCloudTrailError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform cloudtrail:LookupEvents.", readOnly, eventID, verified)
		return
	}

	filter := store.CloudTrailLookupFilter{MaxResults: cloudtrailIntParam(params["MaxResults"], 50)}
	if v, ok := params["StartTime"]; ok {
		t, err := cloudtrailParseTime(v)
		if err != nil {
			s.writeCloudTrailError(w, r, body, requestID, http.StatusBadRequest, "InvalidParameterException",
				"StartTime is invalid.", readOnly, eventID, verified)
			return
		}
		filter.StartTime = &t
	}
	if v, ok := params["EndTime"]; ok {
		t, err := cloudtrailParseTime(v)
		if err != nil {
			s.writeCloudTrailError(w, r, body, requestID, http.StatusBadRequest, "InvalidParameterException",
				"EndTime is invalid.", readOnly, eventID, verified)
			return
		}
		filter.EndTime = &t
	}
	if filter.StartTime != nil && filter.EndTime != nil && filter.StartTime.After(*filter.EndTime) {
		s.writeCloudTrailError(w, r, body, requestID, http.StatusBadRequest, "InvalidTimeRangeException",
			"The specified time range is invalid.", readOnly, eventID, verified)
		return
	}

	if attrs, ok := params["LookupAttributes"].([]any); ok && len(attrs) > 0 {
		first, _ := attrs[0].(map[string]any)
		if first != nil {
			filter.AttributeKey, _ = first["AttributeKey"].(string)
			filter.AttributeValue, _ = first["AttributeValue"].(string)
		}
	}
	if v, ok := params["EventCategory"].(string); ok {
		filter.EventCategory = v
	} else if v, ok := params["eventCategory"].(string); ok {
		filter.EventCategory = v
	}

	filter.RecipientAccountID = verified.AccountID

	events, err := store.LookupCloudTrailEvents(s.cfg.DataRoot, filter)
	if err != nil {
		s.writeCloudTrailError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to look up events.", readOnly, eventID, verified)
		return
	}

	payload, err := cloudtrailsvc.LookupEventsJSON(events)
	if err != nil {
		s.writeCloudTrailError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to build response.", readOnly, eventID, verified)
		return
	}
	s.writeCloudTrailOK(w, requestID, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, cloudtrailEventSource, "LookupEvents", readOnly)
}

func (s *Server) cloudtrailPutEventSelectors(
	w http.ResponseWriter,
	r *http.Request,
	body []byte,
	requestID, eventID string,
	verified *authn.Verified,
	readOnly bool,
	params map[string]any,
) {
	if !s.authorize(verified, catalog.ActionCloudTrailPutEventSelectors, "*") {
		s.writeCloudTrailError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform cloudtrail:PutEventSelectors.", readOnly, eventID, verified)
		return
	}
	name, _ := params["TrailName"].(string)
	name = strings.TrimSpace(name)
	if name == "" {
		s.writeCloudTrailError(w, r, body, requestID, http.StatusBadRequest, "InvalidTrailNameException",
			"TrailName is required.", readOnly, eventID, verified)
		return
	}
	var selectors []any
	if raw, ok := params["EventSelectors"].([]any); ok {
		selectors = raw
	}
	sel := store.ParseCloudTrailEventSelectorsFromAPI(selectors)
	if err := s.store.PutCloudTrailEventSelectors(verified.AccountID, name, sel); err != nil {
		if errors.Is(err, store.ErrCloudTrailNotFound) {
			s.writeCloudTrailError(w, r, body, requestID, http.StatusBadRequest, "TrailNotFoundException",
				"Trail not found.", readOnly, eventID, verified)
			return
		}
		s.writeCloudTrailError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to update event selectors.", readOnly, eventID, verified)
		return
	}
	s.writeCloudTrailOK(w, requestID, []byte(`{}`))
	s.writeSuccessAudit(r, requestID, eventID, verified, cloudtrailEventSource, "PutEventSelectors", readOnly)
}

func (s *Server) cloudtrailGetEventSelectors(
	w http.ResponseWriter,
	r *http.Request,
	body []byte,
	requestID, eventID string,
	verified *authn.Verified,
	readOnly bool,
	params map[string]any,
) {
	if !s.authorize(verified, catalog.ActionCloudTrailGetEventSelectors, "*") {
		s.writeCloudTrailError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform cloudtrail:GetEventSelectors.", readOnly, eventID, verified)
		return
	}
	name, _ := params["TrailName"].(string)
	name = strings.TrimSpace(name)
	if name == "" {
		s.writeCloudTrailError(w, r, body, requestID, http.StatusBadRequest, "InvalidTrailNameException",
			"TrailName is required.", readOnly, eventID, verified)
		return
	}
	sel, err := s.store.GetCloudTrailEventSelectors(verified.AccountID, name)
	if errors.Is(err, store.ErrCloudTrailNotFound) {
		s.writeCloudTrailError(w, r, body, requestID, http.StatusBadRequest, "TrailNotFoundException",
			"Trail not found.", readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeCloudTrailError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to get event selectors.", readOnly, eventID, verified)
		return
	}
	payload, err := cloudtrailsvc.EventSelectorsJSON(sel)
	if err != nil {
		s.writeCloudTrailError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to build response.", readOnly, eventID, verified)
		return
	}
	s.writeCloudTrailOK(w, requestID, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, cloudtrailEventSource, "GetEventSelectors", readOnly)
}

func (s *Server) cloudtrailValidateLogs(
	w http.ResponseWriter,
	r *http.Request,
	body []byte,
	requestID, eventID string,
	verified *authn.Verified,
	readOnly bool,
	params map[string]any,
) {
	if !s.authorize(verified, catalog.ActionCloudTrailValidateLogs, "*") {
		s.writeCloudTrailError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform cloudtrail:ValidateLogs.", readOnly, eventID, verified)
		return
	}
	bucket, _ := params["S3BucketName"].(string)
	key, _ := params["S3ObjectKey"].(string)
	if key == "" {
		key, _ = params["S3Key"].(string)
	}
	bucket = strings.TrimSpace(bucket)
	key = strings.TrimSpace(key)
	if bucket == "" || key == "" {
		s.writeCloudTrailError(w, r, body, requestID, http.StatusBadRequest, "InvalidParameterException",
			"S3BucketName and S3ObjectKey are required.", readOnly, eventID, verified)
		return
	}
	err := s.store.ValidateCloudTrailLogFile(verified.AccountID, bucket, key)
	if errors.Is(err, store.ErrCloudTrailDigestMismatch) {
		s.writeCloudTrailError(w, r, body, requestID, http.StatusBadRequest, "DigestValidationException",
			"Log file digest does not match the digest sidecar.", readOnly, eventID, verified)
		return
	}
	if errors.Is(err, store.ErrNoSuchKey) || errors.Is(err, store.ErrNoSuchBucket) {
		s.writeCloudTrailError(w, r, body, requestID, http.StatusBadRequest, "S3ObjectNotFoundException",
			"S3 object not found.", readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeCloudTrailError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to validate log file.", readOnly, eventID, verified)
		return
	}
	payload, _ := json.Marshal(map[string]any{"Valid": true})
	s.writeCloudTrailOK(w, requestID, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, cloudtrailEventSource, "ValidateLogs", readOnly)
}

func cloudtrailParseTime(v any) (time.Time, error) {
	switch t := v.(type) {
	case string:
		if ts, err := time.Parse(time.RFC3339, t); err == nil {
			return ts, nil
		}
		return time.Parse(time.RFC3339Nano, t)
	case float64:
		return time.Unix(int64(t), 0).UTC(), nil
	default:
		return time.Time{}, errCloudTrailBadTime
	}
}

func cloudtrailIntParam(v any, def int) int {
	switch n := v.(type) {
	case float64:
		return int(n)
	case int:
		return n
	default:
		return def
	}
}

func cloudtrailBoolParam(v any) bool {
	switch b := v.(type) {
	case bool:
		return b
	case string:
		return strings.EqualFold(strings.TrimSpace(b), "true")
	default:
		return false
	}
}

func (s *Server) writeCloudTrailOK(w http.ResponseWriter, requestID string, payload []byte) {
	w.Header().Set(requestIDHeader, requestID)
	w.Header().Set("Content-Type", cloudtrailJSONContentType)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(payload)
}

func (s *Server) writeCloudTrailError(
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
	w.Header().Set("Content-Type", cloudtrailJSONContentType)
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
