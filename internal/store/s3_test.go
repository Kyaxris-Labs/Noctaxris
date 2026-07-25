package store_test

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

func openS3Store(t *testing.T) *store.Store {
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
	t.Cleanup(func() { _ = st.Close() })
	return st
}

func TestS3CreateBucketPutGet(t *testing.T) {
	st := openS3Store(t)
	account := "000000000001"

	b, err := st.CreateBucket(account, "lab-bucket")
	if err != nil {
		t.Fatal(err)
	}
	if b.Name != "lab-bucket" || b.CreationDate == "" {
		t.Fatalf("bucket=%+v", b)
	}

	payload := []byte("hello-s3")
	meta, err := st.PutObject(account, "lab-bucket", "docs/readme.txt", store.PutObjectMeta{
		ContentType: "text/plain",
		Data:        payload,
		PlainSize:   int64(len(payload)),
	})
	if err != nil {
		t.Fatal(err)
	}
	if meta.ETag == "" || meta.Size != int64(len(payload)) {
		t.Fatalf("meta=%+v", meta)
	}

	gotMeta, data, err := st.GetObject(account, "lab-bucket", "docs/readme.txt")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(data, payload) {
		t.Fatalf("data=%q", data)
	}
	if gotMeta.ContentType != "text/plain" {
		t.Fatalf("content-type=%q", gotMeta.ContentType)
	}

	abs := filepath.Join(st.DataRoot(), gotMeta.StoragePath)
	if _, err := os.Stat(abs); err != nil {
		t.Fatalf("missing file %s: %v", abs, err)
	}
}

func TestS3BucketPolicyRoundTrip(t *testing.T) {
	st := openS3Store(t)
	account := "000000000001"
	if _, err := st.CreateBucket(account, "policy-bucket"); err != nil {
		t.Fatal(err)
	}
	policy := `{"Version":"2012-10-17","Statement":[{"Effect":"Deny","Action":"s3:GetObject","Resource":"*"}]}`
	if err := st.PutBucketPolicy(account, "policy-bucket", policy); err != nil {
		t.Fatal(err)
	}
	got, err := st.GetBucketPolicy(account, "policy-bucket")
	if err != nil {
		t.Fatal(err)
	}
	if got != policy {
		t.Fatalf("policy=%q", got)
	}
	if err := st.DeleteBucketPolicy(account, "policy-bucket"); err != nil {
		t.Fatal(err)
	}
	_, err = st.GetBucketPolicy(account, "policy-bucket")
	if !errors.Is(err, store.ErrNoSuchBucketPolicy) {
		t.Fatalf("err=%v want NoSuchBucketPolicy", err)
	}
}

func TestS3RejectTraversalKey(t *testing.T) {
	st := openS3Store(t)
	account := "000000000001"
	if _, err := st.CreateBucket(account, "safe-bucket"); err != nil {
		t.Fatal(err)
	}
	_, err := st.PutObject(account, "safe-bucket", "../evil", store.PutObjectMeta{Data: []byte("x"), PlainSize: 1})
	if !errors.Is(err, store.ErrInvalidObjectKey) {
		t.Fatalf("err=%v want InvalidObjectKey", err)
	}
	_, err = st.PutObject(account, "safe-bucket", "a/\x00/b", store.PutObjectMeta{Data: []byte("x"), PlainSize: 1})
	if !errors.Is(err, store.ErrInvalidObjectKey) {
		t.Fatalf("null byte err=%v want InvalidObjectKey", err)
	}
	_, err = st.PutObject(account, "safe-bucket", `..\evil`, store.PutObjectMeta{Data: []byte("x"), PlainSize: 1})
	if !errors.Is(err, store.ErrInvalidObjectKey) {
		t.Fatalf(`backslash traversal err=%v want InvalidObjectKey`, err)
	}
	_, err = st.PutObject(account, "safe-bucket", `foo\bar`, store.PutObjectMeta{Data: []byte("x"), PlainSize: 1})
	if !errors.Is(err, store.ErrInvalidObjectKey) {
		t.Fatalf(`backslash key err=%v want InvalidObjectKey`, err)
	}
}

func TestSanitizeObjectKey(t *testing.T) {
	_, err := store.SanitizeObjectKey("../evil")
	if !errors.Is(err, store.ErrInvalidObjectKey) {
		t.Fatalf("err=%v", err)
	}
	got, err := store.SanitizeObjectKey("/ok/path")
	if err != nil {
		t.Fatal(err)
	}
	if got != "ok/path" {
		t.Fatalf("got=%q", got)
	}
	for _, key := range []string{`..\evil`, `foo\bar`, `a\b\c`, `ok\..\escape`} {
		if _, err := store.SanitizeObjectKey(key); !errors.Is(err, store.ErrInvalidObjectKey) {
			t.Fatalf("key=%q err=%v want InvalidObjectKey", key, err)
		}
	}
}

func TestDeleteBucketNotEmpty(t *testing.T) {
	st := openS3Store(t)
	account := "000000000001"
	if _, err := st.CreateBucket(account, "full-bucket"); err != nil {
		t.Fatal(err)
	}
	if _, err := st.PutObject(account, "full-bucket", "a.txt", store.PutObjectMeta{Data: []byte("a"), PlainSize: 1}); err != nil {
		t.Fatal(err)
	}
	err := st.DeleteBucket(account, "full-bucket")
	if !errors.Is(err, store.ErrBucketNotEmpty) {
		t.Fatalf("err=%v", err)
	}
}

func TestS3GetBucketByNameGlobalUniqueness(t *testing.T) {
	st := openS3Store(t)
	account := "000000000001"
	if _, err := st.CreateBucket(account, "global-lookup"); err != nil {
		t.Fatal(err)
	}
	got, err := st.GetBucketByName("global-lookup")
	if err != nil {
		t.Fatal(err)
	}
	if got.AccountID != account || got.Name != "global-lookup" {
		t.Fatalf("bucket=%+v", got)
	}
	_, err = st.CreateBucket("000000000002", "global-lookup")
	if !errors.Is(err, store.ErrBucketAlreadyExists) {
		t.Fatalf("err=%v want BucketAlreadyExists", err)
	}
}
