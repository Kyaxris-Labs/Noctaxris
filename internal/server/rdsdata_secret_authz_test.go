package server

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Kyaxris-Labs/Noctaxris/internal/config"
	"github.com/Kyaxris-Labs/Noctaxris/internal/kernel/audit"
	"github.com/Kyaxris-Labs/Noctaxris/internal/kernel/authn"
	"github.com/Kyaxris-Labs/Noctaxris/internal/kernel/identity"
	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

func TestRDSDataMasterCredsAuthorizesGetSecretValue(t *testing.T) {
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
	account := "000000000001"
	if err := st.EnsureRoot(account, "AKIAROOTEXAMPLE01", "secret-root-value"); err != nil {
		t.Fatal(err)
	}
	aud, err := audit.NewWriter(filepath.Join(dir, "cloudtrail"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = aud.Close() })
	srv := New(config.Config{ListenAddr: "127.0.0.1:0", DataRoot: dir, AccountID: account}, st, aud)

	sec, err := st.CreateSecret(account, "us-east-1", "rds-authz-secret", `{"username":"postgres","password":"lab-pass"}`, nil, "", "", "")
	if err != nil {
		t.Fatal(err)
	}
	inst := store.RDSDBInstance{
		DBInstanceIdentifier: "lab-pg",
		Engine:               "postgres",
		MasterUsername:       "postgres",
		MasterUserSecretARN:  sec.ARN,
	}

	userName := "rds-no-secret"
	if _, _, err := st.CreateUser(account, userName); err != nil {
		t.Fatal(err)
	}
	akid, _, err := st.CreateUserAccessKey(account, userName)
	if err != nil {
		t.Fatal(err)
	}
	allowRDS := `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Action":"rds-data:*","Resource":"*"}]}`
	polARN, err := st.CreateManagedPolicy(account, "RDSDataOnly", allowRDS)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.AttachUserPolicy(account, userName, polARN); err != nil {
		t.Fatal(err)
	}

	v := &authn.Verified{
		AccountID:   account,
		AccessKeyID: akid,
		Region:      "us-east-1",
		Service:     "rds-data",
		Principal:   identity.UserPrincipal(account, userName, akid),
	}
	ctx := withRDSDataVerified(context.Background(), v)
	_, _, err = srv.rdsDataMasterCreds(ctx, account, inst, sec.ARN)
	if err == nil || !strings.Contains(err.Error(), "secretsmanager:GetSecretValue") {
		t.Fatalf("want GetSecretValue deny, got %v", err)
	}

	_, _, err = srv.rdsDataMasterCreds(context.Background(), account, inst, sec.ARN)
	if err == nil || !strings.Contains(err.Error(), "secretsmanager:GetSecretValue") {
		t.Fatalf("want fail-closed without verified, got %v", err)
	}

	allowSecret := `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Action":"secretsmanager:GetSecretValue","Resource":"*"}]}`
	secPol, err := st.CreateManagedPolicy(account, "RDSSecretRead", allowSecret)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.AttachUserPolicy(account, userName, secPol); err != nil {
		t.Fatal(err)
	}
	user, pass, err := srv.rdsDataMasterCreds(ctx, account, inst, sec.ARN)
	if err != nil {
		t.Fatalf("GetSecretValue Allow: %v", err)
	}
	if user != "postgres" || pass != "lab-pass" {
		t.Fatalf("creds user=%q pass=%q", user, pass)
	}
}
