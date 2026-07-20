package server_test

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestSigV4BodySwapRejectedOnServer(t *testing.T) {
	srv, _ := newTestServer(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	signedBody := []byte("Action=GetCallerIdentity&Version=2011-06-15")
	swapped := []byte("Action=GetSessionToken&Version=2011-06-15")
	signedReq := mustNewRequest(t, http.MethodPost, "http://127.0.0.1:4566/", signedBody)
	signHeader(t, signedReq, signedBody, testAccessKey, testSecret, testRegion, testSvc, now)

	req := mustNewRequest(t, http.MethodPost, "http://127.0.0.1:4566/", swapped)
	req.Header = signedReq.Header.Clone()

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status=%d want 403 body=%q", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "SignatureDoesNotMatch") {
		t.Fatalf("expected SignatureDoesNotMatch in %q", rec.Body.String())
	}
}

func TestAssumedRoleIdentityDocsAllowListUsers(t *testing.T) {
	srv, st, _ := newTestServerStore(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	trust := `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":{"AWS":"arn:aws:iam::` + testAccountID + `:root"},"Action":"sts:AssumeRole"}]}`
	mustCreateIAMRole(t, handler, "lab-session-role", trust, now)
	roleARN := "arn:aws:iam::" + testAccountID + ":role/lab-session-role"

	denyList := mustIAMListUsersWithSession(t, handler, roleARN, "no-policy", now)
	if denyList.Code == http.StatusOK {
		t.Fatalf("ListUsers without role policy should deny, got OK: %s", denyList.Body.String())
	}

	allowDoc := `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Action":"iam:ListUsers","Resource":"*"}]}`
	policyARN, err := st.CreateManagedPolicy(testAccountID, "SessionListUsers", allowDoc)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.AttachRolePolicy(testAccountID, "lab-session-role", policyARN); err != nil {
		t.Fatal(err)
	}

	allowList := mustIAMListUsersWithSession(t, handler, roleARN, "with-policy", now)
	if allowList.Code != http.StatusOK || !strings.Contains(allowList.Body.String(), "ListUsersResponse") {
		t.Fatalf("ListUsers with role policy: %d %s", allowList.Code, allowList.Body.String())
	}
}

func mustIAMListUsersWithSession(t *testing.T, handler http.Handler, roleARN, sessionName string, now time.Time) *httptest.ResponseRecorder {
	t.Helper()
	assumeBody := []byte("Action=AssumeRole&Version=2011-06-15&RoleArn=" + url.QueryEscape(roleARN) +
		"&RoleSessionName=" + url.QueryEscape(sessionName))
	assumeReq := mustNewRequest(t, http.MethodPost, "http://127.0.0.1:4566/", assumeBody)
	signHeader(t, assumeReq, assumeBody, testAccessKey, testSecret, testRegion, "sts", now)
	assumeRec := httptest.NewRecorder()
	handler.ServeHTTP(assumeRec, assumeReq)
	if assumeRec.Code != http.StatusOK {
		t.Fatalf("AssumeRole status=%d body=%q", assumeRec.Code, assumeRec.Body.String())
	}
	akid := xmlTag(t, assumeRec.Body.String(), "AccessKeyId")
	secret := xmlTag(t, assumeRec.Body.String(), "SecretAccessKey")
	token := xmlTag(t, assumeRec.Body.String(), "SessionToken")

	listBody := []byte("Action=ListUsers&Version=2010-05-08")
	req := mustNewRequest(t, http.MethodPost, "http://127.0.0.1:4566/", listBody)
	signHeader(t, req, listBody, akid, secret, testRegion, "iam", now)
	req.Header.Set("X-Amz-Security-Token", token)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}

func TestRegistryV2IAMDenyDespiteToken(t *testing.T) {
	srv, st, _ := newTestServerStore(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	createRec := mustECRJSON(t, handler, "CreateRepository", map[string]any{
		"repositoryName": "locked-repo",
	}, now)
	if createRec.Code != http.StatusOK {
		t.Fatalf("CreateRepository status=%d body=%q", createRec.Code, createRec.Body.String())
	}

	_, userARN, err := st.CreateUser(testAccountID, "ecr-token-only")
	if err != nil {
		t.Fatal(err)
	}
	userAKID, userSecret, err := st.CreateUserAccessKey(testAccountID, "ecr-token-only")
	if err != nil {
		t.Fatal(err)
	}
	tokenOnly := `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Action":"ecr:GetAuthorizationToken","Resource":"*"}]}`
	policyARN, err := st.CreateManagedPolicy(testAccountID, "ECRTokenOnly", tokenOnly)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.AttachUserPolicy(testAccountID, "ecr-token-only", policyARN); err != nil {
		t.Fatal(err)
	}

	token := issueRegistryTokenWithCreds(t, handler, userAKID, userSecret, now)
	auth := registryAuthHeader(token)
	account := testAccountID
	repo := "locked-repo"

	uploadReq := httptest.NewRequest(http.MethodPost, fmt.Sprintf("http://127.0.0.1:4566/v2/%s/%s/blobs/uploads/", account, repo), nil)
	uploadReq.Header.Set("Authorization", auth)
	uploadRec := httptest.NewRecorder()
	handler.ServeHTTP(uploadRec, uploadReq)
	if uploadRec.Code != http.StatusForbidden {
		t.Fatalf("blob upload start status=%d want 403 body=%q user=%s", uploadRec.Code, uploadRec.Body.String(), userARN)
	}
	if !strings.Contains(uploadRec.Body.String(), "DENIED") {
		t.Fatalf("expected DENIED in %q", uploadRec.Body.String())
	}
}

func TestRegistryV2RepoPolicyAllowPush(t *testing.T) {
	srv, st, _ := newTestServerStore(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	createRec := mustECRJSON(t, handler, "CreateRepository", map[string]any{
		"repositoryName": "policy-push",
	}, now)
	if createRec.Code != http.StatusOK {
		t.Fatalf("CreateRepository status=%d body=%q", createRec.Code, createRec.Body.String())
	}

	_, userARN, err := st.CreateUser(testAccountID, "ecr-pusher")
	if err != nil {
		t.Fatal(err)
	}
	userAKID, userSecret, err := st.CreateUserAccessKey(testAccountID, "ecr-pusher")
	if err != nil {
		t.Fatal(err)
	}
	tokenOnly := `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Action":"ecr:GetAuthorizationToken","Resource":"*"}]}`
	policyARN, err := st.CreateManagedPolicy(testAccountID, "ECRTokenForPush", tokenOnly)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.AttachUserPolicy(testAccountID, "ecr-pusher", policyARN); err != nil {
		t.Fatal(err)
	}

	repoPolicy := fmt.Sprintf(`{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":{"AWS":"%s"},"Action":["ecr:InitiateLayerUpload","ecr:UploadLayerPart","ecr:PutImage","ecr:BatchGetImage","ecr:ListImages"],"Resource":"*"}]}`, userARN)
	setPol := mustECRJSON(t, handler, "SetRepositoryPolicy", map[string]any{
		"repositoryName": "policy-push",
		"policyText":     repoPolicy,
	}, now)
	if setPol.Code != http.StatusOK {
		t.Fatalf("SetRepositoryPolicy status=%d body=%q", setPol.Code, setPol.Body.String())
	}

	token := issueRegistryTokenWithCreds(t, handler, userAKID, userSecret, now)
	auth := registryAuthHeader(token)
	account := testAccountID
	repo := "policy-push"

	uploadReq := httptest.NewRequest(http.MethodPost, fmt.Sprintf("http://127.0.0.1:4566/v2/%s/%s/blobs/uploads/", account, repo), nil)
	uploadReq.Header.Set("Authorization", auth)
	uploadRec := httptest.NewRecorder()
	handler.ServeHTTP(uploadRec, uploadReq)
	if uploadRec.Code != http.StatusAccepted {
		t.Fatalf("blob upload start status=%d want 202 body=%q", uploadRec.Code, uploadRec.Body.String())
	}
}

func issueRegistryTokenWithCreds(t *testing.T, handler http.Handler, akid, secret string, now time.Time) string {
	t.Helper()
	rec := mustECRJSONWithCreds(t, handler, "GetAuthorizationToken", map[string]any{}, akid, secret, now)
	if rec.Code != http.StatusOK {
		t.Fatalf("GetAuthorizationToken status=%d body=%q", rec.Code, rec.Body.String())
	}
	var out map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	data, _ := out["authorizationData"].([]any)
	entry, _ := data[0].(map[string]any)
	tokenB64, _ := entry["authorizationToken"].(string)
	raw, err := base64.StdEncoding.DecodeString(tokenB64)
	if err != nil {
		t.Fatal(err)
	}
	parts := strings.SplitN(string(raw), ":", 2)
	if len(parts) != 2 {
		t.Fatalf("token=%q", string(raw))
	}
	return parts[1]
}