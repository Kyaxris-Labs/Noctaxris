package codec_test

import (
	"bytes"
	"compress/gzip"
	"encoding/base64"
	"encoding/json"
	"testing"

	"github.com/Kyaxris-Labs/Noctaxris/internal/codec"
)

func TestCompressDecompressRoundTrip(t *testing.T) {
	raw := []byte(`{"messageType":"DATA_MESSAGE","logEvents":[]}`)
	compressed, err := codec.Compress(raw)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(compressed, raw) {
		t.Fatal("expected gzip output to differ from plaintext")
	}
	out, err := codec.Decompress(compressed)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(out, raw) {
		t.Fatalf("got %q want %q", out, raw)
	}
}

func TestAwslogsEnvelopeRoundTrip(t *testing.T) {
	raw := []byte(`{"messageType":"DATA_MESSAGE","owner":"123","logGroup":"g","logStream":"s","logEvents":[]}`)
	envelope, err := codec.AwslogsEnvelopeEncode(raw)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := codec.AwslogsEnvelopeDecode(envelope)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(decoded, raw) {
		t.Fatalf("decoded %q want %q", decoded, raw)
	}
	var wrap struct {
		Awslogs struct {
			Data string `json:"data"`
		} `json:"awslogs"`
	}
	if err := json.Unmarshal(envelope, &wrap); err != nil {
		t.Fatal(err)
	}
	gz, err := base64.StdEncoding.DecodeString(wrap.Awslogs.Data)
	if err != nil {
		t.Fatal(err)
	}
	gr, err := gzip.NewReader(bytes.NewReader(gz))
	if err != nil {
		t.Fatal(err)
	}
	defer gr.Close()
	var buf bytes.Buffer
	if _, err := buf.ReadFrom(gr); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(buf.Bytes(), raw) {
		t.Fatal("inner gzip mismatch")
	}
}
