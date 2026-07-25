package catalog

// STS actions (all 11 lab routes).

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

// Organizations actions.

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

// IAM lab actions.

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
	ActionIAMCreatePolicy            = "iam:CreatePolicy"
	ActionIAMGetPolicy               = "iam:GetPolicy"
	ActionIAMListPolicies            = "iam:ListPolicies"
	ActionIAMDeletePolicy            = "iam:DeletePolicy"
	ActionIAMCreatePolicyVersion     = "iam:CreatePolicyVersion"
	ActionIAMGetPolicyVersion        = "iam:GetPolicyVersion"
	ActionIAMListPolicyVersions      = "iam:ListPolicyVersions"
	ActionIAMDeletePolicyVersion     = "iam:DeletePolicyVersion"
	ActionIAMSetDefaultPolicyVersion = "iam:SetDefaultPolicyVersion"
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
	ActionIAMListInstanceProfilesForRole   = "iam:ListInstanceProfilesForRole"
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

// KMS lab actions.
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
	ActionKMSListResourceTags                = "kms:ListResourceTags"
	ActionKMSTagResource                     = "kms:TagResource"
	ActionKMSUntagResource                   = "kms:UntagResource"
)

// S3 lab actions.
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
	ActionS3PutBucketVersioning           = "s3:PutBucketVersioning"
	ActionS3GetBucketVersioning           = "s3:GetBucketVersioning"
	ActionS3ListBucketVersions            = "s3:ListBucketVersions"
	ActionS3PutBucketNotification         = "s3:PutBucketNotification"
	ActionS3GetBucketNotification         = "s3:GetBucketNotification"
)

// DynamoDB lab actions.
const (
	ActionDynamoDBCreateTable               = "dynamodb:CreateTable"
	ActionDynamoDBDescribeTable             = "dynamodb:DescribeTable"
	ActionDynamoDBDeleteTable               = "dynamodb:DeleteTable"
	ActionDynamoDBListTables                = "dynamodb:ListTables"
	ActionDynamoDBUpdateTable               = "dynamodb:UpdateTable"
	ActionDynamoDBPutItem                   = "dynamodb:PutItem"
	ActionDynamoDBGetItem                   = "dynamodb:GetItem"
	ActionDynamoDBDeleteItem                = "dynamodb:DeleteItem"
	ActionDynamoDBUpdateItem                = "dynamodb:UpdateItem"
	ActionDynamoDBQuery                     = "dynamodb:Query"
	ActionDynamoDBScan                      = "dynamodb:Scan"
	ActionDynamoDBBatchGetItem              = "dynamodb:BatchGetItem"
	ActionDynamoDBBatchWriteItem            = "dynamodb:BatchWriteItem"
	ActionDynamoDBTransactGetItems          = "dynamodb:TransactGetItems"
	ActionDynamoDBTransactWriteItems        = "dynamodb:TransactWriteItems"
	ActionDynamoDBConditionCheckItem        = "dynamodb:ConditionCheckItem"
	ActionDynamoDBPutResourcePolicy         = "dynamodb:PutResourcePolicy"
	ActionDynamoDBGetResourcePolicy         = "dynamodb:GetResourcePolicy"
	ActionDynamoDBDeleteResourcePolicy      = "dynamodb:DeleteResourcePolicy"
	ActionDynamoDBUpdateTimeToLive          = "dynamodb:UpdateTimeToLive"
	ActionDynamoDBDescribeTimeToLive        = "dynamodb:DescribeTimeToLive"
	ActionDynamoDBDescribeContinuousBackups = "dynamodb:DescribeContinuousBackups"
	ActionDynamoDBListTagsOfResource        = "dynamodb:ListTagsOfResource"
	ActionDynamoDBTagResource               = "dynamodb:TagResource"
	ActionDynamoDBUntagResource             = "dynamodb:UntagResource"
)

// DynamoDB Streams lab actions.
const (
	ActionDynamoDBStreamsListStreams      = "dynamodbstreams:ListStreams"
	ActionDynamoDBStreamsDescribeStream   = "dynamodbstreams:DescribeStream"
	ActionDynamoDBStreamsGetShardIterator = "dynamodbstreams:GetShardIterator"
	ActionDynamoDBStreamsGetRecords       = "dynamodbstreams:GetRecords"
)

// EventBridge Pipes lab actions.
const (
	ActionPipesCreatePipe   = "pipes:CreatePipe"
	ActionPipesDescribePipe = "pipes:DescribePipe"
	ActionPipesDeletePipe   = "pipes:DeletePipe"
	ActionPipesListPipes    = "pipes:ListPipes"
)

// Amazon MQ lab actions.
const (
	ActionMQCreateBroker   = "mq:CreateBroker"
	ActionMQDescribeBroker = "mq:DescribeBroker"
	ActionMQListBrokers    = "mq:ListBrokers"
	ActionMQDeleteBroker   = "mq:DeleteBroker"
)

// RDS lab actions.
const (
	ActionRDSCreateDBInstance    = "rds:CreateDBInstance"
	ActionRDSDescribeDBInstances = "rds:DescribeDBInstances"
	ActionRDSDeleteDBInstance    = "rds:DeleteDBInstance"
)

// RDS Data API lab actions.
const (
	ActionRDSDataExecuteStatement      = "rds-data:ExecuteStatement"
	ActionRDSDataBatchExecuteStatement = "rds-data:BatchExecuteStatement"
	ActionRDSDataBeginTransaction      = "rds-data:BeginTransaction"
	ActionRDSDataCommitTransaction     = "rds-data:CommitTransaction"
	ActionRDSDataRollbackTransaction   = "rds-data:RollbackTransaction"
)

// ElastiCache lab actions.
const (
	ActionElastiCacheCreateCacheCluster    = "elasticache:CreateCacheCluster"
	ActionElastiCacheDescribeCacheClusters = "elasticache:DescribeCacheClusters"
	ActionElastiCacheDeleteCacheCluster    = "elasticache:DeleteCacheCluster"
)

// DocumentDB lab actions. IAM action names use the rds: prefix (AWS DocumentDB shares RDS control-plane IAM).
const (
	ActionDocDBCreateDBCluster    = "rds:CreateDBCluster"
	ActionDocDBDescribeDBClusters = "rds:DescribeDBClusters"
	ActionDocDBDeleteDBCluster    = "rds:DeleteDBCluster"
)

// Transfer Family lab actions.
const (
	ActionTransferCreateServer   = "transfer:CreateServer"
	ActionTransferDescribeServer = "transfer:DescribeServer"
	ActionTransferListServers    = "transfer:ListServers"
	ActionTransferDeleteServer   = "transfer:DeleteServer"
	ActionTransferCreateUser     = "transfer:CreateUser"
	ActionTransferDeleteUser     = "transfer:DeleteUser"
	ActionTransferPutFile        = "transfer:PutFile"
	ActionTransferGetFile        = "transfer:GetFile"
	ActionTransferListDirectory  = "transfer:ListDirectory"
)

