package sts_test

import (
	"testing"
	"time"

	"github.com/Kyaxris-Labs/Noctaxris/internal/kernel/sts"
)

func TestLabTokenCodeDeterministic(t *testing.T) {
	seed := []byte("lab-seed")
	at := time.Unix(1_700_000_000, 0).UTC()
	code := sts.LabTokenCode(seed, at)
	if len(code) != 6 {
		t.Fatalf("code len=%d want 6: %q", len(code), code)
	}
	if got := sts.LabTokenCode(seed, at); got != code {
		t.Fatalf("not deterministic: %q vs %q", got, code)
	}
	if sts.LabTokenCode(seed, at.Add(time.Minute)) == code {
		t.Fatal("adjacent minute should differ")
	}
}

func TestValidateLabTokenCodeSkew(t *testing.T) {
	seed := []byte("lab-seed")
	at := time.Unix(1_700_000_060, 0).UTC()
	code := sts.LabTokenCode(seed, at.Add(-time.Minute))
	if !sts.ValidateLabTokenCode(seed, code, at, 1) {
		t.Fatal("expected skew=1 to accept previous minute")
	}
	if sts.ValidateLabTokenCode(seed, "000000", at, 1) {
		t.Fatal("invalid code accepted")
	}
}
