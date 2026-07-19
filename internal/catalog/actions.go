package catalog

// STS actions (all 11 Phase 3 routes).

// ActionSTSGetCallerIdentity is the IAM action for STS GetCallerIdentity.
const ActionSTSGetCallerIdentity = "sts:GetCallerIdentity"

// ActionSTSAssumeRole is the IAM action for STS AssumeRole.
const ActionSTSAssumeRole = "sts:AssumeRole"

// ActionSTSGetSessionToken is the IAM action for STS GetSessionToken.
const ActionSTSGetSessionToken = "sts:GetSessionToken"

// ActionSTSGetFederationToken is the IAM action for STS GetFederationToken.
const ActionSTSGetFederationToken = "sts:GetFederationToken"

// ActionSTSAssumeRoleWithSAML is the IAM action for STS AssumeRoleWithSAML.
const ActionSTSAssumeRoleWithSAML = "sts:AssumeRoleWithSAML"

// ActionSTSAssumeRoleWithWebIdentity is the IAM action for STS AssumeRoleWithWebIdentity.
const ActionSTSAssumeRoleWithWebIdentity = "sts:AssumeRoleWithWebIdentity"

// ActionSTSAssumeRoot is the IAM action for STS AssumeRoot.
const ActionSTSAssumeRoot = "sts:AssumeRoot"

// ActionSTSDecodeAuthorizationMessage is the IAM action for STS DecodeAuthorizationMessage.
const ActionSTSDecodeAuthorizationMessage = "sts:DecodeAuthorizationMessage"

// ActionSTSGetAccessKeyInfo is the IAM action for STS GetAccessKeyInfo.
const ActionSTSGetAccessKeyInfo = "sts:GetAccessKeyInfo"

// ActionSTSGetDelegatedAccessToken is the IAM action for STS GetDelegatedAccessToken.
const ActionSTSGetDelegatedAccessToken = "sts:GetDelegatedAccessToken"

// ActionSTSGetWebIdentityToken is the IAM action for STS GetWebIdentityToken.
const ActionSTSGetWebIdentityToken = "sts:GetWebIdentityToken"

// Organizations actions (Phase 2).

// ActionOrgsCreateAccount is the IAM action for Organizations CreateAccount.
const ActionOrgsCreateAccount = "organizations:CreateAccount"

// ActionOrgsDescribeCreateAccountStatus is the IAM action for Organizations DescribeCreateAccountStatus.
const ActionOrgsDescribeCreateAccountStatus = "organizations:DescribeCreateAccountStatus"

// IAM lab actions (Phase 3).

// Users
const (
	ActionIAMCreateUser = "iam:CreateUser"
	ActionIAMGetUser    = "iam:GetUser"
	ActionIAMListUsers  = "iam:ListUsers"
	ActionIAMDeleteUser = "iam:DeleteUser"
)

// Access keys
const (
	ActionIAMCreateAccessKey = "iam:CreateAccessKey"
	ActionIAMDeleteAccessKey = "iam:DeleteAccessKey"
	ActionIAMListAccessKeys  = "iam:ListAccessKeys"
	ActionIAMUpdateAccessKey = "iam:UpdateAccessKey"
)

// Managed policies
const (
	ActionIAMCreatePolicy = "iam:CreatePolicy"
	ActionIAMGetPolicy    = "iam:GetPolicy"
	ActionIAMListPolicies = "iam:ListPolicies"
	ActionIAMDeletePolicy = "iam:DeletePolicy"
)

// Attachments
const (
	ActionIAMAttachUserPolicy         = "iam:AttachUserPolicy"
	ActionIAMDetachUserPolicy         = "iam:DetachUserPolicy"
	ActionIAMAttachRolePolicy         = "iam:AttachRolePolicy"
	ActionIAMDetachRolePolicy         = "iam:DetachRolePolicy"
	ActionIAMListAttachedUserPolicies = "iam:ListAttachedUserPolicies"
	ActionIAMListAttachedRolePolicies = "iam:ListAttachedRolePolicies"
)

// Inline user policies
const (
	ActionIAMPutUserPolicy    = "iam:PutUserPolicy"
	ActionIAMGetUserPolicy    = "iam:GetUserPolicy"
	ActionIAMDeleteUserPolicy = "iam:DeleteUserPolicy"
	ActionIAMListUserPolicies = "iam:ListUserPolicies"
)

// Inline role policies
const (
	ActionIAMPutRolePolicy    = "iam:PutRolePolicy"
	ActionIAMGetRolePolicy    = "iam:GetRolePolicy"
	ActionIAMDeleteRolePolicy = "iam:DeleteRolePolicy"
	ActionIAMListRolePolicies = "iam:ListRolePolicies"
)

