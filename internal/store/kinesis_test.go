package store_test

import (
	"path/filepath"
	"testing"

	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

func openKinesisStore(t *testing.T) *store.Store {
	t.Helper()
	dir := t.TempDir()
	key, err := store.LoadOrCreateMasterKey(filepath.Join(dir, "master.key"))
	if err != nil {
		t.Fatal(err)
	}
	st, err := store.Open(dir, key)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := st.Close(); err != nil {
			t.Errorf("close: %v", err)
		}
	})
	return st
}

func TestKinesisPutGetRoundTrip(t *testing.T) {
	st := openKinesisStore(t)
	account := "000000000001"
	region := "us-east-1"

	_, err := st.CreateKinesisStream(account, region, "lab-stream", 1)
	if err != nil {
		t.Fatal(err)
	}
	_, err = st.CreateKinesisStream(account, region, "lab-stream", 1)
	if err != store.ErrKinesisStreamExists {
		t.Fatalf("want Exists got %v", err)
	}

	seq, shard, err := st.PutKinesisRecord(account, "lab-stream", "pk-1", []byte("hello"))
	if err != nil {
		t.Fatal(err)
	}
	if seq == "" || shard != store.LabKinesisShardID(0) {
		t.Fatalf("seq=%q shard=%q", seq, shard)
	}

	it, err := st.GetKinesisShardIterator(account, "lab-stream", store.LabKinesisShardID(0), "TRIM_HORIZON", "")
	if err != nil {
		t.Fatal(err)
	}
	recs, next, err := st.GetKinesisRecords(it, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(recs) != 1 || string(recs[0].Data) != "hello" {
		t.Fatalf("recs=%v", recs)
	}
	if next == "" || next == it {
		t.Fatalf("next iterator should advance: %q", next)
	}

	names, err := st.ListKinesisStreams(account, "lab")
	if err != nil || len(names) != 1 || names[0] != "lab-stream" {
		t.Fatalf("list=%v err=%v", names, err)
	}

	if err := st.DeleteKinesisStream(account, "lab-stream"); err != nil {
		t.Fatal(err)
	}
	if _, err := st.DescribeKinesisStream(account, "lab-stream"); err != store.ErrKinesisStreamNotFound {
		t.Fatalf("want not found got %v", err)
	}
}
