package server_test

import (
	"net/http"
	"testing"
	"time"
)

func TestS3PutObjectAwsChunkedStreamingTrailer(t *testing.T) {
	srv, _, _ := newTestServerStore(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	mb := mustS3(t, handler, http.MethodPut, "http://127.0.0.1:4566/chunked-lab", nil, "s3", now, nil)
	if mb.Code < 200 || mb.Code >= 300 {
		t.Fatalf("CreateBucket %d %s", mb.Code, mb.Body.String())
	}

	framed := "" +
		"5\r\n" +
		"hello\r\n" +
		"0\r\n" +
		"x-amz-checksum-crc64nvme:M3eFcAZSQlc=\r\n" +
		"\r\n"
	put := mustS3(t, handler, http.MethodPut, "http://127.0.0.1:4566/chunked-lab/smoke.txt", []byte(framed), "s3", now, map[string]string{
		"Content-Type":                   "text/plain",
		"Content-Encoding":               "aws-chunked",
		"X-Amz-Content-Sha256":           "STREAMING-UNSIGNED-PAYLOAD-TRAILER",
		"X-Amz-Decoded-Content-Length":   "5",
		"X-Amz-Trailer":                  "x-amz-checksum-crc64nvme",
		"x-amz-sdk-checksum-algorithm":   "CRC64NVME",
	})
	if put.Code != http.StatusOK {
		t.Fatalf("PutObject aws-chunked %d %s", put.Code, put.Body.String())
	}

	get := mustS3(t, handler, http.MethodGet, "http://127.0.0.1:4566/chunked-lab/smoke.txt", nil, "s3", now, nil)
	if get.Code != http.StatusOK {
		t.Fatalf("GetObject %d %s", get.Code, get.Body.String())
	}
	if got := get.Body.String(); got != "hello" {
		t.Fatalf("stored object=%q want hello", got)
	}
}
