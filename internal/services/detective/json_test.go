package detective

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

func TestListGraphsJSONShape(t *testing.T) {
	payload, err := ListGraphsJSON([]store.DetectiveGraph{{
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
}

func TestCreateGraphJSONShape(t *testing.T) {
	payload, err := CreateGraphJSON("arn:aws:detective:us-east-1:1:graph:x")
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
