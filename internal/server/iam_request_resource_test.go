package server

import (
	"testing"

	"github.com/Kyaxris-Labs/Noctaxris/internal/catalog"
)

func TestIAMRequestResourceListReportNotBareStar(t *testing.T) {
	s := &Server{}
	accountID := "000000000001"
	cases := []struct {
		action string
		want   string
	}{
		{catalog.ActionIAMListUsers, "arn:aws:iam::" + accountID + ":user/*"},
		{"ListUsers", "arn:aws:iam::" + accountID + ":user/*"},
		{catalog.ActionIAMListRoles, "arn:aws:iam::" + accountID + ":role/*"},
		{"ListRoles", "arn:aws:iam::" + accountID + ":role/*"},
		{catalog.ActionIAMListGroups, "arn:aws:iam::" + accountID + ":group/*"},
		{catalog.ActionIAMListPolicies, "arn:aws:iam::" + accountID + ":policy/*"},
		{catalog.ActionIAMListInstanceProfiles, "arn:aws:iam::" + accountID + ":instance-profile/*"},
		{catalog.ActionIAMListOpenIDConnectProviders, "arn:aws:iam::" + accountID + ":oidc-provider/*"},
		{catalog.ActionIAMListSAMLProviders, "arn:aws:iam::" + accountID + ":saml-provider/*"},
		{catalog.ActionIAMGenerateCredentialReport, "arn:aws:iam::" + accountID + ":root"},
		{catalog.ActionIAMGetCredentialReport, "arn:aws:iam::" + accountID + ":root"},
		{"GetCredentialReport", "arn:aws:iam::" + accountID + ":root"},
	}
	for _, tc := range cases {
		got := s.iamRequestResource(accountID, tc.action, map[string]string{}, nil)
		if got == "*" {
			t.Fatalf("%s authorize resource is bare *; want scoped ARN", tc.action)
		}
		if got != tc.want {
			t.Fatalf("%s resource=%q want %q", tc.action, got, tc.want)
		}
	}
}

func TestIAMRequestResourceMutationKeepsNamedARN(t *testing.T) {
	s := &Server{}
	accountID := "000000000001"
	got := s.iamRequestResource(accountID, catalog.ActionIAMDeleteUser, map[string]string{"UserName": "alice"}, nil)
	if got != "arn:aws:iam::"+accountID+":user/alice" {
		t.Fatalf("DeleteUser resource=%q", got)
	}
	got = s.iamRequestResource(accountID, catalog.ActionIAMDeleteRole, map[string]string{"RoleName": "LabRole"}, nil)
	if got != "arn:aws:iam::"+accountID+":role/LabRole" {
		t.Fatalf("DeleteRole resource=%q", got)
	}
}
