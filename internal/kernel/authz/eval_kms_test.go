package authz_test

import (
	"testing"

	"github.com/Kyaxris-Labs/Noctaxris/internal/kernel/authz"
	"github.com/Kyaxris-Labs/Noctaxris/internal/kernel/identity"
)

func TestEvaluateKMSIdentityAllowEmptyKeyPolicyOnKeyARNDenies(t *testing.T) {
	ctx := authz.RequestContext{
		Principal: identity.Principal{
			Kind:      identity.KindUser,
			AccountID: "000000000001",
			UserName:  "alice",
		},
		Action:   "kms:Encrypt",
		Resource: "arn:aws:kms:us-east-1:000000000001:key/abc",
	}
	identityAllow := `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Action":"kms:Encrypt","Resource":"*"}]}`
	got := authz.EvaluateKMS(authz.KMSRequest{
		Caller:       ctx,
		IdentityDocs: []string{identityAllow},
		KeyPolicyDoc: "",
	})
	if got != authz.Deny {
		t.Fatalf("got %v, want Deny", got)
	}
}

func TestEvaluateKMSBothAllow(t *testing.T) {
	ctx := authz.RequestContext{
		Principal: identity.Principal{
			Kind:      identity.KindUser,
			AccountID: "000000000001",
			UserName:  "alice",
		},
		Action:   "kms:Encrypt",
		Resource: "arn:aws:kms:us-east-1:000000000001:key/abc",
	}
	identityAllow := `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Action":"kms:Encrypt","Resource":"*"}]}`
	keyAllow := `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Action":"kms:*","Resource":"*"}]}`
	got := authz.EvaluateKMS(authz.KMSRequest{
		Caller:       ctx,
		IdentityDocs: []string{identityAllow},
		KeyPolicyDoc: keyAllow,
	})
	if got != authz.Allow {
		t.Fatalf("got %v, want Allow", got)
	}
}

func TestEvaluateKMSGrantPathAllow(t *testing.T) {
	ctx := authz.RequestContext{
		Principal: identity.Principal{
			Kind:      identity.KindUser,
			AccountID: "000000000001",
			UserName:  "bob",
		},
		Action:   "kms:Encrypt",
		Resource: "arn:aws:kms:us-east-1:000000000001:key/abc",
	}
	keyAllow := `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Action":"kms:*","Resource":"*"}]}`
	got := authz.EvaluateKMS(authz.KMSRequest{
		Caller:         ctx,
		IdentityDocs:   nil,
		KeyPolicyDoc:   keyAllow,
		GrantSatisfied: true,
	})
	if got != authz.Allow {
		t.Fatalf("got %v, want Allow", got)
	}
}

func TestEvaluateKMSCreateKeyIdentityOnly(t *testing.T) {
	ctx := authz.RequestContext{
		Principal: identity.RootPrincipal("000000000001", "AKIAROOT"),
		Action:    "kms:CreateKey",
		Resource:  "*",
	}
	got := authz.EvaluateKMS(authz.KMSRequest{
		Caller:       ctx,
		IdentityDocs: nil,
		KeyPolicyDoc: "",
	})
	if got != authz.Allow {
		t.Fatalf("root CreateKey got %v, want Allow", got)
	}
}

func TestEvaluateKMSKeyPolicyDenyOverridesGrant(t *testing.T) {
	ctx := authz.RequestContext{
		Principal: identity.Principal{
			Kind:      identity.KindUser,
			AccountID: "000000000001",
			UserName:  "bob",
		},
		Action:   "kms:Encrypt",
		Resource: "arn:aws:kms:us-east-1:000000000001:key/abc",
	}
	keyDeny := `{"Version":"2012-10-17","Statement":[{"Effect":"Deny","Action":"kms:Encrypt","Resource":"*"}]}`
	got := authz.EvaluateKMS(authz.KMSRequest{
		Caller:         ctx,
		IdentityDocs:   nil,
		KeyPolicyDoc:   keyDeny,
		GrantSatisfied: true,
	})
	if got != authz.Deny {
		t.Fatalf("got %v, want Deny", got)
	}
}
