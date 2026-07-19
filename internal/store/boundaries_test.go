package store_test

import (
	"testing"
)

func TestUserPermissionsBoundary(t *testing.T) {
	st := openTestStore(t)
	const accountID = "000000000001"
	boundaryDoc := `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Action":"s3:*","Resource":"*"}]}`

	if _, _, err := st.CreateUser(accountID, "alice"); err != nil {
		t.Fatal(err)
	}
	policyARN, err := st.CreateManagedPolicy(accountID, "Boundary", boundaryDoc)
	if err != nil {
		t.Fatal(err)
	}

	if err := st.PutUserPermissionsBoundary(accountID, "alice", policyARN); err != nil {
		t.Fatal(err)
	}
	gotARN, err := st.GetUserPermissionsBoundary(accountID, "alice")
	if err != nil {
		t.Fatal(err)
	}
	if gotARN != policyARN {
		t.Fatalf("gotARN = %q, want %q", gotARN, policyARN)
	}

	doc, ok, err := st.PermissionsBoundaryDoc(accountID, "user", "alice")
	if err != nil {
		t.Fatal(err)
	}
	if !ok || doc != boundaryDoc {
		t.Fatalf("doc=%q ok=%v", doc, ok)
	}

	if err := st.DeleteUserPermissionsBoundary(accountID, "alice"); err != nil {
		t.Fatal(err)
	}
	_, err = st.GetUserPermissionsBoundary(accountID, "alice")
	if err == nil {
		t.Fatal("expected missing boundary error")
	}
	_, ok, err = st.PermissionsBoundaryDoc(accountID, "user", "alice")
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Fatal("expected ok=false after delete")
	}
}

func TestRolePermissionsBoundary(t *testing.T) {
	st := openTestStore(t)
	const accountID = "000000000001"
	boundaryDoc := `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Action":"ec2:*","Resource":"*"}]}`

	if _, err := st.CreateRole(accountID, "AppRole", `{"Version":"2012-10-17","Statement":[]}`); err != nil {
		t.Fatal(err)
	}
	policyARN, err := st.CreateManagedPolicy(accountID, "RoleBoundary", boundaryDoc)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.PutRolePermissionsBoundary(accountID, "AppRole", policyARN); err != nil {
		t.Fatal(err)
	}
	gotARN, err := st.GetRolePermissionsBoundary(accountID, "AppRole")
	if err != nil {
		t.Fatal(err)
	}
	if gotARN != policyARN {
		t.Fatalf("gotARN = %q", gotARN)
	}
	doc, ok, err := st.PermissionsBoundaryDoc(accountID, "role", "AppRole")
	if err != nil || !ok || doc != boundaryDoc {
		t.Fatalf("doc=%q ok=%v err=%v", doc, ok, err)
	}
	if err := st.DeleteRolePermissionsBoundary(accountID, "AppRole"); err != nil {
		t.Fatal(err)
	}
}
