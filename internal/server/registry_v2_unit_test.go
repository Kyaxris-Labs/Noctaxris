package server

import (
	"net/http"
	"strings"
	"testing"
)

func TestIsRegistryV2Path(t *testing.T) {
	t.Parallel()
	if !isRegistryV2Path("/v2/") || !isRegistryV2Path("/v2/token") {
		t.Fatal("registry paths")
	}
	if isRegistryV2Path("/v2/apis/foo") {
		t.Fatal("apigw steal")
	}
	if isRegistryV2Path("/v2/email/send") {
		t.Fatal("ses steal")
	}
}

func TestParseRegistryRoute(t *testing.T) {
	t.Parallel()
	r, err := parseRegistryRoute("/v2/000000000001/myrepo/blobs/sha256:abc")
	if err != nil || r.accountID != "000000000001" || r.repoName != "myrepo" {
		t.Fatalf("%+v err=%v", r, err)
	}
	if !strings.HasPrefix(r.remainder, "blobs/") {
		t.Fatalf("remainder=%q", r.remainder)
	}
	_, err = parseRegistryRoute("/v2/")
	if err == nil {
		t.Fatal("short path")
	}
}

func TestRegistryV2Action(t *testing.T) {
	t.Parallel()
	act, ok := registryV2Action(http.MethodGet, "tags/list")
	if !ok || act == "" {
		t.Fatalf("tags list act=%q ok=%v", act, ok)
	}
	_, ok = registryV2Action(http.MethodPatch, "blobs/uploads/uuid")
	if !ok {
		t.Fatal("patch upload")
	}
}

func TestParseRegistryContentRange(t *testing.T) {
	t.Parallel()
	start, end, ok := parseRegistryContentRange("bytes 0-9/*")
	if !ok || start != 0 || end != 9 {
		t.Fatalf("%d-%d ok=%v", start, end, ok)
	}
	_, _, ok = parseRegistryContentRange("bad")
	if ok {
		t.Fatal("bad range")
	}
}

	const validDigest = "sha256:0000000000000000000000000000000000000000000000000000000000000001"

func TestParseRegistryDigest(t *testing.T) {
	t.Parallel()
	algo, hex, err := parseRegistryDigest(validDigest)
	if err != nil || algo != "sha256" || hex != "0000000000000000000000000000000000000000000000000000000000000001" {
		t.Fatalf("%s %s %v", algo, hex, err)
	}
	_, _, err = parseRegistryDigest("..")
	if err == nil {
		t.Fatal("malicious")
	}
}

func TestIsRegistryManifestDigest(t *testing.T) {
	t.Parallel()
	if !isRegistryManifestDigest(validDigest) {
		t.Fatal("digest ref")
	}
	if isRegistryManifestDigest("latest") {
		t.Fatal("tag ref")
	}
}

func TestRegistryUploadPath(t *testing.T) {
	t.Parallel()
	p, err := registryUploadPath(t.TempDir(), "upload-id")
	if err != nil || !strings.Contains(p, "upload-id") {
		t.Fatalf("%q err=%v", p, err)
	}
	_, err = registryUploadPath(t.TempDir(), "../evil")
	if err == nil {
		t.Fatal("traversal")
	}
}
