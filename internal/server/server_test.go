package server_test

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
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
	if !strings.Contains(rec.Body.String(), "Phase 1") {
		t.Fatalf("expected Phase 1 message in %q", rec.Body.String())
	}

	data, err := os.ReadFile(filepath.Join(auditDir, "events.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `"errorCode":"NotImplemented"`) {
		t.Fatalf("expected NotImplemented audit in %q", data)
	}
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
