package compute

import (
	"context"
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
	dir := writeMicroVMTestTLSCerts(t)
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

func TestNewFunctionInvokerMicroVMWSL2FailsClosed(t *testing.T) {
	_, err := NewFunctionInvoker(InvokerConfig{
		Runtime: RuntimeMicroVM,
		ProbeOverrides: &MicroVMProbeOpts{
			GOOS:        "linux",
			ProcVersion: "Linux version 6.6.0-microsoft-standard-WSL2",
			Stat: func(string) (os.FileInfo, error) {
				return fakeFileInfo{}, nil
			},
			LookPath: func(string) (string, error) {
				return "/usr/bin/firecracker", nil
			},
		},
	})
	if err == nil {
		t.Fatal("expected microVM WSL2 failure")
	}
	if !strings.Contains(err.Error(), "WSL2") {
		t.Fatalf("err=%v", err)
	}
	if strings.Contains(strings.ToLower(err.Error()), "docker.sock") {
		t.Fatalf("must not mention docker.sock fallthrough: %v", err)
	}
}

func TestMicroVMRunnerRejectsInvokeAfterForcedConstruct(t *testing.T) {
	// Simulate a runner that somehow skipped NewMicroVMRunner: probe still fails closed.
	r := &MicroVMRunner{ProbeOpts: MicroVMProbeOpts{
		GOOS:        "linux",
		ProcVersion: "Linux version 6.6.0-microsoft-standard-WSL2",
	}}
	_, err := r.RunInvoke(context.Background(), RunOpts{
		CodeHostPath: "/var/lib/noctaxris/lambda/code",
		Runtime:      "python3.12",
		Handler:      "handler.handler",
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "WSL2") {
		t.Fatalf("err=%v", err)
	}
}

func TestMicroVMRunnerNotImplementedWhenProbeOK(t *testing.T) {
	r, err := NewMicroVMRunner(MicroVMProbeOpts{
		GOOS:            "linux",
		ProcVersion:     "Linux version 6.1.0-generic",
		FirecrackerPath: "/opt/firecracker/firecracker",
		Stat: func(string) (os.FileInfo, error) {
			return fakeFileInfo{}, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = r.RunInvoke(context.Background(), RunOpts{
		CodeHostPath: "/var/lib/noctaxris/lambda/code",
		Runtime:      "python3.12",
		Handler:      "handler.handler",
	})
	if err == nil || !strings.Contains(err.Error(), "not implemented") {
		t.Fatalf("want not implemented, got %v", err)
	}
	_, err = r.RunImageInvoke(context.Background(), ImageRunOpts{
		ImageURI:      "public.ecr.aws/lambda/python:3.12",
		Handler:       "handler.handler",
		EventHostPath: "/var/lib/noctaxris/lambda/events",
	})
	if err == nil || !strings.Contains(err.Error(), "not implemented") {
		t.Fatalf("want not implemented image path, got %v", err)
	}
	_, err = r.RunECSTask(context.Background(), ECSRunOpts{ImageURI: "alpine:3.20"})
	if err == nil || !strings.Contains(err.Error(), "not implemented") {
		t.Fatalf("want not implemented ECS path, got %v", err)
	}
}

func writeMicroVMTestTLSCerts(t *testing.T) string {
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
