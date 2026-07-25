package store_test

import (
	"testing"

	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

func TestListAccountsAndManagement(t *testing.T) {
	st := openTestStore(t)
	const mgmt = "000000000001"
	if err := st.EnsureRoot(mgmt, "AKIAROOTEXAMPLE01", "secret"); err != nil {
		t.Fatal(err)
	}
	_, memberID, err := st.CreateMemberAccount(mgmt, "member@example.com", "Member")
	if err != nil {
		t.Fatal(err)
	}

	accounts, err := st.ListAccounts()
	if err != nil {
		t.Fatal(err)
	}
	if len(accounts) != 2 {
		t.Fatalf("accounts=%#v", accounts)
	}
	ids := map[string]bool{}
	for _, a := range accounts {
		ids[a.AccountID] = true
	}
	if !ids[mgmt] || !ids[memberID] {
		t.Fatalf("ids=%v", ids)
	}

	if !st.IsManagementAccount(mgmt) {
		t.Fatal("mgmt should be management account")
	}
	if st.IsManagementAccount(memberID) {
		t.Fatal("member must not be management account")
	}
}

func TestListOrgPoliciesParentsAndAccountsForParent(t *testing.T) {
	st := openTestStore(t)
	const mgmt = "000000000001"
	if err := st.EnsureRoot(mgmt, "AKIAROOTEXAMPLE01", "secret"); err != nil {
		t.Fatal(err)
	}
	_, memberID, err := st.CreateMemberAccount(mgmt, "list@example.com", "List")
	if err != nil {
		t.Fatal(err)
	}
	ouID, err := st.CreateOrganizationalUnit(store.OrgRootID, "Team")
	if err != nil {
		t.Fatal(err)
	}
	if err := st.MoveAccount(memberID, store.OrgRootID, ouID); err != nil {
		t.Fatal(err)
	}
	scpID, err := st.CreateOrgPolicy("SCP", "DenyS3", `{"Version":"2012-10-17","Statement":[{"Effect":"Deny","Action":"s3:*","Resource":"*"}]}`)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.AttachOrgPolicy(scpID, "ou", ouID); err != nil {
		t.Fatal(err)
	}

	all, err := st.ListOrgPolicies("")
	if err != nil || len(all) != 1 {
		t.Fatalf("ListOrgPolicies all=%#v err=%v", all, err)
	}
	byType, err := st.ListOrgPolicies("SCP")
	if err != nil || len(byType) != 1 || byType[0].ID != scpID {
		t.Fatalf("ListOrgPolicies SCP=%#v err=%v", byType, err)
	}

	underOU, err := st.ListAccountsForParent(ouID)
	if err != nil || len(underOU) != 1 || underOU[0].AccountID != memberID {
		t.Fatalf("ListAccountsForParent ou=%#v err=%v", underOU, err)
	}

	parents, err := st.ListParents(memberID)
	if err != nil || len(parents) != 2 || parents[0].Type != "ORGANIZATIONAL_UNIT" || parents[1].Type != "ROOT" {
		t.Fatalf("ListParents=%#v err=%v", parents, err)
	}
}

func TestOrgOUsPoliciesAndSCPRCPDocs(t *testing.T) {
	st := openTestStore(t)
	const mgmt = "000000000001"
	if err := st.EnsureRoot(mgmt, "AKIAROOTEXAMPLE01", "secret"); err != nil {
		t.Fatal(err)
	}
	_, memberID, err := st.CreateMemberAccount(mgmt, "m@example.com", "M")
	if err != nil {
		t.Fatal(err)
	}

	ouID, err := st.CreateOrganizationalUnit("r-root", "Workloads")
	if err != nil {
		t.Fatal(err)
	}
	if ouID == "" {
		t.Fatal("empty ou id")
	}

	scpDoc := `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Action":"*","Resource":"*"}]}`
	rcpDoc := `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Action":"s3:*","Resource":"*"}]}`
	scpID, err := st.CreateOrgPolicy("SCP", "FullAWS", scpDoc)
	if err != nil {
		t.Fatal(err)
	}
	rcpID, err := st.CreateOrgPolicy("RCP", "S3Only", rcpDoc)
	if err != nil {
		t.Fatal(err)
	}

	if err := st.AttachOrgPolicy(scpID, "root", "r-root"); err != nil {
		t.Fatal(err)
	}
	if err := st.AttachOrgPolicy(scpID, "account", memberID); err != nil {
		t.Fatal(err)
	}
	if err := st.AttachOrgPolicy(rcpID, "account", memberID); err != nil {
		t.Fatal(err)
	}
	if err := st.AttachOrgPolicy(scpID, "ou", ouID); err != nil {
		t.Fatal(err)
	}

	listed, err := st.ListOrganizationalUnitsForParent("r-root")
	if err != nil || len(listed) != 1 || listed[0].ID != ouID {
		t.Fatalf("list OUs=%#v err=%v", listed, err)
	}

	if err := st.EnableOrgPolicyType("r-root", "SCP"); err != nil {
		t.Fatal(err)
	}
	if !st.IsOrgPolicyTypeEnabled("r-root", "SCP") {
		t.Fatal("SCP should be enabled")
	}
	got, err := st.GetOrgPolicy(scpID)
	if err != nil || got.Name != "FullAWS" {
		t.Fatalf("get policy=%#v err=%v", got, err)
	}
	if err := st.DetachOrgPolicy(scpID, "account", memberID); err != nil {
		t.Fatal(err)
	}

	rootPolicies, err := st.ListPoliciesForTarget("root", "r-root")
	if err != nil || len(rootPolicies) != 1 || rootPolicies[0].ID != scpID {
		t.Fatalf("rootPolicies=%#v err=%v", rootPolicies, err)
	}

	scpDocs, err := st.SCPDocsForAccount(memberID)
	if err != nil {
		t.Fatal(err)
	}
	// root + account attachments (deduped by policy id / same doc once or twice is ok if same policy)
	if len(scpDocs) < 1 {
		t.Fatalf("scpDocs=%#v", scpDocs)
	}
	found := false
	for _, d := range scpDocs {
		if d == scpDoc {
			found = true
		}
	}
	if !found {
		t.Fatalf("missing scp doc in %#v", scpDocs)
	}

	rcpDocs, err := st.RCPDocsForAccount(memberID)
	if err != nil {
		t.Fatal(err)
	}
	if len(rcpDocs) != 1 || rcpDocs[0] != rcpDoc {
		t.Fatalf("rcpDocs=%#v", rcpDocs)
	}

	// Management account still gets docs from store helpers; authz skips applying them.
	mgmtSCPs, err := st.SCPDocsForAccount(mgmt)
	if err != nil {
		t.Fatal(err)
	}
	_ = mgmtSCPs
}

func TestAccountPlacementAndOUInheritedSCP(t *testing.T) {
	st := openTestStore(t)
	const mgmt = "000000000001"
	if err := st.EnsureRoot(mgmt, "AKIAROOTEXAMPLE01", "secret"); err != nil {
		t.Fatal(err)
	}
	_, memberID, err := st.CreateMemberAccount(mgmt, "place@example.com", "Place")
	if err != nil {
		t.Fatal(err)
	}

	parent, err := st.AccountParentID(memberID)
	if err != nil || parent != store.OrgRootID {
		t.Fatalf("default parent=%q err=%v want %s", parent, err, store.OrgRootID)
	}
	path, err := st.OUPathToRoot(memberID)
	if err != nil || len(path) != 0 {
		t.Fatalf("default OU path=%#v err=%v", path, err)
	}

	parentOU, err := st.CreateOrganizationalUnit(store.OrgRootID, "Workloads")
	if err != nil {
		t.Fatal(err)
	}
	childOU, err := st.CreateOrganizationalUnit(parentOU, "Prod")
	if err != nil {
		t.Fatal(err)
	}

	restrictDoc := `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Action":"iam:*","Resource":"*"}]}`
	parentDoc := `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Action":["iam:*","s3:*"],"Resource":"*"}]}`
	restrictID, err := st.CreateOrgPolicy("SCP", "NoS3OnChild", restrictDoc)
	if err != nil {
		t.Fatal(err)
	}
	parentID, err := st.CreateOrgPolicy("SCP", "ParentAllow", parentDoc)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.AttachOrgPolicy(restrictID, "ou", childOU); err != nil {
		t.Fatal(err)
	}
	if err := st.AttachOrgPolicy(parentID, "ou", parentOU); err != nil {
		t.Fatal(err)
	}

	// Before move: OU-only attachments must not apply.
	before, err := st.SCPDocsForAccount(memberID)
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range before {
		if d == restrictDoc || d == parentDoc {
			t.Fatalf("OU SCP leaked before MoveAccount: %#v", before)
		}
	}

	if err := st.MoveAccount(memberID, store.OrgRootID, childOU); err != nil {
		t.Fatal(err)
	}
	if got, err := st.AccountParentID(memberID); err != nil || got != childOU {
		t.Fatalf("after move parent=%q err=%v", got, err)
	}
	path, err = st.OUPathToRoot(memberID)
	if err != nil || len(path) != 2 || path[0] != childOU || path[1] != parentOU {
		t.Fatalf("OU path=%#v err=%v", path, err)
	}

	after, err := st.SCPDocsForAccount(memberID)
	if err != nil {
		t.Fatal(err)
	}
	foundRestrict, foundParent := false, false
	for _, d := range after {
		if d == restrictDoc {
			foundRestrict = true
		}
		if d == parentDoc {
			foundParent = true
		}
	}
	if !foundRestrict || !foundParent {
		t.Fatalf("inherited SCPs missing restrict=%v parent=%v docs=%#v", foundRestrict, foundParent, after)
	}

	if err := st.MoveAccount(memberID, store.OrgRootID, parentOU); err == nil {
		t.Fatal("expected source parent mismatch")
	}

	// Management account placement under same OU must not change IsManagementAccount.
	if err := st.SetAccountParent(mgmt, childOU); err != nil {
		t.Fatal(err)
	}
	if !st.IsManagementAccount(mgmt) {
		t.Fatal("management account must remain management")
	}
	mgmtDocs, err := st.SCPDocsForAccount(mgmt)
	if err != nil {
		t.Fatal(err)
	}
	_ = mgmtDocs
}
