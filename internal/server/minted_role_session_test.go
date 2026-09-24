package server_test

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

func TestMintedRoleSessionAuthorizesS3AndIAM(t *testing.T) {
	srv, st, _ := newTestServerStore(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	const roleName = "mint-bridge"
	roleARN := "arn:aws:iam::" + testAccountID + ":role/" + roleName
	trust := `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":{"AWS":"arn:aws:iam::` + testAccountID + `:root"},"Action":"sts:AssumeRole"}]}`
	mustCreateIAMRole(t, handler, roleName, trust, now)
	doc := `{"Version":"2012-10-17","Statement":[
		{"Effect":"Allow","Action":"s3:GetObject","Resource":"arn:aws:s3:::mint-ok/*"},
		{"Effect":"Allow","Action":"iam:GetRole","Resource":"` + roleARN + `"}
	]}`
	if err := st.PutInlinePolicy(roleARN, "bridge", doc); err != nil {
		t.Fatal(err)
	}

	mustS3(t, handler, http.MethodPut, "http://127.0.0.1:4566/mint-ok", nil, "s3", now, nil)
	mustS3(t, handler, http.MethodPut, "http://127.0.0.1:4566/mint-ok/obj.txt", []byte("live"), "s3", now, nil)
	mustS3(t, handler, http.MethodPut, "http://127.0.0.1:4566/mint-no", nil, "s3", now, nil)
	mustS3(t, handler, http.MethodPut, "http://127.0.0.1:4566/mint-no/obj.txt", []byte("hidden"), "s3", now, nil)

	assumeAK, assumeSecret, assumeToken := assumeRoleSession(t, handler, roleARN, "player", now)
	assertUpperASIA(t, assumeAK)
	exerciseMintedSession(t, handler, assumeAK, assumeSecret, assumeToken, roleName, now)

	// IoT credentials provider, nested compute, and delivery all mint through this store path.
	storeAK, err := st.MintTempCredentialsOpts(store.MintTempOpts{
		AccountID:    testAccountID,
		RoleARN:      roleARN,
		SessionName:  "device",
		Secret:       "store-mint-secret",
		SessionToken: "store-mint-token",
		Expires:      now.Add(time.Hour),
	})
	if err != nil {
		t.Fatal(err)
	}
	assertUpperASIA(t, storeAK)
	ak, err := st.LookupAccessKeyRecord(storeAK)
	if err != nil {
		t.Fatal(err)
	}
	if ak.RoleARN != roleARN || ak.SessionName != "device" {
		t.Fatalf("stored role=%q session=%q", ak.RoleARN, ak.SessionName)
	}
	exerciseMintedSession(t, handler, storeAK, "store-mint-secret", "store-mint-token", roleName, now)
}

func assumeRoleSession(t *testing.T, handler http.Handler, roleARN, sessionName string, now time.Time) (akid, secret, token string) {
	t.Helper()
	body := []byte("Action=AssumeRole&Version=2011-06-15&RoleArn=" + url.QueryEscape(roleARN) +
		"&RoleSessionName=" + url.QueryEscape(sessionName))
	req := mustNewRequest(t, http.MethodPost, "http://127.0.0.1:4566/", body)
	signHeader(t, req, body, testAccessKey, testSecret, testRegion, "sts", now)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("AssumeRole status=%d body=%q", rec.Code, rec.Body.String())
	}
	return xmlTag(t, rec.Body.String(), "AccessKeyId"),
		xmlTag(t, rec.Body.String(), "SecretAccessKey"),
		xmlTag(t, rec.Body.String(), "SessionToken")
}

func exerciseMintedSession(t *testing.T, handler http.Handler, akid, secret, token, roleName string, now time.Time) {
	t.Helper()
	tokenHdr := map[string]string{"X-Amz-Security-Token": token}

	okGet := mustS3WithCreds(t, handler, http.MethodGet, "http://127.0.0.1:4566/mint-ok/obj.txt", nil, akid, secret, "s3", now, tokenHdr)
	if okGet.Code != http.StatusOK || okGet.Body.String() != "live" {
		t.Fatalf("allowed GetObject status=%d body=%q", okGet.Code, okGet.Body.String())
	}
	deniedGet := mustS3WithCreds(t, handler, http.MethodGet, "http://127.0.0.1:4566/mint-no/obj.txt", nil, akid, secret, "s3", now, tokenHdr)
	if deniedGet.Code != http.StatusForbidden {
		t.Fatalf("out-of-policy GetObject status=%d body=%q", deniedGet.Code, deniedGet.Body.String())
	}
	noToken := mustS3WithCreds(t, handler, http.MethodGet, "http://127.0.0.1:4566/mint-ok/obj.txt", nil, akid, secret, "s3", now, nil)
	if noToken.Code == http.StatusOK || !strings.Contains(noToken.Body.String(), "InvalidClientTokenId") {
		t.Fatalf("missing session token status=%d body=%q", noToken.Code, noToken.Body.String())
	}

	getRole := iamQueryWithSession(t, handler, "Action=GetRole&Version=2010-05-08&RoleName="+url.QueryEscape(roleName), akid, secret, token, now)
	if getRole.Code != http.StatusOK || !strings.Contains(getRole.Body.String(), roleName) {
		t.Fatalf("GetRole status=%d body=%q", getRole.Code, getRole.Body.String())
	}
	createUser := iamQueryWithSession(t, handler, "Action=CreateUser&Version=2010-05-08&UserName=not-allowed", akid, secret, token, now)
	if createUser.Code != http.StatusForbidden {
		t.Fatalf("CreateUser status=%d body=%q", createUser.Code, createUser.Body.String())
	}
}

func iamQueryWithSession(t *testing.T, handler http.Handler, form, akid, secret, token string, now time.Time) *httptest.ResponseRecorder {
	t.Helper()
	body := []byte(form)
	req := mustNewRequest(t, http.MethodPost, "http://127.0.0.1:4566/", body)
	req.Header.Set("X-Amz-Security-Token", token)
	signHeader(t, req, body, akid, secret, testRegion, "iam", now)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}

func assertUpperASIA(t *testing.T, akid string) {
	t.Helper()
	if !strings.HasPrefix(akid, "ASIA") || len(akid) != 20 || akid != strings.ToUpper(akid) {
		t.Fatalf("access key id %q, want 20-char uppercase ASIA", akid)
	}
}
