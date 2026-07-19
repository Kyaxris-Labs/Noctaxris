package store_test

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

func TestS3MultipartUploadComplete(t *testing.T) {
	st := openS3Store(t)
	account := "000000000001"
	if _, err := st.CreateBucket(account, "mp-bucket"); err != nil {
		t.Fatal(err)
	}

	upload, err := st.CreateMultipartUpload(account, "mp-bucket", "big.bin", store.CreateMultipartUploadMeta{
		ContentType: "application/octet-stream",
	})
	if err != nil {
		t.Fatal(err)
	}
	if upload.UploadID == "" {
		t.Fatal("missing upload id")
	}

	part1 := bytes.Repeat([]byte("a"), store.MinMultipartPartSize)
	part2 := []byte("tail")
	p1, err := st.UploadPart(account, "mp-bucket", "big.bin", upload.UploadID, 1, store.UploadPartMeta{Data: part1})
	if err != nil {
		t.Fatal(err)
	}
	p2, err := st.UploadPart(account, "mp-bucket", "big.bin", upload.UploadID, 2, store.UploadPartMeta{Data: part2})
	if err != nil {
		t.Fatal(err)
	}

	obj, err := st.CompleteMultipartUpload(account, "mp-bucket", "big.bin", upload.UploadID, []store.CompletedPartInput{
		{PartNumber: 1, ETag: p1.ETag},
		{PartNumber: 2, ETag: p2.ETag},
	})
	if err != nil {
		t.Fatal(err)
	}
	final, err := st.PutObject(account, "mp-bucket", "big.bin", store.PutObjectMeta{
		ContentType: obj.Upload.ContentType,
		Data:        obj.Data,
		PlainSize:   int64(len(obj.Data)),
		ETag:        obj.ETag,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := st.CleanupMultipartUpload(upload.UploadID); err != nil {
		t.Fatal(err)
	}
	wantSize := int64(len(part1) + len(part2))
	if final.Size != wantSize {
		t.Fatalf("size=%d want %d", final.Size, wantSize)
	}

	gotMeta, data, err := st.GetObject(account, "mp-bucket", "big.bin")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(data, append(part1, part2...)) {
		t.Fatalf("data len=%d", len(data))
	}
	if gotMeta.ETag != final.ETag {
		t.Fatalf("etag mismatch meta=%q obj=%q", gotMeta.ETag, final.ETag)
	}

	parts, err := st.ListMultipartUploads(account, "mp-bucket", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(parts) != 0 {
		t.Fatalf("uploads still present: %+v", parts)
	}
}

func TestS3MultipartAbortDeletesParts(t *testing.T) {
	st := openS3Store(t)
	account := "000000000001"
	if _, err := st.CreateBucket(account, "abort-bucket"); err != nil {
		t.Fatal(err)
	}
	upload, err := st.CreateMultipartUpload(account, "abort-bucket", "x.bin", store.CreateMultipartUploadMeta{})
	if err != nil {
		t.Fatal(err)
	}
	part, err := st.UploadPart(account, "abort-bucket", "x.bin", upload.UploadID, 1, store.UploadPartMeta{Data: []byte("x")})
	if err != nil {
		t.Fatal(err)
	}
	abs := filepath.Join(st.DataRoot(), part.StoragePath)
	if _, err := os.Stat(abs); err != nil {
		t.Fatal(err)
	}
	if err := st.AbortMultipartUpload(account, "abort-bucket", "x.bin", upload.UploadID); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(abs); !os.IsNotExist(err) {
		t.Fatalf("part blob still exists: %v", err)
	}
}

func TestS3MultipartEntityTooSmall(t *testing.T) {
	st := openS3Store(t)
	account := "000000000001"
	if _, err := st.CreateBucket(account, "small-bucket"); err != nil {
		t.Fatal(err)
	}
	upload, err := st.CreateMultipartUpload(account, "small-bucket", "y.bin", store.CreateMultipartUploadMeta{})
	if err != nil {
		t.Fatal(err)
	}
	p1, err := st.UploadPart(account, "small-bucket", "y.bin", upload.UploadID, 1, store.UploadPartMeta{Data: []byte("tiny")})
	if err != nil {
		t.Fatal(err)
	}
	p2, err := st.UploadPart(account, "small-bucket", "y.bin", upload.UploadID, 2, store.UploadPartMeta{Data: []byte("last")})
	if err != nil {
		t.Fatal(err)
	}
	_, err = st.CompleteMultipartUpload(account, "small-bucket", "y.bin", upload.UploadID, []store.CompletedPartInput{
		{PartNumber: 1, ETag: p1.ETag},
		{PartNumber: 2, ETag: p2.ETag},
	})
	if !errors.Is(err, store.ErrEntityTooSmall) {
		t.Fatalf("err=%v want EntityTooSmall", err)
	}
}

func TestS3ListParts(t *testing.T) {
	st := openS3Store(t)
	account := "000000000001"
	if _, err := st.CreateBucket(account, "list-bucket"); err != nil {
		t.Fatal(err)
	}
	upload, err := st.CreateMultipartUpload(account, "list-bucket", "z.bin", store.CreateMultipartUploadMeta{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.UploadPart(account, "list-bucket", "z.bin", upload.UploadID, 1, store.UploadPartMeta{Data: bytes.Repeat([]byte("z"), store.MinMultipartPartSize)}); err != nil {
		t.Fatal(err)
	}
	if _, err := st.UploadPart(account, "list-bucket", "z.bin", upload.UploadID, 2, store.UploadPartMeta{Data: []byte("z")}); err != nil {
		t.Fatal(err)
	}
	parts, truncated, err := st.ListParts(account, "list-bucket", "z.bin", upload.UploadID, 0, 10)
	if err != nil {
		t.Fatal(err)
	}
	if truncated || len(parts) != 2 {
		t.Fatalf("parts=%+v truncated=%v", parts, truncated)
	}
}
