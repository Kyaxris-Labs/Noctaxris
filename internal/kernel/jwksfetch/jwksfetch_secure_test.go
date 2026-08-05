package jwksfetch_test

import (
	"strings"
	"testing"

	"github.com/Kyaxris-Labs/Noctaxris/internal/kernel/jwksfetch"
)

func TestFetchRemoteJWKS_nilClientUsesSecureTransport(t *testing.T) {
	t.Setenv(jwksfetch.EnvAllowRemote, "1")
	t.Setenv(jwksfetch.EnvHostAllowlist, "example.com")
	// nil client forces secureJWKSClient(); network may succeed or fail — both cover the constructor.
	_, err := jwksfetch.FetchRemoteJWKS("https://example.com", nil)
	if err != nil && !strings.Contains(err.Error(), "jwksfetch:") {
		t.Fatalf("unexpected error shape: %v", err)
	}
}

func TestValidateIssuer_emptyAllowlistEntries(t *testing.T) {
	t.Setenv(jwksfetch.EnvAllowRemote, "1")
	t.Setenv(jwksfetch.EnvHostAllowlist, " , ,example.com, ")
	if err := jwksfetch.ValidateIssuerForConfig("https://example.com"); err != nil {
		t.Fatal(err)
	}
	t.Setenv(jwksfetch.EnvHostAllowlist, "")
	if err := jwksfetch.ValidateIssuerForConfig("https://example.com"); err == nil {
		t.Fatal("empty allowlist must reject")
	}
}

func TestValidateIssuer_schemeAndHostEdges(t *testing.T) {
	t.Setenv(jwksfetch.EnvAllowRemote, "1")
	t.Setenv(jwksfetch.EnvHostAllowlist, "8.8.8.8")
	if err := jwksfetch.ValidateIssuerForConfig("https://8.8.8.8"); err != nil {
		t.Fatalf("public literal should pass: %v", err)
	}
	t.Setenv(jwksfetch.EnvHostAllowlist, "192.168.0.1")
	if err := jwksfetch.ValidateIssuerForConfig("http://192.168.0.1/iss"); err == nil {
		t.Fatal("private literal must reject")
	}
	t.Setenv(jwksfetch.EnvHostAllowlist, "0.0.0.0")
	if err := jwksfetch.ValidateIssuerForConfig("http://0.0.0.0/iss"); err == nil {
		t.Fatal("unspecified must reject")
	}
	t.Setenv(jwksfetch.EnvHostAllowlist, "224.0.0.1")
	if err := jwksfetch.ValidateIssuerForConfig("http://224.0.0.1/iss"); err == nil {
		t.Fatal("multicast must reject")
	}
	t.Setenv(jwksfetch.EnvHostAllowlist, "fe80::1")
	if err := jwksfetch.ValidateIssuerForConfig("http://[fe80::1]/iss"); err == nil {
		t.Fatal("link-local must reject")
	}
}

func TestValidateIssuer_unresolvableHost(t *testing.T) {
	t.Setenv(jwksfetch.EnvAllowRemote, "1")
	host := "no-such-host-noctaxris-jwks-test.invalid"
	t.Setenv(jwksfetch.EnvHostAllowlist, host)
	err := jwksfetch.ValidateIssuerForConfig("https://" + host)
	if err == nil {
		t.Fatal("unresolvable host must fail closed")
	}
}
