package store_test

import (
	"errors"
	"path/filepath"
	"testing"

	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

func openSSMStore(t *testing.T) *store.Store {
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
	if err := st.EnsureSSMSchema(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	return st
}

func TestSSMPutGetStringRoundTrip(t *testing.T) {
	st := openSSMStore(t)
	account := "000000000001"

	put, err := st.PutParameter(account, "us-east-1", "/app/config", store.ParamTypeString, "hello", "", false)
	if err != nil {
		t.Fatal(err)
	}
	if put.Type != store.ParamTypeString || put.Value != "hello" || put.Version != 1 {
		t.Fatalf("put=%+v", put)
	}
	if put.ARN != "arn:aws:ssm:us-east-1:000000000001:parameter/app/config" {
		t.Fatalf("arn=%q", put.ARN)
	}

	got, err := st.GetParameter(account, "/app/config", true)
	if err != nil {
		t.Fatal(err)
	}
	if got.Value != "hello" || got.Type != store.ParamTypeString || got.Version != 1 {
		t.Fatalf("got=%+v", got)
	}
}

func TestSSMPutGetSecureStringRoundTrip(t *testing.T) {
	st := openSSMStore(t)
	account := "000000000001"

	put, err := st.PutParameter(account, "us-east-1", "secret/token", store.ParamTypeSecureString, "s3cr3t", "", false)
	if err != nil {
		t.Fatal(err)
	}
	if put.Type != store.ParamTypeSecureString || put.Value != "s3cr3t" || put.KeyID == "" {
		t.Fatalf("put=%+v", put)
	}

	got, err := st.GetParameter(account, "/secret/token", true)
	if err != nil {
		t.Fatal(err)
	}
	if got.Value != "s3cr3t" || got.Type != store.ParamTypeSecureString {
		t.Fatalf("got=%+v", got)
	}

	keyID, err := st.ResolveKeyID(account, store.AliasAWSSSM)
	if err != nil {
		t.Fatalf("resolve alias/aws/ssm: %v", err)
	}
	if got.KeyID != keyID {
		t.Fatalf("keyID=%q want %q", got.KeyID, keyID)
	}
}

func TestSSMPutParameterOverwrite(t *testing.T) {
	st := openSSMStore(t)
	account := "000000000001"

	if _, err := st.PutParameter(account, "us-east-1", "/x", store.ParamTypeString, "v1", "", false); err != nil {
		t.Fatal(err)
	}
	if _, err := st.PutParameter(account, "us-east-1", "x", store.ParamTypeString, "v2", "", false); !errors.Is(err, store.ErrParameterAlreadyExists) {
		t.Fatalf("want ErrParameterAlreadyExists, got %v", err)
	}

	updated, err := st.PutParameter(account, "us-east-1", "/x", store.ParamTypeString, "v2", "", true)
	if err != nil {
		t.Fatal(err)
	}
	if updated.Value != "v2" || updated.Version != 2 {
		t.Fatalf("updated=%+v", updated)
	}
}

func TestSSMGetParametersAndDescribe(t *testing.T) {
	st := openSSMStore(t)
	account := "000000000001"

	for _, spec := range []struct {
		name, val string
		typ       string
	}{
		{"/app/a", "one", store.ParamTypeString},
		{"/app/b", "two", store.ParamTypeSecureString},
		{"/other/c", "three", store.ParamTypeString},
	} {
		if _, err := st.PutParameter(account, "us-east-1", spec.name, spec.typ, spec.val, "", false); err != nil {
			t.Fatal(err)
		}
	}

	batch, err := st.GetParameters(account, []string{"/app/a", "/app/b"}, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(batch) != 2 {
		t.Fatalf("batch=%+v", batch)
	}
	vals := map[string]string{}
	for _, p := range batch {
		vals[p.Name] = p.Value
	}
	if vals["/app/a"] != "one" || vals["/app/b"] != "two" {
		t.Fatalf("vals=%v", vals)
	}

	desc, err := st.DescribeParameters(account, "/app/")
	if err != nil {
		t.Fatal(err)
	}
	if len(desc) != 2 {
		t.Fatalf("describe=%+v", desc)
	}
	for _, p := range desc {
		if p.Value != "" {
			t.Fatalf("describe should omit values, got %+v", p)
		}
	}
}

func TestSSMDeleteParameter(t *testing.T) {
	st := openSSMStore(t)
	account := "000000000001"

	if _, err := st.PutParameter(account, "us-east-1", "/gone", store.ParamTypeString, "x", "", false); err != nil {
		t.Fatal(err)
	}
	if err := st.DeleteParameter(account, "/gone"); err != nil {
		t.Fatal(err)
	}
	if _, err := st.GetParameter(account, "/gone", true); !errors.Is(err, store.ErrParameterNotFound) {
		t.Fatalf("want ErrParameterNotFound, got %v", err)
	}
}

func TestSSMGetParameterNotFound(t *testing.T) {
	st := openSSMStore(t)
	if _, err := st.GetParameter("000000000001", "/missing", true); !errors.Is(err, store.ErrParameterNotFound) {
		t.Fatalf("want ErrParameterNotFound, got %v", err)
	}
}
