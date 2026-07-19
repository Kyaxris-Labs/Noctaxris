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
}
