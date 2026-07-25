package config_test

import (
	"os"
	"path/filepath"
	"strings"
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
	dir := t.TempDir()
	for _, name := range []string{"ca.pem", "cert.pem", "key.pem"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("NOCTAXRIS_DOCKER_HOST", "tcp://noctaxris-engine:2376")
	t.Setenv("NOCTAXRIS_DOCKER_CERT_PATH", dir)

	cfg, err := config.LoadFromEnv()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.DockerHost != "tcp://noctaxris-engine:2376" {
		t.Fatalf("DockerHost = %q, want %q", cfg.DockerHost, "tcp://noctaxris-engine:2376")
	}
	if cfg.DockerTLSCertPath != dir {
		t.Fatalf("DockerTLSCertPath = %q, want %q", cfg.DockerTLSCertPath, dir)
	}
	if cfg.ComputeRuntime != "dind" {
		t.Fatalf("ComputeRuntime = %q, want dind default", cfg.ComputeRuntime)
	}
}

func TestLoadFromEnvDockerHostRejectsSock(t *testing.T) {
	t.Setenv("NOCTAXRIS_DOCKER_HOST", "unix:///var/run/docker.sock")
	t.Setenv("NOCTAXRIS_DOCKER_CERT_PATH", t.TempDir())
	_, err := config.LoadFromEnv()
	if err == nil {
		t.Fatal("expected error for docker.sock host")
	}
}

func TestLoadFromEnvDockerHostRequiresCertPath(t *testing.T) {
	t.Setenv("NOCTAXRIS_DOCKER_HOST", "tcp://noctaxris-engine:2376")
	t.Setenv("NOCTAXRIS_DOCKER_CERT_PATH", "")
	_, err := config.LoadFromEnv()
	if err == nil {
		t.Fatal("expected error when cert path missing")
	}
}

func TestLoadFromEnvComputeRuntimeRejectsMicroVM(t *testing.T) {
	t.Setenv("NOCTAXRIS_COMPUTE_RUNTIME", "microvm")
	_, err := config.LoadFromEnv()
	if err == nil {
		t.Fatal("expected microvm rejection")
	}
	if !strings.Contains(err.Error(), "NOCTAXRIS_COMPUTE_RUNTIME") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestLoadFromEnvComputeRuntimeUnknown(t *testing.T) {
	t.Setenv("NOCTAXRIS_COMPUTE_RUNTIME", "host")
	_, err := config.LoadFromEnv()
	if err == nil {
		t.Fatal("expected unknown runtime error")
	}
}

func TestLoadFromEnvCognitoInsecureCodes(t *testing.T) {
	t.Setenv(config.EnvCognitoInsecureCodes, "")
	cfg, err := config.LoadFromEnv()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.CognitoInsecureCodes {
		t.Fatal("CognitoInsecureCodes default want false")
	}
	t.Setenv(config.EnvCognitoInsecureCodes, "1")
	cfg, err = config.LoadFromEnv()
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.CognitoInsecureCodes {
		t.Fatal("CognitoInsecureCodes=1 want true")
	}
}
