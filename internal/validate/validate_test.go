package validate_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Kyaxris-Labs/Noctaxris/internal/validate"
)

func TestAccountID(t *testing.T) {
	if err := validate.AccountID("000000000001"); err != nil {
		t.Fatal(err)
	}
	if err := validate.AccountID("abc"); err == nil || !validate.IsInvalid(err) {
		t.Fatalf("want invalid, got %v", err)
	}
}

func TestIAMName(t *testing.T) {
	if err := validate.IAMName("lab-user_1"); err != nil {
		t.Fatal(err)
	}
	if err := validate.IAMName("../evil"); err == nil {
		t.Fatal("expected reject")
	}
	if err := validate.IAMName(""); err == nil {
		t.Fatal("expected reject empty")
	}
}

func TestEmail(t *testing.T) {
	if err := validate.Email("member@example.com"); err != nil {
		t.Fatal(err)
	}
	if err := validate.Email("not-an-email"); err == nil {
		t.Fatal("expected reject")
	}
}

func TestPolicyDocument(t *testing.T) {
	ok := `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Action":"*","Resource":"*"}]}`
	if err := validate.PolicyDocument(ok); err != nil {
		t.Fatal(err)
	}
	if err := validate.PolicyDocument(`{"Version":"2012-10-17"}`); err == nil {
		t.Fatal("expected reject missing Statement")
	}
	if err := validate.PolicyDocument(`not-json`); err == nil {
		t.Fatal("expected reject")
	}
}

func TestDataPathSegment(t *testing.T) {
	if err := validate.DataPathSegment("UserName", "alice"); err != nil {
		t.Fatal(err)
	}
	if err := validate.DataPathSegment("Name", "job_1.v2"); err != nil {
		t.Fatal(err)
	}
	for _, bad := range []string{"", ".", "..", "../evil", `..\evil`, "a/b", `a\b`, "bad name"} {
		if err := validate.DataPathSegment("Name", bad); err == nil || !validate.IsInvalid(err) {
			t.Fatalf("Name %q: want invalid, got %v", bad, err)
		}
	}
}

func TestPathUnderRootAndJoinDataPath(t *testing.T) {
	root := t.TempDir()
	joined, err := validate.JoinDataPath(root, "transfer", "alice")
	if err != nil {
		t.Fatal(err)
	}
	if !validate.PathUnderRoot(root, joined) {
		t.Fatalf("joined %q not under %q", joined, root)
	}
	if _, err := validate.JoinDataPath(root, "..", "escape"); err == nil {
		t.Fatal("expected reject .. segment")
	}
	outside := filepath.Join(filepath.Dir(root), "outside")
	if validate.PathUnderRoot(root, outside) {
		t.Fatalf("outside %q should not be under %q", outside, root)
	}
}

func TestResolveUnderRoot(t *testing.T) {
	root := t.TempDir()
	got, err := validate.ResolveUnderRoot(root, "ecr/manifests/acct/repo/sha256/ab.json")
	if err != nil {
		t.Fatal(err)
	}
	if !validate.PathUnderRoot(root, got) {
		t.Fatalf("resolved %q not under %q", got, root)
	}
	if _, err := validate.ResolveUnderRoot(root, "../outside.json"); err == nil || !validate.IsInvalid(err) {
		t.Fatalf("want escape reject, got %v", err)
	}
	if _, err := validate.ResolveUnderRoot(root, "/abs/path"); err == nil || !validate.IsInvalid(err) {
		t.Fatalf("want absolute reject, got %v", err)
	}
}

func TestReadableFilePath(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "meta.xml")
	if err := os.WriteFile(p, []byte("<EntityDescriptor/>"), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := validate.ReadableFilePath(p)
	if err != nil {
		t.Fatal(err)
	}
	if got != filepath.Clean(p) {
		t.Fatalf("got %q", got)
	}
	if _, err := validate.ReadableFilePath(filepath.Join(dir, "missing.xml")); err == nil {
		t.Fatal("expected missing reject")
	}
}

func TestOIDCIssuerURL(t *testing.T) {
	if err := validate.OIDCIssuerURL("https://token.actions.githubusercontent.com"); err != nil {
		t.Fatal(err)
	}
	if err := validate.OIDCIssuerURL("ftp://bad"); err == nil {
		t.Fatal("expected reject")
	}
}

func TestRoleARN(t *testing.T) {
	if err := validate.RoleARN("arn:aws:iam::000000000001:role/LabRole"); err != nil {
		t.Fatal(err)
	}
	if err := validate.RoleARN("arn:aws:iam::000000000001:user/x"); err == nil {
		t.Fatal("expected reject")
	}
}
