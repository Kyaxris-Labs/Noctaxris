package store

import (
	"strings"
	"testing"
	"unicode"
)

func TestPrefixedAccessKeyIDsMatchAWSShape(t *testing.T) {
	ak, err := newAccessKeyID()
	if err != nil {
		t.Fatal(err)
	}
	asia, err := newTempAccessKeyID()
	if err != nil {
		t.Fatal(err)
	}
	assertAWSAccessKeyID(t, ak, "AKIA")
	assertAWSAccessKeyID(t, asia, "ASIA")
	if ak == asia {
		t.Fatal("long-lived and temporary ids collided")
	}
}

func assertAWSAccessKeyID(t *testing.T, id, prefix string) {
	t.Helper()
	if len(id) != 20 {
		t.Fatalf("len(%q)=%d, want 20", id, len(id))
	}
	if !strings.HasPrefix(id, prefix) {
		t.Fatalf("%q, want prefix %s", id, prefix)
	}
	for _, r := range id[len(prefix):] {
		if unicode.IsLower(r) {
			t.Fatalf("%q contains lowercase %q", id, r)
		}
		if (r < '0' || r > '9') && (r < 'A' || r > 'F') {
			t.Fatalf("%q has non-hex %q after prefix", id, r)
		}
	}
}
