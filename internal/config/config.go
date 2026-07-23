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
	// ComputeRuntime is dind (default). Only nested DinD is supported (NOCTAXRIS_COMPUTE_RUNTIME).
	ComputeRuntime string
	// LambdaEndpointURL is injected into nested functions as AWS_ENDPOINT_URL.
	// Defaults empty; Compose sets http://host.docker.internal:4566.
	LambdaEndpointURL string
	// Optional IdP bootstrap for STS federation (fail-closed when unset).
	SAMLIdPMetadataPath string
	SAMLIdPName         string
	OIDCIssuerURL       string
	OIDCClientID        string
	// HTTPAPIAllowSetCookie permits Set-Cookie on HTTP API Lambda proxy responses
	// (NOCTAXRIS_HTTP_API_ALLOW_SET_COOKIE=1). Default strips Set-Cookie.
	HTTPAPIAllowSetCookie bool
	// FunctionURLCORSOrigins is a comma-separated allowlist for Function URL AuthType NONE
	// (NOCTAXRIS_FUNCTION_URL_CORS_ORIGINS). Empty keeps lab default Access-Control-Allow-Origin: *.
	FunctionURLCORSOrigins []string
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
		LambdaEndpointURL:   getenv("NOCTAXRIS_LAMBDA_ENDPOINT_URL", ""),
		SAMLIdPMetadataPath: getenv("NOCTAXRIS_SAML_IDP_METADATA", ""),
		SAMLIdPName:         getenv("NOCTAXRIS_SAML_IDP_NAME", "default"),
		OIDCIssuerURL:          getenv("NOCTAXRIS_OIDC_ISSUER_URL", ""),
		OIDCClientID:           getenv("NOCTAXRIS_OIDC_CLIENT_ID", ""),
		HTTPAPIAllowSetCookie:  strings.EqualFold(os.Getenv("NOCTAXRIS_HTTP_API_ALLOW_SET_COOKIE"), "1") ||
			strings.EqualFold(os.Getenv("NOCTAXRIS_HTTP_API_ALLOW_SET_COOKIE"), "true"),
		FunctionURLCORSOrigins: splitCSVEnv("NOCTAXRIS_FUNCTION_URL_CORS_ORIGINS"),
	}

	runtime, err := compute.ParseComputeRuntime(cfg.ComputeRuntime)
	if err != nil {
		return Config{}, fmt.Errorf("NOCTAXRIS_COMPUTE_RUNTIME: %w", err)
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
	if err := ValidateListenSecurity(cfg); err != nil {
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

func splitCSVEnv(key string) []string {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}
