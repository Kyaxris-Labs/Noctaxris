package store_test

import (
	"errors"
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

func TestLambdaCreateFunctionZipRuntimes(t *testing.T) {
	st := openLambdaStore(t)
	account := "000000000001"
	zip := testZip(t, map[string]string{"app.py": "def handler(e,c): return e"})

	names := map[string]string{
		store.LambdaRuntimePython311: "fn-py311",
		store.LambdaRuntimePython312: "fn-py312",
		store.LambdaRuntimeNodejs20x: "fn-node20",
	}
	for rt, name := range names {
		created, err := st.CreateFunction(store.CreateFunctionMeta{
			AccountID: account, Region: "us-east-1", FunctionName: name,
			RoleARN: "arn:aws:iam::000000000001:role/r", Runtime: rt,
			Handler: "app.handler", Timeout: 3, Memory: 128, Zip: zip,
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
		Runtime: store.LambdaRuntimeNodejs20x,
		Handler: "app.handler",
		Timeout: 3,
		Memory:  128,
		Env:     map[string]string{},
	})
	if err != nil {
		t.Fatal(err)
	}
	if updated.Runtime != store.LambdaRuntimeNodejs20x {
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
