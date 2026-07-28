package server

import (
	"bytes"
	"compress/gzip"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestReadBodyGzipAndLimit(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	if _, err := io.WriteString(zw, `{"ok":true}`); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	req, err := http.NewRequest(http.MethodPost, "http://example/", bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Encoding", "gzip")
	got, err := readBody(req, 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != `{"ok":true}` {
		t.Fatalf("body=%q", got)
	}

	req2, err := http.NewRequest(http.MethodPost, "http://example/", strings.NewReader("plain"))
	if err != nil {
		t.Fatal(err)
	}
	req2.Header.Set("Content-Encoding", "br")
	if _, err := readBody(req2, 1<<20); err == nil || !strings.Contains(err.Error(), "unsupported Content-Encoding") {
		t.Fatalf("want unsupported encoding error, got %v", err)
	}
}
