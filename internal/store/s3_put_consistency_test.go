package store_test

import (
	"bytes"
	"path/filepath"
	"sync"
	"testing"

	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

func TestPutObjectConcurrentSameKeyConsistent(t *testing.T) {
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

	account := "000000000001"
	if _, err := st.CreateBucket(account, "put-lock"); err != nil {
		t.Fatal(err)
	}

	const n = 16
	var wg sync.WaitGroup
	errs := make(chan error, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			body := bytes.Repeat([]byte{byte('A' + i%26)}, 64)
			_, err := st.PutObject(account, "put-lock", "same-key", store.PutObjectMeta{
				Data: body, PlainSize: int64(len(body)),
			})
			errs <- err
		}(i)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("PutObject: %v", err)
		}
	}
	meta, data, err := st.GetObject(account, "put-lock", "same-key")
	if err != nil {
		t.Fatal(err)
	}
	if meta.Size != 64 || len(data) != 64 {
		t.Fatalf("size=%d len(data)=%d", meta.Size, len(data))
	}
	if !bytes.Equal(bytes.Repeat(data[:1], 64), data) {
		t.Fatalf("blob not uniform after concurrent puts")
	}
}
