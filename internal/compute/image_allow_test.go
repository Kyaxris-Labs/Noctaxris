package compute_test

import (
	"strings"
	"testing"

	"github.com/Kyaxris-Labs/Noctaxris/internal/compute"
	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

func TestAllowImagePullLabAndPinned(t *testing.T) {
	t.Parallel()
	ok := []string{
		store.LabRegistryHost + "/000000000001/repo:tag",
		"host.docker.internal:4566/000000000001/repo:tag",
		"public.ecr.aws/lambda/python:3.12",
		"public.ecr.aws/lambda/python:3.12-v2",
		"alpine:3.20",
		"postgres:16-alpine",
	}
	for _, ref := range ok {
		if err := compute.AllowImagePull(ref); err != nil {
			t.Fatalf("%s: %v", ref, err)
		}
	}
}

func TestAllowImagePullRejectsAttackerRegistry(t *testing.T) {
	t.Parallel()
	err := compute.AllowImagePull("evil.registry.example/malware:latest")
	if err == nil {
		t.Fatal("expected reject")
	}
	if !strings.Contains(err.Error(), "not allowlisted") {
		t.Fatalf("error=%v", err)
	}
	err = compute.AllowImagePull("169.254.169.254/latest/meta:1")
	if err == nil {
		t.Fatal("expected reject metadata-style host")
	}
}

func TestAllowImagePullExtraAllowlistRequiresDigest(t *testing.T) {
	t.Setenv(compute.EnvImagePullAllowlist, "ghcr.io/kyaxris-labs/")
	err := compute.AllowImagePull("ghcr.io/kyaxris-labs/tool:latest")
	if err == nil || !strings.Contains(err.Error(), "digest") {
		t.Fatalf("expected digest requirement, got %v", err)
	}
	if err := compute.AllowImagePull("ghcr.io/kyaxris-labs/tool@sha256:" + strings.Repeat("a", 64)); err != nil {
		t.Fatal(err)
	}
}

func TestValidateECSRunOptsRejectsForeignImage(t *testing.T) {
	t.Parallel()
	err := compute.ValidateECSRunOpts(compute.ECSRunOpts{ImageURI: "attacker.example/x:1"})
	if err == nil {
		t.Fatal("expected error")
	}
}
