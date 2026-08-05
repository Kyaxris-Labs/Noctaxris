package store_test

import (
	"errors"
	"testing"

	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

func TestLabJSONPathExtractTopLevel(t *testing.T) {
	data := map[string]any{"color": "red", "n": float64(1)}

	got, err := store.LabJSONPathExtract(data, "$.color", store.LabPathTopLevelOnly)
	if err != nil || got != "red" {
		t.Fatalf("color: got=%v err=%v", got, err)
	}

	_, err = store.LabJSONPathExtract(data, "$.missing", store.LabPathTopLevelOnly)
	if !store.LabJSONPathMissingKey(err) {
		t.Fatalf("missing key want LabJSONPathMissingKey, got %v", err)
	}

	// Boundary: $ and empty return whole document
	for _, path := range []string{"$", "", "  $  "} {
		got, err := store.LabJSONPathExtract(data, path, store.LabPathTopLevelOnly)
		if err != nil {
			t.Fatalf("path %q: %v", path, err)
		}
		m, ok := got.(map[string]any)
		if !ok || m["color"] != "red" {
			t.Fatalf("path %q: got %#v", path, got)
		}
	}

	// Negative: nested path rejected for top-level profile
	if _, err := store.LabJSONPathExtract(data, "$.a.b", store.LabPathTopLevelOnly); err == nil {
		t.Fatal("expected nested path error for LabPathTopLevelOnly")
	}
	if _, err := store.LabJSONPathExtract(data, "$.arr[*]", store.LabPathTopLevelOnly); err == nil {
		t.Fatal("expected bracket/wildcard reject")
	}
	if _, err := store.LabJSONPathExtract(data, "color", store.LabPathTopLevelOnly); err == nil {
		t.Fatal("expected must be $.field")
	}
	if _, err := store.LabJSONPathExtract("not-object", "$.x", store.LabPathTopLevelOnly); err == nil {
		t.Fatal("expected non-object error")
	}
}

func TestLabJSONPathExtractNested(t *testing.T) {
	data := map[string]any{
		"a": map[string]any{
			"b-hyphen": map[string]any{"c": "ok"},
		},
	}
	got, err := store.LabJSONPathExtract(data, "$.a.b-hyphen.c", store.LabPathNested)
	if err != nil || got != "ok" {
		t.Fatalf("nested: got=%v err=%v", got, err)
	}

	if _, err := store.LabJSONPathExtract(data, "$.a.missing", store.LabPathNested); err == nil {
		t.Fatal("expected missing nested key")
	}
	if _, err := store.LabJSONPathExtract(data, "$.a..b", store.LabPathNested); err == nil {
		t.Fatal("expected empty segment")
	}
	if _, err := store.LabJSONPathExtract(data, "a.b", store.LabPathNested); err == nil {
		t.Fatal("expected must start with $")
	}
	if _, err := store.LabJSONPathExtract(data, "$.x[*]", store.LabPathNested); err == nil {
		t.Fatal("expected wildcard reject")
	}

	// Negative: unknown profile
	if _, err := store.LabJSONPathExtract(data, "$.a", store.LabPathProfile(99)); err == nil {
		t.Fatal("expected unknown profile")
	}
	if store.LabJSONPathMissingKey(errors.New("other")) {
		t.Fatal("LabJSONPathMissingKey should be false for unrelated errors")
	}
}
