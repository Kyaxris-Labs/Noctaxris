package authn_test

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/Kyaxris-Labs/Noctaxris/internal/kernel/authn"
	"github.com/Kyaxris-Labs/Noctaxris/internal/kernel/identity"
)

const (
	testAKID   = "AKIAIOSFODNN7EXAMPLE"
	testSecret = "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY"
	testAcct   = "123456789012"
	testRegion = "us-east-1"
	testSvc    = "sts"
)

func fixedNow() time.Time {
	return time.Date(2024, 6, 15, 12, 0, 0, 0, time.UTC)
}

func lookupOK(accessKeyID string) (authn.ResolvedKey, error) {
	if accessKeyID != testAKID {
		return authn.ResolvedKey{}, fmt.Errorf("unknown key")
	}
	return authn.ResolvedKey{
		AccountID: testAcct,
		Secret:    testSecret,
		IsRoot:    true,
	}, nil
}

func TestVerifyHeaderSignedRequest(t *testing.T) {
	now := fixedNow()
	body := []byte("Action=GetCallerIdentity&Version=2011-06-15")
	req := mustNewRequest(t, http.MethodPost, "http://127.0.0.1:4566/", body)
	signHeader(t, req, body, testAKID, testSecret, testRegion, testSvc, now)

	got, err := authn.Verify(req, body, now, 15*time.Minute, lookupOK)
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if got.AccessKeyID != testAKID {
		t.Fatalf("AccessKeyID = %q", got.AccessKeyID)
	}
	if got.AccountID != testAcct {
		t.Fatalf("AccountID = %q", got.AccountID)
	}
	if got.Region != testRegion || got.Service != testSvc {
		t.Fatalf("scope = %s/%s", got.Region, got.Service)
	}
	if !got.Principal.IsRoot {
		t.Fatal("expected root principal")
	}
	if got.SecretAccessKey != testSecret {
		t.Fatal("secret mismatch")
	}
}

func TestVerifyQuerySignedRequest(t *testing.T) {
	now := fixedNow()
	req := mustNewRequest(t, http.MethodGet, "http://127.0.0.1:4566/?Action=GetCallerIdentity&Version=2011-06-15", nil)
	signQuery(t, req, nil, testAKID, testSecret, testRegion, testSvc, now, 900)

	got, err := authn.Verify(req, nil, now, 15*time.Minute, lookupOK)
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if got.AccessKeyID != testAKID || got.Service != testSvc {
		t.Fatalf("got %+v", got)
	}
}

func TestVerifyWrongSignature(t *testing.T) {
	now := fixedNow()
	body := []byte("Action=GetCallerIdentity&Version=2011-06-15")
	req := mustNewRequest(t, http.MethodPost, "http://127.0.0.1:4566/", body)
	signHeader(t, req, body, testAKID, testSecret, testRegion, testSvc, now)

	auth := req.Header.Get("Authorization")
	idx := strings.Index(auth, "Signature=")
	req.Header.Set("Authorization", auth[:idx]+"Signature="+strings.Repeat("ab", 32))

	_, err := authn.Verify(req, body, now, 15*time.Minute, lookupOK)
	if authn.Code(err) != authn.CodeSignatureDoesNotMatch {
		t.Fatalf("Code = %q, err = %v", authn.Code(err), err)
	}
}

func TestVerifyUnknownKey(t *testing.T) {
	now := fixedNow()
	body := []byte("Action=GetCallerIdentity&Version=2011-06-15")
	req := mustNewRequest(t, http.MethodPost, "http://127.0.0.1:4566/", body)
	signHeader(t, req, body, testAKID, testSecret, testRegion, testSvc, now)

	_, err := authn.Verify(req, body, now, 15*time.Minute, func(string) (authn.ResolvedKey, error) {
		return authn.ResolvedKey{}, fmt.Errorf("not found")
	})
	if authn.Code(err) != authn.CodeInvalidClientTokenId {
		t.Fatalf("Code = %q, err = %v", authn.Code(err), err)
	}
}

func TestVerifySkewedTime(t *testing.T) {
	now := fixedNow()
	body := []byte("Action=GetCallerIdentity&Version=2011-06-15")
	req := mustNewRequest(t, http.MethodPost, "http://127.0.0.1:4566/", body)
	signHeader(t, req, body, testAKID, testSecret, testRegion, testSvc, now)

	serverNow := now.Add(20 * time.Minute)
	_, err := authn.Verify(req, body, serverNow, 15*time.Minute, lookupOK)
	if authn.Code(err) != authn.CodeRequestTimeTooSkewed {
		t.Fatalf("Code = %q, err = %v", authn.Code(err), err)
	}
}

