package catalog_test

import (
	"testing"

	"github.com/Kyaxris-Labs/Noctaxris/internal/catalog"
)

func TestKnownActionPhase4(t *testing.T) {
	known := []string{
		// STS (11)
		catalog.ActionSTSGetCallerIdentity,
		catalog.ActionSTSAssumeRole,
		catalog.ActionSTSGetSessionToken,
		catalog.ActionSTSGetFederationToken,
		catalog.ActionSTSAssumeRoleWithSAML,
		catalog.ActionSTSAssumeRoleWithWebIdentity,
		catalog.ActionSTSAssumeRoot,
		catalog.ActionSTSDecodeAuthorizationMessage,
		catalog.ActionSTSGetAccessKeyInfo,
		catalog.ActionSTSGetDelegatedAccessToken,
		catalog.ActionSTSGetWebIdentityToken,
		// Organizations (Phase 2)
		catalog.ActionOrgsCreateAccount,
		catalog.ActionOrgsDescribeCreateAccountStatus,
		// IAM lab
		catalog.ActionIAMCreateUser,
		catalog.ActionIAMGetUser,
		catalog.ActionIAMListUsers,
		catalog.ActionIAMDeleteUser,
		catalog.ActionIAMCreateAccessKey,
		catalog.ActionIAMDeleteAccessKey,
		catalog.ActionIAMListAccessKeys,
		catalog.ActionIAMUpdateAccessKey,
		catalog.ActionIAMCreatePolicy,
		catalog.ActionIAMGetPolicy,
		catalog.ActionIAMListPolicies,
		catalog.ActionIAMDeletePolicy,
		catalog.ActionIAMAttachUserPolicy,
		catalog.ActionIAMDetachUserPolicy,
		catalog.ActionIAMAttachRolePolicy,
		catalog.ActionIAMDetachRolePolicy,
		catalog.ActionIAMListAttachedUserPolicies,
		catalog.ActionIAMListAttachedRolePolicies,
		catalog.ActionIAMPutUserPolicy,
		catalog.ActionIAMGetUserPolicy,
		catalog.ActionIAMDeleteUserPolicy,
		catalog.ActionIAMListUserPolicies,
		catalog.ActionIAMPutRolePolicy,
		catalog.ActionIAMGetRolePolicy,
		catalog.ActionIAMDeleteRolePolicy,
		catalog.ActionIAMListRolePolicies,
		catalog.ActionIAMCreateRole,
		catalog.ActionIAMGetRole,
		catalog.ActionIAMListRoles,
		catalog.ActionIAMDeleteRole,
		catalog.ActionIAMUpdateAssumeRolePolicy,
		// KMS lab
		catalog.ActionKMSCreateKey,
		catalog.ActionKMSDescribeKey,
		catalog.ActionKMSListKeys,
		catalog.ActionKMSEnableKey,
		catalog.ActionKMSDisableKey,
		catalog.ActionKMSGetKeyPolicy,
		catalog.ActionKMSPutKeyPolicy,
		catalog.ActionKMSEncrypt,
		catalog.ActionKMSDecrypt,
		catalog.ActionKMSGenerateDataKey,
		catalog.ActionKMSGenerateDataKeyWithoutPlaintext,
		catalog.ActionKMSCreateGrant,
		catalog.ActionKMSListGrants,
		catalog.ActionKMSRetireGrant,
		catalog.ActionKMSRevokeGrant,
		catalog.ActionKMSCreateAlias,
		catalog.ActionKMSListAliases,
		catalog.ActionKMSDeleteAlias,
		catalog.ActionKMSUpdateAlias,
		// S3 lab
		catalog.ActionS3CreateBucket,
		catalog.ActionS3DeleteBucket,
		catalog.ActionS3ListAllMyBuckets,
		catalog.ActionS3ListBucket,
		catalog.ActionS3GetBucketPolicy,
		catalog.ActionS3PutBucketPolicy,
		catalog.ActionS3DeleteBucketPolicy,
		catalog.ActionS3GetObject,
		catalog.ActionS3PutObject,
		catalog.ActionS3DeleteObject,
	}
	for _, action := range known {
		if !catalog.KnownAction(action) {
			t.Fatalf("KnownAction(%q) = false, want true", action)
		}
	}
}

func TestKnownActionUnknown(t *testing.T) {
	cases := []string{
		"",
		"s3:GetBucketAcl",
		"GetCallerIdentity",
		"sts:getcalleridentity",
		"organizations:ListAccounts",
		"iam:CreateGroup",
		"sts:UnknownAction",
		"kms:ReEncrypt",
	}
	for _, action := range cases {
		if catalog.KnownAction(action) {
			t.Fatalf("KnownAction(%q) = true, want false", action)
		}
	}
}

