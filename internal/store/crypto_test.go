package store_test

import (
	"bytes"
	"path/filepath"
	"testing"

	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

func TestSealOpenRoundTrip(t *testing.T) {
	dir := t.TempDir()
	key, err := store.LoadOrCreateMasterKey(filepath.Join(dir, "master.key"))
	if err != nil {
		t.Fatal(err)
	}
	ct, err := store.Seal(key, []byte("wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY"))
	if err != nil {
		t.Fatal(err)
	}
	pt, err := store.Unseal(key, ct)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(pt, []byte("wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY")) {
		t.Fatalf("got %q", pt)
	}
}
