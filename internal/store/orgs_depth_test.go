package store_test

import (
	"testing"
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
