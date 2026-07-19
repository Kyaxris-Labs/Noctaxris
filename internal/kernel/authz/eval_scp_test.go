package authz_test

import (
	"testing"

	"github.com/Kyaxris-Labs/Noctaxris/internal/kernel/authz"
	"github.com/Kyaxris-Labs/Noctaxris/internal/kernel/identity"
)

func TestSCPMemberRequiresAllowMatch(t *testing.T) {
	ctx := userCtx("sts:GetCallerIdentity")
	identityAllow := `{
		"Version":"2012-10-17",
		"Statement":[{"Effect":"Allow","Action":"sts:GetCallerIdentity","Resource":"*"}]
	}`
	scpFull := `{
		"Version":"2012-10-17",
		"Statement":[{"Effect":"Allow","Action":"*","Resource":"*"}]
	}`
	scpNoSTS := `{
		"Version":"2012-10-17",
		"Statement":[{"Effect":"Allow","Action":"s3:*","Resource":"*"}]
	}`

	if got := authz.EvaluateFull(ctx, authz.EvalInputs{
		IdentityDocs:        []string{identityAllow},
		SCPDocs:             []string{scpFull},
		IsManagementAccount: false,
	}); got != authz.Allow {
		t.Fatalf("member + FullAWSAccess SCP got %v, want Allow", got)
	}

	if got := authz.EvaluateFull(ctx, authz.EvalInputs{
		IdentityDocs:        []string{identityAllow},
		SCPDocs:             []string{scpNoSTS},
		IsManagementAccount: false,
	}); got != authz.Deny {
		t.Fatalf("member SCP without Allow match got %v, want Deny", got)
	}
}

func TestSCPEveryDocMustAllow(t *testing.T) {
	ctx := userCtx("sts:GetCallerIdentity")
	identityAllow := `{
		"Version":"2012-10-17",
		"Statement":[{"Effect":"Allow","Action":"sts:*","Resource":"*"}]
	}`
	scpFull := `{
		"Version":"2012-10-17",
		"Statement":[{"Effect":"Allow","Action":"*","Resource":"*"}]
	}`
	scpRestrict := `{
		"Version":"2012-10-17",
		"Statement":[{"Effect":"Allow","Action":"s3:*","Resource":"*"}]
	}`
	if got := authz.EvaluateFull(ctx, authz.EvalInputs{
		IdentityDocs:        []string{identityAllow},
		SCPDocs:             []string{scpFull, scpRestrict},
		IsManagementAccount: false,
	}); got != authz.Deny {
		t.Fatalf("AND of SCPs without Allow in every doc got %v, want Deny", got)
	}
}

func TestManagementAccountExemptFromSCP(t *testing.T) {
	ctx := userCtx("sts:GetCallerIdentity")
	ctx.Principal.AccountID = "000000000001"
	identityAllow := `{
		"Version":"2012-10-17",
		"Statement":[{"Effect":"Allow","Action":"sts:GetCallerIdentity","Resource":"*"}]
	}`
	scpNoSTS := `{
		"Version":"2012-10-17",
		"Statement":[{"Effect":"Allow","Action":"s3:*","Resource":"*"}]
	}`
	if got := authz.EvaluateFull(ctx, authz.EvalInputs{
		IdentityDocs:        []string{identityAllow},
		SCPDocs:             []string{scpNoSTS},
		IsManagementAccount: true,
	}); got != authz.Allow {
		t.Fatalf("management SCP exempt got %v, want Allow", got)
	}
}

func TestRCPRequiresAllowWhenPresent(t *testing.T) {
	ctx := userCtx("s3:GetObject")
	identityAllow := `{
		"Version":"2012-10-17",
		"Statement":[{"Effect":"Allow","Action":"s3:GetObject","Resource":"*"}]
	}`
	rcpFull := `{
		"Version":"2012-10-17",
		"Statement":[{"Effect":"Allow","Action":"*","Resource":"*"}]
	}`
	rcpNoS3 := `{
		"Version":"2012-10-17",
		"Statement":[{"Effect":"Allow","Action":"kms:*","Resource":"*"}]
	}`

	if got := authz.EvaluateFull(ctx, authz.EvalInputs{
		IdentityDocs: []string{identityAllow},
		RCPDocs:      []string{rcpFull},
	}); got != authz.Allow {
		t.Fatalf("RCP FullAWSAccess got %v, want Allow", got)
	}
	if got := authz.EvaluateFull(ctx, authz.EvalInputs{
		IdentityDocs: []string{identityAllow},
		RCPDocs:      []string{rcpNoS3},
	}); got != authz.Deny {
		t.Fatalf("RCP without Allow match got %v, want Deny", got)
	}
}

func TestEmptySCPAndRCPDocsDoNotRestrict(t *testing.T) {
	ctx := userCtx("sts:GetCallerIdentity")
	identityAllow := `{
		"Version":"2012-10-17",
		"Statement":[{"Effect":"Allow","Action":"sts:GetCallerIdentity","Resource":"*"}]
	}`
	if got := authz.EvaluateFull(ctx, authz.EvalInputs{
		IdentityDocs:        []string{identityAllow},
		SCPDocs:             nil,
		RCPDocs:             nil,
		IsManagementAccount: false,
	}); got != authz.Allow {
		t.Fatalf("empty org docs got %v, want Allow", got)
	}
}

func TestSessionIntersectionAfterIdentityAndBoundary(t *testing.T) {
	ctx := userCtx("sts:GetCallerIdentity")
	identityAllow := `{
		"Version":"2012-10-17",
		"Statement":[{"Effect":"Allow","Action":"sts:*","Resource":"*"}]
	}`
	boundaryAllow := `{
		"Version":"2012-10-17",
		"Statement":[{"Effect":"Allow","Action":"*","Resource":"*"}]
	}`
	sessionDeny := `{
		"Version":"2012-10-17",
		"Statement":[{"Effect":"Deny","Action":"sts:GetCallerIdentity","Resource":"*"}]
	}`
	sessionAllow := `{
		"Version":"2012-10-17",
		"Statement":[{"Effect":"Allow","Action":"sts:GetCallerIdentity","Resource":"*"}]
	}`

	if got := authz.EvaluateFull(ctx, authz.EvalInputs{
		IdentityDocs: []string{identityAllow},
		BoundaryDoc:  boundaryAllow,
		SessionDocs:  []string{sessionDeny},
	}); got != authz.Deny {
		t.Fatalf("session Deny got %v, want Deny", got)
	}
	if got := authz.EvaluateFull(ctx, authz.EvalInputs{
		IdentityDocs: []string{identityAllow},
		BoundaryDoc:  boundaryAllow,
		SessionDocs:  []string{sessionAllow},
	}); got != authz.Allow {
		t.Fatalf("session Allow got %v, want Allow", got)
	}
}

func TestRootStillAllowsViaEvaluateFullIdentity(t *testing.T) {
	ctx := authz.RequestContext{
		Principal: identity.RootPrincipal("000000000001", "AKIAROOT"),
		Action:    "sts:GetCallerIdentity",
		Resource:  "*",
	}
	if got := authz.EvaluateFull(ctx, authz.EvalInputs{}); got != authz.Allow {
		t.Fatalf("root EvaluateFull got %v, want Allow", got)
	}
}
