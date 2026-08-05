package server

import (
	"strings"
	"testing"
)

func TestDynamoAttrTypes(t *testing.T) {
	t.Parallel()
	got := dynamoAttrTypes(map[string]any{
		"AttributeDefinitions": []any{
			map[string]any{"AttributeName": "pk", "AttributeType": "S"},
			map[string]any{"AttributeName": "sk", "AttributeType": "N"},
			"skip",
			map[string]any{"AttributeName": "", "AttributeType": "S"},
		},
	})
	if got["pk"] != "S" || got["sk"] != "N" {
		t.Fatalf("%#v", got)
	}
	if len(dynamoAttrTypes(nil)) != 0 {
		t.Fatal("nil")
	}
}

func TestDynamoParseGSISpec(t *testing.T) {
	t.Parallel()
	attrs := map[string]string{"gsi_pk": "S", "gsi_sk": "S"}
	_, err := dynamoParseGSISpec(map[string]any{}, attrs)
	if err == nil || !strings.Contains(err.Error(), "IndexName") {
		t.Fatalf("want IndexName err, got %v", err)
	}
	_, err = dynamoParseGSISpec(map[string]any{
		"IndexName":  "G1",
		"Projection": map[string]any{"ProjectionType": "KEYS_ONLY"},
	}, attrs)
	if err == nil || !strings.Contains(err.Error(), "ALL") {
		t.Fatalf("want projection err, got %v", err)
	}
	_, err = dynamoParseGSISpec(map[string]any{
		"IndexName": "G1",
		"KeySchema": []any{map[string]any{"AttributeName": "gsi_pk", "KeyType": "RANGE"}},
	}, attrs)
	if err == nil || !strings.Contains(err.Error(), "HASH") {
		t.Fatalf("want HASH err, got %v", err)
	}
	_, err = dynamoParseGSISpec(map[string]any{
		"IndexName": "G1",
		"KeySchema": []any{
			map[string]any{"AttributeName": "gsi_pk", "KeyType": "HASH"},
			map[string]any{"AttributeName": "missing", "KeyType": "RANGE"},
		},
	}, attrs)
	if err == nil || !strings.Contains(err.Error(), "RANGE") {
		t.Fatalf("want RANGE type err, got %v", err)
	}
	gsi, err := dynamoParseGSISpec(map[string]any{
		"IndexName":  "G1",
		"Projection": map[string]any{"ProjectionType": "ALL"},
		"KeySchema": []any{
			map[string]any{"AttributeName": "gsi_pk", "KeyType": "HASH"},
			map[string]any{"AttributeName": "gsi_sk", "KeyType": "RANGE"},
			"skip",
		},
	}, attrs)
	if err != nil {
		t.Fatal(err)
	}
	if gsi.IndexName != "G1" || gsi.HashKeyName != "gsi_pk" || gsi.RangeKeyName != "gsi_sk" {
		t.Fatalf("%#v", gsi)
	}
}

func TestDynamoParseLSISpec(t *testing.T) {
	t.Parallel()
	attrs := map[string]string{"pk": "S", "lsi_sk": "S"}
	_, err := dynamoParseLSISpec(map[string]any{}, attrs, "pk")
	if err == nil || !strings.Contains(err.Error(), "IndexName") {
		t.Fatalf("want IndexName err, got %v", err)
	}
	_, err = dynamoParseLSISpec(map[string]any{
		"IndexName":  "L1",
		"Projection": map[string]any{"ProjectionType": "INCLUDE"},
	}, attrs, "pk")
	if err == nil || !strings.Contains(err.Error(), "ALL") {
		t.Fatalf("want projection err, got %v", err)
	}
	_, err = dynamoParseLSISpec(map[string]any{
		"IndexName": "L1",
		"KeySchema": []any{
			map[string]any{"AttributeName": "other", "KeyType": "HASH"},
			map[string]any{"AttributeName": "lsi_sk", "KeyType": "RANGE"},
		},
	}, attrs, "pk")
	if err == nil {
		t.Fatal("want hash mismatch err")
	}
	lsi, err := dynamoParseLSISpec(map[string]any{
		"IndexName": "L1",
		"KeySchema": []any{
			map[string]any{"AttributeName": "pk", "KeyType": "HASH"},
			map[string]any{"AttributeName": "lsi_sk", "KeyType": "RANGE"},
		},
	}, attrs, "pk")
	if err != nil {
		t.Fatal(err)
	}
	if lsi.IndexName != "L1" || lsi.RangeKeyName != "lsi_sk" {
		t.Fatalf("%#v", lsi)
	}
}