// Roles
const (
	ActionIAMCreateRole             = "iam:CreateRole"
	ActionIAMGetRole                = "iam:GetRole"
	ActionIAMListRoles              = "iam:ListRoles"
	ActionIAMDeleteRole             = "iam:DeleteRole"
	ActionIAMUpdateAssumeRolePolicy = "iam:UpdateAssumeRolePolicy"
)

// KMS lab actions (Phase 4).
const (
	ActionKMSCreateKey                       = "kms:CreateKey"
	ActionKMSDescribeKey                     = "kms:DescribeKey"
	ActionKMSListKeys                        = "kms:ListKeys"
	ActionKMSEnableKey                       = "kms:EnableKey"
	ActionKMSDisableKey                      = "kms:DisableKey"
	ActionKMSGetKeyPolicy                    = "kms:GetKeyPolicy"
	ActionKMSPutKeyPolicy                    = "kms:PutKeyPolicy"
	ActionKMSEncrypt                         = "kms:Encrypt"
	ActionKMSDecrypt                         = "kms:Decrypt"
	ActionKMSGenerateDataKey                 = "kms:GenerateDataKey"
	ActionKMSGenerateDataKeyWithoutPlaintext = "kms:GenerateDataKeyWithoutPlaintext"
	ActionKMSCreateGrant                     = "kms:CreateGrant"
	ActionKMSListGrants                      = "kms:ListGrants"
	ActionKMSRetireGrant                     = "kms:RetireGrant"
	ActionKMSRevokeGrant                     = "kms:RevokeGrant"
	ActionKMSCreateAlias                     = "kms:CreateAlias"
	ActionKMSListAliases                     = "kms:ListAliases"
	ActionKMSDeleteAlias                     = "kms:DeleteAlias"
	ActionKMSUpdateAlias                     = "kms:UpdateAlias"
)

// KnownAction reports whether action is recognized in the current catalog.
func KnownAction(action string) bool {
	switch action {
	case ActionSTSGetCallerIdentity,
		ActionSTSAssumeRole,
		ActionSTSGetSessionToken,
		ActionSTSGetFederationToken,
		ActionSTSAssumeRoleWithSAML,
		ActionSTSAssumeRoleWithWebIdentity,
		ActionSTSAssumeRoot,
		ActionSTSDecodeAuthorizationMessage,
		ActionSTSGetAccessKeyInfo,
		ActionSTSGetDelegatedAccessToken,
		ActionSTSGetWebIdentityToken,
		ActionOrgsCreateAccount,
		ActionOrgsDescribeCreateAccountStatus,
		ActionIAMCreateUser,
		ActionIAMGetUser,
		ActionIAMListUsers,
		ActionIAMDeleteUser,
		ActionIAMCreateAccessKey,
		ActionIAMDeleteAccessKey,
		ActionIAMListAccessKeys,
		ActionIAMUpdateAccessKey,
		ActionIAMCreatePolicy,
		ActionIAMGetPolicy,
		ActionIAMListPolicies,
		ActionIAMDeletePolicy,
		ActionIAMAttachUserPolicy,
		ActionIAMDetachUserPolicy,
		ActionIAMAttachRolePolicy,
		ActionIAMDetachRolePolicy,
		ActionIAMListAttachedUserPolicies,
		ActionIAMListAttachedRolePolicies,
		ActionIAMPutUserPolicy,
		ActionIAMGetUserPolicy,
		ActionIAMDeleteUserPolicy,
		ActionIAMListUserPolicies,
		ActionIAMPutRolePolicy,
		ActionIAMGetRolePolicy,
		ActionIAMDeleteRolePolicy,
		ActionIAMListRolePolicies,
		ActionIAMCreateRole,
		ActionIAMGetRole,
		ActionIAMListRoles,
		ActionIAMDeleteRole,
		ActionIAMUpdateAssumeRolePolicy,
		ActionKMSCreateKey,
		ActionKMSDescribeKey,
		ActionKMSListKeys,
		ActionKMSEnableKey,
		ActionKMSDisableKey,
		ActionKMSGetKeyPolicy,
		ActionKMSPutKeyPolicy,
		ActionKMSEncrypt,
		ActionKMSDecrypt,
		ActionKMSGenerateDataKey,
		ActionKMSGenerateDataKeyWithoutPlaintext,
		ActionKMSCreateGrant,
		ActionKMSListGrants,
		ActionKMSRetireGrant,
		ActionKMSRevokeGrant,
		ActionKMSCreateAlias,
		ActionKMSListAliases,
		ActionKMSDeleteAlias,
		ActionKMSUpdateAlias:
		return true
	default:
		return false
	}
}
