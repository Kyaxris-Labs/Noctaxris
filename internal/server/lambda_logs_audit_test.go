package server_test

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

func TestLambdaInvokeShipsCloudWatchLogs(t *testing.T) {
	srv, st, _ := newTestServerStore(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	srv.SetLambdaInvokeHookForTest(func(_ context.Context, _, _ string, _ store.LambdaFunction, _, _ string) ([]byte, error) {
		return []byte(`{"ok":true}`), nil
	})

	mustCreateIAMRole(t, handler, "logs-invoke-role", lambdaTrustOK, now)
	roleARN := "arn:aws:iam::" + testAccountID + ":role/logs-invoke-role"
	fnName := "logs-ship-fn"
	createRec := mustLambdaJSON(t, handler, "CreateFunction", map[string]any{
		"FunctionName": fnName,
		"Runtime":      "python3.12",
		"Role":         roleARN,
		"Handler":      "app.handler",
		"Code":         map[string]any{"ZipFile": testLambdaZipB64(t)},
	}, now)
	if createRec.Code != http.StatusOK {
		t.Fatalf("CreateFunction status=%d body=%q", createRec.Code, createRec.Body.String())
	}

	invokeRec := mustLambdaJSON(t, handler, "Invoke", map[string]any{
		"FunctionName": fnName,
		"Payload":      `{"ping":true}`,
	}, now)
	if invokeRec.Code != http.StatusOK {
		t.Fatalf("Invoke status=%d body=%q", invokeRec.Code, invokeRec.Body.String())
	}

	group := "/aws/lambda/" + fnName
	streams, err := st.DescribeLogStreams(testAccountID, group, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(streams) == 0 {
		t.Fatalf("expected log stream under %s", group)
	}
	events, err := st.GetLogEvents(testAccountID, group, streams[0].LogStreamName, 0, 0, true, 50)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) < 3 {
		t.Fatalf("expected at least START/END/REPORT events, got %d", len(events))
	}
	var joined strings.Builder
	for _, ev := range events {
		joined.WriteString(ev.Message)
		joined.WriteByte('\n')
	}
	text := joined.String()
	if !strings.Contains(text, "START RequestId:") {
		t.Fatalf("missing START line: %q", text)
	}
	if !strings.Contains(text, "END RequestId:") {
		t.Fatalf("missing END line: %q", text)
	}
	if !strings.Contains(text, "REPORT RequestId:") {
		t.Fatalf("missing REPORT line: %q", text)
	}
}

func TestLambdaESMPollWritesInvokeAudit(t *testing.T) {
	srv, _, auditDir := newTestServerStore(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	srv.SetLambdaInvokeHookForTest(func(_ context.Context, _, _ string, _ store.LambdaFunction, _, eventJSON string) ([]byte, error) {
		return []byte(eventJSON), nil
	})

	mustCreateIAMRole(t, handler, "esm-audit-role", lambdaTrustOK, now)
	roleARN := "arn:aws:iam::" + testAccountID + ":role/esm-audit-role"
	createRec := mustLambdaJSON(t, handler, "CreateFunction", map[string]any{
		"FunctionName": "esm-audit-fn",
		"Runtime":      "python3.12",
		"Role":         roleARN,
		"Handler":      "app.handler",
		"Code":         map[string]any{"ZipFile": testLambdaZipB64(t)},
	}, now)
	if createRec.Code != http.StatusOK {
		t.Fatalf("CreateFunction status=%d body=%q", createRec.Code, createRec.Body.String())
	}

	qRec := mustSQSJSON(t, handler, "CreateQueue", map[string]any{
		"QueueName": "esm-audit-q",
	}, now)
	if qRec.Code != http.StatusOK {
		t.Fatalf("CreateQueue status=%d body=%q", qRec.Code, qRec.Body.String())
	}
	var qOut map[string]any
	if err := json.Unmarshal(qRec.Body.Bytes(), &qOut); err != nil {
		t.Fatal(err)
	}
	queueURL, _ := qOut["QueueUrl"].(string)
	queueARN := store.QueueARN("us-east-1", testAccountID, "esm-audit-q")
	esmQueuePolicy := `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":{"Service":"lambda.amazonaws.com"},"Action":["sqs:ReceiveMessage","sqs:DeleteMessage"],"Resource":"*"}]}`
	attrRec := mustSQSJSON(t, handler, "SetQueueAttributes", map[string]any{
		"QueueUrl":   queueURL,
		"Attributes": map[string]string{"Policy": esmQueuePolicy},
	}, now)
	if attrRec.Code != http.StatusOK {
		t.Fatalf("SetQueueAttributes status=%d body=%q", attrRec.Code, attrRec.Body.String())
	}

	sendRec := mustSQSJSON(t, handler, "SendMessage", map[string]any{
		"QueueUrl":    queueURL,
		"MessageBody": `{"from":"esm"}`,
	}, now)
	if sendRec.Code != http.StatusOK {
		t.Fatalf("SendMessage status=%d body=%q", sendRec.Code, sendRec.Body.String())
	}

	esmRec := mustLambdaJSON(t, handler, "CreateEventSourceMapping", map[string]any{
		"FunctionName":   "esm-audit-fn",
		"EventSourceArn": queueARN,
		"BatchSize":      1,
		"Enabled":        true,
	}, now)
	if esmRec.Code != http.StatusOK {
		t.Fatalf("CreateEventSourceMapping status=%d body=%q", esmRec.Code, esmRec.Body.String())
	}
	var created map[string]any
	if err := json.Unmarshal(esmRec.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	esmUUID, _ := created["UUID"].(string)
	if esmUUID == "" {
		t.Fatalf("missing UUID in %v", created)
	}

	data, err := os.ReadFile(filepath.Join(auditDir, "events.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		if line == "" {
			continue
		}
		var ev map[string]any
		if err := json.Unmarshal([]byte(line), &ev); err != nil {
			t.Fatal(err)
		}
		if ev["eventName"] != "Invoke" {
			continue
		}
		params, _ := ev["requestParameters"].(map[string]any)
		if params == nil {
			continue
		}
		if params["functionName"] != "esm-audit-fn" {
			continue
		}
		if params["eventSourceMappingUUID"] != esmUUID {
			continue
		}
		found = true
		break
	}
	if !found {
		t.Fatalf("no Invoke audit with eventSourceMappingUUID=%s in %s", esmUUID, string(data))
	}
}
