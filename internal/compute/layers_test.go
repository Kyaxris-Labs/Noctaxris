package compute_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Kyaxris-Labs/Noctaxris/internal/compute"
)

func TestMergeLayerDirs(t *testing.T) {
	root := t.TempDir()
	layer1 := filepath.Join(root, "layer1")
	layer2 := filepath.Join(root, "layer2")
	dest := filepath.Join(root, "merged")
	if err := os.MkdirAll(filepath.Join(layer1, "python"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(layer1, "python", "a.py"), []byte("a=1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(layer2, "python"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(layer2, "python", "b.py"), []byte("b=2\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := compute.MergeLayerDirs([]string{layer1, layer2}, dest); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dest, "python", "a.py")); err != nil {
		t.Fatalf("missing a.py: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dest, "python", "b.py")); err != nil {
		t.Fatalf("missing b.py: %v", err)
	}
}
