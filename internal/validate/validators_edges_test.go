package validate_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Kyaxris-Labs/Noctaxris/internal/validate"
)

func TestAccountNameAndOIDCClientID(t *testing.T) {
	if err := validate.AccountName("Lab Member"); err != nil {
		t.Fatal(err)
	}
	if err := validate.AccountName(""); err == nil || !validate.IsInvalid(err) {
		t.Fatalf("empty AccountName: %v", err)
	}
	if err := validate.AccountName(string(make([]byte, 51))); err == nil {
		t.Fatal("want reject oversized AccountName")
	}

	if err := validate.OIDCClientID("sts.amazonaws.com"); err != nil {
		t.Fatal(err)
	}
	if err := validate.OIDCClientID(""); err == nil || !validate.IsInvalid(err) {
		t.Fatalf("empty OIDCClientID: %v", err)
	}
}

func TestAccessKeyIDShapes(t *testing.T) {
	if err := validate.AccessKeyID("AKIAROOTEXAMPLE01"); err != nil {
		t.Fatal(err)
	}
	if err := validate.AccessKeyID("ASIAEXAMPLETEMPKEY1"); err != nil {
		t.Fatal(err)
	}
	if err := validate.AccessKeyID("short"); err == nil || !validate.IsInvalid(err) {
		t.Fatalf("short key: %v", err)
	}
	if err := validate.AccessKeyID("AKIAROOT-NOT-ALNUM!"); err == nil {
		t.Fatal("want reject non-alphanum AccessKeyID")
	}
}

func TestOIDCIssuerURLEdges(t *testing.T) {
	if err := validate.OIDCIssuerURL("http://idp.example.local/oidc"); err != nil {
		t.Fatal(err)
	}
	if err := validate.OIDCIssuerURL(""); err == nil {
		t.Fatal("want reject empty issuer")
	}
	if err := validate.OIDCIssuerURL("https://"); err == nil {
		t.Fatal("want reject hostless https URL")
	}
	if err := validate.OIDCIssuerURL("not-a-url"); err == nil {
		t.Fatal("want reject non-URL")
	}
}

func TestRoleARNAndPolicyDocumentEdges(t *testing.T) {
	if err := validate.RoleARN(""); err == nil {
		t.Fatal("want reject empty RoleArn")
	}
	if err := validate.RoleARN("arn:aws:s3:::bucket"); err == nil {
		t.Fatal("want reject non-IAM RoleArn")
	}

	if err := validate.PolicyDocument(""); err == nil {
		t.Fatal("want reject empty policy")
	}
	if err := validate.PolicyDocument(`{"Version":"2012-10-17","Statement":null}`); err == nil {
		t.Fatal("want reject null Statement")
	}
}

func TestPathHelpersEdges(t *testing.T) {
	if validate.PathUnderRoot("", "/tmp/x") {
		t.Fatal("empty root must be false")
	}
	if validate.PathUnderRoot("/data", "") {
		t.Fatal("empty candidate must be false")
	}
	root := t.TempDir()
	if !validate.PathUnderRoot(root, root) {
		t.Fatal("root equals candidate")
	}
	if _, err := validate.JoinDataPath("", "seg"); err == nil {
		t.Fatal("want reject empty JoinDataPath root")
	}
	if _, err := validate.ResolveUnderRoot("", "rel/path"); err == nil {
		t.Fatal("want reject empty ResolveUnderRoot root")
	}
	if _, err := validate.ResolveUnderRoot(root, ""); err == nil {
		t.Fatal("want reject empty rel")
	}
	if _, err := validate.ResolveUnderRoot(root, "."); err == nil {
		t.Fatal("want reject cleaned rel '.'")
	}
	if _, err := validate.ResolveUnderRoot(root, string([]byte{'a', 0, 'b'})); err == nil {
		t.Fatal("want reject null byte in rel")
	}
	if err := validate.DataPathSegment("", string([]byte{'x', 0})); err == nil {
		t.Fatal("want reject null byte segment")
	}
}

func TestReadableFilePathDirectoryRejected(t *testing.T) {
	dir := t.TempDir()
	if _, err := validate.ReadableFilePath(dir); err == nil || !validate.IsInvalid(err) {
		t.Fatalf("directory should fail: %v", err)
	}
	if _, err := validate.ReadableFilePath(""); err == nil {
		t.Fatal("want reject empty path")
	}
	if _, err := validate.ReadableFilePath("."); err == nil {
		t.Fatal("want reject '.'")
	}
	p := filepath.Join(dir, "ok.txt")
	if err := os.WriteFile(p, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := validate.ReadableFilePath(p); err != nil {
		t.Fatal(err)
	}
}
