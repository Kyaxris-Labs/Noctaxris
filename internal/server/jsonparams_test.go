package server

import (
	"encoding/json"
	"testing"
)

func TestJSONBodyMap(t *testing.T) {
	t.Parallel()
	m := jsonBodyMap([]byte(`{"a":1,"b":"x"}`))
	if m["b"] != "x" {
		t.Fatalf("b=%v", m["b"])
	}
	if _, ok := m["a"].(float64); !ok {
		t.Fatalf("a type=%T", m["a"])
	}
	if len(jsonBodyMap(nil)) != 0 {
		t.Fatal("nil body should yield empty map")
	}
}

func TestStringSliceParam(t *testing.T) {
	t.Parallel()
	got := stringSliceParam([]any{"a", 1, "b"})
	if len(got) != 2 || got[0] != "a" || got[1] != "b" {
		t.Fatalf("got %v", got)
	}
	if stringSliceParam([]string{"x"})[0] != "x" {
		t.Fatal("[]string passthrough")
	}
	if stringSliceParam("nope") != nil {
		t.Fatal("invalid type should be nil")
	}
}

func TestIntParam(t *testing.T) {
	t.Parallel()
	const def = 99
	if intParam(float64(3), def) != 3 {
		t.Fatal("float64")
	}
	if intParam(int(7), def) != 7 {
		t.Fatal("int")
	}
	if intParam(int64(11), def) != 11 {
		t.Fatal("int64")
	}
	if intParam(json.Number("42"), def) != 42 {
		t.Fatal("json.Number")
	}
	if intParam(json.Number("x"), def) != def {
		t.Fatal("bad json.Number")
	}
	if intParam("  5 ", def) != 5 {
		t.Fatal("string")
	}
	if intParam("nope", def) != def {
		t.Fatal("bad string")
	}
	if intParam(nil, def) != def {
		t.Fatal("nil")
	}
	// intFromJSONNumber compatibility: zero default
	if intParam(float64(8), 0) != 8 {
		t.Fatal("zero default")
	}
}
