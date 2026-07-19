package store_test

import (
	"strings"
	"testing"

	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

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
