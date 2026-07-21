package compute

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

// MergeLayerDirs overlays layer unpacked directories into destDir (later layers win).
func MergeLayerDirs(layerPaths []string, destDir string) error {
	if len(layerPaths) == 0 {
		return nil
	}
	if err := os.RemoveAll(destDir); err != nil {
		return fmt.Errorf("compute: clear merged layer dir: %w", err)
	}
	if err := os.MkdirAll(destDir, store.LabSharedDirMode); err != nil {
		return fmt.Errorf("compute: mkdir merged layer dir: %w", err)
	}
	if err := os.Chmod(destDir, store.LabSharedDirMode); err != nil {
		return fmt.Errorf("compute: chmod merged layer dir: %w", err)
	}
	for _, src := range layerPaths {
		src = strings.TrimSpace(src)
		if src == "" {
			continue
		}
		if err := copyTree(src, destDir); err != nil {
			return err
		}
	}
	return nil
}

func copyTree(srcRoot, destRoot string) error {
	srcAbs, err := filepath.Abs(srcRoot)
	if err != nil {
		return err
	}
	destAbs, err := filepath.Abs(destRoot)
	if err != nil {
		return err
	}
	info, err := os.Stat(srcAbs)
	if err != nil {
		return fmt.Errorf("compute: layer path %s: %w", srcRoot, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("compute: layer path %s is not a directory", srcRoot)
	}
	return filepath.WalkDir(srcAbs, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, err := filepath.Rel(srcAbs, path)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}
		target := filepath.Join(destAbs, rel)
		if d.IsDir() {
			if err := os.MkdirAll(target, store.LabSharedDirMode); err != nil {
				return err
			}
			return os.Chmod(target, store.LabSharedDirMode)
		}
		if err := os.MkdirAll(filepath.Dir(target), store.LabSharedDirMode); err != nil {
			return err
		}
		if err := os.Chmod(filepath.Dir(target), store.LabSharedDirMode); err != nil {
			return err
		}
		return copyFile(path, target)
	})
}

func copyFile(src, dest string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dest, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, store.LabSharedFileMode)
	if err != nil {
		return err
	}
	defer out.Close()
	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	if err := out.Close(); err != nil {
		return err
	}
	return os.Chmod(dest, store.LabSharedFileMode)
}