// SQS lab actions.
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
	ActionSQSListQueueTags           = "sqs:ListQueueTags"
	ActionSQSTagQueue                = "sqs:TagQueue"
	ActionSQSUntagQueue              = "sqs:UntagQueue"
)

// SSM lab actions.
const (
	ActionSSMPutParameter           = "ssm:PutParameter"
	ActionSSMGetParameter           = "ssm:GetParameter"
	ActionSSMGetParameters          = "ssm:GetParameters"
	ActionSSMGetParametersByPath    = "ssm:GetParametersByPath"
	ActionSSMDeleteParameter        = "ssm:DeleteParameter"
	ActionSSMDescribeParameters     = "ssm:DescribeParameters"
	ActionSSMListTagsForResource    = "ssm:ListTagsForResource"
	ActionSSMAddTagsToResource      = "ssm:AddTagsToResource"
	ActionSSMRemoveTagsFromResource = "ssm:RemoveTagsFromResource"
)

// SNS lab actions.
const (
	ActionSNSCreateTopic               = "sns:CreateTopic"
	ActionSNSDeleteTopic               = "sns:DeleteTopic"
	ActionSNSListTopics                = "sns:ListTopics"
	ActionSNSGetTopicAttributes        = "sns:GetTopicAttributes"
	ActionSNSSetTopicAttributes        = "sns:SetTopicAttributes"
	ActionSNSPublish                   = "sns:Publish"
	ActionSNSSubscribe                 = "sns:Subscribe"
	ActionSNSConfirmSubscription       = "sns:ConfirmSubscription"
	ActionSNSUnsubscribe               = "sns:Unsubscribe"
	ActionSNSListSubscriptions         = "sns:ListSubscriptions"
	ActionSNSListSubscriptionsByTopic  = "sns:ListSubscriptionsByTopic"
	ActionSNSGetSubscriptionAttributes = "sns:GetSubscriptionAttributes"
	ActionSNSAddPermission             = "sns:AddPermission"
	ActionSNSRemovePermission          = "sns:RemovePermission"
	ActionSNSListTagsForResource       = "sns:ListTagsForResource"
	ActionSNSTagResource               = "sns:TagResource"
	ActionSNSUntagResource             = "sns:UntagResource"
)

// EventBridge lab actions.
const (
	ActionEventsPutEvents           = "events:PutEvents"
	ActionEventsCreateEventBus      = "events:CreateEventBus"
	ActionEventsDeleteEventBus      = "events:DeleteEventBus"
	ActionEventsDescribeEventBus    = "events:DescribeEventBus"
	ActionEventsListEventBuses      = "events:ListEventBuses"
	ActionEventsPutRule             = "events:PutRule"
	ActionEventsDescribeRule        = "events:DescribeRule"
	ActionEventsListRules           = "events:ListRules"
	ActionEventsDeleteRule          = "events:DeleteRule"
	ActionEventsEnableRule          = "events:EnableRule"
	ActionEventsDisableRule         = "events:DisableRule"
	ActionEventsPutTargets          = "events:PutTargets"
	ActionEventsRemoveTargets       = "events:RemoveTargets"
	ActionEventsListTargetsByRule   = "events:ListTargetsByRule"
	ActionEventsPutPermission       = "events:PutPermission"
	ActionEventsRemovePermission    = "events:RemovePermission"
	ActionEventsListTagsForResource = "events:ListTagsForResource"
	ActionEventsTagResource         = "events:TagResource"
	ActionEventsUntagResource       = "events:UntagResource"
)

// ECR lab actions.
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

// ECS lab actions.
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
	ActionECSCreateService            = "ecs:CreateService"
	ActionECSUpdateService            = "ecs:UpdateService"
	ActionECSDeleteService            = "ecs:DeleteService"
	ActionECSDescribeServices         = "ecs:DescribeServices"
	ActionECSListServices             = "ecs:ListServices"
)

// Secrets Manager lab actions.
const (
	ActionSecretsCreateSecret             = "secretsmanager:CreateSecret"
	ActionSecretsGetSecretValue           = "secretsmanager:GetSecretValue"
	ActionSecretsPutSecretValue           = "secretsmanager:PutSecretValue"
	ActionSecretsDeleteSecret             = "secretsmanager:DeleteSecret"
	ActionSecretsRestoreSecret            = "secretsmanager:RestoreSecret"
	ActionSecretsRotateSecret             = "secretsmanager:RotateSecret"
	ActionSecretsUpdateSecretVersionStage = "secretsmanager:UpdateSecretVersionStage"
	ActionSecretsDescribeSecret           = "secretsmanager:DescribeSecret"
	ActionSecretsListSecrets              = "secretsmanager:ListSecrets"
	ActionSecretsPutResourcePolicy        = "secretsmanager:PutResourcePolicy"
	ActionSecretsGetResourcePolicy        = "secretsmanager:GetResourcePolicy"
	ActionSecretsDeleteResourcePolicy     = "secretsmanager:DeleteResourcePolicy"
	ActionSecretsListTagsForResource      = "secretsmanager:ListTagsForResource"
	ActionSecretsTagResource              = "secretsmanager:TagResource"
	ActionSecretsUntagResource            = "secretsmanager:UntagResource"
)

// CloudTrail lab actions.
const (
	ActionCloudTrailLookupEvents  = "cloudtrail:LookupEvents"
	ActionCloudTrailCreateTrail   = "cloudtrail:CreateTrail"
	ActionCloudTrailDescribeTrails = "cloudtrail:DescribeTrails"
	ActionCloudTrailDeleteTrail   = "cloudtrail:DeleteTrail"
	ActionCloudTrailStartLogging  = "cloudtrail:StartLogging"
	ActionCloudTrailStopLogging   = "cloudtrail:StopLogging"
)

// CloudWatch Logs lab actions.
const (
	ActionLogsCreateLogGroup              = "logs:CreateLogGroup"
	ActionLogsCreateLogStream             = "logs:CreateLogStream"
	ActionLogsDeleteLogGroup              = "logs:DeleteLogGroup"
	ActionLogsDeleteLogStream             = "logs:DeleteLogStream"
	ActionLogsDescribeLogStreams          = "logs:DescribeLogStreams"
	ActionLogsPutLogEvents                = "logs:PutLogEvents"
	ActionLogsGetLogEvents                = "logs:GetLogEvents"
	ActionLogsFilterLogEvents             = "logs:FilterLogEvents"
	ActionLogsDescribeLogGroups           = "logs:DescribeLogGroups"
	ActionLogsPutSubscriptionFilter       = "logs:PutSubscriptionFilter"
	ActionLogsDeleteSubscriptionFilter    = "logs:DeleteSubscriptionFilter"
	ActionLogsDescribeSubscriptionFilters = "logs:DescribeSubscriptionFilters"
	ActionLogsPutMetricFilter             = "logs:PutMetricFilter"
	ActionLogsDeleteMetricFilter          = "logs:DeleteMetricFilter"
	ActionLogsDescribeMetricFilters       = "logs:DescribeMetricFilters"
	ActionLogsPutResourcePolicy           = "logs:PutResourcePolicy"
	ActionLogsGetResourcePolicy           = "logs:GetResourcePolicy"
	ActionLogsDeleteResourcePolicy        = "logs:DeleteResourcePolicy"
	ActionLogsDescribeResourcePolicies    = "logs:DescribeResourcePolicies"
	ActionLogsPutRetentionPolicy          = "logs:PutRetentionPolicy"
	ActionLogsDeleteRetentionPolicy       = "logs:DeleteRetentionPolicy"
)

