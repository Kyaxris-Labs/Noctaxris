package server_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/Kyaxris-Labs/Noctaxris/internal/config"
	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

func TestStandaloneSeedDisabledIsNotFound(t *testing.T) {
	srv, _ := newTestServer(t)
	req := mustNewRequest(t, http.MethodPost, "http://127.0.0.1:4566"+standalonePath(), nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status=%d want 404 body=%q", rec.Code, rec.Body.String())
	}
}

func TestStandaloneSeedRootAndIsolation(t *testing.T) {
	srv, st, _ := newTestServerStoreWith(t, func(cfg *config.Config) {
		cfg.LabStandalone = true
		cfg.RootSecretAccessKey = testSecret
	})
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	_, memberID, err := st.CreateMemberAccount(testAccountID, "member@example.com", "Member")
	if err != nil {
		t.Fatal(err)
	}
	const memberKey = "AKIAMEMBERROOT009"
	const memberSecret = "secret-member-root-9"
	if err := st.EnsureRoot(memberID, memberKey, memberSecret); err != nil {
		t.Fatal(err)
	}
	memberRec := postSeed(t, handler, memberKey, memberSecret, nil, now)
	if memberRec.Code != http.StatusForbidden || !strings.Contains(memberRec.Body.String(), "AccessDenied") {
		t.Fatalf("member root seed status=%d body=%q", memberRec.Code, memberRec.Body.String())
	}

	first := postSeed(t, handler, testAccessKey, testSecret, []byte(`{"name":"home"}`), now)
	if first.Code != http.StatusOK {
		t.Fatalf("seed status=%d body=%q", first.Code, first.Body.String())
	}
	var seeded map[string]string
	if err := json.Unmarshal(first.Body.Bytes(), &seeded); err != nil {
		t.Fatal(err)
	}
	if seeded["accountId"] == testAccountID || seeded["secretAccessKey"] == "" || seeded["secretAccessKey"] == testSecret {
		t.Fatalf("seeded=%v", seeded)
	}
	key, err := st.LookupAccessKeyRecord(seeded["accessKeyId"])
	if err != nil {
		t.Fatal(err)
	}
	if key.IsRoot || key.AccountID != seeded["accountId"] {
		t.Fatalf("key=%+v", key)
	}

	second := postSeed(t, handler, testAccessKey, testSecret, []byte(`{"name":"home"}`), now)
	var again map[string]string
	if err := json.Unmarshal(second.Body.Bytes(), &again); err != nil {
		t.Fatal(err)
	}
	if again["accessKeyId"] != seeded["accessKeyId"] || again["secretAccessKey"] != seeded["secretAccessKey"] {
		t.Fatalf("reseed rotated first=%v second=%v", seeded, again)
	}

	userKey := seeded["accessKeyId"]
	userSecret := seeded["secretAccessKey"]
	home := seeded["accountId"]

	create := mustS3WithCreds(t, handler, http.MethodPut, "http://127.0.0.1:4566/home-bucket", nil, userKey, userSecret, "s3", now, nil)
	if create.Code != http.StatusOK {
		t.Fatalf("CreateBucket status=%d body=%q", create.Code, create.Body.String())
	}
	put := mustS3WithCreds(t, handler, http.MethodPut, "http://127.0.0.1:4566/home-bucket/note.txt", []byte("home"), userKey, userSecret, "s3", now, nil)
	if put.Code != http.StatusOK {
		t.Fatalf("PutObject status=%d body=%q", put.Code, put.Body.String())
	}
	ownList := mustS3WithCreds(t, handler, http.MethodGet, "http://127.0.0.1:4566/", nil, userKey, userSecret, "s3", now, nil)
	if ownList.Code != http.StatusOK || !strings.Contains(ownList.Body.String(), "home-bucket") {
		t.Fatalf("home ListBuckets status=%d body=%q", ownList.Code, ownList.Body.String())
	}
	labList := mustS3WithCreds(t, handler, http.MethodGet, "http://127.0.0.1:4566/", nil, testAccessKey, testSecret, "s3", now, nil)
	if labList.Code != http.StatusOK || strings.Contains(labList.Body.String(), "home-bucket") {
		t.Fatalf("lab ListBuckets status=%d body=%q", labList.Code, labList.Body.String())
	}

	labBucket := mustS3(t, handler, http.MethodPut, "http://127.0.0.1:4566/lab-bucket", nil, "s3", now, nil)
	if labBucket.Code != http.StatusOK {
		t.Fatalf("lab CreateBucket status=%d body=%q", labBucket.Code, labBucket.Body.String())
	}
	labObject := mustS3(t, handler, http.MethodPut, "http://127.0.0.1:4566/lab-bucket/secret.txt", []byte("lab"), "s3", now, nil)
	if labObject.Code != http.StatusOK {
		t.Fatalf("lab PutObject status=%d body=%q", labObject.Code, labObject.Body.String())
	}
	denied := mustS3WithCreds(t, handler, http.MethodGet, "http://127.0.0.1:4566/lab-bucket/secret.txt", nil, userKey, userSecret, "s3", now, nil)
	if denied.Code != http.StatusForbidden {
		t.Fatalf("cross GetObject status=%d body=%q", denied.Code, denied.Body.String())
	}
	policy := `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":{"AWS":"` + seeded["userArn"] + `"},"Action":"s3:GetObject","Resource":"arn:aws:s3:::lab-bucket/*"}]}`
	if err := st.PutBucketPolicy(testAccountID, "lab-bucket", policy); err != nil {
		t.Fatal(err)
	}
	allowed := mustS3WithCreds(t, handler, http.MethodGet, "http://127.0.0.1:4566/lab-bucket/secret.txt", nil, userKey, userSecret, "s3", now, nil)
	if allowed.Code != http.StatusOK || allowed.Body.String() != "lab" {
		t.Fatalf("allowed GetObject status=%d body=%q", allowed.Code, allowed.Body.String())
	}

	listAccounts := stsOrOrg(t, handler, "Action=ListAccounts&Version=2016-11-28", userKey, userSecret, "organizations", now)
	if listAccounts.Code != http.StatusForbidden {
		t.Fatalf("ListAccounts status=%d body=%q", listAccounts.Code, listAccounts.Body.String())
	}
	assume := stsOrOrg(t, handler, "Action=AssumeRoot&Version=2011-06-15&TargetAccount="+home, testAccessKey, testSecret, "sts", now)
	if assume.Code != http.StatusForbidden || !strings.Contains(assume.Body.String(), "AccessDenied") {
		t.Fatalf("AssumeRoot status=%d body=%q", assume.Code, assume.Body.String())
	}

	policyARN, err := st.CreateManagedPolicy(home, "HomePolicy", `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Action":"s3:GetObject","Resource":"*"}]}`)
	if err != nil {
		t.Fatal(err)
	}
	foreign := iamForm(t, handler, "Action=ListPolicyVersions&Version=2010-05-08&PolicyArn="+url.QueryEscape(policyARN), now)
	if foreign.Code != http.StatusForbidden || !strings.Contains(foreign.Body.String(), "AccessDenied") {
		t.Fatalf("foreign ListPolicyVersions status=%d body=%q", foreign.Code, foreign.Body.String())
	}
	awsManaged := iamForm(t, handler, "Action=GetPolicy&Version=2010-05-08&PolicyArn="+url.QueryEscape("arn:aws:iam::aws:policy/AdministratorAccess"), now)
	if awsManaged.Code == http.StatusForbidden {
		t.Fatalf("aws managed GetPolicy denied body=%q", awsManaged.Body.String())
	}

	const bad = "000000000066"
	if err := st.InsertAccount(bad, "not-a-parent"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := st.CreateUser(bad, "admin"); err != nil {
		t.Fatal(err)
	}
	if err := st.PutInlinePolicy(store.UserARN(bad, "/", "admin"), "AdministratorAccess", `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Action":"*","Resource":"*"}]}`); err != nil {
		t.Fatal(err)
	}
	badKey, badSecret, err := st.CreateUserAccessKey(bad, "admin")
	if err != nil {
		t.Fatal(err)
	}
	badBucket := mustS3WithCreds(t, handler, http.MethodPut, "http://127.0.0.1:4566/bad-parent-bucket", nil, badKey, badSecret, "s3", now, nil)
	if badBucket.Code != http.StatusForbidden {
		t.Fatalf("bad parent CreateBucket status=%d body=%q", badBucket.Code, badBucket.Body.String())
	}
}

func standalonePath() string {
	return "/_noctaxris/lab/accounts:seedStandalone"
}

func postSeed(t *testing.T, handler http.Handler, akid, secret string, body []byte, now time.Time) *httptest.ResponseRecorder {
	t.Helper()
	req := mustNewRequest(t, http.MethodPost, "http://127.0.0.1:4566"+standalonePath(), body)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	signHeader(t, req, body, akid, secret, testRegion, "iam", now)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}

func stsOrOrg(t *testing.T, handler http.Handler, body, akid, secret, service string, now time.Time) *httptest.ResponseRecorder {
	t.Helper()
	req := mustNewRequest(t, http.MethodPost, "http://127.0.0.1:4566/", []byte(body))
	signHeader(t, req, []byte(body), akid, secret, testRegion, service, now)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}
