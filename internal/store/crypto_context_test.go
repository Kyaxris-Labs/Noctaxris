package store_test

import (
	"bytes"
	"testing"

	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

func TestEncryptUnderCMKEncryptionContextAAD(t *testing.T) {
	cmk := bytes.Repeat([]byte{0x42}, 32)
	keyID := "key-lab-1"
	plain := []byte("bound-plaintext")
	ctx := map[string]string{"purpose": "lab", "owner": "alice"}

	blob, err := store.EncryptUnderCMK(cmk, keyID, plain, ctx)
	if err != nil {
		t.Fatal(err)
	}
	got, err := store.DecryptUnderCMK(cmk, blob, ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, plain) {
		t.Fatalf("plaintext mismatch: %q", got)
	}
	if _, err := store.DecryptUnderCMK(cmk, blob, map[string]string{"purpose": "other"}); err == nil {
		t.Fatal("expected decrypt failure for mismatched context")
	}
	if _, err := store.DecryptUnderCMK(cmk, blob, nil); err == nil {
		t.Fatal("expected decrypt failure for empty context")
	}

	// Empty context remains round-trippable with empty context (legacy blobs).
	legacy, err := store.EncryptUnderCMK(cmk, keyID, plain, nil)
	if err != nil {
		t.Fatal(err)
	}
	gotLegacy, err := store.DecryptUnderCMK(cmk, legacy, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(gotLegacy, plain) {
		t.Fatalf("legacy plaintext mismatch: %q", gotLegacy)
	}
}
