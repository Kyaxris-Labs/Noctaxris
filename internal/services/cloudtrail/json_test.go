package cloudtrail_test

import (
	"encoding/json"
	"testing"
	"time"

	ctsvc "github.com/Kyaxris-Labs/Noctaxris/internal/services/cloudtrail"
	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

func TestCloudTrailJSON(t *testing.T) {
	sel := store.CloudTrailEventSelectors{IncludeManagementEvents: true, S3DataEventsEnabled: false}
	selRaw, err := ctsvc.EventSelectorsJSON(sel)
	if err != nil {
		t.Fatal(err)
	}
	var selOut map[string]any
	if err := json.Unmarshal(selRaw, &selOut); err != nil {
		t.Fatal(err)
	}
	selectors, _ := selOut["EventSelectors"].([]any)
	if len(selectors) != 1 {
		t.Fatalf("selectors=%v", selOut)
	}
	first, _ := selectors[0].(map[string]any)
	if _, ok := first["DataResources"]; ok {
		t.Fatalf("expected no data resources: %v", first)
	}

	sel.S3DataEventsEnabled = true
	selRaw, err = ctsvc.EventSelectorsJSON(sel)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(selRaw, &selOut); err != nil {
		t.Fatal(err)
	}
	selectors, _ = selOut["EventSelectors"].([]any)
	first, _ = selectors[0].(map[string]any)
	if first["DataResources"] == nil {
		t.Fatalf("expected data resources: %v", first)
	}

	ev := store.CloudTrailEventRecord{
		EventId: "e-1", EventName: "PutObject", EventSource: "s3.amazonaws.com",
		CloudTrailEvent: `{}`, Username: "alice",
		EventTime: time.Date(2024, 1, 2, 3, 4, 5, 0, time.UTC),
	}
	lookupRaw, err := ctsvc.LookupEventsJSON([]store.CloudTrailEventRecord{ev, {}})
	if err != nil {
		t.Fatal(err)
	}
	var lookupOut map[string]any
	if err := json.Unmarshal(lookupRaw, &lookupOut); err != nil {
		t.Fatal(err)
	}
	events, _ := lookupOut["Events"].([]any)
	if len(events) != 2 {
		t.Fatalf("events=%v", lookupOut)
	}
	e0, _ := events[0].(map[string]any)
	if e0["Username"] != "alice" || e0["EventTime"] == "" {
		t.Fatalf("event0=%v", e0)
	}
	e1, _ := events[1].(map[string]any)
	if _, ok := e1["EventTime"]; ok {
		t.Fatalf("zero time should omit EventTime: %v", e1)
	}
}
