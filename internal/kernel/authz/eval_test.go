package authz_test

import (
	"testing"

	"github.com/Kyaxris-Labs/Noctaxris/internal/kernel/authz"
	"github.com/Kyaxris-Labs/Noctaxris/internal/kernel/identity"
)

func TestRootAllows(t *testing.T) {
	ctx := authz.RequestContext{
		Principal: identity.RootPrincipal("000000000001", "AKIAROOT"),
		Action:    "sts:GetCallerIdentity",
		Resource:  "*",
	}
	if got := authz.Evaluate(ctx, nil); got != authz.Allow {
		t.Fatalf("root got %v, want Allow", got)
	}
	denyDoc := `{"Version":"2012-10-17","Statement":[{"Effect":"Deny","Action":"*","Resource":"*"}]}`
	if got := authz.Evaluate(ctx, []string{denyDoc}); got != authz.Allow {
		t.Fatalf("root with deny policy got %v, want Allow", got)
	}
}

func TestAllowWhenIdentityAllows(t *testing.T) {
	ctx := authz.RequestContext{
		Principal: identity.Principal{
			Kind:      identity.KindUser,
			AccountID: "000000000001",
			IsRoot:    false,
		},
		Action:   "sts:GetCallerIdentity",
		Resource: "*",
		ConditionKeys: map[string]string{
			"aws:PrincipalAccount": "000000000001",
			"aws:RequestedRegion":  "us-east-1",
		},
	}
	allowDoc := `{
		"Version":"2012-10-17",
		"Statement":[{
			"Effect":"Allow",
			"Action":"sts:GetCallerIdentity",
			"Resource":"*"
		}]
	}`
	if got := authz.Evaluate(ctx, []string{allowDoc}); got != authz.Allow {
		t.Fatalf("got %v, want Allow", got)
	}

	prefixDoc := `{
		"Version":"2012-10-17",
		"Statement":[{
			"Effect":"Allow",
			"Action":"sts:*",
			"Resource":"*"
		}]
	}`
	if got := authz.Evaluate(ctx, []string{prefixDoc}); got != authz.Allow {
		t.Fatalf("prefix wildcard got %v, want Allow", got)
	}
}

func TestImplicitDenyWhenNoAllow(t *testing.T) {
	ctx := authz.RequestContext{
		Principal: identity.Principal{
			Kind:      identity.KindUser,
			AccountID: "000000000001",
			IsRoot:    false,
		},
		Action:   "sts:GetCallerIdentity",
		Resource: "*",
	}
	if got := authz.Evaluate(ctx, nil); got != authz.Deny {
		t.Fatalf("empty policies got %v, want Deny", got)
	}
	unrelated := `{
		"Version":"2012-10-17",
		"Statement":[{
			"Effect":"Allow",
			"Action":"s3:GetObject",
			"Resource":"*"
		}]
	}`
	if got := authz.Evaluate(ctx, []string{unrelated}); got != authz.Deny {
		t.Fatalf("unrelated allow got %v, want Deny", got)
	}
}

func TestExplicitDenyOverridesAllow(t *testing.T) {
	ctx := authz.RequestContext{
		Principal: identity.Principal{
			Kind:      identity.KindUser,
			AccountID: "000000000001",
			IsRoot:    false,
		},
		Action:   "sts:GetCallerIdentity",
		Resource: "*",
		ConditionKeys: map[string]string{
			"aws:PrincipalAccount": "000000000001",
			"aws:RequestedRegion":  "us-east-1",
		},
	}
	allowDoc := `{
		"Version":"2012-10-17",
		"Statement":[{
			"Effect":"Allow",
			"Action":"sts:*",
			"Resource":"*"
		}]
	}`
	denyDoc := `{
		"Version":"2012-10-17",
		"Statement":[{
			"Effect":"Deny",
			"Action":"sts:GetCallerIdentity",
			"Resource":"*"
		}]
	}`
	if got := authz.Evaluate(ctx, []string{allowDoc, denyDoc}); got != authz.Deny {
		t.Fatalf("got %v, want Deny", got)
	}
}

func TestEvaluateWithSessionIntersection(t *testing.T) {
	ctx := authz.RequestContext{
		Principal: identity.Principal{
			Kind:      identity.KindUser,
			AccountID: "000000000001",
			IsRoot:    false,
		},
		Action:   "sts:GetCallerIdentity",
		Resource: "*",
	}
	allowDoc := `{
		"Version":"2012-10-17",
		"Statement":[{
			"Effect":"Allow",
			"Action":"sts:GetCallerIdentity",
			"Resource":"*"
		}]
	}`
	denyDoc := `{
		"Version":"2012-10-17",
		"Statement":[{
			"Effect":"Deny",
			"Action":"sts:GetCallerIdentity",
			"Resource":"*"
		}]
	}`

	if got := authz.EvaluateWithSession(ctx, []string{allowDoc}, []string{denyDoc}); got != authz.Deny {
		t.Fatalf("identity Allow + session Deny got %v, want Deny", got)
	}
	if got := authz.EvaluateWithSession(ctx, []string{allowDoc}, []string{allowDoc}); got != authz.Allow {
		t.Fatalf("identity Allow + session Allow got %v, want Allow", got)
	}
	if got := authz.EvaluateWithSession(ctx, []string{denyDoc}, []string{allowDoc}); got != authz.Deny {
		t.Fatalf("identity Deny + session Allow got %v, want Deny", got)
	}
}

func TestEvaluateWithSessionEmptySameAsEvaluate(t *testing.T) {
	ctx := authz.RequestContext{
		Principal: identity.Principal{
			Kind:      identity.KindUser,
			AccountID: "000000000001",
			IsRoot:    false,
		},
		Action:   "sts:GetCallerIdentity",
		Resource: "*",
	}
	allowDoc := `{
		"Version":"2012-10-17",
		"Statement":[{
			"Effect":"Allow",
			"Action":"sts:GetCallerIdentity",
			"Resource":"*"
		}]
	}`
	if got := authz.EvaluateWithSession(ctx, []string{allowDoc}, nil); got != authz.Allow {
		t.Fatalf("nil session got %v, want Allow", got)
	}
	if got := authz.EvaluateWithSession(ctx, []string{allowDoc}, []string{}); got != authz.Allow {
		t.Fatalf("empty session got %v, want Allow", got)
	}
	if got := authz.EvaluateWithSession(ctx, nil, nil); got != authz.Deny {
		t.Fatalf("no identity policies got %v, want Deny", got)
	}
}
