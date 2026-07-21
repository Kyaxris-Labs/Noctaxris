package server_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestIAMCreatePolicyVersionPrivescPath(t *testing.T) {
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

	// Weak managed policy attached to attacker; CreatePolicyVersion + SetAsDefault escalates.
	weakDoc := url.QueryEscape(`{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Action":"sts:GetCallerIdentity","Resource":"*"}]}`)
	rec := iamPost(testAccessKey, testSecret, "Action=CreatePolicy&Version=2010-05-08&PolicyName=WeakAttached&PolicyDocument="+weakDoc)
	if rec.Code != http.StatusOK {
		t.Fatalf("CreatePolicy: %d %s", rec.Code, rec.Body.String())
	}
	policyARN := xmlTag(t, rec.Body.String(), "Arn")

	rec = iamPost(testAccessKey, testSecret, "Action=CreateUser&Version=2010-05-08&UserName=attacker")
	if rec.Code != http.StatusOK {
		t.Fatal(rec.Body.String())
	}
	rec = iamPost(testAccessKey, testSecret, "Action=CreateAccessKey&Version=2010-05-08&UserName=attacker")
	if rec.Code != http.StatusOK {
		t.Fatal(rec.Body.String())
	}
	atkKey := xmlTag(t, rec.Body.String(), "AccessKeyId")
	atkSecret := xmlTag(t, rec.Body.String(), "SecretAccessKey")

	grantDoc := url.QueryEscape(fmt.Sprintf(
		`{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Action":["iam:CreatePolicyVersion","iam:SetDefaultPolicyVersion","iam:GetPolicyVersion","iam:ListPolicyVersions"],"Resource":"%s"}]}`,
		policyARN,
	))
	rec = iamPost(testAccessKey, testSecret, "Action=PutUserPolicy&Version=2010-05-08&UserName=attacker&PolicyName=versioning&PolicyDocument="+grantDoc)
	if rec.Code != http.StatusOK {
		t.Fatalf("PutUserPolicy: %d %s", rec.Code, rec.Body.String())
	}
	rec = iamPost(testAccessKey, testSecret, "Action=AttachUserPolicy&Version=2010-05-08&UserName=attacker&PolicyArn="+url.QueryEscape(policyARN))
	if rec.Code != http.StatusOK {
		t.Fatalf("AttachUserPolicy: %d %s", rec.Code, rec.Body.String())
	}

	adminDoc := url.QueryEscape(`{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Action":"*","Resource":"*"}]}`)
	rec = iamPost(atkKey, atkSecret, fmt.Sprintf(
		"Action=CreatePolicyVersion&Version=2010-05-08&PolicyArn=%s&PolicyDocument=%s&SetAsDefault=true",
		url.QueryEscape(policyARN), adminDoc,
	))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "<VersionId>v2</VersionId>") {
		t.Fatalf("CreatePolicyVersion: %d %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "<IsDefaultVersion>true</IsDefaultVersion>") {
		t.Fatalf("expected default version true: %s", rec.Body.String())
	}

	// Escalated attached policy should now allow ListUsers.
	rec = iamPost(atkKey, atkSecret, "Action=ListUsers&Version=2010-05-08")
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "<ListUsersResponse") {
		t.Fatalf("attacker ListUsers after version privesc: %d %s", rec.Code, rec.Body.String())
	}

	rec = iamPost(atkKey, atkSecret, "Action=ListPolicyVersions&Version=2010-05-08&PolicyArn="+url.QueryEscape(policyARN))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "<VersionId>v1</VersionId>") {
		t.Fatalf("ListPolicyVersions: %d %s", rec.Code, rec.Body.String())
	}
}
