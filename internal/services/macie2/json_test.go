package macie2_test

import (
	"encoding/json"
	"testing"

	macie2svc "github.com/Kyaxris-Labs/Noctaxris/internal/services/macie2"
	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

func TestMacie2JSON(t *testing.T) {
	if raw, err := macie2svc.EnableMacieJSON(); err != nil || string(raw) != "{}" {
		t.Fatalf("enable: %s %v", raw, err)
	}

	sess := store.MacieSession{Status: "ENABLED", CreatedAt: 1_700_000_000_000, UpdatedAt: 1_700_000_001_000}
	raw, err := macie2svc.GetMacieSessionJSON(sess)
	if err != nil {
		t.Fatal(err)
	}
	var out map[string]any
	_ = json.Unmarshal(raw, &out)
	if out["status"] != "ENABLED" {
		t.Fatalf("session=%v", out)
	}

	job := store.MacieJob{JobID: "j1", JobARN: "arn:job", Name: "n", JobType: "ONE_TIME", JobStatus: "RUNNING"}
	raw, _ = macie2svc.CreateClassificationJobJSON(job)
	_ = json.Unmarshal(raw, &out)

	raw, _ = macie2svc.DescribeClassificationJobJSON(job)
	_ = json.Unmarshal(raw, &out)
	if out["jobId"] != "j1" {
		t.Fatalf("describe=%v", out)
	}
	job.Raw = map[string]any{"custom": true}
	raw, _ = macie2svc.DescribeClassificationJobJSON(job)
	_ = json.Unmarshal(raw, &out)
	if out["custom"] != true {
		t.Fatalf("raw job=%v", out)
	}

	raw, _ = macie2svc.ListClassificationJobsJSON([]store.MacieJob{job})
	_ = json.Unmarshal(raw, &out)
	items, _ := out["items"].([]any)
	if len(items) != 1 {
		t.Fatalf("list jobs=%v", out)
	}

	raw, _ = macie2svc.ListFindingsJSON(nil)
	_ = json.Unmarshal(raw, &out)
	raw, _ = macie2svc.ListFindingsJSON([]string{"f-1"})
	_ = json.Unmarshal(raw, &out)

	raw, _ = macie2svc.GetFindingsJSON(nil)
	_ = json.Unmarshal(raw, &out)
	raw, _ = macie2svc.GetFindingsJSON([]store.MacieFinding{{Id: "f-1"}})
	_ = json.Unmarshal(raw, &out)

	raw, _ = macie2svc.InjectFindingsJSON([]string{"inj-1"})
	_ = json.Unmarshal(raw, &out)
}
