package securityhub_test

import (
	"encoding/json"
	"testing"

	shsvc "github.com/Kyaxris-Labs/Noctaxris/internal/services/securityhub"
	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

func TestSecurityHubJSON(t *testing.T) {
	batchRaw, err := shsvc.BatchImportFindingsJSON(nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	var batch map[string]any
	if err := json.Unmarshal(batchRaw, &batch); err != nil {
		t.Fatal(err)
	}
	if batch["SuccessCount"].(float64) != 0 || batch["FailedCount"].(float64) != 0 {
		t.Fatalf("batch=%v", batch)
	}

	batchRaw, err = shsvc.BatchImportFindingsJSON([]string{"id-1"}, []map[string]any{{"Id": "bad"}})
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(batchRaw, &batch); err != nil {
		t.Fatal(err)
	}
	if batch["SuccessCount"].(float64) != 1 || batch["FailedCount"].(float64) != 1 {
		t.Fatalf("batch=%v", batch)
	}

	findings := []store.SecurityHubFinding{{Id: "f-1", Title: "t"}}
	getRaw, err := shsvc.GetFindingsJSON(findings)
	if err != nil {
		t.Fatal(err)
	}
	var getOut map[string]any
	if err := json.Unmarshal(getRaw, &getOut); err != nil {
		t.Fatal(err)
	}
	arr, _ := getOut["Findings"].([]any)
	if len(arr) != 1 {
		t.Fatalf("findings=%v", getOut)
	}

	nilRaw, err := shsvc.GetFindingsJSON(nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(nilRaw, &getOut); err != nil {
		t.Fatal(err)
	}
	arr, _ = getOut["Findings"].([]any)
	if len(arr) != 0 {
		t.Fatalf("nil findings=%v", getOut)
	}
}
