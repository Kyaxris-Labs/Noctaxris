package server_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
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

func TestSFNHandlersTaskTokenAndValidationCoverage(t *testing.T) {
	srv, _ := newTestServer(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	mustCreateIAMRole(t, handler, "sfn-cov-role", `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":{"Service":"states.amazonaws.com"},"Action":"sts:AssumeRole"}]}`, now)
	roleARN := "arn:aws:iam::" + testAccountID + ":role/sfn-cov-role"

	def := `{
  "StartAt": "WaitCb",
  "States": {
    "WaitCb": {
      "Type": "Task",
      "Resource": "arn:aws:states:::sqs:sendMessage.waitForTaskToken",
      "Parameters": {
        "WaitForTaskToken": true,
        "MessageBody": {"token.$": "$$.Task.Token"}
      },
      "End": true
    }
  }
}`
	create := mustSFNJSON(t, handler, "CreateStateMachine", map[string]any{
		"name": "cov-callback-sm", "definition": def, "roleArn": roleARN,
	}, now)
	if create.Code != http.StatusOK {
		t.Fatalf("CreateStateMachine %d %s", create.Code, create.Body.String())
	}
	var createOut map[string]any
	_ = json.Unmarshal(create.Body.Bytes(), &createOut)
	smARN, _ := createOut["stateMachineArn"].(string)

	noToken := mustSFNJSON(t, handler, "SendTaskSuccess", map[string]any{"output": `{}`}, now)
	if noToken.Code != http.StatusBadRequest || !strings.Contains(noToken.Body.String(), "InvalidToken") {
		t.Fatalf("SendTaskSuccess no token want InvalidToken got %d %s", noToken.Code, noToken.Body.String())
	}
	badToken := mustSFNJSON(t, handler, "SendTaskHeartbeat", map[string]any{"taskToken": "not-a-token"}, now)
	if badToken.Code != http.StatusBadRequest {
		t.Fatalf("SendTaskHeartbeat bad token want 400 got %d %s", badToken.Code, badToken.Body.String())
	}

	start := mustSFNJSON(t, handler, "StartExecution", map[string]any{
		"stateMachineArn": smARN, "name": "cb-run", "input": `{}`,
	}, now)
	if start.Code != http.StatusOK {
		t.Fatalf("StartExecution %d %s", start.Code, start.Body.String())
	}
	var startOut map[string]any
	_ = json.Unmarshal(start.Body.Bytes(), &startOut)
	execARN, _ := startOut["executionArn"].(string)

	desc := mustSFNJSON(t, handler, "DescribeExecution", map[string]any{"executionArn": execARN}, now)
	if desc.Code != http.StatusOK {
		t.Fatalf("DescribeExecution %d", desc.Code)
	}
	var descOut map[string]any
	_ = json.Unmarshal(desc.Body.Bytes(), &descOut)
	if descOut["status"] != "RUNNING" {
		t.Fatalf("want RUNNING got %v", descOut["status"])
	}

	hist := mustSFNJSON(t, handler, "GetExecutionHistory", map[string]any{"executionArn": execARN}, now)
	if hist.Code != http.StatusOK {
		t.Fatalf("GetExecutionHistory %d %s", hist.Code, hist.Body.String())
	}
	token := sfnTaskTokenFromHistoryBody(t, hist.Body.Bytes())
	if token == "" {
		t.Fatalf("missing taskToken in history %s", hist.Body.String())
	}

	hb := mustSFNJSON(t, handler, "SendTaskHeartbeat", map[string]any{"taskToken": token}, now)
	if hb.Code != http.StatusOK {
		t.Fatalf("SendTaskHeartbeat %d %s", hb.Code, hb.Body.String())
	}

	fail := mustSFNJSON(t, handler, "SendTaskFailure", map[string]any{
		"taskToken": token, "error": "ManualReject", "cause": "denied",
	}, now)
	if fail.Code != http.StatusOK {
		t.Fatalf("SendTaskFailure %d %s", fail.Code, fail.Body.String())
	}

	descFail := mustSFNJSON(t, handler, "DescribeExecution", map[string]any{"executionArn": execARN}, now)
	_ = json.Unmarshal(descFail.Body.Bytes(), &descOut)
	if descOut["status"] != "FAILED" {
		t.Fatalf("want FAILED got %v", descOut["status"])
	}

	putPolBad := mustSFNJSON(t, handler, "PutResourcePolicy", map[string]any{}, now)
	if putPolBad.Code != http.StatusBadRequest {
		t.Fatalf("PutResourcePolicy empty want 400 got %d", putPolBad.Code)
	}
	getPolMissing := mustSFNJSON(t, handler, "GetResourcePolicy", map[string]any{"stateMachineArn": smARN}, now)
	if getPolMissing.Code != http.StatusBadRequest || !strings.Contains(getPolMissing.Body.String(), "ResourceNotFound") {
		t.Fatalf("GetResourcePolicy missing want ResourceNotFound got %d %s", getPolMissing.Code, getPolMissing.Body.String())
	}

	_ = mustSFNJSON(t, handler, "DeleteStateMachine", map[string]any{"stateMachineArn": smARN}, now)
}

func sfnTaskTokenFromHistoryBody(t *testing.T, raw []byte) string {
	t.Helper()
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatal(err)
	}
	events, _ := out["events"].([]any)
	for _, ev := range events {
		m, _ := ev.(map[string]any)
		for _, key := range []string{
			"stateEnteredEventDetails",
			"taskScheduledEventDetails",
			"taskStartedEventDetails",
		} {
			details, _ := m[key].(map[string]any)
			if tok, ok := details["taskToken"].(string); ok && tok != "" {
				return tok
			}
		}
	}
	return ""
}
