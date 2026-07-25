package compute_test

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Kyaxris-Labs/Noctaxris/internal/compute"
)

func TestValidateDockerHostRejectsSockAndUnix(t *testing.T) {
	t.Parallel()
	cases := []string{
		"unix:///var/run/docker.sock",
		"npipe:////./pipe/docker_engine",
		"tcp://127.0.0.1:2375/var/run/docker.sock",
	}
	for _, host := range cases {
		err := compute.ValidateDockerHost(host, "/certs")
		if err == nil {
			t.Fatalf("expected reject for %q", host)
		}
	}
}

func TestValidateDockerHostRequiresAllowlistAndTLS(t *testing.T) {
	dir := writeTestTLSCerts(t)
	err := compute.ValidateDockerHost("tcp://evil-engine:2376", dir)
	if err == nil || !strings.Contains(err.Error(), "not allowlisted") {
		t.Fatalf("expected allowlist error, got %v", err)
	}
	err = compute.ValidateDockerHost("tcp://noctaxris-engine:2376", "")
	if err == nil || !strings.Contains(err.Error(), "DOCKER_CERT_PATH") {
		t.Fatalf("expected cert path error, got %v", err)
	}
	if err := compute.ValidateDockerHost("tcp://noctaxris-engine:2376", dir); err != nil {
		t.Fatal(err)
	}
	t.Setenv(compute.EnvDockerHostAllowlist, "tcp://127.0.0.1:2376")
	if err := compute.ValidateDockerHost("tcp://127.0.0.1:2376", dir); err != nil {
		t.Fatal(err)
	}
}

func TestNewClientRejectsUnixSock(t *testing.T) {
	_, err := compute.NewClient("unix:///var/run/docker.sock", t.TempDir(), "")
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "unix") && !strings.Contains(err.Error(), "docker.sock") {
		t.Fatalf("error=%v", err)
	}
}

func TestNewClientAllowlistedWithTLS(t *testing.T) {
	dir := writeTestTLSCerts(t)
	t.Setenv(compute.EnvDockerHostAllowlist, "tcp://127.0.0.1:1")
	cli, err := compute.NewClient("tcp://127.0.0.1:1", dir, "127.0.0.1:4566")
	if err != nil {
		t.Fatal(err)
	}
	_ = cli.Close()
}

func writeTestTLSCerts(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "noctaxris-test"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(24 * time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth},
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})
	for _, name := range []string{"ca.pem", "cert.pem"} {
		if err := os.WriteFile(filepath.Join(dir, name), certPEM, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(dir, "key.pem"), keyPEM, 0o600); err != nil {
		t.Fatal(err)
	}
	return dir
}