func TestVerifyExpiresOverMax(t *testing.T) {
	now := fixedNow()
	req := mustNewRequest(t, http.MethodGet, "http://127.0.0.1:4566/bucket/key", nil)
	signQuery(t, req, nil, testAKID, testSecret, testRegion, "s3", now, 604801)
	_, err := authn.Verify(req, nil, now, 15*time.Minute, lookupOK)
	if authn.Code(err) != authn.CodeSignatureDoesNotMatch {
		t.Fatalf("Code = %q, err = %v", authn.Code(err), err)
	}
}

func TestVerifyMissingAuth(t *testing.T) {
	req := mustNewRequest(t, http.MethodPost, "http://127.0.0.1:4566/", nil)
	_, err := authn.Verify(req, nil, fixedNow(), 15*time.Minute, lookupOK)
	if authn.Code(err) != authn.CodeMissingAuthenticationToken {
		t.Fatalf("Code = %q, err = %v", authn.Code(err), err)
	}
}

func TestVerifyUnsignedPayloadHeaderRejected(t *testing.T) {
	now := fixedNow()
	body := []byte("ignored-for-unsigned")
	req := mustNewRequest(t, http.MethodPost, "http://127.0.0.1:4566/", body)
	req.Header.Set("X-Amz-Content-Sha256", "UNSIGNED-PAYLOAD")
	signHeader(t, req, body, testAKID, testSecret, testRegion, testSvc, now)

	_, err := authn.Verify(req, body, now, 15*time.Minute, lookupOK)
	if authn.Code(err) != authn.CodeSignatureDoesNotMatch {
		t.Fatalf("Code = %q, err = %v", authn.Code(err), err)
	}
}

func TestVerifyBodyHashMismatch(t *testing.T) {
	now := fixedNow()
	signedBody := []byte("Action=GetCallerIdentity&Version=2011-06-15")
	swapped := []byte("Action=AssumeRole&Version=2011-06-15&RoleArn=arn:aws:iam::123456789012:role/Evil")
	req := mustNewRequest(t, http.MethodPost, "http://127.0.0.1:4566/", signedBody)
	signHeader(t, req, signedBody, testAKID, testSecret, testRegion, testSvc, now)

	_, err := authn.Verify(req, swapped, now, 15*time.Minute, lookupOK)
	if authn.Code(err) != authn.CodeSignatureDoesNotMatch {
		t.Fatalf("Code = %q, err = %v", authn.Code(err), err)
	}
}

func TestVerifyS3QueryUnsignedPayloadOK(t *testing.T) {
	now := fixedNow()
	req := mustNewRequest(t, http.MethodGet, "http://127.0.0.1:4566/bucket/key", nil)
	signQueryUnsigned(t, req, testAKID, testSecret, testRegion, "s3", now, 900)

	if _, err := authn.Verify(req, nil, now, 15*time.Minute, lookupOK); err != nil {
		t.Fatalf("Verify S3 query UNSIGNED-PAYLOAD: %v", err)
	}
}

func TestVerifyRequiresSessionToken(t *testing.T) {
	now := fixedNow()
	body := []byte("Action=GetCallerIdentity&Version=2011-06-15")
	req := mustNewRequest(t, http.MethodPost, "http://127.0.0.1:4566/", body)
	signHeader(t, req, body, testAKID, testSecret, testRegion, testSvc, now)

	lookupTemp := func(string) (authn.ResolvedKey, error) {
		return authn.ResolvedKey{
			AccountID:    testAcct,
			Secret:       testSecret,
			SessionToken: "session-token-value",
			RoleARN:      "arn:aws:iam::" + testAcct + ":role/OrganizationAccountAccessRole",
			SessionName:  "admin",
			ExpiresAt:    now.Add(time.Hour),
		}, nil
	}

	_, err := authn.Verify(req, body, now, 15*time.Minute, lookupTemp)
	if authn.Code(err) != authn.CodeInvalidClientTokenId {
		t.Fatalf("missing token Code=%q err=%v", authn.Code(err), err)
	}

	req2 := mustNewRequest(t, http.MethodPost, "http://127.0.0.1:4566/", body)
	req2.Header.Set("X-Amz-Security-Token", "session-token-value")
	signHeader(t, req2, body, testAKID, testSecret, testRegion, testSvc, now)
	// Re-set token after sign (signHeader does not include security token in signed headers for this helper)
	req2.Header.Set("X-Amz-Security-Token", "session-token-value")

	got, err := authn.Verify(req2, body, now, 15*time.Minute, lookupTemp)
	if err != nil {
		t.Fatalf("Verify with token: %v", err)
	}
	if got.Principal.Kind != identity.KindRole || got.Principal.SessionName != "admin" {
		t.Fatalf("principal=%+v", got.Principal)
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

// Independent SigV4 header signer for tests (not production code).
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
		"", // no query for these POSTs
		canonicalHeaders,
		signedHeaderList,
		payloadHash,
	}, "\n")

	scope := dateStamp + "/" + region + "/" + service + "/aws4_request"
	sts := strings.Join([]string{
		"AWS4-HMAC-SHA256",
		amzDate,
		scope,
		hexSHA256(canonicalRequest),
	}, "\n")

	sig := hex.EncodeToString(hmacSHA256(deriveKey(secret, dateStamp, region, service), sts))
	req.Header.Set("Authorization", fmt.Sprintf(
		"AWS4-HMAC-SHA256 Credential=%s/%s, SignedHeaders=%s, Signature=%s",
		akid, scope, signedHeaderList, sig,
	))
}

