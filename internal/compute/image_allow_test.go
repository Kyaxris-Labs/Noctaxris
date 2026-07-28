package compute_test

import (
	"strings"
	"testing"

	"github.com/Kyaxris-Labs/Noctaxris/internal/compute"
	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

func TestAllowImagePullLabAndPinned(t *testing.T) {
	t.Parallel()
	listen := "127.0.0.1:4566"
	ok := []string{
		store.LabRegistryHost + "/000000000001/repo:tag",
		compute.DinDPullHost(listen) + "/000000000001/repo:tag",
		"public.ecr.aws/lambda/python:3.12",
		"public.ecr.aws/lambda/python:3.13",
		"public.ecr.aws/lambda/python:3.14",
		"public.ecr.aws/lambda/nodejs:20",
		"public.ecr.aws/lambda/nodejs:22",
		"public.ecr.aws/lambda/nodejs:24",
		"public.ecr.aws/lambda/java:21",
		"public.ecr.aws/lambda/java:25",
		"python:3.13-slim",
		"python:3.14-slim",
		"node:22-slim",
		"node:24-slim",
		"eclipse-temurin:21-jdk",
		"eclipse-temurin:25-jdk",
		"eclipse-temurin:21-jre",
		"eclipse-temurin:25-jre",
		"public.ecr.aws/lambda/python@sha256:" + strings.Repeat("a", 64),
		"alpine:3.20",
		"postgres:16-alpine",
		"mysql:8.0",
		"mariadb:11",
		"valkey/valkey:8-alpine",
		"mongo:7",
		"rabbitmq:3.13-alpine",
		"apache/activemq-classic:5.18.3",
		"opensearchproject/opensearch:2.11.1",
		"tinkerpop/gremlin-server:3.7.3",
		"redpandadata/redpanda:v24.2.4",
		"eclipse-mosquitto:2.0.20",
	}
	for _, ref := range ok {
		if err := compute.AllowImagePull(ref, listen); err != nil {
			t.Fatalf("%s: %v", ref, err)
		}
	}
}

func TestAllowImagePullPinsHostDockerInternalPort(t *testing.T) {
	t.Parallel()
	listen := "127.0.0.1:4566"
	allowed := compute.DinDPullHost(listen) + "/000000000001/repo:tag"
	if err := compute.AllowImagePull(allowed, listen); err != nil {
		t.Fatalf("lab DinD pull host: %v", err)
	}

	err := compute.AllowImagePull("host.docker.internal:9999/evil/img", listen)
	if err == nil {
		t.Fatal("expected deny for host.docker.internal on non-lab port")
	}
	if !strings.Contains(err.Error(), "not allowlisted") {
		t.Fatalf("error=%v", err)
	}

	err = compute.AllowImagePull("host.docker.internal:9999/000000000001/repo:tag", listen)
	if err == nil {
		t.Fatal("expected deny for account-shaped path on wrong port")
	}

	err = compute.AllowImagePull(compute.DinDPullHost(listen)+"/evil/img", listen)
	if err == nil {
		t.Fatal("expected deny for non ACCOUNT/REPO shape")
	}

	listenAlt := "0.0.0.0:8080"
	pinned := compute.DinDPullHost(listenAlt) + "/000000000001/repo:v1"
	if err := compute.AllowImagePull(pinned, listenAlt); err != nil {
		t.Fatalf("listen-pinned DinD host: %v", err)
	}
	if err := compute.AllowImagePull("host.docker.internal:4566/000000000001/repo:v1", listenAlt); err == nil {
		t.Fatal("expected deny when port does not match DinDPullHost(ListenAddr)")
	}
}

func TestAllowImagePullEmptyListenDeniesDinDHost(t *testing.T) {
	t.Parallel()
	// Empty listen must not silently allow host.docker.internal:4566.
	err := compute.AllowImagePull("host.docker.internal:4566/000000000001/repo:tag", "")
	if err == nil {
		t.Fatal("expected deny for DinD host when listenAddr is empty")
	}
	if err := compute.AllowImagePull(store.LabRegistryHost+"/000000000001/repo:tag", ""); err != nil {
		t.Fatalf("lab registry host must still allow without listen: %v", err)
	}
}

func TestResolveLabImagePullAuthForDinDHost(t *testing.T) {
	t.Parallel()
	listen := "127.0.0.1:4566"
	labURI := store.LabRegistryHost + "/000000000001/lambda-lab:v1"
	pullRef, useAuth := compute.ResolveLabImagePull(labURI, listen)
	if !useAuth {
		t.Fatal("lab registry rewrite must require auth")
	}
	want := compute.DinDPullHost(listen) + "/000000000001/lambda-lab:v1"
	if pullRef != want {
		t.Fatalf("pullRef=%q want %q", pullRef, want)
	}

	already := want
	pullRef, useAuth = compute.ResolveLabImagePull(already, listen)
	if !useAuth || pullRef != already {
		t.Fatalf("DinD lab host ref must require auth pullRef=%q useAuth=%v", pullRef, useAuth)
	}

	_, useAuth = compute.ResolveLabImagePull("host.docker.internal:9999/000000000001/repo:tag", listen)
	if useAuth {
		t.Fatal("wrong-port host.docker.internal must not set useAuth")
	}
}

func TestAllowImagePullRejectsAttackerRegistry(t *testing.T) {
	t.Parallel()
	err := compute.AllowImagePull("evil.registry.example/malware:latest", "")
	if err == nil {
		t.Fatal("expected reject")
	}
	if !strings.Contains(err.Error(), "not allowlisted") {
		t.Fatalf("error=%v", err)
	}
	err = compute.AllowImagePull("169.254.169.254/latest/meta:1", "")
	if err == nil {
		t.Fatal("expected reject metadata-style host")
	}
}

func TestAllowImagePullExtraAllowlistRequiresDigest(t *testing.T) {
	t.Setenv(compute.EnvImagePullAllowlist, "ghcr.io/kyaxris-labs/")
	err := compute.AllowImagePull("ghcr.io/kyaxris-labs/tool:latest", "")
	if err == nil || !strings.Contains(err.Error(), "digest") {
		t.Fatalf("expected digest requirement, got %v", err)
	}
	if err := compute.AllowImagePull("ghcr.io/kyaxris-labs/tool@sha256:"+strings.Repeat("a", 64), ""); err != nil {
		t.Fatal(err)
	}
}

func TestValidateECSRunOptsRejectsForeignImage(t *testing.T) {
	t.Parallel()
	err := compute.ValidateECSRunOpts(compute.ECSRunOpts{ImageURI: "attacker.example/x:1", ListenAddr: "127.0.0.1:4566"})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestValidateECSRunOptsPinsDinDListen(t *testing.T) {
	t.Parallel()
	opts := compute.ECSRunOpts{
		ImageURI:   "host.docker.internal:4566/000000000001/repo:tag",
		ListenAddr: "0.0.0.0:8080",
	}
	if err := compute.ValidateECSRunOpts(opts); err == nil {
		t.Fatal("expected deny when ListenAddr port differs from image host")
	}
	opts.ListenAddr = "127.0.0.1:4566"
	if err := compute.ValidateECSRunOpts(opts); err != nil {
		t.Fatal(err)
	}
}

func TestAllowImagePullRejectsUnpinnedLambdaTagVariant(t *testing.T) {
	t.Parallel()
	err := compute.AllowImagePull("public.ecr.aws/lambda/python:3.12-v2", "")
	if err == nil {
		t.Fatal("expected reject for unpinned tag variant")
	}
}
