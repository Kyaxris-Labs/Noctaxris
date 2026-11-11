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

func TestStandaloneSeedDisabledSignedRootIsNotFound(t *testing.T) {
	srv, _ := newTestServer(t)
	now := time.Now().UTC().Truncate(time.Second)
	rec := postSeed(t, srv.Handler(), testAccessKey, testSecret, []byte(`{"name":"home"}`), now)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status=%d want 404 body=%q", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "secretAccessKey") || strings.Contains(rec.Body.String(), "AccessDenied") {
		t.Fatalf("disabled seed must stay unregistered body=%q", rec.Body.String())
	}
}

func TestStandaloneSeedEnabledUnsignedIsForbidden(t *testing.T) {
	srv, _, _ := newTestServerStoreWith(t, func(cfg *config.Config) {
		cfg.LabStandalone = true
	})
	body := []byte(`{"name":"home"}`)
	req := mustNewRequest(t, http.MethodPost, "http://127.0.0.1:4566"+standalonePath(), body)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status=%d want 403 body=%q", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "MissingAuthenticationToken") {
		t.Fatalf("unsigned seed must fail SigV4 body=%q", rec.Body.String())
	}
}

func TestStandaloneSeedEnabledNonPostIsNotFound(t *testing.T) {
	srv, _, _ := newTestServerStoreWith(t, func(cfg *config.Config) {
		cfg.LabStandalone = true
	})
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)
	for _, method := range []string{http.MethodGet, http.MethodPut, http.MethodDelete} {
		req := mustNewRequest(t, method, "http://127.0.0.1:4566"+standalonePath(), nil)
		signHeader(t, req, nil, testAccessKey, testSecret, testRegion, "iam", now)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusNotFound {
			t.Fatalf("%s status=%d want 404 body=%q", method, rec.Code, rec.Body.String())
		}
	}
	req := mustNewRequest(t, http.MethodGet, "http://127.0.0.1:4566"+standalonePath(), nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("unsigned GET status=%d want 404 body=%q", rec.Code, rec.Body.String())
	}
}

func TestStandaloneSeedCallerGateAndIdempotentSecret(t *testing.T) {
	srv, st, _ := newTestServerStoreWith(t, func(cfg *config.Config) {
		cfg.LabStandalone = true
		cfg.RootSecretAccessKey = testSecret
	})
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)
	before := accountIDSet(t, st)

	if _, _, err := st.CreateUser(testAccountID, "seed-admin"); err != nil {
		t.Fatal(err)
	}
	allowAll := `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Action":"*","Resource":"*"}]}`
	if err := st.PutInlinePolicy(store.UserARN(testAccountID, "/", "seed-admin"), "AdministratorAccess", allowAll); err != nil {
		t.Fatal(err)
	}
	userKey, userSecret, err := st.CreateUserAccessKey(testAccountID, "seed-admin")
	if err != nil {
		t.Fatal(err)
	}
	userRec, err := st.LookupAccessKeyRecord(userKey)
	if err != nil {
		t.Fatal(err)
	}
	if userRec.IsRoot {
		t.Fatal("management user key is root")
	}
	deniedUser := postSeed(t, handler, userKey, userSecret, []byte(`{"name":"home"}`), now)
	if deniedUser.Code != http.StatusForbidden || !strings.Contains(deniedUser.Body.String(), "AccessDenied") {
		t.Fatalf("user seed status=%d body=%q", deniedUser.Code, deniedUser.Body.String())
	}
	assertSameAccounts(t, st, before)

	_, memberID, err := st.CreateMemberAccount(testAccountID, "member@example.com", "Member")
	if err != nil {
		t.Fatal(err)
	}
	const memberKey = "AKIAMEMBERROOT011"
	const memberSecret = "secret-member-root-11"
	if err := st.EnsureRoot(memberID, memberKey, memberSecret); err != nil {
		t.Fatal(err)
	}
	memberRec, err := st.LookupAccessKeyRecord(memberKey)
	if err != nil {
		t.Fatal(err)
	}
	if !memberRec.IsRoot || memberRec.AccountID != memberID {
		t.Fatalf("member key=%+v", memberRec)
	}
	deniedMember := postSeed(t, handler, memberKey, memberSecret, []byte(`{"name":"home"}`), now)
	if deniedMember.Code != http.StatusForbidden || !strings.Contains(deniedMember.Body.String(), "AccessDenied") {
		t.Fatalf("member root seed status=%d body=%q", deniedMember.Code, deniedMember.Body.String())
	}
	afterMember := accountIDSet(t, st)
	if _, ok := afterMember[memberID]; !ok {
		t.Fatal("member account missing")
	}
	if len(afterMember) != len(before)+1 {
		t.Fatalf("member seed created an extra account before=%d after=%d", len(before), len(afterMember))
	}

	first := postSeed(t, handler, testAccessKey, testSecret, []byte(`{"name":"home"}`), now)
	if first.Code != http.StatusOK {
		t.Fatalf("seed status=%d body=%q", first.Code, first.Body.String())
	}
	seeded := decodeSeed(t, first)
	if seeded["accountId"] == "" || seeded["accountId"] == testAccountID || seeded["userName"] == "" || seeded["userArn"] == "" || seeded["accessKeyId"] == "" || seeded["secretAccessKey"] == "" || seeded["secretAccessKey"] == testSecret {
		t.Fatalf("seeded=%v", seeded)
	}
	key, err := st.LookupAccessKeyRecord(seeded["accessKeyId"])
	if err != nil {
		t.Fatal(err)
	}
	if key.IsRoot || key.AccountID != seeded["accountId"] || key.Secret != seeded["secretAccessKey"] {
		t.Fatalf("key=%+v", key)
	}

	second := postSeed(t, handler, testAccessKey, testSecret, []byte(`{"name":"home"}`), now)
	if second.Code != http.StatusOK {
		t.Fatalf("reseed status=%d body=%q", second.Code, second.Body.String())
	}
	again := decodeSeed(t, second)
	if again["accessKeyId"] != seeded["accessKeyId"] || again["secretAccessKey"] != seeded["secretAccessKey"] || again["accountId"] != seeded["accountId"] {
		t.Fatalf("reseed rotated first=%v second=%v", seeded, again)
	}
}

