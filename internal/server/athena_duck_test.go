package server_test

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/Kyaxris-Labs/Noctaxris/internal/compute"
)

func TestAthenaDuckDBEngineFailClosed(t *testing.T) {
	t.Setenv(compute.EnvAthenaEngine, "duckdb")
	t.Setenv(compute.EnvDuckDBURL, "")
	// Empty DockerHost on test server → DuckDB unavailable → FAILED execution.
	srv, _, _ := newTestServerStore(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	createDB := mustJSONTarget(t, handler, "AWSGlue.CreateDatabase", "glue", map[string]any{
		"DatabaseInput": map[string]any{"Name": "duckdbfail"},
	}, now)
	if createDB.Code != http.StatusOK {
		t.Fatalf("CreateDatabase status=%d body=%q", createDB.Code, createDB.Body.String())
	}
	createTbl := mustJSONTarget(t, handler, "AWSGlue.CreateTable", "glue", map[string]any{
		"DatabaseName": "duckdbfail",
		"TableInput": map[string]any{
			"Name": "t1",
			"StorageDescriptor": map[string]any{
				"Location": "s3://missing-bucket/p/",
				"Columns":  []map[string]any{{"Name": "id", "Type": "string"}},
			},
		},
	}, now)
	if createTbl.Code != http.StatusOK {
		t.Fatalf("CreateTable status=%d body=%q", createTbl.Code, createTbl.Body.String())
	}

	start := mustJSONTarget(t, handler, "AmazonAthena.StartQueryExecution", "athena", map[string]any{
		"QueryString": "SELECT * FROM duckdbfail.t1",
		"QueryExecutionContext": map[string]any{
			"Database": "duckdbfail",
		},
	}, now)
	if start.Code != http.StatusOK {
		t.Fatalf("StartQueryExecution status=%d body=%q", start.Code, start.Body.String())
	}
	var startBody map[string]any
	if err := json.Unmarshal(start.Body.Bytes(), &startBody); err != nil {
		t.Fatal(err)
	}
	qid, _ := startBody["QueryExecutionId"].(string)
	if qid == "" {
		t.Fatalf("missing QueryExecutionId: %s", start.Body.String())
	}
	get := mustJSONTarget(t, handler, "AmazonAthena.GetQueryExecution", "athena", map[string]any{
		"QueryExecutionId": qid,
	}, now)
	if get.Code != http.StatusOK {
		t.Fatalf("GetQueryExecution status=%d body=%q", get.Code, get.Body.String())
	}
	if !jsonContainsState(get.Body.Bytes(), "FAILED") {
		t.Fatalf("expected FAILED DuckDB path, body=%s", get.Body.String())
	}
}

func jsonContainsState(body []byte, state string) bool {
	var m map[string]any
	if err := json.Unmarshal(body, &m); err != nil {
		return false
	}
	qe, _ := m["QueryExecution"].(map[string]any)
	st, _ := qe["Status"].(map[string]any)
	got, _ := st["State"].(string)
	return got == state
}
