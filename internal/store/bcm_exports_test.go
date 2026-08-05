package store_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

func openBCMStore(t *testing.T) *store.Store {
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
	if err := st.EnsureBCMExportSchema(); err != nil {
		t.Fatal(err)
	}
	return st
}

func TestBCMExportARNAndCRUD(t *testing.T) {
	st := openBCMStore(t)
	account := "000000000001"

	if err := store.EnsureBCMExportSchema(nil); err == nil {
		t.Fatal("nil db must fail")
	}
	arn := store.BCMExportARN("", account, "n", "id")
	if !strings.HasPrefix(arn, "arn:aws:bcm-data-exports:us-east-1:") {
		t.Fatalf("default region arn=%q", arn)
	}

	_, err := st.CreateBCMExport(account, "us-east-1", "", "d", "CSV")
	if !errors.Is(err, store.ErrBCMExportBadRequest) {
		t.Fatalf("empty name err=%v", err)
	}
	_, err = st.CreateBCMExport(account, "us-east-1", "bad", "d", "XML")
	if !errors.Is(err, store.ErrBCMExportBadRequest) {
		t.Fatalf("bad format err=%v", err)
	}

	csvExp, err := st.CreateBCMExport(account, "us-east-1", "lab-cur", "sample", "")
	if err != nil {
		t.Fatal(err)
	}
	if csvExp.Format != "CSV" || csvExp.ExportARN == "" {
		t.Fatalf("csv=%+v", csvExp)
	}
	raw, err := os.ReadFile(csvExp.FilePath)
	if err != nil || !strings.Contains(string(raw), account) {
		t.Fatalf("csv sample=%q err=%v", string(raw), err)
	}
	if !strings.HasPrefix(filepath.Clean(csvExp.FilePath), filepath.Clean(st.DataRoot())) {
		t.Fatalf("export path escapes data root: %s", csvExp.FilePath)
	}

	jsonExp, err := st.CreateBCMExport(account, "us-west-2", "lab-json", "", "json")
	if err != nil {
		t.Fatal(err)
	}
	jraw, err := os.ReadFile(jsonExp.FilePath)
	if err != nil || !strings.Contains(string(jraw), "AmazonS3") {
		t.Fatalf("json sample=%q err=%v", string(jraw), err)
	}

	got, err := st.GetBCMExport(account, csvExp.ExportARN)
	if err != nil || got.ExportName != "lab-cur" {
		t.Fatalf("get=%+v err=%v", got, err)
	}
	_, err = st.GetBCMExport(account, "arn:aws:bcm-data-exports:us-east-1:1:export/x/y")
	if !errors.Is(err, store.ErrBCMExportNotFound) {
		t.Fatalf("missing get err=%v", err)
	}
	list, err := st.ListBCMExports(account)
	if err != nil || len(list) != 2 {
		t.Fatalf("list=%v err=%v", list, err)
	}
	if err := st.DeleteBCMExport(account, csvExp.ExportARN); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(csvExp.FilePath); !os.IsNotExist(err) {
		t.Fatalf("sample should be removed: %v", err)
	}
	if err := st.DeleteBCMExport(account, csvExp.ExportARN); !errors.Is(err, store.ErrBCMExportNotFound) {
		t.Fatalf("delete missing err=%v", err)
	}
}
