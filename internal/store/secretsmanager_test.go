package store_test

import (
	"bytes"
	"errors"
	"path/filepath"
	"testing"

	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

func openSecretsStore(t *testing.T) *store.Store {
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
	if err := st.EnsureSecretsSchema(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	return st
}

func TestSecretsCreateGetStringRoundTrip(t *testing.T) {
	st := openSecretsStore(t)
	account := "000000000001"

	created, err := st.CreateSecret(account, "us-east-1", "app/db-password", "s3cr3t!", nil, "", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if created.Name != "app/db-password" || created.Version != 1 {
		t.Fatalf("created=%+v", created)
	}
	if created.ARN == "" || created.KmsKeyID == "" {
		t.Fatalf("arn/kms missing: %+v", created)
	}
	if created.SecretString != "s3cr3t!" {
		t.Fatalf("create should echo secret string, got %q", created.SecretString)
	}

	got, err := st.GetSecretValue(account, "app/db-password")
	if err != nil {
		t.Fatal(err)
	}
	if got.SecretString != "s3cr3t!" || got.Version != 1 {
		t.Fatalf("got=%+v", got)
	}
	if got.KmsKeyID != created.KmsKeyID {
		t.Fatalf("kms key mismatch: got=%q want=%q", got.KmsKeyID, created.KmsKeyID)
	}

	keyID, err := st.ResolveKeyID(account, store.AliasAWSSecretsManager)
	if err != nil {
		t.Fatal(err)
	}
	if got.KmsKeyID != keyID {
		t.Fatalf("keyID=%q want alias target %q", got.KmsKeyID, keyID)
	}
}

func TestSecretsCreateGetBinaryRoundTrip(t *testing.T) {
	st := openSecretsStore(t)
	account := "000000000001"
	bin := []byte{0xde, 0xad, 0xbe, 0xef}

	created, err := st.CreateSecret(account, "us-east-1", "app/cert", "", bin, "", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(created.SecretBinary, bin) {
		t.Fatalf("created binary=%v", created.SecretBinary)
	}

	got, err := st.GetSecretValue(account, created.ARN)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got.SecretBinary, bin) {
		t.Fatalf("got binary=%v", got.SecretBinary)
	}
}

func TestSecretsCreateAlreadyExists(t *testing.T) {
	st := openSecretsStore(t)
	account := "000000000001"

	if _, err := st.CreateSecret(account, "us-east-1", "dup", "v1", nil, "", "", ""); err != nil {
		t.Fatal(err)
	}
	if _, err := st.CreateSecret(account, "us-east-1", "dup", "v2", nil, "", "", ""); !errors.Is(err, store.ErrSecretAlreadyExists) {
		t.Fatalf("want ErrSecretAlreadyExists, got %v", err)
	}
}

func TestSecretsPutSecretValueAndDescribe(t *testing.T) {
	st := openSecretsStore(t)
	account := "000000000001"

	if _, err := st.CreateSecret(account, "us-east-1", "rotating", "v1", nil, "", "", ""); err != nil {
		t.Fatal(err)
	}

	updated, err := st.PutSecretValue(account, "rotating", "v2", nil)
	if err != nil {
		t.Fatal(err)
	}
	if updated.SecretString != "v2" || updated.Version != 2 {
		t.Fatalf("updated=%+v", updated)
	}

	desc, err := st.DescribeSecret(account, "rotating")
	if err != nil {
		t.Fatal(err)
	}
	if desc.Version != 2 || desc.SecretString != "" {
		t.Fatalf("describe should omit value, got %+v", desc)
	}
	if desc.LastChangedDate == "" || desc.CreatedDate == "" {
		t.Fatalf("dates missing: %+v", desc)
	}
}

func TestSecretsListSecrets(t *testing.T) {
	st := openSecretsStore(t)
	account := "000000000001"

	for _, name := range []string{"alpha", "beta"} {
		if _, err := st.CreateSecret(account, "us-east-1", name, name, nil, "", "", ""); err != nil {
			t.Fatal(err)
		}
	}

	list, err := st.ListSecrets(account)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 2 {
		t.Fatalf("list=%+v", list)
	}
	for _, sec := range list {
		if sec.SecretString != "" || len(sec.SecretBinary) > 0 {
			t.Fatalf("list should omit values, got %+v", sec)
		}
	}
}

func TestSecretsDeleteSecret(t *testing.T) {
	st := openSecretsStore(t)
	account := "000000000001"

	if _, err := st.CreateSecret(account, "us-east-1", "gone", "x", nil, "", "", ""); err != nil {
		t.Fatal(err)
	}
	if err := st.DeleteSecret(account, "gone"); err != nil {
		t.Fatal(err)
	}
	if _, err := st.GetSecretValue(account, "gone"); !errors.Is(err, store.ErrSecretNotFound) {
		t.Fatalf("want ErrSecretNotFound, got %v", err)
	}
}

func TestSecretsResourcePolicyLifecycle(t *testing.T) {
	st := openSecretsStore(t)
	account := "000000000001"

	if _, err := st.CreateSecret(account, "us-east-1", "policy-test", "x", nil, "", "", ""); err != nil {
		t.Fatal(err)
	}

	if _, err := st.GetSecretResourcePolicy(account, "policy-test"); !errors.Is(err, store.ErrNoSuchResourcePolicy) {
		t.Fatalf("want ErrNoSuchResourcePolicy, got %v", err)
	}

	policy := `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":"*","Action":"secretsmanager:GetSecretValue","Resource":"*"}]}`
	if err := st.PutSecretResourcePolicy(account, "policy-test", policy); err != nil {
		t.Fatal(err)
	}
	got, err := st.GetSecretResourcePolicy(account, "policy-test")
	if err != nil {
		t.Fatal(err)
	}
	if got != policy {
		t.Fatalf("policy=%q", got)
	}

	if err := st.DeleteSecretResourcePolicy(account, "policy-test"); err != nil {
		t.Fatal(err)
	}
	if _, err := st.GetSecretResourcePolicy(account, "policy-test"); !errors.Is(err, store.ErrNoSuchResourcePolicy) {
		t.Fatalf("want ErrNoSuchResourcePolicy after delete, got %v", err)
	}
}

func TestSecretsGetSecretNotFound(t *testing.T) {
	st := openSecretsStore(t)
	if _, err := st.GetSecretValue("000000000001", "missing"); !errors.Is(err, store.ErrSecretNotFound) {
		t.Fatalf("want ErrSecretNotFound, got %v", err)
	}
}
