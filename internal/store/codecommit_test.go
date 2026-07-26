package store_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

func openCodeCommitStore(t *testing.T) *store.Store {
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
	if err := st.EnsureCodeCommitSchema(); err != nil {
		t.Fatal(err)
	}
	return st
}

func TestCodeCommitRepositoryCRUD(t *testing.T) {
	st := openCodeCommitStore(t)
	account := "000000000001"
	region := store.DefaultCodeCommitRegion

	repo, err := st.CreateCodeCommitRepository(account, region, "lab-repo", "demo")
	if err != nil {
		t.Fatal(err)
	}
	if repo.RepositoryID == "" || repo.ARN == "" || repo.DefaultBranch != "main" {
		t.Fatalf("repo=%+v", repo)
	}
	if _, err := st.CreateCodeCommitRepository(account, region, "lab-repo", ""); err != store.ErrCodeCommitRepoExists {
		t.Fatalf("dup err=%v", err)
	}
	if _, err := st.CreateCodeCommitRepository(account, region, "bad.git", ""); err != store.ErrCodeCommitInvalidInput {
		t.Fatalf("invalid name err=%v", err)
	}

	got, err := st.GetCodeCommitRepository(account, region, "lab-repo")
	if err != nil {
		t.Fatal(err)
	}
	if got.RepositoryName != "lab-repo" || got.Description != "demo" {
		t.Fatalf("get=%+v", got)
	}

	list, err := st.ListCodeCommitRepositories(account)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || list[0].RepositoryName != "lab-repo" {
		t.Fatalf("list=%+v", list)
	}

	id, err := st.DeleteCodeCommitRepository(account, "lab-repo")
	if err != nil {
		t.Fatal(err)
	}
	if id != repo.RepositoryID {
		t.Fatalf("deleted id=%q want %q", id, repo.RepositoryID)
	}
	if _, err := st.GetCodeCommitRepository(account, region, "lab-repo"); err != store.ErrCodeCommitRepoNotFound {
		t.Fatalf("after delete err=%v", err)
	}
	id, err = st.DeleteCodeCommitRepository(account, "lab-repo")
	if err != nil || id != "" {
		t.Fatalf("second delete id=%q err=%v", id, err)
	}
}

func TestCodeCommitPutGetFolderAndMaterialize(t *testing.T) {
	st := openCodeCommitStore(t)
	account := "000000000001"
	region := store.DefaultCodeCommitRegion

	if _, err := st.CreateCodeCommitRepository(account, region, "src-repo", ""); err != nil {
		t.Fatal(err)
	}

	put, err := st.PutCodeCommitFile(account, region, "src-repo", "main", "README.md", []byte("# hi"), "")
	if err != nil {
		t.Fatal(err)
	}
	if put.BlobID == "" || put.CommitID == "" {
		t.Fatalf("put=%+v", put)
	}

	_, err = st.PutCodeCommitFile(account, region, "src-repo", "main", "src/app.go", []byte("package main"), put.CommitID)
	if err != nil {
		t.Fatal(err)
	}

	_, err = st.PutCodeCommitFile(account, region, "src-repo", "main", "src/app.go", []byte("package main\n"), "stale")
	if err != store.ErrCodeCommitParentOutdated {
		t.Fatalf("stale parent err=%v", err)
	}

	file, err := st.GetCodeCommitFile(account, region, "src-repo", "README.md")
	if err != nil {
		t.Fatal(err)
	}
	if string(file.FileContent) != "# hi" || file.FilePath != "README.md" {
		t.Fatalf("file=%+v", file)
	}

	folder, err := st.GetCodeCommitFolder(account, region, "src-repo", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(folder.Files) != 1 || folder.Files[0].RelativePath != "README.md" {
		t.Fatalf("root files=%+v", folder.Files)
	}
	if len(folder.SubFolders) != 1 || folder.SubFolders[0].RelativePath != "src" {
		t.Fatalf("root folders=%+v", folder.SubFolders)
	}

	tree, err := st.ExportCodeCommitRepoTree(account, "src-repo")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(tree, "README.md")); err != nil {
		t.Fatalf("export tree missing README: %v", err)
	}

	dest := filepath.Join(t.TempDir(), "clone")
	if err := st.MaterializeCodeCommitRepo(account, "src-repo", dest); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(dest, "src", "app.go"))
	if err != nil {
		t.Fatal(err)
	}
	if string(raw) != "package main" {
		t.Fatalf("materialized=%q", raw)
	}

	_, err = st.BatchPutCodeCommitFiles(account, region, "src-repo", "main", []store.CodeCommitFileInput{
		{FilePath: "a.txt", FileContent: []byte("a")},
		{FilePath: "b.txt", FileContent: []byte("b")},
	}, "")
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"a.txt", "b.txt"} {
		if _, err := st.GetCodeCommitFile(account, region, "src-repo", name); err != nil {
			t.Fatalf("batch get %s: %v", name, err)
		}
	}

	if _, err := st.PutCodeCommitFile(account, region, "src-repo", "main", "../escape", []byte("x"), ""); err != store.ErrCodeCommitPathEscape {
		t.Fatalf("escape err=%v", err)
	}
}
