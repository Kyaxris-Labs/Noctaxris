package codec_test

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"

	"github.com/Kyaxris-Labs/Noctaxris/internal/codec"
)

func TestCompress_empty(t *testing.T) {
	compressed, err := codec.Compress(nil)
	if err != nil {
		t.Fatal(err)
	}
	out, err := codec.Decompress(compressed)
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 0 {
		t.Fatalf("got %q want empty", out)
	}
}

func TestDecompress_invalidGzip(t *testing.T) {
	_, err := codec.Decompress([]byte("not-gzip"))
	if err == nil {
		t.Fatal("expected error for invalid gzip")
	}
}

func TestDecompress_truncatedGzip(t *testing.T) {
	raw := []byte("hello-truncated")
	compressed, err := codec.Compress(raw)
	if err != nil {
		t.Fatal(err)
	}
	if len(compressed) < 4 {
		t.Fatal("unexpected short compress")
	}
	_, err = codec.Decompress(compressed[:len(compressed)/2])
	if err == nil {
		t.Fatal("expected error for truncated gzip")
	}
}

func TestAwslogsEnvelopeDecode_invalidJSON(t *testing.T) {
	_, err := codec.AwslogsEnvelopeDecode([]byte("{"))
	if err == nil {
		t.Fatal("expected JSON error")
	}
}

func TestAwslogsEnvelopeDecode_missingData(t *testing.T) {
	_, err := codec.AwslogsEnvelopeDecode([]byte(`{"awslogs":{}}`))
	if err == nil || !strings.Contains(err.Error(), "awslogs.data missing") {
		t.Fatalf("want missing data error, got %v", err)
	}
}

func TestAwslogsEnvelopeDecode_badBase64(t *testing.T) {
	_, err := codec.AwslogsEnvelopeDecode([]byte(`{"awslogs":{"data":"!!!not-b64!!!"}}`))
	if err == nil {
		t.Fatal("expected base64 error")
	}
}

func TestAwslogsEnvelopeDecode_badGzipInside(t *testing.T) {
	payload, err := json.Marshal(map[string]any{
		"awslogs": map[string]string{
			"data": base64.StdEncoding.EncodeToString([]byte("not-gzip-payload")),
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = codec.AwslogsEnvelopeDecode(payload)
	if err == nil {
		t.Fatal("expected gzip decompress error")
	}
}

func TestAwslogsEnvelopeEncode_emptyJSON(t *testing.T) {
	envelope, err := codec.AwslogsEnvelopeEncode([]byte("{}"))
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := codec.AwslogsEnvelopeDecode(envelope)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(decoded, []byte("{}")) {
		t.Fatalf("got %q", decoded)
	}
}
