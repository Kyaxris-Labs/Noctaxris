package server

import (
	"encoding/base64"
	"testing"
)

func TestSecretsSecretID(t *testing.T) {
	t.Parallel()
	if secretsSecretID(map[string]any{"SecretId": "  arn "}) != "arn" {
		t.Fatal("SecretId")
	}
	if secretsSecretID(map[string]any{"Name": "n"}) != "n" {
		t.Fatal("Name")
	}
	if secretsSecretID(map[string]any{}) != "" {
		t.Fatal("empty")
	}
}

func TestSecretsBoolParam(t *testing.T) {
	t.Parallel()
	if !secretsBoolParam(true, false) || secretsBoolParam(false, true) {
		t.Fatal("bool only")
	}
	if secretsBoolParam(nil, true) != true {
		t.Fatal("default")
	}
}

func TestSecretsStringSliceParam(t *testing.T) {
	t.Parallel()
	got := secretsStringSliceParam([]any{"a", 1, "b"})
	if len(got) != 2 || got[0] != "a" {
		t.Fatalf("%#v", got)
	}
}

func TestSecretsStagesContain(t *testing.T) {
	t.Parallel()
	if !secretsStagesContain([]string{"AWSCURRENT", "AWSPENDING"}, "AWSCURRENT") {
		t.Fatal("contains")
	}
	if secretsStagesContain(nil, "x") {
		t.Fatal("nil")
	}
}

func TestSecretsBinaryParam(t *testing.T) {
	t.Parallel()
	raw := base64.StdEncoding.EncodeToString([]byte("hi"))
	b, err := secretsBinaryParam(raw)
	if err != nil || string(b) != "hi" {
		t.Fatalf("%q err=%v", b, err)
	}
	if b, err := secretsBinaryParam(42); err != nil || b != nil {
		t.Fatalf("non-string got %v err=%v", b, err)
	}
	if b, err := secretsBinaryParam(""); err != nil || b != nil {
		t.Fatal("empty string")
	}
}
