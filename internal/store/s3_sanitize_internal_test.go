package store

import (
	"errors"
	"path/filepath"
	"testing"
)

func TestEnsureS3PathWithinBucket(t *testing.T) {
	root := filepath.Join(t.TempDir(), "s3", "000000000001", "safe-bucket")
	inside := filepath.Join(root, "ok", "obj.txt")
	if err := ensureS3PathWithinBucket(root, inside); err != nil {
		t.Fatalf("inside path: %v", err)
	}
	if err := ensureS3PathWithinBucket(root, root); err != nil {
		t.Fatalf("bucket root: %v", err)
	}
	outside := filepath.Join(root, "..", "..", "master.key")
	if err := ensureS3PathWithinBucket(root, outside); !errors.Is(err, ErrInvalidObjectKey) {
		t.Fatalf("escape err=%v want InvalidObjectKey", err)
	}
	sibling := filepath.Join(filepath.Dir(root), "other-bucket", "obj.txt")
	if err := ensureS3PathWithinBucket(root, sibling); !errors.Is(err, ErrInvalidObjectKey) {
		t.Fatalf("sibling bucket err=%v want InvalidObjectKey", err)
	}
}

func TestS3ObjectAbsPathRejectsEscape(t *testing.T) {
	dataRoot := t.TempDir()
	account := "000000000001"
	bucket := "safe-bucket"

	rel, abs, err := s3ObjectAbsPath(dataRoot, account, bucket, "dir/obj.txt")
	if err != nil {
		t.Fatal(err)
	}
	wantRel := filepath.Join("s3", account, bucket, filepath.FromSlash("dir/obj.txt"))
	if rel != wantRel {
		t.Fatalf("rel=%q want=%q", rel, wantRel)
	}
	wantAbs := filepath.Clean(filepath.Join(dataRoot, wantRel))
	if abs != wantAbs {
		t.Fatalf("abs=%q want=%q", abs, wantAbs)
	}

	// Defense in depth: even if a key somehow joined outside the bucket, Rel fails closed.
	escapedKey := filepath.Join("..", "..", "master.key")
	escapedKey = filepath.ToSlash(escapedKey)
	_, _, err = s3ObjectAbsPath(dataRoot, account, bucket, escapedKey)
	if !errors.Is(err, ErrInvalidObjectKey) {
		t.Fatalf("escaped join err=%v want InvalidObjectKey", err)
	}
}
