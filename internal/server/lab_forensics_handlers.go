package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/Kyaxris-Labs/Noctaxris/internal/catalog"
	"github.com/Kyaxris-Labs/Noctaxris/internal/kernel/authn"
	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

const labForensicsEventSource = "noctaxris.amazonaws.com"

func (s *Server) effectiveNow() time.Time {
	s.clockMu.RLock()
	defer s.clockMu.RUnlock()
	return s.labClockNowLocked()
}

// authClock is wall time for SigV4 skew checks (lab clock override must not affect auth).
func (s *Server) authClock() time.Time {
	return time.Now().UTC()
}

func (s *Server) labClockNowLocked() time.Time {
	if s.clockOverride != nil {
		return s.clockOverride.UTC()
	}
	return time.Now().UTC()
}

func (s *Server) freezeLabClock() time.Time {
	s.clockMu.Lock()
	defer s.clockMu.Unlock()
	t := s.labClockNowLocked()
	s.clockOverride = &t
	return t
}

func (s *Server) setLabClock(t time.Time) time.Time {
	utc := t.UTC()
	s.clockMu.Lock()
	defer s.clockMu.Unlock()
	s.clockOverride = &utc
	return utc
}

func (s *Server) unfreezeLabClock() {
	s.clockMu.Lock()
	defer s.clockMu.Unlock()
	s.clockOverride = nil
}

func (s *Server) handleLabForensics(
	w http.ResponseWriter,
	r *http.Request,
	body []byte,
	requestID, eventID, action string,
	verified *authn.Verified,
	readOnly bool,
) {
	params := jsonBodyMap(body)
	action = labForensicsAction(action)

	switch action {
	case catalog.ActionLabFreezeClock:
		s.labFreezeClock(w, r, body, requestID, eventID, verified, readOnly)
	case catalog.ActionLabUnfreezeClock:
		s.labUnfreezeClock(w, r, body, requestID, eventID, verified, readOnly)
	case catalog.ActionLabSetClock:
		s.labSetClock(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionLabBulkSeed:
		s.labBulkSeed(w, r, body, requestID, eventID, verified, readOnly, params)
	default:
		s.writeLabForensicsError(w, r, body, requestID, http.StatusNotImplemented, "InvalidAction",
			"This NoctaxrisLab action is not implemented.", readOnly, eventID, verified)
	}
}

func labForensicsAction(action string) string {
	if strings.Contains(action, ":") {
		return action
	}
	switch action {
	case "FreezeClock":
		return catalog.ActionLabFreezeClock
	case "UnfreezeClock":
		return catalog.ActionLabUnfreezeClock
	case "SetClock":
		return catalog.ActionLabSetClock
	case "BulkSeed":
		return catalog.ActionLabBulkSeed
	default:
		return action
	}
}

func (s *Server) labForensicsEnabled(w http.ResponseWriter, r *http.Request, body []byte, requestID string, readOnly bool, eventID string, verified *authn.Verified) bool {
	if s.cfg.LabForensics {
		return true
	}
	s.writeLabForensicsError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
		"NoctaxrisLab forensics is disabled. Set NOCTAXRIS_LAB_FORENSICS=1 to enable.", readOnly, eventID, verified)
	return false
}

func (s *Server) labFreezeClock(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool,
) {
	if !s.labForensicsEnabled(w, r, body, requestID, readOnly, eventID, verified) {
		return
	}
	if !s.authorize(verified, catalog.ActionLabFreezeClock, "*") {
		s.writeLabForensicsError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform noctaxris-lab:FreezeClock.", readOnly, eventID, verified)
		return
	}
	t := s.freezeLabClock()
	payload, _ := json.Marshal(map[string]any{
		"ClockTime": t.Format(time.RFC3339),
		"Frozen":    true,
	})
	s.writeLabForensicsOK(w, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, labForensicsEventSource, "FreezeClock", readOnly)
}

