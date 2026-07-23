package server_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestLambdaGetFunctionCodeSigningConfigREST20200630(t *testing.T) {
	srv, _ := newTestServer(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	mustCreateIAMRole(t, handler, "lambda-csign-exec", lambdaTrustOK, now)
	roleARN := "arn:aws:iam::" + testAccountID + ":role/lambda-csign-exec"

	fnName := "rest-csign-fn"
	createRec := mustLambdaJSON(t, handler, "CreateFunction", map[string]any{
		"FunctionName": fnName,
		"Runtime":      "python3.12",
		"Role":         roleARN,
		"Handler":      "app.handler",
		"Code": map[string]any{
			"ZipFile": testLambdaZipB64(t),
		},
	}, now)
	if createRec.Code != http.StatusOK {
		t.Fatalf("CreateFunction status=%d body=%q", createRec.Code, createRec.Body.String())
	}

	// AWS CLI / Terraform provider use the 2020-06-30 REST date for this action.
	path := "/2020-06-30/functions/" + fnName + "/code-signing-config"
	req := mustNewRequest(t, http.MethodGet, "http://127.0.0.1:4566"+path, nil)
	signHeader(t, req, nil, testAccessKey, testSecret, testRegion, "lambda", now)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("GetFunctionCodeSigningConfig REST status=%d want 200 body=%q", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if strings.Contains(body, "NoSuchBucket") || strings.HasPrefix(strings.TrimSpace(body), "<") {
		t.Fatalf("expected Lambda JSON success, got S3/XML body=%q", body)
	}
	var out map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("json: %v body=%q", err, body)
	}
	if out["FunctionName"] != fnName {
		t.Fatalf("FunctionName=%v want %q", out["FunctionName"], fnName)
	}
	if _, ok := out["CodeSigningConfigArn"]; ok {
		t.Fatalf("lab response must omit CodeSigningConfigArn, got %q", body)
	}
}
