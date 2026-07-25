package main

import (
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunFailClosedMissingRootKeys(t *testing.T) {
	t.Setenv("NOCTAXRIS_ROOT_ACCESS_KEY_ID", "")
	t.Setenv("NOCTAXRIS_ROOT_SECRET_ACCESS_KEY", "")
	t.Setenv("NOCTAXRIS_DATA_ROOT", t.TempDir())
	t.Setenv("NOCTAXRIS_DOCKER_HOST", "")
	t.Setenv("NOCTAXRIS_DOCKER_CERT_PATH", "")
	t.Setenv("NOCTAXRIS_COMPUTE_RUNTIME", "")
	t.Setenv("NOCTAXRIS_ACCOUNT_ID", "000000000001")

	err := run(nil)
	if err == nil {
		t.Fatal("expected error when root keys are unset")
	}
	if !strings.Contains(err.Error(), "NOCTAXRIS_ROOT_ACCESS_KEY_ID") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRunFailClosedInvalidAccountID(t *testing.T) {
	t.Setenv("NOCTAXRIS_ROOT_ACCESS_KEY_ID", "AKIAROOTEXAMPLE01")
	t.Setenv("NOCTAXRIS_ROOT_SECRET_ACCESS_KEY", "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY")
	t.Setenv("NOCTAXRIS_ACCOUNT_ID", "not-twelve")
	t.Setenv("NOCTAXRIS_DATA_ROOT", t.TempDir())
	t.Setenv("NOCTAXRIS_DOCKER_HOST", "")
	t.Setenv("NOCTAXRIS_DOCKER_CERT_PATH", "")
	t.Setenv("NOCTAXRIS_COMPUTE_RUNTIME", "")

	err := run(nil)
	if err == nil {
		t.Fatal("expected error for invalid account id")
	}
	if !strings.Contains(err.Error(), "NOCTAXRIS_ACCOUNT_ID") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRunFailClosedUnknownComputeRuntime(t *testing.T) {
	t.Setenv("NOCTAXRIS_ROOT_ACCESS_KEY_ID", "AKIAROOTEXAMPLE01")
	t.Setenv("NOCTAXRIS_ROOT_SECRET_ACCESS_KEY", "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY")
	t.Setenv("NOCTAXRIS_ACCOUNT_ID", "000000000001")
	t.Setenv("NOCTAXRIS_DATA_ROOT", t.TempDir())
	t.Setenv("NOCTAXRIS_DOCKER_HOST", "")
	t.Setenv("NOCTAXRIS_DOCKER_CERT_PATH", "")
	t.Setenv("NOCTAXRIS_COMPUTE_RUNTIME", "not-a-runtime")

	err := run(nil)
	if err == nil {
		t.Fatal("expected error for unknown compute runtime")
	}
	if !strings.Contains(err.Error(), "NOCTAXRIS_COMPUTE_RUNTIME") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRunFailClosedDockerHostWithoutCerts(t *testing.T) {
	t.Setenv("NOCTAXRIS_ROOT_ACCESS_KEY_ID", "AKIAROOTEXAMPLE01")
	t.Setenv("NOCTAXRIS_ROOT_SECRET_ACCESS_KEY", "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY")
	t.Setenv("NOCTAXRIS_ACCOUNT_ID", "000000000001")
	t.Setenv("NOCTAXRIS_DATA_ROOT", t.TempDir())
	t.Setenv("NOCTAXRIS_DOCKER_HOST", "tcp://noctaxris-engine:2376")
	t.Setenv("NOCTAXRIS_DOCKER_CERT_PATH", "")
	t.Setenv("NOCTAXRIS_COMPUTE_RUNTIME", "")

	err := run(nil)
	if err == nil {
		t.Fatal("expected error when Docker host is set without cert path")
	}
}

func TestRunFailClosedMissingSAMLMetadataFile(t *testing.T) {
	root := t.TempDir()
	t.Setenv("NOCTAXRIS_ROOT_ACCESS_KEY_ID", "AKIAROOTEXAMPLE01")
	t.Setenv("NOCTAXRIS_ROOT_SECRET_ACCESS_KEY", "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY")
	t.Setenv("NOCTAXRIS_ACCOUNT_ID", "000000000001")
	t.Setenv("NOCTAXRIS_DATA_ROOT", root)
	t.Setenv("NOCTAXRIS_DOCKER_HOST", "")
	t.Setenv("NOCTAXRIS_DOCKER_CERT_PATH", "")
	t.Setenv("NOCTAXRIS_COMPUTE_RUNTIME", "")
	t.Setenv("NOCTAXRIS_SAML_IDP_METADATA", filepath.Join(root, "missing-idp.xml"))
	t.Setenv("NOCTAXRIS_OIDC_ISSUER_URL", "")
	t.Setenv("NOCTAXRIS_OIDC_CLIENT_ID", "")

	err := run(nil)
	if err == nil {
		t.Fatal("expected error when SAML metadata path is missing")
	}
	if !strings.Contains(err.Error(), "SAML") && !strings.Contains(err.Error(), "NOCTAXRIS_SAML") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRunHealthcheckFailsWhenAPIDown(t *testing.T) {
	t.Setenv("NOCTAXRIS_LISTEN", "127.0.0.1:1")
	err := run([]string{"healthcheck"})
	if err == nil {
		t.Fatal("expected healthcheck failure when nothing listens")
	}
}

func TestRunRefusesExampleRootsOnNonLoopback(t *testing.T) {
	t.Setenv("NOCTAXRIS_ROOT_ACCESS_KEY_ID", "AKIAROOTEXAMPLE01")
	t.Setenv("NOCTAXRIS_ROOT_SECRET_ACCESS_KEY", "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY")
	t.Setenv("NOCTAXRIS_ACCOUNT_ID", "000000000001")
	t.Setenv("NOCTAXRIS_DATA_ROOT", t.TempDir())
	t.Setenv("NOCTAXRIS_DOCKER_HOST", "")
	t.Setenv("NOCTAXRIS_DOCKER_CERT_PATH", "")
	t.Setenv("NOCTAXRIS_COMPUTE_RUNTIME", "")
	t.Setenv("NOCTAXRIS_LISTEN", "0.0.0.0:4566")
	t.Setenv("NOCTAXRIS_ALLOW_NONLOOPBACK_LISTEN", "1")

	err := run(nil)
	if err == nil {
		t.Fatal("expected error for example roots on non-loopback listen")
	}
	if !strings.Contains(err.Error(), "example root") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRunAllowsExampleRootsOnLoopbackPastGate(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()
	addr := ln.Addr().String()

	t.Setenv("NOCTAXRIS_ROOT_ACCESS_KEY_ID", "AKIAROOTEXAMPLE01")
	t.Setenv("NOCTAXRIS_ROOT_SECRET_ACCESS_KEY", "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY")
	t.Setenv("NOCTAXRIS_ACCOUNT_ID", "000000000001")
	t.Setenv("NOCTAXRIS_DATA_ROOT", t.TempDir())
	t.Setenv("NOCTAXRIS_DOCKER_HOST", "")
	t.Setenv("NOCTAXRIS_DOCKER_CERT_PATH", "")
	t.Setenv("NOCTAXRIS_COMPUTE_RUNTIME", "")
	t.Setenv("NOCTAXRIS_LISTEN", addr)
	t.Setenv("NOCTAXRIS_ALLOW_NONLOOPBACK_LISTEN", "")

	err = run(nil)
	if err == nil {
		t.Fatal("expected listen bind failure after passing example-root gate")
	}
	if strings.Contains(err.Error(), "example root") {
		t.Fatalf("loopback must allow example roots past refuse gate, got: %v", err)
	}
}

func TestMainPackageDoesNotStartOnMissingKeys(t *testing.T) {
	// Guard against accidental env bleed from the parent process in CI.
	for _, k := range []string{
		"NOCTAXRIS_ROOT_ACCESS_KEY_ID",
		"NOCTAXRIS_ROOT_SECRET_ACCESS_KEY",
		"NOCTAXRIS_DOCKER_HOST",
		"NOCTAXRIS_DOCKER_CERT_PATH",
		"NOCTAXRIS_SAML_IDP_METADATA",
		"NOCTAXRIS_OIDC_ISSUER_URL",
		"NOCTAXRIS_OIDC_CLIENT_ID",
	} {
		_ = os.Unsetenv(k)
	}
	t.Setenv("NOCTAXRIS_DATA_ROOT", t.TempDir())
	err := run(nil)
	if err == nil {
		t.Fatal("expected fail-closed startup")
	}
}
