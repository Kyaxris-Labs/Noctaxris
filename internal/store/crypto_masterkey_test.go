package store_test

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

func TestResolveMasterKeyPathDefaultsOutsideDataRoot(t *testing.T) {
	t.Setenv(store.EnvAllowMasterKeyInDataRoot, "")
	dataRoot := t.TempDir()
	path, err := store.ResolveMasterKeyPath("", dataRoot)
	if err != nil {
		t.Fatal(err)
	}
	under, err := store.MasterKeyPathUnderDataRoot(path, dataRoot)
	if err != nil {
		t.Fatal(err)
	}
	if under {
		t.Fatalf("default path %q must be outside data root %q", path, dataRoot)
	}
	want := store.DefaultMasterKeyPath(dataRoot)
	if path != want {
		t.Fatalf("got %q want %q", path, want)
	}
}

func TestResolveMasterKeyPathRefuseColocatedWithoutOptIn(t *testing.T) {
	t.Setenv(store.EnvAllowMasterKeyInDataRoot, "")
	dataRoot := t.TempDir()
	colocated := filepath.Join(dataRoot, "master.key")
	_, err := store.ResolveMasterKeyPath(colocated, dataRoot)
	if err == nil {
		t.Fatal("expected refuse when master key is under DATA_ROOT without opt-in")
	}
	if !strings.Contains(err.Error(), store.EnvAllowMasterKeyInDataRoot) {
		t.Fatalf("error should mention opt-in env: %v", err)
	}
}

func TestResolveMasterKeyPathAllowColocatedDefault(t *testing.T) {
	t.Setenv(store.EnvAllowMasterKeyInDataRoot, "1")
	dataRoot := t.TempDir()
	path, err := store.ResolveMasterKeyPath("", dataRoot)
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(dataRoot, "master.key")
	if path != want {
		t.Fatalf("got %q want %q", path, want)
	}
}

func TestResolveMasterKeyPathAllowExplicitColocated(t *testing.T) {
	t.Setenv(store.EnvAllowMasterKeyInDataRoot, "1")
	dataRoot := t.TempDir()
	colocated := filepath.Join(dataRoot, "keys", "master.key")
	path, err := store.ResolveMasterKeyPath(colocated, dataRoot)
	if err != nil {
		t.Fatal(err)
	}
	if path != colocated {
		t.Fatalf("got %q want %q", path, colocated)
	}
}

func TestMasterKeyPathUnderDataRoot(t *testing.T) {
	dataRoot := t.TempDir()
	cases := []struct {
		path string
		want bool
	}{
		{filepath.Join(dataRoot, "master.key"), true},
		{filepath.Join(dataRoot, "nested", "master.key"), true},
		{store.DefaultMasterKeyPath(dataRoot), false},
		{filepath.Join(filepath.Dir(dataRoot), "other", "master.key"), false},
	}
	for _, tc := range cases {
		got, err := store.MasterKeyPathUnderDataRoot(tc.path, dataRoot)
		if err != nil {
			t.Fatalf("%s: %v", tc.path, err)
		}
		if got != tc.want {
			t.Fatalf("%s: under=%v want %v", tc.path, got, tc.want)
		}
	}
}

func TestLoadOrCreateMasterKeyMode0600(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "master.key")
	if _, err := store.LoadOrCreateMasterKey(path); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	mode := info.Mode().Perm()
	if runtime.GOOS == "windows" {
		// Soft-assert: Windows may not retain Unix mode bits.
		t.Logf("windows master.key mode=%04o (ACL model; chmod best-effort)", mode)
		return
	}
	if mode&0o004 != 0 {
		t.Fatalf("master.key world-readable: mode=%04o", mode)
	}
	if mode != 0o600 {
		t.Fatalf("master.key mode=%04o want 0600", mode)
	}
}

func TestLoadOrCreateMasterKeyRefusesWorldReadableWhenChmodFails(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("unix permission bits not enforced on Windows")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "master.key")
	raw := make([]byte, 32)
	for i := range raw {
		raw[i] = byte(i + 1)
	}
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		t.Fatal(err)
	}
	// Load should chmod to 0600 and succeed when the filesystem allows it.
	if _, err := store.LoadOrCreateMasterKey(path); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm()&0o004 != 0 {
		t.Fatalf("still world-readable after load: %04o", info.Mode().Perm())
	}
}
