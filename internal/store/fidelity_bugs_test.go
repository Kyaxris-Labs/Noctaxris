package store_test

import (
	"path/filepath"
	"sync/atomic"
	"testing"

	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

func TestDeleteAccessKeyInAccountScoped(t *testing.T) {
	dir := t.TempDir()
	key, err := store.LoadOrCreateMasterKey(filepath.Join(dir, "master.key"))
	if err != nil {
		t.Fatal(err)
	}
	st, err := store.Open(dir, key)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })

	if err := st.EnsureRoot("000000000001", "AKIAROOT1", "secret-root-1"); err != nil {
		t.Fatal(err)
	}
	if err := st.EnsureRoot("000000000002", "AKIAROOT2", "secret-root-2"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := st.CreateUser("000000000002", "bob"); err != nil {
		t.Fatal(err)
	}
	keyID, _, err := st.CreateUserAccessKey("000000000002", "bob")
	if err != nil {
		t.Fatal(err)
	}
	if err := st.DeleteAccessKeyInAccount("000000000001", keyID); err == nil {
		t.Fatal("cross-account delete should fail")
	}
	if err := st.DeleteAccessKeyInAccount("000000000002", keyID); err != nil {
		t.Fatal(err)
	}
}

func TestEnqueueAsyncInvokeNotifiesStarter(t *testing.T) {
	dir := t.TempDir()
	key, err := store.LoadOrCreateMasterKey(filepath.Join(dir, "master.key"))
	if err != nil {
		t.Fatal(err)
	}
	st, err := store.Open(dir, key)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })

	var called atomic.Int32
	st.SetOnAsyncEnqueue(func(job store.LambdaAsyncInvocation) {
		if job.FunctionName != "fanout" {
			t.Errorf("function=%q", job.FunctionName)
		}
		called.Add(1)
	})
	if _, err := st.EnqueueAsyncInvoke("000000000001", "fanout", "$LATEST", `{"ok":true}`); err != nil {
		t.Fatal(err)
	}
	if called.Load() != 1 {
		t.Fatalf("starter calls=%d want 1", called.Load())
	}
}
