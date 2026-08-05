package store

import (
	"bytes"
	"testing"
)

func TestFirehoseCoverageWave2InternalDocBody(t *testing.T) {
	jsonBody := firehoseOpenSearchDocBody([]byte(`{"a":1}`))
	if string(jsonBody) != `{"a":1}` {
		t.Fatalf("json body=%s", jsonBody)
	}
	wrapped := firehoseOpenSearchDocBody([]byte("plain"))
	if !bytes.Contains(wrapped, []byte(`"data"`)) {
		t.Fatalf("wrapped=%s", wrapped)
	}
	emptyJSON := firehoseOpenSearchDocBody([]byte("   "))
	if !bytes.Contains(emptyJSON, []byte(`"data"`)) {
		t.Fatalf("whitespace body=%s", emptyJSON)
	}
}
