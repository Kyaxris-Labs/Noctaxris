package events_test

import (
	"encoding/json"
	"testing"

	eventssvc "github.com/Kyaxris-Labs/Noctaxris/internal/services/events"
	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

func TestEventBridgeJSON(t *testing.T) {
	if eventssvc.JSONContentType() == "" {
		t.Fatal("content type")
	}
	result := store.PutEventsResult{FailedEntryCount: 1, Entries: []store.PutEventsResultEntry{
		{EventID: "ev-1"},
		{ErrorCode: "InternalFailure", ErrorMessage: "fail"},
	}}
	raw, err := eventssvc.PutEventsJSON(result)
	if err != nil {
		t.Fatal(err)
	}
	var pe map[string]any
	if err := json.Unmarshal(raw, &pe); err != nil {
		t.Fatal(err)
	}

	rule := store.EventRule{Name: "lab-rule", ARN: "arn:aws:events:us-east-1:1:rule/lab-rule", State: "ENABLED"}
	if _, err := eventssvc.PutRuleJSON(rule); err != nil {
		t.Fatal(err)
	}
	if _, err := eventssvc.DescribeRuleJSON(rule); err != nil {
		t.Fatal(err)
	}
	if _, err := eventssvc.ListRulesJSON([]store.EventRule{rule}); err != nil {
		t.Fatal(err)
	}

	bus := store.EventBus{Name: "custom", ARN: "arn:aws:events:us-east-1:1:event-bus/custom", CreationDate: "2024-01-01T00:00:00Z"}
	if _, err := eventssvc.CreateEventBusJSON(bus); err != nil {
		t.Fatal(err)
	}
	if _, err := eventssvc.DescribeEventBusJSON(bus); err != nil {
		t.Fatal(err)
	}
	if _, err := eventssvc.ListEventBusesJSON([]store.EventBus{bus}); err != nil {
		t.Fatal(err)
	}

	target := store.EventTarget{
		ID: "t1", ARN: "arn:aws:lambda:us-east-1:1:function:fn", RoleARN: "arn:aws:iam::1:role/lambda",
		Input: "{}", InputPath: "$.detail", InputTransformerJSON: `{"InputPathsMap":{"x":"$.x"},"InputTemplate":"{}"}`,
	}
	if _, err := eventssvc.PutTargetsJSON(nil); err != nil {
		t.Fatal(err)
	}
	if _, err := eventssvc.ListTargetsByRuleJSON([]store.EventTarget{target}); err != nil {
		t.Fatal(err)
	}
	if _, err := eventssvc.EmptyOKJSON(); err != nil {
		t.Fatal(err)
	}
	if _, err := eventssvc.ListTagsForResourceJSON([]store.ResourceTag{{Key: "k", Value: "v"}}); err != nil {
		t.Fatal(err)
	}
}
