package identity_test

import (
	"testing"

	"github.com/Kyaxris-Labs/Noctaxris/internal/kernel/identity"
)

func TestRootPrincipalARN(t *testing.T) {
	p := identity.RootPrincipal("123456789012", "AKIAEXAMPLEKEYID01")

	if p.Kind != identity.KindRoot {
		t.Fatalf("Kind = %q, want %q", p.Kind, identity.KindRoot)
	}
	if !p.IsRoot {
		t.Fatal("IsRoot = false, want true")
	}
	if p.AccountID != "123456789012" {
		t.Fatalf("AccountID = %q, want %q", p.AccountID, "123456789012")
	}
	if p.AccessKeyID != "AKIAEXAMPLEKEYID01" {
		t.Fatalf("AccessKeyID = %q, want %q", p.AccessKeyID, "AKIAEXAMPLEKEYID01")
	}

	wantARN := "arn:aws:iam::123456789012:root"
	if got := p.ARN(); got != wantARN {
		t.Fatalf("ARN() = %q, want %q", got, wantARN)
	}
}

func TestNonRootARNEmpty(t *testing.T) {
	p := identity.Principal{
		Kind:        identity.KindUser,
		AccountID:   "123456789012",
		AccessKeyID: "AKIAEXAMPLEKEYID01",
		IsRoot:      false,
	}
	if got := p.ARN(); got != "" {
		t.Fatalf("ARN() = %q, want empty", got)
	}
}
