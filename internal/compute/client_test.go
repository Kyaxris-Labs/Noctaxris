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
	_, err := compute.NewClient("", "", "")
	if err == nil {
		t.Fatal("expected error for empty docker host")
	}
}

func TestNewClientRejectsWithoutTLS(t *testing.T) {
	t.Setenv(compute.EnvDockerHostAllowlist, "tcp://127.0.0.1:1")
	_, err := compute.NewClient("tcp://127.0.0.1:1", "", "")
	if err == nil {
		t.Fatal("expected error when TLS cert path is empty")
	}
}

func TestNewClientInvalidTLSCertPath(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"ca.pem", "cert.pem", "key.pem"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("invalid"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv(compute.EnvDockerHostAllowlist, "tcp://127.0.0.1:1")
	_, err := compute.NewClient("tcp://127.0.0.1:1", dir, "")
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
	t.Run("accepts Java Class::method handler", func(t *testing.T) {
		err := compute.ValidateRunOpts(compute.RunOpts{
			Runtime:      store.LambdaRuntimeJava21,
			CodeHostPath: "/var/lib/noctaxris/fn",
			Handler:      "example.Echo::handleRequest",
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
	t.Run("accepts empty Handler for pinned Image base", func(t *testing.T) {
		err := compute.ValidateImageRunOpts(compute.ImageRunOpts{
			ImageURI:      "public.ecr.aws/lambda/python:3.12",
			EventHostPath: "/var/lib/noctaxris/events",
		})
		if err != nil {
			t.Fatal(err)
		}
	})
	t.Run("rejects empty Handler without one-shot or default entrypoint", func(t *testing.T) {
		err := compute.ValidateImageRunOpts(compute.ImageRunOpts{
			ImageURI:      "host.docker.internal:4566/000000000001/repo:tag",
			EventHostPath: "/var/lib/noctaxris/events",
			ListenAddr:    "127.0.0.1:4566",
		})
		if err == nil {
			t.Fatal("expected error")
		}
	})
	t.Run("accepts empty Handler with AllowDefaultEntrypoint", func(t *testing.T) {
		err := compute.ValidateImageRunOpts(compute.ImageRunOpts{
			ImageURI:               "host.docker.internal:4566/000000000001/repo:tag",
			EventHostPath:          "/var/lib/noctaxris/events",
			ListenAddr:             "127.0.0.1:4566",
			AllowDefaultEntrypoint: true,
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
	cli, err := compute.NewClient(host, certPath, "127.0.0.1:4566")
	if err != nil {
		t.Fatal(err)
	}
	defer cli.Close()
	if err := cli.Ping(context.Background()); err != nil {
		t.Skipf("engine not reachable: %v", err)
	}
}
