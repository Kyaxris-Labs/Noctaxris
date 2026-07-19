package store_test

import (
	"database/sql"
	"strings"
	"testing"

	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

func TestUsersAndAccessKeys(t *testing.T) {
	st := openTestStore(t)
	const accountID = "000000000001"

	userID, arn, err := st.CreateUser(accountID, "alice")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(userID, "AIDA") {
		t.Fatalf("userID = %q", userID)
	}
	wantARN := store.UserARN(accountID, "/", "alice")
	if arn != wantARN {
		t.Fatalf("arn = %q, want %q", arn, wantARN)
	}

	u, err := st.GetUser(accountID, "alice")
	if err != nil {
		t.Fatal(err)
	}
	if u.UserID != userID || u.ARN != arn || u.UserName != "alice" {
		t.Fatalf("user = %+v", u)
	}

	users, err := st.ListUsers(accountID)
	if err != nil {
		t.Fatal(err)
	}
	if len(users) != 1 || users[0].UserName != "alice" {
		t.Fatalf("users = %#v", users)
	}

	keyID, secret, err := st.CreateUserAccessKey(accountID, "alice")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(keyID, "AKIA") || secret == "" {
		t.Fatalf("keyID=%q secret empty=%v", keyID, secret == "")
	}

	ak, err := st.LookupAccessKeyRecord(keyID)
	if err != nil {
		t.Fatal(err)
	}
	if ak.IsRoot || ak.UserName != "alice" || ak.Secret != secret || ak.SessionToken != "" {
		t.Fatalf("ak=%+v", ak)
	}
	if ak.Status != store.AccessKeyStatusActive {
		t.Fatalf("status = %q", ak.Status)
	}

	keys, err := st.ListAccessKeys(accountID, "alice")
	if err != nil {
		t.Fatal(err)
	}
	if len(keys) != 1 || keys[0].AccessKeyID != keyID {
		t.Fatalf("keys = %#v", keys)
	}

	if err := st.UpdateAccessKey(keyID, store.AccessKeyStatusInactive); err != nil {
		t.Fatal(err)
	}
	ak, err = st.LookupAccessKeyRecord(keyID)
	if err != nil {
		t.Fatal(err)
	}
	if ak.Status != store.AccessKeyStatusInactive {
		t.Fatalf("status = %q", ak.Status)
	}

	if err := st.DeleteUser(accountID, "alice"); err == nil {
		t.Fatal("expected delete user to fail while keys remain")
	}

	if err := st.DeleteAccessKey(keyID); err != nil {
		t.Fatal(err)
	}
	if err := st.DeleteUser(accountID, "alice"); err != nil {
		t.Fatal(err)
	}
	_, err = st.GetUser(accountID, "alice")
	if err != sql.ErrNoRows {
		t.Fatalf("err = %v, want sql.ErrNoRows", err)
	}
}

func TestManagedAndInlinePolicies(t *testing.T) {
	st := openTestStore(t)
	const accountID = "000000000001"
	doc := `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Action":"s3:*","Resource":"*"}]}`

	_, userARN, err := st.CreateUser(accountID, "bob")
	if err != nil {
		t.Fatal(err)
	}
	roleARN, err := st.CreateRole(accountID, "AppRole", `{"Version":"2012-10-17","Statement":[]}`)
	if err != nil {
		t.Fatal(err)
	}

	policyARN, err := st.CreateManagedPolicy(accountID, "AllowS3", doc)
	if err != nil {
		t.Fatal(err)
	}
	wantARN := store.PolicyARN(accountID, "/", "AllowS3")
	if policyARN != wantARN {
		t.Fatalf("policyARN = %q, want %q", policyARN, wantARN)
	}

	mp, err := st.GetManagedPolicy(policyARN)
	if err != nil {
		t.Fatal(err)
	}
	if mp.Document != doc || mp.PolicyName != "AllowS3" || mp.DefaultVersionID != "v1" {
		t.Fatalf("mp=%+v", mp)
	}

	listed, err := st.ListManagedPolicies(accountID)
	if err != nil || len(listed) != 1 {
		t.Fatalf("listed=%#v err=%v", listed, err)
	}

	if err := st.AttachUserPolicy(accountID, "bob", policyARN); err != nil {
		t.Fatal(err)
	}
	docs, err := st.ListAttachedPolicyDocuments(userARN)
	if err != nil {
		t.Fatal(err)
	}
	if len(docs) != 1 || docs[0] != doc {
		t.Fatalf("docs=%#v", docs)
	}
	if err := st.DetachUserPolicy(accountID, "bob", policyARN); err != nil {
		t.Fatal(err)
	}

	if err := st.AttachRolePolicy(accountID, "AppRole", policyARN); err != nil {
		t.Fatal(err)
	}
	docs, err = st.ListAttachedPolicyDocuments(roleARN)
	if err != nil || len(docs) != 1 || docs[0] != doc {
		t.Fatalf("role docs=%#v err=%v", docs, err)
	}
	if err := st.DetachRolePolicy(accountID, "AppRole", policyARN); err != nil {
		t.Fatal(err)
	}

	if err := st.DeleteManagedPolicy(policyARN); err != nil {
		t.Fatal(err)
	}

	inlineDoc := `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Action":"sts:*","Resource":"*"}]}`
	if err := st.PutInlinePolicy(userARN, "inline1", inlineDoc); err != nil {
		t.Fatal(err)
	}
	got, err := st.GetInlinePolicy(userARN, "inline1")
	if err != nil || got.Document != inlineDoc {
		t.Fatalf("got=%+v err=%v", got, err)
	}
	inlines, err := st.ListInlinePolicies(userARN)
	if err != nil || len(inlines) != 1 {
		t.Fatalf("inlines=%#v err=%v", inlines, err)
	}
	if err := st.DeleteInlinePolicy(userARN, "inline1"); err != nil {
		t.Fatal(err)
	}
}

func TestCreateListDeleteRoles(t *testing.T) {
	st := openTestStore(t)
	const accountID = "000000000001"
	trust := `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":{"AWS":"*"},"Action":"sts:AssumeRole"}]}`

	arn, err := st.CreateRole(accountID, "Demo", trust)
	if err != nil {
		t.Fatal(err)
	}
	if arn != store.RoleARN(accountID, "Demo") {
		t.Fatalf("arn = %q", arn)
	}

	roles, err := st.ListRoles(accountID)
	if err != nil || len(roles) != 1 || roles[0].RoleName != "Demo" {
		t.Fatalf("roles=%#v err=%v", roles, err)
	}
	if !strings.HasPrefix(roles[0].RoleID, "AROA") {
		t.Fatalf("roleID = %q", roles[0].RoleID)
	}

	updated := `{"Version":"2012-10-17","Statement":[]}`
	if err := st.UpdateAssumeRolePolicy(accountID, "Demo", updated); err != nil {
		t.Fatal(err)
	}
	_, gotTrust, err := st.GetRole(accountID, "Demo")
	if err != nil || gotTrust != updated {
		t.Fatalf("trust=%q err=%v", gotTrust, err)
	}

	if err := st.DeleteRole(accountID, "Demo"); err != nil {
		t.Fatal(err)
	}
	_, _, err = st.GetRole(accountID, "Demo")
	if err != sql.ErrNoRows {
		t.Fatalf("err = %v, want sql.ErrNoRows", err)
	}
}

func TestOIDCAndSAMLProviders(t *testing.T) {
	st := openTestStore(t)
	const accountID = "000000000001"

	oidcARN, err := st.PutOIDCProvider(accountID, "https://token.actions.githubusercontent.com", "sts.amazonaws.com")
	if err != nil {
		t.Fatal(err)
	}
	wantOIDC := store.OIDCProviderARN(accountID, "https://token.actions.githubusercontent.com")
	if oidcARN != wantOIDC {
		t.Fatalf("oidcARN = %q, want %q", oidcARN, wantOIDC)
	}
	p, err := st.GetOIDCProviderByURL(accountID, "https://token.actions.githubusercontent.com")
	if err != nil || p.ClientID != "sts.amazonaws.com" {
		t.Fatalf("p=%+v err=%v", p, err)
	}
	listed, err := st.ListOIDCProviders(accountID)
	if err != nil || len(listed) != 1 {
		t.Fatalf("listed=%#v err=%v", listed, err)
	}

	samlARN, err := st.PutSAMLProvider(accountID, "MyIdP", "<EntityDescriptor/>")
	if err != nil {
		t.Fatal(err)
	}
	sp, err := st.GetSAMLProvider(samlARN)
	if err != nil || sp.MetadataXML != "<EntityDescriptor/>" {
		t.Fatalf("sp=%+v err=%v", sp, err)
	}
}
