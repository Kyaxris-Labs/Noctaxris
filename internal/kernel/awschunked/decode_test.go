package awschunked_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/Kyaxris-Labs/Noctaxris/internal/kernel/awschunked"
)

func TestDecodeSimpleAndTrailer(t *testing.T) {
	framed := "" +
		"5\r\n" +
		"hello\r\n" +
		"0\r\n" +
		"x-amz-checksum-crc64nvme:M3eFcAZSQlc=\r\n" +
		"\r\n"
	payload, trailers, err := awschunked.Decode(strings.NewReader(framed))
	if err != nil {
		t.Fatal(err)
	}
	if string(payload) != "hello" {
		t.Fatalf("payload=%q", payload)
	}
	if got := trailers.Get("x-amz-checksum-crc64nvme"); got != "M3eFcAZSQlc=" {
		t.Fatalf("trailer=%q", got)
	}
}

func TestDecodeChunkSignatureExtension(t *testing.T) {
	framed := "" +
		"5;chunk-signature=abcdef0123456789\r\n" +
		"hello\r\n" +
		"0;chunk-signature=fedcba9876543210\r\n" +
		"\r\n"
	payload, _, err := awschunked.Decode(bytes.NewReader([]byte(framed)))
	if err != nil {
		t.Fatal(err)
	}
	if string(payload) != "hello" {
		t.Fatalf("payload=%q", payload)
	}
}

func TestDecodeMultiChunk(t *testing.T) {
	framed := "" +
		"3\r\n" +
		"hel\r\n" +
		"2\r\n" +
		"lo\r\n" +
		"0\r\n" +
		"\r\n"
	payload, _, err := awschunked.Decode(strings.NewReader(framed))
	if err != nil {
		t.Fatal(err)
	}
	if string(payload) != "hello" {
		t.Fatalf("payload=%q", payload)
	}
}

func TestHasAndStripEncoding(t *testing.T) {
	if !awschunked.HasEncoding("aws-chunked") {
		t.Fatal("expected aws-chunked")
	}
	if !awschunked.HasEncoding("gzip, aws-chunked") {
		t.Fatal("expected aws-chunked in list")
	}
	if awschunked.HasEncoding("gzip") {
		t.Fatal("unexpected")
	}
	if got := awschunked.StripEncoding("gzip, aws-chunked"); got != "gzip" {
		t.Fatalf("strip=%q", got)
	}
	if got := awschunked.StripEncoding("aws-chunked"); got != "" {
		t.Fatalf("strip empty=%q", got)
	}
}
