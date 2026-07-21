package store

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// Lab-shared compute modes: the API runs as UID 65532 while nested DinD function
// containers do not share that UID. Bind-mounted code/event trees must stay
// other-readable (dirs executable) or Invoke fails with container exit 1.
const (
	LabSharedDirMode  = 0o755
	LabSharedFileMode = 0o644
)

func ensureLabSharedDir(path string) error {
	if err := os.MkdirAll(path, LabSharedDirMode); err != nil {
		return err
	}
	return os.Chmod(path, LabSharedDirMode)
}

// ensureLabSharedAncestors chmods each directory from absPath up through
// dataRoot/lambda so nested containers can traverse the bind mount.
func ensureLabSharedAncestors(dataRoot, absPath string) error {
	dataAbs, err := filepath.Abs(dataRoot)
	if err != nil {
		return err
	}
	lambdaRoot := filepath.Join(dataAbs, "lambda")
	cur, err := filepath.Abs(absPath)
	if err != nil {
		return err
	}
	sep := string(os.PathSeparator)
	if cur != lambdaRoot && !strings.HasPrefix(cur, lambdaRoot+sep) {
		return os.Chmod(cur, LabSharedDirMode)
	}
	for {
		if err := os.Chmod(cur, LabSharedDirMode); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("chmod lab-shared dir %s: %w", cur, err)
		}
		if cur == lambdaRoot {
			return nil
		}
		parent := filepath.Dir(cur)
		if parent == cur {
			return nil
		}
		cur = parent
	}
}

func chmodLabSharedTree(root string) error {
	return filepath.WalkDir(root, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		mode := os.FileMode(LabSharedFileMode)
		if d.IsDir() {
			mode = os.FileMode(LabSharedDirMode)
		}
		if err := os.Chmod(path, mode); err != nil {
			return fmt.Errorf("chmod lab-shared %s: %w", path, err)
		}
		return nil
	})
}
