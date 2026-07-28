package store_test

import (
	"strings"
	"testing"

	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

func TestOrgFilterDocsForAccountSharesOneOUPath(t *testing.T) {
	st := openTestStore(t)
	const mgmt = "000000000001"
	if err := st.EnsureRoot(mgmt, "AKIAROOTEXAMPLE01", "secret"); err != nil {
		t.Fatal(err)
	}
	_, memberID, err := st.CreateMemberAccount(mgmt, "share@example.com", "Share")
	if err != nil {
		t.Fatal(err)
	}
	ouID, err := st.CreateOrganizationalUnit(store.OrgRootID, "Shared")
	if err != nil {
		t.Fatal(err)
	}
	if err := st.MoveAccount(memberID, store.OrgRootID, ouID); err != nil {
		t.Fatal(err)
	}
	scpID, err := st.CreateOrgPolicy("SCP", "Full", `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Action":"*","Resource":"*"}]}`)
	if err != nil {
		t.Fatal(err)
	}
	rcpID, err := st.CreateOrgPolicy("RCP", "FullRCP", `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Action":"*","Resource":"*"}]}`)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.AttachOrgPolicy(scpID, "ou", ouID); err != nil {
		t.Fatal(err)
	}
	if err := st.AttachOrgPolicy(rcpID, "ou", ouID); err != nil {
		t.Fatal(err)
	}

	scp, rcp, err := st.OrgFilterDocsForAccount(memberID)
	if err != nil {
		t.Fatal(err)
	}
	if len(scp) != 1 || len(rcp) != 1 {
		t.Fatalf("scp=%d rcp=%d", len(scp), len(rcp))
	}

	// Equivalence with single-type helpers (same path membership).
	wantSCP, err := st.SCPDocsForAccount(memberID)
	if err != nil {
		t.Fatal(err)
	}
	wantRCP, err := st.RCPDocsForAccount(memberID)
	if err != nil {
		t.Fatal(err)
	}
	if len(scp) != len(wantSCP) || scp[0] != wantSCP[0] {
		t.Fatalf("scp mismatch shared=%#v single=%#v", scp, wantSCP)
	}
	if len(rcp) != len(wantRCP) || rcp[0] != wantRCP[0] {
		t.Fatalf("rcp mismatch shared=%#v single=%#v", rcp, wantRCP)
	}
}

func TestOrgFilterDocsForAccountOUPathError(t *testing.T) {
	st := openTestStore(t)
	const mgmt = "000000000001"
	if err := st.EnsureRoot(mgmt, "AKIAROOTEXAMPLE01", "secret"); err != nil {
		t.Fatal(err)
	}
	_, memberID, err := st.CreateMemberAccount(mgmt, "cycle@example.com", "Cycle")
	if err != nil {
		t.Fatal(err)
	}
	ouA, err := st.CreateOrganizationalUnit(store.OrgRootID, "A")
	if err != nil {
		t.Fatal(err)
	}
	ouB, err := st.CreateOrganizationalUnit(ouA, "B")
	if err != nil {
		t.Fatal(err)
	}
	if err := st.MoveAccount(memberID, store.OrgRootID, ouB); err != nil {
		t.Fatal(err)
	}
	if err := st.UnsafeSetOUParentForTest(ouA, ouB); err != nil {
		t.Fatal(err)
	}
	_, _, err = st.OrgFilterDocsForAccount(memberID)
	if err == nil || !strings.Contains(err.Error(), "cycle") {
		t.Fatalf("want cycle error, got %v", err)
	}
}
