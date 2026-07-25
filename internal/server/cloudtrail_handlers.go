package server

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/Kyaxris-Labs/Noctaxris/internal/catalog"
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
	region := s.cloudtrailRegion(verified)
	trailARN := store.CloudTrailTrailARN(region, verified.AccountID, name)
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

func cloudtrailTrailResponse(accountID string, t store.CloudTrailTrail) map[string]any {
	out := map[string]any{
		"Name":         t.Name,
		"S3BucketName": t.S3BucketName,
		"TrailARN":     store.CloudTrailTrailARN(t.HomeRegion, accountID, t.Name),
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

func cloudtrailParseTime(v any) (time.Time, error) {
	switch t := v.(type) {
	case string:
		if ts, err := time.Parse(time.RFC3339, t); err == nil {
			return ts, nil
		}
		return time.Parse(time.RFC3339Nano, t)
	case float64:
		// Epoch seconds (AWS JSON sometimes sends numbers).
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

