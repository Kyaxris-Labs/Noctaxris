package compute_test

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/Kyaxris-Labs/Noctaxris/internal/compute"
	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

func TestLabImageForDinD(t *testing.T) {
	t.Parallel()
	labURI := store.LabRegistryHost + "/000000000001/lambda-lab:v1"
	ref, isLab := compute.LabImageForDinD(labURI, "127.0.0.1:4566")
	if !isLab {
		t.Fatal("expected lab registry detection")
	}
	want := "host.docker.internal:4566/000000000001/lambda-lab:v1"
	if ref != want {
		t.Fatalf("ref=%q want %q", ref, want)
	}

	public := "public.ecr.aws/lambda/python:3.12"
	ref, isLab = compute.LabImageForDinD(public, "127.0.0.1:4566")
	if isLab {
		t.Fatal("public image must not be treated as lab registry")
	}
	if ref != public {
		t.Fatalf("ref=%q want unchanged %q", ref, public)
	}
}

func TestRequireLabRegistryCredsFailClosed(t *testing.T) {
	t.Parallel()
	if err := compute.RequireLabRegistryCreds(false, compute.LabRegistryPullCreds{}); err != nil {
		t.Fatalf("public pull: %v", err)
	}
	err := compute.RequireLabRegistryCreds(true, compute.LabRegistryPullCreds{})
	if err == nil {
		t.Fatal("expected error when lab pull lacks credentials")
	}
	if !strings.Contains(err.Error(), "requires registry credentials") {
		t.Fatalf("error=%q", err)
	}
	if err := compute.RequireLabRegistryCreds(true, compute.LabRegistryPullCreds{
		Username: "AWS",
		Password: "token",
	}); err != nil {
		t.Fatal(err)
	}
}

func TestValidateImageRunOptsLabRegistryAuth(t *testing.T) {
	t.Parallel()
	base := compute.ImageRunOpts{
		ImageURI:        "host.docker.internal:4566/000000000001/repo:tag",
		Handler:         "app.handler",
		EventHostPath:   "/tmp/noctaxris-event",
		LabRegistryPull: true,
	}
	err := compute.ValidateImageRunOpts(base)
	if err == nil {
		t.Fatal("expected error for lab pull without credentials")
	}
	base.RegistryUsername = "AWS"
	base.RegistryPassword = "lab-token"
	if err := compute.ValidateImageRunOpts(base); err != nil {
		t.Fatal(err)
	}
}

func TestIssueLabRegistryPullLambdaPrincipal(t *testing.T) {
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
	labURI := store.LabRegistryHost + "/000000000001/lambda-glue:v1"
	pullRef, useAuth, user, pass, err := compute.IssueLabRegistryPull(st, "127.0.0.1:4566", account, labURI, "lambda.amazonaws.com")
	if err != nil {
		t.Fatal(err)
	}
	if !useAuth {
		t.Fatal("expected lab registry auth")
	}
	if user != "AWS" || pass == "" {
		t.Fatalf("creds user=%q pass empty=%v", user, pass == "")
	}
	wantRef := "host.docker.internal:4566/000000000001/lambda-glue:v1"
	if pullRef != wantRef {
		t.Fatalf("pullRef=%q want %q", pullRef, wantRef)
	}
	gotAccount, gotPrincipal, _, err := st.ValidateAuthorizationToken(pass)
	if err != nil {
		t.Fatalf("ValidateAuthorizationToken: %v", err)
	}
	if gotAccount != account {
		t.Fatalf("token account=%q want %q", gotAccount, account)
	}
	if gotPrincipal != "lambda.amazonaws.com" {
		t.Fatalf("token principal=%q", gotPrincipal)
	}

	public := "public.ecr.aws/lambda/python:3.12"
	pullRef, useAuth, user, pass, err = compute.IssueLabRegistryPull(st, "127.0.0.1:4566", account, public, "lambda.amazonaws.com")
	if err != nil {
		t.Fatal(err)
	}
	if useAuth || user != "" || pass != "" {
		t.Fatalf("public image must not issue auth useAuth=%v user=%q", useAuth, user)
	}
	if pullRef != public {
		t.Fatalf("pullRef=%q want %q", pullRef, public)
	}
}
