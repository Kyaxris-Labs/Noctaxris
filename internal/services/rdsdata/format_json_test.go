package rdsdata

import (
	"encoding/json"
	"testing"

	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

func TestFormatRecordsAsJSON(t *testing.T) {
	label := "n"
	var n int64 = 9
	null := false
	flag := true
	res := store.RDSDataExecuteResult{
		ColumnMetadata: []store.RDSDataColumnMeta{
			{Name: "n", TypeName: "BIGINT", Label: "n"},
			{Name: "flag", TypeName: "BOOLEAN", Label: "flag"},
		},
		Records: [][]store.RDSDataField{
			{
				{LongValue: &n, IsNull: &null},
				{BooleanValue: &flag, IsNull: &null},
			},
		},
		FormattedRecords: `{"noctaxrisExecutor":"pgx"}`,
	}
	out, err := ApplyFormatRecordsAsJSON(res)
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Records) != 0 || len(out.ColumnMetadata) != 0 {
		t.Fatalf("JSON mode must clear records/metadata: %+v", out)
	}
	var rows []map[string]any
	if err := json.Unmarshal([]byte(out.FormattedRecords), &rows); err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0]["n"].(float64) != 9 || rows[0]["flag"] != true {
		t.Fatalf("rows=%v label=%s", rows, label)
	}
}

func TestBatchExecuteStatementJSONGeneratedFields(t *testing.T) {
	var id int64 = 42
	null := false
	raw, err := BatchExecuteStatementJSON([]store.RDSDataExecuteResult{
		{
			GeneratedFields: []store.RDSDataField{{LongValue: &id, IsNull: &null}},
		},
		{
			Records: [][]store.RDSDataField{{{LongValue: &id, IsNull: &null}}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	var body map[string]any
	if err := json.Unmarshal(raw, &body); err != nil {
		t.Fatal(err)
	}
	updates := body["updateResults"].([]any)
	if len(updates) != 2 {
		t.Fatalf("updates=%v", updates)
	}
	first := updates[0].(map[string]any)["generatedFields"].([]any)
	if len(first) != 1 {
		t.Fatalf("first generatedFields=%v", first)
	}
	second := updates[1].(map[string]any)["generatedFields"].([]any)
	if len(second) != 1 {
		t.Fatalf("second generatedFields from RETURNING records=%v", second)
	}
}