func (s *Server) labUnfreezeClock(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool,
) {
	if !s.labForensicsEnabled(w, r, body, requestID, readOnly, eventID, verified) {
		return
	}
	if !s.authorize(verified, catalog.ActionLabUnfreezeClock, "*") {
		s.writeLabForensicsError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform noctaxris-lab:UnfreezeClock.", readOnly, eventID, verified)
		return
	}
	s.unfreezeLabClock()
	s.writeLabForensicsOK(w, []byte(`{}`))
	s.writeSuccessAudit(r, requestID, eventID, verified, labForensicsEventSource, "UnfreezeClock", readOnly)
}

func (s *Server) labSetClock(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	if !s.labForensicsEnabled(w, r, body, requestID, readOnly, eventID, verified) {
		return
	}
	if !s.authorize(verified, catalog.ActionLabSetClock, "*") {
		s.writeLabForensicsError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform noctaxris-lab:SetClock.", readOnly, eventID, verified)
		return
	}
	raw := labForensicsString(params, "FixedTime", "fixedTime", "ClockTime", "clockTime")
	if raw == "" {
		s.writeLabForensicsError(w, r, body, requestID, http.StatusBadRequest, "InvalidParameterException",
			"FixedTime is required (RFC3339).", readOnly, eventID, verified)
		return
	}
	t, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		if t, err = time.Parse(time.RFC3339Nano, raw); err != nil {
			s.writeLabForensicsError(w, r, body, requestID, http.StatusBadRequest, "InvalidParameterException",
				"FixedTime must be RFC3339.", readOnly, eventID, verified)
			return
		}
	}
	set := s.setLabClock(t)
	payload, _ := json.Marshal(map[string]any{
		"ClockTime": set.Format(time.RFC3339),
	})
	s.writeLabForensicsOK(w, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, labForensicsEventSource, "SetClock", readOnly,
		WithAuditRequestParameters(map[string]any{"fixedTime": set.Format(time.RFC3339)}))
}

func (s *Server) labBulkSeed(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	if !s.labForensicsEnabled(w, r, body, requestID, readOnly, eventID, verified) {
		return
	}
	if !s.authorize(verified, catalog.ActionLabBulkSeed, "*") {
		s.writeLabForensicsError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform noctaxris-lab:BulkSeed.", readOnly, eventID, verified)
		return
	}
	if s.audit == nil {
		s.writeLabForensicsError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Audit writer is not configured.", readOnly, eventID, verified)
		return
	}

	scenarioID := labForensicsString(params, "ScenarioId", "scenarioId")
	if scenarioID == "" {
		s.writeLabForensicsError(w, r, body, requestID, http.StatusBadRequest, "InvalidParameterException",
			"ScenarioId is required.", readOnly, eventID, verified)
		return
	}

	region := labForensicsString(params, "Region", "region")
	if region == "" {
		region = verified.Region
	}
	if region == "" {
		region = store.DefaultGuardDutyRegion
	}

	includeGD := true
	if v, ok := params["IncludeGuardDuty"]; ok {
		includeGD = labForensicsBool(v)
	} else if v, ok := params["includeGuardDuty"]; ok {
		includeGD = labForensicsBool(v)
	}

	base := s.now().UTC()
	rawEvents, gdFindings, err := labScenarioPack(scenarioID, verified.AccountID, region, base)
	if err != nil {
		s.writeLabForensicsError(w, r, body, requestID, http.StatusBadRequest, "InvalidParameterException",
			err.Error(), readOnly, eventID, verified)
		return
	}

	eventIDs := make([]string, 0, len(rawEvents))
	for i, raw := range rawEvents {
		ev, buildErr := buildInjectedAuditEvent(raw, verified, requestID, region, base)
		if buildErr != nil {
			s.writeLabForensicsError(w, r, body, requestID, http.StatusBadRequest, "InvalidParameterException",
				fmt.Sprintf("scenario event[%d]: %s", i, buildErr.Error()), readOnly, eventID, verified)
			return
		}
		if err := s.audit.Write(r.Context(), ev); err != nil {
			s.writeLabForensicsError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
				"Unable to write seeded CloudTrail event.", readOnly, eventID, verified)
			return
		}
		eventIDs = append(eventIDs, ev.EventID)
	}

	var findingIDs []string
	if includeGD && len(gdFindings) > 0 {
		detectorID, detErr := s.store.EnsureGuardDutyDetector(verified.AccountID)
		if detErr != nil {
			s.writeLabForensicsError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
				"Unable to ensure GuardDuty detector.", readOnly, eventID, verified)
			return
		}
		findingIDs, err = s.store.InjectGuardDutyFindings(verified.AccountID, detectorID, region, gdFindings)
		if err != nil {
			s.writeLabForensicsError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
				"Unable to inject GuardDuty findings.", readOnly, eventID, verified)
			return
		}
	}

	payload, _ := json.Marshal(map[string]any{
		"ScenarioId":            scenarioID,
		"CloudTrailEventCount":  len(eventIDs),
		"CloudTrailEventIDs":    eventIDs,
		"GuardDutyFindingCount": len(findingIDs),
		"GuardDutyFindingIds":   findingIDs,
	})
	s.writeLabForensicsOK(w, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, labForensicsEventSource, "BulkSeed", readOnly,
		WithAuditRequestParameters(map[string]any{
			"scenarioId":           scenarioID,
			"cloudTrailEventCount": len(eventIDs),
			"guardDutyFindingCount": len(findingIDs),
		}))
}

