package conditionkeys

//go:generate python genkeys.py

import (
	"strings"
)

// labExact adds OIDC claim keys used by AssumeRoleWithWebIdentity labs that are
// not always present in the SAR snapshot (GitHub Actions :sub / :aud).
var labExact = map[string]struct{}{
	"token.actions.githubusercontent.com:sub": {},
	"token.actions.githubusercontent.com:aud": {},
}

// Known reports whether key is in the generated catalog (exact or template match).
// Template catalog entries use ${TagKey} or <key> placeholders for the suffix.
func Known(key string) bool {
	if key == "" {
		return false
	}
	if _, ok := knownExact[key]; ok {
		return true
	}
	if _, ok := labExact[key]; ok {
		return true
	}
	for pattern := range knownExact {
		if templateMatch(pattern, key) {
			return true
		}
	}
	// Lab OIDC: {issuer-host-or-path}:sub / :aud for registered IdP shapes.
	if strings.HasSuffix(key, ":sub") || strings.HasSuffix(key, ":aud") {
		host := strings.TrimSuffix(strings.TrimSuffix(key, ":sub"), ":aud")
		if host != "" && (strings.Contains(host, ".") || strings.Contains(host, "/")) {
			return true
		}
	}
	return false
}

func templateMatch(pattern, key string) bool {
	const angle = "<key>"
	switch {
	case strings.Contains(pattern, "${"):
		// Match catalog templates such as ${TagKey} or ${EncryptionContextKey}.
		start := strings.Index(pattern, "${")
		end := strings.Index(pattern[start:], "}")
		if end < 0 {
			return false
		}
		prefix := pattern[:start]
		suffix := pattern[start+end+1:]
		if !strings.HasPrefix(key, prefix) || !strings.HasSuffix(key, suffix) {
			return false
		}
		mid := key[len(prefix) : len(key)-len(suffix)]
		return mid != ""
	case strings.Contains(pattern, angle):
		prefix := strings.Split(pattern, angle)[0]
		return strings.HasPrefix(key, prefix) && len(key) > len(prefix)
	default:
		return false
	}
}
