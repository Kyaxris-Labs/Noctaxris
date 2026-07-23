package compute

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
)

func TestNewFunctionInvokerDefaultDinD(t *testing.T) {
	dir := writeInvokerTestTLSCerts(t)
	t.Setenv(EnvDockerHostAllowlist, "tcp://127.0.0.1:1")
	inv, err := NewFunctionInvoker(InvokerConfig{
		Runtime:           "",
		DockerHost:        "tcp://127.0.0.1:1",
		DockerTLSCertPath: dir,
	})
	if err != nil {
		t.Fatal(err)
	}
	cli, ok := inv.(*Client)
	if !ok || cli == nil {
		t.Fatalf("got %T, want *Client", inv)
	}
	_ = cli.Close()
}

func TestNewFunctionInvokerUnknownFailsClosed(t *testing.T) {
	_, err := NewFunctionInvoker(InvokerConfig{Runtime: "host-docker"})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "unknown") {
		t.Fatalf("err=%v", err)
	}
}

func TestNewFunctionInvokerRejectsMicroVM(t *testing.T) {
	_, err := NewFunctionInvoker(InvokerConfig{Runtime: "microvm"})
	if err == nil {
		t.Fatal("expected microvm rejection")
	}
	if !strings.Contains(err.Error(), "unknown") {
		t.Fatalf("err=%v", err)
	}
}

func writeInvokerTestTLSCerts(t *testing.T) string {
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
