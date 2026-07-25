package cloudtrail

import (
	"encoding/json"
	"time"

	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

// EventSelectorsJSON builds GetEventSelectors response body.
func EventSelectorsJSON(sel store.CloudTrailEventSelectors) ([]byte, error) {
	selector := map[string]any{
		"ReadWriteType":           "All",
		"IncludeManagementEvents": sel.IncludeManagementEvents,
	}
	if sel.S3DataEventsEnabled {
		selector["DataResources"] = []map[string]any{
			{
				"Type":   "AWS::S3::Object",
				"Values": []string{"arn:aws:s3:::"},
			},
		}
	}
	return json.Marshal(map[string]any{
		"EventSelectors":       []map[string]any{selector},
		"AdvancedEventSelectors": []any{},
	})
}

func LookupEventsJSON(events []store.CloudTrailEventRecord) ([]byte, error) {
	out := make([]map[string]any, 0, len(events))
	for _, ev := range events {
		entry := map[string]any{
			"EventId":         ev.EventId,
			"EventName":       ev.EventName,
			"EventSource":     ev.EventSource,
			"CloudTrailEvent": ev.CloudTrailEvent,
		}
		if !ev.EventTime.IsZero() {
			entry["EventTime"] = ev.EventTime.UTC().Format(time.RFC3339)
		}
		if ev.Username != "" {
			entry["Username"] = ev.Username
		}
		out = append(out, entry)
	}
	return json.Marshal(map[string]any{"Events": out})
}