func TestActionConstants(t *testing.T) {
	cases := map[string]string{
		catalog.ActionSTSGetCallerIdentity:               "sts:GetCallerIdentity",
		catalog.ActionSTSAssumeRole:                      "sts:AssumeRole",
		catalog.ActionSTSGetSessionToken:                 "sts:GetSessionToken",
		catalog.ActionSTSGetFederationToken:              "sts:GetFederationToken",
		catalog.ActionSTSAssumeRoleWithSAML:              "sts:AssumeRoleWithSAML",
		catalog.ActionSTSAssumeRoleWithWebIdentity:       "sts:AssumeRoleWithWebIdentity",
		catalog.ActionSTSAssumeRoot:                      "sts:AssumeRoot",
		catalog.ActionSTSDecodeAuthorizationMessage:      "sts:DecodeAuthorizationMessage",
		catalog.ActionSTSGetAccessKeyInfo:                "sts:GetAccessKeyInfo",
		catalog.ActionSTSGetDelegatedAccessToken:         "sts:GetDelegatedAccessToken",
		catalog.ActionSTSGetWebIdentityToken:             "sts:GetWebIdentityToken",
		catalog.ActionOrgsCreateAccount:                  "organizations:CreateAccount",
		catalog.ActionOrgsDescribeCreateAccountStatus:    "organizations:DescribeCreateAccountStatus",
		catalog.ActionIAMCreateUser:                      "iam:CreateUser",
		catalog.ActionIAMGetUser:                         "iam:GetUser",
		catalog.ActionIAMListUsers:                       "iam:ListUsers",
		catalog.ActionIAMDeleteUser:                      "iam:DeleteUser",
		catalog.ActionIAMCreateAccessKey:                 "iam:CreateAccessKey",
		catalog.ActionIAMDeleteAccessKey:                 "iam:DeleteAccessKey",
		catalog.ActionIAMListAccessKeys:                  "iam:ListAccessKeys",
		catalog.ActionIAMUpdateAccessKey:                 "iam:UpdateAccessKey",
		catalog.ActionIAMCreatePolicy:                    "iam:CreatePolicy",
		catalog.ActionIAMGetPolicy:                       "iam:GetPolicy",
		catalog.ActionIAMListPolicies:                    "iam:ListPolicies",
		catalog.ActionIAMDeletePolicy:                    "iam:DeletePolicy",
		catalog.ActionIAMAttachUserPolicy:                "iam:AttachUserPolicy",
		catalog.ActionIAMDetachUserPolicy:                "iam:DetachUserPolicy",
		catalog.ActionIAMAttachRolePolicy:                "iam:AttachRolePolicy",
		catalog.ActionIAMDetachRolePolicy:                "iam:DetachRolePolicy",
		catalog.ActionIAMListAttachedUserPolicies:        "iam:ListAttachedUserPolicies",
		catalog.ActionIAMListAttachedRolePolicies:        "iam:ListAttachedRolePolicies",
		catalog.ActionIAMPutUserPolicy:                   "iam:PutUserPolicy",
		catalog.ActionIAMGetUserPolicy:                   "iam:GetUserPolicy",
		catalog.ActionIAMDeleteUserPolicy:                "iam:DeleteUserPolicy",
		catalog.ActionIAMListUserPolicies:                "iam:ListUserPolicies",
		catalog.ActionIAMPutRolePolicy:                   "iam:PutRolePolicy",
		catalog.ActionIAMGetRolePolicy:                   "iam:GetRolePolicy",
		catalog.ActionIAMDeleteRolePolicy:                "iam:DeleteRolePolicy",
		catalog.ActionIAMListRolePolicies:                "iam:ListRolePolicies",
		catalog.ActionIAMCreateRole:                      "iam:CreateRole",
		catalog.ActionIAMGetRole:                         "iam:GetRole",
		catalog.ActionIAMListRoles:                       "iam:ListRoles",
		catalog.ActionIAMDeleteRole:                      "iam:DeleteRole",
		catalog.ActionIAMUpdateAssumeRolePolicy:          "iam:UpdateAssumeRolePolicy",
		catalog.ActionKMSCreateKey:                       "kms:CreateKey",
		catalog.ActionKMSDescribeKey:                     "kms:DescribeKey",
		catalog.ActionKMSListKeys:                        "kms:ListKeys",
		catalog.ActionKMSEnableKey:                       "kms:EnableKey",
		catalog.ActionKMSDisableKey:                      "kms:DisableKey",
		catalog.ActionKMSGetKeyPolicy:                    "kms:GetKeyPolicy",
		catalog.ActionKMSPutKeyPolicy:                    "kms:PutKeyPolicy",
		catalog.ActionKMSEncrypt:                         "kms:Encrypt",
		catalog.ActionKMSDecrypt:                         "kms:Decrypt",
		catalog.ActionKMSGenerateDataKey:                 "kms:GenerateDataKey",
		catalog.ActionKMSGenerateDataKeyWithoutPlaintext: "kms:GenerateDataKeyWithoutPlaintext",
		catalog.ActionKMSCreateGrant:                     "kms:CreateGrant",
		catalog.ActionKMSListGrants:                      "kms:ListGrants",
		catalog.ActionKMSRetireGrant:                     "kms:RetireGrant",
		catalog.ActionKMSRevokeGrant:                     "kms:RevokeGrant",
		catalog.ActionKMSCreateAlias:                     "kms:CreateAlias",
		catalog.ActionKMSListAliases:                     "kms:ListAliases",
		catalog.ActionKMSDeleteAlias:                     "kms:DeleteAlias",
		catalog.ActionKMSUpdateAlias:                     "kms:UpdateAlias",
		catalog.ActionS3CreateBucket:                     "s3:CreateBucket",
		catalog.ActionS3DeleteBucket:                     "s3:DeleteBucket",
		catalog.ActionS3ListAllMyBuckets:                 "s3:ListAllMyBuckets",
		catalog.ActionS3ListBucket:                       "s3:ListBucket",
		catalog.ActionS3GetBucketPolicy:                  "s3:GetBucketPolicy",
		catalog.ActionS3PutBucketPolicy:                  "s3:PutBucketPolicy",
		catalog.ActionS3DeleteBucketPolicy:               "s3:DeleteBucketPolicy",
		catalog.ActionS3GetObject:                        "s3:GetObject",
		catalog.ActionS3PutObject:                        "s3:PutObject",
		catalog.ActionS3DeleteObject:                     "s3:DeleteObject",
	}
	for got, want := range cases {
		if got != want {
			t.Fatalf("constant = %q, want %q", got, want)
		}
	}
}
