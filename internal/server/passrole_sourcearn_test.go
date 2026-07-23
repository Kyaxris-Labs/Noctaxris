package server_test

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

func trustWithSourceArn(service, sourceARN string) string {
	return `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":{"Service":"` + service + `"},"Action":"sts:AssumeRole","Condition":{"ArnEquals":{"aws:SourceArn":"` + sourceARN + `"}}}]}`
}

func TestSchedulerCreateSchedulePassRoleSourceArn(t *testing.T) {
	srv, _ := newTestServer(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	name := "srcarn-sched"
	schedARN := store.ScheduleARN(store.DefaultSchedulerRegion, testAccountID, store.DefaultScheduleGroup, name)
	queueARN := "arn:aws:sqs:us-east-1:" + testAccountID + ":srcarn-sched-q"

	mustCreateIAMRole(t, handler, "sched-srcarn-ok", trustWithSourceArn("scheduler.amazonaws.com", schedARN), now)
	mustCreateIAMRole(t, handler, "sched-srcarn-bad", trustWithSourceArn("scheduler.amazonaws.com",
		store.ScheduleARN(store.DefaultSchedulerRegion, testAccountID, store.DefaultScheduleGroup, "other")), now)
	okARN := "arn:aws:iam::" + testAccountID + ":role/sched-srcarn-ok"
	badARN := "arn:aws:iam::" + testAccountID + ":role/sched-srcarn-bad"

	deny := mustSchedulerJSON(t, handler, "CreateSchedule", map[string]any{
		"Name":               name + "-deny",
		"ScheduleExpression": "rate(1 minutes)",
		"FlexibleTimeWindow": map[string]any{"Mode": "OFF"},
		"Target":             map[string]any{"Arn": queueARN, "RoleArn": badARN},
	}, now)
	if deny.Code != http.StatusForbidden {
		t.Fatalf("wrong SourceArn status=%d want 403 body=%q", deny.Code, deny.Body.String())
	}

	allow := mustSchedulerJSON(t, handler, "CreateSchedule", map[string]any{
		"Name":               name,
		"ScheduleExpression": "rate(1 minutes)",
		"FlexibleTimeWindow": map[string]any{"Mode": "OFF"},
		"Target":             map[string]any{"Arn": queueARN, "RoleArn": okARN},
	}, now)
	if allow.Code != http.StatusOK {
		t.Fatalf("matching SourceArn status=%d body=%q", allow.Code, allow.Body.String())
	}
}

func TestPipesCreatePipePassRoleSourceArn(t *testing.T) {
	srv, st, _ := newTestServerStore(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	src, err := st.CreateQueue(testAccountID, "us-east-1", "127.0.0.1:4566", "pipe-srcarn-src", nil)
	if err != nil {
		t.Fatal(err)
	}
	dst, err := st.CreateQueue(testAccountID, "us-east-1", "127.0.0.1:4566", "pipe-srcarn-dst", nil)
	if err != nil {
		t.Fatal(err)
	}

	name := "srcarn-pipe"
	pipeARN := store.PipeARN(store.DefaultPipesRegion, testAccountID, name)
	mustCreateIAMRole(t, handler, "pipes-srcarn-ok", trustWithSourceArn("pipes.amazonaws.com", pipeARN), now)
	mustCreateIAMRole(t, handler, "pipes-srcarn-bad", trustWithSourceArn("pipes.amazonaws.com",
		store.PipeARN(store.DefaultPipesRegion, testAccountID, "other")), now)
	okARN := "arn:aws:iam::" + testAccountID + ":role/pipes-srcarn-ok"
	badARN := "arn:aws:iam::" + testAccountID + ":role/pipes-srcarn-bad"

	deny := mustPipesJSON(t, handler, "CreatePipe", map[string]any{
		"Name": name + "-deny", "Source": src.QueueARN, "Target": dst.QueueARN, "RoleArn": badARN,
	}, now)
	if deny.Code != http.StatusForbidden {
		t.Fatalf("wrong SourceArn status=%d want 403 body=%q", deny.Code, deny.Body.String())
	}

	allow := mustPipesJSON(t, handler, "CreatePipe", map[string]any{
		"Name": name, "Source": src.QueueARN, "Target": dst.QueueARN, "RoleArn": okARN,
	}, now)
	if allow.Code != http.StatusOK {
		t.Fatalf("matching SourceArn status=%d body=%q", allow.Code, allow.Body.String())
	}
}

func TestSecretsRotatePassRoleSourceArn(t *testing.T) {
	srv, _, _ := newTestServerStore(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	srv.SetLambdaInvokeHookForTest(func(_ context.Context, _, _ string, _ store.LambdaFunction, _, _ string) ([]byte, error) {
		return []byte(`{"ok":true}`), nil
	})

	createSec := mustSecretsJSON(t, handler, "CreateSecret", map[string]any{
		"Name": "srcarn-rot-secret", "SecretString": "before",
	}, now)
	if createSec.Code != http.StatusOK {
		t.Fatalf("CreateSecret status=%d body=%q", createSec.Code, createSec.Body.String())
	}
	var secOut map[string]any
	if err := json.Unmarshal(createSec.Body.Bytes(), &secOut); err != nil {
		t.Fatal(err)
	}
	secretARN, _ := secOut["ARN"].(string)
	if secretARN == "" {
		t.Fatalf("missing secret ARN: %s", createSec.Body.String())
	}

	okTrust := `{"Version":"2012-10-17","Statement":[` +
		`{"Effect":"Allow","Principal":{"Service":"lambda.amazonaws.com"},"Action":"sts:AssumeRole"},` +
		`{"Effect":"Allow","Principal":{"Service":"secretsmanager.amazonaws.com"},"Action":"sts:AssumeRole",` +
		`"Condition":{"ArnEquals":{"aws:SourceArn":"` + secretARN + `"}}}]}`
	badTrust := `{"Version":"2012-10-17","Statement":[` +
		`{"Effect":"Allow","Principal":{"Service":"lambda.amazonaws.com"},"Action":"sts:AssumeRole"},` +
		`{"Effect":"Allow","Principal":{"Service":"secretsmanager.amazonaws.com"},"Action":"sts:AssumeRole",` +
		`"Condition":{"ArnEquals":{"aws:SourceArn":"arn:aws:secretsmanager:us-east-1:` + testAccountID + `:secret:other-abcdef"}}}]}`

	mustCreateIAMRole(t, handler, "sec-srcarn-ok", okTrust, now)
	mustCreateIAMRole(t, handler, "sec-srcarn-bad", badTrust, now)
	okRole := "arn:aws:iam::" + testAccountID + ":role/sec-srcarn-ok"
	badRole := "arn:aws:iam::" + testAccountID + ":role/sec-srcarn-bad"

	createBadFn := mustLambdaJSON(t, handler, "CreateFunction", map[string]any{
		"FunctionName": "srcarn-rot-bad-fn",
		"Runtime":      "python3.12",
		"Role":         badRole,
		"Handler":      "app.handler",
		"Code":         map[string]any{"ZipFile": testLambdaZipB64(t)},
	}, now)
	if createBadFn.Code != http.StatusOK {
		t.Fatalf("CreateFunction bad role status=%d body=%q", createBadFn.Code, createBadFn.Body.String())
	}
	var badFnOut map[string]any
	_ = json.Unmarshal(createBadFn.Body.Bytes(), &badFnOut)
	badFnARN, _ := badFnOut["FunctionArn"].(string)

	deny := mustSecretsJSON(t, handler, "RotateSecret", map[string]any{
		"SecretId": "srcarn-rot-secret", "RotationLambdaARN": badFnARN,
		"ClientRequestToken": "srcarn-deny-tok",
	}, now)
	if deny.Code != http.StatusForbidden {
		t.Fatalf("wrong SourceArn RotateSecret status=%d want 403 body=%q", deny.Code, deny.Body.String())
	}

	createOkFn := mustLambdaJSON(t, handler, "CreateFunction", map[string]any{
		"FunctionName": "srcarn-rot-ok-fn",
		"Runtime":      "python3.12",
		"Role":         okRole,
		"Handler":      "app.handler",
		"Code":         map[string]any{"ZipFile": testLambdaZipB64(t)},
	}, now)
	if createOkFn.Code != http.StatusOK {
		t.Fatalf("CreateFunction ok role status=%d body=%q", createOkFn.Code, createOkFn.Body.String())
	}
	var okFnOut map[string]any
	_ = json.Unmarshal(createOkFn.Body.Bytes(), &okFnOut)
	okFnARN, _ := okFnOut["FunctionArn"].(string)

	allow := mustSecretsJSON(t, handler, "RotateSecret", map[string]any{
		"SecretId": "srcarn-rot-secret", "RotationLambdaARN": okFnARN,
		"ClientRequestToken": "srcarn-ok-tok",
	}, now)
	if allow.Code != http.StatusOK {
		t.Fatalf("matching SourceArn RotateSecret status=%d body=%q", allow.Code, allow.Body.String())
	}
}

func TestAPIGatewayCredentialsArnPassRoleSourceArn(t *testing.T) {
	srv, _ := newTestServer(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	apiRec := mustJSONTarget(t, handler, "ApiGatewayV2.CreateApi", "apigateway", map[string]any{
		"Name": "srcarn-api", "ProtocolType": "HTTP",
	}, now)
	if apiRec.Code != http.StatusOK {
		t.Fatalf("CreateApi status=%d body=%q", apiRec.Code, apiRec.Body.String())
	}
	var apiResp map[string]any
	_ = json.Unmarshal(apiRec.Body.Bytes(), &apiResp)
	apiID, _ := apiResp["ApiId"].(string)
	if apiID == "" {
		t.Fatalf("missing ApiId: %s", apiRec.Body.String())
	}
	apiARN := "arn:aws:apigateway:" + store.DefaultAPIGatewayRegion + "::/apis/" + apiID
	lambdaARN := "arn:aws:lambda:us-east-1:" + testAccountID + ":function:unused"

	mustCreateIAMRole(t, handler, "apigw-srcarn-ok", trustWithSourceArn("apigateway.amazonaws.com", apiARN), now)
	mustCreateIAMRole(t, handler, "apigw-srcarn-bad", trustWithSourceArn("apigateway.amazonaws.com",
		"arn:aws:apigateway:"+store.DefaultAPIGatewayRegion+"::/apis/other"), now)
	okARN := "arn:aws:iam::" + testAccountID + ":role/apigw-srcarn-ok"
	badARN := "arn:aws:iam::" + testAccountID + ":role/apigw-srcarn-bad"

	deny := mustJSONTarget(t, handler, "ApiGatewayV2.CreateIntegration", "apigateway", map[string]any{
		"ApiId": apiID, "IntegrationType": "AWS_PROXY", "IntegrationUri": lambdaARN,
		"CredentialsArn": badARN,
	}, now)
	if deny.Code != http.StatusForbidden {
		t.Fatalf("wrong SourceArn status=%d want 403 body=%q", deny.Code, deny.Body.String())
	}

	allow := mustJSONTarget(t, handler, "ApiGatewayV2.CreateIntegration", "apigateway", map[string]any{
		"ApiId": apiID, "IntegrationType": "AWS_PROXY", "IntegrationUri": lambdaARN,
		"CredentialsArn": okARN,
	}, now)
	if allow.Code != http.StatusOK {
		t.Fatalf("matching SourceArn status=%d body=%q", allow.Code, allow.Body.String())
	}
	if !strings.Contains(allow.Body.String(), okARN) {
		t.Fatalf("response missing CredentialsArn: %s", allow.Body.String())
	}
}
