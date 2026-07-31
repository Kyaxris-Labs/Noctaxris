package store

import (
	"testing"
)

func TestMarshalJSONColumn_emptyCollections(t *testing.T) {
	t.Parallel()
	got, err := marshalJSONColumn(map[string]string(nil))
	if err != nil || got != "{}" {
		t.Fatalf("nil map: got %q err %v", got, err)
	}
	got, err = marshalJSONColumn(map[string]string{})
	if err != nil || got != "{}" {
		t.Fatalf("empty map: got %q err %v", got, err)
	}
	got, err = marshalJSONColumn([]string(nil))
	if err != nil || got != "[]" {
		t.Fatalf("nil slice: got %q err %v", got, err)
	}
	got, err = marshalJSONColumn([]string{})
	if err != nil || got != "[]" {
		t.Fatalf("empty slice: got %q err %v", got, err)
	}
}

func TestUnmarshalJSONColumn_emptyString(t *testing.T) {
	t.Parallel()
	var m map[string]string
	if err := unmarshalJSONColumn("", &m); err != nil {
		t.Fatal(err)
	}
	if m != nil && len(m) != 0 {
		t.Fatalf("expected zero map, got %#v", m)
	}
	var s []string
	if err := unmarshalJSONColumn("  ", &s); err != nil {
		t.Fatal(err)
	}
	if s != nil && len(s) != 0 {
		t.Fatalf("expected zero slice, got %#v", s)
	}
}

func TestArnAccountID(t *testing.T) {
	t.Parallel()
	acct, ok := arnAccountID("arn:aws:sqs:us-east-1:123456789012:my-queue")
	if !ok || acct != "123456789012" {
		t.Fatalf("got %q ok=%v", acct, ok)
	}
	_, ok = arnAccountID("arn:aws:iam::123456789012:role/x")
	if !ok {
		t.Fatal("expected IAM role ARN account")
	}
	_, ok = arnAccountID("not-an-arn")
	if ok {
		t.Fatal("expected false for garbage")
	}
}