var errUnknownLabScenario = errors.New("unknown ScenarioId")

func labScenarioPack(scenarioID, accountID, region string, base time.Time) ([]map[string]any, []store.GuardDutyFinding, error) {
	switch strings.ToLower(strings.TrimSpace(scenarioID)) {
	case "suspicious-login":
		return labScenarioSuspiciousLogin(accountID, region, base), nil, nil
	case "s3-data-exfil":
		return labScenarioS3DataExfil(accountID, region, base)
	case "crypto-mining":
		return labScenarioCryptoMining(accountID, region, base)
	default:
		return nil, nil, fmt.Errorf("%w: %q (known: suspicious-login, s3-data-exfil, crypto-mining)", errUnknownLabScenario, scenarioID)
	}
}

func labScenarioSuspiciousLogin(accountID, region string, base time.Time) []map[string]any {
	t := base.Add(-2 * time.Hour).Format(time.RFC3339)
	return []map[string]any{
		{
			"eventID":         "seed-login-1",
			"eventName":       "ConsoleLogin",
			"eventSource":     "signin.amazonaws.com",
			"eventTime":       t,
			"awsRegion":       region,
			"sourceIPAddress": "203.0.113.44",
			"readOnly":        false,
			"userIdentity": map[string]any{
				"type":      "IAMUser",
				"accountId": accountID,
				"userName":  "contractor",
			},
			"responseElements": map[string]any{
				"ConsoleLogin": "Success",
			},
		},
	}
}

