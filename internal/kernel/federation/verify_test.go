package federation

import (
	"testing"
	"time"
)

func TestValidateWebIdentityClaimsExpAndNBF(t *testing.T) {
	const issuer = "https://oidc.example/issuer"
	const client = "lab-client"
	now := time.Date(2026, 7, 21, 12, 0, 0, 0, time.UTC)

	if _, err := validateWebIdentityClaims(map[string]any{
		"iss": issuer,
		"sub": "user-1",
		"aud": client,
	}, issuer, client, now); err == nil {
		t.Fatal("expected reject for missing exp")
	}

	if _, err := validateWebIdentityClaims(map[string]any{
		"iss": issuer,
		"sub": "user-1",
		"aud": client,
		"exp": now.Add(-time.Hour).Unix(),
	}, issuer, client, now); err == nil {
		t.Fatal("expected reject for expired exp")
	}

	if _, err := validateWebIdentityClaims(map[string]any{
		"iss": issuer,
		"sub": "user-1",
		"aud": client,
		"exp": now.Add(time.Hour).Unix(),
		"nbf": now.Add(time.Hour).Unix(),
	}, issuer, client, now); err == nil {
		t.Fatal("expected reject for future nbf")
	}

	got, err := validateWebIdentityClaims(map[string]any{
		"iss": issuer,
		"sub": "user-1",
		"aud": client,
		"exp": now.Add(time.Hour).Unix(),
		"nbf": now.Add(-time.Minute).Unix(),
	}, issuer, client, now)
	if err != nil {
		t.Fatalf("validateWebIdentityClaims: %v", err)
	}
	if got.Subject != "user-1" || got.Issuer != issuer {
		t.Fatalf("got %+v", got)
	}
}
