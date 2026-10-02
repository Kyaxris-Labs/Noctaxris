package federation

import (
	"testing"
)

func TestWebIdentityConditionKeysAndHosts(t *testing.T) {
	t.Parallel()
	if oidcIssuerHost("") != "" {
		t.Fatal("empty host")
	}
	if got := oidcIssuerHost("https://token.actions.githubusercontent.com/"); got != "token.actions.githubusercontent.com" {
		t.Fatalf("host=%q", got)
	}
	if !isGitHubActionsIssuer("token.actions.githubusercontent.com") {
		t.Fatal("github issuer")
	}
	if !isGitHubActionsIssuer("token.actions.example.ghe.com") {
		t.Fatal("ghe issuer")
	}
	if isGitHubActionsIssuer("accounts.google.com") {
		t.Fatal("non-github")
	}

	keys := WebIdentityConditionKeys(OIDCClaims{
		Issuer:   "https://token.actions.githubusercontent.com",
		Subject:  "repo:org/app:ref:refs/heads/main",
		Audience: []string{"sts.amazonaws.com"},
		StringClaims: map[string]string{
			"repository": "org/app",
			"ref":        "refs/heads/main",
			"actor":      "bot",
		},
	})
	if keys["token.actions.githubusercontent.com:sub"] == "" || keys["token.actions.githubusercontent.com:aud"] != "sts.amazonaws.com" {
		t.Fatalf("keys=%v", keys)
	}
	if keys["token.actions.githubusercontent.com:repository"] != "org/app" {
		t.Fatalf("repo key=%v", keys)
	}

	empty := WebIdentityConditionKeys(OIDCClaims{})
	if len(empty) != 0 {
		t.Fatalf("empty=%v", empty)
	}

	_, err := VerifyWebIdentityJWT("", "https://example.com", "client", nil)
	if err == nil {
		t.Fatal("empty token should fail")
	}
	if fe, ok := err.(*Error); !ok || fe.Code() != CodeInvalidIdentityToken {
		t.Fatalf("err=%v", err)
	}
	_, err = VerifyWebIdentityJWTClients("tok", "", []string{"c"}, nil)
	if err == nil {
		t.Fatal("empty issuer should fail")
	}
	_, err = VerifyWebIdentityJWTClients("tok", "https://example.com", nil, nil)
	if err == nil {
		t.Fatal("empty client list should fail")
	}
}
