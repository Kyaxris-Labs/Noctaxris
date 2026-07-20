package server

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/Kyaxris-Labs/Noctaxris/internal/catalog"
	"github.com/Kyaxris-Labs/Noctaxris/internal/kernel/authn"
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
	default:
		return action
	}
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