func signQuery(t *testing.T, req *http.Request, body []byte, akid, secret, region, service string, when time.Time, expires int) {
	t.Helper()
	amzDate := when.UTC().Format("20060102T150405Z")
	dateStamp := when.UTC().Format("20060102")
	scope := dateStamp + "/" + region + "/" + service + "/aws4_request"
	cred := akid + "/" + scope

	q := req.URL.Query()
	q.Set("X-Amz-Algorithm", "AWS4-HMAC-SHA256")
	q.Set("X-Amz-Credential", cred)
	q.Set("X-Amz-Date", amzDate)
	q.Set("X-Amz-Expires", fmt.Sprintf("%d", expires))
	q.Set("X-Amz-SignedHeaders", "host")
	req.URL.RawQuery = q.Encode()

	payloadHash := "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"
	if len(body) > 0 {
		sum := sha256.Sum256(body)
		payloadHash = hex.EncodeToString(sum[:])
	}

	canonicalHeaders := "host:" + headerVal(req, "host") + "\n"
	canonicalRequest := strings.Join([]string{
		req.Method,
		canonicalPath(req),
		canonicalQuery(req.URL.Query(), true),
		canonicalHeaders,
		"host",
		payloadHash,
	}, "\n")

	sts := strings.Join([]string{
		"AWS4-HMAC-SHA256",
		amzDate,
		scope,
		hexSHA256(canonicalRequest),
	}, "\n")

	sig := hex.EncodeToString(hmacSHA256(deriveKey(secret, dateStamp, region, service), sts))
	q.Set("X-Amz-Signature", sig)
	req.URL.RawQuery = q.Encode()
}

func signQueryUnsigned(t *testing.T, req *http.Request, akid, secret, region, service string, when time.Time, expires int) {
	t.Helper()
	amzDate := when.UTC().Format("20060102T150405Z")
	dateStamp := when.UTC().Format("20060102")
	scope := dateStamp + "/" + region + "/" + service + "/aws4_request"
	cred := akid + "/" + scope

	q := req.URL.Query()
	q.Set("X-Amz-Algorithm", "AWS4-HMAC-SHA256")
	q.Set("X-Amz-Credential", cred)
	q.Set("X-Amz-Date", amzDate)
	q.Set("X-Amz-Expires", fmt.Sprintf("%d", expires))
	q.Set("X-Amz-SignedHeaders", "host")
	req.URL.RawQuery = q.Encode()

	payloadHash := "UNSIGNED-PAYLOAD"
	canonicalHeaders := "host:" + headerVal(req, "host") + "\n"
	canonicalRequest := strings.Join([]string{
		req.Method,
		canonicalPath(req),
		canonicalQuery(req.URL.Query(), true),
		canonicalHeaders,
		"host",
		payloadHash,
	}, "\n")

	sts := strings.Join([]string{
		"AWS4-HMAC-SHA256",
		amzDate,
		scope,
		hexSHA256(canonicalRequest),
	}, "\n")

	sig := hex.EncodeToString(hmacSHA256(deriveKey(secret, dateStamp, region, service), sts))
	q.Set("X-Amz-Signature", sig)
	req.URL.RawQuery = q.Encode()
}

func canonicalPath(req *http.Request) string {
	p := req.URL.EscapedPath()
	if p == "" {
		return "/"
	}
	return p
}

func canonicalQuery(q url.Values, excludeSignature bool) string {
	type kv struct{ k, v string }
	var pairs []kv
	for key, values := range q {
		if excludeSignature && strings.EqualFold(key, "X-Amz-Signature") {
			continue
		}
		ek := uriEncode(key)
		if len(values) == 0 {
			pairs = append(pairs, kv{ek, ""})
			continue
		}
		for _, v := range values {
			pairs = append(pairs, kv{ek, uriEncode(v)})
		}
	}
	sort.Slice(pairs, func(i, j int) bool {
		if pairs[i].k == pairs[j].k {
			return pairs[i].v < pairs[j].v
		}
		return pairs[i].k < pairs[j].k
	})
	parts := make([]string, len(pairs))
	for i, p := range pairs {
		parts[i] = p.k + "=" + p.v
	}
	return strings.Join(parts, "&")
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

func uriEncode(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		c := s[i]
		if (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') ||
			c == '-' || c == '_' || c == '.' || c == '~' {
			b.WriteByte(c)
			continue
		}
		fmt.Fprintf(&b, "%%%02X", c)
	}
	return b.String()
}
