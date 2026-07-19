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
