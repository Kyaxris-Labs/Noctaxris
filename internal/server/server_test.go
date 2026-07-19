package server_test

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/Kyaxris-Labs/Noctaxris/internal/config"
	"github.com/Kyaxris-Labs/Noctaxris/internal/kernel/audit"
	"github.com/Kyaxris-Labs/Noctaxris/internal/server"
	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

const (
	testAccountID = "000000000001"
	testAccessKey = "AKIAROOTEXAMPLE01"
	testSecret    = "secret-root-value"
	testRegion    = "us-east-1"
	testSvc       = "sts"
)

func newTestServer(t *testing.T) (*server.Server, string) {
	t.Helper()

	dir := t.TempDir()
	key, err := store.LoadOrCreateMasterKey(filepath.Join(dir, "master.key"))
	if err != nil {
		t.Fatal(err)
	}
	st, err := store.Open(dir, key)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := st.Close(); err != nil {
			t.Errorf("close store: %v", err)
		}
	})
	if err := st.EnsureRoot(testAccountID, testAccessKey, testSecret); err != nil {
		t.Fatal(err)
	}

	auditDir := filepath.Join(dir, "cloudtrail")
	aud, err := audit.NewWriter(auditDir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := aud.Close(); err != nil {
			t.Errorf("close audit: %v", err)
		}
	})

	cfg := config.Config{
		ListenAddr: "127.0.0.1:0",
		DataRoot:   dir,
		AccountID:  testAccountID,
	}

	return server.New(cfg, st, aud), auditDir
}

func TestHealthOK(t *testing.T) {
	srv, _ := newTestServer(t)
	handler := srv.Handler()

	req := httptest.NewRequest(http.MethodGet, "/_noctaxris/health", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d want %d", rec.Code, http.StatusOK)
	}
	if body := rec.Body.String(); body != "ok" {
		t.Fatalf("body=%q want ok", body)
	}
}

func TestAWSPathRejectsMissingAuth(t *testing.T) {
	srv, auditDir := newTestServer(t)
	handler := srv.Handler()

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status=%d want %d", rec.Code, http.StatusForbidden)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "MissingAuthenticationToken") {
		t.Fatalf("expected MissingAuthenticationToken in body %q", body)
	}
	if rec.Header().Get("x-amz-request-id") == "" {
		t.Fatal("expected x-amz-request-id header")
	}

	eventsPath := filepath.Join(auditDir, "events.jsonl")
	data, err := os.ReadFile(eventsPath)
	if err != nil {
		t.Fatal(err)
	}
	if len(data) == 0 {
		t.Fatal("expected audit event")
	}
	if !strings.Contains(string(data), `"errorCode":"MissingAuthenticationToken"`) {
		t.Fatalf("expected audit error code in %q", data)
	}
}

func TestSignedGetCallerIdentityOK(t *testing.T) {
	srv, auditDir := newTestServer(t)
	handler := srv.Handler()

	now := time.Now().UTC().Truncate(time.Second)
	body := []byte("Action=GetCallerIdentity&Version=2011-06-15")
	req := mustNewRequest(t, http.MethodPost, "http://127.0.0.1:4566/", body)
	signHeader(t, req, body, testAccessKey, testSecret, testRegion, testSvc, now)

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d want %d body=%q", rec.Code, http.StatusOK, rec.Body.String())
	}
	resp := rec.Body.String()
	if !strings.Contains(resp, "<GetCallerIdentityResponse") {
		t.Fatalf("missing GetCallerIdentityResponse in %q", resp)
	}
	if !strings.Contains(resp, "<Account>"+testAccountID+"</Account>") {
		t.Fatalf("missing Account in %q", resp)
	}
	if !strings.Contains(resp, "arn:aws:iam::"+testAccountID+":root") {
		t.Fatalf("missing root ARN in %q", resp)
	}
	if !strings.Contains(resp, "<UserId>"+testAccountID+"</UserId>") {
		t.Fatalf("missing UserId in %q", resp)
	}
	if rec.Header().Get("x-amz-request-id") == "" {
		t.Fatal("expected x-amz-request-id")
	}

	eventsPath := filepath.Join(auditDir, "events.jsonl")
	data, err := os.ReadFile(eventsPath)
	if err != nil {
		t.Fatal(err)
	}
	auditText := string(data)
	if !strings.Contains(auditText, `"eventName":"GetCallerIdentity"`) {
		t.Fatalf("expected GetCallerIdentity audit in %q", auditText)
	}
	if strings.Contains(auditText, `"errorCode"`) {
		t.Fatalf("success audit must not set errorCode: %q", auditText)
	}
	if strings.Contains(auditText, testSecret) {
		t.Fatal("audit must not contain secret")
	}
}

