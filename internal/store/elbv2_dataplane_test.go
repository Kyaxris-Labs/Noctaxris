package store_test

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

func elbv2OpenTestStore(t *testing.T) *store.Store {
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

func TestELBv2NLBForwardURLIPTarget(t *testing.T) {
	st := elbv2OpenTestStore(t)
	account := "000000000001"
	tg, err := st.CreateELBv2TargetGroup(account, "us-east-1", "nlb-ip-tg", "ip", "TCP", 8080)
	if err != nil {
		t.Fatalf("CreateELBv2TargetGroup: %v", err)
	}
	target := store.ELBv2Target{ID: "10.0.0.42", Port: 0}
	got, err := st.ELBv2NLBForwardURL(account, tg, target, "/health", "x=1")
	if err != nil {
		t.Fatalf("ELBv2NLBForwardURL: %v", err)
	}
	if !strings.HasPrefix(got, "http://10.0.0.42:8080/health?x=1") {
		t.Fatalf("url=%q", got)
	}
}

func TestELBv2NLBForwardURLDeniesLinkLocalAndUnspecified(t *testing.T) {
	st := elbv2OpenTestStore(t)
	account := "000000000001"
	tg, err := st.CreateELBv2TargetGroup(account, "us-east-1", "nlb-deny-tg", "ip", "TCP", 80)
	if err != nil {
		t.Fatalf("CreateELBv2TargetGroup: %v", err)
	}
	for _, id := range []string{"0.0.0.0", "169.254.169.254", "::"} {
		_, err := st.ELBv2NLBForwardURL(account, tg, store.ELBv2Target{ID: id}, "/", "")
		if err == nil {
			t.Fatalf("want deny for %s", id)
		}
	}
}

func TestELBv2NLBForwardURLInstanceRequiresPrivateIP(t *testing.T) {
	st := elbv2OpenTestStore(t)
	account := "000000000001"
	tg, err := st.CreateELBv2TargetGroup(account, "us-east-1", "nlb-inst-tg", "instance", "TCP", 80)
	if err != nil {
		t.Fatalf("CreateELBv2TargetGroup: %v", err)
	}
	_, err = st.ELBv2NLBForwardURL(account, tg, store.ELBv2Target{ID: "i-missing"}, "/", "")
	if err == nil || !strings.Contains(err.Error(), "not reachable") {
		t.Fatalf("want not reachable, got %v", err)
	}
}
