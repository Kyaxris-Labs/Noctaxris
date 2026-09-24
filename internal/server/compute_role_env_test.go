package server

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

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
	akid := env["AWS_ACCESS_KEY_ID"]
	if !strings.HasPrefix(akid, "ASIA") || len(akid) != 20 {
		t.Fatalf("AWS_ACCESS_KEY_ID=%q, want 20-char ASIA id", akid)
	}
	if akid != strings.ToUpper(akid) {
		t.Fatalf("AWS_ACCESS_KEY_ID=%q, want uppercase", akid)
	}
	if env["AWS_SECRET_ACCESS_KEY"] == "" || env["AWS_SESSION_TOKEN"] == "" {
		t.Fatal("missing secret or session token")
	}
	wantEndpoint := "http://host.docker.internal:4566"
	if env["AWS_ENDPOINT_URL"] != wantEndpoint {
		t.Fatalf("AWS_ENDPOINT_URL=%q", env["AWS_ENDPOINT_URL"])
	}
	for _, key := range []string{
		"AWS_ENDPOINT_URL_STS",
		"AWS_ENDPOINT_URL_IAM",
		"AWS_ENDPOINT_URL_S3",
		"AWS_ENDPOINT_URL_DYNAMODB",
		"AWS_ENDPOINT_URL_SQS",
		"AWS_ENDPOINT_URL_LAMBDA",
		"AWS_ENDPOINT_URL_KMS",
		"AWS_ENDPOINT_URL_ECR",
		"AWS_ENDPOINT_URL_ECS",
		"AWS_ENDPOINT_URL_SNS",
		"AWS_ENDPOINT_URL_LOGS",
		"AWS_ENDPOINT_URL_CODEBUILD",
		"AWS_ENDPOINT_URL_SECRETSMANAGER",
		"AWS_ENDPOINT_URL_SECRETS_MANAGER",
	} {
		if env[key] != wantEndpoint {
			t.Fatalf("%s=%q want %q", key, env[key], wantEndpoint)
		}
	}
	acct, secret, isRoot, err := st.LookupAccessKey(env["AWS_ACCESS_KEY_ID"])
	if err != nil {
		t.Fatalf("LookupAccessKey: %v", err)
	}
	if acct != accountID || isRoot || secret != env["AWS_SECRET_ACCESS_KEY"] {
		t.Fatalf("lookup account=%q isRoot=%v secret match=%v", acct, isRoot, secret == env["AWS_SECRET_ACCESS_KEY"])
	}
}

func TestMintRoleSessionEnvIgnoresLabClock(t *testing.T) {
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
	roleARN, err := st.CreateRole(accountID, "clock-job", `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":{"Service":"ecs-tasks.amazonaws.com"},"Action":"sts:AssumeRole"}]}`)
	if err != nil {
		t.Fatal(err)
	}
	aud, err := audit.NewWriter(filepath.Join(dir, "cloudtrail"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = aud.Close() })
	srv := New(config.Config{ListenAddr: "127.0.0.1:4566", DataRoot: dir, AccountID: accountID}, st, aud)
	future := time.Date(2099, 6, 15, 12, 0, 0, 0, time.UTC)
	srv.setLabClock(future)

	before := time.Now().UTC()
	env, err := srv.mintRoleSessionEnv(roleARN, "noctaxris-clock", "http://127.0.0.1:4566", "us-east-1")
	if err != nil {
		t.Fatal(err)
	}
	after := time.Now().UTC()
	ak, err := st.LookupAccessKeyRecord(env["AWS_ACCESS_KEY_ID"])
	if err != nil {
		t.Fatal(err)
	}
	if ak.ExpiresAt.Year() >= 2090 {
		t.Fatalf("lab clock leaked into ExpiresAt %s", ak.ExpiresAt)
	}
	lo := before.Add(defaultSessionDuration).Add(-2 * time.Second)
	hi := after.Add(defaultSessionDuration).Add(5 * time.Second)
	if ak.ExpiresAt.Before(lo) || ak.ExpiresAt.After(hi) {
		t.Fatalf("ExpiresAt=%s want between %s and %s", ak.ExpiresAt, lo, hi)
	}
	if !srv.effectiveNow().Equal(future) {
		t.Fatalf("lab clock should stay %s got %s", future, srv.effectiveNow())
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
	roleARN := "arn:aws:iam::" + accountID + ":role/batch-exec"
	labURI := store.LabRegistryHost + "/" + accountID + "/batch-img:v1"
	pullRef, useAuth, user, pass, err := compute.IssueLabRegistryPull(
		st, "127.0.0.1:4566", accountID, labURI, roleARN,
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
	_, gotPrincipal, _, err := st.ValidateAuthorizationToken(pass)
	if err != nil {
		t.Fatal(err)
	}
	if gotPrincipal != roleARN {
		t.Fatalf("principal=%q want %q", gotPrincipal, roleARN)
	}
	public := "alpine:3.20"
	pullRef, useAuth, _, _, err = compute.IssueLabRegistryPull(
		st, "127.0.0.1:4566", accountID, public, roleARN,
	)
	if err != nil {
		t.Fatal(err)
	}
	if useAuth || pullRef != public {
		t.Fatalf("public pullRef=%q useAuth=%v", pullRef, useAuth)
	}
}
