package server

import (
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

func TestResolveRestAuthorizerIdentityTokenSources(t *testing.T) {
	req := httptest.NewRequest("GET", "http://example.com/prod/ping?q=abc", nil)
	req.Header.Set("Authorization", "Bearer root-tok")
	req.Header.Set("X-Lab", "header-val")

	cases := []struct {
		source string
		want   string
	}{
		{"method.request.header.Authorization", "Bearer root-tok"},
		{"$request.header.X-Lab", "header-val"},
		{"method.request.querystring.q", "abc"},
		{"$request.querystring.q", "abc"},
		{"Authorization", "Bearer root-tok"},
		{"", "Bearer root-tok"},
	}
	for _, tc := range cases {
		got := resolveRestAuthorizerIdentityToken(req, tc.source)
		if got != tc.want {
			t.Fatalf("source=%q got %q want %q", tc.source, got, tc.want)
		}
	}
}

func TestBuildRestAPIAuthorizerEventTOKEN(t *testing.T) {
	req := httptest.NewRequest("GET", "http://example.com/stage/items", nil)
	req.Header.Set("Authorization", "tok")
	raw, err := buildRestAPIAuthorizerEvent(
		req, "000000000001", "apiid", "stage", "/items", "/items", "resid", "reqid",
		store.RestAuthorizer{Type: store.APIGatewayAuthorizerTOKEN, IdentitySource: "method.request.header.Authorization"},
		"arn:aws:execute-api:us-east-1:1:apiid/stage/GET/items",
	)
	if err != nil {
		t.Fatal(err)
	}
	s := string(raw)
	for _, part := range []string{`"TOKEN"`, `"authorizationToken":"tok"`, `"methodArn"`} {
		if !strings.Contains(s, part) {
			t.Fatalf("missing %s in event=%s", part, s)
		}
	}
}

func TestBuildRestAPIAuthorizerEventREQUEST(t *testing.T) {
	req := httptest.NewRequest("POST", "http://example.com/stage/items?x=1", nil)
	req.Header.Set("Authorization", "tok")
	raw, err := buildRestAPIAuthorizerEvent(
		req, "000000000001", "apiid", "stage", "/items", "/items", "resid", "reqid",
		store.RestAuthorizer{Type: store.APIGatewayAuthorizerREQUEST, IdentitySource: "method.request.header.Authorization"},
		"arn:aws:execute-api:us-east-1:1:apiid/stage/POST/items",
	)
	if err != nil {
		t.Fatal(err)
	}
	s := string(raw)
	for _, part := range []string{`"REQUEST"`, `"httpMethod":"POST"`, `"requestContext"`} {
		if !strings.Contains(s, part) {
			t.Fatalf("missing %s in event=%s", part, s)
		}
	}
}
