package sdk_test

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"testing"
	"time"
)

// signedJSONTarget POSTs application/x-amz-json-1.1 with X-Amz-Target for lab
// services that are not AWS REST/XML (CloudFront, Route53, ELBv2, Transfer PutFile).
func signedJSONTarget(t *testing.T, service, target string, payload any) (status int, body []byte, parsed map[string]any) {
	t.Helper()
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	req, err := http.NewRequest(http.MethodPost, endpoint()+"/", bytes.NewReader(raw))
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	req.Header.Set("Content-Type", "application/x-amz-json-1.1")
	req.Header.Set("X-Amz-Target", target)
	signV4(t, req, raw, service)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	defer resp.Body.Close()
	body, err = io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if len(body) > 0 {
		_ = json.Unmarshal(body, &parsed)
	}
	return resp.StatusCode, body, parsed
}

// signedHTTP performs a SigV4-signed request for lab path APIs (CloudFront edge, Transfer home, OpenSearch query).
func signedHTTP(t *testing.T, service, method, path string, body []byte, contentType string) (status int, respBody []byte) {
	t.Helper()
	u, err := url.Parse(endpoint() + path)
	if err != nil {
		t.Fatalf("parse url: %v", err)
	}
	var rdr io.Reader
	if body != nil {
		rdr = bytes.NewReader(body)
	}
	req, err := http.NewRequest(method, u.String(), rdr)
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	signV4(t, req, body, service)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	defer resp.Body.Close()
	respBody, err = io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	return resp.StatusCode, respBody
}

func signV4(t *testing.T, req *http.Request, body []byte, service string) {
	t.Helper()
	akid := envOr("AWS_ACCESS_KEY_ID", "AKIAROOTEXAMPLE01")
	secret := envOr("AWS_SECRET_ACCESS_KEY", "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY")
	region := envOr("AWS_DEFAULT_REGION", "us-east-1")
	when := time.Now().UTC()
	amzDate := when.Format("20060102T150405Z")
	dateStamp := when.Format("20060102")
	req.Header.Set("X-Amz-Date", amzDate)
	if body == nil {
		body = []byte{}
	}
	sum := sha256.Sum256(body)
	payloadHash := hex.EncodeToString(sum[:])
	req.Header.Set("X-Amz-Content-Sha256", payloadHash)

	signedHeaders := []string{"host", "x-amz-content-sha256", "x-amz-date"}
	if req.Header.Get("Content-Type") != "" {
		signedHeaders = append(signedHeaders, "content-type")
	}
	if req.Header.Get("X-Amz-Target") != "" {
		signedHeaders = append(signedHeaders, "x-amz-target")
	}
	sort.Strings(signedHeaders)

	canonicalHeaders := ""
	for _, h := range signedHeaders {
		canonicalHeaders += h + ":" + headerValue(req, h) + "\n"
	}
	signedHeaderList := strings.Join(signedHeaders, ";")
	path := req.URL.EscapedPath()
	if path == "" {
		path = "/"
	}
	canonicalRequest := strings.Join([]string{
		req.Method,
		path,
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
		hexSHA256String(canonicalRequest),
	}, "\n")
	sig := hex.EncodeToString(hmacSHA256Bytes(deriveSigningKey(secret, dateStamp, region, service), stringToSign))
	req.Header.Set("Authorization", fmt.Sprintf(
		"AWS4-HMAC-SHA256 Credential=%s/%s, SignedHeaders=%s, Signature=%s",
		akid, scope, signedHeaderList, sig,
	))
}

func headerValue(req *http.Request, name string) string {
	if name == "host" {
		return strings.TrimSpace(req.Host)
	}
	return strings.TrimSpace(req.Header.Get(name))
}

func hexSHA256String(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

func hmacSHA256Bytes(key []byte, data string) []byte {
	m := hmac.New(sha256.New, key)
	_, _ = m.Write([]byte(data))
	return m.Sum(nil)
}

func deriveSigningKey(secret, date, region, service string) []byte {
	kDate := hmacSHA256Bytes([]byte("AWS4"+secret), date)
	kRegion := hmacSHA256Bytes(kDate, region)
	kService := hmacSHA256Bytes(kRegion, service)
	return hmacSHA256Bytes(kService, "aws4_request")
}
