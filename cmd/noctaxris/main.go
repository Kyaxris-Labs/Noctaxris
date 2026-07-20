package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/Kyaxris-Labs/Noctaxris/internal/config"
	"github.com/Kyaxris-Labs/Noctaxris/internal/kernel/audit"
	"github.com/Kyaxris-Labs/Noctaxris/internal/server"
	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
	"github.com/Kyaxris-Labs/Noctaxris/internal/validate"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "noctaxris: %v\n", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) > 0 && args[0] == "healthcheck" {
		return runHealthcheck()
	}

	cfg, err := config.LoadFromEnv()
	if err != nil {
		return err
	}

	if cfg.RootAccessKeyID == "" || cfg.RootSecretAccessKey == "" {
		return fmt.Errorf("set NOCTAXRIS_ROOT_ACCESS_KEY_ID and NOCTAXRIS_ROOT_SECRET_ACCESS_KEY")
	}

	if err := os.MkdirAll(cfg.DataRoot, 0o700); err != nil {
		return fmt.Errorf("create data root: %w", err)
	}

	masterKeyPath := cfg.MasterKeyPath
	if masterKeyPath == "" {
		masterKeyPath = filepath.Join(cfg.DataRoot, "master.key")
	}

	master, err := store.LoadOrCreateMasterKey(masterKeyPath)
	if err != nil {
		return fmt.Errorf("load master key: %w", err)
	}

	st, err := store.Open(cfg.DataRoot, master)
	if err != nil {
		return fmt.Errorf("open store: %w", err)
	}
	defer st.Close()

	if err := st.EnsureRoot(cfg.AccountID, cfg.RootAccessKeyID, cfg.RootSecretAccessKey); err != nil {
		return fmt.Errorf("ensure root: %w", err)
	}

	if err := seedIdPFromConfig(st, cfg); err != nil {
		return err
	}

	aud, err := audit.NewWriter(filepath.Join(cfg.DataRoot, "cloudtrail"))
	if err != nil {
		return fmt.Errorf("open audit writer: %w", err)
	}
	defer aud.Close()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	srv := server.New(cfg, st, aud)
	return srv.ListenAndServeContext(ctx)
}

// runHealthcheck probes readiness for Compose/distroless HEALTHCHECK (no curl).
func runHealthcheck() error {
	addr := strings.TrimSpace(os.Getenv("NOCTAXRIS_LISTEN"))
	if addr == "" {
		addr = "127.0.0.1:4566"
	}
	// Container listens on 0.0.0.0; probe loopback.
	if strings.HasPrefix(addr, "0.0.0.0:") {
		addr = "127.0.0.1:" + strings.TrimPrefix(addr, "0.0.0.0:")
	}
	url := "http://" + addr + "/_noctaxris/ready"
	client := &http.Client{Timeout: 3 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return fmt.Errorf("healthcheck: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("healthcheck: status %d", resp.StatusCode)
	}
	return nil
}

func seedIdPFromConfig(st *store.Store, cfg config.Config) error {
	if cfg.SAMLIdPMetadataPath != "" {
		path, err := validate.ReadableFilePath(cfg.SAMLIdPMetadataPath)
		if err != nil {
			return fmt.Errorf("NOCTAXRIS_SAML_IDP_METADATA: %w", err)
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("read SAML IdP metadata %s: %w", path, err)
		}
		name := cfg.SAMLIdPName
		if name == "" {
			name = "default"
		}
		if name != "default" {
			if err := validate.IAMName(name); err != nil {
				return fmt.Errorf("NOCTAXRIS_SAML_IDP_NAME: %w", err)
			}
		}
		if _, err := st.PutSAMLProvider(cfg.AccountID, name, string(raw)); err != nil {
			return fmt.Errorf("seed SAML IdP: %w", err)
		}
	}
	if cfg.OIDCIssuerURL != "" {
		if err := validate.OIDCIssuerURL(cfg.OIDCIssuerURL); err != nil {
			return fmt.Errorf("NOCTAXRIS_OIDC_ISSUER_URL: %w", err)
		}
		if err := validate.OIDCClientID(cfg.OIDCClientID); err != nil {
			return fmt.Errorf("NOCTAXRIS_OIDC_CLIENT_ID: %w", err)
		}
		if _, err := st.PutOIDCProvider(cfg.AccountID, cfg.OIDCIssuerURL, cfg.OIDCClientID); err != nil {
			return fmt.Errorf("seed OIDC IdP: %w", err)
		}
	}
	return nil
}
