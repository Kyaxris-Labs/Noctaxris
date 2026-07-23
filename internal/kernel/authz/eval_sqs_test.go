package authz_test

import (
	"testing"

	"github.com/Kyaxris-Labs/Noctaxris/internal/kernel/authz"
	"github.com/Kyaxris-Labs/Noctaxris/internal/kernel/identity"
)

func TestEvaluateSQSIdentityAllowEmptyQueue(t *testing.T) {
	ctx := authz.RequestContext{
		Principal: identity.Principal{
			Kind:      identity.KindUser,
			AccountID: "000000000001",
			UserName:  "alice",
		},
		Action:   "sqs:SendMessage",
		Resource: "arn:aws:sqs:us-east-1:000000000001:lab",
	}
	identityAllow := `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":{"AWS":"arn:aws:iam::000000000001:user/alice"},"Action":"sqs:SendMessage","Resource":"*"}]}`
	got := authz.EvaluateSQS(authz.SQSRequest{
		Caller:         ctx,
		IdentityDocs:   []string{identityAllow},
		QueuePolicyDoc: "",
	})
	if got != authz.Allow {
		t.Fatalf("got %v, want Allow", got)
	}
}

func TestEvaluateSQSIdentityDenyQueueAllow(t *testing.T) {
	ctx := authz.RequestContext{
		Principal: identity.Principal{
			Kind:      identity.KindUser,
			AccountID: "000000000001",
			UserName:  "alice",
		},
		Action:   "sqs:SendMessage",
		Resource: "arn:aws:sqs:us-east-1:000000000001:lab",
	}
	identityDeny := `{"Version":"2012-10-17","Statement":[{"Effect":"Deny","Action":"sqs:SendMessage","Resource":"*"}]}`
	queueAllow := `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":{"AWS":"arn:aws:iam::000000000001:user/alice"},"Action":"sqs:SendMessage","Resource":"*"}]}`
	got := authz.EvaluateSQS(authz.SQSRequest{
		Caller:         ctx,
		IdentityDocs:   []string{identityDeny},
		QueuePolicyDoc: queueAllow,
	})
	if got != authz.Deny {
		t.Fatalf("got %v, want Deny", got)
	}
}

func TestEvaluateSQSIdentityEmptyQueueAllow(t *testing.T) {
	ctx := authz.RequestContext{
		Principal: identity.Principal{
			Kind:      identity.KindUser,
			AccountID: "000000000001",
			UserName:  "alice",
		},
		Action:   "sqs:SendMessage",
		Resource: "arn:aws:sqs:us-east-1:000000000001:lab",
	}
	queueAllow := `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":{"AWS":"arn:aws:iam::000000000001:user/alice"},"Action":"sqs:SendMessage","Resource":"*"}]}`
	got := authz.EvaluateSQS(authz.SQSRequest{
		Caller:         ctx,
		IdentityDocs:   nil,
		QueuePolicyDoc: queueAllow,
	})
	if got != authz.Allow {
		t.Fatalf("got %v, want Allow", got)
	}
}
