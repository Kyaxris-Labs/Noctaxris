package server_test

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

func TestS3CopyObjectPutGet(t *testing.T) {
	srv, _ := newTestServer(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	mustS3(t, handler, http.MethodPut, "http://127.0.0.1:4566/copy-src-bucket", nil, "s3", now, nil)
	mustS3(t, handler, http.MethodPut, "http://127.0.0.1:4566/copy-dest-bucket", nil, "s3", now, nil)
	payload := []byte("copied-object")
	mustS3(t, handler, http.MethodPut, "http://127.0.0.1:4566/copy-src-bucket/source.txt", payload, "s3", now, map[string]string{
		"Content-Type": "text/plain",
	})

	copyRec := mustS3(t, handler, http.MethodPut, "http://127.0.0.1:4566/copy-dest-bucket/dest.txt", nil, "s3", now, map[string]string{
		"x-amz-copy-source": "/copy-src-bucket/source.txt",
	})
	if copyRec.Code != http.StatusOK {
		t.Fatalf("CopyObject status=%d body=%q", copyRec.Code, copyRec.Body.String())
	}
	if !strings.Contains(copyRec.Body.String(), "<ETag>") {
		t.Fatalf("missing ETag in %q", copyRec.Body.String())
	}

	getRec := mustS3(t, handler, http.MethodGet, "http://127.0.0.1:4566/copy-dest-bucket/dest.txt", nil, "s3", now, nil)
	if getRec.Code != http.StatusOK {
		t.Fatalf("GetObject status=%d body=%q", getRec.Code, getRec.Body.String())
	}
	if getRec.Body.String() != string(payload) {
		t.Fatalf("body=%q want %q", getRec.Body.String(), payload)
	}
}

func TestS3BucketDefaultEncryptionPutObject(t *testing.T) {
	srv, _ := newTestServer(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	mustS3(t, handler, http.MethodPut, "http://127.0.0.1:4566/default-enc-bucket", nil, "s3", now, nil)
	encBody := []byte(`<ServerSideEncryptionConfiguration><Rule><ApplyServerSideEncryptionByDefault><SSEAlgorithm>AES256</SSEAlgorithm></ApplyServerSideEncryptionByDefault></Rule></ServerSideEncryptionConfiguration>`)
	putEncRec := mustS3(t, handler, http.MethodPut, "http://127.0.0.1:4566/default-enc-bucket?encryption", encBody, "s3", now, map[string]string{
		"Content-Type": "application/xml",
	})
	if putEncRec.Code != http.StatusOK {
		t.Fatalf("PutBucketEncryption status=%d body=%q", putEncRec.Code, putEncRec.Body.String())
	}

	payload := []byte("encrypted-by-default")
	putRec := mustS3(t, handler, http.MethodPut, "http://127.0.0.1:4566/default-enc-bucket/obj.bin", payload, "s3", now, nil)
	if putRec.Code != http.StatusOK {
		t.Fatalf("PutObject status=%d body=%q", putRec.Code, putRec.Body.String())
	}
	if putRec.Header().Get("x-amz-server-side-encryption") != "AES256" {
		t.Fatalf("missing default SSE header: %v", putRec.Header())
	}

	getRec := mustS3(t, handler, http.MethodGet, "http://127.0.0.1:4566/default-enc-bucket/obj.bin", nil, "s3", now, nil)
	if getRec.Code != http.StatusOK {
		t.Fatalf("GetObject status=%d body=%q", getRec.Code, getRec.Body.String())
	}
	if getRec.Body.String() != string(payload) {
		t.Fatalf("body=%q", getRec.Body.String())
	}
}

func TestS3MultipartInheritsBucketDefaultEncryption(t *testing.T) {
	srv, _ := newTestServer(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	mustS3(t, handler, http.MethodPut, "http://127.0.0.1:4566/mp-enc-bucket", nil, "s3", now, nil)
	encBody := []byte(`<ServerSideEncryptionConfiguration><Rule><ApplyServerSideEncryptionByDefault><SSEAlgorithm>AES256</SSEAlgorithm></ApplyServerSideEncryptionByDefault></Rule></ServerSideEncryptionConfiguration>`)
	mustS3(t, handler, http.MethodPut, "http://127.0.0.1:4566/mp-enc-bucket?encryption", encBody, "s3", now, map[string]string{
		"Content-Type": "application/xml",
	})

	createRec := mustS3(t, handler, http.MethodPost, "http://127.0.0.1:4566/mp-enc-bucket/mp.bin?uploads", nil, "s3", now, nil)
	if createRec.Code != http.StatusOK {
		t.Fatalf("CreateMultipartUpload status=%d body=%q", createRec.Code, createRec.Body.String())
	}
	if createRec.Header().Get("x-amz-server-side-encryption") != "AES256" {
		t.Fatalf("multipart create missing default SSE: %v", createRec.Header())
	}
	uploadID := parseMultipartUploadID(t, createRec.Body.Bytes())

	part := strings.Repeat("m", store.MinMultipartPartSize)
	p1Rec := mustS3(t, handler, http.MethodPut, "http://127.0.0.1:4566/mp-enc-bucket/mp.bin?partNumber=1&uploadId="+uploadID, []byte(part), "s3", now, nil)
	etag1 := strings.Trim(p1Rec.Header().Get("ETag"), `"`)
	completeBody := []byte(`<CompleteMultipartUpload><Part><PartNumber>1</PartNumber><ETag>"` + etag1 + `"</ETag></Part></CompleteMultipartUpload>`)
	completeRec := mustS3(t, handler, http.MethodPost, "http://127.0.0.1:4566/mp-enc-bucket/mp.bin?uploadId="+uploadID, completeBody, "s3", now, map[string]string{
		"Content-Type": "application/xml",
	})
	if completeRec.Code != http.StatusOK {
		t.Fatalf("CompleteMultipartUpload status=%d body=%q", completeRec.Code, completeRec.Body.String())
	}

	getRec := mustS3(t, handler, http.MethodGet, "http://127.0.0.1:4566/mp-enc-bucket/mp.bin", nil, "s3", now, nil)
	if getRec.Code != http.StatusOK {
		t.Fatalf("GetObject status=%d body=%q", getRec.Code, getRec.Body.String())
	}
	if getRec.Body.String() != part {
		t.Fatalf("body len=%d want %d", getRec.Body.Len(), len(part))
	}
}
