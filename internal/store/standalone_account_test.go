package store_test

import (
	"strings"
	"testing"

	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

func TestOutsideParentSkipsOrgFilters(t *testing.T) {
	st := openTestStore(t)
	const mgmt = "000000000001"
	if err := st.EnsureRoot(mgmt, "AKIAROOTEXAMPLE01", "secret"); err != nil {
		t.Fatal(err)
	}
	scpID, err := st.CreateOrgPolicy("SCP", "DenyS3", `{"Version":"2012-10-17","Statement":[{"Effect":"Deny","Action":"s3:*","Resource":"*"}]}`)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.AttachOrgPolicy(scpID, "root", store.OrgRootID); err != nil {
		t.Fatal(err)
	}
	rcpID, err := st.CreateOrgPolicy("RCP", "DenyRCP", `{"Version":"2012-10-17","Statement":[{"Effect":"Deny","Action":"*","Resource":"*"}]}`)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.AttachOrgPolicy(rcpID, "root", store.OrgRootID); err != nil {
		t.Fatal(err)
	}

	const missing = "000000000077"
	if err := st.InsertAccount(missing, ""); err != nil {
		t.Fatal(err)
	}
	parent, err := st.AccountParentID(missing)
	if err != nil || parent != store.OrgRootID {
		t.Fatalf("missing parent row=%q err=%v", parent, err)
	}
	scp, rcp, err := st.OrgFilterDocsForAccount(missing)
	if err != nil {
		t.Fatal(err)
	}
	if !docsContain(scp, "Deny") || !docsContain(rcp, "Deny") {
		t.Fatalf("missing row should inherit root filters scp=%v rcp=%v", scp, rcp)
	}

	seeded, err := st.SeedStandaloneAccount("home", "secret")
	if err != nil {
		t.Fatal(err)
	}
	if seeded.AccountID == mgmt || seeded.SecretAccessKey == "secret" || seeded.SecretAccessKey == "" {
		t.Fatalf("seed account=%s rejectedSecret=%v emptySecret=%v", seeded.AccountID, seeded.SecretAccessKey == "secret", seeded.SecretAccessKey == "")
	}
	parent, err = st.AccountParentID(seeded.AccountID)
	if err != nil || parent != store.OrgOutsideID {
		t.Fatalf("outside parent=%q err=%v", parent, err)
	}
	scp, rcp, err = st.OrgFilterDocsForAccount(seeded.AccountID)
	if err != nil {
		t.Fatal(err)
	}
	if len(scp) != 0 || len(rcp) != 0 {
		t.Fatalf("outside account inherited filters scp=%v rcp=%v", scp, rcp)
	}
	path, err := st.OUPathToRoot(seeded.AccountID)
	if err != nil || len(path) != 0 {
		t.Fatalf("outside ou path=%v err=%v", path, err)
	}
	listed, err := st.ListAccountsForParent(store.OrgRootID)
	if err != nil {
		t.Fatal(err)
	}
	sawMissing := false
	for _, account := range listed {
		if account.AccountID == seeded.AccountID {
			t.Fatalf("outside account listed under %s", store.OrgRootID)
		}
		if account.AccountID == missing {
			sawMissing = true
		}
	}
	if !sawMissing {
		t.Fatal("account with no parent row was omitted from r-root")
	}
	parents, err := st.ListParents(seeded.AccountID)
	if err != nil || len(parents) != 0 {
		t.Fatalf("outside parents=%v err=%v", parents, err)
	}
	if st.IsOrgMemberAccount(mgmt, seeded.AccountID) {
		t.Fatal("standalone account must not be an organization member")
	}
	if _, _, err := st.GetRole(seeded.AccountID, store.OrganizationAccountAccessRoleName); err == nil {
		t.Fatal("standalone account has OrganizationAccountAccessRole")
	}
	if seeded.UserName != "admin" {
		t.Fatalf("user = %q", seeded.UserName)
	}
	pol, err := st.GetInlinePolicy(seeded.UserARN, "AdministratorAccess")
	if err != nil {
		t.Fatal(err)
	}
	const wantDoc = `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Action":"*","Resource":"*"}]}`
	if pol.Document != wantDoc {
		t.Fatalf("admin policy = %s", pol.Document)
	}
	if err := st.SetAccountParent(seeded.AccountID, store.OrgOutsideID); err == nil {
		t.Fatal("SetAccountParent accepted outside")
	}
	parent, err = st.AccountParentID(seeded.AccountID)
	if err != nil || parent != store.OrgOutsideID {
		t.Fatalf("parent after rejected set=%q err=%v", parent, err)
	}
	again, err := st.SeedStandaloneAccount("home", seeded.SecretAccessKey)
	if err != nil {
		t.Fatal(err)
	}
	if again.AccessKeyID != seeded.AccessKeyID || again.SecretAccessKey != seeded.SecretAccessKey || again.AccountID != seeded.AccountID {
		t.Fatalf("reseed rotated credentials account=%s key=%s", seeded.AccountID, seeded.AccessKeyID)
	}
	key, err := st.LookupAccessKeyRecord(seeded.AccessKeyID)
	if err != nil {
		t.Fatal(err)
	}
	if key.IsRoot || key.UserName != "admin" || key.AccountID != seeded.AccountID {
		t.Fatalf("standalone key root=%v user=%q account=%q", key.IsRoot, key.UserName, key.AccountID)
	}

	const bad = "000000000088"
	if err := st.InsertAccount(bad, "not-a-parent"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := st.OrgFilterDocsForAccount(bad); err == nil {
		t.Fatal("bad parent should fail closed")
	}
	if _, err := st.OUPathToRoot(bad); err == nil {
		t.Fatal("bad parent ou path should fail closed")
	}

	const otherRoot = "000000000099"
	if err := st.InsertAccount(otherRoot, "r-other"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := st.OrgFilterDocsForAccount(otherRoot); err == nil {
		t.Fatal("other r-* parent should fail closed")
	}
	if _, err := st.OUPathToRoot(otherRoot); err == nil {
		t.Fatal("other r-* ou path should fail closed")
	}
}

func TestStandaloneAccountSeedNameBoundary(t *testing.T) {
	st := openTestStore(t)
	before, err := st.ListAccounts()
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"", " ", "bad name", "bad/name"} {
		if _, err := st.SeedStandaloneAccount(name, "secret"); err == nil {
			t.Fatalf("seed %q succeeded", name)
		}
	}
	after, err := st.ListAccounts()
	if err != nil {
		t.Fatal(err)
	}
	if len(after) != len(before) {
		t.Fatalf("boundary seeds created accounts before=%d after=%d", len(before), len(after))
	}
}

func TestStandaloneAccountDistinctSeeds(t *testing.T) {
	st := openTestStore(t)
	first, err := st.SeedStandaloneAccount("home", "avoid-me")
	if err != nil {
		t.Fatal(err)
	}
	second, err := st.SeedStandaloneAccount("other", "avoid-me")
	if err != nil {
		t.Fatal(err)
	}
	if first.AccountID == second.AccountID {
		t.Fatalf("distinct seeds share account %s", first.AccountID)
	}
	if first.AccessKeyID == second.AccessKeyID {
		t.Fatalf("distinct seeds share access key %s", first.AccessKeyID)
	}
	if !twelveDigitAccountID(first.AccountID) || !twelveDigitAccountID(second.AccountID) {
		t.Fatalf("account ids = %s %s", first.AccountID, second.AccountID)
	}
	if first.AccountID == "000000000001" || second.AccountID == "000000000001" {
		t.Fatal("seed used the management account id")
	}
	if first.SecretAccessKey == "avoid-me" || second.SecretAccessKey == "avoid-me" || first.SecretAccessKey == "" || second.SecretAccessKey == "" {
		t.Fatal("seed secret missing or equal to the avoided secret")
	}
	if st.IsOrgMemberAccount("000000000001", first.AccountID) || st.IsOrgMemberAccount("000000000001", second.AccountID) {
		t.Fatal("standalone account must not be an organization member")
	}
}

func twelveDigitAccountID(id string) bool {
	if len(id) != 12 {
		return false
	}
	for _, c := range id {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

func docsContain(docs []string, needle string) bool {
	for _, doc := range docs {
		if strings.Contains(doc, needle) {
			return true
		}
	}
	return false
}
