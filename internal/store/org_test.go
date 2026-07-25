package store_test

import (
	"database/sql"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
	"github.com/Kyaxris-Labs/Noctaxris/internal/validate"
)

func openTestStore(t *testing.T) *store.Store {
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
	return st
}

func TestCreateMemberAccount(t *testing.T) {
	st := openTestStore(t)
	const mgmt = "000000000001"
	if err := st.EnsureRoot(mgmt, "AKIAROOTEXAMPLE01", "secret"); err != nil {
		t.Fatal(err)
	}

	reqID, accountID, err := st.CreateMemberAccount(mgmt, "member@example.com", "Member")
	if err != nil {
		t.Fatal(err)
	}
	if reqID == "" || !strings.HasPrefix(reqID, "car-") {
		t.Fatalf("requestID = %q", reqID)
	}
	if accountID != "000000000002" {
		t.Fatalf("accountID = %q, want 000000000002", accountID)
	}
	if accountID == mgmt {
		t.Fatal("member account id must not equal management")
	}

	status, gotAcc, email, failure, requestedBy, err := st.DescribeCreateAccountStatus(reqID)
	if err != nil {
		t.Fatal(err)
	}
	if status != "SUCCEEDED" || gotAcc != accountID || email != "member@example.com" || failure != "" {
		t.Fatalf("status=%q acc=%q email=%q failure=%q", status, gotAcc, email, failure)
	}
	if requestedBy != mgmt {
		t.Fatalf("requestedBy = %q, want %q", requestedBy, mgmt)
	}

	roleARN, trust, err := st.GetRole(accountID, store.OrganizationAccountAccessRoleName)
	if err != nil {
		t.Fatal(err)
	}
	wantARN := store.RoleARN(accountID, store.OrganizationAccountAccessRoleName)
	if roleARN != wantARN {
		t.Fatalf("roleARN = %q, want %q", roleARN, wantARN)
	}
	wantTrust := store.OrganizationAccountAccessRoleTrustPolicy(mgmt)
	if trust != wantTrust {
		t.Fatalf("trust = %q, want %q", trust, wantTrust)
	}
	if !strings.Contains(trust, "arn:aws:iam::"+mgmt+":root") {
		t.Fatalf("trust missing mgmt principal: %s", trust)
	}
	if !strings.Contains(trust, "sts:AssumeRole") {
		t.Fatalf("trust missing sts:AssumeRole: %s", trust)
	}
}

func TestCreateMemberAccountRequiresManagement(t *testing.T) {
	st := openTestStore(t)
	const nonMgmt = "000000000002"
	if err := st.EnsureRoot(nonMgmt, "AKIAROOTEXAMPLE02", "secret"); err != nil {
		t.Fatal(err)
	}
	_, _, err := st.CreateMemberAccount(nonMgmt, "a@example.com", "A")
	if err == nil {
		t.Fatal("expected CreateMemberAccount to reject non-management account")
	}
	if !validate.IsInvalid(err) {
		t.Fatalf("want invalid error, got %v", err)
	}
}

func TestCreateMemberAccountSkipsMgmtID(t *testing.T) {
	st := openTestStore(t)
	const mgmt = store.ManagementAccountID
	if err := st.EnsureRoot(mgmt, "AKIAROOTEXAMPLE01", "secret"); err != nil {
		t.Fatal(err)
	}
	// Pre-seed an account that would collide with sequential allocation start,
	// forcing allocateAccountID to skip past ManagementAccountID when scanning.
	if err := st.EnsureRoot("000000000002", "AKIAROOTPRESEED02", "secret-pre"); err != nil {
		t.Fatal(err)
	}
	_, accountID, err := st.CreateMemberAccount(mgmt, "a@example.com", "A")
	if err != nil {
		t.Fatal(err)
	}
	if accountID == mgmt {
		t.Fatal("allocated mgmt id")
	}
	if accountID != "000000000003" {
		t.Fatalf("accountID = %q, want 000000000003 (skip existing 000000000002)", accountID)
	}
}

func TestPutGetRole(t *testing.T) {
	st := openTestStore(t)
	const accountID = "000000000001"
	arn := store.RoleARN(accountID, "DemoRole")
	trust := `{"Version":"2012-10-17","Statement":[]}`
	if err := st.PutRole(accountID, "DemoRole", arn, trust); err != nil {
		t.Fatal(err)
	}
	gotARN, gotTrust, err := st.GetRole(accountID, "DemoRole")
	if err != nil {
		t.Fatal(err)
	}
	if gotARN != arn || gotTrust != trust {
		t.Fatalf("got ARN=%q trust=%q", gotARN, gotTrust)
	}
	_, _, err = st.GetRole(accountID, "Missing")
	if err == nil {
		t.Fatal("expected error for missing role")
	}
	if err != sql.ErrNoRows {
		t.Fatalf("err = %v, want sql.ErrNoRows", err)
	}
}

func TestMintTempCredentialsAndLookup(t *testing.T) {
	st := openTestStore(t)
	const accountID = "000000000002"
	roleARN := store.RoleARN(accountID, store.OrganizationAccountAccessRoleName)
	expires := time.Now().UTC().Add(time.Hour).Truncate(time.Second)
	keyID, err := st.MintTempCredentials(accountID, roleARN, "admin-session", "temp-secret", "temp-session-token", expires)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(keyID, "ASIA") {
		t.Fatalf("accessKeyID = %q, want ASIA prefix", keyID)
	}

	ak, err := st.LookupAccessKeyRecord(keyID)
	if err != nil {
		t.Fatal(err)
	}
	if ak.AccountID != accountID || ak.Secret != "temp-secret" || ak.IsRoot {
		t.Fatalf("ak=%+v", ak)
	}
	if ak.SessionToken != "temp-session-token" {
		t.Fatalf("SessionToken = %q", ak.SessionToken)
	}
	if ak.RoleARN != roleARN || ak.SessionName != "admin-session" {
		t.Fatalf("RoleARN=%q SessionName=%q", ak.RoleARN, ak.SessionName)
	}
	if !ak.ExpiresAt.Equal(expires) {
		t.Fatalf("ExpiresAt = %v, want %v", ak.ExpiresAt, expires)
	}

	acc, sec, root, err := st.LookupAccessKey(keyID)
	if err != nil || root || acc != accountID || sec != "temp-secret" {
		t.Fatalf("wrapper acc=%s root=%v sec=%q err=%v", acc, root, sec, err)
	}
}

func TestLookupAccessKeyRecordLongLivedEmptySession(t *testing.T) {
	st := openTestStore(t)
	if err := st.EnsureRoot("000000000001", "AKIAROOTEXAMPLE01", "secret-root"); err != nil {
		t.Fatal(err)
	}
	ak, err := st.LookupAccessKeyRecord("AKIAROOTEXAMPLE01")
	if err != nil {
		t.Fatal(err)
	}
	if ak.SessionToken != "" || ak.RoleARN != "" || ak.SessionName != "" || !ak.ExpiresAt.IsZero() {
		t.Fatalf("expected empty session fields, got %+v", ak)
	}
	if !ak.IsRoot || ak.Secret != "secret-root" {
		t.Fatalf("ak=%+v", ak)
	}
}

func TestDescribeCreateAccountStatusMissing(t *testing.T) {
	st := openTestStore(t)
	_, _, _, _, _, err := st.DescribeCreateAccountStatus("car-missing")
	if err == nil {
		t.Fatal("expected error")
	}
}