func TestStandaloneSeedInvalidBodyDoesNotCreate(t *testing.T) {
	srv, st, _ := newTestServerStoreWith(t, func(cfg *config.Config) {
		cfg.LabStandalone = true
		cfg.RootSecretAccessKey = testSecret
	})
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)
	before := accountIDSet(t, st)
	cases := [][]byte{
		[]byte(`{`),
		[]byte(`{"name":"bad name"}`),
		[]byte(`{"name":" "}`),
	}
	for _, body := range cases {
		rec := postSeed(t, handler, testAccessKey, testSecret, body, now)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("body=%q status=%d want 400 response=%q", body, rec.Code, rec.Body.String())
		}
		assertSameAccounts(t, st, before)
	}
}

func TestStandaloneSeedCrossAccountBoundary(t *testing.T) {
	srv, st, _ := newTestServerStoreWith(t, func(cfg *config.Config) {
		cfg.LabStandalone = true
		cfg.RootSecretAccessKey = testSecret
	})
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	first := postSeed(t, handler, testAccessKey, testSecret, []byte(`{"name":"home"}`), now)
	if first.Code != http.StatusOK {
		t.Fatalf("seed status=%d body=%q", first.Code, first.Body.String())
	}
	seeded := decodeSeed(t, first)
	userKey := seeded["accessKeyId"]
	userSecret := seeded["secretAccessKey"]
	home := seeded["accountId"]
	key, err := st.LookupAccessKeyRecord(userKey)
	if err != nil {
		t.Fatal(err)
	}
	if key.IsRoot {
		t.Fatal("standalone user key is root")
	}

	const homeBucket = "seed-home-bucket"
	const labBucket = "seed-lab-bucket"
	const labBytes = "lab-bytes-marker"
	create := mustS3WithCreds(t, handler, http.MethodPut, "http://127.0.0.1:4566/"+homeBucket, nil, userKey, userSecret, "s3", now, nil)
	if create.Code != http.StatusOK {
		t.Fatalf("CreateBucket status=%d body=%q", create.Code, create.Body.String())
	}
	ownList := mustS3WithCreds(t, handler, http.MethodGet, "http://127.0.0.1:4566/", nil, userKey, userSecret, "s3", now, nil)
	if ownList.Code != http.StatusOK || !strings.Contains(ownList.Body.String(), homeBucket) {
		t.Fatalf("home ListBuckets status=%d body=%q", ownList.Code, ownList.Body.String())
	}
	labList := mustS3WithCreds(t, handler, http.MethodGet, "http://127.0.0.1:4566/", nil, testAccessKey, testSecret, "s3", now, nil)
	if labList.Code != http.StatusOK || strings.Contains(labList.Body.String(), homeBucket) {
		t.Fatalf("other account ListBuckets status=%d body=%q", labList.Code, labList.Body.String())
	}

	labCreate := mustS3(t, handler, http.MethodPut, "http://127.0.0.1:4566/"+labBucket, nil, "s3", now, nil)
	if labCreate.Code != http.StatusOK {
		t.Fatalf("lab CreateBucket status=%d body=%q", labCreate.Code, labCreate.Body.String())
	}
	labPut := mustS3(t, handler, http.MethodPut, "http://127.0.0.1:4566/"+labBucket+"/secret.txt", []byte(labBytes), "s3", now, nil)
	if labPut.Code != http.StatusOK {
		t.Fatalf("lab PutObject status=%d body=%q", labPut.Code, labPut.Body.String())
	}
	denied := mustS3WithCreds(t, handler, http.MethodGet, "http://127.0.0.1:4566/"+labBucket+"/secret.txt", nil, userKey, userSecret, "s3", now, nil)
	if denied.Code != http.StatusForbidden || strings.Contains(denied.Body.String(), labBytes) {
		t.Fatalf("cross GetObject status=%d body=%q", denied.Code, denied.Body.String())
	}
	if !strings.Contains(denied.Body.String(), "AccessDenied") {
		t.Fatalf("cross GetObject want AccessDenied body=%q", denied.Body.String())
	}
	policy := `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":{"AWS":"` + seeded["userArn"] + `"},"Action":"s3:GetObject","Resource":"arn:aws:s3:::` + labBucket + `/*"}]}`
	if err := st.PutBucketPolicy(testAccountID, labBucket, policy); err != nil {
		t.Fatal(err)
	}
	allowed := mustS3WithCreds(t, handler, http.MethodGet, "http://127.0.0.1:4566/"+labBucket+"/secret.txt", nil, userKey, userSecret, "s3", now, nil)
	if allowed.Code != http.StatusOK || allowed.Body.String() != labBytes {
		t.Fatalf("allowed GetObject status=%d body=%q", allowed.Code, allowed.Body.String())
	}

	listAccounts := stsOrOrg(t, handler, "Action=ListAccounts&Version=2016-11-28", userKey, userSecret, "organizations", now)
	if listAccounts.Code != http.StatusForbidden || !strings.Contains(listAccounts.Body.String(), "AccessDenied") {
		t.Fatalf("ListAccounts status=%d body=%q", listAccounts.Code, listAccounts.Body.String())
	}
	assume := stsOrOrg(t, handler, "Action=AssumeRoot&Version=2011-06-15&TargetAccount="+home, testAccessKey, testSecret, "sts", now)
	if assume.Code != http.StatusForbidden || !strings.Contains(assume.Body.String(), "AccessDenied") || strings.Contains(assume.Body.String(), "SecretAccessKey") {
		t.Fatalf("AssumeRoot status=%d body=%q", assume.Code, assume.Body.String())
	}
	if st.IsOrgMemberAccount(testAccountID, home) {
		t.Fatal("standalone account has a create_account_requests row")
	}

	const marker = "StandaloneForeignMarker"
	policyDoc := `{"Version":"2012-10-17","Statement":[{"Sid":"` + marker + `","Effect":"Allow","Action":"s3:GetObject","Resource":"*"}]}`
	policyARN, err := st.CreateManagedPolicy(home, "HomePolicy", policyDoc)
	if err != nil {
		t.Fatal(err)
	}
	for _, body := range []string{
		"Action=ListPolicyVersions&Version=2010-05-08&PolicyArn=" + url.QueryEscape(policyARN),
		"Action=GetPolicy&Version=2010-05-08&PolicyArn=" + url.QueryEscape(policyARN),
		"Action=GetPolicyVersion&Version=2010-05-08&PolicyArn=" + url.QueryEscape(policyARN) + "&VersionId=v1",
	} {
		rec := iamForm(t, handler, body, now)
		if rec.Code != http.StatusForbidden || !strings.Contains(rec.Body.String(), "AccessDenied") || strings.Contains(rec.Body.String(), marker) {
			t.Fatalf("foreign policy body=%s status=%d response=%q", body, rec.Code, rec.Body.String())
		}
	}
	awsManaged := iamForm(t, handler, "Action=GetPolicy&Version=2010-05-08&PolicyArn="+url.QueryEscape("arn:aws:iam::aws:policy/AdministratorAccess"), now)
	if awsManaged.Code == http.StatusForbidden && strings.Contains(awsManaged.Body.String(), "AccessDenied") {
		t.Fatalf("aws managed GetPolicy denied body=%q", awsManaged.Body.String())
	}

	const bad = "000000000067"
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
	badList := mustS3WithCreds(t, handler, http.MethodGet, "http://127.0.0.1:4566/", nil, badKey, badSecret, "s3", now, nil)
	if strings.Contains(badList.Body.String(), "bad-parent-bucket") {
		t.Fatalf("bad parent listed a bucket status=%d body=%q", badList.Code, badList.Body.String())
	}
}

