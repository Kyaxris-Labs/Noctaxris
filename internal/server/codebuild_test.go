package server_test

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func mustCodeBuildJSON(t *testing.T, handler http.Handler, target string, payload map[string]any, now time.Time) *httptest.ResponseRecorder {
	t.Helper()
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	req := mustNewRequest(t, http.MethodPost, "http://127.0.0.1:4566/", raw)
	req.Header.Set("Content-Type", "application/x-amz-json-1.1")
	req.Header.Set("X-Amz-Target", "CodeBuild_20161006."+target)
	signHeader(t, req, raw, testAccessKey, testSecret, testRegion, "codebuild", now)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}

const codebuildTrustOK = `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":{"Service":"codebuild.amazonaws.com"},"Action":"sts:AssumeRole"}]}`
const codebuildTrustBad = `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":{"AWS":"arn:aws:iam::000000000001:root"},"Action":"sts:AssumeRole"}]}`

func TestCodeBuildCreateProjectPassRoleDeny(t *testing.T) {
	srv, _ := newTestServer(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	mustCreateIAMRole(t, handler, "cb-bad", codebuildTrustBad, now)
	roleARN := "arn:aws:iam::" + testAccountID + ":role/cb-bad"

	rec := mustCodeBuildJSON(t, handler, "CreateProject", map[string]any{
		"name":        "denied-proj",
		"serviceRole": roleARN,
		"source": map[string]any{
			"type":      "NO_SOURCE",
			"buildspec": `{"version":"0.2","phases":{"build":{"commands":["echo hi"]}}}`,
		},
		"environment": map[string]any{
			"type":  "LINUX_CONTAINER",
			"image": "alpine:3.20",
		},
		"artifacts": map[string]any{"type": "NO_ARTIFACTS"},
	}, now)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status=%d body=%q", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "not authorized to pass role to CodeBuild") {
		t.Fatalf("body=%q", rec.Body.String())
	}
}

