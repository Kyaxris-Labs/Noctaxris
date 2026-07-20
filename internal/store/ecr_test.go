package store_test

import (
	"errors"
	"path/filepath"
	"testing"

	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

func openECRStore(t *testing.T) *store.Store {
	t.Helper()
	dir := t.TempDir()
	key, err := store.LoadOrCreateMasterKey(filepath.Join(dir, "master.key"))
	if err != nil {
		t.Fatal(err)
	}
	st, err := store.Open(dir, key)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	return st
}

func TestECRCreateRepository(t *testing.T) {
	st := openECRStore(t)
	account := "000000000001"

	created, err := st.CreateRepository(account, "us-east-1", "noctaxris-lab")
	if err != nil {
		t.Fatal(err)
	}
	wantARN := "arn:aws:ecr:us-east-1:000000000001:repository/noctaxris-lab"
	wantURI := "127.0.0.1:4566/000000000001/noctaxris-lab"
	if created.Name != "noctaxris-lab" || created.ARN != wantARN || created.URI != wantURI {
		t.Fatalf("created=%+v want arn=%q uri=%q", created, wantARN, wantURI)
	}
	if created.CreatedAt == "" {
		t.Fatal("created_at missing")
	}

	repos, err := st.DescribeRepositories(account, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(repos) != 1 || repos[0].Name != "noctaxris-lab" {
		t.Fatalf("describe=%+v", repos)
	}
}

func TestECRCreateRepositoryAlreadyExists(t *testing.T) {
	st := openECRStore(t)
	account := "000000000001"

	if _, err := st.CreateRepository(account, "us-east-1", "dup"); err != nil {
		t.Fatal(err)
	}
	if _, err := st.CreateRepository(account, "us-east-1", "dup"); !errors.Is(err, store.ErrRepositoryAlreadyExists) {
		t.Fatalf("want ErrRepositoryAlreadyExists, got %v", err)
	}
}

func TestECRRepositoryPolicyLifecycle(t *testing.T) {
	st := openECRStore(t)
	account := "000000000001"

	if _, err := st.CreateRepository(account, "us-east-1", "policy-repo"); err != nil {
		t.Fatal(err)
	}

	if _, err := st.GetRepositoryPolicy(account, "policy-repo"); !errors.Is(err, store.ErrNoSuchResourcePolicy) {
		t.Fatalf("want ErrNoSuchResourcePolicy, got %v", err)
	}

	policy := `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":"*","Action":"ecr:BatchGetImage","Resource":"*"}]}`
	if err := st.SetRepositoryPolicy(account, "policy-repo", policy); err != nil {
		t.Fatal(err)
	}
	got, err := st.GetRepositoryPolicy(account, "policy-repo")
	if err != nil {
		t.Fatal(err)
	}
	if got != policy {
		t.Fatalf("policy=%q", got)
	}

	if err := st.DeleteRepositoryPolicy(account, "policy-repo"); err != nil {
		t.Fatal(err)
	}
	if _, err := st.GetRepositoryPolicy(account, "policy-repo"); !errors.Is(err, store.ErrNoSuchResourcePolicy) {
		t.Fatalf("want ErrNoSuchResourcePolicy after delete, got %v", err)
	}
}

func TestECRPutImageListImages(t *testing.T) {
	st := openECRStore(t)
	account := "000000000001"

	if _, err := st.CreateRepository(account, "us-east-1", "img-repo"); err != nil {
		t.Fatal(err)
	}

	img, err := st.PutImage(account, "img-repo", "sha256:abc123", []string{"latest", "lab"}, "/ecr/manifest.json")
	if err != nil {
		t.Fatal(err)
	}
	if img.ImageDigest != "sha256:abc123" || len(img.ImageTags) != 2 {
		t.Fatalf("put=%+v", img)
	}

	images, err := st.ListImages(account, "img-repo")
	if err != nil {
		t.Fatal(err)
	}
	if len(images) != 1 || images[0].ImageDigest != "sha256:abc123" {
		t.Fatalf("list=%+v", images)
	}

	got, err := st.BatchGetImage(account, "img-repo", nil, []string{"lab"})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].ImageDigest != "sha256:abc123" {
		t.Fatalf("batch=%+v", got)
	}
}

func TestECRAuthorizationTokenRoundTrip(t *testing.T) {
	st := openECRStore(t)
	account := "000000000001"

	token, expiresAt, err := st.IssueAuthorizationToken(account, "arn:aws:iam::000000000001:root", 0)
	if err != nil {
		t.Fatal(err)
	}
	if token == "" || expiresAt.IsZero() {
		t.Fatalf("token=%q expires=%v", token, expiresAt)
	}

	gotAccount, gotPrincipal, _, err := st.ValidateAuthorizationToken(token)
	if err != nil {
		t.Fatal(err)
	}
	if gotAccount != account || gotPrincipal != "arn:aws:iam::000000000001:root" {
		t.Fatalf("account=%q principal=%q", gotAccount, gotPrincipal)
	}
}
