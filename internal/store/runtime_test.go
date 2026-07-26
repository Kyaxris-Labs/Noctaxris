package store_test

import (
	"archive/zip"
	"bytes"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

func TestValidateLambdaRuntime(t *testing.T) {
	for _, rt := range store.SupportedLambdaRuntimes() {
		if err := store.ValidateLambdaRuntime(rt); err != nil {
			t.Fatalf("runtime %q: %v", rt, err)
		}
	}
	for _, rt := range []string{"", "python3.9", "nodejs18.x", "java11"} {
		if err := store.ValidateLambdaRuntime(rt); !errors.Is(err, store.ErrInvalidRuntime) {
			t.Fatalf("runtime %q: want ErrInvalidRuntime, got %v", rt, err)
		}
	}
}

func TestIsPythonAndNodeLambdaRuntime(t *testing.T) {
	for _, rt := range []string{
		store.LambdaRuntimePython311,
		store.LambdaRuntimePython312,
		store.LambdaRuntimePython313,
		store.LambdaRuntimePython314,
	} {
		if !store.IsPythonLambdaRuntime(rt) {
			t.Fatalf("IsPythonLambdaRuntime(%q)=false", rt)
		}
		if store.IsNodeLambdaRuntime(rt) {
			t.Fatalf("IsNodeLambdaRuntime(%q)=true", rt)
		}
		if store.IsJavaLambdaRuntime(rt) {
			t.Fatalf("IsJavaLambdaRuntime(%q)=true", rt)
		}
	}
	for _, rt := range []string{
		store.LambdaRuntimeNodejs20x,
		store.LambdaRuntimeNodejs22x,
		store.LambdaRuntimeNodejs24x,
	} {
		if !store.IsNodeLambdaRuntime(rt) {
			t.Fatalf("IsNodeLambdaRuntime(%q)=false", rt)
		}
		if store.IsPythonLambdaRuntime(rt) {
			t.Fatalf("IsPythonLambdaRuntime(%q)=true", rt)
		}
		if store.IsJavaLambdaRuntime(rt) {
			t.Fatalf("IsJavaLambdaRuntime(%q)=true", rt)
		}
	}
	if store.IsPythonLambdaRuntime("java21") || store.IsNodeLambdaRuntime("java21") {
		t.Fatal("java21 must not classify as python or node")
	}
}

func TestIsJavaLambdaRuntime(t *testing.T) {
	for _, rt := range []string{store.LambdaRuntimeJava21, store.LambdaRuntimeJava25} {
		if !store.IsJavaLambdaRuntime(rt) {
			t.Fatalf("IsJavaLambdaRuntime(%q)=false", rt)
		}
		if store.IsPythonLambdaRuntime(rt) || store.IsNodeLambdaRuntime(rt) {
			t.Fatalf("%q must not classify as python or node", rt)
		}
	}
	if store.IsJavaLambdaRuntime(store.LambdaRuntimePython312) {
		t.Fatal("python must not classify as java")
	}
}

func TestLambdaCreateFunctionZipRuntimes(t *testing.T) {
	st := openLambdaStore(t)
	account := "000000000001"
	zip := testZip(t, map[string]string{"app.py": "def handler(e,c): return e"})

	names := map[string]string{
		store.LambdaRuntimePython311: "fn-py311",
		store.LambdaRuntimePython312: "fn-py312",
		store.LambdaRuntimePython313: "fn-py313",
		store.LambdaRuntimePython314: "fn-py314",
		store.LambdaRuntimeNodejs20x: "fn-node20",
		store.LambdaRuntimeNodejs22x: "fn-node22",
		store.LambdaRuntimeNodejs24x: "fn-node24",
		store.LambdaRuntimeJava21:    "fn-java21",
		store.LambdaRuntimeJava25:    "fn-java25",
	}
	for rt, name := range names {
		handler := "app.handler"
		if store.IsJavaLambdaRuntime(rt) {
			handler = "example.Echo::handleRequest"
		}
		created, err := st.CreateFunction(store.CreateFunctionMeta{
			AccountID: account, Region: "us-east-1", FunctionName: name,
			RoleARN: "arn:aws:iam::000000000001:role/r", Runtime: rt,
			Handler: handler, Timeout: 3, Memory: 128, Zip: zip,
		})
		if err != nil {
			t.Fatalf("runtime %q: %v", rt, err)
		}
		if created.Runtime != rt {
			t.Fatalf("runtime %q stored as %q", rt, created.Runtime)
		}
	}

	_, err := st.CreateFunction(store.CreateFunctionMeta{
		AccountID: account, Region: "us-east-1", FunctionName: "fn-bad",
		RoleARN: "arn:aws:iam::000000000001:role/r", Runtime: "python3.9",
		Handler: "app.handler", Timeout: 3, Memory: 128, Zip: zip,
	})
	if !errors.Is(err, store.ErrInvalidRuntime) {
		t.Fatalf("want ErrInvalidRuntime, got %v", err)
	}
}

func TestLambdaCreateFunctionJavaZipWithHostJavac(t *testing.T) {
	if _, err := exec.LookPath("javac"); err != nil {
		t.Skip("javac not on PATH")
	}
	dir := t.TempDir()
	srcDir := filepath.Join(dir, "example")
	if err := os.MkdirAll(srcDir, 0o755); err != nil {
		t.Fatal(err)
	}
	src := filepath.Join(srcDir, "Echo.java")
	const echoSrc = `package example;
public class Echo {
  public static String handleRequest(String in) { return in; }
}
`
	if err := os.WriteFile(src, []byte(echoSrc), 0o644); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("javac", "-d", dir, src)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("javac: %v\n%s", err, out)
	}
	classBytes, err := os.ReadFile(filepath.Join(dir, "example", "Echo.class"))
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	w, err := zw.Create("example/Echo.class")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.Write(classBytes); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}

	st := openLambdaStore(t)
	created, err := st.CreateFunction(store.CreateFunctionMeta{
		AccountID: "000000000001", Region: "us-east-1", FunctionName: "fn-java-class",
		RoleARN: "arn:aws:iam::000000000001:role/r", Runtime: store.LambdaRuntimeJava21,
		Handler: "example.Echo::handleRequest", Timeout: 3, Memory: 128, Zip: buf.Bytes(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if created.Runtime != store.LambdaRuntimeJava21 {
		t.Fatalf("Runtime=%q", created.Runtime)
	}
}

func TestLambdaCreateFunctionImageSkipsRuntimeValidation(t *testing.T) {
	st := openLambdaStore(t)
	created, err := st.CreateFunction(store.CreateFunctionMeta{
		AccountID:    "000000000001",
		Region:       "us-east-1",
		FunctionName: "img-no-runtime",
		RoleARN:      "arn:aws:iam::000000000001:role/r",
		Handler:      "app.handler",
		Timeout:      3,
		Memory:       128,
		PackageType:  store.LambdaPackageTypeImage,
		ImageURI:     "public.ecr.aws/lambda/python:3.12",
	})
	if err != nil {
		t.Fatal(err)
	}
	if created.PackageType != store.LambdaPackageTypeImage {
		t.Fatalf("PackageType=%q", created.PackageType)
	}
}

func TestLambdaUpdateFunctionConfigurationRuntime(t *testing.T) {
	st := openLambdaStore(t)
	account := "000000000001"
	zip := testZip(t, map[string]string{"app.py": "def handler(e,c): return e"})
	_, err := st.CreateFunction(store.CreateFunctionMeta{
		AccountID: account, Region: "us-east-1", FunctionName: "cfg-fn",
		RoleARN: "arn:aws:iam::000000000001:role/r", Runtime: store.LambdaRuntimePython312,
		Handler: "app.handler", Timeout: 3, Memory: 128, Zip: zip,
	})
	if err != nil {
		t.Fatal(err)
	}

	updated, err := st.UpdateFunctionConfiguration(account, "cfg-fn", store.UpdateFunctionConfigurationMeta{
		RoleARN: "arn:aws:iam::000000000001:role/r",
		Runtime: store.LambdaRuntimeNodejs24x,
		Handler: "app.handler",
		Timeout: 3,
		Memory:  128,
		Env:     map[string]string{},
	})
	if err != nil {
		t.Fatal(err)
	}
	if updated.Runtime != store.LambdaRuntimeNodejs24x {
		t.Fatalf("Runtime=%q", updated.Runtime)
	}

	_, err = st.UpdateFunctionConfiguration(account, "cfg-fn", store.UpdateFunctionConfigurationMeta{
		RoleARN: "arn:aws:iam::000000000001:role/r",
		Runtime: "ruby2.7",
		Handler: "app.handler",
		Timeout: 3,
		Memory:  128,
		Env:     map[string]string{},
	})
	if !errors.Is(err, store.ErrInvalidRuntime) {
		t.Fatalf("want ErrInvalidRuntime, got %v", err)
	}
}
