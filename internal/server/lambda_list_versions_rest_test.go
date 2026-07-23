package server_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestLambdaListVersionsByFunctionREST(t *testing.T) {
	srv, _ := newTestServer(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	mustCreateIAMRole(t, handler, "lambda-versions-exec", lambdaTrustOK, now)
	roleARN := "arn:aws:iam::" + testAccountID + ":role/lambda-versions-exec"

	fnName := "rest-versions-fn"
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

	pubRec := mustLambdaJSON(t, handler, "PublishVersion", map[string]any{
		"FunctionName": fnName,
	}, now)
	if pubRec.Code != http.StatusOK {
		t.Fatalf("PublishVersion status=%d body=%q", pubRec.Code, pubRec.Body.String())
	}

	path := "/2015-03-31/functions/" + fnName + "/versions"
	req := mustNewRequest(t, http.MethodGet, "http://127.0.0.1:4566"+path, nil)
	signHeader(t, req, nil, testAccessKey, testSecret, testRegion, "lambda", now)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("ListVersionsByFunction REST status=%d body=%q", rec.Code, rec.Body.String())
	}
	var out map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	versions, _ := out["Versions"].([]any)
	if len(versions) == 0 {
		t.Fatalf("Versions empty body=%q", rec.Body.String())
	}
}
