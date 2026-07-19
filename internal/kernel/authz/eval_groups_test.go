package authz_test

import (
	"testing"

	"github.com/Kyaxris-Labs/Noctaxris/internal/kernel/authz"
)

func TestGroupPolicyInIdentityDocsAllows(t *testing.T) {
	ctx := userCtx("s3:GetObject")
	// Store helper folds group attached/inline into IdentityDocs; authz only
	// evaluates the combined set (groups are not separate principals).
	userOnlyUnrelated := `{
		"Version":"2012-10-17",
		"Statement":[{"Effect":"Allow","Action":"sts:GetCallerIdentity","Resource":"*"}]
	}`
	groupAllowS3 := `{
		"Version":"2012-10-17",
		"Statement":[{"Effect":"Allow","Action":"s3:GetObject","Resource":"*"}]
	}`

	if got := authz.EvaluateFull(ctx, authz.EvalInputs{
		IdentityDocs: []string{userOnlyUnrelated},
	}); got != authz.Deny {
		t.Fatalf("user-only unrelated got %v, want Deny", got)
	}

	if got := authz.EvaluateFull(ctx, authz.EvalInputs{
		IdentityDocs: []string{userOnlyUnrelated, groupAllowS3},
	}); got != authz.Allow {
		t.Fatalf("user + group identity docs got %v, want Allow", got)
	}

	// Existing Evaluate path must also treat combined docs as identity.
	if got := authz.Evaluate(ctx, []string{userOnlyUnrelated, groupAllowS3}); got != authz.Allow {
		t.Fatalf("Evaluate with group doc got %v, want Allow", got)
	}
}
