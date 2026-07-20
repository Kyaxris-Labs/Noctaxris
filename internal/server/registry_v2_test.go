package server_test

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func issueRegistryToken(t *testing.T, handler http.Handler, now time.Time) string {
	t.Helper()
	rec := mustECRJSON(t, handler, "GetAuthorizationToken", map[string]any{}, now)
	if rec.Code != http.StatusOK {
		t.Fatalf("GetAuthorizationToken status=%d body=%q", rec.Code, rec.Body.String())
	}
	var out map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	data, _ := out["authorizationData"].([]any)
	entry, _ := data[0].(map[string]any)
	tokenB64, _ := entry["authorizationToken"].(string)
	raw, err := base64.StdEncoding.DecodeString(tokenB64)
	if err != nil {
		t.Fatal(err)
	}
	parts := strings.SplitN(string(raw), ":", 2)
	if len(parts) != 2 {
		t.Fatalf("token=%q", string(raw))
	}
	return parts[1]
}

func registryAuthHeader(token string) string {
	return "Basic " + base64.StdEncoding.EncodeToString([]byte("AWS:"+token))
}

func TestRegistryV2Unauthorized(t *testing.T) {
	srv, _ := newTestServer(t)
	handler := srv.Handler()

	req := httptest.NewRequest(http.MethodGet, "http://127.0.0.1:4566/v2/", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status=%d want 401 body=%q", rec.Code, rec.Body.String())
	}
	www := rec.Header().Get("WWW-Authenticate")
	if !strings.Contains(www, `Bearer realm="http://127.0.0.1:4566/v2/token"`) || !strings.Contains(www, `service="ecr"`) {
		t.Fatalf("WWW-Authenticate=%q", www)
	}
}

func TestRegistryV2TokenEndpoint(t *testing.T) {
	srv, _ := newTestServer(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	token := issueRegistryToken(t, handler, now)
	auth := registryAuthHeader(token)

	unauthReq := httptest.NewRequest(http.MethodGet, "http://127.0.0.1:4566/v2/token?service=ecr", nil)
	unauthRec := httptest.NewRecorder()
	handler.ServeHTTP(unauthRec, unauthReq)
	if unauthRec.Code != http.StatusUnauthorized {
		t.Fatalf("unauth token status=%d want 401 body=%q", unauthRec.Code, unauthRec.Body.String())
	}
	if !strings.Contains(unauthRec.Header().Get("WWW-Authenticate"), "Basic realm=") {
		t.Fatalf("token WWW-Authenticate=%q want Basic", unauthRec.Header().Get("WWW-Authenticate"))
	}

	req := httptest.NewRequest(http.MethodGet, "http://127.0.0.1:4566/v2/token?service=ecr&account=AWS", nil)
	req.Header.Set("Authorization", auth)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("token status=%d body=%q", rec.Code, rec.Body.String())
	}
	var out map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	gotToken, _ := out["token"].(string)
	gotAccess, _ := out["access_token"].(string)
	if gotToken != token || gotAccess != token {
		t.Fatalf("token=%q access_token=%q want %q", gotToken, gotAccess, token)
	}
	expiresIn, ok := out["expires_in"].(float64)
	if !ok || expiresIn <= 0 {
		t.Fatalf("expires_in=%v want > 0", out["expires_in"])
	}
	if issuedAt, _ := out["issued_at"].(string); issuedAt == "" {
		t.Fatal("missing issued_at")
	}

	bearerReq := httptest.NewRequest(http.MethodGet, "http://127.0.0.1:4566/v2/", nil)
	bearerReq.Header.Set("Authorization", "Bearer "+gotToken)
	bearerRec := httptest.NewRecorder()
	handler.ServeHTTP(bearerRec, bearerReq)
	if bearerRec.Code != http.StatusOK {
		t.Fatalf("bearer /v2/ status=%d body=%q", bearerRec.Code, bearerRec.Body.String())
	}
}

