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

// Organizations depth actions.
const (
	ActionOrgsListAccounts                     = "organizations:ListAccounts"
	ActionOrgsCreateOrganizationalUnit         = "organizations:CreateOrganizationalUnit"
	ActionOrgsListOrganizationalUnitsForParent = "organizations:ListOrganizationalUnitsForParent"
	ActionOrgsEnablePolicyType                 = "organizations:EnablePolicyType"
	ActionOrgsCreatePolicy                     = "organizations:CreatePolicy"
	ActionOrgsAttachPolicy                     = "organizations:AttachPolicy"
	ActionOrgsDetachPolicy                     = "organizations:DetachPolicy"
	ActionOrgsDescribePolicy                   = "organizations:DescribePolicy"
	ActionOrgsMoveAccount                      = "organizations:MoveAccount"
)

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

// Groups
const (
	ActionIAMCreateGroup               = "iam:CreateGroup"
	ActionIAMDeleteGroup               = "iam:DeleteGroup"
	ActionIAMGetGroup                  = "iam:GetGroup"
	ActionIAMListGroups                = "iam:ListGroups"
	ActionIAMAddUserToGroup            = "iam:AddUserToGroup"
	ActionIAMRemoveUserFromGroup       = "iam:RemoveUserFromGroup"
	ActionIAMAttachGroupPolicy         = "iam:AttachGroupPolicy"
	ActionIAMDetachGroupPolicy         = "iam:DetachGroupPolicy"
	ActionIAMListAttachedGroupPolicies = "iam:ListAttachedGroupPolicies"
	ActionIAMPutGroupPolicy            = "iam:PutGroupPolicy"
	ActionIAMGetGroupPolicy            = "iam:GetGroupPolicy"
	ActionIAMDeleteGroupPolicy         = "iam:DeleteGroupPolicy"
	ActionIAMListGroupPolicies         = "iam:ListGroupPolicies"
)

// Permissions boundaries
const (
	ActionIAMPutUserPermissionsBoundary    = "iam:PutUserPermissionsBoundary"
	ActionIAMGetUserPermissionsBoundary    = "iam:GetUserPermissionsBoundary"
	ActionIAMDeleteUserPermissionsBoundary = "iam:DeleteUserPermissionsBoundary"
	ActionIAMPutRolePermissionsBoundary    = "iam:PutRolePermissionsBoundary"
	ActionIAMGetRolePermissionsBoundary    = "iam:GetRolePermissionsBoundary"
	ActionIAMDeleteRolePermissionsBoundary = "iam:DeleteRolePermissionsBoundary"
)

// Instance profiles
const (
	ActionIAMCreateInstanceProfile         = "iam:CreateInstanceProfile"
	ActionIAMDeleteInstanceProfile         = "iam:DeleteInstanceProfile"
	ActionIAMGetInstanceProfile            = "iam:GetInstanceProfile"
	ActionIAMAddRoleToInstanceProfile      = "iam:AddRoleToInstanceProfile"
	ActionIAMRemoveRoleFromInstanceProfile = "iam:RemoveRoleFromInstanceProfile"
	ActionIAMListInstanceProfiles          = "iam:ListInstanceProfiles"
)

// Identity providers
const (
	ActionIAMCreateOpenIDConnectProvider = "iam:CreateOpenIDConnectProvider"
	ActionIAMDeleteOpenIDConnectProvider = "iam:DeleteOpenIDConnectProvider"
	ActionIAMListOpenIDConnectProviders  = "iam:ListOpenIDConnectProviders"
	ActionIAMGetOpenIDConnectProvider    = "iam:GetOpenIDConnectProvider"
	ActionIAMCreateSAMLProvider          = "iam:CreateSAMLProvider"
	ActionIAMDeleteSAMLProvider          = "iam:DeleteSAMLProvider"
	ActionIAMListSAMLProviders           = "iam:ListSAMLProviders"
	ActionIAMGetSAMLProvider             = "iam:GetSAMLProvider"
)

