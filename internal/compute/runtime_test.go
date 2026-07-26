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
		store.LambdaRuntimePython313: {
			Preferred: "public.ecr.aws/lambda/python:3.13",
			Fallback:  "python:3.13-slim",
		},
		store.LambdaRuntimePython314: {
			Preferred: "public.ecr.aws/lambda/python:3.14",
			Fallback:  "python:3.14-slim",
		},
		store.LambdaRuntimeNodejs20x: {
			Preferred: "public.ecr.aws/lambda/nodejs:20",
			Fallback:  "node:20-slim",
		},
		store.LambdaRuntimeNodejs22x: {
			Preferred: "public.ecr.aws/lambda/nodejs:22",
			Fallback:  "node:22-slim",
		},
		store.LambdaRuntimeNodejs24x: {
			Preferred: "public.ecr.aws/lambda/nodejs:24",
			Fallback:  "node:24-slim",
		},
		store.LambdaRuntimeJava21: {
			Preferred: "eclipse-temurin:21-jdk",
			Fallback:  "public.ecr.aws/lambda/java:21",
		},
		store.LambdaRuntimeJava25: {
			Preferred: "eclipse-temurin:25-jdk",
			Fallback:  "public.ecr.aws/lambda/java:25",
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

	py313, err := zipOneShotCommand(store.LambdaRuntimePython313)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(py313.Script, "python3.13") {
		t.Fatalf("python313 script: %q", py313.Script)
	}

	py314, err := zipOneShotCommand(store.LambdaRuntimePython314)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(py314.Script, "python3.14") {
		t.Fatalf("python314 script: %q", py314.Script)
	}

	for _, rt := range []string{
		store.LambdaRuntimeNodejs20x,
		store.LambdaRuntimeNodejs22x,
		store.LambdaRuntimeNodejs24x,
	} {
		node, err := zipOneShotCommand(rt)
		if err != nil {
			t.Fatalf("runtime %q: %v", rt, err)
		}
		if node.Exe != "node" || !strings.Contains(node.Script, "require") {
			t.Fatalf("nodejs %q: %+v", rt, node)
		}
	}

	for _, rt := range []string{store.LambdaRuntimeJava21, store.LambdaRuntimeJava25} {
		java, err := zipOneShotCommand(rt)
		if err != nil {
			t.Fatalf("runtime %q: %v", rt, err)
		}
		if java.Exe != "/bin/sh" || java.Flag != "-c" {
			t.Fatalf("java %q: %+v", rt, java)
		}
		if !strings.Contains(java.Script, "NoctaxrisInvoke") || !strings.Contains(java.Script, "javac") {
			t.Fatalf("java %q script missing bootstrap: %q", rt, java.Script)
		}
		if !strings.Contains(java.Script, "::") || !strings.Contains(java.Script, "handleRequest") {
			t.Fatalf("java %q script missing handler parse: %q", rt, java.Script)
		}
	}
}

func TestImageOneShotCommand(t *testing.T) {
	cmd, ok := imageOneShotCommand("public.ecr.aws/lambda/python:3.11")
	if !ok || cmd.Exe != "python" || !strings.Contains(cmd.Script, "python3.11") {
		t.Fatalf("python3.11 image: ok=%v cmd=%+v", ok, cmd)
	}
	cmd, ok = imageOneShotCommand("public.ecr.aws/lambda/python:3.13")
	if !ok || cmd.Exe != "python" || !strings.Contains(cmd.Script, "python3.13") {
		t.Fatalf("python3.13 image: ok=%v cmd=%+v", ok, cmd)
	}
	cmd, ok = imageOneShotCommand("public.ecr.aws/lambda/python:3.14")
	if !ok || cmd.Exe != "python" || !strings.Contains(cmd.Script, "python3.14") {
		t.Fatalf("python3.14 image: ok=%v cmd=%+v", ok, cmd)
	}
	cmd, ok = imageOneShotCommand("python:3.13-slim")
	if !ok || !strings.Contains(cmd.Script, "python3.13") {
		t.Fatalf("python:3.13-slim: ok=%v cmd=%+v", ok, cmd)
	}
	cmd, ok = imageOneShotCommand("public.ecr.aws/lambda/nodejs:20")
	if !ok || cmd.Exe != "node" {
		t.Fatalf("nodejs20 image: ok=%v cmd=%+v", ok, cmd)
	}
	cmd, ok = imageOneShotCommand("public.ecr.aws/lambda/nodejs:22")
	if !ok || cmd.Exe != "node" {
		t.Fatalf("nodejs22 image: ok=%v cmd=%+v", ok, cmd)
	}
	cmd, ok = imageOneShotCommand("node:24-slim")
	if !ok || cmd.Exe != "node" {
		t.Fatalf("node:24-slim: ok=%v cmd=%+v", ok, cmd)
	}
	cmd, ok = imageOneShotCommand("public.ecr.aws/lambda/java:21")
	if !ok || cmd.Exe != "/bin/sh" || !strings.Contains(cmd.Script, "javac") {
		t.Fatalf("java21 image: ok=%v cmd=%+v", ok, cmd)
	}
	cmd, ok = imageOneShotCommand("eclipse-temurin:25-jdk")
	if !ok || cmd.Exe != "/bin/sh" {
		t.Fatalf("temurin25 image: ok=%v cmd=%+v", ok, cmd)
	}
	_, ok = imageOneShotCommand("alpine:3.19")
	if ok {
		t.Fatal("expected no one-shot override for generic image")
	}
}
