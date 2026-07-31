package server

import "testing"

func TestParseTagListKeyValue(t *testing.T) {
	t.Parallel()
	raw := []any{
		map[string]any{"Key": " a ", "Value": "v1"},
		map[string]any{"Key": "", "Value": "skip"},
		map[string]any{"TagKey": "wrong"},
	}
	out, err := parseTagList(raw, "Key", "Value", true)
	if err != nil {
		t.Fatal(err)
	}
	if out["a"] != "v1" || len(out) != 1 {
		t.Fatalf("got %v", out)
	}
}

func TestParseKeyValueTags(t *testing.T) {
	t.Parallel()
	if parseKeyValueTags(nil) != nil {
		t.Fatal("nil")
	}
	if parseKeyValueTags([]any{}) != nil {
		t.Fatal("empty slice")
	}
	tags := parseKeyValueTags([]any{
		map[string]any{"Key": "k", "Value": "v"},
	})
	if tags["k"] != "v" {
		t.Fatalf("got %v", tags)
	}
}

func TestParseKMSTags(t *testing.T) {
	t.Parallel()
	tags := parseKMSTags([]any{
		map[string]any{"TagKey": " tk ", "TagValue": "tv"},
	})
	if tags["tk"] != "tv" {
		t.Fatalf("got %v", tags)
	}
	if parseKMSTags([]any{}) != nil {
		t.Fatal("empty")
	}
}

func TestParseEMRTags(t *testing.T) {
	t.Parallel()
	if parseEMRTags("x") != nil {
		t.Fatal("non-array")
	}
	empty := parseEMRTags([]any{})
	if empty == nil || len(empty) != 0 {
		t.Fatalf("empty array should be map, got %v", empty)
	}
	tags := parseEMRTags([]any{
		map[string]any{"Key": " k ", "Value": "v"},
	})
	if tags[" k "] != "v" {
		t.Fatalf("EMR keys are not trimmed: %v", tags)
	}
}

func TestParseResourceGroupTagMap(t *testing.T) {
	t.Parallel()
	m := parseResourceGroupTagMap(map[string]any{
		"a": "1",
		"b": 2,
	})
	if m["a"] != "1" || m["b"] != "" {
		t.Fatalf("got %v", m)
	}
}
