package detective_test

import (
	"encoding/json"
	"testing"
	"time"

	detectivesvc "github.com/Kyaxris-Labs/Noctaxris/internal/services/detective"
	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

func TestListGraphsJSONShape(t *testing.T) {
	payload, err := detectivesvc.ListGraphsJSON([]store.DetectiveGraph{{
		GraphARN:  "arn:aws:detective:us-east-1:111122223333:graph:abc",
		CreatedAt: time.Date(2020, 1, 22, 11, 35, 11, 372000000, time.UTC),
	}})
	if err != nil {
		t.Fatal(err)
	}
	var out map[string]any
	if err := json.Unmarshal(payload, &out); err != nil {
		t.Fatal(err)
	}
	list, ok := out["GraphList"].([]any)
	if !ok || len(list) != 1 {
		t.Fatalf("GraphList=%v", out["GraphList"])
	}
	item := list[0].(map[string]any)
	if item["Arn"] == "" || item["CreatedTime"] == "" {
		t.Fatalf("item=%v", item)
	}
	_, _ = detectivesvc.ListGraphsJSON(nil)
}

func TestCreateGraphJSONShape(t *testing.T) {
	payload, err := detectivesvc.CreateGraphJSON("arn:aws:detective:us-east-1:1:graph:x")
	if err != nil {
		t.Fatal(err)
	}
	var out map[string]string
	if err := json.Unmarshal(payload, &out); err != nil {
		t.Fatal(err)
	}
	if out["GraphArn"] == "" {
		t.Fatalf("out=%v", out)
	}
}

func TestDetectiveSearchAndHelpers(t *testing.T) {
	if _, err := detectivesvc.AcceptInvitationJSON(); err != nil {
		t.Fatal(err)
	}
	created := time.Date(2020, 1, 22, 11, 35, 11, 0, time.UTC)
	if detectivesvc.FormatGraphCreatedTime(created) == "" {
		t.Fatal("format time")
	}
	res := store.DetectiveSearchResult{
		GraphARN: "arn:graph", CloudTrailEvents: []json.RawMessage{json.RawMessage(`{"e":1}`)},
		GuardDutyFindings: []store.GuardDutyFinding{{Id: "f1"}},
		MatchedResourceArn: "arn:res", MatchedIpAddress: "1.2.3.4",
	}
	raw, err := detectivesvc.SearchGraphJSON(res)
	if err != nil {
		t.Fatal(err)
	}
	var out map[string]any
	_ = json.Unmarshal(raw, &out)
	if out["MatchedResourceArn"] != "arn:res" {
		t.Fatalf("search=%v", out)
	}
	res.CloudTrailEvents, res.GuardDutyFindings = nil, nil
	raw, _ = detectivesvc.SearchGraphJSON(res)
	_ = json.Unmarshal(raw, &out)
}
