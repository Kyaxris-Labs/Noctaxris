package store_test

import (
	"strings"
	"testing"

	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

func TestGroupsMembershipAndPolicies(t *testing.T) {
	st := openTestStore(t)
	const accountID = "000000000001"
	doc := `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Action":"s3:ListBucket","Resource":"*"}]}`
	inlineDoc := `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Action":"sts:GetCallerIdentity","Resource":"*"}]}`

	if _, _, err := st.CreateUser(accountID, "alice"); err != nil {
		t.Fatal(err)
	}
	groupID, groupARN, err := st.CreateGroup(accountID, "Admins")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(groupID, "AGPA") {
		t.Fatalf("groupID = %q", groupID)
	}
	wantARN := store.GroupARN(accountID, "/", "Admins")
	if groupARN != wantARN {
		t.Fatalf("groupARN = %q, want %q", groupARN, wantARN)
	}

	if err := st.AddUserToGroup(accountID, "Admins", "alice"); err != nil {
		t.Fatal(err)
	}
	groups, err := st.ListGroupsForUser(accountID, "alice")
	if err != nil {
		t.Fatal(err)
	}
	if len(groups) != 1 || groups[0].GroupName != "Admins" {
		t.Fatalf("groups = %#v", groups)
	}

	policyARN, err := st.CreateManagedPolicy(accountID, "GroupS3", doc)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.AttachGroupPolicy(accountID, "Admins", policyARN); err != nil {
		t.Fatal(err)
	}
	attached, err := st.ListGroupPolicies(accountID, "Admins")
	if err != nil || len(attached) != 1 || attached[0] != policyARN {
		t.Fatalf("attached=%#v err=%v", attached, err)
	}

	if err := st.PutGroupInlinePolicy(accountID, "Admins", "inline1", inlineDoc); err != nil {
		t.Fatal(err)
	}

	userDoc := `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Action":"iam:GetUser","Resource":"*"}]}`
	userPolicyARN, err := st.CreateManagedPolicy(accountID, "UserIAM", userDoc)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.AttachUserPolicy(accountID, "alice", userPolicyARN); err != nil {
		t.Fatal(err)
	}
	userInline := `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Action":"ec2:Describe*","Resource":"*"}]}`
	u, err := st.GetUser(accountID, "alice")
	if err != nil {
		t.Fatal(err)
	}
	if err := st.PutInlinePolicy(u.ARN, "userInline", userInline); err != nil {
		t.Fatal(err)
	}

	docs, err := st.IdentityPolicyDocsForUser(accountID, "alice")
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]bool{userDoc: true, userInline: true, doc: true, inlineDoc: true}
	if len(docs) != 4 {
		t.Fatalf("docs=%#v want 4", docs)
	}
	for _, d := range docs {
		if !want[d] {
			t.Fatalf("unexpected doc %q in %#v", d, docs)
		}
	}
}

func TestListGroupsForUserEmpty(t *testing.T) {
	st := openTestStore(t)
	const accountID = "000000000001"
	if _, _, err := st.CreateUser(accountID, "solo"); err != nil {
		t.Fatal(err)
	}
	groups, err := st.ListGroupsForUser(accountID, "solo")
	if err != nil {
		t.Fatal(err)
	}
	if len(groups) != 0 {
		t.Fatalf("groups = %#v", groups)
	}
}
