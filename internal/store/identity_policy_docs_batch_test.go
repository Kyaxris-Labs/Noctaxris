package store_test

import (
	"fmt"
	"testing"

	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

func docAllow(action string) string {
	return fmt.Sprintf(
		`{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Action":%q,"Resource":"*"}]}`,
		action,
	)
}

func assertDocSetEqual(t *testing.T, got, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("len(got)=%d len(want)=%d\ngot=%#v\nwant=%#v", len(got), len(want), got, want)
	}
	wantCounts := make(map[string]int, len(want))
	for _, d := range want {
		wantCounts[d]++
	}
	gotCounts := make(map[string]int, len(got))
	for _, d := range got {
		gotCounts[d]++
	}
	for d, n := range wantCounts {
		if gotCounts[d] != n {
			t.Fatalf("doc %q: got count %d want %d\ngot=%#v\nwant=%#v", d, gotCounts[d], n, got, want)
		}
	}
}

func assertBatchEqualsSequential(t *testing.T, st *store.Store, accountID, userName string) {
	t.Helper()
	seq, err := st.IdentityPolicyDocsForUserSequentialForTest(accountID, userName)
	if err != nil {
		t.Fatalf("sequential: %v", err)
	}
	batched, err := st.IdentityPolicyDocsForUser(accountID, userName)
	if err != nil {
		t.Fatalf("batched: %v", err)
	}
	assertDocSetEqual(t, batched, seq)
}

func TestIdentityPolicyDocsBatchZeroGroups(t *testing.T) {
	st := openTestStore(t)
	const accountID = "000000000001"
	if _, _, err := st.CreateUser(accountID, "solo"); err != nil {
		t.Fatal(err)
	}
	userDoc := docAllow("iam:GetUser")
	userPolicyARN, err := st.CreateManagedPolicy(accountID, "SoloUser", userDoc)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.AttachUserPolicy(accountID, "solo", userPolicyARN); err != nil {
		t.Fatal(err)
	}
	u, err := st.GetUser(accountID, "solo")
	if err != nil {
		t.Fatal(err)
	}
	userInline := docAllow("ec2:Describe*")
	if err := st.PutInlinePolicy(u.ARN, "soloInline", userInline); err != nil {
		t.Fatal(err)
	}
	assertBatchEqualsSequential(t, st, accountID, "solo")
}