func TestCodeBuildCreateAndListBuildsWithoutCompute(t *testing.T) {
	srv, _ := newTestServer(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	mustCreateIAMRole(t, handler, "cb-ok", codebuildTrustOK, now)
	roleARN := "arn:aws:iam::" + testAccountID + ":role/cb-ok"

	create := mustCodeBuildJSON(t, handler, "CreateProject", map[string]any{
		"name":        "lab-proj",
		"serviceRole": roleARN,
		"source": map[string]any{
			"type":      "NO_SOURCE",
			"buildspec": `{"version":"0.2","phases":{"build":{"commands":["echo hi"]}}}`,
		},
		"environment": map[string]any{
			"type":  "LINUX_CONTAINER",
			"image": "alpine:3.20",
		},
		"artifacts": map[string]any{"type": "NO_ARTIFACTS"},
	}, now)
	if create.Code != http.StatusOK {
		t.Fatalf("CreateProject status=%d body=%q", create.Code, create.Body.String())
	}

	start := mustCodeBuildJSON(t, handler, "StartBuild", map[string]any{
		"projectName": "lab-proj",
	}, now)
	if start.Code != http.StatusServiceUnavailable {
		t.Fatalf("StartBuild status=%d want 503 body=%q", start.Code, start.Body.String())
	}
	if !strings.Contains(start.Body.String(), "compute unavailable") {
		t.Fatalf("body=%q", start.Body.String())
	}

	list := mustCodeBuildJSON(t, handler, "ListBuilds", map[string]any{}, now)
	if list.Code != http.StatusOK {
		t.Fatalf("ListBuilds status=%d body=%q", list.Code, list.Body.String())
	}
}

func TestCodeBuildProjectCRUDAndEnvVars(t *testing.T) {
	srv, _ := newTestServer(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	mustCreateIAMRole(t, handler, "cb-crud", codebuildTrustOK, now)
	roleARN := "arn:aws:iam::" + testAccountID + ":role/cb-crud"

	create := mustCodeBuildJSON(t, handler, "CreateProject", map[string]any{
		"name":        "env-proj",
		"description": "with env",
		"serviceRole": roleARN,
		"source": map[string]any{
			"type":      "NO_SOURCE",
			"buildspec": `{"version":"0.2","phases":{"build":{"commands":["echo hi"]}}}`,
		},
		"environment": map[string]any{
			"type":  "LINUX_CONTAINER",
			"image": "alpine:3.20",
			"environmentVariables": []any{
				map[string]any{"name": "FOO", "value": "bar"},
			},
		},
		"artifacts": map[string]any{"type": "NO_ARTIFACTS"},
	}, now)
	if create.Code != http.StatusOK {
		t.Fatalf("CreateProject status=%d body=%q", create.Code, create.Body.String())
	}
	var createBody map[string]any
	if err := json.Unmarshal(create.Body.Bytes(), &createBody); err != nil {
		t.Fatal(err)
	}
	proj, _ := createBody["project"].(map[string]any)
	env, _ := proj["environment"].(map[string]any)
	vars, _ := env["environmentVariables"].([]any)
	if len(vars) != 1 {
		t.Fatalf("create env vars=%v", env["environmentVariables"])
	}

	list := mustCodeBuildJSON(t, handler, "ListProjects", map[string]any{}, now)
	if list.Code != http.StatusOK {
		t.Fatalf("ListProjects status=%d body=%q", list.Code, list.Body.String())
	}
	if !strings.Contains(list.Body.String(), "env-proj") {
		t.Fatalf("ListProjects body=%q", list.Body.String())
	}

	batch := mustCodeBuildJSON(t, handler, "BatchGetProjects", map[string]any{
		"names": []string{"env-proj", "missing-proj"},
	}, now)
	if batch.Code != http.StatusOK {
		t.Fatalf("BatchGetProjects status=%d body=%q", batch.Code, batch.Body.String())
	}
	var batchBody map[string]any
	if err := json.Unmarshal(batch.Body.Bytes(), &batchBody); err != nil {
		t.Fatal(err)
	}
	projects, _ := batchBody["projects"].([]any)
	if len(projects) != 1 {
		t.Fatalf("projects=%v", batchBody["projects"])
	}
	notFound, _ := batchBody["projectsNotFound"].([]any)
	if len(notFound) != 1 || notFound[0] != "missing-proj" {
		t.Fatalf("projectsNotFound=%v", batchBody["projectsNotFound"])
	}
	gotProj, _ := projects[0].(map[string]any)
	gotEnv, _ := gotProj["environment"].(map[string]any)
	gotVars, _ := gotEnv["environmentVariables"].([]any)
	if len(gotVars) != 1 {
		t.Fatalf("batch env vars=%v", gotEnv["environmentVariables"])
	}

	update := mustCodeBuildJSON(t, handler, "UpdateProject", map[string]any{
		"name":        "env-proj",
		"description": "updated",
		"serviceRole": roleARN,
		"source": map[string]any{
			"type":      "NO_SOURCE",
			"buildspec": `{"version":"0.2","phases":{"build":{"commands":["echo updated"]}}}`,
		},
		"environment": map[string]any{
			"type":  "LINUX_CONTAINER",
			"image": "alpine:3.20",
			"environmentVariables": []any{
				map[string]any{"name": "FOO", "value": "baz"},
				map[string]any{"name": "EXTRA", "value": "1"},
			},
		},
		"artifacts": map[string]any{"type": "NO_ARTIFACTS"},
	}, now)
	if update.Code != http.StatusOK {
		t.Fatalf("UpdateProject status=%d body=%q", update.Code, update.Body.String())
	}
	if !strings.Contains(update.Body.String(), `"value":"baz"`) || !strings.Contains(update.Body.String(), "EXTRA") {
		t.Fatalf("UpdateProject body=%q", update.Body.String())
	}

	del := mustCodeBuildJSON(t, handler, "DeleteProject", map[string]any{"name": "env-proj"}, now)
	if del.Code != http.StatusOK {
		t.Fatalf("DeleteProject status=%d body=%q", del.Code, del.Body.String())
	}

	batchAfter := mustCodeBuildJSON(t, handler, "BatchGetProjects", map[string]any{
		"names": []string{"env-proj"},
	}, now)
	if batchAfter.Code != http.StatusOK {
		t.Fatalf("BatchGetProjects after delete status=%d body=%q", batchAfter.Code, batchAfter.Body.String())
	}
	var afterBody map[string]any
	if err := json.Unmarshal(batchAfter.Body.Bytes(), &afterBody); err != nil {
		t.Fatal(err)
	}
	if projects, _ := afterBody["projects"].([]any); len(projects) != 0 {
		t.Fatalf("expected empty projects after delete, got %v", afterBody["projects"])
	}
}

func TestCodeBuildStartBuildEnvOverrideAndStopMissing(t *testing.T) {
	srv, _ := newTestServer(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	mustCreateIAMRole(t, handler, "cb-start", codebuildTrustOK, now)
	roleARN := "arn:aws:iam::" + testAccountID + ":role/cb-start"

	create := mustCodeBuildJSON(t, handler, "CreateProject", map[string]any{
		"name":        "start-proj",
		"serviceRole": roleARN,
		"source": map[string]any{
			"type":      "NO_SOURCE",
			"buildspec": `{"version":"0.2","phases":{"build":{"commands":["echo hi"]}}}`,
		},
		"environment": map[string]any{
			"type":  "LINUX_CONTAINER",
			"image": "alpine:3.20",
			"environmentVariables": []any{
				map[string]any{"name": "BASE", "value": "1"},
			},
		},
		"artifacts": map[string]any{"type": "NO_ARTIFACTS"},
	}, now)
	if create.Code != http.StatusOK {
		t.Fatalf("CreateProject status=%d body=%q", create.Code, create.Body.String())
	}

	// Env override is parsed at the API layer; without DockerHost StartBuild still returns 503.
	start := mustCodeBuildJSON(t, handler, "StartBuild", map[string]any{
		"projectName": "start-proj",
		"environmentVariablesOverride": []any{
			map[string]any{"name": "OVERRIDE", "value": "yes"},
		},
	}, now)
	if start.Code != http.StatusServiceUnavailable {
		t.Fatalf("StartBuild status=%d want 503 body=%q", start.Code, start.Body.String())
	}
	if !strings.Contains(start.Body.String(), "compute unavailable") {
		t.Fatalf("body=%q", start.Body.String())
	}

	stop := mustCodeBuildJSON(t, handler, "StopBuild", map[string]any{
		"id": "does-not-exist",
	}, now)
	if stop.Code != http.StatusBadRequest {
		t.Fatalf("StopBuild status=%d want 400 body=%q", stop.Code, stop.Body.String())
	}
	if !strings.Contains(stop.Body.String(), "ResourceNotFoundException") {
		t.Fatalf("StopBuild body=%q", stop.Body.String())
	}
}

func TestCodeBuildOverrideLockRejectsStartBuild(t *testing.T) {
	srv, _ := newTestServer(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	mustCreateIAMRole(t, handler, "cb-lock", codebuildTrustOK, now)
	roleARN := "arn:aws:iam::" + testAccountID + ":role/cb-lock"

	create := mustCodeBuildJSON(t, handler, "CreateProject", map[string]any{
		"name":        "lock-proj",
		"serviceRole": roleARN,
		"source": map[string]any{
			"type":          "NO_SOURCE",
			"buildspec":     `{"version":"0.2","phases":{"build":{"commands":["echo hi"]}}}`,
			"allowOverride": false,
		},
		"environment": map[string]any{
			"type":  "LINUX_CONTAINER",
			"image": "alpine:3.20",
		},
		"artifacts": map[string]any{"type": "NO_ARTIFACTS"},
	}, now)
	if create.Code != http.StatusOK {
		t.Fatalf("CreateProject status=%d body=%q", create.Code, create.Body.String())
	}
	var createBody map[string]any
	if err := json.Unmarshal(create.Body.Bytes(), &createBody); err != nil {
		t.Fatal(err)
	}
	proj, _ := createBody["project"].(map[string]any)
	src, _ := proj["source"].(map[string]any)
	if src["allowOverride"] != false {
		t.Fatalf("allowOverride=%v", src["allowOverride"])
	}

	denied := mustCodeBuildJSON(t, handler, "StartBuild", map[string]any{
		"projectName":       "lock-proj",
		"buildspecOverride": "echo denied",
	}, now)
	if denied.Code != http.StatusBadRequest {
		t.Fatalf("StartBuild status=%d want 400 body=%q", denied.Code, denied.Body.String())
	}
	if !strings.Contains(denied.Body.String(), "InvalidInputException") {
		t.Fatalf("body=%q", denied.Body.String())
	}

	deniedEnv := mustCodeBuildJSON(t, handler, "StartBuild", map[string]any{
		"projectName": "lock-proj",
		"environmentVariablesOverride": []any{
			map[string]any{"name": "X", "value": "1"},
		},
	}, now)
	if deniedEnv.Code != http.StatusBadRequest || !strings.Contains(deniedEnv.Body.String(), "InvalidInputException") {
		t.Fatalf("env override status=%d body=%q", deniedEnv.Code, deniedEnv.Body.String())
	}

	// Without overrides, compute gate still applies.
	ok := mustCodeBuildJSON(t, handler, "StartBuild", map[string]any{
		"projectName": "lock-proj",
	}, now)
	if ok.Code != http.StatusServiceUnavailable {
		t.Fatalf("StartBuild status=%d want 503 body=%q", ok.Code, ok.Body.String())
	}
}

func TestCodeBuildStartBuildBatchWithoutCompute(t *testing.T) {
	srv, _ := newTestServer(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	mustCreateIAMRole(t, handler, "cb-batch", codebuildTrustOK, now)
	roleARN := "arn:aws:iam::" + testAccountID + ":role/cb-batch"

	create := mustCodeBuildJSON(t, handler, "CreateProject", map[string]any{
		"name":        "batch-api-proj",
		"serviceRole": roleARN,
		"source": map[string]any{
			"type": "NO_SOURCE",
			"buildspec": `{"version":"0.2","batch":{"build-list":[{"identifier":"a"},{"identifier":"b"}]},"phases":{"build":{"commands":["echo hi"]}}}`,
		},
		"environment": map[string]any{
			"type":  "LINUX_CONTAINER",
			"image": "alpine:3.20",
		},
		"artifacts": map[string]any{"type": "NO_ARTIFACTS"},
	}, now)
	if create.Code != http.StatusOK {
		t.Fatalf("CreateProject status=%d body=%q", create.Code, create.Body.String())
	}

	start := mustCodeBuildJSON(t, handler, "StartBuildBatch", map[string]any{
		"projectName": "batch-api-proj",
		"matrix": []any{
			[]any{map[string]any{"name": "FOO", "value": "1"}},
			[]any{map[string]any{"name": "FOO", "value": "2"}},
		},
	}, now)
	if start.Code != http.StatusServiceUnavailable {
		t.Fatalf("StartBuildBatch status=%d want 503 body=%q", start.Code, start.Body.String())
	}
}

func TestCodeBuildCodeCommitCreateAndStartWithoutCompute(t *testing.T) {
	srv, _ := newTestServer(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	mustCreateIAMRole(t, handler, "cb-cc", codebuildTrustOK, now)
	roleARN := "arn:aws:iam::" + testAccountID + ":role/cb-cc"

	repo := mustCodeCommitJSON(t, handler, "CreateRepository", map[string]any{
		"repositoryName": "cb-src",
	}, now)
	if repo.Code != http.StatusOK {
		t.Fatalf("CreateRepository status=%d body=%q", repo.Code, repo.Body.String())
	}
	put := mustCodeCommitJSON(t, handler, "PutFile", map[string]any{
		"repositoryName": "cb-src",
		"branchName":     "main",
		"filePath":       "README.md",
		"fileContent":    base64.StdEncoding.EncodeToString([]byte("from-cc")),
	}, now)
	if put.Code != http.StatusOK {
		t.Fatalf("PutFile status=%d body=%q", put.Code, put.Body.String())
	}

	create := mustCodeBuildJSON(t, handler, "CreateProject", map[string]any{
		"name":        "cc-api-proj",
		"serviceRole": roleARN,
		"source": map[string]any{
			"type":      "CODECOMMIT",
			"location":  "cb-src",
			"buildspec": `{"version":"0.2","phases":{"build":{"commands":["cat README.md"]}}}`,
		},
		"environment": map[string]any{
			"type":  "LINUX_CONTAINER",
			"image": "alpine:3.20",
		},
		"artifacts": map[string]any{"type": "NO_ARTIFACTS"},
		"vpcConfig": map[string]any{
			"vpcId":            "vpc-lab",
			"subnets":          []any{"subnet-a"},
			"securityGroupIds": []any{"sg-a"},
		},
		"cache": map[string]any{"type": "NO_CACHE"},
		"reportGroupArns": []any{
			"arn:aws:codebuild:us-east-1:" + testAccountID + ":report-group/lab",
		},
	}, now)
	if create.Code != http.StatusOK {
		t.Fatalf("CreateProject status=%d body=%q", create.Code, create.Body.String())
	}
	if !strings.Contains(create.Body.String(), "CODECOMMIT") || !strings.Contains(create.Body.String(), "vpc-lab") {
		t.Fatalf("CreateProject body=%q", create.Body.String())
	}

	batch := mustCodeBuildJSON(t, handler, "BatchGetProjects", map[string]any{
		"names": []any{"cc-api-proj"},
	}, now)
	if batch.Code != http.StatusOK {
		t.Fatalf("BatchGetProjects status=%d body=%q", batch.Code, batch.Body.String())
	}
	if !strings.Contains(batch.Body.String(), "NO_CACHE") || !strings.Contains(batch.Body.String(), "report-group/lab") {
		t.Fatalf("BatchGetProjects stubs body=%q", batch.Body.String())
	}

	start := mustCodeBuildJSON(t, handler, "StartBuild", map[string]any{
		"projectName": "cc-api-proj",
	}, now)
	if start.Code != http.StatusServiceUnavailable {
		t.Fatalf("StartBuild status=%d want 503 body=%q", start.Code, start.Body.String())
	}
	if !strings.Contains(start.Body.String(), "compute unavailable") {
		t.Fatalf("body=%q", start.Body.String())
	}

	missing := mustCodeBuildJSON(t, handler, "CreateProject", map[string]any{
		"name":        "cc-missing-proj",
		"serviceRole": roleARN,
		"source": map[string]any{
			"type":      "CODECOMMIT",
			"location":  "no-such-repo",
			"buildspec": `{"version":"0.2","phases":{"build":{"commands":["echo x"]}}}`,
		},
		"environment": map[string]any{
			"type":  "LINUX_CONTAINER",
			"image": "alpine:3.20",
		},
		"artifacts": map[string]any{"type": "NO_ARTIFACTS"},
	}, now)
	if missing.Code != http.StatusOK {
		t.Fatalf("CreateProject missing-repo status=%d body=%q", missing.Code, missing.Body.String())
	}
	denied := mustCodeBuildJSON(t, handler, "StartBuild", map[string]any{
		"projectName": "cc-missing-proj",
	}, now)
	if denied.Code != http.StatusBadRequest {
		t.Fatalf("StartBuild missing repo status=%d want 400 body=%q", denied.Code, denied.Body.String())
	}
	if !strings.Contains(denied.Body.String(), "not found") {
		t.Fatalf("missing repo body=%q", denied.Body.String())
	}
}

func TestCodeBuildCreateDeleteListWebhook(t *testing.T) {
	srv, _ := newTestServer(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	mustCreateIAMRole(t, handler, "cb-wh", codebuildTrustOK, now)
	roleARN := "arn:aws:iam::" + testAccountID + ":role/cb-wh"
	create := mustCodeBuildJSON(t, handler, "CreateProject", map[string]any{
		"name":        "wh-proj",
		"serviceRole": roleARN,
		"source": map[string]any{
			"type":      "NO_SOURCE",
			"buildspec": `{"version":"0.2","phases":{"build":{"commands":["echo hi"]}}}`,
		},
		"environment": map[string]any{"type": "LINUX_CONTAINER", "image": "alpine:3.20"},
		"artifacts":   map[string]any{"type": "NO_ARTIFACTS"},
	}, now)
	if create.Code != http.StatusOK {
		t.Fatalf("CreateProject status=%d body=%q", create.Code, create.Body.String())
	}

	wh := mustCodeBuildJSON(t, handler, "CreateWebhook", map[string]any{
		"projectName": "wh-proj",
		"filterGroups": []any{
			[]any{map[string]any{"type": "EVENT", "pattern": "PUSH"}},
		},
	}, now)
	if wh.Code != http.StatusOK {
		t.Fatalf("CreateWebhook status=%d body=%q", wh.Code, wh.Body.String())
	}
	if !strings.Contains(wh.Body.String(), "/_noctaxris/codebuild/webhook/") || !strings.Contains(wh.Body.String(), "secret") {
		t.Fatalf("CreateWebhook body=%q", wh.Body.String())
	}

	list := mustCodeBuildJSON(t, handler, "ListWebhooks", map[string]any{
		"projectName": "wh-proj",
	}, now)
	if list.Code != http.StatusOK || !strings.Contains(list.Body.String(), "wh-proj") && !strings.Contains(list.Body.String(), "payloadUrl") {
		t.Fatalf("ListWebhooks status=%d body=%q", list.Code, list.Body.String())
	}
	if !strings.Contains(list.Body.String(), "payloadUrl") {
		t.Fatalf("ListWebhooks missing payloadUrl body=%q", list.Body.String())
	}

	del := mustCodeBuildJSON(t, handler, "DeleteWebhook", map[string]any{"projectName": "wh-proj"}, now)
	if del.Code != http.StatusOK {
		t.Fatalf("DeleteWebhook status=%d body=%q", del.Code, del.Body.String())
	}
	listAfter := mustCodeBuildJSON(t, handler, "ListWebhooks", map[string]any{"projectName": "wh-proj"}, now)
	if listAfter.Code != http.StatusOK || strings.Contains(listAfter.Body.String(), `"secret"`) {
		// empty webhooks array is fine
		var parsed map[string]any
		_ = json.Unmarshal(listAfter.Body.Bytes(), &parsed)
		arr, _ := parsed["webhooks"].([]any)
		if len(arr) != 0 {
			t.Fatalf("ListWebhooks after delete=%q", listAfter.Body.String())
		}
	}
}

func TestCodeBuildConfigStubValidation(t *testing.T) {
	srv, _ := newTestServer(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)
	mustCreateIAMRole(t, handler, "cb-stub", codebuildTrustOK, now)
	roleARN := "arn:aws:iam::" + testAccountID + ":role/cb-stub"

	badCache := mustCodeBuildJSON(t, handler, "CreateProject", map[string]any{
		"name":        "bad-cache",
		"serviceRole": roleARN,
		"source":      map[string]any{"type": "NO_SOURCE", "buildspec": "echo"},
		"environment": map[string]any{"type": "LINUX_CONTAINER", "image": "alpine:3.20"},
		"artifacts":   map[string]any{"type": "NO_ARTIFACTS"},
		"cache":       map[string]any{"type": "REDIS"},
	}, now)
	if badCache.Code != http.StatusBadRequest {
		t.Fatalf("bad cache status=%d body=%q", badCache.Code, badCache.Body.String())
	}

	badVPC := mustCodeBuildJSON(t, handler, "CreateProject", map[string]any{
		"name":        "bad-vpc",
		"serviceRole": roleARN,
		"source":      map[string]any{"type": "NO_SOURCE", "buildspec": "echo"},
		"environment": map[string]any{"type": "LINUX_CONTAINER", "image": "alpine:3.20"},
		"artifacts":   map[string]any{"type": "NO_ARTIFACTS"},
		"vpcConfig":   map[string]any{"vpcId": "vpc-1"},
	}, now)
	if badVPC.Code != http.StatusBadRequest {
		t.Fatalf("bad vpc status=%d body=%q", badVPC.Code, badVPC.Body.String())
	}
}

func TestCodeBuildProjectNegativesAndDeleteMissing(t *testing.T) {
	srv, _ := newTestServer(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	mustCreateIAMRole(t, handler, "cb-neg-role", codebuildTrustOK, now)
	roleARN := "arn:aws:iam::" + testAccountID + ":role/cb-neg-role"

	emptyName := mustCodeBuildJSON(t, handler, "CreateProject", map[string]any{
		"name":        "",
		"serviceRole": roleARN,
		"source":      map[string]any{"type": "NO_SOURCE", "buildspec": "version: 0.2"},
		"environment": map[string]any{"type": "LINUX_CONTAINER", "image": "alpine:3.20"},
		"artifacts":   map[string]any{"type": "NO_ARTIFACTS"},
	}, now)
	if emptyName.Code == http.StatusOK {
		t.Fatalf("empty name should fail: %s", emptyName.Body.String())
	}

	missingRole := mustCodeBuildJSON(t, handler, "CreateProject", map[string]any{
		"name":        "no-role-proj",
		"serviceRole": "arn:aws:iam::" + testAccountID + ":role/missing-cb-role",
		"source":      map[string]any{"type": "NO_SOURCE", "buildspec": "version: 0.2"},
		"environment": map[string]any{"type": "LINUX_CONTAINER", "image": "alpine:3.20"},
		"artifacts":   map[string]any{"type": "NO_ARTIFACTS"},
	}, now)
	if missingRole.Code == http.StatusOK {
		t.Fatalf("missing role should fail: %s", missingRole.Body.String())
	}

	create := mustCodeBuildJSON(t, handler, "CreateProject", map[string]any{
		"name":        "neg-proj",
		"serviceRole": roleARN,
		"source":      map[string]any{"type": "NO_SOURCE", "buildspec": `{"version":"0.2","phases":{"build":{"commands":["echo hi"]}}}`},
		"environment": map[string]any{"type": "LINUX_CONTAINER", "image": "alpine:3.20"},
		"artifacts":   map[string]any{"type": "NO_ARTIFACTS"},
	}, now)
	if create.Code != http.StatusOK {
		t.Fatalf("CreateProject status=%d body=%q", create.Code, create.Body.String())
	}
	dup := mustCodeBuildJSON(t, handler, "CreateProject", map[string]any{
		"name":        "neg-proj",
		"serviceRole": roleARN,
		"source":      map[string]any{"type": "NO_SOURCE", "buildspec": "version: 0.2"},
		"environment": map[string]any{"type": "LINUX_CONTAINER", "image": "alpine:3.20"},
		"artifacts":   map[string]any{"type": "NO_ARTIFACTS"},
	}, now)
	if dup.Code == http.StatusOK {
		t.Fatalf("duplicate project should fail: %s", dup.Body.String())
	}

	list := mustCodeBuildJSON(t, handler, "ListProjects", map[string]any{}, now)
	if list.Code != http.StatusOK || !strings.Contains(list.Body.String(), "neg-proj") {
		t.Fatalf("ListProjects status=%d body=%q", list.Code, list.Body.String())
	}
	batch := mustCodeBuildJSON(t, handler, "BatchGetProjects", map[string]any{"names": []string{"neg-proj"}}, now)
	if batch.Code != http.StatusOK || !strings.Contains(batch.Body.String(), "neg-proj") {
		t.Fatalf("BatchGetProjects status=%d body=%q", batch.Code, batch.Body.String())
	}
	emptyBatch := mustCodeBuildJSON(t, handler, "BatchGetProjects", map[string]any{"names": []string{}}, now)
	if emptyBatch.Code != http.StatusOK {
		t.Fatalf("BatchGetProjects empty status=%d body=%q", emptyBatch.Code, emptyBatch.Body.String())
	}

	updMissing := mustCodeBuildJSON(t, handler, "UpdateProject", map[string]any{
		"name":        "no-such-proj",
		"serviceRole": roleARN,
		"source":      map[string]any{"type": "NO_SOURCE", "buildspec": "version: 0.2"},
		"environment": map[string]any{"type": "LINUX_CONTAINER", "image": "alpine:3.20"},
		"artifacts":   map[string]any{"type": "NO_ARTIFACTS"},
	}, now)
	if updMissing.Code == http.StatusOK {
		t.Fatalf("UpdateProject missing should fail")
	}

	del := mustCodeBuildJSON(t, handler, "DeleteProject", map[string]any{"name": "neg-proj"}, now)
	if del.Code != http.StatusOK {
		t.Fatalf("DeleteProject status=%d body=%q", del.Code, del.Body.String())
	}
	delMiss := mustCodeBuildJSON(t, handler, "DeleteProject", map[string]any{"name": "neg-proj"}, now)
	if delMiss.Code == http.StatusOK {
		t.Fatalf("DeleteProject missing should fail: %s", delMiss.Body.String())
	}
}