// MFA devices
const (
	ActionIAMCreateVirtualMFADevice = "iam:CreateVirtualMFADevice"
	ActionIAMEnableMFADevice        = "iam:EnableMFADevice"
	ActionIAMListMFADevices         = "iam:ListMFADevices"
	ActionIAMDeactivateMFADevice    = "iam:DeactivateMFADevice"
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
	ActionKMSScheduleKeyDeletion             = "kms:ScheduleKeyDeletion"
	ActionKMSCancelKeyDeletion               = "kms:CancelKeyDeletion"
	ActionKMSEnableKeyRotation               = "kms:EnableKeyRotation"
	ActionKMSDisableKeyRotation              = "kms:DisableKeyRotation"
	ActionKMSGetKeyRotationStatus            = "kms:GetKeyRotationStatus"
	ActionKMSReEncryptFrom                   = "kms:ReEncryptFrom"
	ActionKMSReEncryptTo                     = "kms:ReEncryptTo"
)

// S3 lab actions (Phase 5).
const (
	ActionS3CreateBucket                  = "s3:CreateBucket"
	ActionS3DeleteBucket                  = "s3:DeleteBucket"
	ActionS3ListAllMyBuckets              = "s3:ListAllMyBuckets"
	ActionS3ListBucket                    = "s3:ListBucket"
	ActionS3GetBucketPolicy               = "s3:GetBucketPolicy"
	ActionS3PutBucketPolicy               = "s3:PutBucketPolicy"
	ActionS3DeleteBucketPolicy            = "s3:DeleteBucketPolicy"
	ActionS3GetObject                     = "s3:GetObject"
	ActionS3PutObject                     = "s3:PutObject"
	ActionS3DeleteObject                  = "s3:DeleteObject"
	ActionS3CreateMultipartUpload         = "s3:CreateMultipartUpload"
	ActionS3UploadPart                    = "s3:UploadPart"
	ActionS3CompleteMultipartUpload       = "s3:CompleteMultipartUpload"
	ActionS3AbortMultipartUpload          = "s3:AbortMultipartUpload"
	ActionS3ListParts                     = "s3:ListParts"
	ActionS3ListMultipartUploads          = "s3:ListMultipartUploads"
	ActionS3PutEncryptionConfiguration    = "s3:PutEncryptionConfiguration"
	ActionS3GetEncryptionConfiguration    = "s3:GetEncryptionConfiguration"
	ActionS3DeleteEncryptionConfiguration = "s3:DeleteEncryptionConfiguration"
)

// DynamoDB lab actions (Phase 6).
const (
	ActionDynamoDBCreateTable          = "dynamodb:CreateTable"
	ActionDynamoDBDescribeTable        = "dynamodb:DescribeTable"
	ActionDynamoDBDeleteTable          = "dynamodb:DeleteTable"
	ActionDynamoDBListTables           = "dynamodb:ListTables"
	ActionDynamoDBUpdateTable          = "dynamodb:UpdateTable"
	ActionDynamoDBPutItem              = "dynamodb:PutItem"
	ActionDynamoDBGetItem              = "dynamodb:GetItem"
	ActionDynamoDBDeleteItem           = "dynamodb:DeleteItem"
	ActionDynamoDBUpdateItem           = "dynamodb:UpdateItem"
	ActionDynamoDBQuery                = "dynamodb:Query"
	ActionDynamoDBScan                 = "dynamodb:Scan"
	ActionDynamoDBBatchGetItem         = "dynamodb:BatchGetItem"
	ActionDynamoDBBatchWriteItem       = "dynamodb:BatchWriteItem"
	ActionDynamoDBPutResourcePolicy    = "dynamodb:PutResourcePolicy"
	ActionDynamoDBGetResourcePolicy    = "dynamodb:GetResourcePolicy"
	ActionDynamoDBDeleteResourcePolicy = "dynamodb:DeleteResourcePolicy"
	ActionDynamoDBUpdateTimeToLive     = "dynamodb:UpdateTimeToLive"
	ActionDynamoDBDescribeTimeToLive   = "dynamodb:DescribeTimeToLive"
)

