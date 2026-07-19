package server_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func mustSSMJSON(t *testing.T, handler http.Handler, target string, payload map[string]any, now time.Time) *httptest.ResponseRecorder {
	t.Helper()
	return mustSSMJSONWithCreds(t, handler, target, payload, testAccessKey, testSecret, now)
}

func mustSSMJSONWithCreds(
	t *testing.T,
	handler http.Handler,
	target string,
	payload map[string]any,
	akid, secret string,
	now time.Time,
) *httptest.ResponseRecorder {
	t.Helper()
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	req := mustNewRequest(t, http.MethodPost, "http://127.0.0.1:4566/", raw)
	req.Header.Set("Content-Type", "application/x-amz-json-1.1")
	req.Header.Set("X-Amz-Target", "AmazonSSM."+target)
	signHeader(t, req, raw, akid, secret, testRegion, "ssm", now)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}

func TestSSMPutGetStringRoundTrip(t *testing.T) {
	srv, _ := newTestServer(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	putRec := mustSSMJSON(t, handler, "PutParameter", map[string]any{
		"Name":  "/app/config",
		"Value": "hello-ssm",
		"Type":  "String",
	}, now)
	if putRec.Code != http.StatusOK {
		t.Fatalf("PutParameter status=%d body=%q", putRec.Code, putRec.Body.String())
	}
	var putOut map[string]any
	if err := json.Unmarshal(putRec.Body.Bytes(), &putOut); err != nil {
		t.Fatal(err)
	}
	if v, _ := putOut["Version"].(float64); int(v) != 1 {
		t.Fatalf("Version=%v want 1 body=%q", putOut["Version"], putRec.Body.String())
	}

	getRec := mustSSMJSON(t, handler, "GetParameter", map[string]any{
		"Name": "/app/config",
	}, now)
	if getRec.Code != http.StatusOK {
		t.Fatalf("GetParameter status=%d body=%q", getRec.Code, getRec.Body.String())
	}
	var getOut map[string]any
	if err := json.Unmarshal(getRec.Body.Bytes(), &getOut); err != nil {
		t.Fatal(err)
	}
	param, _ := getOut["Parameter"].(map[string]any)
	if param == nil {
		t.Fatalf("missing Parameter in %q", getRec.Body.String())
	}
	val, _ := param["Value"].(string)
	if val != "hello-ssm" {
		t.Fatalf("Value=%q want hello-ssm body=%q", val, getRec.Body.String())
	}
}

func TestSSMPutGetSecureStringRoundTrip(t *testing.T) {
	srv, _ := newTestServer(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	putRec := mustSSMJSON(t, handler, "PutParameter", map[string]any{
		"Name":  "secret/token",
		"Value": "s3cr3t",
		"Type":  "SecureString",
	}, now)
	if putRec.Code != http.StatusOK {
		t.Fatalf("PutParameter status=%d body=%q", putRec.Code, putRec.Body.String())
	}

	getRec := mustSSMJSON(t, handler, "GetParameter", map[string]any{
		"Name":           "/secret/token",
		"WithDecryption": true,
	}, now)
	if getRec.Code != http.StatusOK {
		t.Fatalf("GetParameter status=%d body=%q", getRec.Code, getRec.Body.String())
	}
	var getOut map[string]any
	if err := json.Unmarshal(getRec.Body.Bytes(), &getOut); err != nil {
		t.Fatal(err)
	}
	param, _ := getOut["Parameter"].(map[string]any)
	val, _ := param["Value"].(string)
	if val != "s3cr3t" {
		t.Fatalf("Value=%q want s3cr3t body=%q", val, getRec.Body.String())
	}
	typ, _ := param["Type"].(string)
	if typ != "SecureString" {
		t.Fatalf("Type=%q want SecureString", typ)
	}
}

func TestSSMPutParameterAlreadyExists(t *testing.T) {
	srv, _ := newTestServer(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	first := mustSSMJSON(t, handler, "PutParameter", map[string]any{
		"Name":  "/dup",
		"Value": "v1",
		"Type":  "String",
	}, now)
	if first.Code != http.StatusOK {
		t.Fatalf("first PutParameter status=%d body=%q", first.Code, first.Body.String())
	}

	second := mustSSMJSON(t, handler, "PutParameter", map[string]any{
		"Name":  "dup",
		"Value": "v2",
		"Type":  "String",
	}, now)
	if second.Code != http.StatusBadRequest {
		t.Fatalf("second PutParameter status=%d want 400 body=%q", second.Code, second.Body.String())
	}
	if !strings.Contains(second.Body.String(), "ParameterAlreadyExists") {
		t.Fatalf("expected ParameterAlreadyExists in %q", second.Body.String())
	}
}

func TestSSMGetParameterNotFound(t *testing.T) {
	srv, _ := newTestServer(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	rec := mustSSMJSON(t, handler, "GetParameter", map[string]any{
		"Name": "/missing",
	}, now)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("GetParameter status=%d want 400 body=%q", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "ParameterNotFound") {
		t.Fatalf("expected ParameterNotFound in %q", rec.Body.String())
	}
}

func TestSSMDeleteParameter(t *testing.T) {
	srv, _ := newTestServer(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	putRec := mustSSMJSON(t, handler, "PutParameter", map[string]any{
		"Name":  "/gone",
		"Value": "x",
		"Type":  "String",
	}, now)
	if putRec.Code != http.StatusOK {
		t.Fatalf("PutParameter status=%d body=%q", putRec.Code, putRec.Body.String())
	}

	delRec := mustSSMJSON(t, handler, "DeleteParameter", map[string]any{
		"Name": "/gone",
	}, now)
	if delRec.Code != http.StatusOK {
		t.Fatalf("DeleteParameter status=%d body=%q", delRec.Code, delRec.Body.String())
	}

	getRec := mustSSMJSON(t, handler, "GetParameter", map[string]any{
		"Name": "/gone",
	}, now)
	if getRec.Code != http.StatusBadRequest {
		t.Fatalf("GetParameter after delete status=%d want 400 body=%q", getRec.Code, getRec.Body.String())
	}
}

func TestSSMGetParametersAndDescribe(t *testing.T) {
	srv, _ := newTestServer(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	for _, spec := range []struct {
		name, val, typ string
	}{
		{"/app/a", "one", "String"},
		{"/app/b", "two", "SecureString"},
		{"/other/c", "three", "String"},
	} {
		rec := mustSSMJSON(t, handler, "PutParameter", map[string]any{
			"Name":  spec.name,
			"Value": spec.val,
			"Type":  spec.typ,
		}, now)
		if rec.Code != http.StatusOK {
			t.Fatalf("PutParameter %s status=%d body=%q", spec.name, rec.Code, rec.Body.String())
		}
	}

	batchRec := mustSSMJSON(t, handler, "GetParameters", map[string]any{
		"Names":          []string{"/app/a", "/app/b", "/missing"},
		"WithDecryption": true,
	}, now)
	if batchRec.Code != http.StatusOK {
		t.Fatalf("GetParameters status=%d body=%q", batchRec.Code, batchRec.Body.String())
	}
	var batchOut map[string]any
	if err := json.Unmarshal(batchRec.Body.Bytes(), &batchOut); err != nil {
		t.Fatal(err)
	}
	params, _ := batchOut["Parameters"].([]any)
	if len(params) != 2 {
		t.Fatalf("Parameters len=%d want 2 body=%q", len(params), batchRec.Body.String())
	}
	invalid, _ := batchOut["InvalidParameters"].([]any)
	if len(invalid) != 1 {
		t.Fatalf("InvalidParameters len=%d want 1 body=%q", len(invalid), batchRec.Body.String())
	}

	descRec := mustSSMJSON(t, handler, "DescribeParameters", map[string]any{
		"ParameterFilters": []map[string]any{
			{
				"Key":    "Name",
				"Option": "BeginsWith",
				"Values": []string{"/app/"},
			},
		},
	}, now)
	if descRec.Code != http.StatusOK {
		t.Fatalf("DescribeParameters status=%d body=%q", descRec.Code, descRec.Body.String())
	}
	var descOut map[string]any
	if err := json.Unmarshal(descRec.Body.Bytes(), &descOut); err != nil {
		t.Fatal(err)
	}
	descParams, _ := descOut["Parameters"].([]any)
	if len(descParams) != 2 {
		t.Fatalf("Describe Parameters len=%d want 2 body=%q", len(descParams), descRec.Body.String())
	}
}

func TestSSMAccessDeniedWithoutIdentityPolicy(t *testing.T) {
	srv, st, _ := newTestServerStore(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	_, userARN, err := st.CreateUser(testAccountID, "ssm-deny")
	if err != nil {
		t.Fatal(err)
	}
	userAKID, userSecret, err := st.CreateUserAccessKey(testAccountID, "ssm-deny")
	if err != nil {
		t.Fatal(err)
	}
	denyOnly := `{"Version":"2012-10-17","Statement":[{"Effect":"Deny","Action":"ssm:*","Resource":"*"}]}`
	if err := st.PutInlinePolicy(userARN, "nossm", denyOnly); err != nil {
		t.Fatal(err)
	}

	rec := mustSSMJSONWithCreds(t, handler, "PutParameter", map[string]any{
		"Name":  "/denied",
		"Value": "x",
		"Type":  "String",
	}, userAKID, userSecret, now)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("PutParameter status=%d want 403 body=%q", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "AccessDeniedException") {
		t.Fatalf("expected AccessDeniedException in %q", rec.Body.String())
	}
}
