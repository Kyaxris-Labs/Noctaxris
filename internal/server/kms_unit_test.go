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

func TestGrantConstraintStringMapEquivalence(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		in   any
		ok   bool
		want map[string]string
	}{
		{"string map", map[string]string{"a": "1"}, true, map[string]string{"a": "1"}},
		{"any map", map[string]any{"b": "2"}, true, map[string]string{"b": "2"}},
		{"non-string value", map[string]any{"c": 3}, false, nil},
		{"wrong type", []string{"x"}, false, nil},
		{"nil", nil, false, nil},
	}
	for _, tc := range cases {
		got, ok := grantConstraintStringMap(tc.in)
		if ok != tc.ok {
			t.Fatalf("%s ok=%v want %v", tc.name, ok, tc.ok)
		}
		if tc.ok {
			for k, v := range tc.want {
				if got[k] != v {
					t.Fatalf("%s got=%v want=%v", tc.name, got, tc.want)
				}
			}
		}
	}
}