// SQS lab actions (Phase 6).
const (
	ActionSQSCreateQueue             = "sqs:CreateQueue"
	ActionSQSGetQueueUrl             = "sqs:GetQueueUrl"
	ActionSQSGetQueueAttributes      = "sqs:GetQueueAttributes"
	ActionSQSSetQueueAttributes      = "sqs:SetQueueAttributes"
	ActionSQSDeleteQueue             = "sqs:DeleteQueue"
	ActionSQSListQueues              = "sqs:ListQueues"
	ActionSQSPurgeQueue              = "sqs:PurgeQueue"
	ActionSQSSendMessage             = "sqs:SendMessage"
	ActionSQSReceiveMessage          = "sqs:ReceiveMessage"
	ActionSQSDeleteMessage           = "sqs:DeleteMessage"
	ActionSQSSendMessageBatch        = "sqs:SendMessageBatch"
	ActionSQSDeleteMessageBatch      = "sqs:DeleteMessageBatch"
	ActionSQSChangeMessageVisibility = "sqs:ChangeMessageVisibility"
)

// SSM lab actions (Phase 8).
const (
	ActionSSMPutParameter       = "ssm:PutParameter"
	ActionSSMGetParameter       = "ssm:GetParameter"
	ActionSSMGetParameters      = "ssm:GetParameters"
	ActionSSMDeleteParameter    = "ssm:DeleteParameter"
	ActionSSMDescribeParameters = "ssm:DescribeParameters"
)

// SNS lab actions (Phase 9).
const (
	ActionSNSCreateTopic               = "sns:CreateTopic"
	ActionSNSDeleteTopic               = "sns:DeleteTopic"
	ActionSNSListTopics                = "sns:ListTopics"
	ActionSNSGetTopicAttributes        = "sns:GetTopicAttributes"
	ActionSNSSetTopicAttributes        = "sns:SetTopicAttributes"
	ActionSNSPublish                   = "sns:Publish"
	ActionSNSSubscribe                 = "sns:Subscribe"
	ActionSNSUnsubscribe               = "sns:Unsubscribe"
	ActionSNSListSubscriptions         = "sns:ListSubscriptions"
	ActionSNSListSubscriptionsByTopic  = "sns:ListSubscriptionsByTopic"
	ActionSNSGetSubscriptionAttributes = "sns:GetSubscriptionAttributes"
	ActionSNSAddPermission             = "sns:AddPermission"
	ActionSNSRemovePermission          = "sns:RemovePermission"
)

// EventBridge lab actions (Phase 9).
const (
	ActionEventsPutEvents         = "events:PutEvents"
	ActionEventsCreateEventBus    = "events:CreateEventBus"
	ActionEventsDeleteEventBus    = "events:DeleteEventBus"
	ActionEventsDescribeEventBus  = "events:DescribeEventBus"
	ActionEventsListEventBuses    = "events:ListEventBuses"
	ActionEventsPutRule           = "events:PutRule"
	ActionEventsDescribeRule      = "events:DescribeRule"
	ActionEventsListRules         = "events:ListRules"
	ActionEventsDeleteRule        = "events:DeleteRule"
	ActionEventsEnableRule        = "events:EnableRule"
	ActionEventsDisableRule       = "events:DisableRule"
	ActionEventsPutTargets        = "events:PutTargets"
	ActionEventsRemoveTargets     = "events:RemoveTargets"
	ActionEventsListTargetsByRule = "events:ListTargetsByRule"
)

// ECR lab actions (Phase 9).
const (
	ActionECRCreateRepository            = "ecr:CreateRepository"
	ActionECRDescribeRepositories        = "ecr:DescribeRepositories"
	ActionECRDeleteRepository            = "ecr:DeleteRepository"
	ActionECRGetAuthorizationToken       = "ecr:GetAuthorizationToken"
	ActionECRGetRepositoryPolicy         = "ecr:GetRepositoryPolicy"
	ActionECRSetRepositoryPolicy         = "ecr:SetRepositoryPolicy"
	ActionECRDeleteRepositoryPolicy      = "ecr:DeleteRepositoryPolicy"
	ActionECRPutImage                    = "ecr:PutImage"
	ActionECRBatchGetImage               = "ecr:BatchGetImage"
	ActionECRListImages                  = "ecr:ListImages"
	ActionECRBatchDeleteImage            = "ecr:BatchDeleteImage"
	ActionECRInitiateLayerUpload         = "ecr:InitiateLayerUpload"
	ActionECRUploadLayerPart             = "ecr:UploadLayerPart"
	ActionECRCompleteLayerUpload         = "ecr:CompleteLayerUpload"
	ActionECRBatchCheckLayerAvailability = "ecr:BatchCheckLayerAvailability"
)