// Resource Groups Tagging API lab actions.
const (
	ActionTaggingTagResources   = "tag:TagResources"
	ActionTaggingUntagResources = "tag:UntagResources"
	ActionTaggingGetResources   = "tag:GetResources"
)

// Kinesis lab actions.
const (
	ActionKinesisCreateStream         = "kinesis:CreateStream"
	ActionKinesisDeleteStream         = "kinesis:DeleteStream"
	ActionKinesisDescribeStream       = "kinesis:DescribeStream"
	ActionKinesisListStreams          = "kinesis:ListStreams"
	ActionKinesisPutRecord            = "kinesis:PutRecord"
	ActionKinesisPutRecords           = "kinesis:PutRecords"
	ActionKinesisGetShardIterator     = "kinesis:GetShardIterator"
	ActionKinesisGetRecords           = "kinesis:GetRecords"
	ActionKinesisPutResourcePolicy    = "kinesis:PutResourcePolicy"
	ActionKinesisGetResourcePolicy    = "kinesis:GetResourcePolicy"
	ActionKinesisDeleteResourcePolicy = "kinesis:DeleteResourcePolicy"
)

// AppConfig lab actions.
const (
	ActionAppConfigCreateApplication                = "appconfig:CreateApplication"
	ActionAppConfigCreateEnvironment                = "appconfig:CreateEnvironment"
	ActionAppConfigCreateConfigurationProfile       = "appconfig:CreateConfigurationProfile"
	ActionAppConfigCreateHostedConfigurationVersion = "appconfig:CreateHostedConfigurationVersion"
	ActionAppConfigGetConfiguration                 = "appconfig:GetConfiguration"
	ActionAppConfigStartDeployment                  = "appconfig:StartDeployment"
	ActionAppConfigGetDeployment                    = "appconfig:GetDeployment"
	ActionAppConfigListDeployments                  = "appconfig:ListDeployments"
	ActionAppConfigDataStartConfigurationSession    = "appconfigdata:StartConfigurationSession"
	ActionAppConfigDataGetLatestConfiguration       = "appconfigdata:GetLatestConfiguration"
)

// SES lab actions.
const (
	ActionSESVerifyEmailIdentity          = "ses:VerifyEmailIdentity"
	ActionSESListIdentities               = "ses:ListIdentities"
	ActionSESSendEmail                    = "ses:SendEmail"
	ActionSESSendRawEmail                 = "ses:SendRawEmail"
	ActionSESGetSendStatistics            = "ses:GetSendStatistics"
	ActionSESSetIdentityNotificationTopic = "ses:SetIdentityNotificationTopic"
)

// Step Functions lab actions.
const (
	ActionSFNCreateStateMachine   = "states:CreateStateMachine"
	ActionSFNDeleteStateMachine   = "states:DeleteStateMachine"
	ActionSFNDescribeStateMachine = "states:DescribeStateMachine"
	ActionSFNListStateMachines    = "states:ListStateMachines"
	ActionSFNStartExecution       = "states:StartExecution"
	ActionSFNDescribeExecution    = "states:DescribeExecution"
	ActionSFNGetExecutionHistory  = "states:GetExecutionHistory"
	ActionSFNPutResourcePolicy    = "states:PutResourcePolicy"
	ActionSFNGetResourcePolicy    = "states:GetResourcePolicy"
	ActionSFNDeleteResourcePolicy = "states:DeleteResourcePolicy"
)

// CloudFormation lab actions.
const (
	ActionCFNCreateStack                       = "cloudformation:CreateStack"
	ActionCFNDescribeStacks                    = "cloudformation:DescribeStacks"
	ActionCFNDeleteStack                       = "cloudformation:DeleteStack"
	ActionCFNListStacks                        = "cloudformation:ListStacks"
	ActionCFNUpdateStack                       = "cloudformation:UpdateStack"
	ActionCFNCreateChangeSet                   = "cloudformation:CreateChangeSet"
	ActionCFNDescribeChangeSet                 = "cloudformation:DescribeChangeSet"
	ActionCFNExecuteChangeSet                  = "cloudformation:ExecuteChangeSet"
	ActionCFNDetectStackDrift                  = "cloudformation:DetectStackDrift"
	ActionCFNDescribeStackDriftDetectionStatus = "cloudformation:DescribeStackDriftDetectionStatus"
	ActionCFNDescribeStackResourceDrifts       = "cloudformation:DescribeStackResourceDrifts"
)

// CodePipeline lab actions.
const (
	ActionCodePipelineCreatePipeline         = "codepipeline:CreatePipeline"
	ActionCodePipelineGetPipeline            = "codepipeline:GetPipeline"
	ActionCodePipelineDeletePipeline         = "codepipeline:DeletePipeline"
	ActionCodePipelineStartPipelineExecution = "codepipeline:StartPipelineExecution"
	ActionCodePipelineGetPipelineState       = "codepipeline:GetPipelineState"
)

// Firehose lab actions.
const (
	ActionFirehoseCreateDeliveryStream   = "firehose:CreateDeliveryStream"
	ActionFirehoseDeleteDeliveryStream   = "firehose:DeleteDeliveryStream"
	ActionFirehoseDescribeDeliveryStream = "firehose:DescribeDeliveryStream"
	ActionFirehoseListDeliveryStreams    = "firehose:ListDeliveryStreams"
	ActionFirehosePutRecord              = "firehose:PutRecord"
	ActionFirehosePutRecordBatch         = "firehose:PutRecordBatch"
)

// Glue Data Catalog lab actions.
const (
	ActionGlueCreateDatabase = "glue:CreateDatabase"
	ActionGlueGetDatabase    = "glue:GetDatabase"
	ActionGlueGetDatabases   = "glue:GetDatabases"
	ActionGlueDeleteDatabase = "glue:DeleteDatabase"
	ActionGlueCreateTable    = "glue:CreateTable"
	ActionGlueGetTable       = "glue:GetTable"
	ActionGlueGetTables      = "glue:GetTables"
	ActionGlueDeleteTable    = "glue:DeleteTable"
	ActionGlueCreateCrawler  = "glue:CreateCrawler"
	ActionGlueStartCrawler   = "glue:StartCrawler"
	ActionGlueGetCrawler     = "glue:GetCrawler"
	ActionGlueDeleteCrawler  = "glue:DeleteCrawler"
	ActionGlueListCrawlers   = "glue:ListCrawlers"
)

