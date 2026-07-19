package config

import (
	"fmt"
	"os"
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
	}

	if len(cfg.AccountID) != 12 {
		return Config{}, fmt.Errorf("account ID must be 12 characters, got %d", len(cfg.AccountID))
	}

	return cfg, nil
}

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
