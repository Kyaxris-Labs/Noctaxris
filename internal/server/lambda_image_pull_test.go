package server

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/Kyaxris-Labs/Noctaxris/internal/config"
	"github.com/Kyaxris-Labs/Noctaxris/internal/kernel/audit"
	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

func TestPrepareLambdaImageRunOptsLabECRAuth(t *testing.T) {
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
	roleARN, err := st.CreateRole(account, "lambda-exec", `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":{"Service":"lambda.amazonaws.com"},"Action":"sts:AssumeRole"}]}`)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.PutInlinePolicy(roleARN, "ecr-pull", `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Action":["ecr:BatchGetImage","ecr:GetDownloadUrlForLayer","ecr:GetAuthorizationToken"],"Resource":"*"}]}`); err != nil {
		t.Fatal(err)
	}
	if _, err := st.CreateRepository(account, store.DefaultECRRegion, "lambda-lab"); err != nil {
		t.Fatal(err)
	}
	aud, err := audit.NewWriter(filepath.Join(dir, "cloudtrail"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = aud.Close() })

	labURI := store.LabRegistryHost + "/" + account + "/lambda-lab:v1"
	s := New(config.Config{ListenAddr: "127.0.0.1:4566", DataRoot: dir, AccountID: account}, st, aud)
	fn := store.LambdaFunction{
		PackageType: store.LambdaPackageTypeImage,
		ImageURI:    labURI,
		Handler:     "app.handler",
		Timeout:     3,
		RoleARN:     roleARN,
	}
	opts, err := s.prepareLambdaImageRunOpts(account, fn, nil, `{}`, "http://127.0.0.1:4566", "/tmp/event")
	if err != nil {
		t.Fatal(err)
	}
	if !opts.LabRegistryPull {
		t.Fatal("lab ImageUri must request authenticated registry pull")
	}
	if opts.RegistryUsername != "AWS" || opts.RegistryPassword == "" {
		t.Fatalf("expected lab registry creds got user=%q pass empty=%v", opts.RegistryUsername, opts.RegistryPassword == "")
	}
	if !strings.HasPrefix(opts.ImageURI, "host.docker.internal:4566/") {
		t.Fatalf("ImageURI=%q want DinD rewrite", opts.ImageURI)
	}
	_, principal, _, err := st.ValidateAuthorizationToken(opts.RegistryPassword)
	if err != nil {
		t.Fatal(err)
	}
	if principal != roleARN {
		t.Fatalf("token principal=%q want %q", principal, roleARN)
	}
	if !opts.AllowDefaultEntrypoint {
		t.Fatal("lab ECR ImageUri should allow default entrypoint (full-container exec)")
	}

	public := store.LambdaFunction{
		PackageType: store.LambdaPackageTypeImage,
		ImageURI:    "public.ecr.aws/lambda/python:3.12",
		Handler:     "app.handler",
		Timeout:     3,
		RoleARN:     roleARN,
	}
	pubOpts, err := s.prepareLambdaImageRunOpts(account, public, nil, `{}`, "http://127.0.0.1:4566", "/tmp/event")
	if err != nil {
		t.Fatal(err)
	}
	if pubOpts.LabRegistryPull {
		t.Fatal("public ImageUri must not request lab registry auth")
	}
	if pubOpts.ImageURI != public.ImageURI {
		t.Fatalf("public ImageURI changed: %q", pubOpts.ImageURI)
	}
}
