package server_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"
)

func TestLambdaListTags(t *testing.T) {
	srv, st, _ := newTestServerStore(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	mustCreateIAMRole(t, handler, "lambda-tags-exec", lambdaTrustOK, now)
	roleARN := "arn:aws:iam::" + testAccountID + ":role/lambda-tags-exec"

	createRec := mustLambdaJSON(t, handler, "CreateFunction", map[string]any{
		"FunctionName": "tagged-fn",
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
	fnARN := "arn:aws:lambda:us-east-1:" + testAccountID + ":function:tagged-fn"

	listEmpty := mustLambdaJSON(t, handler, "ListTags", map[string]any{
		"Resource": fnARN,
	}, now)
	if listEmpty.Code != http.StatusOK {
		t.Fatalf("ListTags status=%d body=%q", listEmpty.Code, listEmpty.Body.String())
	}
	var emptyOut map[string]any
	if err := json.Unmarshal(listEmpty.Body.Bytes(), &emptyOut); err != nil {
		t.Fatal(err)
	}
	if tags, _ := emptyOut["Tags"].(map[string]any); len(tags) != 0 {
		t.Fatalf("Tags=%v want empty", emptyOut["Tags"])
	}

	if _, err := st.TagResources(testAccountID, []string{fnARN}, map[string]string{"env": "lab"}); err != nil {
		t.Fatal(err)
	}

	listRec := mustLambdaJSON(t, handler, "ListTags", map[string]any{
		"Resource": fnARN,
	}, now)
	if listRec.Code != http.StatusOK {
		t.Fatalf("ListTags#2 status=%d body=%q", listRec.Code, listRec.Body.String())
	}
	var listOut map[string]any
	if err := json.Unmarshal(listRec.Body.Bytes(), &listOut); err != nil {
		t.Fatal(err)
	}
	tags, _ := listOut["Tags"].(map[string]any)
	if tags["env"] != "lab" {
		t.Fatalf("Tags=%v", listOut["Tags"])
	}

	// REST path used by AWS CLI / Terraform provider.
	path := "/2017-03-31/tags/" + url.PathEscape(fnARN)
	req := mustNewRequest(t, http.MethodGet, "http://127.0.0.1:4566"+path, nil)
	signHeader(t, req, nil, testAccessKey, testSecret, testRegion, "lambda", now)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("ListTags REST status=%d body=%q", rec.Code, rec.Body.String())
	}
	var restOut map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &restOut); err != nil {
		t.Fatal(err)
	}
	restTags, _ := restOut["Tags"].(map[string]any)
	if restTags["env"] != "lab" {
		t.Fatalf("REST Tags=%v", restOut["Tags"])
	}
}
