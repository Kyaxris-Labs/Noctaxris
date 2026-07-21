package authz_test

import (
	"testing"

	"github.com/Kyaxris-Labs/Noctaxris/internal/kernel/authz"
	"github.com/Kyaxris-Labs/Noctaxris/internal/kernel/identity"
)

func TestFederatedPrincipalTrustAllow(t *testing.T) {
	providerARN := "arn:aws:iam::111111111111:oidc-provider/token.actions.githubusercontent.com"
	roleARN := "arn:aws:iam::111111111111:role/gha"
	req := authz.CrossAccountRequest{
		Caller: authz.RequestContext{
			Principal: identity.FederatedProviderPrincipal("111111111111", providerARN, "oidc", "ASIA"),
			Action:    "sts:AssumeRoleWithWebIdentity",
			Resource:  roleARN,
		},
		CallerIdentityDocs: []string{`{
			"Version":"2012-10-17",
			"Statement":[{"Effect":"Allow","Action":"sts:AssumeRoleWithWebIdentity","Resource":"*"}]
		}`},
		TrustPolicyDoc: `{
			"Version":"2012-10-17",
			"Statement":[{
				"Effect":"Allow",
				"Principal":{"Federated":"` + providerARN + `"},
				"Action":"sts:AssumeRoleWithWebIdentity"
			}]
		}`,
	}
	if got := authz.EvaluateCrossAccount(req); got != authz.Allow {
		t.Fatalf("Federated principal got %v, want Allow", got)
	}
}

func TestFederatedPrincipalRejectsAccountRootTrust(t *testing.T) {
	providerARN := "arn:aws:iam::111111111111:saml-provider/Corp"
	roleARN := "arn:aws:iam::111111111111:role/saml-role"
	req := authz.CrossAccountRequest{
		Caller: authz.RequestContext{
			Principal: identity.FederatedProviderPrincipal("111111111111", providerARN, "saml", "ASIA"),
			Action:    "sts:AssumeRoleWithSAML",
			Resource:  roleARN,
		},
		CallerIdentityDocs: []string{`{
			"Version":"2012-10-17",
			"Statement":[{"Effect":"Allow","Action":"sts:AssumeRoleWithSAML","Resource":"*"}]
		}`},
		TrustPolicyDoc: `{
			"Version":"2012-10-17",
			"Statement":[{
				"Effect":"Allow",
				"Principal":{"AWS":"arn:aws:iam::111111111111:root"},
				"Action":"sts:AssumeRoleWithSAML"
			}]
		}`,
	}
	if got := authz.EvaluateCrossAccount(req); got != authz.Deny {
		t.Fatalf("AWS root trust for federation got %v, want Deny", got)
	}
}

func TestExternalIdConditionOnTrust(t *testing.T) {
	roleARN := "arn:aws:iam::222222222222:role/vendor"
	caller := identity.UserPrincipal("111111111111", "alice", "AKIAA")
	identityAllow := `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Action":"sts:AssumeRole","Resource":"*"}]}`
	trust := `{
		"Version":"2012-10-17",
		"Statement":[{
			"Effect":"Allow",
			"Principal":{"AWS":"arn:aws:iam::111111111111:root"},
			"Action":"sts:AssumeRole",
			"Condition":{"StringEquals":{"sts:ExternalId":"unique-phrase"}}
		}]
	}`

	denyReq := authz.CrossAccountRequest{
		Caller: authz.RequestContext{
			Principal:     caller,
			Action:        "sts:AssumeRole",
			Resource:      roleARN,
			ConditionKeys: map[string]string{},
		},
		CallerIdentityDocs: []string{identityAllow},
		TrustPolicyDoc:     trust,
	}
	if got := authz.EvaluateCrossAccount(denyReq); got != authz.Deny {
		t.Fatalf("missing ExternalId got %v, want Deny", got)
	}

	allowReq := denyReq
	allowReq.Caller.ConditionKeys = map[string]string{"sts:ExternalId": "unique-phrase"}
	if got := authz.EvaluateCrossAccount(allowReq); got != authz.Allow {
		t.Fatalf("matching ExternalId got %v, want Allow", got)
	}
}
