package server_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func mustS3WithCreds(
	t *testing.T,
	handler http.Handler,
	method, rawURL string,
	body []byte,
	akid, secret, service string,
	now time.Time,
	extraHeaders map[string]string,
) *httptest.ResponseRecorder {
	t.Helper()
	req := mustNewRequest(t, method, rawURL, body)
	if body == nil {
		req.Header.Del("Content-Type")
	} else if extraHeaders == nil || extraHeaders["Content-Type"] == "" {
		req.Header.Set("Content-Type", "application/octet-stream")
	}
	for k, v := range extraHeaders {
		req.Header.Set(k, v)
	}
	signS3Header(t, req, body, akid, secret, testRegion, service, now)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}

func mustLambdaJSONWithCreds(
	t *testing.T,
	handler http.Handler,
	target string,
	payload map[string]any,
	akid, secret string,
	now time.Time,
) *httptest.ResponseRecorder {
	t.Helper()
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	req := mustNewRequest(t, http.MethodPost, "http://127.0.0.1:4566/", raw)
	req.Header.Set("Content-Type", "application/x-amz-json-1.1")
	req.Header.Set("X-Amz-Target", "AWSLambda."+target)
	signHeader(t, req, raw, akid, secret, testRegion, "lambda", now)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}

func TestAuthorizeS3SCPDenyDespiteIdentityAllow(t *testing.T) {
	srv, st, _ := newTestServerStore(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	_, memberID, err := st.CreateMemberAccount(testAccountID, "scp-s3@example.com", "SCPMember")
	if err != nil {
		t.Fatal(err)
	}
	const memberAKID = "AKIAMEEMBERROOT01"
	const memberSecret = "secret-member-root"
	if err := st.EnsureRoot(memberID, memberAKID, memberSecret); err != nil {
		t.Fatal(err)
	}

	mustS3WithCreds(t, handler, http.MethodPut, "http://127.0.0.1:4566/scp-bucket", nil, memberAKID, memberSecret, "s3", now, nil)
	mustS3WithCreds(t, handler, http.MethodPut, "http://127.0.0.1:4566/scp-bucket/obj.txt", []byte("secret"), memberAKID, memberSecret, "s3", now, nil)

	_, userARN, err := st.CreateUser(memberID, "alice")
	if err != nil {
		t.Fatal(err)
	}
	userAKID, userSecret, err := st.CreateUserAccessKey(memberID, "alice")
	if err != nil {
		t.Fatal(err)
	}
	s3Allow := `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Action":"s3:*","Resource":"*"}]}`
	if err := st.PutInlinePolicy(userARN, "s3all", s3Allow); err != nil {
		t.Fatal(err)
	}

	scpDoc := `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Action":"iam:*","Resource":"*"}]}`
	scpID, err := st.CreateOrgPolicy("SCP", "NoS3", scpDoc)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.AttachOrgPolicy(scpID, "account", memberID); err != nil {
		t.Fatal(err)
	}

	getRec := mustS3WithCreds(t, handler, http.MethodGet, "http://127.0.0.1:4566/scp-bucket/obj.txt", nil, userAKID, userSecret, "s3", now, nil)
	if getRec.Code != http.StatusForbidden {
		t.Fatalf("GetObject status=%d want 403 body=%q", getRec.Code, getRec.Body.String())
	}
	if !strings.Contains(getRec.Body.String(), "AccessDenied") {
		t.Fatalf("expected AccessDenied in %q", getRec.Body.String())
	}
}

func TestAuthorizeS3BoundaryDenyDespiteIdentityAllow(t *testing.T) {
	srv, st, _ := newTestServerStore(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	mustS3(t, handler, http.MethodPut, "http://127.0.0.1:4566/bound-bucket", nil, "s3", now, nil)
	mustS3(t, handler, http.MethodPut, "http://127.0.0.1:4566/bound-bucket/obj.txt", []byte("secret"), "s3", now, nil)

	_, userARN, err := st.CreateUser(testAccountID, "bound-user")
	if err != nil {
		t.Fatal(err)
	}
	userAKID, userSecret, err := st.CreateUserAccessKey(testAccountID, "bound-user")
	if err != nil {
		t.Fatal(err)
	}
	s3Allow := `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Action":"s3:*","Resource":"*"}]}`
	if err := st.PutInlinePolicy(userARN, "s3all", s3Allow); err != nil {
		t.Fatal(err)
	}
	boundaryDoc := `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Action":"iam:*","Resource":"*"}]}`
	boundARN, err := st.CreateManagedPolicy(testAccountID, "NoS3Boundary", boundaryDoc)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.PutUserPermissionsBoundary(testAccountID, "bound-user", boundARN); err != nil {
		t.Fatal(err)
	}

	getRec := mustS3WithCreds(t, handler, http.MethodGet, "http://127.0.0.1:4566/bound-bucket/obj.txt", nil, userAKID, userSecret, "s3", now, nil)
	if getRec.Code != http.StatusForbidden {
		t.Fatalf("GetObject status=%d want 403 body=%q", getRec.Code, getRec.Body.String())
	}
	if !strings.Contains(getRec.Body.String(), "AccessDenied") {
		t.Fatalf("expected AccessDenied in %q", getRec.Body.String())
	}
}

func TestAuthorizeS3BucketPolicyAllowWithoutIdentity(t *testing.T) {
	srv, st, _ := newTestServerStore(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	_, userARN, err := st.CreateUser(testAccountID, "bucket-only")
	if err != nil {
		t.Fatal(err)
	}
	userAKID, userSecret, err := st.CreateUserAccessKey(testAccountID, "bucket-only")
	if err != nil {
		t.Fatal(err)
	}

	mustS3(t, handler, http.MethodPut, "http://127.0.0.1:4566/policy-bucket", nil, "s3", now, nil)
	mustS3(t, handler, http.MethodPut, "http://127.0.0.1:4566/policy-bucket/obj.txt", []byte("via-bucket"), "s3", now, nil)

	policy := []byte(`{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":{"AWS":"` + userARN + `"},"Action":"s3:GetObject","Resource":"arn:aws:s3:::policy-bucket/*"}]}`)
	polRec := mustS3(t, handler, http.MethodPut, "http://127.0.0.1:4566/policy-bucket?policy", policy, "s3", now, map[string]string{
		"Content-Type": "application/json",
	})
	if polRec.Code != http.StatusOK {
		t.Fatalf("PutBucketPolicy status=%d body=%q", polRec.Code, polRec.Body.String())
	}

	getRec := mustS3WithCreds(t, handler, http.MethodGet, "http://127.0.0.1:4566/policy-bucket/obj.txt", nil, userAKID, userSecret, "s3", now, nil)
	if getRec.Code != http.StatusOK {
		t.Fatalf("GetObject status=%d want 200 body=%q", getRec.Code, getRec.Body.String())
	}
	if getRec.Body.String() != "via-bucket" {
		t.Fatalf("body=%q", getRec.Body.String())
	}
}

func TestLambdaPassRoleBoundaryDeny(t *testing.T) {
	srv, st, _ := newTestServerStore(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	mustCreateIAMRole(t, handler, "lambda-pass-bound", lambdaTrustOK, now)
	roleARN := "arn:aws:iam::" + testAccountID + ":role/lambda-pass-bound"

	_, userARN, err := st.CreateUser(testAccountID, "passrole-user")
	if err != nil {
		t.Fatal(err)
	}
	userAKID, userSecret, err := st.CreateUserAccessKey(testAccountID, "passrole-user")
	if err != nil {
		t.Fatal(err)
	}
	identityAllow := `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Action":["lambda:CreateFunction","iam:PassRole"],"Resource":"*"}]}`
	if err := st.PutInlinePolicy(userARN, "pass", identityAllow); err != nil {
		t.Fatal(err)
	}
	boundaryDoc := `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Action":"lambda:CreateFunction","Resource":"*"}]}`
	boundARN, err := st.CreateManagedPolicy(testAccountID, "NoPassRoleBoundary", boundaryDoc)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.PutUserPermissionsBoundary(testAccountID, "passrole-user", boundARN); err != nil {
		t.Fatal(err)
	}

	rec := mustLambdaJSONWithCreds(t, handler, "CreateFunction", map[string]any{
		"FunctionName": "blocked-by-boundary",
		"Runtime":      "python3.12",
		"Role":         roleARN,
		"Handler":      "app.handler",
		"Code": map[string]any{
			"ZipFile": testLambdaZipB64(t),
		},
	}, userAKID, userSecret, now)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("CreateFunction status=%d want 403 body=%q", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "AccessDeniedException") {
		t.Fatalf("expected AccessDeniedException in %q", rec.Body.String())
	}
}
