package authz_test

import (
	"testing"

	"github.com/Kyaxris-Labs/Noctaxris/internal/kernel/authz"
	"github.com/Kyaxris-Labs/Noctaxris/internal/kernel/identity"
)

func TestEvaluateSNSIdentityAllowEmptyTopic(t *testing.T) {
	ctx := authz.RequestContext{
		Principal: identity.Principal{
			Kind:      identity.KindUser,
			AccountID: "000000000001",
			UserName:  "alice",
		},
		Action:   "sns:Publish",
		Resource: "arn:aws:sns:us-east-1:000000000001:lab",
	}
	identityAllow := `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":{"AWS":"arn:aws:iam::000000000001:user/alice"},"Action":"sns:Publish","Resource":"*"}]}`
	got := authz.EvaluateSNS(authz.SNSRequest{
		Caller:         ctx,
		IdentityDocs:   []string{identityAllow},
		TopicPolicyDoc: "",
	})
	if got != authz.Allow {
		t.Fatalf("got %v, want Allow", got)
	}
}

func TestEvaluateSNSIdentityDenyTopicAllow(t *testing.T) {
	ctx := authz.RequestContext{
		Principal: identity.Principal{
			Kind:      identity.KindUser,
			AccountID: "000000000001",
			UserName:  "alice",
		},
		Action:   "sns:Publish",
		Resource: "arn:aws:sns:us-east-1:000000000001:lab",
	}
	identityDeny := `{"Version":"2012-10-17","Statement":[{"Effect":"Deny","Action":"sns:Publish","Resource":"*"}]}`
	topicAllow := `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":{"AWS":"arn:aws:iam::000000000001:user/alice"},"Action":"sns:Publish","Resource":"*"}]}`
	got := authz.EvaluateSNS(authz.SNSRequest{
		Caller:         ctx,
		IdentityDocs:   []string{identityDeny},
		TopicPolicyDoc: topicAllow,
	})
	if got != authz.Deny {
		t.Fatalf("got %v, want Deny", got)
	}
}

func TestEvaluateSNSIdentityEmptyTopicAllow(t *testing.T) {
	ctx := authz.RequestContext{
		Principal: identity.Principal{
			Kind:      identity.KindUser,
			AccountID: "000000000001",
			UserName:  "alice",
		},
		Action:   "sns:Publish",
		Resource: "arn:aws:sns:us-east-1:000000000001:lab",
	}
	topicAllow := `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":{"AWS":"arn:aws:iam::000000000001:user/alice"},"Action":"sns:Publish","Resource":"*"}]}`
	got := authz.EvaluateSNS(authz.SNSRequest{
		Caller:         ctx,
		IdentityDocs:   nil,
		TopicPolicyDoc: topicAllow,
	})
	if got != authz.Allow {
		t.Fatalf("got %v, want Allow", got)
	}
}
