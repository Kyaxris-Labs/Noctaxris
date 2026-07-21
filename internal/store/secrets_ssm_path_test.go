package store_test

import (
	"errors"
	"testing"
	"time"

	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

func TestSecretsRecoveryWindowAndRotate(t *testing.T) {
	st := openSecretsStore(t)
	account := "000000000001"

	created, err := st.CreateSecret(account, "us-east-1", "rot-me", "v1", nil, "", "")
	if err != nil {
		t.Fatal(err)
	}
	rotated, err := st.RotateSecret(account, "rot-me")
	if err != nil {
		t.Fatal(err)
	}
	if rotated.Version != created.Version+1 || rotated.SecretString == "" || rotated.SecretString == "v1" {
		t.Fatalf("rotate=%+v created=%+v", rotated, created)
	}

	sec, deletion, err := st.DeleteSecretWithRecovery(account, "rot-me", 7, false)
	if err != nil {
		t.Fatal(err)
	}
	if sec.DeletionDate == "" || deletion.Before(time.Now().UTC()) {
		t.Fatalf("sec=%+v deletion=%v", sec, deletion)
	}
	if _, err := st.GetSecretValue(account, "rot-me"); !errors.Is(err, store.ErrSecretScheduledDeletion) {
		t.Fatalf("want ErrSecretScheduledDeletion, got %v", err)
	}
	if err := st.RestoreSecret(account, "rot-me"); err != nil {
		t.Fatal(err)
	}
	got, err := st.GetSecretValue(account, "rot-me")
	if err != nil {
		t.Fatal(err)
	}
	if got.SecretString != rotated.SecretString {
		t.Fatalf("after restore got=%+v", got)
	}

	if _, _, err := st.DeleteSecretWithRecovery(account, "rot-me", 0, true); err != nil {
		t.Fatal(err)
	}
	if _, err := st.GetSecretValue(account, "rot-me"); !errors.Is(err, store.ErrSecretNotFound) {
		t.Fatalf("want ErrSecretNotFound after force delete, got %v", err)
	}
}

func TestSecretsSweepExpired(t *testing.T) {
	st := openSecretsStore(t)
	account := "000000000001"
	if _, err := st.CreateSecret(account, "us-east-1", "expire-me", "x", nil, "", ""); err != nil {
		t.Fatal(err)
	}
	if _, _, err := st.DeleteSecretWithRecovery(account, "expire-me", 7, false); err != nil {
		t.Fatal(err)
	}
	n, err := st.SweepExpiredSecrets(time.Now().UTC().Add(-48 * time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatalf("sweep before deletion_date should keep secret, n=%d", n)
	}
	n, err = st.SweepExpiredSecrets(time.Now().UTC().Add(40 * 24 * time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("sweep after deletion_date should delete secret, n=%d", n)
	}
}

func TestSSMGetParametersByPath(t *testing.T) {
	st := openSSMStore(t)
	account := "000000000001"
	puts := []struct {
		name, value string
	}{
		{"/app/dev/log", "debug"},
		{"/app/dev/db/host", "localhost"},
		{"/app/prod/log", "info"},
	}
	for _, p := range puts {
		if _, err := st.PutParameter(account, "us-east-1", p.name, store.ParamTypeString, p.value, "", false); err != nil {
			t.Fatal(err)
		}
	}

	shallow, err := st.GetParametersByPath(account, "/app/dev", false, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(shallow) != 1 || shallow[0].Name != "/app/dev/log" {
		t.Fatalf("shallow=%+v", shallow)
	}

	deep, err := st.GetParametersByPath(account, "/app/dev/", true, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(deep) != 2 {
		t.Fatalf("deep=%+v", deep)
	}
}
