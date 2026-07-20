package store_test

import (
	"path/filepath"
	"testing"

	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

func openCodeBuildStore(t *testing.T) *store.Store {
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
	if err := st.EnsureCodeBuildSchema(); err != nil {
		t.Fatal(err)
	}
	return st
}

func TestCodeBuildProjectAndBuildLifecycle(t *testing.T) {
	st := openCodeBuildStore(t)
	account := "000000000001"
	region := store.DefaultCodeBuildRegion

	p, err := st.CreateCodeBuildProject(account, region, store.CreateCodeBuildProjectInput{
		Name:        "lab-proj",
		Description: "lab",
		ServiceRole: "arn:aws:iam::" + account + ":role/CodeBuildRole",
		SourceType:  "NO_SOURCE",
		Buildspec:   `{"version":"0.2","phases":{"build":{"commands":["echo ok"]}}}`,
		Image:       "alpine:3.20",
		Artifacts:   map[string]any{"type": "NO_ARTIFACTS"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if p.ARN == "" || p.Name != "lab-proj" {
		t.Fatalf("project=%+v", p)
	}

	_, err = st.CreateCodeBuildProject(account, region, store.CreateCodeBuildProjectInput{
		Name:        "lab-proj",
		ServiceRole: p.ServiceRole,
		SourceType:  "NO_SOURCE",
		Buildspec:   "echo",
		Image:       "alpine:3.20",
	})
	if err != store.ErrCodeBuildProjectExists {
		t.Fatalf("dup err=%v", err)
	}

	b, err := st.StartCodeBuildBuild(account, region, "lab-proj", "")
	if err != nil {
		t.Fatal(err)
	}
	if b.BuildStatus != store.CodeBuildStatusInProgress {
		t.Fatalf("status=%q", b.BuildStatus)
	}
	if err := st.SetCodeBuildBuildRuntime(account, b.ID, "cid-1", store.CodeBuildStatusSucceeded, "2026-01-01T00:00:00Z"); err != nil {
		t.Fatal(err)
	}

	got, err := st.BatchGetCodeBuildBuilds(account, []string{b.ID, "missing"})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].BuildStatus != store.CodeBuildStatusSucceeded {
		t.Fatalf("builds=%+v", got)
	}

	ids, err := st.ListCodeBuildBuilds(account, "lab-proj")
	if err != nil || len(ids) != 1 || ids[0] != b.ID {
		t.Fatalf("ids=%v err=%v", ids, err)
	}
}

func TestExtractBuildspecCommands(t *testing.T) {
	cmds := store.ExtractBuildspecCommands(`{"version":"0.2","phases":{"build":{"commands":["echo a","echo b"]}}}`)
	if len(cmds) != 2 || cmds[0] != "echo a" {
		t.Fatalf("cmds=%v", cmds)
	}
	yamlish := "version: 0.2\nphases:\n  build:\n    commands:\n      - echo yaml\n"
	cmds = store.ExtractBuildspecCommands(yamlish)
	if len(cmds) != 1 || cmds[0] != "echo yaml" {
		t.Fatalf("yaml cmds=%v", cmds)
	}
}
