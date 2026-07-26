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

func mustCodeCommitJSON(t *testing.T, handler http.Handler, target string, payload map[string]any, now time.Time) *httptest.ResponseRecorder {
	t.Helper()
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	req := mustNewRequest(t, http.MethodPost, "http://127.0.0.1:4566/", raw)
	req.Header.Set("Content-Type", "application/x-amz-json-1.1")
	req.Header.Set("X-Amz-Target", "CodeCommit_20150413."+target)
	signHeader(t, req, raw, testAccessKey, testSecret, testRegion, "codecommit", now)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}

func TestCodeCommitRepositoryAndFiles(t *testing.T) {
	srv, _ := newTestServer(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	create := mustCodeCommitJSON(t, handler, "CreateRepository", map[string]any{
		"repositoryName":        "lab-cc",
		"repositoryDescription": "wave2",
	}, now)
	if create.Code != http.StatusOK {
		t.Fatalf("CreateRepository status=%d body=%q", create.Code, create.Body.String())
	}
	if !strings.Contains(create.Body.String(), `"repositoryName":"lab-cc"`) {
		t.Fatalf("CreateRepository body=%q", create.Body.String())
	}

	dup := mustCodeCommitJSON(t, handler, "CreateRepository", map[string]any{
		"repositoryName": "lab-cc",
	}, now)
	if dup.Code != http.StatusBadRequest || !strings.Contains(dup.Body.String(), "RepositoryNameExistsException") {
		t.Fatalf("dup status=%d body=%q", dup.Code, dup.Body.String())
	}

	list := mustCodeCommitJSON(t, handler, "ListRepositories", map[string]any{}, now)
	if list.Code != http.StatusOK || !strings.Contains(list.Body.String(), "lab-cc") {
		t.Fatalf("ListRepositories status=%d body=%q", list.Code, list.Body.String())
	}

	get := mustCodeCommitJSON(t, handler, "GetRepository", map[string]any{
		"repositoryName": "lab-cc",
	}, now)
	if get.Code != http.StatusOK || !strings.Contains(get.Body.String(), "wave2") {
		t.Fatalf("GetRepository status=%d body=%q", get.Code, get.Body.String())
	}

	content := base64.StdEncoding.EncodeToString([]byte("hello lab"))
	put := mustCodeCommitJSON(t, handler, "PutFile", map[string]any{
		"repositoryName": "lab-cc",
		"branchName":     "main",
		"filePath":       "README.md",
		"fileContent":    content,
		"commitMessage":  "add readme",
	}, now)
	if put.Code != http.StatusOK {
		t.Fatalf("PutFile status=%d body=%q", put.Code, put.Body.String())
	}
	var putBody map[string]any
	if err := json.Unmarshal(put.Body.Bytes(), &putBody); err != nil {
		t.Fatal(err)
	}
	if putBody["commitId"] == nil || putBody["blobId"] == nil {
		t.Fatalf("PutFile body=%v", putBody)
	}

	getFile := mustCodeCommitJSON(t, handler, "GetFile", map[string]any{
		"repositoryName": "lab-cc",
		"filePath":       "README.md",
	}, now)
	if getFile.Code != http.StatusOK {
		t.Fatalf("GetFile status=%d body=%q", getFile.Code, getFile.Body.String())
	}
	var fileBody map[string]any
	if err := json.Unmarshal(getFile.Body.Bytes(), &fileBody); err != nil {
		t.Fatal(err)
	}
	decoded, err := base64.StdEncoding.DecodeString(stringParamAny(fileBody["fileContent"]))
	if err != nil {
		t.Fatal(err)
	}
	if string(decoded) != "hello lab" {
		t.Fatalf("GetFile content=%q", decoded)
	}

	folder := mustCodeCommitJSON(t, handler, "GetFolder", map[string]any{
		"repositoryName": "lab-cc",
		"folderPath":     "",
	}, now)
	if folder.Code != http.StatusOK || !strings.Contains(folder.Body.String(), "README.md") {
		t.Fatalf("GetFolder status=%d body=%q", folder.Code, folder.Body.String())
	}

	del := mustCodeCommitJSON(t, handler, "DeleteRepository", map[string]any{
		"repositoryName": "lab-cc",
	}, now)
	if del.Code != http.StatusOK {
		t.Fatalf("DeleteRepository status=%d body=%q", del.Code, del.Body.String())
	}
	missing := mustCodeCommitJSON(t, handler, "GetRepository", map[string]any{
		"repositoryName": "lab-cc",
	}, now)
	if missing.Code != http.StatusBadRequest || !strings.Contains(missing.Body.String(), "RepositoryDoesNotExistException") {
		t.Fatalf("GetRepository after delete status=%d body=%q", missing.Code, missing.Body.String())
	}
}

func stringParamAny(v any) string {
	s, _ := v.(string)
	return s
}
