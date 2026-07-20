package config

import (
	"fmt"
	"os"
	"strings"

	"github.com/Kyaxris-Labs/Noctaxris/internal/compute"
	"github.com/Kyaxris-Labs/Noctaxris/internal/validate"
)

type Config struct {
	ListenAddr          string
	DataRoot            string
	MasterKeyPath       string
	TLSCertFile         string
	TLSKeyFile          string
	RootAccessKeyID     string
	RootSecretAccessKey string
	AccountID           string
	// DockerHost is the nested engine endpoint (NOCTAXRIS_DOCKER_HOST).
	// Empty disables compute (unit tests / no DinD).
	DockerHost string
	// DockerTLSCertPath is the directory with ca.pem, cert.pem, key.pem for engine TLS.
	DockerTLSCertPath string
	// ComputeRuntime is dind (default) or microvm (NOCTAXRIS_COMPUTE_RUNTIME).
	ComputeRuntime string
	// FirecrackerBin is an optional Firecracker binary path (NOCTAXRIS_FIRECRACKER_BIN).
	FirecrackerBin string
	// LambdaEndpointURL is injected into nested functions as AWS_ENDPOINT_URL.
	// Defaults empty; Compose sets http://host.docker.internal:4566.
	LambdaEndpointURL string
	// Optional IdP bootstrap for STS federation (fail-closed when unset).
	SAMLIdPMetadataPath string
	SAMLIdPName         string
	OIDCIssuerURL       string
	OIDCClientID        string
}

func LoadFromEnv() (Config, error) {
	cfg := Config{
		ListenAddr:          getenv("NOCTAXRIS_LISTEN", "127.0.0.1:4566"),
		DataRoot:            getenv("NOCTAXRIS_DATA_ROOT", "/var/lib/noctaxris"),
		MasterKeyPath:       getenv("NOCTAXRIS_MASTER_KEY_FILE", ""),
		TLSCertFile:         getenv("NOCTAXRIS_TLS_CERT", ""),
		TLSKeyFile:          getenv("NOCTAXRIS_TLS_KEY", ""),
		RootAccessKeyID:     getenv("NOCTAXRIS_ROOT_ACCESS_KEY_ID", ""),
		RootSecretAccessKey: getenv("NOCTAXRIS_ROOT_SECRET_ACCESS_KEY", ""),
		AccountID:           getenv("NOCTAXRIS_ACCOUNT_ID", "000000000001"),
		DockerHost:          getenv("NOCTAXRIS_DOCKER_HOST", ""),
		DockerTLSCertPath:   getenv("NOCTAXRIS_DOCKER_CERT_PATH", ""),
		ComputeRuntime:      getenv("NOCTAXRIS_COMPUTE_RUNTIME", ""),
		FirecrackerBin:      getenv("NOCTAXRIS_FIRECRACKER_BIN", ""),
		LambdaEndpointURL:   getenv("NOCTAXRIS_LAMBDA_ENDPOINT_URL", ""),
		SAMLIdPMetadataPath: getenv("NOCTAXRIS_SAML_IDP_METADATA", ""),
		SAMLIdPName:         getenv("NOCTAXRIS_SAML_IDP_NAME", "default"),
		OIDCIssuerURL:       getenv("NOCTAXRIS_OIDC_ISSUER_URL", ""),
		OIDCClientID:        getenv("NOCTAXRIS_OIDC_CLIENT_ID", ""),
	}

	runtime, err := parseComputeRuntime(cfg.ComputeRuntime)
	if err != nil {
		return Config{}, err
	}
	cfg.ComputeRuntime = runtime

	if err := validate.AccountID(cfg.AccountID); err != nil {
		return Config{}, fmt.Errorf("NOCTAXRIS_ACCOUNT_ID: %w", err)
	}
	if cfg.RootAccessKeyID != "" {
		if err := validate.AccessKeyID(cfg.RootAccessKeyID); err != nil {
			return Config{}, fmt.Errorf("NOCTAXRIS_ROOT_ACCESS_KEY_ID: %w", err)
		}
	}
	if cfg.OIDCIssuerURL != "" {
		if err := validate.OIDCIssuerURL(cfg.OIDCIssuerURL); err != nil {
			return Config{}, fmt.Errorf("NOCTAXRIS_OIDC_ISSUER_URL: %w", err)
		}
		if err := validate.OIDCClientID(cfg.OIDCClientID); err != nil {
			return Config{}, fmt.Errorf("NOCTAXRIS_OIDC_CLIENT_ID: %w", err)
		}
	}
	if cfg.SAMLIdPName != "" && cfg.SAMLIdPName != "default" {
		if err := validate.IAMName(cfg.SAMLIdPName); err != nil {
			return Config{}, fmt.Errorf("NOCTAXRIS_SAML_IDP_NAME: %w", err)
		}
	}
	if err := compute.ValidateDockerHost(cfg.DockerHost, cfg.DockerTLSCertPath); err != nil {
		return Config{}, err
	}

	return cfg, nil
}

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func parseComputeRuntime(raw string) (string, error) {
	v := strings.ToLower(strings.TrimSpace(raw))
	switch v {
	case "", "dind":
		return "dind", nil
	case "microvm":
		return "microvm", nil
	default:
		return "", fmt.Errorf("NOCTAXRIS_COMPUTE_RUNTIME: unknown value %q (want dind or microvm)", raw)
	}
}
