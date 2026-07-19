package store_test

import (
	"path/filepath"
	"testing"

	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

func TestPutAttachListPolicies(t *testing.T) {
	dir := t.TempDir()
	key, err := store.LoadOrCreateMasterKey(filepath.Join(dir, "master.key"))
	if err != nil {
		t.Fatal(err)
	}
	st, err := store.Open(dir, key)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	allowDoc := `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Action":"sts:GetCallerIdentity","Resource":"*"}]}`
	denyDoc := `{"Version":"2012-10-17","Statement":[{"Effect":"Deny","Action":"sts:*","Resource":"*"}]}`

	if err := st.PutPolicy("allow-sts", allowDoc); err != nil {
		t.Fatal(err)
	}
	if err := st.PutPolicy("deny-sts", denyDoc); err != nil {
		t.Fatal(err)
	}

	arn := "arn:aws:iam::000000000001:user/alice"
	if err := st.AttachPolicy(arn, "allow-sts"); err != nil {
		t.Fatal(err)
	}
	if err := st.AttachPolicy(arn, "deny-sts"); err != nil {
		t.Fatal(err)
	}

	docs, err := st.ListAttachedPolicyDocuments(arn)
	if err != nil {
		t.Fatal(err)
	}
	if len(docs) != 2 {
		t.Fatalf("got %d docs, want 2", len(docs))
	}
	got := map[string]bool{}
	for _, d := range docs {
		got[d] = true
	}
	if !got[allowDoc] || !got[denyDoc] {
		t.Fatalf("unexpected documents: %#v", docs)
	}

	other, err := st.ListAttachedPolicyDocuments("arn:aws:iam::000000000001:user/bob")
	if err != nil {
		t.Fatal(err)
	}
	if len(other) != 0 {
		t.Fatalf("expected empty list, got %#v", other)
	}
}

func TestPutPolicyReplacesDocument(t *testing.T) {
	dir := t.TempDir()
	key, err := store.LoadOrCreateMasterKey(filepath.Join(dir, "master.key"))
	if err != nil {
		t.Fatal(err)
	}
	st, err := store.Open(dir, key)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	if err := st.PutPolicy("p1", `{"Version":"2012-10-17","Statement":[]}`); err != nil {
		t.Fatal(err)
	}
	updated := `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Action":"*","Resource":"*"}]}`
	if err := st.PutPolicy("p1", updated); err != nil {
		t.Fatal(err)
	}
	arn := "arn:aws:iam::000000000001:user/alice"
	if err := st.AttachPolicy(arn, "p1"); err != nil {
		t.Fatal(err)
	}
	docs, err := st.ListAttachedPolicyDocuments(arn)
	if err != nil {
		t.Fatal(err)
	}
	if len(docs) != 1 || docs[0] != updated {
		t.Fatalf("got %#v, want [%q]", docs, updated)
	}
}
