package server

import (
	"testing"

	"github.com/Kyaxris-Labs/Noctaxris/internal/catalog"
	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
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

func TestIAMRequestResourceNamedShapes(t *testing.T) {
	s, _ := accessKeyPlumbServer(t)
	accountID := "000000000001"

	cases := []struct {
		action string
		params map[string]string
		want   string
	}{
		{"CreateAccessKey", map[string]string{"UserName": "bob"}, "arn:aws:iam::" + accountID + ":user/bob"},
		{"ListAccessKeys", map[string]string{"UserName": "bob"}, "arn:aws:iam::" + accountID + ":user/bob"},
		{"GetUserPolicy", map[string]string{"UserName": "bob"}, "arn:aws:iam::" + accountID + ":user/bob"},
		{"PutUserPermissionsBoundary", map[string]string{"UserName": "bob"}, "arn:aws:iam::" + accountID + ":user/bob"},
		{"ListMFADevices", map[string]string{"UserName": "bob"}, "arn:aws:iam::" + accountID + ":user/bob"},
		{"AttachRolePolicy", map[string]string{"RoleName": "R"}, "arn:aws:iam::" + accountID + ":role/R"},
		{"PutRolePolicy", map[string]string{"RoleName": "R"}, "arn:aws:iam::" + accountID + ":role/R"},
		{"UpdateAssumeRolePolicy", map[string]string{"RoleName": "R"}, "arn:aws:iam::" + accountID + ":role/R"},
		{"ListInstanceProfilesForRole", map[string]string{"RoleName": "R"}, "arn:aws:iam::" + accountID + ":role/R"},
		{"AddUserToGroup", map[string]string{"GroupName": "G"}, "arn:aws:iam::" + accountID + ":group/G"},
		{"PutGroupPolicy", map[string]string{"GroupName": "G"}, "arn:aws:iam::" + accountID + ":group/G"},
		{"ListGroupPolicies", map[string]string{"GroupName": "G"}, "arn:aws:iam::" + accountID + ":group/G"},
		{"CreatePolicy", map[string]string{"PolicyName": "P"}, "arn:aws:iam::" + accountID + ":policy/P"},
		{"GetPolicy", map[string]string{"PolicyArn": "arn:aws:iam::" + accountID + ":policy/P"}, "arn:aws:iam::" + accountID + ":policy/P"},
		{"CreatePolicyVersion", map[string]string{"PolicyArn": "arn:aws:iam::" + accountID + ":policy/P"}, "arn:aws:iam::" + accountID + ":policy/P"},
		{"CreateInstanceProfile", map[string]string{"InstanceProfileName": "IP"}, "arn:aws:iam::" + accountID + ":instance-profile/IP"},
		{"GetInstanceProfile", map[string]string{"InstanceProfileName": "IP"}, "arn:aws:iam::" + accountID + ":instance-profile/IP"},
		{"CreateOpenIDConnectProvider", map[string]string{"Url": "https://accounts.google.com"}, store.OIDCProviderARN(accountID, "https://accounts.google.com")},
		{"GetOpenIDConnectProvider", map[string]string{"OpenIDConnectProviderArn": "arn:aws:iam::" + accountID + ":oidc-provider/x"}, "arn:aws:iam::" + accountID + ":oidc-provider/x"},
		{"CreateSAMLProvider", map[string]string{"Name": "IdP"}, store.SAMLProviderARN(accountID, "IdP")},
		{"GetSAMLProvider", map[string]string{"SAMLProviderArn": "arn:aws:iam::" + accountID + ":saml-provider/IdP"}, "arn:aws:iam::" + accountID + ":saml-provider/IdP"},
		{"EnableMFADevice", map[string]string{"SerialNumber": "arn:aws:iam::" + accountID + ":mfa/device"}, "arn:aws:iam::" + accountID + ":mfa/device"},
		{"EnableMFADevice", map[string]string{"SerialNumber": "device"}, store.MFADeviceARN(accountID, "device")},
		{"DeactivateMFADevice", map[string]string{"UserName": "bob"}, "arn:aws:iam::" + accountID + ":user/bob"},
		{"DeleteAccessKey", map[string]string{"AccessKeyId": "AKIATEST"}, "arn:aws:iam::" + accountID + ":access-key/AKIATEST"},
		{"UnknownAction", map[string]string{}, "*"},
	}
	for _, tc := range cases {
		got := s.iamRequestResource(accountID, tc.action, tc.params, nil)
		if got != tc.want {
			t.Fatalf("%s params=%v got %q want %q", tc.action, tc.params, got, tc.want)
		}
	}
	if !accessKeyUserNameMismatch("alice", store.AccessKey{UserName: "bob"}) {
		t.Fatal("mismatch")
	}
	if accessKeyUserNameMismatch("", store.AccessKey{UserName: "bob"}) {
		t.Fatal("empty request name")
	}
	if accessKeyUserNameMismatch("bob", store.AccessKey{UserName: "bob"}) {
		t.Fatal("match")
	}
}
