package sts

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"time"
)

// LabTokenCode returns the lab MFA token for seed at the given time.
// Rule: first 6 hex chars of sha256(seed + ":" + unixMinute).
// This is intentionally not RFC 6238 TOTP; it is deterministic for tests.
func LabTokenCode(seed []byte, at time.Time) string {
	unixMinute := at.Unix() / 60
	sum := sha256.Sum256([]byte(fmt.Sprintf("%s:%d", seed, unixMinute)))
	return hex.EncodeToString(sum[:])[:6]
}

// ValidateLabTokenCode reports whether tokenCode matches the lab rule for seed
// at at, or within skewMinutes of adjacent minute windows (inclusive).
func ValidateLabTokenCode(seed []byte, tokenCode string, at time.Time, skewMinutes int) bool {
	if tokenCode == "" || len(seed) == 0 {
		return false
	}
	if skewMinutes < 0 {
		skewMinutes = 0
	}
	for d := -skewMinutes; d <= skewMinutes; d++ {
		t := at.Add(time.Duration(d) * time.Minute)
		if LabTokenCode(seed, t) == tokenCode {
			return true
		}
	}
	return false
}
