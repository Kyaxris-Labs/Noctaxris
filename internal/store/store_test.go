package store_test

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

func TestEnsureRootEncrypted(t *testing.T) {
	dir := t.TempDir()
	key, err := store.LoadOrCreateMasterKey(filepath.Join(dir, "master.key"))
	if err != nil {
		t.Fatal(err)
	}
	st, err := store.Open(dir, key)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	if err := st.EnsureRoot("000000000001", "AKIAROOTEXAMPLE01", "secret-root-value"); err != nil {
		t.Fatal(err)
	}
	acc, sec, root, err := st.LookupAccessKey("AKIAROOTEXAMPLE01")
	if err != nil || !root || acc != "000000000001" || sec != "secret-root-value" {
		t.Fatalf("acc=%s root=%v sec=%q err=%v", acc, root, sec, err)
	}
	raw, _ := os.ReadFile(filepath.Join(dir, "state.db"))
	if bytes.Contains(raw, []byte("secret-root-value")) {
		t.Fatal("plaintext secret found in db file")
	}
}
