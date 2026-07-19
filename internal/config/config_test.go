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
}
