package compute

import (
	"strings"
	"testing"

	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

func TestFunctionImagesForRuntime(t *testing.T) {
	cases := map[string]FunctionImages{
		store.LambdaRuntimePython312: {
			Preferred: PreferredFunctionImage,
			Fallback:  FallbackFunctionImage,
		},
		store.LambdaRuntimePython311: {
			Preferred: "public.ecr.aws/lambda/python:3.11",
			Fallback:  "python:3.11-slim",
		},
		store.LambdaRuntimeNodejs20x: {
			Preferred: "public.ecr.aws/lambda/nodejs:20",
			Fallback:  "node:20-slim",
		},
	}
	for rt, want := range cases {
		got, err := FunctionImagesForRuntime(rt)
		if err != nil {
			t.Fatalf("runtime %q: %v", rt, err)
		}
		if got != want {
			t.Fatalf("runtime %q: got %+v want %+v", rt, got, want)
		}
	}
	if _, err := FunctionImagesForRuntime("python3.9"); err == nil {
		t.Fatal("expected error for unsupported runtime")
	}
}

func TestZipOneShotCommand(t *testing.T) {
	py, err := zipOneShotCommand(store.LambdaRuntimePython312)
	if err != nil {
		t.Fatal(err)
	}
	if py.Exe != "python" || !strings.Contains(py.Script, "python3.12") {
		t.Fatalf("python312: %+v", py)
	}

	py311, err := zipOneShotCommand(store.LambdaRuntimePython311)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(py311.Script, "python3.11") {
		t.Fatalf("python311 script: %q", py311.Script)
	}

	node, err := zipOneShotCommand(store.LambdaRuntimeNodejs20x)
	if err != nil {
		t.Fatal(err)
	}
	if node.Exe != "node" || !strings.Contains(node.Script, "require") {
		t.Fatalf("nodejs: %+v", node)
	}
}

func TestImageOneShotCommand(t *testing.T) {
	cmd, ok := imageOneShotCommand("public.ecr.aws/lambda/python:3.11")
	if !ok || cmd.Exe != "python" || !strings.Contains(cmd.Script, "python3.11") {
		t.Fatalf("python3.11 image: ok=%v cmd=%+v", ok, cmd)
	}
	cmd, ok = imageOneShotCommand("public.ecr.aws/lambda/nodejs:20")
	if !ok || cmd.Exe != "node" {
		t.Fatalf("nodejs image: ok=%v cmd=%+v", ok, cmd)
	}
	_, ok = imageOneShotCommand("alpine:3.19")
	if ok {
		t.Fatal("expected no one-shot override for generic image")
	}
}
