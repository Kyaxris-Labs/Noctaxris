package server_test

import (
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
