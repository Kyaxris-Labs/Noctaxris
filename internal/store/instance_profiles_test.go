package store_test

import (
	"strings"
	"testing"

	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

func TestListInstanceProfilesForRole(t *testing.T) {
	st := openTestStore(t)
	accountID := "000000000001"
	if _, err := st.CreateRole(accountID, "RoleA", `{"Version":"2012-10-17","Statement":[]}`); err != nil {
		t.Fatal(err)
	}
	listed, err := st.ListInstanceProfilesForRole(accountID, "RoleA")
	if err != nil {
		t.Fatal(err)
	}
	if len(listed) != 0 {
		t.Fatalf("want empty, got %#v", listed)
	}
	if _, err := st.CreateInstanceProfile(accountID, "MyProfile"); err != nil {
		t.Fatal(err)
	}
	if err := st.AddRoleToInstanceProfile(accountID, "MyProfile", "RoleA"); err != nil {
		t.Fatal(err)
	}
	listed, err = st.ListInstanceProfilesForRole(accountID, "RoleA")
	if err != nil {
		t.Fatal(err)
	}
	if len(listed) != 1 || listed[0].ProfileName != "MyProfile" {
		t.Fatalf("listed=%#v", listed)
	}
}

func TestInstanceProfileOneRole(t *testing.T) {
	st := openTestStore(t)
	const accountID = "000000000001"
	trust := `{"Version":"2012-10-17","Statement":[]}`

	if _, err := st.CreateRole(accountID, "RoleA", trust); err != nil {
		t.Fatal(err)
	}
	if _, err := st.CreateRole(accountID, "RoleB", trust); err != nil {
		t.Fatal(err)
	}

	arn, err := st.CreateInstanceProfile(accountID, "MyProfile")
	if err != nil {
		t.Fatal(err)
	}
	wantARN := store.InstanceProfileARN(accountID, "/", "MyProfile")
	if arn != wantARN {
		t.Fatalf("arn = %q, want %q", arn, wantARN)
	}

	if err := st.AddRoleToInstanceProfile(accountID, "MyProfile", "RoleA"); err != nil {
		t.Fatal(err)
	}
	prof, err := st.GetInstanceProfile(accountID, "MyProfile")
	if err != nil {
		t.Fatal(err)
	}
	if prof.ProfileName != "MyProfile" || prof.RoleName != "RoleA" || prof.ProfileARN != wantARN {
		t.Fatalf("prof=%+v", prof)
	}

	err = st.AddRoleToInstanceProfile(accountID, "MyProfile", "RoleB")
	if err == nil {
		t.Fatal("expected error when adding second role")
	}
	if !strings.Contains(err.Error(), "one role") && !strings.Contains(err.Error(), "already") {
		t.Fatalf("unexpected error: %v", err)
	}

	prof, err = st.GetInstanceProfile(accountID, "MyProfile")
	if err != nil {
		t.Fatal(err)
	}
	if prof.RoleName != "RoleA" {
		t.Fatalf("role changed to %q", prof.RoleName)
	}
}

func TestInstanceProfileListRemoveDeleteAndValidation(t *testing.T) {
	st := openTestStore(t)
	const accountID = "000000000001"
	trust := `{"Version":"2012-10-17","Statement":[]}`

	if _, err := st.CreateInstanceProfile("bad", "P"); err == nil {
		t.Fatal("invalid account id must fail")
	}
	if _, err := st.CreateInstanceProfile(accountID, ""); err == nil {
		t.Fatal("empty profile name must fail")
	}
	if _, err := st.CreateRole(accountID, "RoleX", trust); err != nil {
		t.Fatal(err)
	}
	if _, err := st.CreateInstanceProfile(accountID, "ProfA"); err != nil {
		t.Fatal(err)
	}
	if _, err := st.CreateInstanceProfile(accountID, "ProfB"); err != nil {
		t.Fatal(err)
	}
	if err := st.AddRoleToInstanceProfile(accountID, "ProfA", "RoleX"); err != nil {
		t.Fatal(err)
	}
	listed, err := st.ListInstanceProfiles(accountID)
	if err != nil || len(listed) != 2 {
		t.Fatalf("list=%v err=%v", listed, err)
	}
	if listed[0].ProfileName != "ProfA" || listed[0].RoleName != "RoleX" {
		t.Fatalf("listed[0]=%+v", listed[0])
	}
	if err := st.RemoveRoleFromInstanceProfile(accountID, "ProfA", "RoleX"); err != nil {
		t.Fatal(err)
	}
	if err := st.RemoveRoleFromInstanceProfile(accountID, "ProfA", "RoleX"); err == nil {
		t.Fatal("second remove must fail")
	}
	prof, err := st.GetInstanceProfile(accountID, "ProfA")
	if err != nil || prof.RoleName != "" {
		t.Fatalf("after remove prof=%+v err=%v", prof, err)
	}
	if err := st.DeleteInstanceProfile(accountID, "ProfA"); err != nil {
		t.Fatal(err)
	}
	if err := st.DeleteInstanceProfile(accountID, "ProfA"); err == nil {
		t.Fatal("delete missing must fail")
	}
	listed, err = st.ListInstanceProfiles(accountID)
	if err != nil || len(listed) != 1 || listed[0].ProfileName != "ProfB" {
		t.Fatalf("after delete list=%v err=%v", listed, err)
	}
}
