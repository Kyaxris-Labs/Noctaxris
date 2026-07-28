package server

import (
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/Kyaxris-Labs/Noctaxris/internal/config"
	"github.com/Kyaxris-Labs/Noctaxris/internal/kernel/audit"
	"github.com/Kyaxris-Labs/Noctaxris/internal/kernel/authn"
	"github.com/Kyaxris-Labs/Noctaxris/internal/kernel/identity"
	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

func accessKeyPlumbServer(t *testing.T) (*Server, *store.Store) {
	t.Helper()
	dir := t.TempDir()
	key, err := store.LoadOrCreateMasterKey(filepath.Join(dir, "master.key"))
	if err != nil {
		t.Fatal(err)
	}
	st, err := store.Open(dir, key)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	accountID := "000000000001"
	if err := st.EnsureRoot(accountID, "AKIAROOTEXAMPLE01", "secret-root-value"); err != nil {
		t.Fatal(err)
	}
	aud, err := audit.NewWriter(filepath.Join(dir, "cloudtrail"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = aud.Close() })
	srv := New(config.Config{ListenAddr: "127.0.0.1:4566", DataRoot: dir, AccountID: accountID}, st, aud)
	return srv, st
}

func TestAccessKeyPlumbLookupKeyMapsMFATime(t *testing.T) {
	srv, st := accessKeyPlumbServer(t)
	accountID := "000000000001"
	mfaAt := time.Date(2026, 7, 28, 12, 0, 0, 0, time.UTC)
	akid, err := st.MintTempCredentialsOpts(store.MintTempOpts{
		AccountID:          accountID,
		UserName:           "mfa-user",
		Secret:             "temp-secret",
		SessionToken:       "temp-token",
		Expires:            time.Now().UTC().Add(time.Hour),
		MFAAuthenticated:   true,
		MFAAuthenticatedAt: mfaAt,
	})
	if err != nil {
		t.Fatal(err)
	}

	got, err := srv.lookupKey(akid)
	if err != nil {
		t.Fatal(err)
	}
	if !got.MFAAuthenticated {
		t.Fatal("expected MFAAuthenticated=true on ResolvedKey")
	}
	if !got.MFAAuthenticatedAt.Equal(mfaAt) {
		t.Fatalf("MFAAuthenticatedAt=%v want %v", got.MFAAuthenticatedAt, mfaAt)
	}
	if got.UserName != "mfa-user" {
		t.Fatalf("UserName=%q", got.UserName)
	}
}

func TestAccessKeyPlumbMFAConditionKeysAfterDeleteAccessKey(t *testing.T) {
	srv, st := accessKeyPlumbServer(t)
	accountID := "000000000001"
	mfaAt := time.Date(2026, 7, 28, 12, 0, 0, 0, time.UTC)
	akid, err := st.MintTempCredentialsOpts(store.MintTempOpts{
		AccountID:          accountID,
		UserName:           "mfa-user",
		Secret:             "temp-secret",
		SessionToken:       "temp-token",
		Expires:            time.Now().UTC().Add(time.Hour),
		MFAAuthenticated:   true,
		MFAAuthenticatedAt: mfaAt,
	})
	if err != nil {
		t.Fatal(err)
	}

	resolved, err := srv.lookupKey(akid)
	if err != nil {
		t.Fatal(err)
	}
	now := mfaAt.Add(90 * time.Second)
	srv.setLabClock(now)

	verified := &authn.Verified{
		Principal:          identity.UserPrincipal(accountID, "mfa-user", akid),
		AccessKeyID:        akid,
		AccountID:          accountID,
		Region:             "us-east-1",
		MFAAuthenticated:   resolved.MFAAuthenticated,
		MFAAuthenticatedAt: resolved.MFAAuthenticatedAt,
		UserName:           resolved.UserName,
		IsRoot:             resolved.IsRoot,
		SessionPolicy:      resolved.SessionPolicy,
		FederatedUser:      resolved.FederatedUser,
	}

	if err := st.DeleteAccessKey(akid); err != nil {
		t.Fatal(err)
	}
	if _, err := st.LookupAccessKeyRecord(akid); err == nil {
		t.Fatal("expected key gone from store")
	}

	keys := srv.conditionKeys(verified)
	if keys["aws:MultiFactorAuthPresent"] != "true" {
		t.Fatalf("MultiFactorAuthPresent=%q after DeleteAccessKey", keys["aws:MultiFactorAuthPresent"])
	}
	age, err := strconv.Atoi(keys["aws:MultiFactorAuthAge"])
	if err != nil {
		t.Fatalf("MultiFactorAuthAge=%q: %v", keys["aws:MultiFactorAuthAge"], err)
	}
	if age != 90 {
		t.Fatalf("MultiFactorAuthAge=%d want 90 (recomputed from MFAAuthenticatedAt+now)", age)
	}
}

func TestAccessKeyPlumbSessionPolicyDocsAfterKeyDelete(t *testing.T) {
	srv, st := accessKeyPlumbServer(t)
	accountID := "000000000001"
	policy := `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Action":"sts:GetCallerIdentity","Resource":"*"}]}`
	akid, err := st.MintTempCredentialsOpts(store.MintTempOpts{
		AccountID:     accountID,
		UserName:      "sess-user",
		Secret:        "temp-secret",
		SessionToken:  "temp-token",
		SessionPolicy: policy,
		Expires:       time.Now().UTC().Add(time.Hour),
	})
	if err != nil {
		t.Fatal(err)
	}
	resolved, err := srv.lookupKey(akid)
	if err != nil {
		t.Fatal(err)
	}
	verified := &authn.Verified{
		Principal:     identity.UserPrincipal(accountID, "sess-user", akid),
		AccessKeyID:   akid,
		AccountID:     accountID,
		SessionPolicy: resolved.SessionPolicy,
		UserName:      resolved.UserName,
	}
	if err := st.DeleteAccessKey(akid); err != nil {
		t.Fatal(err)
	}

	docs := srv.sessionPolicyDocs(verified)
	if len(docs) != 1 || docs[0] != policy {
		t.Fatalf("sessionPolicyDocs after delete = %#v want [%q]", docs, policy)
	}
}

func TestAccessKeyPlumbFederatedEvalInputsWithoutLookup(t *testing.T) {
	srv, st := accessKeyPlumbServer(t)
	accountID := "000000000001"
	if _, _, err := st.CreateUser(accountID, "fed-caller"); err != nil {
		t.Fatal(err)
	}
	userARN := "arn:aws:iam::" + accountID + ":user/fed-caller"
	identityDoc := `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Action":"sts:GetCallerIdentity","Resource":"*"}]}`
	if err := st.PutInlinePolicy(userARN, "fed-id", identityDoc); err != nil {
		t.Fatal(err)
	}
	sessionPolicy := `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Action":"sts:GetCallerIdentity","Resource":"*"}]}`
	akid, err := st.MintTempCredentialsOpts(store.MintTempOpts{
		AccountID:     accountID,
		UserName:      "fed-caller",
		FederatedUser: "broker",
		Secret:        "temp-secret",
		SessionToken:  "temp-token",
		SessionPolicy: sessionPolicy,
		Expires:       time.Now().UTC().Add(time.Hour),
	})
	if err != nil {
		t.Fatal(err)
	}
	resolved, err := srv.lookupKey(akid)
	if err != nil {
		t.Fatal(err)
	}
	verified := &authn.Verified{
		Principal:     identity.FederatedUserPrincipal(accountID, "broker", akid),
		AccessKeyID:   akid,
		AccountID:     accountID,
		Region:        "us-east-1",
		SessionPolicy: resolved.SessionPolicy,
		UserName:      resolved.UserName,
		IsRoot:        resolved.IsRoot,
		FederatedUser: resolved.FederatedUser,
	}
	if err := st.DeleteAccessKey(akid); err != nil {
		t.Fatal(err)
	}

	in, ok := srv.evalInputs(verified)
	if !ok {
		t.Fatal("evalInputs failed after DeleteAccessKey")
	}
	if len(in.SessionDocs) != 1 || in.SessionDocs[0] != sessionPolicy {
		t.Fatalf("SessionDocs=%#v", in.SessionDocs)
	}
	if len(in.IdentityDocs) == 0 {
		t.Fatal("expected calling IAM user identity docs without re-Lookup")
	}
	found := false
	for _, doc := range in.IdentityDocs {
		if doc == identityDoc {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("identity docs missing caller policy: %#v", in.IdentityDocs)
	}
}

func TestAccessKeyPlumbFederatedEmptySessionDenyStubWithoutLookup(t *testing.T) {
	srv, st := accessKeyPlumbServer(t)
	accountID := "000000000001"
	if _, _, err := st.CreateUser(accountID, "fed-empty"); err != nil {
		t.Fatal(err)
	}
	akid, err := st.MintTempCredentialsOpts(store.MintTempOpts{
		AccountID:     accountID,
		UserName:      "fed-empty",
		FederatedUser: "broker",
		Secret:        "temp-secret",
		SessionToken:  "temp-token",
		Expires:       time.Now().UTC().Add(time.Hour),
	})
	if err != nil {
		t.Fatal(err)
	}
	resolved, err := srv.lookupKey(akid)
	if err != nil {
		t.Fatal(err)
	}
	verified := &authn.Verified{
		Principal:     identity.FederatedUserPrincipal(accountID, "broker", akid),
		AccessKeyID:   akid,
		AccountID:     accountID,
		Region:        "us-east-1",
		SessionPolicy: resolved.SessionPolicy,
		UserName:      resolved.UserName,
		IsRoot:        resolved.IsRoot,
		FederatedUser: resolved.FederatedUser,
	}
	if err := st.DeleteAccessKey(akid); err != nil {
		t.Fatal(err)
	}

	in, ok := srv.evalInputs(verified)
	if !ok {
		t.Fatal("evalInputs failed")
	}
	wantStub := `{"Version":"2012-10-17","Statement":[]}`
	if len(in.SessionDocs) != 1 || in.SessionDocs[0] != wantStub {
		t.Fatalf("empty federated session stub=%#v want [%q]", in.SessionDocs, wantStub)
	}
}
