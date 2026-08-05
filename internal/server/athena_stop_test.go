package server_test

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestAthenaStopQueryExecution(t *testing.T) {
	srv, _, _ := newTestServerStore(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	start := mustJSONTarget(t, handler, "AmazonAthena.StartQueryExecution", "athena", map[string]any{
		"QueryString": "SELECT 1",
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

	stop := mustJSONTarget(t, handler, "AmazonAthena.StopQueryExecution", "athena", map[string]any{
		"QueryExecutionId": qid,
	}, now)
	if stop.Code != http.StatusOK {
		t.Fatalf("StopQueryExecution status=%d body=%q", stop.Code, stop.Body.String())
	}

	stopAgain := mustJSONTarget(t, handler, "AmazonAthena.StopQueryExecution", "athena", map[string]any{
		"QueryExecutionId": qid,
	}, now)
	// Idempotent stop or already-terminal may still be OK depending on store.
	if stopAgain.Code != http.StatusOK && stopAgain.Code != http.StatusBadRequest {
		t.Fatalf("Stop again status=%d body=%q", stopAgain.Code, stopAgain.Body.String())
	}

	miss := mustJSONTarget(t, handler, "AmazonAthena.StopQueryExecution", "athena", map[string]any{
		"QueryExecutionId": "missing-qid",
	}, now)
	if miss.Code != http.StatusBadRequest || !strings.Contains(miss.Body.String(), "not found") {
		t.Fatalf("Stop missing status=%d body=%q", miss.Code, miss.Body.String())
	}
	empty := mustJSONTarget(t, handler, "AmazonAthena.StopQueryExecution", "athena", map[string]any{
		"QueryExecutionId": "",
	}, now)
	// Empty id may surface as not-found (400) or store error (500).
	if empty.Code != http.StatusBadRequest && empty.Code != http.StatusInternalServerError {
		t.Fatalf("Stop empty status=%d body=%q", empty.Code, empty.Body.String())
	}
}