// ECS lab actions (Phase 9).
const (
	ActionECSRegisterTaskDefinition   = "ecs:RegisterTaskDefinition"
	ActionECSDescribeTaskDefinition   = "ecs:DescribeTaskDefinition"
	ActionECSListTaskDefinitions      = "ecs:ListTaskDefinitions"
	ActionECSDeregisterTaskDefinition = "ecs:DeregisterTaskDefinition"
	ActionECSRunTask                  = "ecs:RunTask"
	ActionECSDescribeTasks            = "ecs:DescribeTasks"
	ActionECSListTasks                = "ecs:ListTasks"
	ActionECSStopTask                 = "ecs:StopTask"
	ActionECSDescribeClusters         = "ecs:DescribeClusters"
	ActionECSListClusters             = "ecs:ListClusters"
)

// Secrets Manager lab actions (Phase 8).
const (
	ActionSecretsCreateSecret         = "secretsmanager:CreateSecret"
	ActionSecretsGetSecretValue       = "secretsmanager:GetSecretValue"
	ActionSecretsPutSecretValue       = "secretsmanager:PutSecretValue"
	ActionSecretsDeleteSecret         = "secretsmanager:DeleteSecret"
	ActionSecretsDescribeSecret       = "secretsmanager:DescribeSecret"
	ActionSecretsListSecrets          = "secretsmanager:ListSecrets"
	ActionSecretsPutResourcePolicy    = "secretsmanager:PutResourcePolicy"
	ActionSecretsGetResourcePolicy    = "secretsmanager:GetResourcePolicy"
	ActionSecretsDeleteResourcePolicy = "secretsmanager:DeleteResourcePolicy"
)

// CloudTrail lab actions (v3 N-AUDIT).
const (
	ActionCloudTrailLookupEvents = "cloudtrail:LookupEvents"
)

// CloudWatch Logs lab actions (v3 N-AUDIT).
const (
	ActionLogsCreateLogGroup    = "logs:CreateLogGroup"
	ActionLogsCreateLogStream   = "logs:CreateLogStream"
	ActionLogsPutLogEvents      = "logs:PutLogEvents"
	ActionLogsGetLogEvents      = "logs:GetLogEvents"
	ActionLogsDescribeLogGroups = "logs:DescribeLogGroups"
)

// Resource Groups Tagging API lab actions (v3 N-AUDIT).
const (
	ActionTaggingTagResources   = "tag:TagResources"
	ActionTaggingUntagResources = "tag:UntagResources"
	ActionTaggingGetResources   = "tag:GetResources"
)

// Kinesis lab actions (v3 Track B).
const (
	ActionKinesisCreateStream     = "kinesis:CreateStream"
	ActionKinesisDeleteStream     = "kinesis:DeleteStream"
	ActionKinesisDescribeStream   = "kinesis:DescribeStream"
	ActionKinesisListStreams      = "kinesis:ListStreams"
	ActionKinesisPutRecord        = "kinesis:PutRecord"
	ActionKinesisPutRecords       = "kinesis:PutRecords"
	ActionKinesisGetShardIterator = "kinesis:GetShardIterator"
	ActionKinesisGetRecords       = "kinesis:GetRecords"
)

// AppConfig lab actions (v3 Track B).
const (
	ActionAppConfigCreateApplication               = "appconfig:CreateApplication"
	ActionAppConfigCreateEnvironment               = "appconfig:CreateEnvironment"
	ActionAppConfigCreateConfigurationProfile      = "appconfig:CreateConfigurationProfile"
	ActionAppConfigCreateHostedConfigurationVersion = "appconfig:CreateHostedConfigurationVersion"
	ActionAppConfigGetConfiguration                = "appconfig:GetConfiguration"
	ActionAppConfigDataStartConfigurationSession   = "appconfigdata:StartConfigurationSession"
	ActionAppConfigDataGetLatestConfiguration      = "appconfigdata:GetLatestConfiguration"
)

