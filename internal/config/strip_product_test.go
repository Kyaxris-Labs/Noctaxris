package config_test

import (
	"testing"

	"github.com/Kyaxris-Labs/Noctaxris/internal/config"
)

func TestStripProductProbePaths(t *testing.T) {
	t.Parallel()

	def := config.Config{}
	if got := def.HealthPath(); got != "/_noctaxris/health" {
		t.Fatalf("default HealthPath=%q", got)
	}
	if got := def.ReadyPath(); got != "/_noctaxris/ready" {
		t.Fatalf("default ReadyPath=%q", got)
	}
	if got := def.VersionPath(); got != "/_noctaxris/version" {
		t.Fatalf("default VersionPath=%q", got)
	}
	if got := def.LabCertificateOrganization(); got != "Noctaxris Lab" {
		t.Fatalf("default LabCertificateOrganization=%q", got)
	}

	strip := config.Config{StripProduct: true}
	if got := strip.HealthPath(); got != "/_lab/health" {
		t.Fatalf("strip HealthPath=%q", got)
	}
	if got := strip.ReadyPath(); got != "/_lab/ready" {
		t.Fatalf("strip ReadyPath=%q", got)
	}
	if got := strip.VersionPath(); got != "/_lab/version" {
		t.Fatalf("strip VersionPath=%q", got)
	}
	if got := strip.LabCertificateOrganization(); got != "Lab" {
		t.Fatalf("strip LabCertificateOrganization=%q", got)
	}
}

func TestStripProductFromEnv(t *testing.T) {
	t.Setenv(config.EnvStripProduct, "")
	if config.StripProductFromEnv() {
		t.Fatal("unset should be false")
	}
	t.Setenv(config.EnvStripProduct, "0")
	if config.StripProductFromEnv() {
		t.Fatal("0 should be false")
	}
	t.Setenv(config.EnvStripProduct, "1")
	if !config.StripProductFromEnv() {
		t.Fatal("1 should be true")
	}
	t.Setenv(config.EnvStripProduct, "true")
	if !config.StripProductFromEnv() {
		t.Fatal("true should be true")
	}
}

func TestLoadFromEnvStripProduct(t *testing.T) {
	t.Setenv("NOCTAXRIS_LISTEN", "127.0.0.1:4566")
	t.Setenv("NOCTAXRIS_DATA_ROOT", t.TempDir())
	t.Setenv("NOCTAXRIS_ACCOUNT_ID", "000000000001")
	t.Setenv("NOCTAXRIS_ROOT_ACCESS_KEY_ID", "")
	t.Setenv("NOCTAXRIS_ROOT_SECRET_ACCESS_KEY", "")
	t.Setenv(config.EnvStripProduct, "1")

	cfg, err := config.LoadFromEnv()
	if err != nil {
		t.Fatalf("LoadFromEnv: %v", err)
	}
	if !cfg.StripProduct {
		t.Fatal("expected StripProduct true")
	}
}