// Athena lab actions.
const (
	ActionAthenaStartQueryExecution = "athena:StartQueryExecution"
	ActionAthenaGetQueryExecution   = "athena:GetQueryExecution"
	ActionAthenaGetQueryResults     = "athena:GetQueryResults"
	ActionAthenaStopQueryExecution  = "athena:StopQueryExecution"
)

// OpenSearch Service lab actions. IAM uses es: prefix.
const (
	ActionOpenSearchCreateDomain    = "es:CreateDomain"
	ActionOpenSearchDescribeDomain  = "es:DescribeDomain"
	ActionOpenSearchListDomainNames = "es:ListDomainNames"
	ActionOpenSearchDeleteDomain    = "es:DeleteDomain"
)

// WAFv2 lab actions.
const (
	ActionWAFCreateWebACL    = "wafv2:CreateWebACL"
	ActionWAFUpdateWebACL    = "wafv2:UpdateWebACL"
	ActionWAFGetWebACL       = "wafv2:GetWebACL"
	ActionWAFListWebACLs     = "wafv2:ListWebACLs"
	ActionWAFCreateRuleGroup = "wafv2:CreateRuleGroup"
	ActionWAFAssociateWebACL = "wafv2:AssociateWebACL"
	ActionWAFEvaluate        = "wafv2:Evaluate"
)

// AWS Config lab actions.
const (
	ActionConfigPutConfigurationRecorder       = "config:PutConfigurationRecorder"
	ActionConfigPutDeliveryChannel             = "config:PutDeliveryChannel"
	ActionConfigStartConfigurationRecorder     = "config:StartConfigurationRecorder"
	ActionConfigDescribeComplianceByConfigRule = "config:DescribeComplianceByConfigRule"
)

// EventBridge Scheduler lab actions.
const (
	ActionSchedulerCreateSchedule = "scheduler:CreateSchedule"
	ActionSchedulerGetSchedule    = "scheduler:GetSchedule"
	ActionSchedulerUpdateSchedule = "scheduler:UpdateSchedule"
	ActionSchedulerDeleteSchedule = "scheduler:DeleteSchedule"
	ActionSchedulerListSchedules  = "scheduler:ListSchedules"
)

// ACM lab actions.
const (
	ActionACMRequestCertificate  = "acm:RequestCertificate"
	ActionACMDescribeCertificate = "acm:DescribeCertificate"
	ActionACMListCertificates    = "acm:ListCertificates"
	ActionACMDeleteCertificate   = "acm:DeleteCertificate"
)

// Route 53 lab actions.
const (
	ActionRoute53CreateHostedZone         = "route53:CreateHostedZone"
	ActionRoute53DeleteHostedZone         = "route53:DeleteHostedZone"
	ActionRoute53ListHostedZones          = "route53:ListHostedZones"
	ActionRoute53ChangeResourceRecordSets = "route53:ChangeResourceRecordSets"
	ActionRoute53ListResourceRecordSets   = "route53:ListResourceRecordSets"
)

// Cloud Map (Service Discovery) lab actions.
const (
	ActionSDCreatePrivateDnsNamespace = "servicediscovery:CreatePrivateDnsNamespace"
	ActionSDCreateHttpNamespace       = "servicediscovery:CreateHttpNamespace"
	ActionSDCreateService             = "servicediscovery:CreateService"
	ActionSDRegisterInstance          = "servicediscovery:RegisterInstance"
	ActionSDDeregisterInstance        = "servicediscovery:DeregisterInstance"
	ActionSDDiscoverInstances         = "servicediscovery:DiscoverInstances"
)

// Pricing lab actions.
const (
	ActionPricingDescribeServices   = "pricing:DescribeServices"
	ActionPricingGetAttributeValues = "pricing:GetAttributeValues"
	ActionPricingGetProducts        = "pricing:GetProducts"
)

// AppSync lab actions. Cognito User Pools auth included.
const (
	ActionAppSyncCreateGraphqlApi    = "appsync:CreateGraphqlApi"
	ActionAppSyncDeleteGraphqlApi    = "appsync:DeleteGraphqlApi"
	ActionAppSyncGetGraphqlApi       = "appsync:GetGraphqlApi"
	ActionAppSyncListGraphqlApis     = "appsync:ListGraphqlApis"
	ActionAppSyncStartSchemaCreation = "appsync:StartSchemaCreation"
	ActionAppSyncCreateApiKey        = "appsync:CreateApiKey"
	ActionAppSyncCreateDataSource    = "appsync:CreateDataSource"
	ActionAppSyncCreateResolver      = "appsync:CreateResolver"
	ActionAppSyncGraphQL             = "appsync:GraphQL"
)

// API Gateway HTTP API (v2) lab actions.
const (
	ActionAPIGatewayV2CreateApi         = "apigatewayv2:CreateApi"
	ActionAPIGatewayV2GetApi            = "apigatewayv2:GetApi"
	ActionAPIGatewayV2UpdateApi         = "apigatewayv2:UpdateApi"
	ActionAPIGatewayV2DeleteApi         = "apigatewayv2:DeleteApi"
	ActionAPIGatewayV2GetApis           = "apigatewayv2:GetApis"
	ActionAPIGatewayV2CreateIntegration = "apigatewayv2:CreateIntegration"
	ActionAPIGatewayV2GetIntegrations   = "apigatewayv2:GetIntegrations"
	ActionAPIGatewayV2CreateAuthorizer  = "apigatewayv2:CreateAuthorizer"
	ActionAPIGatewayV2GetAuthorizers    = "apigatewayv2:GetAuthorizers"
	ActionAPIGatewayV2CreateRoute       = "apigatewayv2:CreateRoute"
	ActionAPIGatewayV2GetRoutes         = "apigatewayv2:GetRoutes"
	ActionAPIGatewayV2CreateStage       = "apigatewayv2:CreateStage"
)

// execute-api invoke (HTTP API IAM authorizer). Not an HTTP API resource policy.
const ActionExecuteAPIInvoke = "execute-api:Invoke"

