package server_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Kyaxris-Labs/Noctaxris/internal/config"
)

func TestAuditUserNameAndRequestParams(t *testing.T) {
	srv, auditDir := newTestServer(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	// Any successful CloudTrail call writes a success audit line with userName=root.
	rec := mustCloudTrailJSON(t, handler, map[string]any{"MaxResults": 1}, now)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%q", rec.Code, rec.Body.String())
	}

	data, err := os.ReadFile(filepath.Join(auditDir, "events.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	var last map[string]any
	if err := json.Unmarshal([]byte(lines[len(lines)-1]), &last); err != nil {
		t.Fatal(err)
	}
	uid, _ := last["userIdentity"].(map[string]any)
	if name, _ := uid["userName"].(string); name != "root" {
		t.Fatalf("userName=%v want root line=%s", uid["userName"], lines[len(lines)-1])
	}
	params, _ := last["requestParameters"].(map[string]any)
	if params["httpMethod"] == nil || params["path"] == nil {
		t.Fatalf("requestParameters=%v", params)
	}
	if params["xAmzTarget"] == nil {
		t.Fatalf("expected xAmzTarget in requestParameters=%v", params)
	}
}

func TestAuditClientIPTrustXFFOptIn(t *testing.T) {
	t.Run("default_peer", func(t *testing.T) {
		srv, auditDir := newTestServer(t)
		handler := srv.Handler()
		now := time.Now().UTC().Truncate(time.Second)

		raw, _ := json.Marshal(map[string]any{"MaxResults": 1})
		req := mustNewRequest(t, http.MethodPost, "http://127.0.0.1:4566/", raw)
		req.Header.Set("Content-Type", "application/x-amz-json-1.1")
		req.Header.Set("X-Amz-Target", "CloudTrail_20131101.LookupEvents")
		req.Header.Set("X-Forwarded-For", "198.51.100.77")
		req.RemoteAddr = "203.0.113.9:12345"
		signHeader(t, req, raw, testAccessKey, testSecret, testRegion, "cloudtrail", now)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("status=%d body=%q", rec.Code, rec.Body.String())
		}

		data, err := os.ReadFile(filepath.Join(auditDir, "events.jsonl"))
		if err != nil {
			t.Fatal(err)
		}
		lines := strings.Split(strings.TrimSpace(string(data)), "\n")
		var last map[string]any
		if err := json.Unmarshal([]byte(lines[len(lines)-1]), &last); err != nil {
			t.Fatal(err)
		}
		if ip, _ := last["sourceIPAddress"].(string); ip != "203.0.113.9" {
			t.Fatalf("sourceIPAddress=%q want peer 203.0.113.9", ip)
		}
	})

	t.Run("xff_opt_in", func(t *testing.T) {
		srv, _, auditDir := newTestServerStoreWith(t, func(cfg *config.Config) {
			cfg.CloudTrailTrustXFF = true
		})
		handler := srv.Handler()
		now := time.Now().UTC().Truncate(time.Second)

		raw, _ := json.Marshal(map[string]any{"MaxResults": 1})
		req := mustNewRequest(t, http.MethodPost, "http://127.0.0.1:4566/", raw)
		req.Header.Set("Content-Type", "application/x-amz-json-1.1")
		req.Header.Set("X-Amz-Target", "CloudTrail_20131101.LookupEvents")
		req.Header.Set("X-Forwarded-For", "198.51.100.77, 192.0.2.1")
		req.RemoteAddr = "203.0.113.9:12345"
		signHeader(t, req, raw, testAccessKey, testSecret, testRegion, "cloudtrail", now)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("status=%d body=%q", rec.Code, rec.Body.String())
		}

		data, err := os.ReadFile(filepath.Join(auditDir, "events.jsonl"))
		if err != nil {
			t.Fatal(err)
		}
		lines := strings.Split(strings.TrimSpace(string(data)), "\n")
		var last map[string]any
		if err := json.Unmarshal([]byte(lines[len(lines)-1]), &last); err != nil {
			t.Fatal(err)
		}
		if ip, _ := last["sourceIPAddress"].(string); ip != "198.51.100.77" {
			t.Fatalf("sourceIPAddress=%q want XFF 198.51.100.77", ip)
		}
	})
}

