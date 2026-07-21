package sts_test

import (
	"testing"
	"time"

	"github.com/Kyaxris-Labs/Noctaxris/internal/kernel/sts"
)

func TestValidateLabEnrollmentCodesConsecutive(t *testing.T) {
	seed := []byte("lab-mfa-seed")
	at := time.Unix(1_700_000_000, 0).UTC()
	code1 := sts.LabTokenCode(seed, at)
	code2 := sts.LabTokenCode(seed, at.Add(time.Minute))
	if !sts.ValidateLabEnrollmentCodes(seed, code1, code2, at) {
		t.Fatal("expected consecutive codes to validate")
	}
	if sts.ValidateLabEnrollmentCodes(seed, code1, code1, at) {
		t.Fatal("same code twice must fail")
	}
	if sts.ValidateLabEnrollmentCodes(seed, code1, "000000", at) {
		t.Fatal("unrelated code2 must fail")
	}
	if sts.ValidateLabEnrollmentCodes(seed, "", code2, at) {
		t.Fatal("empty code1 must fail")
	}
}
