package store

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"strings"
	"time"
)

func newIAMResourceID(prefix string) (string, error) {
	var b [10]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return prefix + strings.ToUpper(hex.EncodeToString(b[:])), nil
}

func newAccessKeyID() (string, error) {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	// AKIA + 16 hex chars ≈ AWS long-lived key id shape (20 chars total).
	return "AKIA" + strings.ToUpper(hex.EncodeToString(b[:])), nil
}

func newAccessKeySecret() (string, error) {
	var b [30]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return base64.RawStdEncoding.EncodeToString(b[:]), nil
}

func formatRFC3339(t time.Time) string {
	return t.UTC().Format(time.RFC3339)
}

func nowRFC3339() string {
	return formatRFC3339(time.Now())
}
