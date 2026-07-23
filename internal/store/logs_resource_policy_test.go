package store_test

import (
	"errors"
	"path/filepath"
	"testing"

	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

func openLogsPolicyStore(t *testing.T) *store.Store {
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

func TestLogsResourcePolicyLifecycle(t *testing.T) {
	st := openLogsPolicyStore(t)
	account := "000000000001"
	doc := `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":{"Service":"events.amazonaws.com"},"Action":["logs:PutLogEvents","logs:CreateLogStream"],"Resource":"*"}]}`
	p, err := st.PutLogsResourcePolicy(account, "eb-to-logs", doc)
	if err != nil {
		t.Fatal(err)
	}
	if p.PolicyName != "eb-to-logs" || p.PolicyDocument != doc {
		t.Fatalf("put=%+v", p)
	}
	got, err := st.GetLogsResourcePolicy(account, "eb-to-logs")
	if err != nil || got.PolicyDocument != doc {
		t.Fatalf("get=%+v err=%v", got, err)
	}
	list, err := st.DescribeLogsResourcePolicies(account)
	if err != nil || len(list) != 1 {
		t.Fatalf("describe=%+v err=%v", list, err)
	}
	if err := st.DeleteLogsResourcePolicy(account, "eb-to-logs"); err != nil {
		t.Fatal(err)
	}
	if _, err := st.GetLogsResourcePolicy(account, "eb-to-logs"); !errors.Is(err, store.ErrLogsResourcePolicyNotFound) {
		t.Fatalf("want ErrLogsResourcePolicyNotFound, got %v", err)
	}
}

func TestLogsResourcePolicyRequiresPrincipal(t *testing.T) {
	st := openLogsPolicyStore(t)
	_, err := st.PutLogsResourcePolicy("000000000001", "bad", `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Action":"logs:PutLogEvents","Resource":"*"}]}`)
	if err == nil {
		t.Fatal("expected validation error for missing Principal")
	}
}
