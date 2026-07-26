package store_test

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
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
	if p.EnvVarsJSON == "" {
		t.Fatalf("expected EnvVarsJSON default, got empty")
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

	b, err := st.StartCodeBuildBuild(account, region, store.StartCodeBuildBuildOpts{
		ProjectName: "lab-proj",
	})
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

func TestCodeBuildListBatchUpdateDelete(t *testing.T) {
	st := openCodeBuildStore(t)
	account := "000000000002"
	region := store.DefaultCodeBuildRegion
	role := "arn:aws:iam::" + account + ":role/CodeBuildRole"

	for _, name := range []string{"alpha", "beta"} {
		_, err := st.CreateCodeBuildProject(account, region, store.CreateCodeBuildProjectInput{
			Name:        name,
			ServiceRole: role,
			SourceType:  "NO_SOURCE",
			Buildspec:   "echo " + name,
			Image:       "alpine:3.20",
		})
		if err != nil {
			t.Fatal(err)
		}
	}

	listed, err := st.ListCodeBuildProjects(account)
	if err != nil {
		t.Fatal(err)
	}
	if len(listed) != 2 || listed[0].Name != "alpha" || listed[1].Name != "beta" {
		t.Fatalf("listed=%+v", listed)
	}

	batch, err := st.BatchGetCodeBuildProjects(account, []string{"beta", "missing", "alpha"})
	if err != nil {
		t.Fatal(err)
	}
	if len(batch) != 2 || batch[0].Name != "beta" || batch[1].Name != "alpha" {
		t.Fatalf("batch=%+v", batch)
	}

	updated, err := st.UpdateCodeBuildProject(account, region, store.UpdateCodeBuildProjectInput{
		Name:        "alpha",
		Description: "updated",
		ServiceRole: role,
		SourceType:  "NO_SOURCE",
		Buildspec:   "echo updated",
		Image:       "alpine:3.21",
		Artifacts:   map[string]any{"type": "NO_ARTIFACTS"},
		EnvVars:     []store.CodeBuildEnvVar{{Name: "FOO", Value: "bar"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if updated.Description != "updated" || updated.Image != "alpine:3.21" || updated.Buildspec != "echo updated" {
		t.Fatalf("updated=%+v", updated)
	}
	var env []store.CodeBuildEnvVar
	if err := json.Unmarshal([]byte(updated.EnvVarsJSON), &env); err != nil || len(env) != 1 || env[0].Name != "FOO" {
		t.Fatalf("env=%s err=%v", updated.EnvVarsJSON, err)
	}

	got, err := st.GetCodeBuildProject(account, "alpha")
	if err != nil || got.Description != "updated" {
		t.Fatalf("get after update=%+v err=%v", got, err)
	}

	if err := st.DeleteCodeBuildProject(account, "beta"); err != nil {
		t.Fatal(err)
	}
	if err := st.DeleteCodeBuildProject(account, "beta"); err != store.ErrCodeBuildProjectNotFound {
		t.Fatalf("second delete err=%v", err)
	}
	listed, err = st.ListCodeBuildProjects(account)
	if err != nil || len(listed) != 1 || listed[0].Name != "alpha" {
		t.Fatalf("after delete listed=%+v err=%v", listed, err)
	}
}

func TestCodeBuildEnvVarsAndStartOverride(t *testing.T) {
	st := openCodeBuildStore(t)
	account := "000000000003"
	region := store.DefaultCodeBuildRegion

	p, err := st.CreateCodeBuildProject(account, region, store.CreateCodeBuildProjectInput{
		Name:        "env-proj",
		ServiceRole: "arn:aws:iam::" + account + ":role/CodeBuildRole",
		SourceType:  "NO_SOURCE",
		Buildspec:   "echo base",
		Image:       "alpine:3.20",
		EnvVars: []store.CodeBuildEnvVar{
			{Name: "PROJECT_VAR", Value: "from-project"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	var projectEnv []store.CodeBuildEnvVar
	if err := json.Unmarshal([]byte(p.EnvVarsJSON), &projectEnv); err != nil {
		t.Fatal(err)
	}
	if len(projectEnv) != 1 || projectEnv[0].Value != "from-project" {
		t.Fatalf("project env=%v", projectEnv)
	}

	b, err := st.StartCodeBuildBuild(account, region, store.StartCodeBuildBuildOpts{
		ProjectName:       "env-proj",
		BuildspecOverride: "echo override",
		EnvOverride: []store.CodeBuildEnvVar{
			{Name: "OVERRIDE", Value: "yes"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if b.Buildspec != "echo override" {
		t.Fatalf("buildspec=%q", b.Buildspec)
	}
	var buildEnv []store.CodeBuildEnvVar
	if err := json.Unmarshal([]byte(b.EnvVarsJSON), &buildEnv); err != nil {
		t.Fatal(err)
	}
	if len(buildEnv) != 1 || buildEnv[0].Name != "OVERRIDE" {
		t.Fatalf("build env=%v", buildEnv)
	}

	fetched, err := st.BatchGetCodeBuildBuilds(account, []string{b.ID})
	if err != nil || len(fetched) != 1 {
		t.Fatalf("fetched=%+v err=%v", fetched, err)
	}
	if fetched[0].EnvVarsJSON != b.EnvVarsJSON {
		t.Fatalf("persisted env mismatch: %q vs %q", fetched[0].EnvVarsJSON, b.EnvVarsJSON)
	}
}

func TestCodeBuildStopBuildAndLogs(t *testing.T) {
	st := openCodeBuildStore(t)
	account := "000000000004"
	region := store.DefaultCodeBuildRegion

	_, err := st.CreateCodeBuildProject(account, region, store.CreateCodeBuildProjectInput{
		Name:        "stop-proj",
		ServiceRole: "arn:aws:iam::" + account + ":role/CodeBuildRole",
		SourceType:  "NO_SOURCE",
		Buildspec:   "echo stop",
		Image:       "alpine:3.20",
	})
	if err != nil {
		t.Fatal(err)
	}

	b, err := st.StartCodeBuildBuild(account, region, store.StartCodeBuildBuildOpts{ProjectName: "stop-proj"})
	if err != nil {
		t.Fatal(err)
	}
	stopped, err := st.StopCodeBuildBuild(account, b.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stopped.BuildStatus != store.CodeBuildStatusStopped {
		t.Fatalf("status=%q", stopped.BuildStatus)
	}
	if stopped.EndTime == "" {
		t.Fatal("expected end time after stop")
	}

	again, err := st.StopCodeBuildBuild(account, b.ID)
	if err != nil {
		t.Fatal(err)
	}
	if again.BuildStatus != store.CodeBuildStatusStopped || again.EndTime != stopped.EndTime {
		t.Fatalf("idempotent stop=%+v", again)
	}

	if err := st.SetCodeBuildBuildLogs(account, b.ID, "hello logs", "/aws/codebuild/stop-proj", "stream-1"); err != nil {
		t.Fatal(err)
	}
	got, err := st.BatchGetCodeBuildBuilds(account, []string{b.ID})
	if err != nil || len(got) != 1 {
		t.Fatalf("got=%+v err=%v", got, err)
	}
	if got[0].LogsText != "hello logs" || got[0].LogGroup != "/aws/codebuild/stop-proj" || got[0].LogStream != "stream-1" {
		t.Fatalf("logs fields=%+v", got[0])
	}

	if err := st.SetCodeBuildBuildLogs(account, "missing-id", "x", "g", "s"); err != store.ErrCodeBuildBuildNotFound {
		t.Fatalf("missing logs err=%v", err)
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

func TestCodeBuildOverrideLock(t *testing.T) {
	st := openCodeBuildStore(t)
	account := "000000000005"
	region := store.DefaultCodeBuildRegion
	locked := false
	_, err := st.CreateCodeBuildProject(account, region, store.CreateCodeBuildProjectInput{
		Name:            "locked-proj",
		ServiceRole:     "arn:aws:iam::" + account + ":role/CodeBuildRole",
		SourceType:      "NO_SOURCE",
		Buildspec:       "echo locked",
		Image:           "alpine:3.20",
		OverrideAllowed: &locked,
	})
	if err != nil {
		t.Fatal(err)
	}

	_, err = st.StartCodeBuildBuild(account, region, store.StartCodeBuildBuildOpts{
		ProjectName:       "locked-proj",
		BuildspecOverride: "echo no",
	})
	if !errors.Is(err, store.ErrCodeBuildInvalidInput) {
		t.Fatalf("buildspec override err=%v", err)
	}

	_, err = st.StartCodeBuildBuild(account, region, store.StartCodeBuildBuildOpts{
		ProjectName: "locked-proj",
		EnvOverride: []store.CodeBuildEnvVar{{Name: "X", Value: "1"}},
	})
	if !errors.Is(err, store.ErrCodeBuildInvalidInput) {
		t.Fatalf("env override err=%v", err)
	}

	b, err := st.StartCodeBuildBuild(account, region, store.StartCodeBuildBuildOpts{
		ProjectName: "locked-proj",
	})
	if err != nil {
		t.Fatal(err)
	}
	if b.Buildspec != "echo locked" {
		t.Fatalf("buildspec=%q", b.Buildspec)
	}
}

func TestCodeBuildStartBuildBatchMatrix(t *testing.T) {
	st := openCodeBuildStore(t)
	account := "000000000006"
	region := store.DefaultCodeBuildRegion
	buildspec := `{"version":"0.2","batch":{"build-list":[{"identifier":"one","env":{"variables":{"A":"1"}}},{"identifier":"two","env":{"variables":{"A":"2"}}}]},"phases":{"build":{"commands":["echo ok"]}}}`
	_, err := st.CreateCodeBuildProject(account, region, store.CreateCodeBuildProjectInput{
		Name:        "batch-proj",
		ServiceRole: "arn:aws:iam::" + account + ":role/CodeBuildRole",
		SourceType:  "NO_SOURCE",
		Buildspec:   buildspec,
		Image:       "alpine:3.20",
	})
	if err != nil {
		t.Fatal(err)
	}

	batch, builds, err := st.StartCodeBuildBuildBatch(account, region, store.StartCodeBuildBuildBatchOpts{
		ProjectName: "batch-proj",
	})
	if err != nil {
		t.Fatal(err)
	}
	if batch.ID == "" || batch.BuildBatchStatus != store.CodeBuildStatusInProgress {
		t.Fatalf("batch=%+v", batch)
	}
	if len(builds) != 2 || len(batch.ChildBuildIDs) != 2 {
		t.Fatalf("builds=%d children=%v", len(builds), batch.ChildBuildIDs)
	}
	for _, b := range builds {
		if b.BatchID != batch.ID {
			t.Fatalf("child batch id=%q want %q", b.BatchID, batch.ID)
		}
	}

	// Explicit matrix children
	batch2, builds2, err := st.StartCodeBuildBuildBatch(account, region, store.StartCodeBuildBuildBatchOpts{
		ProjectName: "batch-proj",
		Children: []store.CodeBuildBatchChild{
			{Identifier: "x", EnvVars: []store.CodeBuildEnvVar{{Name: "K", Value: "v1"}}},
			{Identifier: "y", EnvVars: []store.CodeBuildEnvVar{{Name: "K", Value: "v2"}}},
			{Identifier: "z", EnvVars: []store.CodeBuildEnvVar{{Name: "K", Value: "v3"}}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(builds2) != 3 || batch2.ProjectName != "batch-proj" {
		t.Fatalf("batch2 builds=%d", len(builds2))
	}
	var env []store.CodeBuildEnvVar
	if err := json.Unmarshal([]byte(builds2[0].EnvVarsJSON), &env); err != nil || len(env) != 1 || env[0].Value != "v1" {
		t.Fatalf("child0 env=%s err=%v", builds2[0].EnvVarsJSON, err)
	}
}

func TestCodeBuildS3ArtifactsPublish(t *testing.T) {
	st := openCodeBuildStore(t)
	account := "000000000007"
	region := store.DefaultCodeBuildRegion

	if _, err := st.CreateBucket(account, "cb-artifacts"); err != nil {
		t.Fatal(err)
	}
	_, err := st.CreateCodeBuildProject(account, region, store.CreateCodeBuildProjectInput{
		Name:        "art-proj",
		ServiceRole: "arn:aws:iam::" + account + ":role/CodeBuildRole",
		SourceType:  "NO_SOURCE",
		Buildspec:   "echo art",
		Image:       "alpine:3.20",
		Artifacts: map[string]any{
			"type":      "S3",
			"location":  "cb-artifacts",
			"path":      "out",
			"name":      "build.zip",
			"packaging": "ZIP",
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	target, ok := store.ParseCodeBuildS3Artifacts(`{"type":"S3","location":"cb-artifacts","path":"out","name":"build.zip","packaging":"ZIP"}`)
	if !ok || target.Bucket != "cb-artifacts" {
		t.Fatalf("parse target=%+v ok=%v", target, ok)
	}
	key := store.CodeBuildArtifactObjectKey(target, "bid-1")
	if key != "out/bid-1/build.zip" {
		t.Fatalf("key=%q", key)
	}

	b, err := st.StartCodeBuildBuild(account, region, store.StartCodeBuildBuildOpts{ProjectName: "art-proj"})
	if err != nil {
		t.Fatal(err)
	}
	if err := st.SetCodeBuildBuildLogs(account, b.ID, "artifact logs", "/aws/codebuild/art-proj", b.ID); err != nil {
		t.Fatal(err)
	}
	b.LogsText = "artifact logs"
	loc, err := st.PublishCodeBuildBuildArtifacts(account, b, nil)
	if err != nil {
		t.Fatal(err)
	}
	wantLoc := "cb-artifacts/out/" + b.ID + "/build.zip"
	if loc != wantLoc {
		t.Fatalf("loc=%q want %q", loc, wantLoc)
	}
	got, err := st.BatchGetCodeBuildBuilds(account, []string{b.ID})
	if err != nil || len(got) != 1 || got[0].ArtifactLocation != wantLoc {
		t.Fatalf("persisted=%+v err=%v", got, err)
	}
	_, data, err := st.GetObject(account, "cb-artifacts", "out/"+b.ID+"/build.zip")
	if err != nil || len(data) < 4 || string(data[:2]) != "PK" {
		t.Fatalf("zip object err=%v len=%d", err, len(data))
	}
}

func TestCodeBuildCodeCommitSourceResolveAndMaterialize(t *testing.T) {
	st := openCodeBuildStore(t)
	account := "000000000008"
	region := store.DefaultCodeBuildRegion

	name, err := store.ParseCodeBuildCodeCommitLocation("lab-src")
	if err != nil || name != "lab-src" {
		t.Fatalf("name parse=%q err=%v", name, err)
	}
	arn := store.CodeCommitRepositoryARN(region, account, "lab-src")
	name, err = store.ParseCodeBuildCodeCommitLocation(arn)
	if err != nil || name != "lab-src" {
		t.Fatalf("arn parse=%q err=%v", name, err)
	}

	_, err = st.CreateCodeBuildProject(account, region, store.CreateCodeBuildProjectInput{
		Name:        "cc-missing",
		ServiceRole: "arn:aws:iam::" + account + ":role/CodeBuildRole",
		SourceType:  "CODECOMMIT",
		SourceLoc:   "missing-repo",
		Buildspec:   `{"version":"0.2","phases":{"build":{"commands":["cat README.md"]}}}`,
		Image:       "alpine:3.20",
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = st.StartCodeBuildBuild(account, region, store.StartCodeBuildBuildOpts{ProjectName: "cc-missing"})
	if !errors.Is(err, store.ErrCodeBuildInvalidInput) {
		t.Fatalf("missing repo start err=%v", err)
	}

	if _, err := st.CreateCodeCommitRepository(account, region, "lab-src", "lab"); err != nil {
		t.Fatal(err)
	}
	if _, err := st.PutCodeCommitFile(account, region, "lab-src", "main", "README.md", []byte("hello-cc"), ""); err != nil {
		t.Fatal(err)
	}
	if _, err := st.PutCodeCommitFile(account, region, "lab-src", "main", "buildspec.yml", []byte("version: 0.2\nphases:\n  build:\n    commands:\n      - cat README.md\n"), ""); err != nil {
		t.Fatal(err)
	}

	p, err := st.CreateCodeBuildProject(account, region, store.CreateCodeBuildProjectInput{
		Name:        "cc-proj",
		ServiceRole: "arn:aws:iam::" + account + ":role/CodeBuildRole",
		SourceType:  "CODECOMMIT",
		SourceLoc:   arn,
		Image:       "alpine:3.20",
		VpcConfig:   map[string]any{"vpcId": "vpc-lab", "subnets": []any{"subnet-1"}},
		Cache:       map[string]any{"type": "NO_CACHE"},
		Fleet:       map[string]any{"fleetArn": "arn:aws:codebuild:us-east-1:" + account + ":fleet/lab"},
		ReportGroupArns: []string{"arn:aws:codebuild:us-east-1:" + account + ":report-group/lab"},
		SecondarySources: []any{
			map[string]any{"type": "NO_SOURCE", "sourceIdentifier": "extra"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if p.SourceType != "CODECOMMIT" || p.VpcConfigJSON == "" || p.CacheJSON == "" {
		t.Fatalf("project stubs=%+v", p)
	}
	got, err := st.GetCodeBuildProject(account, "cc-proj")
	if err != nil {
		t.Fatal(err)
	}
	if got.ReportArnsJSON == "" || got.SecondarySourcesJSON == "" || got.FleetJSON == "" {
		t.Fatalf("persisted stubs=%+v", got)
	}

	dest := t.TempDir()
	repo, err := st.MaterializeCodeBuildCodeCommitSource(account, "lab-src", dest)
	if err != nil || repo != "lab-src" {
		t.Fatalf("materialize repo=%q err=%v", repo, err)
	}
	raw, err := os.ReadFile(filepath.Join(dest, "README.md"))
	if err != nil || string(raw) != "hello-cc" {
		t.Fatalf("materialized README=%q err=%v", raw, err)
	}

	b, err := st.StartCodeBuildBuild(account, region, store.StartCodeBuildBuildOpts{ProjectName: "cc-proj"})
	if err != nil {
		t.Fatal(err)
	}
	spec, err := st.ResolveCodeBuildBuildspec(account, b)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(spec, "cat README.md") {
		t.Fatalf("resolved buildspec=%q", spec)
	}
	cmds := store.ExtractBuildspecCommands(spec)
	shell, err := store.BuildCodeBuildCodeCommitShell(dest, cmds)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(shell, "/codebuild/src") || !strings.Contains(shell, "base64 -d") {
		t.Fatalf("shell=%q", shell)
	}
}

func TestCodeBuildPublishWorkspaceTarArtifacts(t *testing.T) {
	st := openCodeBuildStore(t)
	account := "000000000009"
	region := store.DefaultCodeBuildRegion
	_, err := st.CreateCodeBuildProject(account, region, store.CreateCodeBuildProjectInput{
		Name:        "ws-art-proj",
		ServiceRole: "arn:aws:iam::" + account + ":role/CodeBuildRole",
		SourceType:  "NO_SOURCE",
		Buildspec:   "echo art",
		Image:       "alpine:3.20",
		Artifacts: map[string]any{
			"type":      "S3",
			"location":  "cb-ws-artifacts",
			"name":      "build.zip",
			"packaging": "ZIP",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	b, err := st.StartCodeBuildBuild(account, region, store.StartCodeBuildBuildOpts{ProjectName: "ws-art-proj"})
	if err != nil {
		t.Fatal(err)
	}
	b.LogsText = "logs"
	loc, err := st.PublishCodeBuildBuildArtifacts(account, b, []byte("fake-tar-bytes"))
	if err != nil {
		t.Fatal(err)
	}
	if loc == "" {
		t.Fatal("empty location")
	}
	_, data, err := st.GetObject(account, "cb-ws-artifacts", b.ID+"/build.zip")
	if err != nil {
		t.Fatal(err)
	}
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, f := range zr.File {
		names = append(names, f.Name)
	}
	if !slices.Contains(names, "codebuild-src.tar") || !slices.Contains(names, "build.log") {
		t.Fatalf("zip entries=%v", names)
	}
}
