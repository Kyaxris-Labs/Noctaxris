package authz_test

import (
	"testing"

	"github.com/Kyaxris-Labs/Noctaxris/internal/kernel/authz"
)

func TestEvaluateAnonymousS3GetObjectPolicyAllow(t *testing.T) {
	policy := `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":"*","Action":"s3:GetObject","Resource":"arn:aws:s3:::pub/*"}]}`
	got := authz.EvaluateAnonymousS3GetObject(authz.AnonymousS3GetRequest{
		Resource:          "arn:aws:s3:::pub/a.txt",
		ResourceAccountID: "000000000001",
		BucketPolicyDoc:   policy,
	})
	if got != authz.Allow {
		t.Fatalf("Principal * Allow got %v, want Allow", got)
	}
}

func TestEvaluateAnonymousS3GetObjectAWSStarAllow(t *testing.T) {
	policy := `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":{"AWS":"*"},"Action":"s3:GetObject","Resource":"*"}]}`
	got := authz.EvaluateAnonymousS3GetObject(authz.AnonymousS3GetRequest{
		Resource:          "arn:aws:s3:::pub/a.txt",
		ResourceAccountID: "000000000001",
		BucketPolicyDoc:   policy,
	})
	if got != authz.Allow {
		t.Fatalf("Principal AWS * Allow got %v, want Allow", got)
	}
}

func TestEvaluateAnonymousS3GetObjectEmptyPolicyDeny(t *testing.T) {
	got := authz.EvaluateAnonymousS3GetObject(authz.AnonymousS3GetRequest{
		Resource:          "arn:aws:s3:::priv/a.txt",
		ResourceAccountID: "000000000001",
	})
	if got != authz.Deny {
		t.Fatalf("empty policy got %v, want Deny", got)
	}
}

func TestEvaluateAnonymousS3GetObjectDenyBeatsACL(t *testing.T) {
	policy := `{"Version":"2012-10-17","Statement":[{"Effect":"Deny","Principal":"*","Action":"s3:GetObject","Resource":"*"}]}`
	got := authz.EvaluateAnonymousS3GetObject(authz.AnonymousS3GetRequest{
		Resource:          "arn:aws:s3:::pub/a.txt",
		ResourceAccountID: "000000000001",
		BucketPolicyDoc:   policy,
		ObjectCannedACL:   authz.CannedACLPublicRead,
	})
	if got != authz.Deny {
		t.Fatalf("Deny policy + public-read ACL got %v, want Deny", got)
	}
}

func TestEvaluateAnonymousS3GetObjectPublicReadACL(t *testing.T) {
	got := authz.EvaluateAnonymousS3GetObject(authz.AnonymousS3GetRequest{
		Resource:          "arn:aws:s3:::pub/a.txt",
		ResourceAccountID: "000000000001",
		ObjectCannedACL:   authz.CannedACLPublicRead,
	})
	if got != authz.Allow {
		t.Fatalf("public-read ACL got %v, want Allow", got)
	}
}

func TestEvaluateAnonymousS3GetObjectSpecificPrincipalDoesNotGrantAnonymous(t *testing.T) {
	policy := `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":{"AWS":"arn:aws:iam::000000000001:user/alice"},"Action":"s3:GetObject","Resource":"*"}]}`
	got := authz.EvaluateAnonymousS3GetObject(authz.AnonymousS3GetRequest{
		Resource:          "arn:aws:s3:::pub/a.txt",
		ResourceAccountID: "000000000001",
		BucketPolicyDoc:   policy,
	})
	if got != authz.Deny {
		t.Fatalf("named Principal Allow got %v, want Deny for anonymous", got)
	}
}