func labScenarioS3DataExfil(accountID, region string, base time.Time) ([]map[string]any, []store.GuardDutyFinding, error) {
	t1 := base.Add(-45 * time.Minute).Format(time.RFC3339)
	t2 := base.Add(-30 * time.Minute).Format(time.RFC3339)
	bucket := "corp-sensitive-" + accountID
	events := []map[string]any{
		{
			"eventID":         "seed-s3-list-1",
			"eventName":       "ListBuckets",
			"eventSource":     "s3.amazonaws.com",
			"eventTime":       t1,
			"awsRegion":       region,
			"sourceIPAddress": "198.51.100.8",
			"readOnly":        true,
			"userIdentity": map[string]any{
				"type":      "AssumedRole",
				"accountId": accountID,
				"arn":       fmt.Sprintf("arn:aws:sts::%s:assumed-role/Investigator/session", accountID),
			},
		},
		{
			"eventID":         "seed-s3-get-1",
			"eventName":       "GetObject",
			"eventSource":     "s3.amazonaws.com",
			"eventTime":       t2,
			"awsRegion":       region,
			"sourceIPAddress": "198.51.100.8",
			"readOnly":        true,
			"userIdentity": map[string]any{
				"type":      "AssumedRole",
				"accountId": accountID,
				"arn":       fmt.Sprintf("arn:aws:sts::%s:assumed-role/Investigator/session", accountID),
			},
			"requestParameters": map[string]any{
				"bucketName": bucket,
				"key":        "finance/q1-report.csv",
			},
			"resources": []map[string]any{
				{
					"ARN":              fmt.Sprintf("arn:aws:s3:::%s/finance/q1-report.csv", bucket),
					"type":             "AWS::S3::Object",
					"accountId":        accountID,
					"ARNPrefix":        fmt.Sprintf("arn:aws:s3:::%s/", bucket),
				},
			},
		},
	}
	gd := []store.GuardDutyFinding{
		{
			Id:     "seed-gd-exfil-1",
			Type:   "Exfiltration:S3/ObjectRead.Unusual",
			Title:  "Unusual S3 object read volume",
			Region: region,
			Severity: 6.5,
			Resource: map[string]any{
				"resourceType": "S3Bucket",
				"s3BucketDetails": []map[string]any{
					{"arn": fmt.Sprintf("arn:aws:s3:::%s", bucket)},
				},
			},
		},
	}
	return events, gd, nil
}

func labScenarioCryptoMining(accountID, region string, base time.Time) ([]map[string]any, []store.GuardDutyFinding, error) {
	t := base.Add(-15 * time.Minute).Format(time.RFC3339)
	events := []map[string]any{
		{
			"eventID":         "seed-ec2-run-1",
			"eventName":       "RunInstances",
			"eventSource":     "ec2.amazonaws.com",
			"eventTime":       t,
			"awsRegion":       region,
			"sourceIPAddress": "203.0.113.90",
			"readOnly":        false,
			"userIdentity": map[string]any{
				"type":      "IAMUser",
				"accountId": accountID,
				"userName":  "devops",
			},
			"requestParameters": map[string]any{
				"instanceType": "c5.4xlarge",
				"minCount":     20,
				"maxCount":     20,
			},
		},
	}
	gd := []store.GuardDutyFinding{
		{
			Id:       "seed-gd-crypto-1",
			Type:     "CryptoCurrency:EC2/BitcoinTool.B!DNS",
			Title:    "EC2 instance communicating with Bitcoin-related domain",
			Region:   region,
			Severity: 8.0,
			Resource: map[string]any{
				"resourceType": "Instance",
				"instanceDetails": map[string]any{
					"instanceId": "i-0seedcrypto01",
				},
			},
		},
	}
	return events, gd, nil
}

func labForensicsString(params map[string]any, keys ...string) string {
	for _, k := range keys {
		if v, ok := params[k].(string); ok && strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

func labForensicsBool(v any) bool {
	switch t := v.(type) {
	case bool:
		return t
	case string:
		return strings.EqualFold(t, "true") || t == "1"
	default:
		return false
	}
}

func (s *Server) writeLabForensicsOK(w http.ResponseWriter, payload []byte) {
	w.Header().Set("Content-Type", "application/x-amz-json-1.1")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(payload)
}

func (s *Server) writeLabForensicsError(
	w http.ResponseWriter, r *http.Request, body []byte, requestID string,
	status int, code, message string, readOnly bool, eventID string, verified *authn.Verified,
) {
	ak, acct := "", ""
	known := false
	if verified != nil {
		ak, acct, known = verified.AccessKeyID, verified.AccountID, true
	}
	s.writeAPIError(w, r, body, requestID, status, code, message, readOnly, eventID, ak, acct, known)
}
