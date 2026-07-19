package store_test

import (
	"bytes"
	"errors"
	"testing"

	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

func TestS3BucketEncryptionRoundTrip(t *testing.T) {
	st := openS3Store(t)
	account := "000000000001"
	if _, err := st.CreateBucket(account, "enc-bucket"); err != nil {
		t.Fatal(err)
	}
	enc := store.BucketEncryption{Algorithm: "AES256"}
	if err := st.PutBucketEncryption(account, "enc-bucket", enc); err != nil {
		t.Fatal(err)
	}
	got, err := st.GetBucketEncryption(account, "enc-bucket")
	if err != nil {
		t.Fatal(err)
	}
	if got.Algorithm != "AES256" || got.KMSKeyID != "" {
		t.Fatalf("got=%+v", got)
	}
	if err := st.DeleteBucketEncryption(account, "enc-bucket"); err != nil {
		t.Fatal(err)
	}
	_, err = st.GetBucketEncryption(account, "enc-bucket")
	if !errors.Is(err, store.ErrNoSuchBucketEncryption) {
		t.Fatalf("err=%v want ErrNoSuchBucketEncryption", err)
	}
}

func TestS3CopyObjectSameAccount(t *testing.T) {
	st := openS3Store(t)
	account := "000000000001"
	if _, err := st.CreateBucket(account, "src-bucket"); err != nil {
		t.Fatal(err)
	}
	if _, err := st.CreateBucket(account, "dest-bucket"); err != nil {
		t.Fatal(err)
	}
	payload := []byte("copy-me")
	if _, err := st.PutObject(account, "src-bucket", "a.txt", store.PutObjectMeta{
		ContentType: "text/plain",
		Data:        payload,
		PlainSize:   int64(len(payload)),
	}); err != nil {
		t.Fatal(err)
	}
	obj, err := st.CopyObject(account, "src-bucket", "a.txt", "dest-bucket", "b.txt", store.PutObjectMeta{})
	if err != nil {
		t.Fatal(err)
	}
	if obj.Bucket != "dest-bucket" || obj.Key != "b.txt" {
		t.Fatalf("obj=%+v", obj)
	}
	_, data, err := st.GetObject(account, "dest-bucket", "b.txt")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(data, payload) {
		t.Fatalf("data=%q", data)
	}
}
