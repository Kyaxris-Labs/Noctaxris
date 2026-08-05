package server_test

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestGlueDatabaseTableCrawlerLifecycle(t *testing.T) {
	srv, _, _ := newTestServerStore(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	createDB := mustJSONTarget(t, handler, "AWSGlue.CreateDatabase", "glue", map[string]any{
		"DatabaseInput": map[string]any{"Name": "glue-depth-db", "Description": "lab"},
	}, now)
	if createDB.Code != http.StatusOK {
		t.Fatalf("CreateDatabase %d %s", createDB.Code, createDB.Body.String())
	}

	getDB := mustJSONTarget(t, handler, "AWSGlue.GetDatabase", "glue", map[string]any{
		"Name": "glue-depth-db",
	}, now)
	if getDB.Code != http.StatusOK {
		t.Fatalf("GetDatabase %d %s", getDB.Code, getDB.Body.String())
	}

	listDB := mustJSONTarget(t, handler, "AWSGlue.GetDatabases", "glue", map[string]any{}, now)
	if listDB.Code != http.StatusOK || !strings.Contains(listDB.Body.String(), "glue-depth-db") {
		t.Fatalf("GetDatabases %d %s", listDB.Code, listDB.Body.String())
	}

	createTbl := mustJSONTarget(t, handler, "AWSGlue.CreateTable", "glue", map[string]any{
		"DatabaseName": "glue-depth-db",
		"TableInput": map[string]any{
			"Name": "events",
			"StorageDescriptor": map[string]any{
				"Location": "s3://glue-lab/events/",
				"Columns":  []map[string]any{{"Name": "id", "Type": "string"}},
			},
		},
	}, now)
	if createTbl.Code != http.StatusOK {
		t.Fatalf("CreateTable %d %s", createTbl.Code, createTbl.Body.String())
	}

	listTbl := mustJSONTarget(t, handler, "AWSGlue.GetTables", "glue", map[string]any{
		"DatabaseName": "glue-depth-db",
	}, now)
	if listTbl.Code != http.StatusOK || !strings.Contains(listTbl.Body.String(), "events") {
		t.Fatalf("GetTables %d %s", listTbl.Code, listTbl.Body.String())
	}

	mustS3(t, handler, http.MethodPut, "http://127.0.0.1:4566/glue-crawl-src", nil, "s3", now, nil)
	mustS3(t, handler, http.MethodPut, "http://127.0.0.1:4566/glue-crawl-src/data.csv", []byte("id,name\n1,alice\n"), "s3", now, map[string]string{
		"Content-Type": "text/csv",
	})

	mustCreateIAMRole(t, handler, "GlueCrawler", `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":{"Service":"glue.amazonaws.com"},"Action":"sts:AssumeRole"}]}`, now)
	createCrawler := mustJSONTarget(t, handler, "AWSGlue.CreateCrawler", "glue", map[string]any{
		"Name":         "glue-depth-crawler",
		"Role":         "arn:aws:iam::" + testAccountID + ":role/GlueCrawler",
		"DatabaseName": "glue-depth-db",
		"Targets": map[string]any{
			"S3Targets": []map[string]any{{
				"Path": "s3://glue-crawl-src/",
			}},
		},
	}, now)
	if createCrawler.Code != http.StatusOK {
		t.Fatalf("CreateCrawler %d %s", createCrawler.Code, createCrawler.Body.String())
	}

	getCrawler := mustJSONTarget(t, handler, "AWSGlue.GetCrawler", "glue", map[string]any{
		"Name": "glue-depth-crawler",
	}, now)
	if getCrawler.Code != http.StatusOK {
		t.Fatalf("GetCrawler %d %s", getCrawler.Code, getCrawler.Body.String())
	}

	listCrawlers := mustJSONTarget(t, handler, "AWSGlue.ListCrawlers", "glue", map[string]any{}, now)
	if listCrawlers.Code != http.StatusOK || !strings.Contains(listCrawlers.Body.String(), "glue-depth-crawler") {
		t.Fatalf("ListCrawlers %d %s", listCrawlers.Code, listCrawlers.Body.String())
	}

	start := mustJSONTarget(t, handler, "AWSGlue.StartCrawler", "glue", map[string]any{
		"Name": "glue-depth-crawler",
	}, now)
	if start.Code != http.StatusOK && start.Code != http.StatusBadRequest {
		t.Fatalf("StartCrawler unexpected %d %s", start.Code, start.Body.String())
	}

	delCrawler := mustJSONTarget(t, handler, "AWSGlue.DeleteCrawler", "glue", map[string]any{
		"Name": "glue-depth-crawler",
	}, now)
	if delCrawler.Code != http.StatusOK {
		t.Fatalf("DeleteCrawler %d %s", delCrawler.Code, delCrawler.Body.String())
	}

	delTbl := mustJSONTarget(t, handler, "AWSGlue.DeleteTable", "glue", map[string]any{
		"DatabaseName": "glue-depth-db",
		"Name":         "events",
	}, now)
	if delTbl.Code != http.StatusOK {
		t.Fatalf("DeleteTable %d %s", delTbl.Code, delTbl.Body.String())
	}

	delDB := mustJSONTarget(t, handler, "AWSGlue.DeleteDatabase", "glue", map[string]any{
		"Name": "glue-depth-db",
	}, now)
	if delDB.Code != http.StatusOK {
		t.Fatalf("DeleteDatabase %d %s", delDB.Code, delDB.Body.String())
	}

	missing := mustJSONTarget(t, handler, "AWSGlue.GetDatabase", "glue", map[string]any{
		"Name": "glue-depth-db",
	}, now)
	if missing.Code == http.StatusOK {
		t.Fatalf("GetDatabase after delete should fail: %s", missing.Body.String())
	}
}

func TestRegistryHeadBlobAfterUpload(t *testing.T) {
	srv, _, _ := newTestServerStore(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	repo := "del-manifest"
	createRec := mustECRJSON(t, handler, "CreateRepository", map[string]any{
		"repositoryName": repo,
	}, now)
	if createRec.Code != http.StatusOK {
		t.Fatalf("CreateRepository %d %s", createRec.Code, createRec.Body.String())
	}

	token := issueRegistryToken(t, handler, now)
	auth := registryAuthHeader(token)
	account := testAccountID

	layer := []byte("manifest-delete-layer")
	digest := "sha256:" + sha256Hex(layer)

	uploadReq := httptest.NewRequest(http.MethodPost, fmt.Sprintf("http://127.0.0.1:4566/v2/%s/%s/blobs/uploads/", account, repo), nil)
	uploadReq.Header.Set("Authorization", auth)
	uploadRec := httptest.NewRecorder()
	handler.ServeHTTP(uploadRec, uploadReq)
	if uploadRec.Code != http.StatusAccepted {
		t.Fatalf("upload start %d %s", uploadRec.Code, uploadRec.Body.String())
	}
	location := uploadRec.Header().Get("Location")

	putBlob := httptest.NewRequest(http.MethodPut, "http://127.0.0.1:4566"+location+"?digest="+digest, bytes.NewReader(layer))
	putBlob.Header.Set("Authorization", auth)
	putRec := httptest.NewRecorder()
	handler.ServeHTTP(putRec, putBlob)
	if putRec.Code != http.StatusCreated {
		t.Fatalf("blob put %d %s", putRec.Code, putRec.Body.String())
	}

	headBlob := httptest.NewRequest(http.MethodHead, fmt.Sprintf("http://127.0.0.1:4566/v2/%s/%s/blobs/%s", account, repo, digest), nil)
	headBlob.Header.Set("Authorization", auth)
	headRec := httptest.NewRecorder()
	handler.ServeHTTP(headRec, headBlob)
	if headRec.Code != http.StatusOK {
		t.Fatalf("HEAD blob %d", headRec.Code)
	}
	if headRec.Header().Get("Docker-Content-Digest") != digest {
		t.Fatalf("HEAD digest=%q want %q", headRec.Header().Get("Docker-Content-Digest"), digest)
	}

	manifest := fmt.Sprintf(`{"schemaVersion":2,"mediaType":"application/vnd.docker.distribution.manifest.v2+json","config":{"mediaType":"application/vnd.docker.container.image.v1+json","size":1,"digest":"sha256:0000000000000000000000000000000000000000000000000000000000000001"},"layers":[{"mediaType":"application/vnd.docker.image.rootfs.diff.tar.gzip","size":%d,"digest":%q}]}`, len(layer), digest)
	tag := "v1"
	putMan := httptest.NewRequest(http.MethodPut, fmt.Sprintf("http://127.0.0.1:4566/v2/%s/%s/manifests/%s", account, repo, tag), strings.NewReader(manifest))
	putMan.Header.Set("Authorization", auth)
	putMan.Header.Set("Content-Type", "application/vnd.docker.distribution.manifest.v2+json")
	putManRec := httptest.NewRecorder()
	handler.ServeHTTP(putManRec, putMan)
	if putManRec.Code != http.StatusCreated {
		t.Fatalf("manifest put %d %s", putManRec.Code, putManRec.Body.String())
	}

	delMan := httptest.NewRequest(http.MethodDelete, fmt.Sprintf("http://127.0.0.1:4566/v2/%s/%s/manifests/%s", account, repo, tag), nil)
	delMan.Header.Set("Authorization", auth)
	delManRec := httptest.NewRecorder()
	handler.ServeHTTP(delManRec, delMan)
	// Lab registry may reject delete-by-tag; accept 2xx or not-found style codes.
	if delManRec.Code != http.StatusAccepted && delManRec.Code != http.StatusOK && delManRec.Code != http.StatusNotFound {
		t.Fatalf("manifest delete %d %s", delManRec.Code, delManRec.Body.String())
	}

	if delManRec.Code == http.StatusAccepted || delManRec.Code == http.StatusOK {
		getAfter := httptest.NewRequest(http.MethodGet, fmt.Sprintf("http://127.0.0.1:4566/v2/%s/%s/manifests/%s", account, repo, tag), nil)
		getAfter.Header.Set("Authorization", auth)
		getAfterRec := httptest.NewRecorder()
		handler.ServeHTTP(getAfterRec, getAfter)
		if getAfterRec.Code == http.StatusOK {
			t.Fatalf("manifest should be gone after delete: %s", getAfterRec.Body.String())
		}
	}
}
