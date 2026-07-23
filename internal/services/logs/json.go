package logs

import (
	"encoding/json"

	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

// EmptyOKJSON is the empty success body used by Create/Delete LogGroup and LogStream.
func EmptyOKJSON() ([]byte, error) {
	return []byte(`{}`), nil
}

// DescribeLogStreamsJSON builds a DescribeLogStreams response.
func DescribeLogStreamsJSON(streams []store.LogStream) ([]byte, error) {
	entries := make([]map[string]any, 0, len(streams))
	for _, st := range streams {
		entries = append(entries, map[string]any{
			"logGroupName":        st.LogGroupName,
			"logStreamName":       st.LogStreamName,
			"arn":                 st.Arn,
			"creationTime":        st.CreationTime,
			"firstEventTimestamp": st.FirstEventTimestamp,
			"lastEventTimestamp":  st.LastEventTimestamp,
			"lastIngestionTime":   st.LastIngestionTime,
			"uploadSequenceToken": st.UploadSequenceToken,
			"storedBytes":         st.StoredBytes,
		})
	}
	return json.Marshal(map[string]any{"logStreams": entries})
}

// DescribeLogGroupsJSON builds a DescribeLogGroups response.
func DescribeLogGroupsJSON(groups []store.LogGroup) ([]byte, error) {
	entries := make([]map[string]any, 0, len(groups))
	for _, g := range groups {
		entry := map[string]any{
			"logGroupName":      g.LogGroupName,
			"arn":               g.Arn,
			"creationTime":      g.CreationTime,
			"storedBytes":       g.StoredBytes,
			"metricFilterCount": g.MetricFilterCount,
		}
		if g.RetentionInDays > 0 {
			entry["retentionInDays"] = g.RetentionInDays
		}
		entries = append(entries, entry)
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

// FilterLogEventsJSON builds a FilterLogEvents response.
func FilterLogEventsJSON(events []store.FilteredLogEvent, nextToken string) ([]byte, error) {
	entries := make([]map[string]any, 0, len(events))
	for _, ev := range events {
		entries = append(entries, map[string]any{
			"logStreamName": ev.LogStreamName,
			"timestamp":     ev.Timestamp,
			"message":       ev.Message,
			"ingestionTime": ev.IngestionTime,
			"eventId":       ev.EventID,
		})
	}
	out := map[string]any{"events": entries}
	if nextToken != "" {
		out["nextToken"] = nextToken
	}
	return json.Marshal(out)
}

// DescribeSubscriptionFiltersJSON builds DescribeSubscriptionFilters response.
func DescribeSubscriptionFiltersJSON(filters []store.LogsSubscriptionFilter) ([]byte, error) {
	entries := make([]map[string]any, 0, len(filters))
	for _, f := range filters {
		entries = append(entries, map[string]any{
			"filterName":      f.FilterName,
			"logGroupName":    f.LogGroupName,
			"filterPattern":   f.FilterPattern,
			"destinationArn":  f.DestinationARN,
			"roleArn":         f.RoleARN,
			"creationTime":    f.CreatedAt,
		})
	}
	return json.Marshal(map[string]any{"subscriptionFilters": entries})
}

// DescribeMetricFiltersJSON builds DescribeMetricFilters response.
func DescribeMetricFiltersJSON(filters []store.LogsMetricFilter) ([]byte, error) {
	entries := make([]map[string]any, 0, len(filters))
	for _, f := range filters {
		entries = append(entries, map[string]any{
			"filterName":    f.FilterName,
			"logGroupName":  f.LogGroupName,
			"filterPattern": f.FilterPattern,
			"metricTransformations": []map[string]any{{
				"metricName":      f.MetricName,
				"metricNamespace": f.MetricNamespace,
				"metricValue":     f.MetricValue,
			}},
			"creationTime": f.CreatedAt,
		})
	}
	return json.Marshal(map[string]any{"metricFilters": entries})
}
