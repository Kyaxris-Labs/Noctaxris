package main

import (
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Kyaxris-Labs/Noctaxris/internal/config"
	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
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

func TestRunHealthcheck_successAndZeroHostRewrite(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/_noctaxris/ready" {
			http.NotFound(w, r)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)

	host := strings.TrimPrefix(srv.URL, "http://")
	t.Setenv("NOCTAXRIS_LISTEN", host)
	if err := run([]string{"healthcheck"}); err != nil {
		t.Fatalf("healthcheck success: %v", err)
	}

	// Rewrite 0.0.0.0:port to 127.0.0.1:port — use real loopback listener address port.
	_, port, err := net.SplitHostPort(host)
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("NOCTAXRIS_LISTEN", "0.0.0.0:"+port)
	// httptest only listens on the assigned host; 127.0.0.1:port of the same server works when host is 127.0.0.1.
	if strings.HasPrefix(host, "127.0.0.1:") {
		if err := run([]string{"healthcheck"}); err != nil {
			t.Fatalf("0.0.0.0 rewrite healthcheck: %v", err)
		}
	}
}

func TestRunHealthcheck_nonOKStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	t.Cleanup(srv.Close)
	t.Setenv("NOCTAXRIS_LISTEN", strings.TrimPrefix(srv.URL, "http://"))
	err := run([]string{"healthcheck"})
	if err == nil || !strings.Contains(err.Error(), "status") {
		t.Fatalf("want status error, got %v", err)
	}
}

func TestRunHealthcheck_defaultListen(t *testing.T) {
	t.Setenv("NOCTAXRIS_LISTEN", "")
	err := run([]string{"healthcheck"})
	if err == nil {
		t.Fatal("expected failure against default listen with nothing up")
	}
}

func TestSeedIdPFromConfig_OIDCAndSAML(t *testing.T) {
	root := t.TempDir()
	metaPath := filepath.Join(root, "idp.xml")
	meta := `<?xml version="1.0"?><EntityDescriptor entityID="https://idp.example/entity" xmlns="urn:oasis:names:tc:SAML:2.0:metadata"></EntityDescriptor>`
	if err := os.WriteFile(metaPath, []byte(meta), 0o600); err != nil {
		t.Fatal(err)
	}

	masterPath := filepath.Join(root, "master.key")
	master, err := store.LoadOrCreateMasterKey(masterPath)
	if err != nil {
		t.Fatal(err)
	}
	st, err := store.Open(root, master)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	cfg := config.Config{
		AccountID:           "000000000001",
		SAMLIdPMetadataPath: metaPath,
		SAMLIdPName:         "default",
		OIDCIssuerURL:       "https://token.actions.githubusercontent.com",
		OIDCClientID:        "sts.amazonaws.com",
	}
	if err := seedIdPFromConfig(st, cfg); err != nil {
		t.Fatal(err)
	}

	cfg.SAMLIdPName = "LabIdP"
	if err := seedIdPFromConfig(st, cfg); err != nil {
		t.Fatal(err)
	}

	cfg.SAMLIdPName = "../evil"
	if err := seedIdPFromConfig(st, cfg); err == nil {
		t.Fatal("expected bad SAML name reject")
	}

	cfg.SAMLIdPMetadataPath = ""
	cfg.SAMLIdPName = "default"
	cfg.OIDCIssuerURL = "ftp://bad"
	if err := seedIdPFromConfig(st, cfg); err == nil {
		t.Fatal("expected bad OIDC issuer reject")
	}

	cfg.OIDCIssuerURL = "https://token.actions.githubusercontent.com"
	cfg.OIDCClientID = ""
	if err := seedIdPFromConfig(st, cfg); err == nil {
		t.Fatal("expected empty OIDC client reject")
	}
}

func TestSeedIdPFromConfig_missingMetadataFile(t *testing.T) {
	root := t.TempDir()
	master, err := store.LoadOrCreateMasterKey(filepath.Join(root, "master.key"))
	if err != nil {
		t.Fatal(err)
	}
	st, err := store.Open(root, master)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	cfg := config.Config{
		AccountID:           "000000000001",
		SAMLIdPMetadataPath: filepath.Join(root, "missing.xml"),
	}
	if err := seedIdPFromConfig(st, cfg); err == nil {
		t.Fatal("expected missing metadata error")
	}
}

func TestRun_seedsOIDCAndStartsThenBindFails(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := ln.Addr().String()
	defer ln.Close()

	root := t.TempDir()
	t.Setenv("NOCTAXRIS_ROOT_ACCESS_KEY_ID", "AKIAROOTEXAMPLE01")
	t.Setenv("NOCTAXRIS_ROOT_SECRET_ACCESS_KEY", "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY")
	t.Setenv("NOCTAXRIS_ACCOUNT_ID", "000000000001")
	t.Setenv("NOCTAXRIS_DATA_ROOT", root)
	t.Setenv("NOCTAXRIS_DOCKER_HOST", "")
	t.Setenv("NOCTAXRIS_DOCKER_CERT_PATH", "")
	t.Setenv("NOCTAXRIS_COMPUTE_RUNTIME", "")
	t.Setenv("NOCTAXRIS_LISTEN", addr)
	t.Setenv("NOCTAXRIS_OIDC_ISSUER_URL", "https://token.actions.githubusercontent.com")
	t.Setenv("NOCTAXRIS_OIDC_CLIENT_ID", "sts.amazonaws.com")
	t.Setenv("NOCTAXRIS_SAML_IDP_METADATA", "")
	t.Setenv("NOCTAXRIS_ALLOW_NONLOOPBACK_LISTEN", "")

	err = run(nil)
	if err == nil {
		t.Fatal("expected bind failure after seeding")
	}
	if strings.Contains(err.Error(), "OIDC") {
		t.Fatalf("OIDC seed should succeed before listen fail: %v", err)
	}
}
