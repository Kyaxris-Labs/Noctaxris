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

	"github.com/Kyaxris-Labs/Noctaxris/internal/config"
	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
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

func TestLambdaAsyncInvokeReturns202(t *testing.T) {
	srv, st, _ := newTestServerStore(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	mustCreateIAMRole(t, handler, "lambda-exec-async", lambdaTrustOK, now)
	roleARN := "arn:aws:iam::" + testAccountID + ":role/lambda-exec-async"
	fnName := "async-fn-202"

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

	asyncRec := mustLambdaJSON(t, handler, "Invoke", map[string]any{
		"FunctionName":   fnName,
		"InvocationType": "Event",
		"Payload":        `{"async":true}`,
	}, now)
	if asyncRec.Code != http.StatusAccepted {
		t.Fatalf("async Invoke status=%d want 202 body=%q", asyncRec.Code, asyncRec.Body.String())
	}
	var asyncBody map[string]any
	if err := json.Unmarshal(asyncRec.Body.Bytes(), &asyncBody); err != nil {
		t.Fatal(err)
	}
	if asyncBody["StatusCode"] != float64(202) {
		t.Fatalf("StatusCode=%v want 202", asyncBody["StatusCode"])
	}

	// Empty DockerHost processes the async worker inline before 202 returns.
	// Assert the job reached a terminal failed state (compute unavailable).
	job, err := st.LatestAsyncInvocation(testAccountID, fnName)
	if err != nil {
		t.Fatalf("LatestAsyncInvocation: %v", err)
	}
	if job.Status != "failed" {
		t.Fatalf("async job status=%q want failed last_error=%q", job.Status, job.LastError)
	}
	if !strings.Contains(job.LastError, "compute unavailable") {
		t.Fatalf("async last_error=%q want compute unavailable", job.LastError)
	}

	syncRec := mustLambdaJSON(t, handler, "Invoke", map[string]any{
		"FunctionName":   fnName,
		"InvocationType": "RequestResponse",
		"Payload":        `{"sync":true}`,
	}, now)
	if syncRec.Code != http.StatusServiceUnavailable {
		t.Fatalf("sync Invoke status=%d want 503 body=%q", syncRec.Code, syncRec.Body.String())
	}
}

func TestLambdaPublishVersionAndAliasQualifier(t *testing.T) {
	srv, _ := newTestServer(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	mustCreateIAMRole(t, handler, "lambda-exec", lambdaTrustOK, now)
	roleARN := "arn:aws:iam::" + testAccountID + ":role/lambda-exec"

	fnName := "versioned-fn"
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
	var published map[string]any
	if err := json.Unmarshal(pubRec.Body.Bytes(), &published); err != nil {
		t.Fatal(err)
	}
	if published["Version"] != "1" {
		t.Fatalf("Version=%v want 1", published["Version"])
	}

	aliasRec := mustLambdaJSON(t, handler, "CreateAlias", map[string]any{
		"FunctionName":    fnName,
		"Name":              "prod",
		"FunctionVersion":   "1",
		"Description":       "lab alias",
	}, now)
	if aliasRec.Code != http.StatusOK {
		t.Fatalf("CreateAlias status=%d body=%q", aliasRec.Code, aliasRec.Body.String())
	}

	getRec := mustLambdaJSON(t, handler, "GetFunction", map[string]any{
		"FunctionName": fnName + ":prod",
	}, now)
	if getRec.Code != http.StatusOK {
		t.Fatalf("GetFunction qualifier status=%d body=%q", getRec.Code, getRec.Body.String())
	}
	var got map[string]any
	if err := json.Unmarshal(getRec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	cfg, _ := got["Configuration"].(map[string]any)
	if cfg == nil || cfg["Version"] != "prod" {
		t.Fatalf("GetFunction qualifier body=%q", getRec.Body.String())
	}

	invokeRec := mustLambdaJSON(t, handler, "Invoke", map[string]any{
		"FunctionName": fnName + ":1",
		"Payload":      `{"ping":true}`,
	}, now)
	if invokeRec.Code != http.StatusServiceUnavailable {
		t.Fatalf("Invoke version status=%d want 503 body=%q", invokeRec.Code, invokeRec.Body.String())
	}
}

func testLambdaLayerZipB64(t *testing.T) string {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	w, err := zw.Create("python/layerlib.py")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.Write([]byte("LAYER_VALUE='from-layer'\n")); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return base64.StdEncoding.EncodeToString(buf.Bytes())
}

func testLambdaHandlerZipB64WithLayerImport(t *testing.T) string {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	w, err := zw.Create("app.py")
	if err != nil {
		t.Fatal(err)
	}
	body := "import layerlib\n\ndef handler(event, context):\n    return {'layer': layerlib.LAYER_VALUE}\n"
	if _, err := w.Write([]byte(body)); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return base64.StdEncoding.EncodeToString(buf.Bytes())
}

func TestLambdaPublishLayerAttachAndGet(t *testing.T) {
	srv, _ := newTestServer(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	mustCreateIAMRole(t, handler, "lambda-exec", lambdaTrustOK, now)
	roleARN := "arn:aws:iam::" + testAccountID + ":role/lambda-exec"

	pubRec := mustLambdaJSON(t, handler, "PublishLayerVersion", map[string]any{
		"LayerName": "shared-lib",
		"Content": map[string]any{
			"ZipFile": testLambdaLayerZipB64(t),
		},
	}, now)
	if pubRec.Code != http.StatusOK {
		t.Fatalf("PublishLayerVersion status=%d body=%q", pubRec.Code, pubRec.Body.String())
	}
	var published map[string]any
	if err := json.Unmarshal(pubRec.Body.Bytes(), &published); err != nil {
		t.Fatal(err)
	}
	layerARN, _ := published["LayerVersionArn"].(string)
	wantLayerARN := "arn:aws:lambda:us-east-1:" + testAccountID + ":layer:shared-lib:1"
	if layerARN != wantLayerARN {
		t.Fatalf("LayerVersionArn=%v want %s", published["LayerVersionArn"], wantLayerARN)
	}

	createRec := mustLambdaJSON(t, handler, "CreateFunction", map[string]any{
		"FunctionName": "layer-fn",
		"Runtime":      "python3.12",
		"Role":         roleARN,
		"Handler":      "app.handler",
		"Layers":       []any{layerARN},
		"Code": map[string]any{
			"ZipFile": testLambdaHandlerZipB64WithLayerImport(t),
		},
	}, now)
	if createRec.Code != http.StatusOK {
		t.Fatalf("CreateFunction status=%d body=%q", createRec.Code, createRec.Body.String())
	}
	var created map[string]any
	if err := json.Unmarshal(createRec.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	layers, ok := created["Layers"].([]any)
	if !ok || len(layers) != 1 || layers[0] != layerARN {
		t.Fatalf("Layers=%v want [%s]", created["Layers"], layerARN)
	}

	getRec := mustLambdaJSON(t, handler, "GetLayerVersion", map[string]any{
		"LayerName":     "shared-lib",
		"VersionNumber": 1,
	}, now)
	if getRec.Code != http.StatusOK {
		t.Fatalf("GetLayerVersion status=%d body=%q", getRec.Code, getRec.Body.String())
	}

	listRec := mustLambdaJSON(t, handler, "ListLayerVersions", map[string]any{
		"LayerName": "shared-lib",
	}, now)
	if listRec.Code != http.StatusOK {
		t.Fatalf("ListLayerVersions status=%d body=%q", listRec.Code, listRec.Body.String())
	}

	invokeRec := mustLambdaJSON(t, handler, "Invoke", map[string]any{
		"FunctionName": "layer-fn",
		"Payload":      `{"ping":true}`,
	}, now)
	if invokeRec.Code != http.StatusServiceUnavailable {
		t.Fatalf("Invoke status=%d want 503 without compute body=%q", invokeRec.Code, invokeRec.Body.String())
	}
}

func TestLambdaCreateFunctionImageWithoutCompute(t *testing.T) {
	srv, _ := newTestServer(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	mustCreateIAMRole(t, handler, "lambda-exec-img", lambdaTrustOK, now)
	roleARN := "arn:aws:iam::" + testAccountID + ":role/lambda-exec-img"
	imageURI := "public.ecr.aws/lambda/python:3.12"

	createRec := mustLambdaJSON(t, handler, "CreateFunction", map[string]any{
		"FunctionName": "img-lab",
		"PackageType":  "Image",
		"Role":         roleARN,
		"Handler":      "app.handler",
		"Code": map[string]any{
			"ImageUri": imageURI,
		},
	}, now)
	if createRec.Code != http.StatusOK {
		t.Fatalf("CreateFunction status=%d body=%q", createRec.Code, createRec.Body.String())
	}
	var created map[string]any
	if err := json.Unmarshal(createRec.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	if created["PackageType"] != "Image" {
		t.Fatalf("PackageType=%v", created["PackageType"])
	}

	getRec := mustLambdaJSON(t, handler, "GetFunction", map[string]any{
		"FunctionName": "img-lab",
	}, now)
	if getRec.Code != http.StatusOK {
		t.Fatalf("GetFunction status=%d body=%q", getRec.Code, getRec.Body.String())
	}
	var got map[string]any
	if err := json.Unmarshal(getRec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	cfg, _ := got["Configuration"].(map[string]any)
	if cfg == nil || cfg["PackageType"] != "Image" {
		t.Fatalf("Configuration=%v", got["Configuration"])
	}
	code, _ := got["Code"].(map[string]any)
	if code == nil || code["ImageUri"] != imageURI {
		t.Fatalf("Code=%v", got["Code"])
	}

	updateRec := mustLambdaJSON(t, handler, "UpdateFunctionCode", map[string]any{
		"FunctionName": "img-lab",
		"ImageUri":     "public.ecr.aws/lambda/python:3.12-updated",
	}, now)
	if updateRec.Code != http.StatusOK {
		t.Fatalf("UpdateFunctionCode status=%d body=%q", updateRec.Code, updateRec.Body.String())
	}

	invokeRec := mustLambdaJSON(t, handler, "Invoke", map[string]any{
		"FunctionName": "img-lab",
		"Payload":      `{"ping":true}`,
	}, now)
	if invokeRec.Code != http.StatusServiceUnavailable {
		t.Fatalf("Invoke status=%d want 503 without compute body=%q", invokeRec.Code, invokeRec.Body.String())
	}
}

func TestLambdaCreateFunctionRejectedRuntime(t *testing.T) {
	srv, _ := newTestServer(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	mustCreateIAMRole(t, handler, "lambda-exec-rt", lambdaTrustOK, now)
	roleARN := "arn:aws:iam::" + testAccountID + ":role/lambda-exec-rt"

	rec := mustLambdaJSON(t, handler, "CreateFunction", map[string]any{
		"FunctionName": "bad-runtime",
		"Runtime":      "python3.9",
		"Role":         roleARN,
		"Handler":      "app.handler",
		"Code": map[string]any{
			"ZipFile": testLambdaZipB64(t),
		},
	}, now)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("CreateFunction status=%d want 400 body=%q", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "python3.11") {
		t.Fatalf("body=%q", rec.Body.String())
	}
}

func TestLambdaCreateFunctionAcceptedRuntimes(t *testing.T) {
	srv, _ := newTestServer(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	mustCreateIAMRole(t, handler, "lambda-exec-rt2", lambdaTrustOK, now)
	roleARN := "arn:aws:iam::" + testAccountID + ":role/lambda-exec-rt2"
	zip := testLambdaZipB64(t)

	for _, tc := range []struct {
		name    string
		runtime string
	}{
		{"py311-fn", store.LambdaRuntimePython311},
		{"node20-fn", store.LambdaRuntimeNodejs20x},
	} {
		t.Run(tc.runtime, func(t *testing.T) {
			rec := mustLambdaJSON(t, handler, "CreateFunction", map[string]any{
				"FunctionName": tc.name,
				"Runtime":      tc.runtime,
				"Role":         roleARN,
				"Handler":      "app.handler",
				"Code": map[string]any{
					"ZipFile": zip,
				},
			}, now)
			if rec.Code != http.StatusOK {
				t.Fatalf("CreateFunction status=%d body=%q", rec.Code, rec.Body.String())
			}
			var created map[string]any
			if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil {
				t.Fatal(err)
			}
			if created["Runtime"] != tc.runtime {
				t.Fatalf("Runtime=%v", created["Runtime"])
			}
		})
	}
}

func TestLambdaUpdateFunctionConfigurationRejectedRuntime(t *testing.T) {
	srv, _ := newTestServer(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	mustCreateIAMRole(t, handler, "lambda-exec-rt3", lambdaTrustOK, now)
	roleARN := "arn:aws:iam::" + testAccountID + ":role/lambda-exec-rt3"

	createRec := mustLambdaJSON(t, handler, "CreateFunction", map[string]any{
		"FunctionName": "upd-runtime",
		"Runtime":      store.LambdaRuntimePython312,
		"Role":         roleARN,
		"Handler":      "app.handler",
		"Code": map[string]any{
			"ZipFile": testLambdaZipB64(t),
		},
	}, now)
	if createRec.Code != http.StatusOK {
		t.Fatalf("CreateFunction status=%d body=%q", createRec.Code, createRec.Body.String())
	}

	rec := mustLambdaJSON(t, handler, "UpdateFunctionConfiguration", map[string]any{
		"FunctionName": "upd-runtime",
		"Runtime":      "java11",
	}, now)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("UpdateFunctionConfiguration status=%d want 400 body=%q", rec.Code, rec.Body.String())
	}
}

func TestLambdaCreateFunctionRejectsReservedEnvKeys(t *testing.T) {
	srv, _ := newTestServer(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	mustCreateIAMRole(t, handler, "lambda-exec-env", lambdaTrustOK, now)
	roleARN := "arn:aws:iam::" + testAccountID + ":role/lambda-exec-env"

	rec := mustLambdaJSON(t, handler, "CreateFunction", map[string]any{
		"FunctionName": "reserved-env",
		"Runtime":      "python3.12",
		"Role":         roleARN,
		"Handler":      "app.handler",
		"Environment": map[string]any{
			"Variables": map[string]any{
				"AWS_ACCESS_KEY_ID": "AKIACLIENT",
				"STAGE":             "lab",
			},
		},
		"Code": map[string]any{
			"ZipFile": testLambdaZipB64(t),
		},
	}, now)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("CreateFunction status=%d want 400 body=%q", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "InvalidParameterValueException") {
		t.Fatalf("body=%q", rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "AWS_ACCESS_KEY_ID") {
		t.Fatalf("body missing reserved key detail: %q", rec.Body.String())
	}
}

func TestLambdaUpdateFunctionConfigurationRejectsReservedEnvKeys(t *testing.T) {
	srv, _ := newTestServer(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	mustCreateIAMRole(t, handler, "lambda-exec-env2", lambdaTrustOK, now)
	roleARN := "arn:aws:iam::" + testAccountID + ":role/lambda-exec-env2"

	createRec := mustLambdaJSON(t, handler, "CreateFunction", map[string]any{
		"FunctionName": "upd-reserved-env",
		"Runtime":      "python3.12",
		"Role":         roleARN,
		"Handler":      "app.handler",
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

	rec := mustLambdaJSON(t, handler, "UpdateFunctionConfiguration", map[string]any{
		"FunctionName": "upd-reserved-env",
		"Environment": map[string]any{
			"Variables": map[string]any{
				"AWS_ENDPOINT_URL": "http://evil.example",
			},
		},
	}, now)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("UpdateFunctionConfiguration status=%d want 400 body=%q", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "AWS_ENDPOINT_URL") {
		t.Fatalf("body=%q", rec.Body.String())
	}
}

func TestLambdaCreateFunctionClampsTimeoutAndMemory(t *testing.T) {
	srv, _ := newTestServer(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	mustCreateIAMRole(t, handler, "lambda-exec-clamp", lambdaTrustOK, now)
	roleARN := "arn:aws:iam::" + testAccountID + ":role/lambda-exec-clamp"

	rec := mustLambdaJSON(t, handler, "CreateFunction", map[string]any{
		"FunctionName": "clamp-limits",
		"Runtime":      "python3.12",
		"Role":         roleARN,
		"Handler":      "app.handler",
		"Timeout":      5000,
		"MemorySize":   64,
		"Code": map[string]any{
			"ZipFile": testLambdaZipB64(t),
		},
	}, now)
	if rec.Code != http.StatusOK {
		t.Fatalf("CreateFunction status=%d body=%q", rec.Code, rec.Body.String())
	}
	var created map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	if created["Timeout"] != float64(900) {
		t.Fatalf("Timeout=%v want 900", created["Timeout"])
	}
	if created["MemorySize"] != float64(128) {
		t.Fatalf("MemorySize=%v want 128", created["MemorySize"])
	}
}

func TestLambdaFunctionPolicyLifecycle(t *testing.T) {
	srv, _ := newTestServer(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	mustCreateIAMRole(t, handler, "policy-role", lambdaTrustOK, now)
	roleARN := "arn:aws:iam::" + testAccountID + ":role/policy-role"

	createRec := mustLambdaJSON(t, handler, "CreateFunction", map[string]any{
		"FunctionName": "policy-target",
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

	addRec := mustLambdaJSON(t, handler, "AddPermission", map[string]any{
		"FunctionName": "policy-target",
		"StatementId":  "root-invoke",
		"Action":       "lambda:InvokeFunction",
		"Principal":    "arn:aws:iam::" + testAccountID + ":root",
	}, now)
	if addRec.Code != http.StatusOK {
		t.Fatalf("AddPermission status=%d body=%q", addRec.Code, addRec.Body.String())
	}

	getPolRec := mustLambdaJSON(t, handler, "GetPolicy", map[string]any{
		"FunctionName": "policy-target",
	}, now)
	if getPolRec.Code != http.StatusOK {
		t.Fatalf("GetPolicy status=%d body=%q", getPolRec.Code, getPolRec.Body.String())
	}
	var polOut map[string]any
	if err := json.Unmarshal(getPolRec.Body.Bytes(), &polOut); err != nil {
		t.Fatal(err)
	}
	gotPolicy, _ := polOut["Policy"].(string)
	if !strings.Contains(gotPolicy, "root-invoke") {
		t.Fatalf("Policy=%q missing root-invoke", gotPolicy)
	}

	rmRec := mustLambdaJSON(t, handler, "RemovePermission", map[string]any{
		"FunctionName": "policy-target",
		"StatementId":  "root-invoke",
	}, now)
	if rmRec.Code != http.StatusOK {
		t.Fatalf("RemovePermission status=%d body=%q", rmRec.Code, rmRec.Body.String())
	}

	getPolRec2 := mustLambdaJSON(t, handler, "GetPolicy", map[string]any{
		"FunctionName": "policy-target",
	}, now)
	if getPolRec2.Code != http.StatusNotFound {
		t.Fatalf("GetPolicy after remove status=%d want 404 body=%q", getPolRec2.Code, getPolRec2.Body.String())
	}
}

func TestLambdaResourcePolicyAloneGrantsInvoke(t *testing.T) {
	srv, st, _ := newTestServerStore(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	mustCreateIAMRole(t, handler, "invoke-policy-role", lambdaTrustOK, now)
	roleARN := "arn:aws:iam::" + testAccountID + ":role/invoke-policy-role"

	createRec := mustLambdaJSON(t, handler, "CreateFunction", map[string]any{
		"FunctionName": "shared-fn",
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

	_, guestARN, err := st.CreateUser(testAccountID, "lambda-guest")
	if err != nil {
		t.Fatal(err)
	}
	guestAKID, guestSecret, err := st.CreateUserAccessKey(testAccountID, "lambda-guest")
	if err != nil {
		t.Fatal(err)
	}

	addRec := mustLambdaJSON(t, handler, "AddPermission", map[string]any{
		"FunctionName": "shared-fn",
		"StatementId":  "guest-invoke",
		"Action":       "lambda:InvokeFunction",
		"Principal":    guestARN,
	}, now)
	if addRec.Code != http.StatusOK {
		t.Fatalf("AddPermission status=%d body=%q", addRec.Code, addRec.Body.String())
	}

	invokeRec := mustLambdaJSONWithCreds(t, handler, "Invoke", map[string]any{
		"FunctionName": "shared-fn",
		"Payload":      `{"ping":true}`,
	}, guestAKID, guestSecret, now)
	if invokeRec.Code == http.StatusForbidden {
		t.Fatalf("Invoke denied by identity-only authz body=%q", invokeRec.Body.String())
	}
	if invokeRec.Code != http.StatusServiceUnavailable {
		t.Fatalf("Invoke status=%d want 503 (compute unavailable) body=%q", invokeRec.Code, invokeRec.Body.String())
	}
	if !strings.Contains(invokeRec.Body.String(), "compute unavailable") {
		t.Fatalf("expected compute unavailable in %q", invokeRec.Body.String())
	}
}

func TestLambdaInvokeDeniedWithoutPolicyOrIdentity(t *testing.T) {
	srv, st, _ := newTestServerStore(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	mustCreateIAMRole(t, handler, "locked-role", lambdaTrustOK, now)
	roleARN := "arn:aws:iam::" + testAccountID + ":role/locked-role"

	createRec := mustLambdaJSON(t, handler, "CreateFunction", map[string]any{
		"FunctionName": "locked-fn",
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

	_, _, err := st.CreateUser(testAccountID, "no-access")
	if err != nil {
		t.Fatal(err)
	}
	denyAKID, denySecret, err := st.CreateUserAccessKey(testAccountID, "no-access")
	if err != nil {
		t.Fatal(err)
	}

	invokeRec := mustLambdaJSONWithCreds(t, handler, "Invoke", map[string]any{
		"FunctionName": "locked-fn",
		"Payload":      `{}`,
	}, denyAKID, denySecret, now)
	if invokeRec.Code != http.StatusForbidden {
		t.Fatalf("Invoke status=%d want 403 body=%q", invokeRec.Code, invokeRec.Body.String())
	}
}

func TestLambdaZipInvokeMicroVMOptInFailsClosed(t *testing.T) {
	srv, _, _ := newTestServerStoreWith(t, func(cfg *config.Config) {
		cfg.ComputeRuntime = "microvm"
	})
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	mustCreateIAMRole(t, handler, "lambda-exec-microvm", lambdaTrustOK, now)
	roleARN := "arn:aws:iam::" + testAccountID + ":role/lambda-exec-microvm"

	createRec := mustLambdaJSON(t, handler, "CreateFunction", map[string]any{
		"FunctionName": "microvm-zip-fn",
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

	invokeRec := mustLambdaJSON(t, handler, "Invoke", map[string]any{
		"FunctionName": "microvm-zip-fn",
		"Payload":      `{"ping":true}`,
	}, now)
	if invokeRec.Code == http.StatusOK {
		t.Fatalf("expected microVM opt-in Invoke to fail closed, got 200")
	}
	body := invokeRec.Body.String()
	if strings.Contains(strings.ToLower(body), "docker.sock") {
		t.Fatalf("must not fall through to host docker.sock: %q", body)
	}
	if !strings.Contains(body, "microVM") && !strings.Contains(body, "Linux") && !strings.Contains(body, "WSL2") {
		t.Fatalf("expected microVM fail-closed message in %q", body)
	}
}

func TestLambdaImageInvokeMicroVMOptInFailsClosed(t *testing.T) {
	srv, _, _ := newTestServerStoreWith(t, func(cfg *config.Config) {
		cfg.ComputeRuntime = "microvm"
	})
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	mustCreateIAMRole(t, handler, "lambda-exec-microvm-img", lambdaTrustOK, now)
	roleARN := "arn:aws:iam::" + testAccountID + ":role/lambda-exec-microvm-img"

	createRec := mustLambdaJSON(t, handler, "CreateFunction", map[string]any{
		"FunctionName": "microvm-img-fn",
		"PackageType":  "Image",
		"Role":         roleARN,
		"Handler":      "app.handler",
		"Code": map[string]any{
			"ImageUri": "public.ecr.aws/lambda/python:3.12",
		},
	}, now)
	if createRec.Code != http.StatusOK {
		t.Fatalf("CreateFunction status=%d body=%q", createRec.Code, createRec.Body.String())
	}

	invokeRec := mustLambdaJSON(t, handler, "Invoke", map[string]any{
		"FunctionName": "microvm-img-fn",
		"Payload":      `{"ping":true}`,
	}, now)
	if invokeRec.Code == http.StatusOK {
		t.Fatalf("expected microVM Image Invoke to fail closed, got 200")
	}
	body := invokeRec.Body.String()
	if strings.Contains(strings.ToLower(body), "docker.sock") {
		t.Fatalf("must not fall through to host docker.sock: %q", body)
	}
	if !strings.Contains(body, "microVM") && !strings.Contains(body, "Linux") && !strings.Contains(body, "WSL2") {
		t.Fatalf("expected microVM fail-closed message in %q", body)
	}
}

func TestLambdaPublishLayerVersionREST(t *testing.T) {
	srv, _ := newTestServer(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	raw, err := json.Marshal(map[string]any{
		"Content": map[string]any{
			"ZipFile": testLambdaLayerZipB64(t),
		},
		"Description": "rest-layer",
	})
	if err != nil {
		t.Fatal(err)
	}
	// AWS CLI uses /2018-10-31/layers/{name}/versions (not X-Amz-Target).
	req := mustNewRequest(t, http.MethodPost, "http://127.0.0.1:4566/2018-10-31/layers/cli-layer/versions", raw)
	req.Header.Set("Content-Type", "application/json")
	signHeader(t, req, raw, testAccessKey, testSecret, testRegion, "lambda", now)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("PublishLayerVersion REST status=%d body=%q", rec.Code, rec.Body.String())
	}
	var published map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &published); err != nil {
		t.Fatal(err)
	}
	want := "arn:aws:lambda:us-east-1:" + testAccountID + ":layer:cli-layer:1"
	if published["LayerVersionArn"] != want {
		t.Fatalf("LayerVersionArn=%v want %s body=%s", published["LayerVersionArn"], want, rec.Body.String())
	}

	getReq := mustNewRequest(t, http.MethodGet, "http://127.0.0.1:4566/2018-10-31/layers/cli-layer/versions/1", nil)
	signHeader(t, getReq, nil, testAccessKey, testSecret, testRegion, "lambda", now)
	getRec := httptest.NewRecorder()
	handler.ServeHTTP(getRec, getReq)
	if getRec.Code != http.StatusOK {
		t.Fatalf("GetLayerVersion REST status=%d body=%q", getRec.Code, getRec.Body.String())
	}
}
