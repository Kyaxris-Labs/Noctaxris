package authz_test

import (
	"testing"

	"github.com/Kyaxris-Labs/Noctaxris/internal/kernel/authz"
	"github.com/Kyaxris-Labs/Noctaxris/internal/kernel/identity"
)

func TestEvaluateResourceAccessMatrix(t *testing.T) {
	const (
		callerAccount   = "000000000001"
		resourceAccount = "000000000002"
		action          = "s3:GetObject"
		resource        = "arn:aws:s3:::lab/a.txt"
	)

	identityAllow := `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Action":"s3:GetObject","Resource":"*"}]}`
	identityDeny := `{"Version":"2012-10-17","Statement":[{"Effect":"Deny","Action":"s3:GetObject","Resource":"*"}]}`
	resourceAllow := `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":{"AWS":"arn:aws:iam::000000000001:user/alice"},"Action":"s3:GetObject","Resource":"*"}]}`
	resourceDeny := `{"Version":"2012-10-17","Statement":[{"Effect":"Deny","Principal":{"AWS":"*"},"Action":"s3:GetObject","Resource":"*"}]}`

	caller := authz.RequestContext{
		Principal: identity.Principal{
			Kind:      identity.KindUser,
			AccountID: callerAccount,
			UserName:  "alice",
		},
		Action:   action,
		Resource: resource,
	}

	tests := []struct {
		name              string
		resourceAccountID string
		identityDocs      []string
		resourcePolicyDoc string
		want              authz.Decision
	}{
		{
			name:              "same-account identity-only",
			resourceAccountID: callerAccount,
			identityDocs:      []string{identityAllow},
			resourcePolicyDoc: "",
			want:              authz.Allow,
		},
		{
			name:              "same-account policy-only",
			resourceAccountID: "",
			identityDocs:      nil,
			resourcePolicyDoc: resourceAllow,
			want:              authz.Allow,
		},
		{
			name:              "same-account both",
			resourceAccountID: callerAccount,
			identityDocs:      []string{identityAllow},
			resourcePolicyDoc: resourceAllow,
			want:              authz.Allow,
		},
		{
			name:              "same-account identity deny",
			resourceAccountID: callerAccount,
			identityDocs:      []string{identityDeny},
			resourcePolicyDoc: resourceAllow,
			want:              authz.Deny,
		},
		{
			name:              "same-account resource deny",
			resourceAccountID: callerAccount,
			identityDocs:      []string{identityAllow},
			resourcePolicyDoc: resourceDeny,
			want:              authz.Deny,
		},
		{
			name:              "cross-account both allow",
			resourceAccountID: resourceAccount,
			identityDocs:      []string{identityAllow},
			resourcePolicyDoc: resourceAllow,
			want:              authz.Allow,
		},
		{
			name:              "cross-account identity-only",
			resourceAccountID: resourceAccount,
			identityDocs:      []string{identityAllow},
			resourcePolicyDoc: "",
			want:              authz.Deny,
		},
		{
			name:              "cross-account policy-only",
			resourceAccountID: resourceAccount,
			identityDocs:      nil,
			resourcePolicyDoc: resourceAllow,
			want:              authz.Deny,
		},
		{
			name:              "cross-account empty policy",
			resourceAccountID: resourceAccount,
			identityDocs:      []string{identityAllow},
			resourcePolicyDoc: "   ",
			want:              authz.Deny,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := authz.EvaluateResourceAccess(authz.ResourceAccessRequest{
				Caller:            caller,
				IdentityDocs:      tt.identityDocs,
				ResourcePolicyDoc: tt.resourcePolicyDoc,
				ResourceAccountID: tt.resourceAccountID,
			})
			if got != tt.want {
				t.Fatalf("got %v, want %v", got, tt.want)
			}
		})
	}
}

func TestEvaluateResourceAccessAccountRootIsNotDirectGrant(t *testing.T) {
	const (
		account  = "000000000001"
		other    = "000000000002"
		action   = "s3:GetObject"
		resource = "arn:aws:s3:::lab/a.txt"
	)
	alice := authz.RequestContext{
		Principal: identity.Principal{
			Kind:      identity.KindUser,
			AccountID: account,
			UserName:  "alice",
		},
		Action:   action,
		Resource: resource,
	}
	roleSess := authz.RequestContext{
		Principal: identity.RoleSessionPrincipal(account, "AppRole", "sess", "ASIA"),
		Action:    action,
		Resource:  resource,
	}
	identityAllow := `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Action":"s3:GetObject","Resource":"*"}]}`
	rootAllow := `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":{"AWS":"arn:aws:iam::` + account + `:root"},"Action":"s3:GetObject","Resource":"*"}]}`
	acctAllow := `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":{"AWS":"` + account + `"},"Action":"s3:GetObject","Resource":"*"}]}`
	rootDeny := `{"Version":"2012-10-17","Statement":[{"Effect":"Deny","Principal":{"AWS":"arn:aws:iam::` + account + `:root"},"Action":"s3:GetObject","Resource":"*"}]}`
	namedAllow := `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":{"AWS":"arn:aws:iam::` + account + `:user/alice"},"Action":"s3:GetObject","Resource":"*"}]}`

	if got := authz.EvaluateResourceAccess(authz.ResourceAccessRequest{
		Caller: alice, ResourcePolicyDoc: rootAllow, ResourceAccountID: account,
	}); got != authz.Deny {
		t.Fatalf("same-account :root without identity got %v, want Deny", got)
	}
	if got := authz.EvaluateResourceAccess(authz.ResourceAccessRequest{
		Caller: alice, ResourcePolicyDoc: acctAllow, ResourceAccountID: account,
	}); got != authz.Deny {
		t.Fatalf("same-account account id without identity got %v, want Deny", got)
	}
	if got := authz.EvaluateResourceAccess(authz.ResourceAccessRequest{
		Caller: roleSess, ResourcePolicyDoc: rootAllow, ResourceAccountID: account,
	}); got != authz.Deny {
		t.Fatalf("same-account :root for role session got %v, want Deny", got)
	}
	if got := authz.EvaluateResourceAccess(authz.ResourceAccessRequest{
		Caller: alice, IdentityDocs: []string{identityAllow}, ResourcePolicyDoc: rootAllow, ResourceAccountID: account,
	}); got != authz.Allow {
		t.Fatalf("same-account identity Allow with :root bucket policy got %v, want Allow", got)
	}
	if got := authz.EvaluateResourceAccess(authz.ResourceAccessRequest{
		Caller: alice, IdentityDocs: []string{identityAllow}, ResourcePolicyDoc: rootDeny, ResourceAccountID: account,
	}); got != authz.Deny {
		t.Fatalf("same-account :root Deny with identity Allow got %v, want Deny", got)
	}
	if got := authz.EvaluateResourceAccess(authz.ResourceAccessRequest{
		Caller: alice, IdentityDocs: []string{identityAllow}, ResourcePolicyDoc: rootAllow, ResourceAccountID: other,
	}); got != authz.Allow {
		t.Fatalf("cross-account :root plus identity got %v, want Allow", got)
	}
	if got := authz.EvaluateResourceAccess(authz.ResourceAccessRequest{
		Caller: alice, ResourcePolicyDoc: namedAllow, ResourceAccountID: account,
	}); got != authz.Allow {
		t.Fatalf("named user Principal still grants same-account OR got %v, want Allow", got)
	}
}