func TestS3CreateBucketAuditResources(t *testing.T) {
	srv, auditDir := newTestServer(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	req := mustNewRequest(t, http.MethodPut, "http://127.0.0.1:4566/audit-res-bucket", nil)
	signHeader(t, req, nil, testAccessKey, testSecret, testRegion, "s3", now)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("CreateBucket status=%d body=%q", rec.Code, rec.Body.String())
	}

	data, err := os.ReadFile(filepath.Join(auditDir, "events.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	var last map[string]any
	if err := json.Unmarshal([]byte(lines[len(lines)-1]), &last); err != nil {
		t.Fatal(err)
	}
	if name, _ := last["eventName"].(string); name != "CreateBucket" {
		t.Fatalf("eventName=%v", last["eventName"])
	}
	resources, _ := last["resources"].([]any)
	if len(resources) < 1 {
		t.Fatalf("resources=%v", last["resources"])
	}
	first, _ := resources[0].(map[string]any)
	if arn, _ := first["ARN"].(string); arn != "arn:aws:s3:::audit-res-bucket" {
		t.Fatalf("ARN=%v", first["ARN"])
	}
	params, _ := last["requestParameters"].(map[string]any)
	if params["bucketName"] != "audit-res-bucket" {
		t.Fatalf("requestParameters=%v", params)
	}
}

func TestSecretsCreateSecretAuditResources(t *testing.T) {
	srv, auditDir := newTestServer(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	create := mustSecretsJSON(t, handler, "CreateSecret", map[string]any{
		"Name": "audit-res-secret", "SecretString": "not-logged",
	}, now)
	if create.Code != http.StatusOK {
		t.Fatalf("CreateSecret status=%d body=%q", create.Code, create.Body.String())
	}
	var created map[string]any
	if err := json.Unmarshal(create.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	wantARN, _ := created["ARN"].(string)
	if wantARN == "" {
		t.Fatalf("missing ARN in CreateSecret response: %s", create.Body.String())
	}

	data, err := os.ReadFile(filepath.Join(auditDir, "events.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	var last map[string]any
	if err := json.Unmarshal([]byte(lines[len(lines)-1]), &last); err != nil {
		t.Fatal(err)
	}
	if name, _ := last["eventName"].(string); name != "CreateSecret" {
		t.Fatalf("eventName=%v", last["eventName"])
	}
	resources, _ := last["resources"].([]any)
	if len(resources) < 1 {
		t.Fatalf("resources=%v", last["resources"])
	}
	first, _ := resources[0].(map[string]any)
	if typ, _ := first["type"].(string); typ != "AWS::SecretsManager::Secret" {
		t.Fatalf("type=%v", first["type"])
	}
	if arn, _ := first["ARN"].(string); arn != wantARN {
		t.Fatalf("ARN=%v want %q", first["ARN"], wantARN)
	}
	params, _ := last["requestParameters"].(map[string]any)
	if params["name"] != "audit-res-secret" {
		t.Fatalf("requestParameters=%v", params)
	}
	rawLine := lines[len(lines)-1]
	if strings.Contains(rawLine, "not-logged") || strings.Contains(rawLine, "SecretString") {
		t.Fatalf("secret material leaked into audit line: %s", rawLine)
	}
}

func TestAuditAPIErrorUsesServiceEventSource(t *testing.T) {
	srv, auditDir := newTestServer(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	rec := mustKMSJSON(t, handler, "CreateAlias", map[string]any{
		"AliasName": "alias/audit-enrichment-missing-target",
	}, now)
	if rec.Code == http.StatusOK {
		t.Fatalf("expected CreateAlias error, got OK body=%q", rec.Body.String())
	}

	data, err := os.ReadFile(filepath.Join(auditDir, "events.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	var last map[string]any
	if err := json.Unmarshal([]byte(lines[len(lines)-1]), &last); err != nil {
		t.Fatal(err)
	}
	if src, _ := last["eventSource"].(string); src != "kms.amazonaws.com" {
		t.Fatalf("eventSource=%q want kms.amazonaws.com line=%s", src, lines[len(lines)-1])
	}
	if cat, _ := last["eventCategory"].(string); cat != "Management" {
		t.Fatalf("eventCategory=%v want Management", last["eventCategory"])
	}
	mgmt, ok := last["managementEvent"].(bool)
	if !ok || !mgmt {
		t.Fatalf("managementEvent=%v want true", last["managementEvent"])
	}
}

func TestSuccessAuditManagementEventAndSessionContext(t *testing.T) {
	srv, st, auditDir := newTestServerStore(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	trust := `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":{"AWS":"*"},"Action":"sts:AssumeRole"}]}`
	roleARN, err := st.CreateRole(testAccountID, "audit-ct-role", trust)
	if err != nil {
		t.Fatal(err)
	}
	_ = roleARN

	assumeBody := []byte("Action=AssumeRole&Version=2011-06-15&RoleArn=" + url.QueryEscape(roleARN) + "&RoleSessionName=audit-sess")
	assumeReq := mustNewRequest(t, http.MethodPost, "http://127.0.0.1:4566/", assumeBody)
	signHeader(t, assumeReq, assumeBody, testAccessKey, testSecret, testRegion, "sts", now)
	assumeRec := httptest.NewRecorder()
	handler.ServeHTTP(assumeRec, assumeReq)
	if assumeRec.Code != http.StatusOK {
		t.Fatalf("AssumeRole status=%d body=%q", assumeRec.Code, assumeRec.Body.String())
	}
	tempAKID := xmlTag(t, assumeRec.Body.String(), "AccessKeyId")
	tempSecret := xmlTag(t, assumeRec.Body.String(), "SecretAccessKey")
	tempToken := xmlTag(t, assumeRec.Body.String(), "SessionToken")

	gciBody := []byte("Action=GetCallerIdentity&Version=2011-06-15")
	gciReq := mustNewRequest(t, http.MethodPost, "http://127.0.0.1:4566/", gciBody)
	signHeader(t, gciReq, gciBody, tempAKID, tempSecret, testRegion, "sts", now)
	gciReq.Header.Set("X-Amz-Security-Token", tempToken)
	gciRec := httptest.NewRecorder()
	handler.ServeHTTP(gciRec, gciReq)
	if gciRec.Code != http.StatusOK {
		t.Fatalf("GetCallerIdentity status=%d body=%q", gciRec.Code, gciRec.Body.String())
	}

	data, err := os.ReadFile(filepath.Join(auditDir, "events.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	var last map[string]any
	if err := json.Unmarshal([]byte(lines[len(lines)-1]), &last); err != nil {
		t.Fatal(err)
	}
	if name, _ := last["eventName"].(string); name != "GetCallerIdentity" {
		t.Fatalf("eventName=%v", last["eventName"])
	}
	if cat, _ := last["eventCategory"].(string); cat != "Management" {
		t.Fatalf("eventCategory=%v want Management", last["eventCategory"])
	}
	mgmt, ok := last["managementEvent"].(bool)
	if !ok || !mgmt {
		t.Fatalf("managementEvent=%v want true", last["managementEvent"])
	}
	uid, _ := last["userIdentity"].(map[string]any)
	if typ, _ := uid["type"].(string); typ != "AssumedRole" {
		t.Fatalf("userIdentity.type=%v want AssumedRole", uid["type"])
	}
	sc, _ := uid["sessionContext"].(map[string]any)
	if sc == nil {
		t.Fatalf("missing sessionContext in %v", uid)
	}
	issuer, _ := sc["sessionIssuer"].(map[string]any)
	if issuer == nil {
		t.Fatalf("missing sessionIssuer in %v", sc)
	}
	if issuer["userName"] != "audit-ct-role" {
		t.Fatalf("sessionIssuer.userName=%v want audit-ct-role", issuer["userName"])
	}
	if arn, _ := issuer["arn"].(string); !strings.Contains(arn, ":role/audit-ct-role") {
		t.Fatalf("sessionIssuer.arn=%v", issuer["arn"])
	}
}
