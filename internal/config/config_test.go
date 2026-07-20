package config_test

import (
	"testing"

	"github.com/Kyaxris-Labs/Noctaxris/internal/config"
)

func TestLoadFromEnvDefaults(t *testing.T) {
	t.Setenv("NOCTAXRIS_LISTEN", "")
	t.Setenv("NOCTAXRIS_DATA_ROOT", "")

	cfg, err := config.LoadFromEnv()
	if err != nil {
		t.Fatal(err)
	}

	if cfg.ListenAddr != "127.0.0.1:4566" {
		t.Fatalf("ListenAddr = %q, want %q", cfg.ListenAddr, "127.0.0.1:4566")
	}
	if cfg.DataRoot != "/var/lib/noctaxris" {
		t.Fatalf("DataRoot = %q, want %q", cfg.DataRoot, "/var/lib/noctaxris")
	}
	if cfg.DockerHost != "" {
		t.Fatalf("DockerHost = %q, want empty (compute disabled)", cfg.DockerHost)
	}
	if cfg.ComputeRuntime != "dind" {
		t.Fatalf("ComputeRuntime = %q, want dind default", cfg.ComputeRuntime)
	}
}

func TestLoadFromEnvDockerHost(t *testing.T) {
	t.Setenv("NOCTAXRIS_DOCKER_HOST", "tcp://noctaxris-engine:2376")
	t.Setenv("NOCTAXRIS_DOCKER_CERT_PATH", "/certs/client")

	cfg, err := config.LoadFromEnv()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.DockerHost != "tcp://noctaxris-engine:2376" {
		t.Fatalf("DockerHost = %q, want %q", cfg.DockerHost, "tcp://noctaxris-engine:2376")
	}
	if cfg.DockerTLSCertPath != "/certs/client" {
		t.Fatalf("DockerTLSCertPath = %q, want %q", cfg.DockerTLSCertPath, "/certs/client")
	}
	if cfg.ComputeRuntime != "dind" {
		t.Fatalf("ComputeRuntime = %q, want dind default", cfg.ComputeRuntime)
	}
}

func TestLoadFromEnvComputeRuntimeMicroVM(t *testing.T) {
	t.Setenv("NOCTAXRIS_COMPUTE_RUNTIME", "microvm")
	t.Setenv("NOCTAXRIS_FIRECRACKER_BIN", "/opt/firecracker/firecracker")

	cfg, err := config.LoadFromEnv()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.ComputeRuntime != "microvm" {
		t.Fatalf("ComputeRuntime = %q, want microvm", cfg.ComputeRuntime)
	}
	if cfg.FirecrackerBin != "/opt/firecracker/firecracker" {
		t.Fatalf("FirecrackerBin = %q", cfg.FirecrackerBin)
	}
}

func TestLoadFromEnvComputeRuntimeUnknown(t *testing.T) {
	t.Setenv("NOCTAXRIS_COMPUTE_RUNTIME", "host")
	_, err := config.LoadFromEnv()
	if err == nil {
		t.Fatal("expected unknown runtime error")
	}
}