func TestBadSignatureForbidden(t *testing.T) {
	srv, auditDir := newTestServer(t)
	handler := srv.Handler()

	now := time.Now().UTC().Truncate(time.Second)
	body := []byte("Action=GetCallerIdentity&Version=2011-06-15")
	req := mustNewRequest(t, http.MethodPost, "http://127.0.0.1:4566/", body)
	signHeader(t, req, body, testAccessKey, testSecret, testRegion, testSvc, now)

	auth := req.Header.Get("Authorization")
	idx := strings.Index(auth, "Signature=")
	req.Header.Set("Authorization", auth[:idx]+"Signature="+strings.Repeat("ab", 32))

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status=%d want %d body=%q", rec.Code, http.StatusForbidden, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "SignatureDoesNotMatch") {
		t.Fatalf("expected SignatureDoesNotMatch in %q", rec.Body.String())
	}

	data, err := os.ReadFile(filepath.Join(auditDir, "events.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `"errorCode":"SignatureDoesNotMatch"`) {
		t.Fatalf("expected SignatureDoesNotMatch audit in %q", data)
	}
}

func TestSignedUnknownActionNotImplemented(t *testing.T) {
	srv, auditDir := newTestServer(t)
	handler := srv.Handler()

	now := time.Now().UTC().Truncate(time.Second)
	body := []byte("Action=ListBuckets&Version=2006-03-01")
	req := mustNewRequest(t, http.MethodPost, "http://127.0.0.1:4566/", body)
	signHeader(t, req, body, testAccessKey, testSecret, testRegion, "s3", now)

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotImplemented {
		t.Fatalf("status=%d want %d body=%q", rec.Code, http.StatusNotImplemented, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "NotImplemented") {
		t.Fatalf("expected NotImplemented in %q", rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "Phase 4") {
		t.Fatalf("expected Phase 4 message in %q", rec.Body.String())
	}

	data, err := os.ReadFile(filepath.Join(auditDir, "events.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `"errorCode":"NotImplemented"`) {
		t.Fatalf("expected NotImplemented audit in %q", data)
	}
}

func TestCreateAccountAssumeRoleFlow(t *testing.T) {
	srv, _ := newTestServer(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	createBody := []byte("Action=CreateAccount&Version=2016-11-28&Email=member%40example.com&AccountName=Member")
	createReq := mustNewRequest(t, http.MethodPost, "http://127.0.0.1:4566/", createBody)
	signHeader(t, createReq, createBody, testAccessKey, testSecret, testRegion, "organizations", now)
	createRec := httptest.NewRecorder()
	handler.ServeHTTP(createRec, createReq)
	if createRec.Code != http.StatusOK {
		t.Fatalf("CreateAccount status=%d body=%q", createRec.Code, createRec.Body.String())
	}
	createXML := createRec.Body.String()
	if !strings.Contains(createXML, "SUCCEEDED") {
		t.Fatalf("CreateAccount body=%q", createXML)
	}
	// Extract Id from <Id>...</Id> inside CreateAccountStatus
	idStart := strings.Index(createXML, "<Id>")
	idEnd := strings.Index(createXML, "</Id>")
	if idStart < 0 || idEnd <= idStart {
		t.Fatalf("missing create request id in %q", createXML)
	}
	createID := createXML[idStart+4 : idEnd]

	descBody := []byte("Action=DescribeCreateAccountStatus&Version=2016-11-28&CreateAccountRequestId=" + createID)
	descReq := mustNewRequest(t, http.MethodPost, "http://127.0.0.1:4566/", descBody)
	signHeader(t, descReq, descBody, testAccessKey, testSecret, testRegion, "organizations", now)
	descRec := httptest.NewRecorder()
	handler.ServeHTTP(descRec, descReq)
	if descRec.Code != http.StatusOK {
		t.Fatalf("Describe status=%d body=%q", descRec.Code, descRec.Body.String())
	}
	if !strings.Contains(descRec.Body.String(), "<AccountId>000000000002</AccountId>") {
		t.Fatalf("Describe body=%q", descRec.Body.String())
	}

	roleARN := "arn:aws:iam::000000000002:role/OrganizationAccountAccessRole"
	assumeBody := []byte("Action=AssumeRole&Version=2011-06-15&RoleArn=" + url.QueryEscape(roleARN) + "&RoleSessionName=admin")
	assumeReq := mustNewRequest(t, http.MethodPost, "http://127.0.0.1:4566/", assumeBody)
	signHeader(t, assumeReq, assumeBody, testAccessKey, testSecret, testRegion, "sts", now)
	assumeRec := httptest.NewRecorder()
	handler.ServeHTTP(assumeRec, assumeReq)
	if assumeRec.Code != http.StatusOK {
		t.Fatalf("AssumeRole status=%d body=%q", assumeRec.Code, assumeRec.Body.String())
	}
	assumeXML := assumeRec.Body.String()
	if !strings.Contains(assumeXML, "ASIA") || !strings.Contains(assumeXML, "SessionToken") {
		t.Fatalf("AssumeRole body=%q", assumeXML)
	}
	if !strings.Contains(assumeXML, "assumed-role/OrganizationAccountAccessRole/admin") {
		t.Fatalf("missing assumed role ARN in %q", assumeXML)
	}
}

func TestCreateUserAccessKeyGetCallerIdentity(t *testing.T) {
	srv, _ := newTestServer(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	createUserBody := []byte("Action=CreateUser&Version=2010-05-08&UserName=alice")
	createUserReq := mustNewRequest(t, http.MethodPost, "http://127.0.0.1:4566/", createUserBody)
	signHeader(t, createUserReq, createUserBody, testAccessKey, testSecret, testRegion, "iam", now)
	createUserRec := httptest.NewRecorder()
	handler.ServeHTTP(createUserRec, createUserReq)
	if createUserRec.Code != http.StatusOK {
		t.Fatalf("CreateUser status=%d body=%q", createUserRec.Code, createUserRec.Body.String())
	}
	if !strings.Contains(createUserRec.Body.String(), "<UserName>alice</UserName>") {
		t.Fatalf("CreateUser body=%q", createUserRec.Body.String())
	}

	createKeyBody := []byte("Action=CreateAccessKey&Version=2010-05-08&UserName=alice")
	createKeyReq := mustNewRequest(t, http.MethodPost, "http://127.0.0.1:4566/", createKeyBody)
	signHeader(t, createKeyReq, createKeyBody, testAccessKey, testSecret, testRegion, "iam", now)
	createKeyRec := httptest.NewRecorder()
	handler.ServeHTTP(createKeyRec, createKeyReq)
	if createKeyRec.Code != http.StatusOK {
		t.Fatalf("CreateAccessKey status=%d body=%q", createKeyRec.Code, createKeyRec.Body.String())
	}
	keyXML := createKeyRec.Body.String()
	akid := xmlTag(t, keyXML, "AccessKeyId")
	secret := xmlTag(t, keyXML, "SecretAccessKey")
	if !strings.HasPrefix(akid, "AKIA") || secret == "" {
		t.Fatalf("CreateAccessKey body=%q", keyXML)
	}

	gciBody := []byte("Action=GetCallerIdentity&Version=2011-06-15")
	gciReq := mustNewRequest(t, http.MethodPost, "http://127.0.0.1:4566/", gciBody)
	signHeader(t, gciReq, gciBody, akid, secret, testRegion, "sts", now)
	gciRec := httptest.NewRecorder()
	handler.ServeHTTP(gciRec, gciReq)
	if gciRec.Code != http.StatusOK {
		t.Fatalf("GetCallerIdentity status=%d body=%q", gciRec.Code, gciRec.Body.String())
	}
	gciXML := gciRec.Body.String()
	if !strings.Contains(gciXML, "arn:aws:iam::"+testAccountID+":user/alice") {
		t.Fatalf("expected IAM user ARN in %q", gciXML)
	}
}

func TestGetSessionToken(t *testing.T) {
	srv, _ := newTestServer(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	body := []byte("Action=GetSessionToken&Version=2011-06-15")
	req := mustNewRequest(t, http.MethodPost, "http://127.0.0.1:4566/", body)
	signHeader(t, req, body, testAccessKey, testSecret, testRegion, "sts", now)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("GetSessionToken status=%d body=%q", rec.Code, rec.Body.String())
	}
	resp := rec.Body.String()
	if !strings.Contains(resp, "<GetSessionTokenResponse") {
		t.Fatalf("missing GetSessionTokenResponse in %q", resp)
	}
	if !strings.Contains(resp, "ASIA") || !strings.Contains(resp, "SessionToken") {
		t.Fatalf("GetSessionToken body=%q", resp)
	}
}

func TestAssumeRoleWithWebIdentityFailsWithoutIdP(t *testing.T) {
	srv, _ := newTestServer(t)
	handler := srv.Handler()

	roleARN := "arn:aws:iam::" + testAccountID + ":role/DoesNotMatter"
	body := []byte("Action=AssumeRoleWithWebIdentity&Version=2011-06-15&RoleArn=" +
		url.QueryEscape(roleARN) +
		"&RoleSessionName=web&WebIdentityToken=not.a.jwt")
	// Federation STS does not require SigV4 (AWS CLI sends unsigned).
	req := mustNewRequest(t, http.MethodPost, "http://127.0.0.1:4566/", body)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status=%d want %d body=%q", rec.Code, http.StatusForbidden, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "IdP not configured") &&
		!strings.Contains(rec.Body.String(), "AccessDenied") {
		t.Fatalf("expected IdP not configured / AccessDenied in %q", rec.Body.String())
	}
}

func mustKMSJSON(t *testing.T, handler http.Handler, target string, payload map[string]any, now time.Time) *httptest.ResponseRecorder {
	t.Helper()
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	req := mustNewRequest(t, http.MethodPost, "http://127.0.0.1:4566/", raw)
	req.Header.Set("Content-Type", "application/x-amz-json-1.1")
	req.Header.Set("X-Amz-Target", "TrentService."+target)
	signHeader(t, req, raw, testAccessKey, testSecret, testRegion, "kms", now)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}

func TestKMSEncryptDecryptRoundTrip(t *testing.T) {
	srv, _ := newTestServer(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	createRec := mustKMSJSON(t, handler, "CreateKey", map[string]any{}, now)
	if createRec.Code != http.StatusOK {
		t.Fatalf("CreateKey status=%d body=%q", createRec.Code, createRec.Body.String())
	}
	var createOut map[string]any
	if err := json.Unmarshal(createRec.Body.Bytes(), &createOut); err != nil {
		t.Fatal(err)
	}
	meta, _ := createOut["KeyMetadata"].(map[string]any)
	keyID, _ := meta["KeyId"].(string)
	if keyID == "" {
		t.Fatalf("missing KeyId in %q", createRec.Body.String())
	}

	plain := base64.StdEncoding.EncodeToString([]byte("hello-kms"))
	encRec := mustKMSJSON(t, handler, "Encrypt", map[string]any{
		"KeyId":     keyID,
		"Plaintext": plain,
	}, now)
	if encRec.Code != http.StatusOK {
		t.Fatalf("Encrypt status=%d body=%q", encRec.Code, encRec.Body.String())
	}
	var encOut map[string]any
	if err := json.Unmarshal(encRec.Body.Bytes(), &encOut); err != nil {
		t.Fatal(err)
	}
	blob, _ := encOut["CiphertextBlob"].(string)
	if blob == "" {
		t.Fatalf("missing CiphertextBlob in %q", encRec.Body.String())
	}

	decRec := mustKMSJSON(t, handler, "Decrypt", map[string]any{
		"KeyId":          keyID,
		"CiphertextBlob": blob,
	}, now)
	if decRec.Code != http.StatusOK {
		t.Fatalf("Decrypt status=%d body=%q", decRec.Code, decRec.Body.String())
	}
	var decOut map[string]any
	if err := json.Unmarshal(decRec.Body.Bytes(), &decOut); err != nil {
		t.Fatal(err)
	}
	gotB64, _ := decOut["Plaintext"].(string)
	got, err := base64.StdEncoding.DecodeString(gotB64)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "hello-kms" {
		t.Fatalf("plaintext=%q", got)
	}
}

func TestKMSKeyPolicyDeny(t *testing.T) {
	srv, _ := newTestServer(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	createRec := mustKMSJSON(t, handler, "CreateKey", map[string]any{}, now)
	if createRec.Code != http.StatusOK {
		t.Fatalf("CreateKey status=%d body=%q", createRec.Code, createRec.Body.String())
	}
	var createOut map[string]any
	if err := json.Unmarshal(createRec.Body.Bytes(), &createOut); err != nil {
		t.Fatal(err)
	}
	meta, _ := createOut["KeyMetadata"].(map[string]any)
	keyID, _ := meta["KeyId"].(string)

	denyPolicy := `{"Version":"2012-10-17","Statement":[{"Effect":"Deny","Action":"kms:Encrypt","Resource":"*"}]}`
	putRec := mustKMSJSON(t, handler, "PutKeyPolicy", map[string]any{
		"KeyId":      keyID,
		"PolicyName": "default",
		"Policy":     denyPolicy,
	}, now)
	if putRec.Code != http.StatusOK {
		t.Fatalf("PutKeyPolicy status=%d body=%q", putRec.Code, putRec.Body.String())
	}

	plain := base64.StdEncoding.EncodeToString([]byte("nope"))
	encRec := mustKMSJSON(t, handler, "Encrypt", map[string]any{
		"KeyId":     keyID,
		"Plaintext": plain,
	}, now)
	if encRec.Code != http.StatusForbidden {
		t.Fatalf("Encrypt status=%d want 403 body=%q", encRec.Code, encRec.Body.String())
	}
	if !strings.Contains(encRec.Body.String(), "AccessDeniedException") {
		t.Fatalf("expected AccessDeniedException in %q", encRec.Body.String())
	}
}

func TestKMSAliasEncrypt(t *testing.T) {
	srv, _ := newTestServer(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	createRec := mustKMSJSON(t, handler, "CreateKey", map[string]any{}, now)
	if createRec.Code != http.StatusOK {
		t.Fatalf("CreateKey status=%d body=%q", createRec.Code, createRec.Body.String())
	}
	var createOut map[string]any
	if err := json.Unmarshal(createRec.Body.Bytes(), &createOut); err != nil {
		t.Fatal(err)
	}
	meta, _ := createOut["KeyMetadata"].(map[string]any)
	keyID, _ := meta["KeyId"].(string)

	aliasRec := mustKMSJSON(t, handler, "CreateAlias", map[string]any{
		"AliasName": "alias/lab",
		"KeyId":     keyID,
	}, now)
	if aliasRec.Code != http.StatusOK {
		t.Fatalf("CreateAlias status=%d body=%q", aliasRec.Code, aliasRec.Body.String())
	}

	plain := base64.StdEncoding.EncodeToString([]byte("via-alias"))
	encRec := mustKMSJSON(t, handler, "Encrypt", map[string]any{
		"KeyId":     "alias/lab",
		"Plaintext": plain,
	}, now)
	if encRec.Code != http.StatusOK {
		t.Fatalf("Encrypt via alias status=%d body=%q", encRec.Code, encRec.Body.String())
	}
}

func xmlTag(t *testing.T, xml, tag string) string {
	t.Helper()
	start := strings.Index(xml, "<"+tag+">")
	end := strings.Index(xml, "</"+tag+">")
	if start < 0 || end <= start {
		t.Fatalf("missing <%s> in %q", tag, xml)
	}
	return xml[start+len(tag)+2 : end]
}

func mustNewRequest(t *testing.T, method, rawURL string, body []byte) *http.Request {
	t.Helper()
	var rdr io.Reader
	if body != nil {
		rdr = strings.NewReader(string(body))
	}
	req, err := http.NewRequest(method, rawURL, rdr)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	return req
}

func signHeader(t *testing.T, req *http.Request, body []byte, akid, secret, region, service string, when time.Time) {
	t.Helper()
	amzDate := when.UTC().Format("20060102T150405Z")
	dateStamp := when.UTC().Format("20060102")
	req.Header.Set("X-Amz-Date", amzDate)

	payloadHash := req.Header.Get("X-Amz-Content-Sha256")
	if payloadHash == "" {
		sum := sha256.Sum256(body)
		payloadHash = hex.EncodeToString(sum[:])
		req.Header.Set("X-Amz-Content-Sha256", payloadHash)
	}

	signedHeaders := []string{"content-type", "host", "x-amz-content-sha256", "x-amz-date"}
	sort.Strings(signedHeaders)

	canonicalHeaders := ""
	for _, h := range signedHeaders {
		canonicalHeaders += h + ":" + headerVal(req, h) + "\n"
	}
	signedHeaderList := strings.Join(signedHeaders, ";")

	canonicalRequest := strings.Join([]string{
		req.Method,
		canonicalPath(req),
		"",
		canonicalHeaders,
		signedHeaderList,
		payloadHash,
	}, "\n")

	scope := dateStamp + "/" + region + "/" + service + "/aws4_request"
	stringToSign := strings.Join([]string{
		"AWS4-HMAC-SHA256",
		amzDate,
		scope,
		hexSHA256(canonicalRequest),
	}, "\n")

	sig := hex.EncodeToString(hmacSHA256(deriveKey(secret, dateStamp, region, service), stringToSign))
	req.Header.Set("Authorization", fmt.Sprintf(
		"AWS4-HMAC-SHA256 Credential=%s/%s, SignedHeaders=%s, Signature=%s",
		akid, scope, signedHeaderList, sig,
	))
}

func canonicalPath(req *http.Request) string {
	p := req.URL.EscapedPath()
	if p == "" {
		return "/"
	}
	return p
}

func headerVal(req *http.Request, name string) string {
	if name == "host" {
		return strings.TrimSpace(req.Host)
	}
	return strings.TrimSpace(req.Header.Get(name))
}

func deriveKey(secret, date, region, service string) []byte {
	kDate := hmacSHA256([]byte("AWS4"+secret), date)
	kRegion := hmacSHA256(kDate, region)
	kService := hmacSHA256(kRegion, service)
	return hmacSHA256(kService, "aws4_request")
}

func hmacSHA256(key []byte, data string) []byte {
	m := hmac.New(sha256.New, key)
	_, _ = m.Write([]byte(data))
	return m.Sum(nil)
}

func hexSHA256(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}
