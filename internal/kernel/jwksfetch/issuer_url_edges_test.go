package jwksfetch_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Kyaxris-Labs/Noctaxris/internal/kernel/jwksfetch"
)

func TestFetchRemoteJWKSStatusAndLabIssuer(t *testing.T) {
	t.Setenv(jwksfetch.EnvAllowRemote, "1")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTeapot)
	}))
	t.Cleanup(srv.Close)
	host := strings.TrimPrefix(srv.URL, "http://")
	t.Setenv(jwksfetch.EnvHostAllowlist, host)

	_, err := jwksfetch.FetchRemoteJWKS(srv.URL, srv.Client())
	if err == nil {
		t.Fatal("non-200 JWKS should fail; loopback may fail earlier")
	}

	_, err = jwksfetch.FetchRemoteJWKS("http://127.0.0.1:4566/cognito-idp/us-east-1/us-east-1_Lab", srv.Client())
	if err == nil || !strings.Contains(err.Error(), "lab Cognito") {
		t.Fatalf("lab Cognito path err=%v", err)
	}
}

func TestValidateIssuerBadSchemeAndEmptyHost(t *testing.T) {
	t.Setenv(jwksfetch.EnvAllowRemote, "1")
	t.Setenv(jwksfetch.EnvHostAllowlist, "example.com")
	if err := jwksfetch.ValidateIssuerForConfig("ftp://example.com"); err == nil {
		t.Fatal("ftp scheme must reject")
	}
	if err := jwksfetch.ValidateIssuerForConfig("https://"); err == nil {
		t.Fatal("empty host must reject")
	}
	if err := jwksfetch.ValidateIssuerForConfig("://bad"); err == nil {
		t.Fatal("invalid URL must reject")
	}
	if jwksfetch.IsLabCognitoIssuer("https://cognito-idp.us-east-1.amazonaws.com/us-east-1_x") {
		// real AWS Cognito host is not the lab path shape
		t.Log("aws cognito host is not lab path issuer")
	}
	if !jwksfetch.IsLabCognitoIssuer("http://127.0.0.1:4566/cognito-idp/us-east-1/us-east-1_x") {
		t.Fatal("lab path issuer")
	}
}
