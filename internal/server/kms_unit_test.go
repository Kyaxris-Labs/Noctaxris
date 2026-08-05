package server

import (
	"testing"

	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

func TestParseStringMapTags(t *testing.T) {
	t.Parallel()
	m := parseStringMapTags(map[string]any{"k": "v", "n": float64(1)})
	if m["k"] != "v" || m["n"] != "1" {
		t.Fatalf("%#v", m)
	}
	if len(parseStringMapTags(nil)) != 0 {
		t.Fatal("nil")
	}
}

func TestResourceTagsToMap(t *testing.T) {
	t.Parallel()
	got := resourceTagsToMap([]store.ResourceTag{{Key: "a", Value: "1"}})
	if got["a"] != "1" {
		t.Fatalf("%#v", got)
	}
}

func TestParseKMSTagKeys(t *testing.T) {
	t.Parallel()
	got := parseKMSTagKeys([]any{"a", 1, "b"})
	if len(got) != 2 || got[0] != "a" {
		t.Fatalf("%#v", got)
	}
}
