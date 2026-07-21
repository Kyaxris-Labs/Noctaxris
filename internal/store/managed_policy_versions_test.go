package store_test

import (
	"errors"
	"path/filepath"
	"testing"

	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

func TestCreatePolicyVersionSetAsDefault(t *testing.T) {
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

	v1Doc := `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Action":"sts:GetCallerIdentity","Resource":"*"}]}`
	arn, err := st.CreateManagedPolicy("000000000001", "Weak", v1Doc)
	if err != nil {
		t.Fatal(err)
	}
	versions, err := st.ListManagedPolicyVersions(arn)
	if err != nil || len(versions) != 1 || versions[0].VersionID != "v1" || !versions[0].IsDefaultVersion {
		t.Fatalf("initial versions=%+v err=%v", versions, err)
	}

	v2Doc := `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Action":"*","Resource":"*"}]}`
	v2, err := st.CreateManagedPolicyVersion(arn, v2Doc, true)
	if err != nil {
		t.Fatal(err)
	}
	if v2.VersionID != "v2" || !v2.IsDefaultVersion {
		t.Fatalf("v2=%+v", v2)
	}
	p, err := st.GetManagedPolicy(arn)
	if err != nil {
		t.Fatal(err)
	}
	if p.DefaultVersionID != "v2" || p.Document != v2Doc {
		t.Fatalf("default not updated: %+v", p)
	}
}

func TestCreatePolicyVersionLimitAndDeleteDefault(t *testing.T) {
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

	doc := `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Action":"sts:GetCallerIdentity","Resource":"*"}]}`
	arn, err := st.CreateManagedPolicy("000000000001", "Limited", doc)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 4; i++ {
		if _, err := st.CreateManagedPolicyVersion(arn, doc, false); err != nil {
			t.Fatalf("version %d: %v", i+2, err)
		}
	}
	if _, err := st.CreateManagedPolicyVersion(arn, doc, false); !errors.Is(err, store.ErrPolicyVersionLimit) {
		t.Fatalf("want ErrPolicyVersionLimit got %v", err)
	}
	if err := st.DeleteManagedPolicyVersion(arn, "v1"); !errors.Is(err, store.ErrDeleteDefaultPolicyVersion) {
		t.Fatalf("want ErrDeleteDefaultPolicyVersion got %v", err)
	}
	if err := st.DeleteManagedPolicyVersion(arn, "v2"); err != nil {
		t.Fatal(err)
	}
	if _, err := st.CreateManagedPolicyVersion(arn, doc, false); err != nil {
		t.Fatal(err)
	}
}

func TestSetDefaultPolicyVersion(t *testing.T) {
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

	v1 := `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Action":"iam:GetUser","Resource":"*"}]}`
	v2 := `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Action":"iam:ListUsers","Resource":"*"}]}`
	arn, err := st.CreateManagedPolicy("000000000001", "Flip", v1)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.CreateManagedPolicyVersion(arn, v2, false); err != nil {
		t.Fatal(err)
	}
	if err := st.SetDefaultManagedPolicyVersion(arn, "v2"); err != nil {
		t.Fatal(err)
	}
	p, err := st.GetManagedPolicy(arn)
	if err != nil {
		t.Fatal(err)
	}
	if p.DefaultVersionID != "v2" || p.Document != v2 {
		t.Fatalf("got %+v", p)
	}
	got, err := st.GetManagedPolicyVersion(arn, "v1")
	if err != nil || got.IsDefaultVersion {
		t.Fatalf("v1 still default: %+v err=%v", got, err)
	}
}
