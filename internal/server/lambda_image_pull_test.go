package server

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/Kyaxris-Labs/Noctaxris/internal/config"
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
	labURI := store.LabRegistryHost + "/" + account + "/lambda-lab:v1"
	s := &Server{
		cfg:   config.Config{ListenAddr: "127.0.0.1:4566"},
		store: st,
	}
	fn := store.LambdaFunction{
		PackageType: store.LambdaPackageTypeImage,
		ImageURI:    labURI,
		Handler:     "app.handler",
		Timeout:     3,
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

	public := store.LambdaFunction{
		PackageType: store.LambdaPackageTypeImage,
		ImageURI:    "public.ecr.aws/lambda/python:3.12",
		Handler:     "app.handler",
		Timeout:     3,
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
