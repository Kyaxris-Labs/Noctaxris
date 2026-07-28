package server_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestIAMDeleteUserDenyOnSpecificUserARN(t *testing.T) {
	srv, st, _ := newTestServerStore(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	_, targetARN, err := st.CreateUser(testAccountID, "protected-user")
	if err != nil {
		t.Fatal(err)
	}
	_, actorARN, err := st.CreateUser(testAccountID, "iam-admin")
	if err != nil {
		t.Fatal(err)
	}
	akid, secret, err := st.CreateUserAccessKey(testAccountID, "iam-admin")
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := st.CreateUser(testAccountID, "other-user"); err != nil {
		t.Fatal(err)
	}

	policy := fmt.Sprintf(`{
		"Version":"2012-10-17",
		"Statement":[
			{"Effect":"Allow","Action":"iam:*","Resource":"*"},
			{"Effect":"Deny","Action":"iam:DeleteUser","Resource":"%s"}
		]
	}`, targetARN)
	if err := st.PutInlinePolicy(actorARN, "scoped-deny", policy); err != nil {
		t.Fatal(err)
	}

	iamPost := func(body string) *httptest.ResponseRecorder {
		t.Helper()
		req := mustNewRequest(t, http.MethodPost, "http://127.0.0.1:4566/", []byte(body))
		signHeader(t, req, []byte(body), akid, secret, testRegion, "iam", now)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		return rec
	}

	rec := iamPost("Action=DeleteUser&Version=2010-05-08&UserName=protected-user")
	if rec.Code != http.StatusForbidden || !strings.Contains(rec.Body.String(), "AccessDenied") {
		t.Fatalf("DeleteUser protected-user want 403 AccessDenied got status=%d body=%q", rec.Code, rec.Body.String())
	}
	if _, err := st.GetUser(testAccountID, "protected-user"); err != nil {
		t.Fatalf("protected-user must still exist after deny: %v", err)
	}

	rec = iamPost("Action=DeleteUser&Version=2010-05-08&UserName=other-user")
	if rec.Code != http.StatusOK {
		t.Fatalf("DeleteUser other-user want 200 got status=%d body=%q", rec.Code, rec.Body.String())
	}
}

func TestIAMDeleteAccessKeyRejectsRootKey(t *testing.T) {
	srv, _ := newTestServer(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	body := "Action=DeleteAccessKey&Version=2010-05-08&AccessKeyId=" + testAccessKey
	req := mustNewRequest(t, http.MethodPost, "http://127.0.0.1:4566/", []byte(body))
	signHeader(t, req, []byte(body), testAccessKey, testSecret, testRegion, "iam", now)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden || !strings.Contains(rec.Body.String(), "AccessDenied") {
		t.Fatalf("DeleteAccessKey root want 403 AccessDenied got status=%d body=%q", rec.Code, rec.Body.String())
	}
}

// Deny on owner user ARN must bind even when request UserName names a different user.
func TestIAMDeleteAccessKeyDenyIgnoresMismatchedUserName(t *testing.T) {
	srv, st, _ := newTestServerStore(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	_, protectedARN, err := st.CreateUser(testAccountID, "protected-user")
	if err != nil {
		t.Fatal(err)
	}
	protectedKey, _, err := st.CreateUserAccessKey(testAccountID, "protected-user")
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := st.CreateUser(testAccountID, "decoy-user"); err != nil {
		t.Fatal(err)
	}
	decoyKey, _, err := st.CreateUserAccessKey(testAccountID, "decoy-user")
	if err != nil {
		t.Fatal(err)
	}
	_, actorARN, err := st.CreateUser(testAccountID, "iam-admin")
	if err != nil {
		t.Fatal(err)
	}
	akid, secret, err := st.CreateUserAccessKey(testAccountID, "iam-admin")
	if err != nil {
		t.Fatal(err)
	}

	policy := fmt.Sprintf(`{
		"Version":"2012-10-17",
		"Statement":[
			{"Effect":"Allow","Action":"iam:*","Resource":"*"},
			{"Effect":"Deny","Action":"iam:DeleteAccessKey","Resource":"%s"}
		]
	}`, protectedARN)
	if err := st.PutInlinePolicy(actorARN, "scoped-deny", policy); err != nil {
		t.Fatal(err)
	}

	iamPost := func(body string) *httptest.ResponseRecorder {
		t.Helper()
		req := mustNewRequest(t, http.MethodPost, "http://127.0.0.1:4566/", []byte(body))
		signHeader(t, req, []byte(body), akid, secret, testRegion, "iam", now)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		return rec
	}

	// Bypass attempt: claim decoy UserName while targeting protected-user's key.
	bypassBody := "Action=DeleteAccessKey&Version=2010-05-08&UserName=decoy-user&AccessKeyId=" + protectedKey
	rec := iamPost(bypassBody)
	if rec.Code != http.StatusForbidden || !strings.Contains(rec.Body.String(), "AccessDenied") {
		t.Fatalf("DeleteAccessKey mismatched UserName want 403 AccessDenied got status=%d body=%q",
			rec.Code, rec.Body.String())
	}
	if _, err := st.LookupAccessKeyRecord(protectedKey); err != nil {
		t.Fatalf("protected key must still exist after deny bypass attempt: %v", err)
	}

	rec = iamPost("Action=DeleteAccessKey&Version=2010-05-08&UserName=decoy-user&AccessKeyId=" + decoyKey)
	if rec.Code != http.StatusOK {
		t.Fatalf("DeleteAccessKey decoy key want 200 got status=%d body=%q", rec.Code, rec.Body.String())
	}
}

func TestGetAccessKeyInfoRequiresAuthorize(t *testing.T) {
	srv, st, _ := newTestServerStore(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	_, userARN, err := st.CreateUser(testAccountID, "sts-denied")
	if err != nil {
		t.Fatal(err)
	}
	akid, secret, err := st.CreateUserAccessKey(testAccountID, "sts-denied")
	if err != nil {
		t.Fatal(err)
	}
	// IAM allow only; no sts:GetAccessKeyInfo.
	if err := st.PutInlinePolicy(userARN, "iam-only", `{
		"Version":"2012-10-17",
		"Statement":[{"Effect":"Allow","Action":"iam:*","Resource":"*"}]
	}`); err != nil {
		t.Fatal(err)
	}

	body := "Action=GetAccessKeyInfo&Version=2011-06-15&AccessKeyId=" + testAccessKey
	req := mustNewRequest(t, http.MethodPost, "http://127.0.0.1:4566/", []byte(body))
	signHeader(t, req, []byte(body), akid, secret, testRegion, "sts", now)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden || !strings.Contains(rec.Body.String(), "AccessDenied") {
		t.Fatalf("GetAccessKeyInfo without sts permission want 403 got status=%d body=%q", rec.Code, rec.Body.String())
	}
}

// Deny on account user/* must bind ListUsers (authorize resource is not bare "*").
func TestIAMListUsersDenyOnUserWildcard(t *testing.T) {
	srv, st, _ := newTestServerStore(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	_, actorARN, err := st.CreateUser(testAccountID, "list-denied")
	if err != nil {
		t.Fatal(err)
	}
	akid, secret, err := st.CreateUserAccessKey(testAccountID, "list-denied")
	if err != nil {
		t.Fatal(err)
	}
	policy := fmt.Sprintf(`{
		"Version":"2012-10-17",
		"Statement":[
			{"Effect":"Allow","Action":"iam:*","Resource":"*"},
			{"Effect":"Deny","Action":"iam:ListUsers","Resource":"arn:aws:iam::%s:user/*"}
		]
	}`, testAccountID)
	if err := st.PutInlinePolicy(actorARN, "list-deny", policy); err != nil {
		t.Fatal(err)
	}

	body := "Action=ListUsers&Version=2010-05-08"
	req := mustNewRequest(t, http.MethodPost, "http://127.0.0.1:4566/", []byte(body))
	signHeader(t, req, []byte(body), akid, secret, testRegion, "iam", now)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden || !strings.Contains(rec.Body.String(), "AccessDenied") {
		t.Fatalf("ListUsers want 403 AccessDenied got status=%d body=%q", rec.Code, rec.Body.String())
	}
}

// Deny on account role/* must bind ListRoles (authorize resource is not bare "*").
func TestIAMListRolesDenyOnRoleWildcard(t *testing.T) {
	srv, st, _ := newTestServerStore(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	_, actorARN, err := st.CreateUser(testAccountID, "role-list-denied")
	if err != nil {
		t.Fatal(err)
	}
	akid, secret, err := st.CreateUserAccessKey(testAccountID, "role-list-denied")
	if err != nil {
		t.Fatal(err)
	}
	policy := fmt.Sprintf(`{
		"Version":"2012-10-17",
		"Statement":[
			{"Effect":"Allow","Action":"iam:*","Resource":"*"},
			{"Effect":"Deny","Action":"iam:ListRoles","Resource":"arn:aws:iam::%s:role/*"}
		]
	}`, testAccountID)
	if err := st.PutInlinePolicy(actorARN, "role-list-deny", policy); err != nil {
		t.Fatal(err)
	}

	body := "Action=ListRoles&Version=2010-05-08"
	req := mustNewRequest(t, http.MethodPost, "http://127.0.0.1:4566/", []byte(body))
	signHeader(t, req, []byte(body), akid, secret, testRegion, "iam", now)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden || !strings.Contains(rec.Body.String(), "AccessDenied") {
		t.Fatalf("ListRoles want 403 AccessDenied got status=%d body=%q", rec.Code, rec.Body.String())
	}
}

// Deny on account root must bind credential report APIs (authorize resource is not bare "*").
func TestIAMCredentialReportDenyOnAccountRoot(t *testing.T) {
	srv, st, _ := newTestServerStore(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	_, actorARN, err := st.CreateUser(testAccountID, "cred-report-denied")
	if err != nil {
		t.Fatal(err)
	}
	akid, secret, err := st.CreateUserAccessKey(testAccountID, "cred-report-denied")
	if err != nil {
		t.Fatal(err)
	}
	policy := fmt.Sprintf(`{
		"Version":"2012-10-17",
		"Statement":[
			{"Effect":"Allow","Action":"iam:*","Resource":"*"},
			{"Effect":"Deny","Action":["iam:GenerateCredentialReport","iam:GetCredentialReport"],"Resource":"arn:aws:iam::%s:root"}
		]
	}`, testAccountID)
	if err := st.PutInlinePolicy(actorARN, "cred-deny", policy); err != nil {
		t.Fatal(err)
	}

	iamPost := func(body string) *httptest.ResponseRecorder {
		t.Helper()
		req := mustNewRequest(t, http.MethodPost, "http://127.0.0.1:4566/", []byte(body))
		signHeader(t, req, []byte(body), akid, secret, testRegion, "iam", now)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		return rec
	}

	rec := iamPost("Action=GenerateCredentialReport&Version=2010-05-08")
	if rec.Code != http.StatusForbidden || !strings.Contains(rec.Body.String(), "AccessDenied") {
		t.Fatalf("GenerateCredentialReport want 403 AccessDenied got status=%d body=%q", rec.Code, rec.Body.String())
	}
	rec = iamPost("Action=GetCredentialReport&Version=2010-05-08")
	if rec.Code != http.StatusForbidden || !strings.Contains(rec.Body.String(), "AccessDenied") {
		t.Fatalf("GetCredentialReport want 403 AccessDenied got status=%d body=%q", rec.Code, rec.Body.String())
	}
}