// SES lab actions (v3 Track B).
const (
	ActionSESVerifyEmailIdentity = "ses:VerifyEmailIdentity"
	ActionSESListIdentities      = "ses:ListIdentities"
	ActionSESSendEmail           = "ses:SendEmail"
	ActionSESSendRawEmail        = "ses:SendRawEmail"
	ActionSESGetSendStatistics   = "ses:GetSendStatistics"
)

// Step Functions lab actions (v3 Track B).
const (
	ActionSFNCreateStateMachine   = "states:CreateStateMachine"
	ActionSFNDeleteStateMachine   = "states:DeleteStateMachine"
	ActionSFNDescribeStateMachine = "states:DescribeStateMachine"
	ActionSFNListStateMachines    = "states:ListStateMachines"
	ActionSFNStartExecution       = "states:StartExecution"
	ActionSFNDescribeExecution    = "states:DescribeExecution"
	ActionSFNGetExecutionHistory  = "states:GetExecutionHistory"
)

// Lambda lab actions (Phase 7).
const (
	ActionLambdaCreateFunction              = "lambda:CreateFunction"
	ActionLambdaGetFunction                 = "lambda:GetFunction"
	ActionLambdaDeleteFunction              = "lambda:DeleteFunction"
	ActionLambdaListFunctions               = "lambda:ListFunctions"
	ActionLambdaUpdateFunctionCode          = "lambda:UpdateFunctionCode"
	ActionLambdaUpdateFunctionConfiguration = "lambda:UpdateFunctionConfiguration"
	ActionLambdaInvoke                      = "lambda:InvokeFunction"
	ActionLambdaPublishVersion              = "lambda:PublishVersion"
	ActionLambdaListVersionsByFunction      = "lambda:ListVersionsByFunction"
	ActionLambdaCreateAlias                 = "lambda:CreateAlias"
	ActionLambdaUpdateAlias                 = "lambda:UpdateAlias"
	ActionLambdaDeleteAlias                 = "lambda:DeleteAlias"
	ActionLambdaGetAlias                    = "lambda:GetAlias"
	ActionLambdaListAliases                 = "lambda:ListAliases"
	ActionLambdaPublishLayerVersion         = "lambda:PublishLayerVersion"
	ActionLambdaGetLayerVersion             = "lambda:GetLayerVersion"
	ActionLambdaListLayerVersions           = "lambda:ListLayerVersions"
	ActionLambdaDeleteLayerVersion          = "lambda:DeleteLayerVersion"
	ActionLambdaAddPermission               = "lambda:AddPermission"
	ActionLambdaRemovePermission            = "lambda:RemovePermission"
	ActionLambdaGetPolicy                   = "lambda:GetPolicy"
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
		ActionOrgsListAccounts,
		ActionOrgsCreateOrganizationalUnit,
		ActionOrgsListOrganizationalUnitsForParent,
		ActionOrgsEnablePolicyType,
		ActionOrgsCreatePolicy,
		ActionOrgsAttachPolicy,
		ActionOrgsDetachPolicy,
		ActionOrgsDescribePolicy,
		ActionOrgsMoveAccount,
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
		ActionIAMCreateGroup,
		ActionIAMDeleteGroup,
		ActionIAMGetGroup,
		ActionIAMListGroups,
		ActionIAMAddUserToGroup,
		ActionIAMRemoveUserFromGroup,
		ActionIAMAttachGroupPolicy,
		ActionIAMDetachGroupPolicy,
		ActionIAMListAttachedGroupPolicies,
		ActionIAMPutGroupPolicy,
		ActionIAMGetGroupPolicy,
		ActionIAMDeleteGroupPolicy,
		ActionIAMListGroupPolicies,
		ActionIAMPutUserPermissionsBoundary,
		ActionIAMGetUserPermissionsBoundary,
		ActionIAMDeleteUserPermissionsBoundary,
		ActionIAMPutRolePermissionsBoundary,
		ActionIAMGetRolePermissionsBoundary,
		ActionIAMDeleteRolePermissionsBoundary,
		ActionIAMCreateInstanceProfile,
		ActionIAMDeleteInstanceProfile,
		ActionIAMGetInstanceProfile,
		ActionIAMAddRoleToInstanceProfile,
		ActionIAMRemoveRoleFromInstanceProfile,
		ActionIAMListInstanceProfiles,
		ActionIAMCreateOpenIDConnectProvider,
		ActionIAMDeleteOpenIDConnectProvider,
		ActionIAMListOpenIDConnectProviders,
		ActionIAMGetOpenIDConnectProvider,
		ActionIAMCreateSAMLProvider,
		ActionIAMDeleteSAMLProvider,
		ActionIAMListSAMLProviders,
		ActionIAMGetSAMLProvider,
		ActionIAMCreateVirtualMFADevice,
		ActionIAMEnableMFADevice,
		ActionIAMListMFADevices,
		ActionIAMDeactivateMFADevice,
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
		ActionKMSUpdateAlias,
		ActionKMSScheduleKeyDeletion,
		ActionKMSCancelKeyDeletion,
		ActionKMSEnableKeyRotation,
		ActionKMSDisableKeyRotation,
		ActionKMSGetKeyRotationStatus,
		ActionKMSReEncryptFrom,
		ActionKMSReEncryptTo,
		ActionS3CreateBucket,
		ActionS3DeleteBucket,
		ActionS3ListAllMyBuckets,
		ActionS3ListBucket,
		ActionS3GetBucketPolicy,
		ActionS3PutBucketPolicy,
		ActionS3DeleteBucketPolicy,
		ActionS3GetObject,
		ActionS3PutObject,
		ActionS3DeleteObject,
		ActionS3CreateMultipartUpload,
		ActionS3UploadPart,
		ActionS3CompleteMultipartUpload,
		ActionS3AbortMultipartUpload,
		ActionS3ListParts,
		ActionS3ListMultipartUploads,
		ActionS3PutEncryptionConfiguration,
		ActionS3GetEncryptionConfiguration,
		ActionS3DeleteEncryptionConfiguration,
		ActionDynamoDBCreateTable,
		ActionDynamoDBDescribeTable,
		ActionDynamoDBDeleteTable,
		ActionDynamoDBListTables,
		ActionDynamoDBUpdateTable,
		ActionDynamoDBPutItem,
		ActionDynamoDBGetItem,
		ActionDynamoDBDeleteItem,
		ActionDynamoDBUpdateItem,
		ActionDynamoDBQuery,
		ActionDynamoDBScan,
		ActionDynamoDBBatchGetItem,
		ActionDynamoDBBatchWriteItem,
		ActionDynamoDBPutResourcePolicy,
		ActionDynamoDBGetResourcePolicy,
		ActionDynamoDBDeleteResourcePolicy,
		ActionDynamoDBUpdateTimeToLive,
		ActionDynamoDBDescribeTimeToLive,
		ActionSQSCreateQueue,
		ActionSQSGetQueueUrl,
		ActionSQSGetQueueAttributes,
		ActionSQSSetQueueAttributes,
		ActionSQSDeleteQueue,
		ActionSQSListQueues,
		ActionSQSPurgeQueue,
		ActionSQSSendMessage,
		ActionSQSReceiveMessage,
		ActionSQSDeleteMessage,
		ActionSQSSendMessageBatch,
		ActionSQSDeleteMessageBatch,
		ActionSQSChangeMessageVisibility,
		ActionLambdaCreateFunction,
		ActionLambdaGetFunction,
		ActionLambdaDeleteFunction,
		ActionLambdaListFunctions,
		ActionLambdaUpdateFunctionCode,
		ActionLambdaUpdateFunctionConfiguration,
		ActionLambdaInvoke,
		ActionLambdaPublishVersion,
		ActionLambdaListVersionsByFunction,
		ActionLambdaCreateAlias,
		ActionLambdaUpdateAlias,
		ActionLambdaDeleteAlias,
		ActionLambdaGetAlias,
		ActionLambdaListAliases,
		ActionLambdaPublishLayerVersion,
		ActionLambdaGetLayerVersion,
		ActionLambdaListLayerVersions,
		ActionLambdaDeleteLayerVersion,
		ActionLambdaAddPermission,
		ActionLambdaRemovePermission,
		ActionLambdaGetPolicy,
		ActionSSMPutParameter,
		ActionSSMGetParameter,
		ActionSSMGetParameters,
		ActionSSMDeleteParameter,
		ActionSSMDescribeParameters,
		ActionSecretsCreateSecret,
		ActionSecretsGetSecretValue,
		ActionSecretsPutSecretValue,
		ActionSecretsDeleteSecret,
		ActionSecretsDescribeSecret,
		ActionSecretsListSecrets,
		ActionSecretsPutResourcePolicy,
		ActionSecretsGetResourcePolicy,
		ActionSecretsDeleteResourcePolicy,
		ActionSNSCreateTopic,
		ActionSNSDeleteTopic,
		ActionSNSListTopics,
		ActionSNSGetTopicAttributes,
		ActionSNSSetTopicAttributes,
		ActionSNSPublish,
		ActionSNSSubscribe,
		ActionSNSUnsubscribe,
		ActionSNSListSubscriptions,
		ActionSNSListSubscriptionsByTopic,
		ActionSNSGetSubscriptionAttributes,
		ActionSNSAddPermission,
		ActionSNSRemovePermission,
		ActionEventsPutEvents,
		ActionEventsCreateEventBus,
		ActionEventsDeleteEventBus,
		ActionEventsDescribeEventBus,
		ActionEventsListEventBuses,
		ActionEventsPutRule,
		ActionEventsDescribeRule,
		ActionEventsListRules,
		ActionEventsDeleteRule,
		ActionEventsEnableRule,
		ActionEventsDisableRule,
		ActionEventsPutTargets,
		ActionEventsRemoveTargets,
		ActionEventsListTargetsByRule,
		ActionECRCreateRepository,
		ActionECRDescribeRepositories,
		ActionECRDeleteRepository,
		ActionECRGetAuthorizationToken,
		ActionECRGetRepositoryPolicy,
		ActionECRSetRepositoryPolicy,
		ActionECRDeleteRepositoryPolicy,
		ActionECRPutImage,
		ActionECRBatchGetImage,
		ActionECRListImages,
		ActionECRBatchDeleteImage,
		ActionECRInitiateLayerUpload,
		ActionECRUploadLayerPart,
		ActionECRCompleteLayerUpload,
		ActionECRBatchCheckLayerAvailability,
		ActionECSRegisterTaskDefinition,
		ActionECSDescribeTaskDefinition,
		ActionECSListTaskDefinitions,
		ActionECSDeregisterTaskDefinition,
		ActionECSRunTask,
		ActionECSDescribeTasks,
		ActionECSListTasks,
		ActionECSStopTask,
		ActionECSDescribeClusters,
		ActionECSListClusters,
		ActionCloudTrailLookupEvents,
		ActionLogsCreateLogGroup,
		ActionLogsCreateLogStream,
		ActionLogsPutLogEvents,
		ActionLogsGetLogEvents,
		ActionLogsDescribeLogGroups,
		ActionTaggingTagResources,
		ActionTaggingUntagResources,
		ActionTaggingGetResources,
		ActionKinesisCreateStream,
		ActionKinesisDeleteStream,
		ActionKinesisDescribeStream,
		ActionKinesisListStreams,
		ActionKinesisPutRecord,
		ActionKinesisPutRecords,
		ActionKinesisGetShardIterator,
		ActionKinesisGetRecords,
		ActionAppConfigCreateApplication,
		ActionAppConfigCreateEnvironment,
		ActionAppConfigCreateConfigurationProfile,
		ActionAppConfigCreateHostedConfigurationVersion,
		ActionAppConfigGetConfiguration,
		ActionAppConfigDataStartConfigurationSession,
		ActionAppConfigDataGetLatestConfiguration,
		ActionSESVerifyEmailIdentity,
		ActionSESListIdentities,
		ActionSESSendEmail,
		ActionSESSendRawEmail,
		ActionSESGetSendStatistics,
		ActionSFNCreateStateMachine,
		ActionSFNDeleteStateMachine,
		ActionSFNDescribeStateMachine,
		ActionSFNListStateMachines,
		ActionSFNStartExecution,
		ActionSFNDescribeExecution,
		ActionSFNGetExecutionHistory:
		return true
	default:
		return false
	}
}
