package server_test

import (
	"archive/zip"
	"bytes"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

func mustLambdaJSON(t *testing.T, handler http.Handler, target string, payload map[string]any, now time.Time) *httptest.ResponseRecorder {
	t.Helper()
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	req := mustNewRequest(t, http.MethodPost, "http://127.0.0.1:4566/", raw)
	req.Header.Set("Content-Type", "application/x-amz-json-1.1")
	req.Header.Set("X-Amz-Target", "AWSLambda."+target)
	signHeader(t, req, raw, testAccessKey, testSecret, testRegion, "lambda", now)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}

func mustCreateIAMRole(t *testing.T, handler http.Handler, roleName, trust string, now time.Time) {
	t.Helper()
	body := []byte("Action=CreateRole&Version=2010-05-08&RoleName=" + url.QueryEscape(roleName) +
		"&AssumeRolePolicyDocument=" + url.QueryEscape(trust))
	req := mustNewRequest(t, http.MethodPost, "http://127.0.0.1:4566/", body)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	signHeader(t, req, body, testAccessKey, testSecret, testRegion, "iam", now)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("CreateRole status=%d body=%q", rec.Code, rec.Body.String())
	}
}

func testLambdaZipB64(t *testing.T) string {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	w, err := zw.Create("app.py")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.Write([]byte("def handler(event, context):\n    return event\n")); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return base64.StdEncoding.EncodeToString(buf.Bytes())
}

const lambdaTrustOK = `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":{"Service":"lambda.amazonaws.com"},"Action":"sts:AssumeRole"}]}`

const lambdaTrustBad = `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":{"AWS":"arn:aws:iam::000000000001:root"},"Action":"sts:AssumeRole"}]}`

func TestLambdaCreateFunctionPassRoleDeny(t *testing.T) {
	srv, _ := newTestServer(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	mustCreateIAMRole(t, handler, "no-lambda-trust", lambdaTrustBad, now)
	roleARN := "arn:aws:iam::" + testAccountID + ":role/no-lambda-trust"

	rec := mustLambdaJSON(t, handler, "CreateFunction", map[string]any{
		"FunctionName": "denied-fn",
		"Runtime":      "python3.12",
		"Role":         roleARN,
		"Handler":      "app.handler",
		"Code": map[string]any{
			"ZipFile": testLambdaZipB64(t),
		},
	}, now)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("CreateFunction status=%d want 403 body=%q", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "AccessDeniedException") {
		t.Fatalf("expected AccessDeniedException in %q", rec.Body.String())
	}
}

func TestLambdaCreateGetWithoutCompute(t *testing.T) {
	srv, _ := newTestServer(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	mustCreateIAMRole(t, handler, "lambda-exec", lambdaTrustOK, now)
	roleARN := "arn:aws:iam::" + testAccountID + ":role/lambda-exec"

	createRec := mustLambdaJSON(t, handler, "CreateFunction", map[string]any{
		"FunctionName": "hello-lab",
		"Runtime":      "python3.12",
		"Role":         roleARN,
		"Handler":      "app.handler",
		"Timeout":      10,
		"MemorySize":   128,
		"Environment": map[string]any{
			"Variables": map[string]any{"STAGE": "lab"},
		},
		"Code": map[string]any{
			"ZipFile": testLambdaZipB64(t),
		},
	}, now)
	if createRec.Code != http.StatusOK {
		t.Fatalf("CreateFunction status=%d body=%q", createRec.Code, createRec.Body.String())
	}
	var created map[string]any
	if err := json.Unmarshal(createRec.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	if created["FunctionName"] != "hello-lab" {
		t.Fatalf("FunctionName=%v", created["FunctionName"])
	}
	wantARN := "arn:aws:lambda:us-east-1:" + testAccountID + ":function:hello-lab"
	if created["FunctionArn"] != wantARN {
		t.Fatalf("FunctionArn=%v want %s", created["FunctionArn"], wantARN)
	}

	getRec := mustLambdaJSON(t, handler, "GetFunction", map[string]any{
		"FunctionName": "hello-lab",
	}, now)
	if getRec.Code != http.StatusOK {
		t.Fatalf("GetFunction status=%d body=%q", getRec.Code, getRec.Body.String())
	}
	var got map[string]any
	if err := json.Unmarshal(getRec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	cfg, _ := got["Configuration"].(map[string]any)
	if cfg == nil || cfg["FunctionName"] != "hello-lab" {
		t.Fatalf("GetFunction body=%q", getRec.Body.String())
	}

	invokeRec := mustLambdaJSON(t, handler, "Invoke", map[string]any{
		"FunctionName": "hello-lab",
		"Payload":      `{"ping":true}`,
	}, now)
	if invokeRec.Code != http.StatusServiceUnavailable {
		t.Fatalf("Invoke status=%d want 503 body=%q", invokeRec.Code, invokeRec.Body.String())
	}
	if !strings.Contains(invokeRec.Body.String(), "compute unavailable") {
		t.Fatalf("expected compute unavailable in %q", invokeRec.Body.String())
	}
}
