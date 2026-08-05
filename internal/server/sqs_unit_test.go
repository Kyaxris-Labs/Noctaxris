package server

import (
	"testing"
)

func TestStringMapParam(t *testing.T) {
	t.Parallel()
	m := stringMapParam(map[string]any{"a": "1", "b": 2})
	if m["a"] != "1" || len(m) != 1 {
		t.Fatalf("%#v", m)
	}
	if len(stringMapParam(nil)) != 0 {
		t.Fatal("nil -> empty map")
	}
}

func TestAttrTruthySQS(t *testing.T) {
	t.Parallel()
	attrs := map[string]string{"FifoQueue": "true", "X": "0"}
	if !attrTruthySQS(attrs, "FifoQueue") || attrTruthySQS(attrs, "X") {
		t.Fatal("truthy")
	}
	if attrTruthySQS(attrs, "Missing") {
		t.Fatal("missing")
	}
}

func TestSQSSendOptsFromParams(t *testing.T) {
	t.Parallel()
	opts := sqsSendOptsFromParams(map[string]string{"FifoQueue": "true"}, map[string]any{
		"MessageGroupId":         "g1",
		"MessageDeduplicationId": "d1",
		"DelaySeconds":           float64(2),
	})
	if opts == nil || opts.MessageGroupID != "g1" || opts.DelaySeconds != 2 {
		t.Fatalf("%#v", opts)
	}
	entry := sqsSendOptsFromEntry(map[string]string{"FifoQueue": "true"}, map[string]any{
		"Id": "1", "MessageBody": "hi", "MessageGroupId": "g",
	})
	if entry == nil || entry.MessageGroupID != "g" {
		t.Fatalf("%#v", entry)
	}
}
