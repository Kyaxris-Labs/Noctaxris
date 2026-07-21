package server

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/Kyaxris-Labs/Noctaxris/internal/compute"
	"github.com/Kyaxris-Labs/Noctaxris/internal/config"
	"github.com/Kyaxris-Labs/Noctaxris/internal/kernel/audit"
	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

func TestMintRoleSessionEnvInjectsCredentials(t *testing.T) {
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
	roleARN, err := st.CreateRole(accountID, "lab-job", `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":{"Service":"ecs-tasks.amazonaws.com"},"Action":"sts:AssumeRole"}]}`)
	if err != nil {
		t.Fatal(err)
	}
	aud, err := audit.NewWriter(filepath.Join(dir, "cloudtrail"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = aud.Close() })
	srv := New(config.Config{ListenAddr: "127.0.0.1:4566", DataRoot: dir, AccountID: accountID}, st, aud)

	env, err := srv.mintRoleSessionEnv(roleARN, "noctaxris-batch", "http://host.docker.internal:4566", store.DefaultBatchRegion)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(env["AWS_ACCESS_KEY_ID"], "ASIA") {
		t.Fatalf("AWS_ACCESS_KEY_ID=%q", env["AWS_ACCESS_KEY_ID"])
	}
	if env["AWS_SECRET_ACCESS_KEY"] == "" || env["AWS_SESSION_TOKEN"] == "" {
		t.Fatal("missing secret or session token")
	}
	if env["AWS_ENDPOINT_URL"] != "http://host.docker.internal:4566" {
		t.Fatalf("AWS_ENDPOINT_URL=%q", env["AWS_ENDPOINT_URL"])
	}
	acct, secret, isRoot, err := st.LookupAccessKey(env["AWS_ACCESS_KEY_ID"])
	if err != nil {
		t.Fatalf("LookupAccessKey: %v", err)
	}
	if acct != accountID || isRoot || secret != env["AWS_SECRET_ACCESS_KEY"] {
		t.Fatalf("lookup account=%q isRoot=%v secret match=%v", acct, isRoot, secret == env["AWS_SECRET_ACCESS_KEY"])
	}
}

func TestIssueLabRegistryPullForBatchPrincipal(t *testing.T) {
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
	labURI := store.LabRegistryHost + "/" + accountID + "/batch-img:v1"
	pullRef, useAuth, user, pass, err := compute.IssueLabRegistryPull(
		st, "127.0.0.1:4566", accountID, labURI, "ecs-tasks.amazonaws.com",
	)
	if err != nil {
		t.Fatal(err)
	}
	if !useAuth || user != "AWS" || pass == "" {
		t.Fatalf("lab auth useAuth=%v user=%q", useAuth, user)
	}
	if pullRef != "host.docker.internal:4566/"+accountID+"/batch-img:v1" {
		t.Fatalf("pullRef=%q", pullRef)
	}
	public := "alpine:3.20"
	pullRef, useAuth, _, _, err = compute.IssueLabRegistryPull(
		st, "127.0.0.1:4566", accountID, public, "codebuild.amazonaws.com",
	)
	if err != nil {
		t.Fatal(err)
	}
	if useAuth || pullRef != public {
		t.Fatalf("public pullRef=%q useAuth=%v", pullRef, useAuth)
	}
}
