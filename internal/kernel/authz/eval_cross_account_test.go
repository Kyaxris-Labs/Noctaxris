package authz_test

import (
	"testing"

	"github.com/Kyaxris-Labs/Noctaxris/internal/kernel/authz"
	"github.com/Kyaxris-Labs/Noctaxris/internal/kernel/identity"
)

const (
	mgmtAccountID = "111111111111"
	oaarRoleARN   = "arn:aws:iam::222222222222:role/OrganizationAccountAccessRole"
	mgmtRootARN   = "arn:aws:iam::111111111111:root"
)

func TestCrossAccountBothAllow(t *testing.T) {
	req := authz.CrossAccountRequest{
		Caller: authz.RequestContext{
			Principal: identity.RootPrincipal(mgmtAccountID, "AKIAROOT"),
			Action:    "sts:AssumeRole",
			Resource:  oaarRoleARN,
		},
		TrustPolicyDoc: `{
			"Version":"2012-10-17",
			"Statement":[{
				"Effect":"Allow",
				"Principal":{"AWS":"arn:aws:iam::111111111111:root"},
				"Action":"sts:AssumeRole"
			}]
		}`,
	}
	if got := authz.EvaluateCrossAccount(req); got != authz.Allow {
		t.Fatalf("got %v, want Allow", got)
	}
}

func TestCrossAccountTrustMissingDeny(t *testing.T) {
	req := authz.CrossAccountRequest{
		Caller: authz.RequestContext{
			Principal: identity.RootPrincipal(mgmtAccountID, "AKIAROOT"),
			Action:    "sts:AssumeRole",
			Resource:  oaarRoleARN,
		},
		TrustPolicyDoc: "",
	}
	if got := authz.EvaluateCrossAccount(req); got != authz.Deny {
		t.Fatalf("empty trust got %v, want Deny", got)
	}

	req.TrustPolicyDoc = `{
		"Version":"2012-10-17",
		"Statement":[{
			"Effect":"Allow",
			"Principal":{"AWS":"arn:aws:iam::999999999999:root"},
			"Action":"sts:AssumeRole"
		}]
	}`
	if got := authz.EvaluateCrossAccount(req); got != authz.Deny {
		t.Fatalf("unrelated principal trust got %v, want Deny", got)
	}
}

func TestCrossAccountIdentityExplicitDeny(t *testing.T) {
	req := authz.CrossAccountRequest{
		Caller: authz.RequestContext{
			Principal: identity.Principal{
				Kind:      identity.KindUser,
				AccountID: mgmtAccountID,
				IsRoot:    false,
			},
			Action:   "sts:AssumeRole",
			Resource: oaarRoleARN,
		},
		CallerIdentityDocs: []string{`{
			"Version":"2012-10-17",
			"Statement":[
				{
					"Effect":"Allow",
					"Action":"sts:AssumeRole",
					"Resource":"*"
				},
				{
					"Effect":"Deny",
					"Action":"sts:AssumeRole",
					"Resource":"*"
				}
			]
		}`},
		TrustPolicyDoc: `{
			"Version":"2012-10-17",
			"Statement":[{
				"Effect":"Allow",
				"Principal":{"AWS":"` + mgmtRootARN + `"},
				"Action":"sts:AssumeRole"
			}]
		}`,
	}
	if got := authz.EvaluateCrossAccount(req); got != authz.Deny {
		t.Fatalf("identity explicit deny got %v, want Deny", got)
	}
}

func TestCrossAccountTrustExplicitDeny(t *testing.T) {
	req := authz.CrossAccountRequest{
		Caller: authz.RequestContext{
			Principal: identity.RootPrincipal(mgmtAccountID, "AKIAROOT"),
			Action:    "sts:AssumeRole",
			Resource:  oaarRoleARN,
		},
		TrustPolicyDoc: `{
			"Version":"2012-10-17",
			"Statement":[
				{
					"Effect":"Allow",
					"Principal":{"AWS":"arn:aws:iam::111111111111:root"},
					"Action":"sts:AssumeRole"
				},
				{
					"Effect":"Deny",
					"Principal":{"AWS":"arn:aws:iam::111111111111:root"},
					"Action":"sts:AssumeRole"
				}
			]
		}`,
	}
	if got := authz.EvaluateCrossAccount(req); got != authz.Deny {
		t.Fatalf("trust explicit deny got %v, want Deny", got)
	}
}
