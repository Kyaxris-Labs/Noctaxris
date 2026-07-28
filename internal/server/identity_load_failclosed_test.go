package server_test

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/Kyaxris-Labs/Noctaxris/internal/kernel/sts"
	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

// forceIdentityLoadError deletes the users row while leaving the access key so
// SigV4 still verifies but IdentityPolicyDocsForUser fails.
func forceIdentityLoadError(t *testing.T, st *store.Store, accountID, userName string) {
	t.Helper()
	if err := st.UnsafeDeleteUserRowForTest(accountID, userName); err != nil {
		t.Fatal(err)
	}
}

func TestIdentityLoadFailClosedAuthorize(t *testing.T) {
	srv, st, _ := newTestServerStore(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	if _, _, err := st.CreateUser(testAccountID, "broken-user"); err != nil {
		t.Fatal(err)
	}
	akid, secret, err := st.CreateUserAccessKey(testAccountID, "broken-user")
	if err != nil {
		t.Fatal(err)
	}
	forceIdentityLoadError(t, st, testAccountID, "broken-user")

	// GetCallerIdentity skips authorize; use a control-plane action that EvaluateFull-gates.
	put := mustS3WithCreds(t, handler, http.MethodPut, "http://127.0.0.1:4566/broken-authz", nil, akid, secret, "s3", now, nil)
	if put.Code != http.StatusForbidden {
		t.Fatalf("authorize after identity load error: status=%d body=%q want 403", put.Code, put.Body.String())
	}
}

func TestIdentityLoadFailClosedDataplaneOR(t *testing.T) {
	srv, st, _ := newTestServerStore(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	mustS3(t, handler, http.MethodPut, "http://127.0.0.1:4566/or-failclosed", nil, "s3", now, nil)
	mustS3(t, handler, http.MethodPut, "http://127.0.0.1:4566/or-failclosed/obj.txt", []byte("x"), "s3", now, nil)

	allowAll := `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":"*","Action":"s3:*","Resource":"*"}]}`
	if err := st.PutBucketPolicy(testAccountID, "or-failclosed", allowAll); err != nil {
		t.Fatal(err)
	}

	if _, _, err := st.CreateUser(testAccountID, "or-user"); err != nil {
		t.Fatal(err)
	}
	akid, secret, err := st.CreateUserAccessKey(testAccountID, "or-user")
	if err != nil {
		t.Fatal(err)
	}
	forceIdentityLoadError(t, st, testAccountID, "or-user")

	getRec := mustS3WithCreds(t, handler, http.MethodGet, "http://127.0.0.1:4566/or-failclosed/obj.txt", nil, akid, secret, "s3", now, nil)
	if getRec.Code != http.StatusForbidden {
		t.Fatalf("dataplane OR + Allow bucket policy + identity load error: status=%d body=%q want 403", getRec.Code, getRec.Body.String())
	}
}

func TestIdentityLoadFailClosedDataplaneKMS(t *testing.T) {
	srv, st, _ := newTestServerStore(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	createRec := mustKMSJSONWithCreds(t, handler, "CreateKey", map[string]any{}, testAccessKey, testSecret, now)
	if createRec.Code != http.StatusOK {
		t.Fatalf("CreateKey status=%d body=%q", createRec.Code, createRec.Body.String())
	}
	var createOut map[string]any
	if err := json.Unmarshal(createRec.Body.Bytes(), &createOut); err != nil {
		t.Fatal(err)
	}
	meta, _ := createOut["KeyMetadata"].(map[string]any)
	keyID, _ := meta["KeyId"].(string)
	keyARN, _ := meta["Arn"].(string)

	if _, _, err := st.CreateUser(testAccountID, "kms-user"); err != nil {
		t.Fatal(err)
	}
	akid, secret, err := st.CreateUserAccessKey(testAccountID, "kms-user")
	if err != nil {
		t.Fatal(err)
	}
	userARN := fmt.Sprintf("arn:aws:iam::%s:user/kms-user", testAccountID)
	keyPolicy := fmt.Sprintf(
		`{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":{"AWS":"%s"},"Action":"kms:*","Resource":"*"}]}`,
		userARN,
	)
	putPol := mustKMSJSONWithCreds(t, handler, "PutKeyPolicy", map[string]any{
		"KeyId":      keyID,
		"PolicyName": "default",
		"Policy":     keyPolicy,
	}, testAccessKey, testSecret, now)
	if putPol.Code != http.StatusOK {
		t.Fatalf("PutKeyPolicy status=%d body=%q", putPol.Code, putPol.Body.String())
	}

	forceIdentityLoadError(t, st, testAccountID, "kms-user")

	encRec := mustKMSJSONWithCreds(t, handler, "Encrypt", map[string]any{
		"KeyId":     keyARN,
		"Plaintext": base64.StdEncoding.EncodeToString([]byte("failclosed")),
	}, akid, secret, now)
	if encRec.Code != http.StatusForbidden {
		t.Fatalf("authorizeDataplaneKMS + Allow key policy + identity load error: status=%d body=%q want 403", encRec.Code, encRec.Body.String())
	}
}

func TestIdentityLoadFailClosedRootARNDeny(t *testing.T) {
	srv, st, _ := newTestServerStore(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	rootARN := fmt.Sprintf("arn:aws:iam::%s:root", testAccountID)
	// Must use authorize/EvaluateFull (not S3 dataplane OR): denyScanFull scans root identity docs.
	denyIAM := `{"Version":"2012-10-17","Statement":[{"Effect":"Deny","Action":"iam:CreateUser","Resource":"*"}]}`
	if err := st.PutInlinePolicy(rootARN, "root-deny", denyIAM); err != nil {
		t.Fatal(err)
	}

	body := []byte("Action=CreateUser&Version=2010-05-08&UserName=should-deny")
	req := mustNewRequest(t, http.MethodPost, "http://127.0.0.1:4566/", body)
	signHeader(t, req, body, testAccessKey, testSecret, testRegion, "iam", now)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("root ARN identity Deny must still Deny (no root identity skip): status=%d body=%q", rec.Code, rec.Body.String())
	}
}

func TestIdentityLoadFailClosedRootSessionDeny(t *testing.T) {
	srv, _, _ := newTestServerStore(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	// Federation from root mints IsRoot temp keys; SessionPolicy still intersects via EvaluateFull.
	sessionDeny := `{"Version":"2012-10-17","Statement":[{"Effect":"Deny","Action":"iam:CreateUser","Resource":"*"}]}`
	fedBody := []byte("Action=GetFederationToken&Version=2011-06-15&Name=root-sess&Policy=" + url.QueryEscape(sessionDeny))
	req := mustNewRequest(t, http.MethodPost, "http://127.0.0.1:4566/", fedBody)
	signHeader(t, req, fedBody, testAccessKey, testSecret, testRegion, "sts", now)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("GetFederationToken mint: status=%d body=%q", rec.Code, rec.Body.String())
	}
	sessAKID := xmlTag(t, rec.Body.String(), "AccessKeyId")
	sessSecret := xmlTag(t, rec.Body.String(), "SecretAccessKey")
	sessToken := xmlTag(t, rec.Body.String(), "SessionToken")

	body := []byte("Action=CreateUser&Version=2010-05-08&UserName=root-sess-deny")
	req = mustNewRequest(t, http.MethodPost, "http://127.0.0.1:4566/", body)
	signHeader(t, req, body, sessAKID, sessSecret, testRegion, "iam", now)
	req.Header.Set("X-Amz-Security-Token", sessToken)
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("root/session Policy Deny must Deny: status=%d body=%q", rec.Code, rec.Body.String())
	}
}

func TestIdentityLoadFailClosedMgmtSCPExemptRCPApplies(t *testing.T) {
	srv, st, _ := newTestServerStore(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	scpDoc := `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Action":"iam:*","Resource":"*"}]}`
	scpID, err := st.CreateOrgPolicy("SCP", "NoS3Mgmt", scpDoc)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.AttachOrgPolicy(scpID, "root", store.OrgRootID); err != nil {
		t.Fatal(err)
	}
	mgmtPut := mustS3(t, handler, http.MethodPut, "http://127.0.0.1:4566/mgmt-scp-exempt", nil, "s3", now, nil)
	if mgmtPut.Code != http.StatusOK {
		t.Fatalf("mgmt SCP-exempt CreateBucket status=%d body=%q want 200", mgmtPut.Code, mgmtPut.Body.String())
	}

	rcpDoc := `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Action":"iam:*","Resource":"*"}]}`
	rcpID, err := st.CreateOrgPolicy("RCP", "NoS3RCP", rcpDoc)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.AttachOrgPolicy(rcpID, "account", testAccountID); err != nil {
		t.Fatal(err)
	}
	rcpPut := mustS3(t, handler, http.MethodPut, "http://127.0.0.1:4566/mgmt-rcp-blocks", nil, "s3", now, nil)
	if rcpPut.Code != http.StatusForbidden {
		t.Fatalf("mgmt RCP must still Deny S3: status=%d body=%q want 403", rcpPut.Code, rcpPut.Body.String())
	}
}

func TestIdentityLoadFailClosedOUPathError(t *testing.T) {
	srv, st, _ := newTestServerStore(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	_, memberID, err := st.CreateMemberAccount(testAccountID, "ou-err@example.com", "OUErr")
	if err != nil {
		t.Fatal(err)
	}
	const memberAKID = "AKIAOUPATHERR001"
	const memberSecret = "secret-ou-path-err"
	if err := st.EnsureRoot(memberID, memberAKID, memberSecret); err != nil {
		t.Fatal(err)
	}
	ouA, err := st.CreateOrganizationalUnit(store.OrgRootID, "CycleA")
	if err != nil {
		t.Fatal(err)
	}
	ouB, err := st.CreateOrganizationalUnit(ouA, "CycleB")
	if err != nil {
		t.Fatal(err)
	}
	if err := st.MoveAccount(memberID, store.OrgRootID, ouB); err != nil {
		t.Fatal(err)
	}
	if err := st.UnsafeSetOUParentForTest(ouA, ouB); err != nil {
		t.Fatal(err)
	}

	put := mustS3WithCreds(t, handler, http.MethodPut, "http://127.0.0.1:4566/ou-path-err", nil, memberAKID, memberSecret, "s3", now, nil)
	if put.Code != http.StatusForbidden {
		t.Fatalf("OU path error must Deny (not empty SCP/RCP): status=%d body=%q", put.Code, put.Body.String())
	}
}

func TestIdentityLoadFailClosedFederatedEmptyPolicy(t *testing.T) {
	srv, st, _ := newTestServerStore(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	if _, _, err := st.CreateUser(testAccountID, "fed-empty"); err != nil {
		t.Fatal(err)
	}
	userARN := fmt.Sprintf("arn:aws:iam::%s:user/fed-empty", testAccountID)
	idDoc := `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Action":["sts:GetFederationToken","sts:DecodeAuthorizationMessage"],"Resource":"*"}]}`
	if err := st.PutInlinePolicy(userARN, "fed-id", idDoc); err != nil {
		t.Fatal(err)
	}
	akid, secret, err := st.CreateUserAccessKey(testAccountID, "fed-empty")
	if err != nil {
		t.Fatal(err)
	}

	emptyBody := []byte("Action=GetFederationToken&Version=2011-06-15&Name=empty")
	req := mustNewRequest(t, http.MethodPost, "http://127.0.0.1:4566/", emptyBody)
	signHeader(t, req, emptyBody, akid, secret, testRegion, "sts", now)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("GetFederationToken empty Policy mint: %d %s", rec.Code, rec.Body.String())
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
	if rec.Code == http.StatusOK {
		t.Fatalf("federated empty SessionPolicy must Deny: status=%d body=%q", rec.Code, rec.Body.String())
	}
}
