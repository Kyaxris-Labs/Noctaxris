package server

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/Kyaxris-Labs/Noctaxris/internal/config"
	"github.com/Kyaxris-Labs/Noctaxris/internal/kernel/audit"
	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

func TestLabRegistryPullDenyWithoutECRAllow(t *testing.T) {
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
	accountID := "000000000001"
	if err := st.EnsureRoot(accountID, "AKIAROOTEXAMPLE01", "secret-root-value"); err != nil {
		t.Fatal(err)
	}
	roleARN, err := st.CreateRole(accountID, "no-ecr", `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":{"Service":"ecs-tasks.amazonaws.com"},"Action":"sts:AssumeRole"}]}`)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.CreateRepository(accountID, store.DefaultECRRegion, "deny-repo"); err != nil {
		t.Fatal(err)
	}
	aud, err := audit.NewWriter(filepath.Join(dir, "cloudtrail"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = aud.Close() })
	srv := New(config.Config{ListenAddr: "127.0.0.1:4566", DataRoot: dir, AccountID: accountID}, st, aud)

	labURI := store.LabRegistryHost + "/" + accountID + "/deny-repo:v1"
	_, _, _, _, err = srv.labRegistryPullOpts(accountID, labURI, roleARN)
	if err == nil || !strings.Contains(err.Error(), "ecr:BatchGetImage") {
		t.Fatalf("expected BatchGetImage deny, got %v", err)
	}
}
