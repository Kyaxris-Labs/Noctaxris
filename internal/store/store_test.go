package store_test

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
	"time"

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
	ver, err := st.SchemaVersion()
	if err != nil || ver < 1 {
		t.Fatalf("schema version=%d err=%v", ver, err)
	}
	if err := st.Ping(); err != nil {
		t.Fatalf("ping: %v", err)
	}
}

func TestOpenEmptyStateTemplateProvidesServiceTables(t *testing.T) {
	// Warm the process template (first Open may pay full DDL).
	dir1 := t.TempDir()
	key1, err := store.LoadOrCreateMasterKey(filepath.Join(dir1, "master.key"))
	if err != nil {
		t.Fatal(err)
	}
	st1, err := store.Open(dir1, key1)
	if err != nil {
		t.Fatal(err)
	}
	if err := st1.Close(); err != nil {
		t.Fatal(err)
	}

	dir2 := t.TempDir()
	key2, err := store.LoadOrCreateMasterKey(filepath.Join(dir2, "master.key"))
	if err != nil {
		t.Fatal(err)
	}
	start := time.Now()
	st2, err := store.Open(dir2, key2)
	elapsed := time.Since(start)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st2.Close() })
	// Template copy + migrate should be far below a cold Ensure* bootstrap (~hundreds of ms).
	if elapsed > 2*time.Second {
		t.Fatalf("templated Open took %v; expected << cold bootstrap", elapsed)
	}
	if _, err := st2.CreateTopic("000000000001", "us-east-1", "template-topic", nil); err != nil {
		t.Fatalf("CreateTopic after templated Open: %v", err)
	}
	if _, err := st2.CreateManagedPolicy("000000000001", "TemplatePolicy", `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Action":"*","Resource":"*"}]}`); err != nil {
		t.Fatalf("CreateManagedPolicy after templated Open: %v", err)
	}
}
