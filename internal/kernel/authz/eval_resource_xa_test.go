package authz_test

import (
	"testing"

	"github.com/Kyaxris-Labs/Noctaxris/internal/kernel/authz"
	"github.com/Kyaxris-Labs/Noctaxris/internal/kernel/identity"
)

func TestEvaluateResourceAccessMatrix(t *testing.T) {
	const (
		callerAccount   = "000000000001"
		resourceAccount = "000000000002"
		action          = "s3:GetObject"
		resource        = "arn:aws:s3:::lab/a.txt"
	)

	identityAllow := `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Action":"s3:GetObject","Resource":"*"}]}`
	identityDeny := `{"Version":"2012-10-17","Statement":[{"Effect":"Deny","Action":"s3:GetObject","Resource":"*"}]}`
	resourceAllow := `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":{"AWS":"arn:aws:iam::000000000001:user/alice"},"Action":"s3:GetObject","Resource":"*"}]}`
	resourceDeny := `{"Version":"2012-10-17","Statement":[{"Effect":"Deny","Principal":{"AWS":"*"},"Action":"s3:GetObject","Resource":"*"}]}`

	caller := authz.RequestContext{
		Principal: identity.Principal{
			Kind:      identity.KindUser,
			AccountID: callerAccount,
			UserName:  "alice",
		},
		Action:   action,
		Resource: resource,
	}

	tests := []struct {
		name                string
		resourceAccountID   string
		identityDocs        []string
		resourcePolicyDoc   string
		want                authz.Decision
	}{
		{
			name:              "same-account identity-only",
			resourceAccountID: callerAccount,
			identityDocs:      []string{identityAllow},
			resourcePolicyDoc: "",
			want:              authz.Allow,
		},
		{
			name:              "same-account policy-only",
			resourceAccountID: "",
			identityDocs:      nil,
			resourcePolicyDoc: resourceAllow,
			want:              authz.Allow,
		},
		{
			name:              "same-account both",
			resourceAccountID: callerAccount,
			identityDocs:      []string{identityAllow},
			resourcePolicyDoc: resourceAllow,
			want:              authz.Allow,
		},
		{
			name:              "same-account identity deny",
			resourceAccountID: callerAccount,
			identityDocs:      []string{identityDeny},
			resourcePolicyDoc: resourceAllow,
			want:              authz.Deny,
		},
		{
			name:              "same-account resource deny",
			resourceAccountID: callerAccount,
			identityDocs:      []string{identityAllow},
			resourcePolicyDoc: resourceDeny,
			want:              authz.Deny,
		},
		{
			name:              "cross-account both allow",
			resourceAccountID: resourceAccount,
			identityDocs:      []string{identityAllow},
			resourcePolicyDoc: resourceAllow,
			want:              authz.Allow,
		},
		{
			name:              "cross-account identity-only",
			resourceAccountID: resourceAccount,
			identityDocs:      []string{identityAllow},
			resourcePolicyDoc: "",
			want:              authz.Deny,
		},
		{
			name:              "cross-account policy-only",
			resourceAccountID: resourceAccount,
			identityDocs:      nil,
			resourcePolicyDoc: resourceAllow,
			want:              authz.Deny,
		},
		{
			name:              "cross-account empty policy",
			resourceAccountID: resourceAccount,
			identityDocs:      []string{identityAllow},
			resourcePolicyDoc: "   ",
			want:              authz.Deny,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := authz.EvaluateResourceAccess(authz.ResourceAccessRequest{
				Caller:              caller,
				IdentityDocs:        tt.identityDocs,
				ResourcePolicyDoc:   tt.resourcePolicyDoc,
				ResourceAccountID:   tt.resourceAccountID,
			})
			if got != tt.want {
				t.Fatalf("got %v, want %v", got, tt.want)
			}
		})
	}
}
