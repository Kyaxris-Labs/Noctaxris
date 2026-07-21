package server_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/Kyaxris-Labs/Noctaxris/internal/kernel/sts"
)

func TestAssumeRoleExternalIdEnforced(t *testing.T) {
	srv, st, _ := newTestServerStore(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	roleName := "extid-role"
	roleARN := "arn:aws:iam::" + testAccountID + ":role/" + roleName
	trust := `{
		"Version":"2012-10-17",
		"Statement":[{
			"Effect":"Allow",
			"Principal":{"AWS":"arn:aws:iam::` + testAccountID + `:root"},
			"Action":"sts:AssumeRole",
			"Condition":{"StringEquals":{"sts:ExternalId":"lab-external-id"}}
		}]
	}`
	if err := st.PutRole(testAccountID, roleName, roleARN, trust); err != nil {
		t.Fatal(err)
	}

	denyBody := []byte("Action=AssumeRole&Version=2011-06-15&RoleArn=" + url.QueryEscape(roleARN) +
		"&RoleSessionName=noext")
	req := mustNewRequest(t, http.MethodPost, "http://127.0.0.1:4566/", denyBody)
	signHeader(t, req, denyBody, testAccessKey, testSecret, testRegion, "sts", now)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code == http.StatusOK {
		t.Fatalf("AssumeRole without ExternalId should Deny, got OK: %s", rec.Body.String())
	}

	allowBody := []byte("Action=AssumeRole&Version=2011-06-15&RoleArn=" + url.QueryEscape(roleARN) +
		"&RoleSessionName=withext&ExternalId=lab-external-id")
	req = mustNewRequest(t, http.MethodPost, "http://127.0.0.1:4566/", allowBody)
	signHeader(t, req, allowBody, testAccessKey, testSecret, testRegion, "sts", now)
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "ASIA") {
		t.Fatalf("AssumeRole with ExternalId: %d %s", rec.Code, rec.Body.String())
	}
}

func TestGetFederationTokenIdentitySessionIntersection(t *testing.T) {
	srv, st, _ := newTestServerStore(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	if _, _, err := st.CreateUser(testAccountID, "fed-user"); err != nil {
		t.Fatal(err)
	}
	userARN := fmt.Sprintf("arn:aws:iam::%s:user/fed-user", testAccountID)
	identityDoc := `{"Version":"2012-10-17","Statement":[
		{"Effect":"Allow","Action":["sts:GetFederationToken","sts:DecodeAuthorizationMessage"],"Resource":"*"}
	]}`
	if err := st.PutInlinePolicy(userARN, "fed-id", identityDoc); err != nil {
		t.Fatal(err)
	}
	akid, secret, err := st.CreateUserAccessKey(testAccountID, "fed-user")
	if err != nil {
		t.Fatal(err)
	}

	sessionAllow := `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Action":"sts:DecodeAuthorizationMessage","Resource":"*"}]}`
	fedBody := []byte("Action=GetFederationToken&Version=2011-06-15&Name=broker&Policy=" + url.QueryEscape(sessionAllow))
	req := mustNewRequest(t, http.MethodPost, "http://127.0.0.1:4566/", fedBody)
	signHeader(t, req, fedBody, akid, secret, testRegion, "sts", now)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("GetFederationToken with session: %d %s", rec.Code, rec.Body.String())
	}
	fedAKID := xmlTag(t, rec.Body.String(), "AccessKeyId")
	fedSecret := xmlTag(t, rec.Body.String(), "SecretAccessKey")
	fedToken := xmlTag(t, rec.Body.String(), "SessionToken")

	encoded := sts.EncodeAuthorizationMessage("AccessDenied", "lab")
	decodeBody := []byte("Action=DecodeAuthorizationMessage&Version=2011-06-15&EncodedMessage=" +
		url.QueryEscape(encoded))
	req = mustNewRequest(t, http.MethodPost, "http://127.0.0.1:4566/", decodeBody)
	signHeader(t, req, decodeBody, fedAKID, fedSecret, testRegion, "sts", now)
	req.Header.Set("X-Amz-Security-Token", fedToken)
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("Decode with identity∩session Allow: %d %s", rec.Code, rec.Body.String())
	}

	emptyBody := []byte("Action=GetFederationToken&Version=2011-06-15&Name=empty")
	req = mustNewRequest(t, http.MethodPost, "http://127.0.0.1:4566/", emptyBody)
	signHeader(t, req, emptyBody, akid, secret, testRegion, "sts", now)
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("GetFederationToken empty session mint: %d %s", rec.Code, rec.Body.String())
	}
	emptyAKID := xmlTag(t, rec.Body.String(), "AccessKeyId")
	emptySecret := xmlTag(t, rec.Body.String(), "SecretAccessKey")
	emptyToken := xmlTag(t, rec.Body.String(), "SessionToken")

	req = mustNewRequest(t, http.MethodPost, "http://127.0.0.1:4566/", decodeBody)
	signHeader(t, req, decodeBody, emptyAKID, emptySecret, testRegion, "sts", now)
	req.Header.Set("X-Amz-Security-Token", emptyToken)
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code == http.StatusOK {
		t.Fatalf("empty session federation token should Deny identity APIs, got OK: %s", rec.Body.String())
	}
}
