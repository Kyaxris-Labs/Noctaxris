package logs_test

import (
	"encoding/json"
	"testing"

	logssvc "github.com/Kyaxris-Labs/Noctaxris/internal/services/logs"
	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

func TestLogsJSON(t *testing.T) {
	if _, err := logssvc.EmptyOKJSON(); err != nil {
		t.Fatal(err)
	}

	stream := store.LogStream{
		LogGroupName: "/g", LogStreamName: "s", Arn: "arn:stream",
		CreationTime: 1, FirstEventTimestamp: 2, LastEventTimestamp: 3,
		LastIngestionTime: 4, UploadSequenceToken: "tok", StoredBytes: 10,
	}
	raw, err := logssvc.DescribeLogStreamsJSON([]store.LogStream{stream})
	if err != nil {
		t.Fatal(err)
	}
	var out map[string]any
	_ = json.Unmarshal(raw, &out)

	g := store.LogGroup{LogGroupName: "/g", Arn: "arn:g", CreationTime: 1, StoredBytes: 0, MetricFilterCount: 0, RetentionInDays: 7}
	raw, _ = logssvc.DescribeLogGroupsJSON([]store.LogGroup{g})
	g.RetentionInDays = 0
	raw, _ = logssvc.DescribeLogGroupsJSON([]store.LogGroup{g})

	raw, _ = logssvc.PutLogEventsJSON("next-tok")
	_ = json.Unmarshal(raw, &out)

	ev := store.LogEvent{Timestamp: 1, Message: "hi", IngestionTime: 2, EventID: "e1"}
	raw, _ = logssvc.GetLogEventsJSON([]store.LogEvent{ev})

	fev := store.FilteredLogEvent{LogStreamName: "s", Timestamp: 1, Message: "m", IngestionTime: 2, EventID: "e2"}
	raw, _ = logssvc.FilterLogEventsJSON([]store.FilteredLogEvent{fev}, "")
	raw, _ = logssvc.FilterLogEventsJSON(nil, "page-2")
	_ = json.Unmarshal(raw, &out)

	sub := store.LogsSubscriptionFilter{
		FilterName: "sub", LogGroupName: "/g", FilterPattern: "", DestinationARN: "arn:dest", RoleARN: "arn:role", CreatedAt: 1,
	}
	raw, _ = logssvc.DescribeSubscriptionFiltersJSON([]store.LogsSubscriptionFilter{sub})

	mf := store.LogsMetricFilter{
		FilterName: "mf", LogGroupName: "/g", FilterPattern: "[...]", MetricName: "m", MetricNamespace: "n", MetricValue: "1", CreatedAt: 1,
	}
	raw, _ = logssvc.DescribeMetricFiltersJSON([]store.LogsMetricFilter{mf})
	_ = json.Unmarshal(raw, &out)
}
