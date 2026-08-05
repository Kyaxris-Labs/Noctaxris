package server_test

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/Kyaxris-Labs/Noctaxris/internal/config"
)

func TestSecretsRotationTickerRotatesDueSecret(t *testing.T) {
	srv, _, _ := newTestServerStoreWith(t, func(cfg *config.Config) {
		cfg.LabForensics = true
	})
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	srv.StartSecretsRotationTicker()
	t.Cleanup(func() { srv.StopSecretsRotationTicker() })

	create := mustSecretsJSON(t, handler, "CreateSecret", map[string]any{
		"Name": "ticker-cov", "SecretString": "before-rotate",
	}, now)
	if create.Code != http.StatusOK {
		t.Fatalf("CreateSecret %d %s", create.Code, create.Body.String())
	}

	rot := mustSecretsJSON(t, handler, "RotateSecret", map[string]any{
		"SecretId": "ticker-cov", "RotateImmediately": false,
		"RotationRules": map[string]any{"AutomaticallyAfterDays": 1},
	}, now)
	if rot.Code != http.StatusOK {
		t.Fatalf("RotateSecret %d %s", rot.Code, rot.Body.String())
	}

	due := now.Add(48 * time.Hour)
	clock := mustLabForensicsJSON(t, handler, "NoctaxrisLab.SetClock", map[string]any{
		"FixedTime": due.Format(time.RFC3339),
	}, now)
	if clock.Code != http.StatusOK {
		t.Fatalf("SetClock %d %s", clock.Code, clock.Body.String())
	}

	time.Sleep(6 * time.Second)

	get := mustSecretsJSON(t, handler, "GetSecretValue", map[string]any{"SecretId": "ticker-cov"}, now)
	if get.Code != http.StatusOK {
		t.Fatalf("GetSecretValue %d", get.Code)
	}
	var val map[string]any
	_ = json.Unmarshal(get.Body.Bytes(), &val)
	if val["SecretString"] == "before-rotate" {
		t.Fatal("expected ticker to rotate secret value")
	}
}
