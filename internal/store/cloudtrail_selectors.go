package store

import (
	"encoding/json"
	"strings"
)

// CloudTrailEventSelectors are lab-lite trail event selector flags (PutEventSelectors).
type CloudTrailEventSelectors struct {
	IncludeManagementEvents bool
	S3DataEventsEnabled     bool
}

// PutCloudTrailEventSelectors stores selector flags on a trail (replaces prior selectors).
func (s *Store) PutCloudTrailEventSelectors(accountID, trailName string, sel CloudTrailEventSelectors) error {
	if err := s.EnsureCloudTrailSchema(); err != nil {
		return err
	}
	trailName = strings.TrimSpace(trailName)
	if _, err := s.GetCloudTrailTrail(accountID, trailName); err != nil {
		return err
	}
	inc := 0
	if sel.IncludeManagementEvents {
		inc = 1
	}
	s3 := 0
	if sel.S3DataEventsEnabled {
		s3 = 1
	}
	_, err := s.db.Exec(
		`UPDATE cloudtrail_trails SET include_management_events = ?, s3_data_events_enabled = ?
		 WHERE account_id = ? AND name = ?`,
		inc, s3, accountID, trailName,
	)
	if err != nil {
		return err
	}
	return nil
}

// GetCloudTrailEventSelectors returns selector flags for a trail.
func (s *Store) GetCloudTrailEventSelectors(accountID, trailName string) (CloudTrailEventSelectors, error) {
	t, err := s.GetCloudTrailTrail(accountID, trailName)
	if err != nil {
		return CloudTrailEventSelectors{}, err
	}
	return CloudTrailEventSelectors{
		IncludeManagementEvents: t.IncludeManagementEvents,
		S3DataEventsEnabled:     t.S3DataEventsEnabled,
	}, nil
}

// ParseCloudTrailEventSelectorsFromAPI maps PutEventSelectors EventSelectors[] to lab flags.
func ParseCloudTrailEventSelectorsFromAPI(eventSelectors []any) CloudTrailEventSelectors {
	out := CloudTrailEventSelectors{IncludeManagementEvents: true}
	for _, item := range eventSelectors {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		if v, ok := m["IncludeManagementEvents"]; ok {
			out.IncludeManagementEvents = cloudTrailBoolish(v, true)
		}
		if resources, ok := m["DataResources"].([]any); ok {
			for _, dr := range resources {
				dm, ok := dr.(map[string]any)
				if !ok {
					continue
				}
				typ, _ := dm["Type"].(string)
				if strings.EqualFold(typ, "AWS::S3::Object") {
					out.S3DataEventsEnabled = true
				}
			}
		}
	}
	return out
}

func cloudTrailBoolish(v any, def bool) bool {
	switch t := v.(type) {
	case bool:
		return t
	case string:
		return strings.EqualFold(t, "true")
	default:
		return def
	}
}

func cloudTrailLinePassesSelectors(line string, sel CloudTrailEventSelectors) bool {
	line = strings.TrimSpace(line)
	if line == "" {
		return false
	}
	var raw map[string]any
	if err := json.Unmarshal([]byte(line), &raw); err != nil {
		return sel.IncludeManagementEvents
	}

	eventSource, _ := raw["eventSource"].(string)
	eventName, _ := raw["eventName"].(string)
	category, _ := raw["eventCategory"].(string)

	isData := strings.EqualFold(category, "Data")
	if v := raw["managementEvent"]; v != nil {
		switch t := v.(type) {
		case bool:
			isData = !t
		case string:
			isData = strings.EqualFold(t, "false")
		}
	}

	isS3Data := strings.EqualFold(eventSource, "s3.amazonaws.com") && cloudTrailS3DataEventName(eventName)
	if isS3Data {
		return sel.S3DataEventsEnabled
	}
	if isData {
		return false
	}
	return sel.IncludeManagementEvents
}

func cloudTrailS3DataEventName(eventName string) bool {
	switch eventName {
	case "GetObject", "PutObject", "DeleteObject", "CopyObject", "CompleteMultipartUpload",
		"UploadPart", "CreateMultipartUpload", "AbortMultipartUpload":
		return true
	default:
		return false
	}
}
