package guardduty_test

import (
	"encoding/json"
	"testing"

	gdsvc "github.com/Kyaxris-Labs/Noctaxris/internal/services/guardduty"
	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

func TestGuardDutyJSON(t *testing.T) {
	createRaw, err := gdsvc.CreateDetectorJSON("det-1")
	if err != nil {
		t.Fatal(err)
	}
	var createOut map[string]string
	if err := json.Unmarshal(createRaw, &createOut); err != nil {
		t.Fatal(err)
	}
	if createOut["DetectorId"] != "det-1" {
		t.Fatalf("create=%v", createOut)
	}

	for _, fn := range []struct {
		name string
		raw  func() ([]byte, error)
		key  string
	}{
		{"list detectors", func() ([]byte, error) { return gdsvc.ListDetectorsJSON(nil) }, "DetectorIds"},
		{"list findings", func() ([]byte, error) { return gdsvc.ListFindingsJSON([]string{"f1"}) }, "FindingIds"},
		{"inject", func() ([]byte, error) { return gdsvc.InjectFindingsJSON(nil) }, "FindingIds"},
	} {
		raw, err := fn.raw()
		if err != nil {
			t.Fatalf("%s: %v", fn.name, err)
		}
		var out map[string]any
		if err := json.Unmarshal(raw, &out); err != nil {
			t.Fatalf("%s unmarshal: %v", fn.name, err)
		}
		if _, ok := out[fn.key]; !ok {
			t.Fatalf("%s missing %s: %v", fn.name, fn.key, out)
		}
	}

	getRaw, err := gdsvc.GetFindingsJSON([]store.GuardDutyFinding{{Id: "f-1"}})
	if err != nil {
		t.Fatal(err)
	}
	var getOut map[string]any
	if err := json.Unmarshal(getRaw, &getOut); err != nil {
		t.Fatal(err)
	}
	findings, _ := getOut["Findings"].([]any)
	if len(findings) != 1 {
		t.Fatalf("get=%v", getOut)
	}

	nilGetRaw, err := gdsvc.GetFindingsJSON(nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(nilGetRaw, &getOut); err != nil {
		t.Fatal(err)
	}
	findings, _ = getOut["Findings"].([]any)
	if len(findings) != 0 {
		t.Fatalf("nil get=%v", getOut)
	}
}
