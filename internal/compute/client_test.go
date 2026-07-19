package compute_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/Kyaxris-Labs/Noctaxris/internal/compute"
	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

func TestNewClientEmptyHost(t *testing.T) {
	_, err := compute.NewClient("", "")
	if err == nil {
		t.Fatal("expected error for empty docker host")
	}
}

func TestNewClientPlainTCP(t *testing.T) {
	cli, err := compute.NewClient("tcp://127.0.0.1:1", "")
	if err != nil {
		t.Fatal(err)
	}
	cli.Close()
}

func TestNewClientInvalidTLSCertPath(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"ca.pem", "cert.pem", "key.pem"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("invalid"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	_, err := compute.NewClient("tcp://127.0.0.1:1", dir)
	if err == nil {
		t.Fatal("expected error for invalid TLS PEMs")
	}
}

func TestValidateRunOpts(t *testing.T) {
	t.Run("rejects empty Runtime", func(t *testing.T) {
		err := compute.ValidateRunOpts(compute.RunOpts{
			CodeHostPath: "/var/lib/noctaxris/fn",
			Handler:      "main.handler",
		})
		if err == nil {
			t.Fatal("expected error")
		}
	})
	t.Run("rejects empty CodeHostPath", func(t *testing.T) {
		err := compute.ValidateRunOpts(compute.RunOpts{
			Runtime: store.LambdaRuntimePython312,
			Handler: "main.handler",
		})
		if err == nil {
			t.Fatal("expected error")
		}
	})
	t.Run("rejects bad Handler", func(t *testing.T) {
		err := compute.ValidateRunOpts(compute.RunOpts{
			Runtime:      store.LambdaRuntimePython312,
			CodeHostPath: "/var/lib/noctaxris/fn",
			Handler:      "nope",
		})
		if err == nil {
			t.Fatal("expected error")
		}
	})
	t.Run("rejects relative CodeHostPath", func(t *testing.T) {
		err := compute.ValidateRunOpts(compute.RunOpts{
			Runtime:      store.LambdaRuntimePython312,
			CodeHostPath: "relative/path",
			Handler:      "main.handler",
		})
		if err == nil {
			t.Fatal("expected error")
		}
	})
	t.Run("accepts minimal opts", func(t *testing.T) {
		err := compute.ValidateRunOpts(compute.RunOpts{
			Runtime:      store.LambdaRuntimePython312,
			CodeHostPath: "/var/lib/noctaxris/fn",
			Handler:      "main.handler",
		})
		if err != nil {
			t.Fatal(err)
		}
	})
}

func TestValidateImageRunOpts(t *testing.T) {
	t.Run("rejects empty ImageURI", func(t *testing.T) {
		err := compute.ValidateImageRunOpts(compute.ImageRunOpts{
			EventHostPath: "/var/lib/noctaxris/events",
			Handler:       "main.handler",
		})
		if err == nil {
			t.Fatal("expected error")
		}
	})
	t.Run("accepts minimal opts", func(t *testing.T) {
		err := compute.ValidateImageRunOpts(compute.ImageRunOpts{
			ImageURI:      "public.ecr.aws/lambda/python:3.12",
			EventHostPath: "/var/lib/noctaxris/events",
			Handler:       "main.handler",
		})
		if err != nil {
			t.Fatal(err)
		}
	})
}

func TestPingSkipsWithoutEngine(t *testing.T) {
	host := os.Getenv("NOCTAXRIS_DOCKER_HOST")
	if host == "" {
		t.Skip("NOCTAXRIS_DOCKER_HOST unset")
	}
	certPath := os.Getenv("NOCTAXRIS_DOCKER_CERT_PATH")
	cli, err := compute.NewClient(host, certPath)
	if err != nil {
		t.Fatal(err)
	}
	defer cli.Close()
	if err := cli.Ping(context.Background()); err != nil {
		t.Skipf("engine not reachable: %v", err)
	}
}
