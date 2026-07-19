package store_test

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

func openLambdaStore(t *testing.T) *store.Store {
	t.Helper()
	dir := t.TempDir()
	key, err := store.LoadOrCreateMasterKey(filepath.Join(dir, "master.key"))
	if err != nil {
		t.Fatal(err)
	}
	st, err := store.Open(dir, key)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	return st
}

func testZip(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, body := range files {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write([]byte(body)); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestLambdaCreateGetUpdateCodeDelete(t *testing.T) {
	st := openLambdaStore(t)
	account := "000000000001"
	zip1 := testZip(t, map[string]string{"app.py": "def handler(e,c): return e"})
	sum1 := sha256.Sum256(zip1)

	created, err := st.CreateFunction(store.CreateFunctionMeta{
		AccountID:    account,
		Region:       "us-east-1",
		FunctionName: "hello-world",
		RoleARN:      "arn:aws:iam::000000000001:role/lambda-exec",
		Runtime:      store.LambdaRuntimePython312,
		Handler:      "app.handler",
		Timeout:      10,
		Memory:       256,
		Env:          map[string]string{"STAGE": "lab"},
		Description:  "lab fn",
		Zip:          zip1,
	})
	if err != nil {
		t.Fatal(err)
	}
	wantARN := "arn:aws:lambda:us-east-1:000000000001:function:hello-world"
	if created.FunctionARN != wantARN {
		t.Fatalf("arn=%q want %q", created.FunctionARN, wantARN)
	}
	if created.CodeSHA256 != hex.EncodeToString(sum1[:]) {
		t.Fatalf("sha=%q", created.CodeSHA256)
	}
	if created.State != store.LambdaStateActive {
		t.Fatalf("state=%q", created.State)
	}
	absZip := filepath.Join(st.DataRoot(), created.CodePath)
	if _, err := os.Stat(absZip); err != nil {
		t.Fatalf("missing zip %s: %v", absZip, err)
	}
	if _, err := os.Stat(filepath.Join(st.DataRoot(), "lambda", account, "hello-world", "code", "app.py")); err != nil {
		t.Fatalf("missing unpacked app.py: %v", err)
	}

	got, err := st.GetFunction(account, "hello-world")
	if err != nil {
		t.Fatal(err)
	}
	if got.Handler != "app.handler" || got.Env["STAGE"] != "lab" || got.Timeout != 10 {
		t.Fatalf("got=%+v", got)
	}

	zip2 := testZip(t, map[string]string{"app.py": "def handler(e,c): return {'ok': True}"})
	sum2 := sha256.Sum256(zip2)
	updated, err := st.UpdateFunctionCode(account, "hello-world", zip2)
	if err != nil {
		t.Fatal(err)
	}
	if updated.CodeSHA256 != hex.EncodeToString(sum2[:]) {
		t.Fatalf("updated sha=%q", updated.CodeSHA256)
	}
	data, err := os.ReadFile(filepath.Join(st.DataRoot(), updated.CodePath))
	if err != nil {
		t.Fatal(err)
	}
	if !bytesEqual(data, zip2) {
		t.Fatalf("zip on disk mismatch")
	}

	cfg, err := st.UpdateFunctionConfiguration(account, "hello-world", store.UpdateFunctionConfigurationMeta{
		RoleARN: "arn:aws:iam::000000000001:role/other",
		Timeout: 30,
		Memory:  512,
		Handler: "app.other",
		Env:     map[string]string{"STAGE": "prod"},
		Runtime: store.LambdaRuntimePython312,
	})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.RoleARN != "arn:aws:iam::000000000001:role/other" || cfg.Timeout != 30 || cfg.Memory != 512 || cfg.Handler != "app.other" || cfg.Env["STAGE"] != "prod" {
		t.Fatalf("cfg=%+v", cfg)
	}

	if err := st.DeleteFunction(account, "hello-world"); err != nil {
		t.Fatal(err)
	}
	if _, err := st.GetFunction(account, "hello-world"); !errors.Is(err, store.ErrNoSuchFunction) {
		t.Fatalf("want ErrNoSuchFunction, got %v", err)
	}
	if _, err := os.Stat(filepath.Join(st.DataRoot(), "lambda", account, "hello-world")); !os.IsNotExist(err) {
		t.Fatalf("want dir removed, err=%v", err)
	}
}

func bytesEqual(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestLambdaListAndRejectDuplicate(t *testing.T) {
	st := openLambdaStore(t)
	account := "000000000001"
	zip := testZip(t, map[string]string{"a.py": "x=1"})

	if _, err := st.CreateFunction(store.CreateFunctionMeta{
		AccountID: account, Region: "us-east-1", FunctionName: "fn-a",
		RoleARN: "arn:aws:iam::000000000001:role/r", Runtime: "python3.12",
		Handler: "a.handler", Timeout: 3, Memory: 128, Zip: zip,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := st.CreateFunction(store.CreateFunctionMeta{
		AccountID: account, Region: "us-east-1", FunctionName: "fn-b",
		RoleARN: "arn:aws:iam::000000000001:role/r", Runtime: "python3.12",
		Handler: "b.handler", Timeout: 3, Memory: 128, Zip: zip,
	}); err != nil {
		t.Fatal(err)
	}

	_, err := st.CreateFunction(store.CreateFunctionMeta{
		AccountID: account, Region: "us-east-1", FunctionName: "fn-a",
		RoleARN: "arn:aws:iam::000000000001:role/r", Runtime: "python3.12",
		Handler: "a.handler", Timeout: 3, Memory: 128, Zip: zip,
	})
	if !errors.Is(err, store.ErrFunctionAlreadyExists) {
		t.Fatalf("want ErrFunctionAlreadyExists, got %v", err)
	}

	list, err := st.ListFunctions(account)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 2 {
		t.Fatalf("list=%+v", list)
	}
	if list[0].FunctionName != "fn-a" || list[1].FunctionName != "fn-b" {
		t.Fatalf("order=%+v", list)
	}
}

func TestLambdaInvalidFunctionName(t *testing.T) {
	st := openLambdaStore(t)
	zip := testZip(t, map[string]string{"a.py": "x=1"})
	_, err := st.CreateFunction(store.CreateFunctionMeta{
		AccountID: "000000000001", Region: "us-east-1", FunctionName: "1bad",
		RoleARN: "arn:aws:iam::000000000001:role/r", Runtime: "python3.12",
		Handler: "a.handler", Timeout: 3, Memory: 128, Zip: zip,
	})
	if !errors.Is(err, store.ErrInvalidFunctionName) {
		t.Fatalf("want ErrInvalidFunctionName, got %v", err)
	}
	if err := store.ValidateFunctionName("ok_name-1"); err != nil {
		t.Fatal(err)
	}
	if err := store.ValidateFunctionName(""); !errors.Is(err, store.ErrInvalidFunctionName) {
		t.Fatalf("empty: %v", err)
	}
}

func TestFunctionARN(t *testing.T) {
	got := store.FunctionARN("000000000001", "us-west-2", "demo")
	want := "arn:aws:lambda:us-west-2:000000000001:function:demo"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
	if store.FunctionARN("000000000001", "", "demo") != "arn:aws:lambda:us-east-1:000000000001:function:demo" {
		t.Fatal("default region")
	}
}