func TestStandaloneUserCannotMutateForeignIAM(t *testing.T) {
	srv, st, _ := newTestServerStoreWith(t, func(cfg *config.Config) {
		cfg.LabStandalone = true
		cfg.RootSecretAccessKey = testSecret
	})
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)
	rec := postSeed(t, handler, testAccessKey, testSecret, []byte(`{"name":"iam-home"}`), now)
	if rec.Code != http.StatusOK {
		t.Fatalf("seed status=%d body=%q", rec.Code, rec.Body.String())
	}
	seeded := decodeSeed(t, rec)
	home := seeded["accountId"]
	userKey := seeded["accessKeyId"]
	userSecret := seeded["secretAccessKey"]

	policyARN, err := st.CreateManagedPolicy(testAccountID, "LabOnlyPolicy", `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Action":"s3:ListBucket","Resource":"*"}]}`)
	if err != nil {
		t.Fatal(err)
	}
	for _, action := range []string{"DeletePolicy", "CreatePolicyVersion", "DeletePolicyVersion", "SetDefaultPolicyVersion"} {
		body := "Action=" + action + "&Version=2010-05-08&PolicyArn=" + url.QueryEscape(policyARN) + "&VersionId=v1&PolicyDocument=" + url.QueryEscape(`{"Version":"2012-10-17","Statement":[{"Effect":"Deny","Action":"*","Resource":"*"}]}`)
		got := stsOrOrg(t, handler, body, userKey, userSecret, "iam", now)
		if got.Code != http.StatusForbidden || !strings.Contains(got.Body.String(), "AccessDenied") {
			t.Fatalf("%s status=%d body=%q", action, got.Code, got.Body.String())
		}
	}
	if _, err := st.GetManagedPolicy(policyARN); err != nil {
		t.Fatalf("foreign policy mutation changed the lab policy: %v", err)
	}

	providerARN, err := st.PutOIDCProvider(testAccountID, "https://idp.example/lab", "lab-client")
	if err != nil {
		t.Fatal(err)
	}
	getProvider := stsOrOrg(t, handler, "Action=GetOpenIDConnectProvider&Version=2010-05-08&OpenIDConnectProviderArn="+url.QueryEscape(providerARN), userKey, userSecret, "iam", now)
	if getProvider.Code != http.StatusForbidden {
		t.Fatalf("GetOpenIDConnectProvider status=%d body=%q", getProvider.Code, getProvider.Body.String())
	}
	delProvider := stsOrOrg(t, handler, "Action=DeleteOpenIDConnectProvider&Version=2010-05-08&OpenIDConnectProviderArn="+url.QueryEscape(providerARN), userKey, userSecret, "iam", now)
	if delProvider.Code != http.StatusForbidden {
		t.Fatalf("DeleteOpenIDConnectProvider status=%d body=%q", delProvider.Code, delProvider.Body.String())
	}
	if _, err := st.GetOIDCProvider(providerARN); err != nil {
		t.Fatalf("foreign provider delete removed the lab provider: %v", err)
	}

	ownARN, err := st.CreateManagedPolicy(home, "HomeOnlyPolicy", `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Action":"s3:ListBucket","Resource":"*"}]}`)
	if err != nil {
		t.Fatal(err)
	}
	ownDelete := stsOrOrg(t, handler, "Action=DeletePolicy&Version=2010-05-08&PolicyArn="+url.QueryEscape(ownARN), userKey, userSecret, "iam", now)
	if ownDelete.Code != http.StatusOK {
		t.Fatalf("own DeletePolicy status=%d body=%q", ownDelete.Code, ownDelete.Body.String())
	}
}

func accountIDSet(t *testing.T, st *store.Store) map[string]bool {
	t.Helper()
	accounts, err := st.ListAccounts()
	if err != nil {
		t.Fatal(err)
	}
	out := make(map[string]bool, len(accounts))
	for _, account := range accounts {
		out[account.AccountID] = true
	}
	return out
}

func assertSameAccounts(t *testing.T, st *store.Store, before map[string]bool) {
	t.Helper()
	after := accountIDSet(t, st)
	if len(after) != len(before) {
		t.Fatalf("account set changed before=%d after=%d", len(before), len(after))
	}
	for id := range after {
		if !before[id] {
			t.Fatalf("created account %s", id)
		}
	}
}

func decodeSeed(t *testing.T, rec *httptest.ResponseRecorder) map[string]string {
	t.Helper()
	var seeded map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &seeded); err != nil {
		t.Fatal(err)
	}
	return seeded
}
