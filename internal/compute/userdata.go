package compute

import (
	"encoding/base64"
	"strings"
	"unicode/utf8"
)

// DecodeUserData turns RunInstances UserData into a shell script body.
// AWS clients send base64; plain text (e.g. lab tests) is accepted as-is.
// Empty input yields empty output with no error.
func DecodeUserData(raw string) string {
	if strings.TrimSpace(raw) == "" {
		return ""
	}
	if decoded, ok := tryBase64UserData(raw); ok {
		return decoded
	}
	return raw
}

func tryBase64UserData(raw string) (string, bool) {
	// Reject obvious plain scripts early so "echo hi" is not treated as base64.
	trimmed := strings.TrimLeft(raw, " \t")
	if strings.HasPrefix(trimmed, "#!") {
		return "", false
	}
	candidate := strings.TrimSpace(raw)
	enc := base64.StdEncoding
	if strings.ContainsAny(candidate, "-_") && !strings.ContainsAny(candidate, "+/") {
		enc = base64.URLEncoding
	}
	b, err := enc.DecodeString(candidate)
	if err != nil {
		// Try RawStdEncoding (no padding) as a fallback.
		b, err = base64.RawStdEncoding.DecodeString(candidate)
		if err != nil {
			return "", false
		}
	}
	if len(b) == 0 || !utf8.Valid(b) {
		return "", false
	}
	return string(b), true
}
