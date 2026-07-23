package authz_test

import (
	"testing"

	"github.com/Kyaxris-Labs/Noctaxris/internal/kernel/authz"
	"github.com/Kyaxris-Labs/Noctaxris/internal/kernel/identity"
)

func TestResourcePolicyPrincipalLessAllowDoesNotGrant(t *testing.T) {
	caller := authz.RequestContext{
		Principal: identity.Principal{
			Kind:      identity.KindUser,
			AccountID: "000000000001",
			UserName:  "alice",
		},
		Action:   "kms:Decrypt",
		Resource: "arn:aws:kms:us-east-1:000000000001:key/abc",
	}
	identityAllow := `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Action":"kms:Decrypt","Resource":"*"}]}`
	principalLess := `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Action":"kms:*","Resource":"*"}]}`
	got := authz.EvaluateKMS(authz.KMSRequest{
		Caller:       caller,
		IdentityDocs: []string{identityAllow},
		KeyPolicyDoc: principalLess,
	})
	if got != authz.Deny {
		t.Fatalf("Principal-less key policy Allow got %v, want Deny", got)
	}
}

func TestResourcePolicyPrincipalBearingAllowWorks(t *testing.T) {
	caller := authz.RequestContext{
		Principal: identity.Principal{
			Kind:      identity.KindUser,
			AccountID: "000000000001",
			UserName:  "alice",
		},
		Action:   "kms:Decrypt",
		Resource: "arn:aws:kms:us-east-1:000000000001:key/abc",
	}
	identityAllow := `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Action":"kms:Decrypt","Resource":"*"}]}`
	withPrincipal := `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":{"AWS":"arn:aws:iam::000000000001:user/alice"},"Action":"kms:*","Resource":"*"}]}`
	got := authz.EvaluateKMS(authz.KMSRequest{
		Caller:       caller,
		IdentityDocs: []string{identityAllow},
		KeyPolicyDoc: withPrincipal,
	})
	if got != authz.Allow {
		t.Fatalf("Principal-bearing key policy Allow got %v, want Allow", got)
	}
}

func TestValidateResourcePolicyDocumentRequiresPrincipal(t *testing.T) {
	err := authz.ValidateResourcePolicyDocument(
		`{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Action":"s3:GetObject","Resource":"*"}]}`,
	)
	if err == nil {
		t.Fatal("expected Principal required error")
	}
	err = authz.ValidateResourcePolicyDocument(
		`{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":{"AWS":"*"},"Action":"s3:GetObject","Resource":"*"}]}`,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestIdentityPolicyStillAllowsOmittedPrincipal(t *testing.T) {
	caller := authz.RequestContext{
		Principal: identity.Principal{
			Kind:      identity.KindUser,
			AccountID: "000000000001",
			UserName:  "alice",
		},
		Action:   "s3:GetObject",
		Resource: "arn:aws:s3:::lab/a.txt",
	}
	identityAllow := `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Action":"s3:GetObject","Resource":"*"}]}`
	got := authz.EvaluateResourceAccess(authz.ResourceAccessRequest{
		Caller:            caller,
		IdentityDocs:      []string{identityAllow},
		ResourcePolicyDoc: "",
		ResourceAccountID: "000000000001",
	})
	if got != authz.Allow {
		t.Fatalf("identity-only Principal-optional got %v, want Allow", got)
	}
}
