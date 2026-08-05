package store_test

import (
	"encoding/base64"
	"errors"
	"testing"

	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

func TestFirehoseCoverageWave2ListDeleteBatchDecode(t *testing.T) {
	st := openTestStore(t)
	account := "000000000001"
	if err := store.EnsureFirehoseSchema(nil); err == nil {
		t.Fatal("nil db must fail")
	}
	if err := st.EnsureFirehoseSchema(); err != nil {
		t.Fatal(err)
	}

	empty, err := st.ListFirehoseStreams(account)
	if err != nil || len(empty) != 0 {
		t.Fatalf("empty=%v err=%v", empty, err)
	}
	if err := st.DeleteFirehoseStream(account, "missing"); !errors.Is(err, store.ErrFirehoseNotFound) {
		t.Fatalf("delete missing: %v", err)
	}
	if _, err := st.PutFirehoseRecordBatch(account, "missing", [][]byte{[]byte("x")}); !errors.Is(err, store.ErrFirehoseNotFound) {
		t.Fatalf("batch missing: %v", err)
	}

	if name := store.ParseFirehoseOpenSearchDomainRef("", ""); name != "" {
		t.Fatalf("empty ref=%q", name)
	}
	if name := store.ParseFirehoseOpenSearchDomainRef("arn:aws:es:us-east-1:1:domain/lab-os", ""); name != "lab-os" {
		t.Fatalf("arn ref=%q", name)
	}
	if name := store.ParseFirehoseOpenSearchDomainRef("arn:x", "explicit"); name != "explicit" {
		t.Fatalf("explicit=%q", name)
	}

	if _, err := store.DecodeFirehoseData(123); err == nil {
		t.Fatal("DecodeFirehoseData non-string must fail")
	}
	raw := []byte("hello-wave2")
	dec, err := store.DecodeFirehoseData(raw)
	if err != nil || string(dec) != "hello-wave2" {
		t.Fatalf("bytes decode=%q err=%v", dec, err)
	}
	b64 := base64.StdEncoding.EncodeToString(raw)
	dec2, err := store.DecodeFirehoseData(b64)
	if err != nil || string(dec2) != "hello-wave2" {
		t.Fatalf("b64 decode=%q err=%v", dec2, err)
	}
	if _, err := store.DecodeFirehoseData("!!!not-b64!!!"); err == nil {
		t.Fatal("invalid b64 must fail")
	}

	if _, err := st.CreateBucket(account, "fh-wave2-dest"); err != nil {
		t.Fatal(err)
	}
	policy := `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":{"Service":"firehose.amazonaws.com"},"Action":"s3:PutObject","Resource":"*"}]}`
	if err := st.PutBucketPolicy(account, "fh-wave2-dest", policy); err != nil {
		t.Fatal(err)
	}
	stream, err := st.CreateFirehoseStream(account, "us-east-1", "wave2-fh", "", "S3", "fh-wave2-dest", "out/", "")
	if err != nil {
		t.Fatal(err)
	}
	listed, err := st.ListFirehoseStreams(account)
	if err != nil || len(listed) != 1 || listed[0].Name != stream.Name {
		t.Fatalf("list=%v err=%v", listed, err)
	}

	failed, err := st.PutFirehoseRecordBatch(account, "wave2-fh", [][]byte{
		[]byte(`{"ok":true}`),
		{},
		[]byte("plain-text-body"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(failed) != 1 || failed[0] != 1 {
		t.Fatalf("failed indexes=%v want [1]", failed)
	}

	if err := st.DeleteFirehoseStream(account, "wave2-fh"); err != nil {
		t.Fatal(err)
	}
	if err := st.DeleteFirehoseStream(account, "wave2-fh"); !errors.Is(err, store.ErrFirehoseNotFound) {
		t.Fatalf("second delete: %v", err)
	}
	after, err := st.ListFirehoseStreams(account)
	if err != nil || len(after) != 0 {
		t.Fatalf("after=%v err=%v", after, err)
	}
}
