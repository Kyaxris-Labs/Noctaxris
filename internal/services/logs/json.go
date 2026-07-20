package logs

import (
	"encoding/json"

	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

// EmptyOKJSON is the empty success body used by CreateLogGroup / CreateLogStream.
func EmptyOKJSON() ([]byte, error) {
	return []byte(`{}`), nil
}

// DescribeLogGroupsJSON builds a DescribeLogGroups response.
func DescribeLogGroupsJSON(groups []store.LogGroup) ([]byte, error) {
	entries := make([]map[string]any, 0, len(groups))
	for _, g := range groups {
		entries = append(entries, map[string]any{
			"logGroupName":      g.LogGroupName,
			"arn":               g.Arn,
			"creationTime":      g.CreationTime,
			"storedBytes":       g.StoredBytes,
			"metricFilterCount": g.MetricFilterCount,
		})
	}
	return json.Marshal(map[string]any{"logGroups": entries})
}

// PutLogEventsJSON builds a PutLogEvents response.
func PutLogEventsJSON(nextSequenceToken string) ([]byte, error) {
	return json.Marshal(map[string]any{
		"nextSequenceToken": nextSequenceToken,
	})
}

// GetLogEventsJSON builds a GetLogEvents response.
func GetLogEventsJSON(events []store.LogEvent) ([]byte, error) {
	entries := make([]map[string]any, 0, len(events))
	for _, ev := range events {
		entries = append(entries, map[string]any{
			"timestamp":     ev.Timestamp,
			"message":       ev.Message,
			"ingestionTime": ev.IngestionTime,
			"eventId":       ev.EventID,
		})
	}
	return json.Marshal(map[string]any{
		"events":            entries,
		"nextForwardToken":  "",
		"nextBackwardToken": "",
	})
}
