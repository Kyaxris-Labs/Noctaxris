package server_test

import (
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/Kyaxris-Labs/Noctaxris/internal/kernel/sts"
)

func TestGetSessionTokenNoIAMPermissionRequired(t *testing.T) {
	srv, _ := newTestServer(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	iamPost := func(akid, secret, body string) *httptest.ResponseRecorder {
		t.Helper()
		req := mustNewRequest(t, http.MethodPost, "http://127.0.0.1:4566/", []byte(body))
		signHeader(t, req, []byte(body), akid, secret, testRegion, "iam", now)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		return rec
	}
	stsPost := func(akid, secret, body string) *httptest.ResponseRecorder {
		t.Helper()
		req := mustNewRequest(t, http.MethodPost, "http://127.0.0.1:4566/", []byte(body))
		signHeader(t, req, []byte(body), akid, secret, testRegion, "sts", now)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		return rec
	}

	rec := iamPost(testAccessKey, testSecret, "Action=CreateUser&Version=2010-05-08&UserName=noperm")
	if rec.Code != http.StatusOK {
		t.Fatalf("CreateUser: %d %s", rec.Code, rec.Body.String())
	}
	rec = iamPost(testAccessKey, testSecret, "Action=CreateAccessKey&Version=2010-05-08&UserName=noperm")
	if rec.Code != http.StatusOK {
		t.Fatalf("CreateAccessKey: %d %s", rec.Code, rec.Body.String())
	}
	akid := xmlTag(t, rec.Body.String(), "AccessKeyId")
	secret := xmlTag(t, rec.Body.String(), "SecretAccessKey")

	// IAM user with no policies must still GetSessionToken after SigV4 (AWS fidelity).
	rec = stsPost(akid, secret, "Action=GetSessionToken&Version=2011-06-15")
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "<GetSessionTokenResponse") {
		t.Fatalf("GetSessionToken without IAM permission: %d %s", rec.Code, rec.Body.String())
	}
}