func TestRegistryV2ManifestRoundTrip(t *testing.T) {
	srv, st, _ := newTestServerStore(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	createRec := mustECRJSON(t, handler, "CreateRepository", map[string]any{
		"repositoryName": "registry-lab",
	}, now)
	if createRec.Code != http.StatusOK {
		t.Fatalf("CreateRepository status=%d body=%q", createRec.Code, createRec.Body.String())
	}

	token := issueRegistryToken(t, handler, now)
	auth := registryAuthHeader(token)
	account := testAccountID
	repo := "registry-lab"

	layer := []byte("noctaxris-registry-layer-payload")
	layerDigest := "sha256:" + sha256Hex(layer)

	uploadReq := httptest.NewRequest(http.MethodPost, fmt.Sprintf("http://127.0.0.1:4566/v2/%s/%s/blobs/uploads/", account, repo), nil)
	uploadReq.Header.Set("Authorization", auth)
	uploadRec := httptest.NewRecorder()
	handler.ServeHTTP(uploadRec, uploadReq)
	if uploadRec.Code != http.StatusAccepted {
		t.Fatalf("blob upload start status=%d body=%q", uploadRec.Code, uploadRec.Body.String())
	}
	location := uploadRec.Header().Get("Location")
	if location == "" {
		t.Fatal("missing upload Location")
	}

	putBlobReq := httptest.NewRequest(http.MethodPut, "http://127.0.0.1:4566"+location+"?digest="+layerDigest, bytes.NewReader(layer))
	putBlobReq.Header.Set("Authorization", auth)
	putBlobRec := httptest.NewRecorder()
	handler.ServeHTTP(putBlobRec, putBlobReq)
	if putBlobRec.Code != http.StatusCreated {
		t.Fatalf("blob put status=%d body=%q", putBlobRec.Code, putBlobRec.Body.String())
	}

	manifest := fmt.Sprintf(`{
  "schemaVersion": 2,
  "mediaType": "application/vnd.docker.distribution.manifest.v2+json",
  "config": {
    "mediaType": "application/vnd.docker.container.image.v1+json",
    "size": 1,
    "digest": "sha256:0000000000000000000000000000000000000000000000000000000000000001"
  },
  "layers": [
    {
      "mediaType": "application/vnd.docker.image.rootfs.diff.tar.gzip",
      "size": %d,
      "digest": %q
    }
  ]
}`, len(layer), layerDigest)
	tag := "lab"

	putManifestReq := httptest.NewRequest(http.MethodPut, fmt.Sprintf("http://127.0.0.1:4566/v2/%s/%s/manifests/%s", account, repo, tag), strings.NewReader(manifest))
	putManifestReq.Header.Set("Authorization", auth)
	putManifestReq.Header.Set("Content-Type", "application/vnd.docker.distribution.manifest.v2+json")
	putManifestRec := httptest.NewRecorder()
	handler.ServeHTTP(putManifestRec, putManifestReq)
	if putManifestRec.Code != http.StatusCreated {
		t.Fatalf("manifest put status=%d body=%q", putManifestRec.Code, putManifestRec.Body.String())
	}
	manifestDigest := putManifestRec.Header().Get("Docker-Content-Digest")
	if manifestDigest == "" {
		t.Fatal("missing Docker-Content-Digest on manifest put")
	}

	getManifestReq := httptest.NewRequest(http.MethodGet, fmt.Sprintf("http://127.0.0.1:4566/v2/%s/%s/manifests/%s", account, repo, tag), nil)
	getManifestReq.Header.Set("Authorization", auth)
	getManifestRec := httptest.NewRecorder()
	handler.ServeHTTP(getManifestRec, getManifestReq)
	if getManifestRec.Code != http.StatusOK {
		t.Fatalf("manifest get status=%d body=%q", getManifestRec.Code, getManifestRec.Body.String())
	}
	gotManifest, err := io.ReadAll(getManifestRec.Body)
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(string(gotManifest)) != strings.TrimSpace(manifest) {
		t.Fatalf("manifest round-trip mismatch:\n got=%q\nwant=%q", gotManifest, manifest)
	}
	if getManifestRec.Header().Get("Docker-Content-Digest") != manifestDigest {
		t.Fatalf("digest header=%q want %q", getManifestRec.Header().Get("Docker-Content-Digest"), manifestDigest)
	}

	tagsReq := httptest.NewRequest(http.MethodGet, fmt.Sprintf("http://127.0.0.1:4566/v2/%s/%s/tags/list", account, repo), nil)
	tagsReq.Header.Set("Authorization", auth)
	tagsRec := httptest.NewRecorder()
	handler.ServeHTTP(tagsRec, tagsReq)
	if tagsRec.Code != http.StatusOK {
		t.Fatalf("tags list status=%d body=%q", tagsRec.Code, tagsRec.Body.String())
	}
	var tagsOut map[string]any
	if err := json.Unmarshal(tagsRec.Body.Bytes(), &tagsOut); err != nil {
		t.Fatal(err)
	}
	tags, _ := tagsOut["tags"].([]any)
	if len(tags) != 1 || tags[0] != tag {
		t.Fatalf("tags=%v want [%q]", tags, tag)
	}

	images, err := st.ListImages(account, repo)
	if err != nil {
		t.Fatal(err)
	}
	if len(images) != 1 {
		t.Fatalf("images len=%d want 1", len(images))
	}
	if images[0].ImageDigest != manifestDigest {
		t.Fatalf("stored digest=%q want %q", images[0].ImageDigest, manifestDigest)
	}
	if len(images[0].ImageTags) != 1 || images[0].ImageTags[0] != tag {
		t.Fatalf("stored tags=%v want [%q]", images[0].ImageTags, tag)
	}
}

func TestRegistryV2RejectsMaliciousDigest(t *testing.T) {
	srv, _ := newTestServer(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	createRec := mustECRJSON(t, handler, "CreateRepository", map[string]any{
		"repositoryName": "registry-lab",
	}, now)
	if createRec.Code != http.StatusOK {
		t.Fatalf("CreateRepository status=%d body=%q", createRec.Code, createRec.Body.String())
	}

	token := issueRegistryToken(t, handler, now)
	auth := registryAuthHeader(token)
	account := testAccountID
	repo := "registry-lab"
	maliciousDigest := "sha256:.."

	getReq := httptest.NewRequest(http.MethodGet, fmt.Sprintf("http://127.0.0.1:4566/v2/%s/%s/blobs/%s", account, repo, maliciousDigest), nil)
	getReq.Header.Set("Authorization", auth)
	getRec := httptest.NewRecorder()
	handler.ServeHTTP(getRec, getReq)
	if getRec.Code != http.StatusBadRequest {
		t.Fatalf("malicious digest GET status=%d want 400 body=%q", getRec.Code, getRec.Body.String())
	}

	uploadReq := httptest.NewRequest(http.MethodPost, fmt.Sprintf("http://127.0.0.1:4566/v2/%s/%s/blobs/uploads/", account, repo), nil)
	uploadReq.Header.Set("Authorization", auth)
	uploadRec := httptest.NewRecorder()
	handler.ServeHTTP(uploadRec, uploadReq)
	if uploadRec.Code != http.StatusAccepted {
		t.Fatalf("blob upload start status=%d body=%q", uploadRec.Code, uploadRec.Body.String())
	}
	location := uploadRec.Header().Get("Location")
	putBlobReq := httptest.NewRequest(http.MethodPut, "http://127.0.0.1:4566"+location+"?digest="+maliciousDigest, strings.NewReader("payload"))
	putBlobReq.Header.Set("Authorization", auth)
	putBlobRec := httptest.NewRecorder()
	handler.ServeHTTP(putBlobRec, putBlobReq)
	if putBlobRec.Code != http.StatusBadRequest {
		t.Fatalf("malicious digest PUT status=%d want 400 body=%q", putBlobRec.Code, putBlobRec.Body.String())
	}
}

func sha256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
