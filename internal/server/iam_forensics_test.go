package server_test

import (
	"encoding/base64"
	"encoding/csv"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestAssumeRoleSuccessAuditEnrichment(t *testing.T) {
	srv, st, auditDir := newTestServerStore(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	trust := `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":{"AWS":"*"},"Action":"sts:AssumeRole"}]}`
	roleARN, err := st.CreateRole(testAccountID, "forensic-audit-role", trust)
	if err != nil {
		t.Fatal(err)
	}

	body := []byte("Action=AssumeRole&Version=2011-06-15&RoleArn=" + url.QueryEscape(roleARN) +
		"&RoleSessionName=forensic-sess&DurationSeconds=3600")
	req := mustNewRequest(t, http.MethodPost, "http://127.0.0.1:4566/", body)
	signHeader(t, req, body, testAccessKey, testSecret, testRegion, "sts", now)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("AssumeRole status=%d body=%q", rec.Code, rec.Body.String())
	}

	data, err := os.ReadFile(filepath.Join(auditDir, "events.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	var assumeEv map[string]any
	for _, line := range lines {
		var ev map[string]any
		if err := json.Unmarshal([]byte(line), &ev); err != nil {
			t.Fatal(err)
		}
		if ev["eventName"] == "AssumeRole" {
			assumeEv = ev
		}
	}
	if assumeEv == nil {
		t.Fatal("no AssumeRole audit event")
	}
	params, _ := assumeEv["requestParameters"].(map[string]any)
	if params["roleArn"] != roleARN {
		t.Fatalf("roleArn=%v", params["roleArn"])
	}
	if params["roleSessionName"] != "forensic-sess" {
		t.Fatalf("roleSessionName=%v", params["roleSessionName"])
	}
	switch v := params["durationSeconds"].(type) {
	case float64:
		if int(v) != 3600 {
			t.Fatalf("durationSeconds=%v", v)
		}
	default:
		t.Fatalf("durationSeconds=%T %v", params["durationSeconds"], params["durationSeconds"])
	}
	resources, _ := assumeEv["resources"].([]any)
	if len(resources) < 1 {
		t.Fatalf("resources=%v", assumeEv["resources"])
	}
	first, _ := resources[0].(map[string]any)
	if first["ARN"] != roleARN {
		t.Fatalf("resource ARN=%v", first["ARN"])
	}
}

func TestIAMGetAccessKeyLastUsedAndCredentialReport(t *testing.T) {
	srv, _ := newTestServer(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	iamPost := func(body string) *httptest.ResponseRecorder {
		t.Helper()
		req := mustNewRequest(t, http.MethodPost, "http://127.0.0.1:4566/", []byte(body))
		signHeader(t, req, []byte(body), testAccessKey, testSecret, testRegion, "iam", now)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		return rec
	}

	rec := iamPost("Action=CreateUser&Version=2010-05-08&UserName=lastused-lab")
	if rec.Code != http.StatusOK {
		t.Fatalf("CreateUser: %d %s", rec.Code, rec.Body.String())
	}
	rec = iamPost("Action=CreateAccessKey&Version=2010-05-08&UserName=lastused-lab")
	if rec.Code != http.StatusOK {
		t.Fatalf("CreateAccessKey: %d %s", rec.Code, rec.Body.String())
	}
	userAK := xmlTag(t, rec.Body.String(), "AccessKeyId")
	userSecret := xmlTag(t, rec.Body.String(), "SecretAccessKey")

	gciBody := []byte("Action=GetCallerIdentity&Version=2011-06-15")
	gciReq := mustNewRequest(t, http.MethodPost, "http://127.0.0.1:4566/", gciBody)
	signHeader(t, gciReq, gciBody, userAK, userSecret, testRegion, "sts", now)
	gciRec := httptest.NewRecorder()
	handler.ServeHTTP(gciRec, gciReq)
	if gciRec.Code != http.StatusOK {
		t.Fatalf("GetCallerIdentity: %d %s", gciRec.Code, gciRec.Body.String())
	}

	rec = iamPost("Action=GetAccessKeyLastUsed&Version=2010-05-08&AccessKeyId=" + url.QueryEscape(userAK))
	if rec.Code != http.StatusOK {
		t.Fatalf("GetAccessKeyLastUsed: %d %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "<ServiceName>sts</ServiceName>") {
		t.Fatalf("body=%s", rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "<Region>"+testRegion+"</Region>") {
		t.Fatalf("body=%s", rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "<UserName>lastused-lab</UserName>") {
		t.Fatalf("body=%s", rec.Body.String())
	}

	rec = iamPost("Action=GetCredentialReport&Version=2010-05-08")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("GetCredentialReport before generate: %d %s", rec.Code, rec.Body.String())
	}

	rec = iamPost("Action=GenerateCredentialReport&Version=2010-05-08")
	if rec.Code != http.StatusOK {
		t.Fatalf("GenerateCredentialReport: %d %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "<State>COMPLETE</State>") {
		t.Fatalf("body=%s", rec.Body.String())
	}

	rec = iamPost("Action=GetCredentialReport&Version=2010-05-08")
	if rec.Code != http.StatusOK {
		t.Fatalf("GetCredentialReport: %d %s", rec.Code, rec.Body.String())
	}
	contentB64 := xmlTag(t, rec.Body.String(), "Content")
	csvBytes, err := base64.StdEncoding.DecodeString(contentB64)
	if err != nil {
		t.Fatal(err)
	}
	rows, err := csv.NewReader(strings.NewReader(string(csvBytes))).ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, row := range rows[1:] {
		if len(row) > 0 && row[0] == "lastused-lab" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("credential report missing user row: %s", string(csvBytes))
	}
}
