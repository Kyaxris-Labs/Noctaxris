package conditionkeys

import "testing"

func TestKnownEmptyLabOIDCAndExact(t *testing.T) {
	if Known("") {
		t.Fatal("empty key must be unknown")
	}
	if !Known("token.actions.githubusercontent.com:sub") {
		t.Fatal("lab GitHub Actions :sub")
	}
	if !Known("token.actions.githubusercontent.com:aud") {
		t.Fatal("lab GitHub Actions :aud")
	}
	if !Known("oidc.example.com:sub") {
		t.Fatal("lab OIDC host:sub shape")
	}
	if !Known("issuer.example/path:aud") {
		t.Fatal("lab OIDC path:aud shape")
	}
	if Known(":sub") {
		t.Fatal("bare :sub suffix without host")
	}
	if Known("nosuffix") {
		t.Fatal("random unknown")
	}
}
