package cloudtrail

import (
	"encoding/json"
	"time"

	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

// LookupEventsJSON builds a LookupEvents response body.
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
