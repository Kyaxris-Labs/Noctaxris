package server

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestParseRegistryDigestRejectsMaliciousInput(t *testing.T) {
	valid := "sha256:" + strings.Repeat("a", 64)
	cases := []struct {
		name   string
		digest string
	}{
		{name: "dotdot hex", digest: "sha256:.."},
		{name: "dotdot in algo", digest: "sha..256:" + strings.Repeat("a", 64)},
		{name: "path separator", digest: "sha256:../escape"},
		{name: "backslash", digest: `sha256:\escape`},
		{name: "wrong length", digest: "sha256:abc"},
		{name: "non-hex", digest: "sha256:" + strings.Repeat("g", 64)},
		{name: "unsupported algo", digest: "sha1:" + strings.Repeat("a", 40)},
		{name: "missing hex", digest: "sha256:"},
		{name: "missing algo", digest: ":abcdef"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, _, err := parseRegistryDigest(tc.digest); err == nil {
				t.Fatalf("parseRegistryDigest(%q) expected error", tc.digest)
			}
		})
	}
	if _, _, err := parseRegistryDigest(valid); err != nil {
		t.Fatalf("parseRegistryDigest(valid) unexpected error: %v", err)
	}
}

func TestRegistryBlobPathStaysUnderDataRoot(t *testing.T) {
	dataRoot := t.TempDir()
	valid := "sha256:" + strings.Repeat("b", 64)
	path, err := registryBlobPath(dataRoot, valid)
	if err != nil {
		t.Fatal(err)
	}
	wantRoot := filepath.Join(dataRoot, "ecr", "blobs")
	if err := ensurePathWithinRoot(wantRoot, path); err != nil {
		t.Fatalf("valid digest escaped blob root: %v", err)
	}

	_, err = registryBlobPath(dataRoot, "sha256:..")
	if err == nil {
		t.Fatal("expected malicious digest to be rejected")
	}
}