// Cognito User Pools lab actions.
const (
	ActionCognitoCreateUserPool         = "cognito-idp:CreateUserPool"
	ActionCognitoDescribeUserPool       = "cognito-idp:DescribeUserPool"
	ActionCognitoUpdateUserPool         = "cognito-idp:UpdateUserPool"
	ActionCognitoListUserPools          = "cognito-idp:ListUserPools"
	ActionCognitoDeleteUserPool         = "cognito-idp:DeleteUserPool"
	ActionCognitoCreateUserPoolClient   = "cognito-idp:CreateUserPoolClient"
	ActionCognitoDescribeUserPoolClient = "cognito-idp:DescribeUserPoolClient"
	ActionCognitoListUserPoolClients    = "cognito-idp:ListUserPoolClients"
	ActionCognitoDeleteUserPoolClient   = "cognito-idp:DeleteUserPoolClient"
	ActionCognitoAdminCreateUser        = "cognito-idp:AdminCreateUser"
	ActionCognitoSignUp                 = "cognito-idp:SignUp"
	ActionCognitoConfirmSignUp          = "cognito-idp:ConfirmSignUp"
	ActionCognitoInitiateAuth           = "cognito-idp:InitiateAuth"
	ActionCognitoAdminInitiateAuth      = "cognito-idp:AdminInitiateAuth"
	ActionCognitoRevokeToken            = "cognito-idp:RevokeToken"
	ActionCognitoAssociateSoftwareToken = "cognito-idp:AssociateSoftwareToken"
	ActionCognitoVerifySoftwareToken    = "cognito-idp:VerifySoftwareToken"
	ActionCognitoRespondToAuthChallenge = "cognito-idp:RespondToAuthChallenge"
)

// Cloud Control API lab actions.
const (
	ActionCloudControlCreateResource           = "cloudcontrol:CreateResource"
	ActionCloudControlGetResource              = "cloudcontrol:GetResource"
	ActionCloudControlListResources            = "cloudcontrol:ListResources"
	ActionCloudControlDeleteResource           = "cloudcontrol:DeleteResource"
	ActionCloudControlUpdateResource           = "cloudcontrol:UpdateResource"
	ActionCloudControlGetResourceRequestStatus = "cloudcontrol:GetResourceRequestStatus"
)

// BCM Data Exports lab actions.
const (
	ActionBCMCreateExport = "bcm-data-exports:CreateExport"
	ActionBCMGetExport    = "bcm-data-exports:GetExport"
	ActionBCMListExports  = "bcm-data-exports:ListExports"
	ActionBCMDeleteExport = "bcm-data-exports:DeleteExport"
)

// Cost Explorer lab actions.
const (
	ActionCEGetCostAndUsage = "ce:GetCostAndUsage"
	ActionCEGetCostForecast = "ce:GetCostForecast"
)

// Budgets lab actions.
const (
	ActionBudgetsCreateBudget    = "budgets:CreateBudget"
	ActionBudgetsDescribeBudget  = "budgets:DescribeBudget"
	ActionBudgetsDescribeBudgets = "budgets:DescribeBudgets"
	ActionBudgetsDeleteBudget    = "budgets:DeleteBudget"
)

// CodeDeploy lab actions.
const (
	ActionCodeDeployCreateApplication     = "codedeploy:CreateApplication"
	ActionCodeDeployCreateDeploymentGroup = "codedeploy:CreateDeploymentGroup"
	ActionCodeDeployCreateDeployment      = "codedeploy:CreateDeployment"
	ActionCodeDeployGetDeployment         = "codedeploy:GetDeployment"
	ActionCodeDeployListDeployments       = "codedeploy:ListDeployments"
)

// CloudFront lab actions.
const (
	ActionCloudFrontCreateDistribution = "cloudfront:CreateDistribution"
	ActionCloudFrontGetDistribution    = "cloudfront:GetDistribution"
	ActionCloudFrontListDistributions  = "cloudfront:ListDistributions"
	ActionCloudFrontDeleteDistribution = "cloudfront:DeleteDistribution"
)

// Elastic Load Balancing v2 lab actions.
const (
	ActionELBv2CreateLoadBalancer    = "elasticloadbalancing:CreateLoadBalancer"
	ActionELBv2DescribeLoadBalancers = "elasticloadbalancing:DescribeLoadBalancers"
	ActionELBv2DeleteLoadBalancer    = "elasticloadbalancing:DeleteLoadBalancer"
	ActionELBv2CreateTargetGroup     = "elasticloadbalancing:CreateTargetGroup"
	ActionELBv2DescribeTargetGroups  = "elasticloadbalancing:DescribeTargetGroups"
	ActionELBv2DeleteTargetGroup     = "elasticloadbalancing:DeleteTargetGroup"
	ActionELBv2CreateListener        = "elasticloadbalancing:CreateListener"
	ActionELBv2DescribeListeners     = "elasticloadbalancing:DescribeListeners"
	ActionELBv2DeleteListener        = "elasticloadbalancing:DeleteListener"
	ActionELBv2RegisterTargets       = "elasticloadbalancing:RegisterTargets"
	ActionELBv2DescribeTargetHealth  = "elasticloadbalancing:DescribeTargetHealth"
	ActionELBv2CreateRule            = "elasticloadbalancing:CreateRule"
	ActionELBv2DescribeRules         = "elasticloadbalancing:DescribeRules"
	ActionELBv2DeleteRule            = "elasticloadbalancing:DeleteRule"
)

// S3 Vectors lab actions.
const (
	ActionS3VectorsCreateVectorBucket = "s3vectors:CreateVectorBucket"
	ActionS3VectorsListVectorBuckets  = "s3vectors:ListVectorBuckets"
	ActionS3VectorsDeleteVectorBucket = "s3vectors:DeleteVectorBucket"
	ActionS3VectorsCreateIndex        = "s3vectors:CreateIndex"
	ActionS3VectorsListIndexes        = "s3vectors:ListIndexes"
	ActionS3VectorsDeleteIndex        = "s3vectors:DeleteIndex"
	ActionS3VectorsPutVectors         = "s3vectors:PutVectors"
	ActionS3VectorsQueryVectors       = "s3vectors:QueryVectors"
)

// Bedrock Runtime lab actions. IAM action prefix is bedrock:.
const (
	ActionBedrockInvokeModel = "bedrock:InvokeModel"
)

// Textract lab actions.
const (
	ActionTextractDetectDocumentText = "textract:DetectDocumentText"
	ActionTextractAnalyzeDocument    = "textract:AnalyzeDocument"
)

// Transcribe lab actions.
const (
	ActionTranscribeStartTranscriptionJob = "transcribe:StartTranscriptionJob"
	ActionTranscribeGetTranscriptionJob   = "transcribe:GetTranscriptionJob"
	ActionTranscribeListTranscriptionJobs = "transcribe:ListTranscriptionJobs"
)

// EMR (Elastic MapReduce) lab actions.
const (
	ActionEMRRunJobFlow        = "elasticmapreduce:RunJobFlow"
	ActionEMRDescribeCluster   = "elasticmapreduce:DescribeCluster"
	ActionEMRListClusters      = "elasticmapreduce:ListClusters"
	ActionEMRTerminateJobFlows = "elasticmapreduce:TerminateJobFlows"
)

// CodeBuild lab actions.
const (
	ActionCodeBuildCreateProject  = "codebuild:CreateProject"
	ActionCodeBuildStartBuild     = "codebuild:StartBuild"
	ActionCodeBuildBatchGetBuilds = "codebuild:BatchGetBuilds"
	ActionCodeBuildListBuilds     = "codebuild:ListBuilds"
)

