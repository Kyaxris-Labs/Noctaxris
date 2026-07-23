package server_test

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"
)

func TestLambdaDestinationConfigNestedObject(t *testing.T) {
	srv, _ := newTestServer(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	mustCreateIAMRole(t, handler, "dest-role", lambdaTrustOK, now)
	roleARN := "arn:aws:iam::" + testAccountID + ":role/dest-role"
	destARN := "arn:aws:sqs:us-east-1:" + testAccountID + ":dest-q"

	createRec := mustLambdaJSON(t, handler, "CreateFunction", map[string]any{
		"FunctionName": "dest-fn",
		"Runtime":      "python3.12",
		"Role":         roleARN,
		"Handler":      "app.handler",
		"Code":         map[string]any{"ZipFile": testLambdaZipB64(t)},
		"DestinationConfig": map[string]any{
			"OnFailure": map[string]any{"Destination": destARN},
			"OnSuccess": map[string]any{"Destination": destARN + "-ok"},
		},
	}, now)
	if createRec.Code != http.StatusOK {
		t.Fatalf("CreateFunction status=%d body=%q", createRec.Code, createRec.Body.String())
	}
	var created map[string]any
	if err := json.Unmarshal(createRec.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	dc, _ := created["DestinationConfig"].(map[string]any)
	onFail, _ := dc["OnFailure"].(map[string]any)
	if onFail["Destination"] != destARN {
		t.Fatalf("OnFailure=%v", dc)
	}
	onOK, _ := dc["OnSuccess"].(map[string]any)
	if onOK["Destination"] != destARN+"-ok" {
		t.Fatalf("OnSuccess=%v", dc)
	}

	putRec := mustLambdaJSON(t, handler, "PutFunctionEventInvokeConfig", map[string]any{
		"FunctionName": "dest-fn",
		"DestinationConfig": map[string]any{
			"OnFailure": map[string]any{"Destination": destARN + "-2"},
		},
	}, now)
	if putRec.Code != http.StatusOK {
		t.Fatalf("PutFunctionEventInvokeConfig status=%d body=%q", putRec.Code, putRec.Body.String())
	}
	var putBody map[string]any
	if err := json.Unmarshal(putRec.Body.Bytes(), &putBody); err != nil {
		t.Fatal(err)
	}
	putDC, _ := putBody["DestinationConfig"].(map[string]any)
	putFail, _ := putDC["OnFailure"].(map[string]any)
	if putFail["Destination"] != destARN+"-2" {
		t.Fatalf("put OnFailure=%v", putBody)
	}
}
