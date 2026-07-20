package jwksfetch_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Kyaxris-Labs/Noctaxris/internal/kernel/jwksfetch"
)

func TestParseLabCognitoIssuer(t *testing.T) {
	t.Parallel()
	region, pool, ok := jwksfetch.ParseLabCognitoIssuer("http://127.0.0.1:4566/cognito-idp/us-east-1/us-east-1_abc")
	if !ok || region != "us-east-1" || pool != "us-east-1_abc" {
		t.Fatalf("lab issuer parse: ok=%v region=%q pool=%q", ok, region, pool)
	}
	region, pool, ok = jwksfetch.ParseLabCognitoIssuer("/cognito-idp/eu-west-1/eu-west-1_xyz")
	if !ok || region != "eu-west-1" || pool != "eu-west-1_xyz" {
		t.Fatalf("path issuer parse: ok=%v region=%q pool=%q", ok, region, pool)
	}
	if _, _, ok := jwksfetch.ParseLabCognitoIssuer("https://evil.example/cognito-idp/us-east-1/p"); ok {
		t.Fatal("evil host must not parse as lab issuer")
	}
}

func TestValidateIssuerRejectsRemoteByDefault(t *testing.T) {
	t.Setenv(jwksfetch.EnvAllowRemote, "")
	t.Setenv(jwksfetch.EnvHostAllowlist, "")
	err := jwksfetch.ValidateIssuerForConfig("https://evil.example/.well-known")
	if err == nil {
		t.Fatal("expected reject")
	}
	if !strings.Contains(err.Error(), jwksfetch.EnvAllowRemote) {
		t.Fatalf("error=%v", err)
	}
	if err := jwksfetch.ValidateIssuerForConfig("http://127.0.0.1:4566/cognito-idp/us-east-1/us-east-1_lab"); err != nil {
		t.Fatalf("lab issuer: %v", err)
	}
}

func TestValidateIssuerRemoteAllowlistBlocksPrivate(t *testing.T) {
	t.Setenv(jwksfetch.EnvAllowRemote, "1")
	t.Setenv(jwksfetch.EnvHostAllowlist, "169.254.169.254,10.0.0.5,127.0.0.1:9999")
	for _, issuer := range []string{
		"http://169.254.169.254/latest",
		"http://10.0.0.5/jwks",
		"http://127.0.0.1:9999/iss",
	} {
		err := jwksfetch.ValidateIssuerForConfig(issuer)
		if err == nil {
			t.Fatalf("expected reject for %s", issuer)
		}
	}
}

func TestFetchRemoteJWKSNoRedirectAndAllowlist(t *testing.T) {
	t.Setenv(jwksfetch.EnvAllowRemote, "1")
	var hits int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		if r.URL.Path != "/.well-known/jwks.json" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"keys":[]}`))
	}))
	t.Cleanup(srv.Close)

	u := strings.TrimPrefix(srv.URL, "http://")
	t.Setenv(jwksfetch.EnvHostAllowlist, u)

	// httptest uses 127.0.0.1 loopback; remote policy rejects loopback even when listed.
	_, err := jwksfetch.FetchRemoteJWKS(srv.URL, srv.Client())
	if err == nil {
		t.Fatal("expected loopback remote fetch to fail closed")
	}
	if hits != 0 {
		t.Fatalf("expected no HTTP hit before host safety check, hits=%d", hits)
	}
}

func TestFetchRemoteJWKSDisabled(t *testing.T) {
	t.Setenv(jwksfetch.EnvAllowRemote, "")
	_, err := jwksfetch.FetchRemoteJWKS("https://example.com", nil)
	if err == nil {
		t.Fatal("expected error")
	}
}
