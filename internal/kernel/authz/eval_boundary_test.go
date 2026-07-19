package authz_test

import (
	"testing"

	"github.com/Kyaxris-Labs/Noctaxris/internal/kernel/authz"
	"github.com/Kyaxris-Labs/Noctaxris/internal/kernel/identity"
)

func userCtx(action string) authz.RequestContext {
	return authz.RequestContext{
		Principal: identity.Principal{
			Kind:      identity.KindUser,
			AccountID: "000000000002",
			UserName:  "alice",
			IsRoot:    false,
		},
		Action:   action,
		Resource: "*",
	}
}

func TestBoundaryIntersectionRequiresBothAllows(t *testing.T) {
	ctx := userCtx("sts:GetCallerIdentity")
	identityAllow := `{
		"Version":"2012-10-17",
		"Statement":[{"Effect":"Allow","Action":"sts:GetCallerIdentity","Resource":"*"}]
	}`
	boundaryAllow := `{
		"Version":"2012-10-17",
		"Statement":[{"Effect":"Allow","Action":"sts:*","Resource":"*"}]
	}`
	boundaryDenyAction := `{
		"Version":"2012-10-17",
		"Statement":[{"Effect":"Allow","Action":"s3:*","Resource":"*"}]
	}`

	if got := authz.EvaluateFull(ctx, authz.EvalInputs{
		IdentityDocs: []string{identityAllow},
		BoundaryDoc:  boundaryAllow,
	}); got != authz.Allow {
		t.Fatalf("identity Allow + boundary Allow got %v, want Allow", got)
	}

	if got := authz.EvaluateFull(ctx, authz.EvalInputs{
		IdentityDocs: []string{identityAllow},
		BoundaryDoc:  boundaryDenyAction,
	}); got != authz.Deny {
		t.Fatalf("identity Allow + boundary no-match got %v, want Deny", got)
	}

	if got := authz.EvaluateFull(ctx, authz.EvalInputs{
		IdentityDocs: nil,
		BoundaryDoc:  boundaryAllow,
	}); got != authz.Deny {
		t.Fatalf("boundary alone got %v, want Deny", got)
	}
}

func TestEmptyBoundaryDocSkipsBoundaryCheck(t *testing.T) {
	ctx := userCtx("sts:GetCallerIdentity")
	identityAllow := `{
		"Version":"2012-10-17",
		"Statement":[{"Effect":"Allow","Action":"sts:GetCallerIdentity","Resource":"*"}]
	}`
	if got := authz.EvaluateFull(ctx, authz.EvalInputs{
		IdentityDocs: []string{identityAllow},
		BoundaryDoc:  "",
	}); got != authz.Allow {
		t.Fatalf("empty BoundaryDoc got %v, want Allow", got)
	}
}

func TestRootSkipsRestrictiveBoundary(t *testing.T) {
	ctx := authz.RequestContext{
		Principal: identity.RootPrincipal("000000000001", "AKIAROOT"),
		Action:    "sts:GetCallerIdentity",
		Resource:  "*",
	}
	identityAllow := `{
		"Version":"2012-10-17",
		"Statement":[{"Effect":"Allow","Action":"sts:GetCallerIdentity","Resource":"*"}]
	}`
	boundaryDenyAction := `{
		"Version":"2012-10-17",
		"Statement":[{"Effect":"Allow","Action":"s3:*","Resource":"*"}]
	}`
	if got := authz.EvaluateFull(ctx, authz.EvalInputs{
		IdentityDocs: []string{identityAllow},
		BoundaryDoc:  boundaryDenyAction,
	}); got != authz.Allow {
		t.Fatalf("root + restrictive boundary got %v, want Allow", got)
	}
}

func TestBoundaryExplicitDenyOverrides(t *testing.T) {
	ctx := userCtx("sts:GetCallerIdentity")
	identityAllow := `{
		"Version":"2012-10-17",
		"Statement":[{"Effect":"Allow","Action":"sts:*","Resource":"*"}]
	}`
	boundaryDeny := `{
		"Version":"2012-10-17",
		"Statement":[
			{"Effect":"Allow","Action":"*","Resource":"*"},
			{"Effect":"Deny","Action":"sts:GetCallerIdentity","Resource":"*"}
		]
	}`
	if got := authz.EvaluateFull(ctx, authz.EvalInputs{
		IdentityDocs: []string{identityAllow},
		BoundaryDoc:  boundaryDeny,
	}); got != authz.Deny {
		t.Fatalf("boundary explicit Deny got %v, want Deny", got)
	}
}
