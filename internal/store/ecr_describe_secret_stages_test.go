package store_test

import (
	"errors"
	"testing"

	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

func TestDescribeRepositoriesListAndFilter(t *testing.T) {
	st := openTestStore(t)
	const account = "000000000001"

	if _, err := st.CreateRepository(account, "us-east-1", "alpha-repo"); err != nil {
		t.Fatal(err)
	}
	if _, err := st.CreateRepository(account, "us-east-1", "beta-repo"); err != nil {
		t.Fatal(err)
	}

	all, err := st.DescribeRepositories(account, nil)
	if err != nil || len(all) < 2 {
		t.Fatalf("list all=%d err=%v", len(all), err)
	}
	filtered, err := st.DescribeRepositories(account, []string{"alpha-repo", "missing-repo"})
	if err != nil || len(filtered) != 1 || filtered[0].Name != "alpha-repo" {
		t.Fatalf("filtered=%+v err=%v", filtered, err)
	}
	empty, err := st.DescribeRepositories(account, []string{"nope-a", "nope-b"})
	if err != nil || empty == nil || len(empty) != 0 {
		t.Fatalf("empty filter=%v err=%v", empty, err)
	}
}

func TestPutSecretValueWithStagesPendingAndCurrent(t *testing.T) {
	st := openTestStore(t)
	const account = "000000000001"

	created, err := st.CreateSecret(account, "us-east-1", "staged/secret", "v1", nil, "", "", "")
	if err != nil {
		t.Fatal(err)
	}

	pending, err := st.PutSecretValueWithStages(account, created.Name, "v2-pending", nil, "ver-pending", []string{store.SecretVersionStagePending})
	if err != nil {
		t.Fatal(err)
	}
	if pending.Name != created.Name {
		t.Fatalf("pending=%+v", pending)
	}

	current, err := st.PutSecretValueWithStages(account, created.Name, "v3-current", nil, "", nil)
	if err != nil {
		t.Fatal(err)
	}
	if current.SecretString != "v3-current" && current.Name != created.Name {
		t.Fatalf("current=%+v", current)
	}

	both, err := st.PutSecretValueWithStages(account, created.Name, "v4-both", nil, "ver-both", []string{
		store.SecretVersionStagePending,
		store.SecretVersionStageCurrent,
	})
	if err != nil {
		t.Fatal(err)
	}
	if both.Name != created.Name {
		t.Fatalf("both=%+v", both)
	}

	if _, err := st.PutSecretValueWithStages(account, "missing-secret", "x", nil, "", nil); !errors.Is(err, store.ErrSecretNotFound) {
		t.Fatalf("missing secret err=%v", err)
	}
}