func TestIdentityPolicyDocsBatchOneGroup(t *testing.T) {
	st := openTestStore(t)
	const accountID = "000000000001"
	if _, _, err := st.CreateUser(accountID, "alice"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := st.CreateGroup(accountID, "Admins"); err != nil {
		t.Fatal(err)
	}
	if err := st.AddUserToGroup(accountID, "Admins", "alice"); err != nil {
		t.Fatal(err)
	}

	userDoc := docAllow("iam:GetUser")
	userPolicyARN, err := st.CreateManagedPolicy(accountID, "UserIAM", userDoc)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.AttachUserPolicy(accountID, "alice", userPolicyARN); err != nil {
		t.Fatal(err)
	}
	u, err := st.GetUser(accountID, "alice")
	if err != nil {
		t.Fatal(err)
	}
	userInline := docAllow("ec2:Describe*")
	if err := st.PutInlinePolicy(u.ARN, "userInline", userInline); err != nil {
		t.Fatal(err)
	}

	groupDoc := docAllow("s3:ListBucket")
	groupPolicyARN, err := st.CreateManagedPolicy(accountID, "GroupS3", groupDoc)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.AttachGroupPolicy(accountID, "Admins", groupPolicyARN); err != nil {
		t.Fatal(err)
	}
	groupInline := docAllow("sts:GetCallerIdentity")
	if err := st.PutGroupInlinePolicy(accountID, "Admins", "gInline", groupInline); err != nil {
		t.Fatal(err)
	}

	assertBatchEqualsSequential(t, st, accountID, "alice")
}

func TestIdentityPolicyDocsBatchNGroups(t *testing.T) {
	st := openTestStore(t)
	const accountID = "000000000001"
	if _, _, err := st.CreateUser(accountID, "bob"); err != nil {
		t.Fatal(err)
	}
	for i, name := range []string{"G0", "G1", "G2"} {
		if _, _, err := st.CreateGroup(accountID, name); err != nil {
			t.Fatal(err)
		}
		if err := st.AddUserToGroup(accountID, name, "bob"); err != nil {
			t.Fatal(err)
		}
		doc := docAllow(fmt.Sprintf("s3:GetObject%d", i))
		arn, err := st.CreateManagedPolicy(accountID, fmt.Sprintf("Pol%d", i), doc)
		if err != nil {
			t.Fatal(err)
		}
		if err := st.AttachGroupPolicy(accountID, name, arn); err != nil {
			t.Fatal(err)
		}
		inline := docAllow(fmt.Sprintf("sns:Publish%d", i))
		if err := st.PutGroupInlinePolicy(accountID, name, "inline", inline); err != nil {
			t.Fatal(err)
		}
	}
	assertBatchEqualsSequential(t, st, accountID, "bob")
}

func TestIdentityPolicyDocsBatchOnlyAttached(t *testing.T) {
	st := openTestStore(t)
	const accountID = "000000000001"
	if _, _, err := st.CreateUser(accountID, "att"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := st.CreateGroup(accountID, "AttachedOnly"); err != nil {
		t.Fatal(err)
	}
	if err := st.AddUserToGroup(accountID, "AttachedOnly", "att"); err != nil {
		t.Fatal(err)
	}
	userDoc := docAllow("iam:ListUsers")
	userARN, err := st.CreateManagedPolicy(accountID, "AttUser", userDoc)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.AttachUserPolicy(accountID, "att", userARN); err != nil {
		t.Fatal(err)
	}
	groupDoc := docAllow("s3:GetObject")
	groupARN, err := st.CreateManagedPolicy(accountID, "AttGroup", groupDoc)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.AttachGroupPolicy(accountID, "AttachedOnly", groupARN); err != nil {
		t.Fatal(err)
	}
	assertBatchEqualsSequential(t, st, accountID, "att")
}

func TestIdentityPolicyDocsBatchOnlyInline(t *testing.T) {
	st := openTestStore(t)
	const accountID = "000000000001"
	if _, _, err := st.CreateUser(accountID, "inl"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := st.CreateGroup(accountID, "InlineOnly"); err != nil {
		t.Fatal(err)
	}
	if err := st.AddUserToGroup(accountID, "InlineOnly", "inl"); err != nil {
		t.Fatal(err)
	}
	u, err := st.GetUser(accountID, "inl")
	if err != nil {
		t.Fatal(err)
	}
	if err := st.PutInlinePolicy(u.ARN, "uInline", docAllow("ec2:DescribeInstances")); err != nil {
		t.Fatal(err)
	}
	if err := st.PutGroupInlinePolicy(accountID, "InlineOnly", "gInline", docAllow("sqs:ReceiveMessage")); err != nil {
		t.Fatal(err)
	}
	assertBatchEqualsSequential(t, st, accountID, "inl")
}

func TestIdentityPolicyDocsBatchEmptyGroup(t *testing.T) {
	st := openTestStore(t)
	const accountID = "000000000001"
	if _, _, err := st.CreateUser(accountID, "emptyg"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := st.CreateGroup(accountID, "Empty"); err != nil {
		t.Fatal(err)
	}
	if err := st.AddUserToGroup(accountID, "Empty", "emptyg"); err != nil {
		t.Fatal(err)
	}
	userDoc := docAllow("iam:GetUser")
	arn, err := st.CreateManagedPolicy(accountID, "EmptyUserPol", userDoc)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.AttachUserPolicy(accountID, "emptyg", arn); err != nil {
		t.Fatal(err)
	}
	assertBatchEqualsSequential(t, st, accountID, "emptyg")
}

func TestIdentityPolicyDocsBatchOmitMissingManaged(t *testing.T) {
	st := openTestStore(t)
	const accountID = "000000000001"
	if _, _, err := st.CreateUser(accountID, "omit"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := st.CreateGroup(accountID, "OmitG"); err != nil {
		t.Fatal(err)
	}
	if err := st.AddUserToGroup(accountID, "OmitG", "omit"); err != nil {
		t.Fatal(err)
	}

	present := docAllow("s3:ListBucket")
	presentARN, err := st.CreateManagedPolicy(accountID, "Present", present)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.AttachUserPolicy(accountID, "omit", presentARN); err != nil {
		t.Fatal(err)
	}
	if err := st.AttachGroupPolicy(accountID, "OmitG", presentARN); err != nil {
		t.Fatal(err)
	}

	u, err := st.GetUser(accountID, "omit")
	if err != nil {
		t.Fatal(err)
	}
	g, err := st.GetGroup(accountID, "OmitG")
	if err != nil {
		t.Fatal(err)
	}
	// Attachment without a synced policies row must be omitted (INNER JOIN contract).
	missingID := "arn:aws:iam::000000000001:policy/MissingSynced"
	if err := st.AttachPolicy(u.ARN, missingID); err != nil {
		t.Fatal(err)
	}
	if err := st.AttachPolicy(g.ARN, missingID); err != nil {
		t.Fatal(err)
	}

	assertBatchEqualsSequential(t, st, accountID, "omit")
	docs, err := st.IdentityPolicyDocsForUserSequentialForTest(accountID, "omit")
	if err != nil {
		t.Fatal(err)
	}
	if len(docs) != 2 {
		t.Fatalf("expected only present docs (user+group attach), got %#v", docs)
	}
	for _, d := range docs {
		if d != present {
			t.Fatalf("unexpected doc %q", d)
		}
	}
}