func TestMFARegistryAndGetSessionToken(t *testing.T) {
	srv, st, _ := newTestServerStore(t)
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
	stsPost := func(akid, secret, body string) *httptest.ResponseRecorder {
		t.Helper()
		req := mustNewRequest(t, http.MethodPost, "http://127.0.0.1:4566/", []byte(body))
		signHeader(t, req, []byte(body), akid, secret, testRegion, "sts", now)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		return rec
	}

	rec := iamPost("Action=CreateUser&Version=2010-05-08&UserName=mfa-user")
	if rec.Code != http.StatusOK {
		t.Fatalf("CreateUser: %d %s", rec.Code, rec.Body.String())
	}
	rec = iamPost("Action=CreateUser&Version=2010-05-08&UserName=other")
	if rec.Code != http.StatusOK {
		t.Fatalf("CreateUser other: %d %s", rec.Code, rec.Body.String())
	}
	rec = iamPost("Action=CreateAccessKey&Version=2010-05-08&UserName=mfa-user")
	if rec.Code != http.StatusOK {
		t.Fatalf("CreateAccessKey: %d %s", rec.Code, rec.Body.String())
	}
	userAKID := xmlTag(t, rec.Body.String(), "AccessKeyId")
	userSecret := xmlTag(t, rec.Body.String(), "SecretAccessKey")

	rec = iamPost("Action=CreateVirtualMFADevice&Version=2010-05-08")
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "CreateVirtualMFADeviceResponse") {
		t.Fatalf("CreateVirtualMFADevice: %d %s", rec.Code, rec.Body.String())
	}
	serial := xmlTag(t, rec.Body.String(), "SerialNumber")
	seedHex := xmlTag(t, rec.Body.String(), "Base32StringSeed")
	seed, err := hex.DecodeString(seedHex)
	if err != nil || len(seed) == 0 {
		t.Fatalf("decode seed %q: %v", seedHex, err)
	}

	rec = iamPost(fmt.Sprintf(
		"Action=EnableMFADevice&Version=2010-05-08&UserName=mfa-user&SerialNumber=%s",
		url.QueryEscape(serial),
	))
	if rec.Code != http.StatusOK {
		t.Fatalf("EnableMFADevice: %d %s", rec.Code, rec.Body.String())
	}

	rec = iamPost(fmt.Sprintf(
		"Action=EnableMFADevice&Version=2010-05-08&UserName=other&SerialNumber=%s",
		url.QueryEscape(serial),
	))
	if rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), "EntityAlreadyExists") {
		t.Fatalf("reassign EnableMFADevice: %d %s", rec.Code, rec.Body.String())
	}

	rec = iamPost("Action=ListMFADevices&Version=2010-05-08&UserName=mfa-user")
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), serial) {
		t.Fatalf("ListMFADevices: %d %s", rec.Code, rec.Body.String())
	}

	// Without MFA: session has no MFA flag.
	rec = stsPost(userAKID, userSecret, "Action=GetSessionToken&Version=2011-06-15")
	if rec.Code != http.StatusOK {
		t.Fatalf("GetSessionToken no MFA: %d %s", rec.Code, rec.Body.String())
	}
	plainAKID := xmlTag(t, rec.Body.String(), "AccessKeyId")
	ak, err := st.LookupAccessKeyRecord(plainAKID)
	if err != nil {
		t.Fatal(err)
	}
	if ak.MFAAuthenticated {
		t.Fatal("expected MFAAuthenticated=false without TokenCode")
	}

	badBody := fmt.Sprintf(
		"Action=GetSessionToken&Version=2011-06-15&SerialNumber=%s&TokenCode=000000",
		url.QueryEscape(serial),
	)
	rec = stsPost(userAKID, userSecret, badBody)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("bad TokenCode status=%d body=%q", rec.Code, rec.Body.String())
	}

	code := sts.LabTokenCode(seed, now)
	okBody := fmt.Sprintf(
		"Action=GetSessionToken&Version=2011-06-15&SerialNumber=%s&TokenCode=%s",
		url.QueryEscape(serial), code,
	)
	rec = stsPost(userAKID, userSecret, okBody)
	if rec.Code != http.StatusOK {
		t.Fatalf("GetSessionToken with MFA: %d %s", rec.Code, rec.Body.String())
	}
	mfaAKID := xmlTag(t, rec.Body.String(), "AccessKeyId")
	mfaSecret := xmlTag(t, rec.Body.String(), "SecretAccessKey")
	mfaToken := xmlTag(t, rec.Body.String(), "SessionToken")
	ak, err = st.LookupAccessKeyRecord(mfaAKID)
	if err != nil {
		t.Fatal(err)
	}
	if !ak.MFAAuthenticated {
		t.Fatal("expected MFAAuthenticated=true")
	}

	// MFA session unlocks a StringEquals aws:MultiFactorAuthPresent policy.
	mfaPolicy := url.QueryEscape(`{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Action":"iam:ListUsers","Resource":"*","Condition":{"StringEquals":{"aws:MultiFactorAuthPresent":"true"}}}]}`)
	rec = iamPost("Action=CreatePolicy&Version=2010-05-08&PolicyName=MFAListUsers&PolicyDocument=" + mfaPolicy)
	if rec.Code != http.StatusOK {
		t.Fatalf("CreatePolicy: %d %s", rec.Code, rec.Body.String())
	}
	policyARN := xmlTag(t, rec.Body.String(), "Arn")
	rec = iamPost(fmt.Sprintf(
		"Action=AttachUserPolicy&Version=2010-05-08&UserName=mfa-user&PolicyArn=%s",
		url.QueryEscape(policyARN),
	))
	if rec.Code != http.StatusOK {
		t.Fatalf("AttachUserPolicy: %d %s", rec.Code, rec.Body.String())
	}

	listBody := "Action=ListUsers&Version=2010-05-08"
	rec = stsPost(userAKID, userSecret, "Action=GetSessionToken&Version=2011-06-15")
	if rec.Code != http.StatusOK {
		t.Fatalf("GetSessionToken plain for deny check: %d %s", rec.Code, rec.Body.String())
	}
	plainAKID = xmlTag(t, rec.Body.String(), "AccessKeyId")
	plainSecret := xmlTag(t, rec.Body.String(), "SecretAccessKey")
	plainToken := xmlTag(t, rec.Body.String(), "SessionToken")

	req := mustNewRequest(t, http.MethodPost, "http://127.0.0.1:4566/", []byte(listBody))
	signHeader(t, req, []byte(listBody), plainAKID, plainSecret, testRegion, "iam", now)
	req.Header.Set("X-Amz-Security-Token", plainToken)
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code == http.StatusOK {
		t.Fatalf("ListUsers without MFA should be denied, got OK: %s", rec.Body.String())
	}

	req = mustNewRequest(t, http.MethodPost, "http://127.0.0.1:4566/", []byte(listBody))
	signHeader(t, req, []byte(listBody), mfaAKID, mfaSecret, testRegion, "iam", now)
	req.Header.Set("X-Amz-Security-Token", mfaToken)
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "ListUsersResponse") {
		t.Fatalf("ListUsers with MFA session: %d %s", rec.Code, rec.Body.String())
	}

	rec = iamPost(fmt.Sprintf(
		"Action=DeactivateMFADevice&Version=2010-05-08&UserName=mfa-user&SerialNumber=%s",
		url.QueryEscape(serial),
	))
	if rec.Code != http.StatusOK {
		t.Fatalf("DeactivateMFADevice: %d %s", rec.Code, rec.Body.String())
	}
}
