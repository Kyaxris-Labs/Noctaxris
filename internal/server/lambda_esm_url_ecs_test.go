package server_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

func TestLambdaEventSourceMappingCRUD(t *testing.T) {
	srv, _ := newTestServer(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	mustCreateIAMRole(t, handler, "lambda-esm-role", lambdaTrustOK, now)
	roleARN := "arn:aws:iam::" + testAccountID + ":role/lambda-esm-role"
	createRec := mustLambdaJSON(t, handler, "CreateFunction", map[string]any{
		"FunctionName": "esm-handler",
		"Runtime":      "python3.12",
		"Role":         roleARN,
		"Handler":      "app.handler",
		"Code":         map[string]any{"ZipFile": testLambdaZipB64(t)},
	}, now)
	if createRec.Code != http.StatusOK {
		t.Fatalf("CreateFunction status=%d body=%q", createRec.Code, createRec.Body.String())
	}

	qRec := mustSQSJSON(t, handler, "CreateQueue", map[string]any{
		"QueueName": "esm-handler-q",
	}, now)
	if qRec.Code != http.StatusOK {
		t.Fatalf("CreateQueue status=%d body=%q", qRec.Code, qRec.Body.String())
	}
	var qOut map[string]any
	if err := json.Unmarshal(qRec.Body.Bytes(), &qOut); err != nil {
		t.Fatal(err)
	}
	queueURL, _ := qOut["QueueUrl"].(string)
	if queueURL == "" {
		t.Fatalf("missing QueueUrl in %q", qRec.Body.String())
	}
	queueARN := store.QueueARN("us-east-1", testAccountID, "esm-handler-q")
	esmQueuePolicy := `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":{"Service":"lambda.amazonaws.com"},"Action":["sqs:ReceiveMessage","sqs:DeleteMessage"],"Resource":"*"}]}`
	attrRec := mustSQSJSON(t, handler, "SetQueueAttributes", map[string]any{
		"QueueUrl":   queueURL,
		"Attributes": map[string]string{"Policy": esmQueuePolicy},
	}, now)
	if attrRec.Code != http.StatusOK {
		t.Fatalf("SetQueueAttributes status=%d body=%q", attrRec.Code, attrRec.Body.String())
	}

	rec := mustLambdaJSON(t, handler, "CreateEventSourceMapping", map[string]any{
		"FunctionName":   "esm-handler",
		"EventSourceArn": queueARN,
		"BatchSize":      5,
		"Enabled":        true,
	}, now)
	if rec.Code != http.StatusOK {
		t.Fatalf("CreateEventSourceMapping status=%d body=%q", rec.Code, rec.Body.String())
	}
	var created map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	uuidStr, _ := created["UUID"].(string)
	if uuidStr == "" {
		t.Fatalf("missing UUID in %v", created)
	}

	getRec := mustLambdaJSON(t, handler, "GetEventSourceMapping", map[string]any{"UUID": uuidStr}, now)
	if getRec.Code != http.StatusOK {
		t.Fatalf("GetEventSourceMapping status=%d", getRec.Code)
	}
	listRec := mustLambdaJSON(t, handler, "ListEventSourceMappings", map[string]any{
		"FunctionName": "esm-handler",
	}, now)
	if listRec.Code != http.StatusOK {
		t.Fatalf("ListEventSourceMappings status=%d", listRec.Code)
	}
	updRec := mustLambdaJSON(t, handler, "UpdateEventSourceMapping", map[string]any{
		"UUID":    uuidStr,
		"Enabled": false,
	}, now)
	if updRec.Code != http.StatusOK {
		t.Fatalf("UpdateEventSourceMapping status=%d body=%q", updRec.Code, updRec.Body.String())
	}
	delRec := mustLambdaJSON(t, handler, "DeleteEventSourceMapping", map[string]any{"UUID": uuidStr}, now)
	if delRec.Code != http.StatusOK {
		t.Fatalf("DeleteEventSourceMapping status=%d", delRec.Code)
	}
}

func TestLambdaFunctionURLConfigAndNONEInvoke(t *testing.T) {
	srv, _ := newTestServer(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	mustCreateIAMRole(t, handler, "lambda-url-role", lambdaTrustOK, now)
	roleARN := "arn:aws:iam::" + testAccountID + ":role/lambda-url-role"
	createRec := mustLambdaJSON(t, handler, "CreateFunction", map[string]any{
		"FunctionName": "url-fn",
		"Runtime":      "python3.12",
		"Role":         roleARN,
		"Handler":      "app.handler",
		"Code":         map[string]any{"ZipFile": testLambdaZipB64(t)},
	}, now)
	if createRec.Code != http.StatusOK {
		t.Fatalf("CreateFunction status=%d body=%q", createRec.Code, createRec.Body.String())
	}

	urlRec := mustLambdaJSON(t, handler, "CreateFunctionUrlConfig", map[string]any{
		"FunctionName": "url-fn",
		"AuthType":     "NONE",
	}, now)
	if urlRec.Code != http.StatusOK {
		t.Fatalf("CreateFunctionUrlConfig status=%d body=%q", urlRec.Code, urlRec.Body.String())
	}
	var cfg map[string]any
	if err := json.Unmarshal(urlRec.Body.Bytes(), &cfg); err != nil {
		t.Fatal(err)
	}
	fnURL, _ := cfg["FunctionUrl"].(string)
	if !strings.Contains(fnURL, "/lambda-url/"+testAccountID+"/url-fn") {
		t.Fatalf("FunctionUrl=%q", fnURL)
	}

	getRec := mustLambdaJSON(t, handler, "GetFunctionUrlConfig", map[string]any{
		"FunctionName": "url-fn",
	}, now)
	if getRec.Code != http.StatusOK {
		t.Fatalf("GetFunctionUrlConfig status=%d", getRec.Code)
	}

	// AuthType NONE: no SigV4. Expect compute unavailable without DockerHost.
	req := httptest.NewRequest(http.MethodPost, "http://127.0.0.1:4566/lambda-url/"+testAccountID+"/url-fn", strings.NewReader(`{"ok":true}`))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("function URL invoke status=%d want 503 body=%q", rec.Code, rec.Body.String())
	}

	delRec := mustLambdaJSON(t, handler, "DeleteFunctionUrlConfig", map[string]any{
		"FunctionName": "url-fn",
	}, now)
	if delRec.Code != http.StatusNoContent && delRec.Code != http.StatusOK {
		t.Fatalf("DeleteFunctionUrlConfig status=%d", delRec.Code)
	}
}

func TestECSCreateServiceLite(t *testing.T) {
	srv, _ := newTestServer(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	mustCreateIAMRole(t, handler, "ecs-svc-task", ecsTrustOK, now)
	mustCreateIAMRole(t, handler, "ecs-svc-exec", ecsTrustOK, now)
	taskRole := "arn:aws:iam::" + testAccountID + ":role/ecs-svc-task"
	execRole := "arn:aws:iam::" + testAccountID + ":role/ecs-svc-exec"

	regRec := mustECSJSON(t, handler, "RegisterTaskDefinition", map[string]any{
		"family": "svc-td",
		"containerDefinitions": []map[string]any{
			{"name": "app", "image": "alpine:3.20", "essential": true},
		},
		"taskRoleArn":      taskRole,
		"executionRoleArn": execRole,
	}, now)
	if regRec.Code != http.StatusOK {
		t.Fatalf("RegisterTaskDefinition status=%d body=%q", regRec.Code, regRec.Body.String())
	}

	createRec := mustECSJSON(t, handler, "CreateService", map[string]any{
		"cluster":        "default",
		"serviceName":    "web",
		"taskDefinition": "svc-td",
		"desiredCount":   0,
	}, now)
	if createRec.Code != http.StatusOK {
		t.Fatalf("CreateService status=%d body=%q", createRec.Code, createRec.Body.String())
	}
	var created map[string]any
	if err := json.Unmarshal(createRec.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	svc, _ := created["service"].(map[string]any)
	if svc == nil || svc["serviceName"] != "web" {
		t.Fatalf("service=%v", created)
	}

	listRec := mustECSJSON(t, handler, "ListServices", map[string]any{"cluster": "default"}, now)
	if listRec.Code != http.StatusOK {
		t.Fatalf("ListServices status=%d", listRec.Code)
	}
	descRec := mustECSJSON(t, handler, "DescribeServices", map[string]any{
		"cluster":  "default",
		"services": []any{"web"},
	}, now)
	if descRec.Code != http.StatusOK {
		t.Fatalf("DescribeServices status=%d", descRec.Code)
	}
	updRec := mustECSJSON(t, handler, "UpdateService", map[string]any{
		"cluster":      "default",
		"service":      "web",
		"desiredCount": 0,
	}, now)
	if updRec.Code != http.StatusOK {
		t.Fatalf("UpdateService status=%d body=%q", updRec.Code, updRec.Body.String())
	}
	delRec := mustECSJSON(t, handler, "DeleteService", map[string]any{
		"cluster": "default",
		"service": "web",
	}, now)
	if delRec.Code != http.StatusOK {
		t.Fatalf("DeleteService status=%d body=%q", delRec.Code, delRec.Body.String())
	}
}
