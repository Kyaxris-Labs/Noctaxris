package authz_test

import (
	"testing"

	"github.com/Kyaxris-Labs/Noctaxris/internal/kernel/authz"
	"github.com/Kyaxris-Labs/Noctaxris/internal/kernel/identity"
)

func TestEvaluateS3IdentityAllowEmptyBucket(t *testing.T) {
	ctx := authz.RequestContext{
		Principal: identity.Principal{
			Kind:      identity.KindUser,
			AccountID: "000000000001",
			UserName:  "alice",
		},
		Action:   "s3:GetObject",
		Resource: "arn:aws:s3:::lab/a.txt",
	}
	identityAllow := `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":{"AWS":"arn:aws:iam::000000000001:user/alice"},"Action":"s3:GetObject","Resource":"*"}]}`
	got := authz.EvaluateS3(authz.S3Request{
		Caller:          ctx,
		IdentityDocs:    []string{identityAllow},
		BucketPolicyDoc: "",
	})
	if got != authz.Allow {
		t.Fatalf("got %v, want Allow", got)
	}
}

func TestEvaluateS3IdentityDenyBucketAllow(t *testing.T) {
	ctx := authz.RequestContext{
		Principal: identity.Principal{
			Kind:      identity.KindUser,
			AccountID: "000000000001",
			UserName:  "alice",
		},
		Action:   "s3:GetObject",
		Resource: "arn:aws:s3:::lab/a.txt",
	}
	identityDeny := `{"Version":"2012-10-17","Statement":[{"Effect":"Deny","Action":"s3:GetObject","Resource":"*"}]}`
	bucketAllow := `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":{"AWS":"arn:aws:iam::000000000001:user/alice"},"Action":"s3:GetObject","Resource":"*"}]}`
	got := authz.EvaluateS3(authz.S3Request{
		Caller:          ctx,
		IdentityDocs:    []string{identityDeny},
		BucketPolicyDoc: bucketAllow,
	})
	if got != authz.Deny {
		t.Fatalf("got %v, want Deny", got)
	}
}

func TestEvaluateS3IdentityEmptyBucketAllow(t *testing.T) {
	ctx := authz.RequestContext{
		Principal: identity.Principal{
			Kind:      identity.KindUser,
			AccountID: "000000000001",
			UserName:  "alice",
		},
		Action:   "s3:GetObject",
		Resource: "arn:aws:s3:::lab/a.txt",
	}
	bucketAllow := `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":{"AWS":"arn:aws:iam::000000000001:user/alice"},"Action":"s3:GetObject","Resource":"*"}]}`
	got := authz.EvaluateS3(authz.S3Request{
		Caller:          ctx,
		IdentityDocs:    nil,
		BucketPolicyDoc: bucketAllow,
	})
	if got != authz.Allow {
		t.Fatalf("got %v, want Allow", got)
	}
}
