package authz_test

import (
	"testing"

	"github.com/Kyaxris-Labs/Noctaxris/internal/kernel/authz"
	"github.com/Kyaxris-Labs/Noctaxris/internal/kernel/identity"
)

func TestEvaluateDynamoDBIdentityAllowEmptyResource(t *testing.T) {
	ctx := authz.RequestContext{
		Principal: identity.Principal{
			Kind:      identity.KindUser,
			AccountID: "000000000001",
			UserName:  "alice",
		},
		Action:   "dynamodb:GetItem",
		Resource: "arn:aws:dynamodb:us-east-1:000000000001:table/lab",
	}
	identityAllow := `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":{"AWS":"arn:aws:iam::000000000001:user/alice"},"Action":"dynamodb:GetItem","Resource":"*"}]}`
	got := authz.EvaluateDynamoDB(authz.DynamoDBRequest{
		Caller:            ctx,
		IdentityDocs:      []string{identityAllow},
		ResourcePolicyDoc: "",
	})
	if got != authz.Allow {
		t.Fatalf("got %v, want Allow", got)
	}
}

func TestEvaluateDynamoDBIdentityDenyResourceAllow(t *testing.T) {
	ctx := authz.RequestContext{
		Principal: identity.Principal{
			Kind:      identity.KindUser,
			AccountID: "000000000001",
			UserName:  "alice",
		},
		Action:   "dynamodb:GetItem",
		Resource: "arn:aws:dynamodb:us-east-1:000000000001:table/lab",
	}
	identityDeny := `{"Version":"2012-10-17","Statement":[{"Effect":"Deny","Action":"dynamodb:GetItem","Resource":"*"}]}`
	resourceAllow := `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":{"AWS":"arn:aws:iam::000000000001:user/alice"},"Action":"dynamodb:GetItem","Resource":"*"}]}`
	got := authz.EvaluateDynamoDB(authz.DynamoDBRequest{
		Caller:            ctx,
		IdentityDocs:      []string{identityDeny},
		ResourcePolicyDoc: resourceAllow,
	})
	if got != authz.Deny {
		t.Fatalf("got %v, want Deny", got)
	}
}

func TestEvaluateDynamoDBIdentityEmptyResourceAllow(t *testing.T) {
	ctx := authz.RequestContext{
		Principal: identity.Principal{
			Kind:      identity.KindUser,
			AccountID: "000000000001",
			UserName:  "alice",
		},
		Action:   "dynamodb:GetItem",
		Resource: "arn:aws:dynamodb:us-east-1:000000000001:table/lab",
	}
	resourceAllow := `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":{"AWS":"arn:aws:iam::000000000001:user/alice"},"Action":"dynamodb:GetItem","Resource":"*"}]}`
	got := authz.EvaluateDynamoDB(authz.DynamoDBRequest{
		Caller:            ctx,
		IdentityDocs:      nil,
		ResourcePolicyDoc: resourceAllow,
	})
	if got != authz.Allow {
		t.Fatalf("got %v, want Allow", got)
	}
}