// Batch lab actions.
const (
	ActionBatchCreateComputeEnvironment    = "batch:CreateComputeEnvironment"
	ActionBatchCreateJobQueue              = "batch:CreateJobQueue"
	ActionBatchRegisterJobDefinition       = "batch:RegisterJobDefinition"
	ActionBatchSubmitJob                   = "batch:SubmitJob"
	ActionBatchDescribeComputeEnvironments = "batch:DescribeComputeEnvironments"
	ActionBatchDescribeJobQueues           = "batch:DescribeJobQueues"
	ActionBatchDescribeJobDefinitions      = "batch:DescribeJobDefinitions"
	ActionBatchDescribeJobs                = "batch:DescribeJobs"
)

// Lambda lab actions.
const (
	ActionLambdaCreateFunction                  = "lambda:CreateFunction"
	ActionLambdaGetFunction                     = "lambda:GetFunction"
	ActionLambdaDeleteFunction                  = "lambda:DeleteFunction"
	ActionLambdaListFunctions                   = "lambda:ListFunctions"
	ActionLambdaUpdateFunctionCode              = "lambda:UpdateFunctionCode"
	ActionLambdaUpdateFunctionConfiguration     = "lambda:UpdateFunctionConfiguration"
	ActionLambdaInvoke                          = "lambda:InvokeFunction"
	ActionLambdaPublishVersion                  = "lambda:PublishVersion"
	ActionLambdaListVersionsByFunction          = "lambda:ListVersionsByFunction"
	ActionLambdaCreateAlias                     = "lambda:CreateAlias"
	ActionLambdaUpdateAlias                     = "lambda:UpdateAlias"
	ActionLambdaDeleteAlias                     = "lambda:DeleteAlias"
	ActionLambdaGetAlias                        = "lambda:GetAlias"
	ActionLambdaListAliases                     = "lambda:ListAliases"
	ActionLambdaPublishLayerVersion             = "lambda:PublishLayerVersion"
	ActionLambdaGetLayerVersion                 = "lambda:GetLayerVersion"
	ActionLambdaListLayerVersions               = "lambda:ListLayerVersions"
	ActionLambdaDeleteLayerVersion              = "lambda:DeleteLayerVersion"
	ActionLambdaAddPermission                   = "lambda:AddPermission"
	ActionLambdaRemovePermission                = "lambda:RemovePermission"
	ActionLambdaGetPolicy                       = "lambda:GetPolicy"
	ActionLambdaCreateEventSourceMapping        = "lambda:CreateEventSourceMapping"
	ActionLambdaGetEventSourceMapping           = "lambda:GetEventSourceMapping"
	ActionLambdaListEventSourceMappings         = "lambda:ListEventSourceMappings"
	ActionLambdaUpdateEventSourceMapping        = "lambda:UpdateEventSourceMapping"
	ActionLambdaDeleteEventSourceMapping        = "lambda:DeleteEventSourceMapping"
	ActionLambdaCreateFunctionUrlConfig         = "lambda:CreateFunctionUrlConfig"
	ActionLambdaGetFunctionUrlConfig            = "lambda:GetFunctionUrlConfig"
	ActionLambdaDeleteFunctionUrlConfig         = "lambda:DeleteFunctionUrlConfig"
	ActionLambdaListFunctionUrlConfigs          = "lambda:ListFunctionUrlConfigs"
	ActionLambdaInvokeFunctionUrl               = "lambda:InvokeFunctionUrl"
	ActionLambdaListTags                        = "lambda:ListTags"
	ActionLambdaGetFunctionCodeSigningConfig    = "lambda:GetFunctionCodeSigningConfig"
	ActionLambdaPutFunctionEventInvokeConfig    = "lambda:PutFunctionEventInvokeConfig"
	ActionLambdaGetFunctionEventInvokeConfig    = "lambda:GetFunctionEventInvokeConfig"
	ActionLambdaDeleteFunctionEventInvokeConfig = "lambda:DeleteFunctionEventInvokeConfig"
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
		ActionIAMCreatePolicyVersion,
		ActionIAMGetPolicyVersion,
		ActionIAMListPolicyVersions,
		ActionIAMDeletePolicyVersion,
		ActionIAMSetDefaultPolicyVersion,
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
		ActionIAMListInstanceProfilesForRole,
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
		ActionKMSListResourceTags,
		ActionKMSTagResource,
		ActionKMSUntagResource,
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
		ActionS3PutBucketVersioning,
		ActionS3GetBucketVersioning,
		ActionS3ListBucketVersions,
		ActionS3PutBucketNotification,
		ActionS3GetBucketNotification,
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
		ActionDynamoDBTransactGetItems,
		ActionDynamoDBTransactWriteItems,
		ActionDynamoDBConditionCheckItem,
		ActionDynamoDBPutResourcePolicy,
		ActionDynamoDBGetResourcePolicy,
		ActionDynamoDBDeleteResourcePolicy,
		ActionDynamoDBUpdateTimeToLive,
		ActionDynamoDBDescribeTimeToLive,
		ActionDynamoDBDescribeContinuousBackups,
		ActionDynamoDBListTagsOfResource,
		ActionDynamoDBTagResource,
		ActionDynamoDBUntagResource,
		ActionDynamoDBStreamsListStreams,
		ActionDynamoDBStreamsDescribeStream,
		ActionDynamoDBStreamsGetShardIterator,
		ActionDynamoDBStreamsGetRecords,
		ActionPipesCreatePipe,
		ActionPipesDescribePipe,
		ActionPipesDeletePipe,
		ActionPipesListPipes,
		ActionMQCreateBroker,
		ActionMQDescribeBroker,
		ActionMQListBrokers,
		ActionMQDeleteBroker,
		ActionRDSCreateDBInstance,
		ActionRDSDescribeDBInstances,
		ActionRDSDeleteDBInstance,
		ActionRDSDataExecuteStatement,
		ActionRDSDataBatchExecuteStatement,
		ActionRDSDataBeginTransaction,
		ActionRDSDataCommitTransaction,
		ActionRDSDataRollbackTransaction,
		ActionElastiCacheCreateCacheCluster,
		ActionElastiCacheDescribeCacheClusters,
		ActionElastiCacheDeleteCacheCluster,
		ActionDocDBCreateDBCluster,
		ActionDocDBDescribeDBClusters,
		ActionDocDBDeleteDBCluster,
		ActionTransferCreateServer,
		ActionTransferDescribeServer,
		ActionTransferListServers,
		ActionTransferDeleteServer,
		ActionTransferCreateUser,
		ActionTransferDeleteUser,
		ActionTransferPutFile,
		ActionTransferGetFile,
		ActionTransferListDirectory,
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
		ActionSQSListQueueTags,
		ActionSQSTagQueue,
		ActionSQSUntagQueue,
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
		ActionLambdaCreateEventSourceMapping,
		ActionLambdaGetEventSourceMapping,
		ActionLambdaListEventSourceMappings,
		ActionLambdaUpdateEventSourceMapping,
		ActionLambdaDeleteEventSourceMapping,
		ActionLambdaCreateFunctionUrlConfig,
		ActionLambdaGetFunctionUrlConfig,
		ActionLambdaDeleteFunctionUrlConfig,
		ActionLambdaListFunctionUrlConfigs,
		ActionLambdaInvokeFunctionUrl,
		ActionLambdaListTags,
		ActionLambdaGetFunctionCodeSigningConfig,
		ActionLambdaPutFunctionEventInvokeConfig,
		ActionLambdaGetFunctionEventInvokeConfig,
		ActionLambdaDeleteFunctionEventInvokeConfig,
		ActionSSMPutParameter,
		ActionSSMGetParameter,
		ActionSSMGetParameters,
		ActionSSMGetParametersByPath,
		ActionSSMDeleteParameter,
		ActionSSMDescribeParameters,
		ActionSSMListTagsForResource,
		ActionSSMAddTagsToResource,
		ActionSSMRemoveTagsFromResource,
		ActionSecretsCreateSecret,
		ActionSecretsGetSecretValue,
		ActionSecretsPutSecretValue,
		ActionSecretsDeleteSecret,
		ActionSecretsRestoreSecret,
		ActionSecretsRotateSecret,
		ActionSecretsUpdateSecretVersionStage,
		ActionSecretsDescribeSecret,
		ActionSecretsListSecrets,
		ActionSecretsPutResourcePolicy,
		ActionSecretsGetResourcePolicy,
		ActionSecretsDeleteResourcePolicy,
		ActionSecretsListTagsForResource,
		ActionSecretsTagResource,
		ActionSecretsUntagResource,
		ActionSNSCreateTopic,
		ActionSNSDeleteTopic,
		ActionSNSListTopics,
		ActionSNSGetTopicAttributes,
		ActionSNSSetTopicAttributes,
		ActionSNSPublish,
		ActionSNSSubscribe,
		ActionSNSConfirmSubscription,
		ActionSNSUnsubscribe,
		ActionSNSListSubscriptions,
		ActionSNSListSubscriptionsByTopic,
		ActionSNSGetSubscriptionAttributes,
		ActionSNSAddPermission,
		ActionSNSRemovePermission,
		ActionSNSListTagsForResource,
		ActionSNSTagResource,
		ActionSNSUntagResource,
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
		ActionEventsPutPermission,
		ActionEventsRemovePermission,
		ActionEventsListTagsForResource,
		ActionEventsTagResource,
		ActionEventsUntagResource,
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
		ActionECSCreateService,
		ActionECSUpdateService,
		ActionECSDeleteService,
		ActionECSDescribeServices,
		ActionECSListServices,
		ActionCloudTrailLookupEvents,
		ActionCloudTrailCreateTrail,
		ActionCloudTrailDescribeTrails,
		ActionCloudTrailDeleteTrail,
		ActionCloudTrailStartLogging,
		ActionCloudTrailStopLogging,
		ActionLogsCreateLogGroup,
		ActionLogsCreateLogStream,
		ActionLogsDeleteLogGroup,
		ActionLogsDeleteLogStream,
		ActionLogsDescribeLogStreams,
		ActionLogsPutLogEvents,
		ActionLogsGetLogEvents,
		ActionLogsFilterLogEvents,
		ActionLogsDescribeLogGroups,
		ActionLogsPutSubscriptionFilter,
		ActionLogsDeleteSubscriptionFilter,
		ActionLogsDescribeSubscriptionFilters,
		ActionLogsPutMetricFilter,
		ActionLogsDeleteMetricFilter,
		ActionLogsDescribeMetricFilters,
		ActionLogsPutResourcePolicy,
		ActionLogsGetResourcePolicy,
		ActionLogsDeleteResourcePolicy,
		ActionLogsDescribeResourcePolicies,
		ActionLogsPutRetentionPolicy,
		ActionLogsDeleteRetentionPolicy,
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
		ActionKinesisPutResourcePolicy,
		ActionKinesisGetResourcePolicy,
		ActionKinesisDeleteResourcePolicy,
		ActionAppConfigCreateApplication,
		ActionAppConfigCreateEnvironment,
		ActionAppConfigCreateConfigurationProfile,
		ActionAppConfigCreateHostedConfigurationVersion,
		ActionAppConfigGetConfiguration,
		ActionAppConfigStartDeployment,
		ActionAppConfigGetDeployment,
		ActionAppConfigListDeployments,
		ActionAppConfigDataStartConfigurationSession,
		ActionAppConfigDataGetLatestConfiguration,
		ActionSESVerifyEmailIdentity,
		ActionSESListIdentities,
		ActionSESSendEmail,
		ActionSESSendRawEmail,
		ActionSESGetSendStatistics,
		ActionSESSetIdentityNotificationTopic,
		ActionSFNCreateStateMachine,
		ActionSFNDeleteStateMachine,
		ActionSFNDescribeStateMachine,
		ActionSFNListStateMachines,
		ActionSFNStartExecution,
		ActionSFNDescribeExecution,
		ActionSFNGetExecutionHistory,
		ActionSFNPutResourcePolicy,
		ActionSFNGetResourcePolicy,
		ActionSFNDeleteResourcePolicy,
		ActionCodeBuildCreateProject,
		ActionCodeBuildStartBuild,
		ActionCodeBuildBatchGetBuilds,
		ActionCodeBuildListBuilds,
		ActionBatchCreateComputeEnvironment,
		ActionBatchCreateJobQueue,
		ActionBatchRegisterJobDefinition,
		ActionBatchSubmitJob,
		ActionBatchDescribeComputeEnvironments,
		ActionBatchDescribeJobQueues,
		ActionBatchDescribeJobDefinitions,
		ActionBatchDescribeJobs,
		ActionCFNCreateStack,
		ActionCFNDescribeStacks,
		ActionCFNDeleteStack,
		ActionCFNListStacks,
		ActionCFNUpdateStack,
		ActionCFNCreateChangeSet,
		ActionCFNDescribeChangeSet,
		ActionCFNExecuteChangeSet,
		ActionCFNDetectStackDrift,
		ActionCFNDescribeStackDriftDetectionStatus,
		ActionCFNDescribeStackResourceDrifts,
		ActionCodePipelineCreatePipeline,
		ActionCodePipelineGetPipeline,
		ActionCodePipelineDeletePipeline,
		ActionCodePipelineStartPipelineExecution,
		ActionCodePipelineGetPipelineState,
		ActionFirehoseCreateDeliveryStream,
		ActionFirehoseDeleteDeliveryStream,
		ActionFirehoseDescribeDeliveryStream,
		ActionFirehoseListDeliveryStreams,
		ActionFirehosePutRecord,
		ActionFirehosePutRecordBatch,
		ActionGlueCreateDatabase,
		ActionGlueGetDatabase,
		ActionGlueGetDatabases,
		ActionGlueDeleteDatabase,
		ActionGlueCreateTable,
		ActionGlueGetTable,
		ActionGlueGetTables,
		ActionGlueDeleteTable,
		ActionGlueCreateCrawler,
		ActionGlueStartCrawler,
		ActionGlueGetCrawler,
		ActionGlueDeleteCrawler,
		ActionGlueListCrawlers,
		ActionAthenaStartQueryExecution,
		ActionAthenaGetQueryExecution,
		ActionAthenaGetQueryResults,
		ActionAthenaStopQueryExecution,
		ActionOpenSearchCreateDomain,
		ActionOpenSearchDescribeDomain,
		ActionOpenSearchListDomainNames,
		ActionOpenSearchDeleteDomain,
		ActionWAFCreateWebACL,
		ActionWAFUpdateWebACL,
		ActionWAFGetWebACL,
		ActionWAFListWebACLs,
		ActionWAFCreateRuleGroup,
		ActionWAFAssociateWebACL,
		ActionWAFEvaluate,
		ActionConfigPutConfigurationRecorder,
		ActionConfigPutDeliveryChannel,
		ActionConfigStartConfigurationRecorder,
		ActionConfigDescribeComplianceByConfigRule,
		ActionSchedulerCreateSchedule,
		ActionSchedulerGetSchedule,
		ActionSchedulerUpdateSchedule,
		ActionSchedulerDeleteSchedule,
		ActionSchedulerListSchedules,
		ActionACMRequestCertificate,
		ActionACMDescribeCertificate,
		ActionACMListCertificates,
		ActionACMDeleteCertificate,
		ActionRoute53CreateHostedZone,
		ActionRoute53DeleteHostedZone,
		ActionRoute53ListHostedZones,
		ActionRoute53ChangeResourceRecordSets,
		ActionRoute53ListResourceRecordSets,
		ActionSDCreatePrivateDnsNamespace,
		ActionSDCreateHttpNamespace,
		ActionSDCreateService,
		ActionSDRegisterInstance,
		ActionSDDeregisterInstance,
		ActionSDDiscoverInstances,
		ActionPricingDescribeServices,
		ActionPricingGetAttributeValues,
		ActionPricingGetProducts,
		ActionAppSyncCreateGraphqlApi,
		ActionAppSyncDeleteGraphqlApi,
		ActionAppSyncGetGraphqlApi,
		ActionAppSyncListGraphqlApis,
		ActionAppSyncStartSchemaCreation,
		ActionAppSyncCreateApiKey,
		ActionAppSyncCreateDataSource,
		ActionAppSyncCreateResolver,
		ActionAppSyncGraphQL,
		ActionAPIGatewayV2CreateApi,
		ActionAPIGatewayV2GetApi,
		ActionAPIGatewayV2UpdateApi,
		ActionAPIGatewayV2DeleteApi,
		ActionAPIGatewayV2GetApis,
		ActionAPIGatewayV2CreateIntegration,
		ActionAPIGatewayV2GetIntegrations,
		ActionAPIGatewayV2CreateAuthorizer,
		ActionAPIGatewayV2GetAuthorizers,
		ActionAPIGatewayV2CreateRoute,
		ActionAPIGatewayV2GetRoutes,
		ActionAPIGatewayV2CreateStage,
		ActionExecuteAPIInvoke,
		ActionCognitoCreateUserPool,
		ActionCognitoDescribeUserPool,
		ActionCognitoUpdateUserPool,
		ActionCognitoListUserPools,
		ActionCognitoDeleteUserPool,
		ActionCognitoCreateUserPoolClient,
		ActionCognitoDescribeUserPoolClient,
		ActionCognitoListUserPoolClients,
		ActionCognitoDeleteUserPoolClient,
		ActionCognitoAdminCreateUser,
		ActionCognitoSignUp,
		ActionCognitoConfirmSignUp,
		ActionCognitoInitiateAuth,
		ActionCognitoAdminInitiateAuth,
		ActionCognitoRevokeToken,
		ActionCognitoAssociateSoftwareToken,
		ActionCognitoVerifySoftwareToken,
		ActionCognitoRespondToAuthChallenge,
		ActionCloudControlCreateResource,
		ActionCloudControlGetResource,
		ActionCloudControlListResources,
		ActionCloudControlDeleteResource,
		ActionCloudControlUpdateResource,
		ActionCloudControlGetResourceRequestStatus,
		ActionBCMCreateExport,
		ActionBCMGetExport,
		ActionBCMListExports,
		ActionBCMDeleteExport,
		ActionCEGetCostAndUsage,
		ActionCEGetCostForecast,
		ActionBudgetsCreateBudget,
		ActionBudgetsDescribeBudget,
		ActionBudgetsDescribeBudgets,
		ActionBudgetsDeleteBudget,
		ActionCodeDeployCreateApplication,
		ActionCodeDeployCreateDeploymentGroup,
		ActionCodeDeployCreateDeployment,
		ActionCodeDeployGetDeployment,
		ActionCodeDeployListDeployments,
		ActionCloudFrontCreateDistribution,
		ActionCloudFrontGetDistribution,
		ActionCloudFrontListDistributions,
		ActionCloudFrontDeleteDistribution,
		ActionELBv2CreateLoadBalancer,
		ActionELBv2DescribeLoadBalancers,
		ActionELBv2DeleteLoadBalancer,
		ActionELBv2CreateTargetGroup,
		ActionELBv2DescribeTargetGroups,
		ActionELBv2DeleteTargetGroup,
		ActionELBv2CreateListener,
		ActionELBv2DescribeListeners,
		ActionELBv2DeleteListener,
		ActionELBv2RegisterTargets,
		ActionELBv2DescribeTargetHealth,
		ActionELBv2CreateRule,
		ActionELBv2DescribeRules,
		ActionELBv2DeleteRule,
		ActionS3VectorsCreateVectorBucket,
		ActionS3VectorsListVectorBuckets,
		ActionS3VectorsDeleteVectorBucket,
		ActionS3VectorsCreateIndex,
		ActionS3VectorsListIndexes,
		ActionS3VectorsDeleteIndex,
		ActionS3VectorsPutVectors,
		ActionS3VectorsQueryVectors,
		ActionBedrockInvokeModel,
		ActionTextractDetectDocumentText,
		ActionTextractAnalyzeDocument,
		ActionTranscribeStartTranscriptionJob,
		ActionTranscribeGetTranscriptionJob,
		ActionTranscribeListTranscriptionJobs,
		ActionEMRRunJobFlow,
		ActionEMRDescribeCluster,
		ActionEMRListClusters,
		ActionEMRTerminateJobFlows:
		return true
	default:
		return false
	}
}
