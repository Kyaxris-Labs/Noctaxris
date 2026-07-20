package server_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func mustSFNJSON(t *testing.T, handler http.Handler, target string, payload map[string]any, now time.Time) *httptest.ResponseRecorder {
	t.Helper()
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	req := mustNewRequest(t, http.MethodPost, "http://127.0.0.1:4566/", raw)
	req.Header.Set("Content-Type", "application/x-amz-json-1.0")
	req.Header.Set("X-Amz-Target", "AWSStepFunctions."+target)
	signHeader(t, req, raw, testAccessKey, testSecret, testRegion, "states", now)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}

func TestSFNPassSucceedRoundTrip(t *testing.T) {
	srv, _ := newTestServer(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	def := `{"StartAt":"P","States":{"P":{"Type":"Pass","Result":{"hello":"world"},"End":true}}}`
	create := mustSFNJSON(t, handler, "CreateStateMachine", map[string]any{
		"name":       "lab-sm",
		"definition": def,
	}, now)
	if create.Code != http.StatusOK {
		t.Fatalf("CreateStateMachine status=%d body=%q", create.Code, create.Body.String())
	}
	var createOut map[string]any
	_ = json.Unmarshal(create.Body.Bytes(), &createOut)
	smARN, _ := createOut["stateMachineArn"].(string)

	start := mustSFNJSON(t, handler, "StartExecution", map[string]any{
		"stateMachineArn": smARN,
		"name":            "exec1",
		"input":           `{"a":1}`,
	}, now)
	if start.Code != http.StatusOK {
		t.Fatalf("StartExecution status=%d body=%q", start.Code, start.Body.String())
	}
	var startOut map[string]any
	_ = json.Unmarshal(start.Body.Bytes(), &startOut)
	execARN, _ := startOut["executionArn"].(string)

	desc := mustSFNJSON(t, handler, "DescribeExecution", map[string]any{
		"executionArn": execARN,
	}, now)
	if desc.Code != http.StatusOK {
		t.Fatalf("DescribeExecution status=%d body=%q", desc.Code, desc.Body.String())
	}
	var descOut map[string]any
	_ = json.Unmarshal(desc.Body.Bytes(), &descOut)
	if descOut["status"] != "SUCCEEDED" {
		t.Fatalf("status=%v body=%q", descOut["status"], desc.Body.String())
	}

	hist := mustSFNJSON(t, handler, "GetExecutionHistory", map[string]any{
		"executionArn": execARN,
	}, now)
	if hist.Code != http.StatusOK {
		t.Fatalf("GetExecutionHistory status=%d body=%q", hist.Code, hist.Body.String())
	}

	list := mustSFNJSON(t, handler, "ListStateMachines", map[string]any{}, now)
	if list.Code != http.StatusOK {
		t.Fatalf("ListStateMachines status=%d body=%q", list.Code, list.Body.String())
	}

	del := mustSFNJSON(t, handler, "DeleteStateMachine", map[string]any{
		"stateMachineArn": smARN,
	}, now)
	if del.Code != http.StatusOK {
		t.Fatalf("DeleteStateMachine status=%d body=%q", del.Code, del.Body.String())
	}
}
