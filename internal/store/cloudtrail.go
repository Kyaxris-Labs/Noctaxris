package store

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

const cloudtrailEventsFile = "events.jsonl"

// CloudTrailLookupFilter selects events from the local JSONL audit file.
type CloudTrailLookupFilter struct {
	StartTime *time.Time
	EndTime   *time.Time
	// AttributeKey/Value are the single LookupAttributes entry (AWS allows one).
	AttributeKey   string
	AttributeValue string
	MaxResults     int
}

// CloudTrailEventRecord is one LookupEvents Events[] member.
type CloudTrailEventRecord struct {
	EventId         string
	EventName       string
	EventSource     string
	EventTime       time.Time
	Username        string
	CloudTrailEvent string // raw JSON line
}

// LookupCloudTrailEvents reads dataRoot/cloudtrail/events.jsonl and returns matching events
// newest-first (AWS LookupEvents order). Missing file yields an empty slice.
func LookupCloudTrailEvents(dataRoot string, filter CloudTrailLookupFilter) ([]CloudTrailEventRecord, error) {
	path := filepath.Join(dataRoot, "cloudtrail", cloudtrailEventsFile)
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("lookup cloudtrail events: open: %w", err)
	}
	defer f.Close()

	max := filter.MaxResults
	if max <= 0 {
		max = 50
	}
	if max > 50 {
		max = 50
	}

	var matched []CloudTrailEventRecord
	sc := bufio.NewScanner(f)
	// Audit lines can be moderately large; raise buffer beyond default 64KiB.
	buf := make([]byte, 0, 64*1024)
	sc.Buffer(buf, 1024*1024)

	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		rec, ok, err := matchCloudTrailLine(line, filter)
		if err != nil {
			return nil, err
		}
		if ok {
			matched = append(matched, rec)
		}
	}
	if err := sc.Err(); err != nil {
		return nil, fmt.Errorf("lookup cloudtrail events: scan: %w", err)
	}

	sort.SliceStable(matched, func(i, j int) bool {
		return matched[i].EventTime.After(matched[j].EventTime)
	})
	if len(matched) > max {
		matched = matched[:max]
	}
	return matched, nil
}

func matchCloudTrailLine(line string, filter CloudTrailLookupFilter) (CloudTrailEventRecord, bool, error) {
	var raw map[string]any
	if err := json.Unmarshal([]byte(line), &raw); err != nil {
		return CloudTrailEventRecord{}, false, fmt.Errorf("lookup cloudtrail events: unmarshal: %w", err)
	}

	eventTimeStr, _ := raw["eventTime"].(string)
	eventTime, err := time.Parse(time.RFC3339, eventTimeStr)
	if err != nil {
		// Soft-skip malformed timestamps rather than failing the whole lookup.
		eventTime = time.Time{}
	}
	if filter.StartTime != nil && !eventTime.IsZero() && eventTime.Before(*filter.StartTime) {
		return CloudTrailEventRecord{}, false, nil
	}
	if filter.EndTime != nil && !eventTime.IsZero() && eventTime.After(*filter.EndTime) {
		return CloudTrailEventRecord{}, false, nil
	}

	eventName, _ := raw["eventName"].(string)
	eventSource, _ := raw["eventSource"].(string)
	eventID, _ := raw["eventID"].(string)
	username := cloudTrailUsername(raw)
	accessKeyID := cloudTrailAccessKeyID(raw)
	readOnly := cloudTrailReadOnlyString(raw)

	if key := strings.TrimSpace(filter.AttributeKey); key != "" {
		want := filter.AttributeValue
		var got string
		switch strings.ToLower(key) {
		case "eventname":
			got = eventName
		case "username":
			got = username
		case "eventid":
			got = eventID
		case "eventsource":
			got = eventSource
		case "accesskeyid":
			got = accessKeyID
		case "readonly":
			got = readOnly
		default:
			return CloudTrailEventRecord{}, false, nil
		}
		if !strings.EqualFold(got, want) {
			return CloudTrailEventRecord{}, false, nil
		}
	}

	return CloudTrailEventRecord{
		EventId:         eventID,
		EventName:       eventName,
		EventSource:     eventSource,
		EventTime:       eventTime,
		Username:        username,
		CloudTrailEvent: line,
	}, true, nil
}

func cloudTrailUsername(raw map[string]any) string {
	ui, _ := raw["userIdentity"].(map[string]any)
	if ui == nil {
		return ""
	}
	if v, _ := ui["userName"].(string); v != "" {
		return v
	}
	if v, _ := ui["principalId"].(string); v != "" {
		return v
	}
	if arn, _ := ui["arn"].(string); arn != "" {
		if i := strings.LastIndex(arn, "/"); i >= 0 && i+1 < len(arn) {
			return arn[i+1:]
		}
	}
	if v, _ := ui["accessKeyId"].(string); v != "" {
		return v
	}
	return ""
}

func cloudTrailAccessKeyID(raw map[string]any) string {
	ui, _ := raw["userIdentity"].(map[string]any)
	if ui == nil {
		return ""
	}
	v, _ := ui["accessKeyId"].(string)
	return v
}

func cloudTrailReadOnlyString(raw map[string]any) string {
	switch v := raw["readOnly"].(type) {
	case bool:
		return strconv.FormatBool(v)
	case string:
		return v
	default:
		return ""
	}
}
