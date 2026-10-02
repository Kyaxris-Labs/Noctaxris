package identity_test

import (
	"testing"

	"github.com/Kyaxris-Labs/Noctaxris/internal/kernel/identity"
)

func TestAnonymousPrincipalARNEmpty(t *testing.T) {
	p := identity.AnonymousPrincipal("000000000001")
	if p.Kind != identity.KindAnonymous {
		t.Fatalf("Kind=%q", p.Kind)
	}
	if p.AccountID != "000000000001" {
		t.Fatalf("AccountID=%q", p.AccountID)
	}
	if got := p.ARN(); got != "" {
		t.Fatalf("Anonymous ARN=%q want empty", got)
	}
}

func TestFederatedProviderPrincipalDefaultsAndARN(t *testing.T) {
	p := identity.FederatedProviderPrincipal(
		"123456789012",
		"arn:aws:iam::123456789012:oidc-provider/token.actions.githubusercontent.com",
		"",
		"ASIAFEDKEY000001",
	)
	if p.Kind != identity.KindFederated {
		t.Fatalf("Kind=%q", p.Kind)
	}
	if p.SessionName != "federated" {
		t.Fatalf("default SessionName=%q", p.SessionName)
	}
	if p.FederatedProviderARN == "" {
		t.Fatal("FederatedProviderARN empty")
	}
	want := "arn:aws:sts::123456789012:federated-user/federated"
	if got := p.ARN(); got != want {
		t.Fatalf("ARN()=%q want %q", got, want)
	}

	named := identity.FederatedProviderPrincipal(
		"123456789012",
		"arn:aws:iam::123456789012:oidc-provider/example.com",
		"gha-run",
		"ASIAFEDKEY000002",
	)
	wantNamed := "arn:aws:sts::123456789012:federated-user/gha-run"
	if got := named.ARN(); got != wantNamed {
		t.Fatalf("ARN()=%q want %q", got, wantNamed)
	}
}
