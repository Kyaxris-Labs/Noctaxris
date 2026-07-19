package conditionkeys

//go:generate python genkeys.py

import (
	"strings"
)

// Known reports whether key is in the generated catalog (exact or template match).
// Template catalog entries use ${TagKey} or <key> placeholders for the suffix.
func Known(key string) bool {
	if key == "" {
		return false
	}
	if _, ok := knownExact[key]; ok {
		return true
	}
	for pattern := range knownExact {
		if templateMatch(pattern, key) {
			return true
		}
	}
	return false
}

func templateMatch(pattern, key string) bool {
	const tag = "${TagKey}"
	const angle = "<key>"
	switch {
	case strings.Contains(pattern, tag):
		prefix := strings.Split(pattern, tag)[0]
		return strings.HasPrefix(key, prefix) && len(key) > len(prefix)
	case strings.Contains(pattern, angle):
		prefix := strings.Split(pattern, angle)[0]
		return strings.HasPrefix(key, prefix) && len(key) > len(prefix)
	default:
		return false
	}
}
