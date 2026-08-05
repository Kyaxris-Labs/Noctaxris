package athena_test

import (
	"encoding/json"
	"testing"

	athenasvc "github.com/Kyaxris-Labs/Noctaxris/internal/services/athena"
	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

func TestAthenaJSON(t *testing.T) {
	raw, err := athenasvc.StartQueryExecutionJSON("q-1")
	if err != nil {
		t.Fatal(err)
	}
	var out map[string]any
	_ = json.Unmarshal(raw, &out)

	qe := store.AthenaQueryExecution{
		QueryExecutionID: "q-1", QueryString: "SELECT 1", WorkGroup: "primary",
		DatabaseName: "db", CatalogName: "AwsDataCatalog", OutputLocation: "s3://out/",
		State: "FAILED", StateChangeReason: "syntax", ErrorMessage: "bad",
		SubmissionMS: 1_700_000_000_000, CompletionMS: 1_700_000_001_000,
		ResultColumns: []store.AthenaColumnInfo{{Name: "c1", Type: "varchar"}},
		ResultRows:    [][]string{{"1"}},
	}
	raw, _ = athenasvc.GetQueryExecutionJSON(qe)
	_ = json.Unmarshal(raw, &out)
	qe.State, qe.ErrorMessage, qe.StateChangeReason, qe.CompletionMS = "SUCCEEDED", "", "", 0
	raw, _ = athenasvc.GetQueryExecutionJSON(qe)

	raw, _ = athenasvc.GetQueryResultsJSON(qe)
	_ = json.Unmarshal(raw, &out)

	for _, fn := range []func() ([]byte, error){
		athenasvc.StopQueryExecutionJSON, athenasvc.CreateWorkGroupJSON, athenasvc.DeleteWorkGroupJSON, athenasvc.UpdateWorkGroupJSON,
	} {
		if _, err := fn(); err != nil {
			t.Fatal(err)
		}
	}

	wg := store.AthenaWorkGroup{
		Name: "wg", State: "ENABLED", Description: "d", CreatedMS: 1_700_000_000_000,
		OutputLocation: "s3://wg/", EnforceWorkGroupConfig: true,
	}
	raw, _ = athenasvc.GetWorkGroupJSON(wg)
	raw, _ = athenasvc.ListWorkGroupsJSON([]store.AthenaWorkGroup{wg})
	_ = json.Unmarshal(raw, &out)
}
