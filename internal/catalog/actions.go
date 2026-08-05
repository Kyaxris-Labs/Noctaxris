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
	ActionOrgsListPolicies                     = "organizations:ListPolicies"
	ActionOrgsListPoliciesForTarget            = "organizations:ListPoliciesForTarget"
	ActionOrgsListParents                      = "organizations:ListParents"
	ActionOrgsListAccountsForParent            = "organizations:ListAccountsForParent"
	ActionOrgsMoveAccount                      = "organizations:MoveAccount"
)

// Control Tower honest stubs (no landing zones in lab).
const (
	ActionControlTowerListLandingZones = "controltower:ListLandingZones"
	ActionControlTowerGetLandingZone   = "controltower:GetLandingZone"
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
	ActionIAMCreateAccessKey          = "iam:CreateAccessKey"
	ActionIAMDeleteAccessKey          = "iam:DeleteAccessKey"
	ActionIAMListAccessKeys           = "iam:ListAccessKeys"
	ActionIAMUpdateAccessKey          = "iam:UpdateAccessKey"
	ActionIAMGetAccessKeyLastUsed     = "iam:GetAccessKeyLastUsed"
	ActionIAMGenerateCredentialReport = "iam:GenerateCredentialReport"
	ActionIAMGetCredentialReport      = "iam:GetCredentialReport"
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
	ActionKMSSign                            = "kms:Sign"
	ActionKMSVerify                          = "kms:Verify"
	ActionKMSGetPublicKey                    = "kms:GetPublicKey"
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
	ActionS3PutBucketLogging              = "s3:PutBucketLogging"
	ActionS3GetBucketLogging              = "s3:GetBucketLogging"
	ActionS3PutBucketCors                 = "s3:PutBucketCors"
	ActionS3GetBucketCors                 = "s3:GetBucketCors"
	ActionS3DeleteBucketCors              = "s3:DeleteBucketCors"
	ActionS3PutLifecycleConfiguration     = "s3:PutLifecycleConfiguration"
	ActionS3GetLifecycleConfiguration     = "s3:GetLifecycleConfiguration"
	ActionS3DeleteLifecycleConfiguration  = "s3:DeleteLifecycleConfiguration"
	ActionS3SelectObjectContent           = "s3:SelectObjectContent"
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
	ActionDynamoDBExecuteStatement          = "dynamodb:ExecuteStatement"
	ActionDynamoDBBatchExecuteStatement     = "dynamodb:BatchExecuteStatement"
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

// MemoryDB lab actions (JSON 1.1 AmazonMemoryDB.*).
const (
	ActionMemoryDBCreateCluster    = "memorydb:CreateCluster"
	ActionMemoryDBDescribeClusters = "memorydb:DescribeClusters"
	ActionMemoryDBDeleteCluster    = "memorydb:DeleteCluster"
	ActionMemoryDBDescribeUsers    = "memorydb:DescribeUsers"
	ActionMemoryDBDescribeACLs     = "memorydb:DescribeACLs"
)

// DocumentDB lab actions. IAM action names use the rds: prefix (AWS DocumentDB shares RDS control-plane IAM).
const (
	ActionDocDBCreateDBCluster    = "rds:CreateDBCluster"
	ActionDocDBDescribeDBClusters = "rds:DescribeDBClusters"
	ActionDocDBDeleteDBCluster    = "rds:DeleteDBCluster"
)

// Neptune lab actions. Control-plane IAM uses the rds: prefix (AWS Neptune shares RDS cluster APIs).
// Catalog strings use neptune: so they stay distinct from DocumentDB routing constants.
const (
	ActionNeptuneCreateDBCluster    = "neptune:CreateDBCluster"
	ActionNeptuneDescribeDBClusters = "neptune:DescribeDBClusters"
	ActionNeptuneDeleteDBCluster    = "neptune:DeleteDBCluster"
)

// Amazon MSK lab actions.
const (
	ActionMSKCreateCluster       = "kafka:CreateCluster"
	ActionMSKDescribeCluster     = "kafka:DescribeCluster"
	ActionMSKListClusters        = "kafka:ListClusters"
	ActionMSKDeleteCluster       = "kafka:DeleteCluster"
	ActionMSKGetBootstrapBrokers = "kafka:GetBootstrapBrokers"
)

// Amazon EKS lab actions (REST-JSON control plane; metadata-only ACTIVE).
const (
	ActionEKSCreateCluster   = "eks:CreateCluster"
	ActionEKSDescribeCluster = "eks:DescribeCluster"
	ActionEKSListClusters    = "eks:ListClusters"
	ActionEKSDeleteCluster   = "eks:DeleteCluster"
	ActionEKSListNodegroups  = "eks:ListNodegroups"
)

// Transfer Family lab actions.
const (
	ActionTransferCreateServer       = "transfer:CreateServer"
	ActionTransferDescribeServer     = "transfer:DescribeServer"
	ActionTransferListServers        = "transfer:ListServers"
	ActionTransferDeleteServer       = "transfer:DeleteServer"
	ActionTransferCreateUser         = "transfer:CreateUser"
	ActionTransferDescribeUser       = "transfer:DescribeUser"
	ActionTransferListUsers          = "transfer:ListUsers"
	ActionTransferDeleteUser         = "transfer:DeleteUser"
	ActionTransferImportSshPublicKey = "transfer:ImportSshPublicKey"
	ActionTransferDeleteSshPublicKey = "transfer:DeleteSshPublicKey"
	ActionTransferPutFile            = "transfer:PutFile"
	ActionTransferGetFile            = "transfer:GetFile"
	ActionTransferListDirectory      = "transfer:ListDirectory"
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
	ActionSSMSendCommand            = "ssm:SendCommand"
	ActionSSMGetCommandInvocation   = "ssm:GetCommandInvocation"
	ActionSSMListCommandInvocations = "ssm:ListCommandInvocations"
	ActionSSMLabelParameterVersion  = "ssm:LabelParameterVersion"
	ActionSSMGetParameterHistory    = "ssm:GetParameterHistory"
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
	ActionECRListTagsForResource         = "ecr:ListTagsForResource"
	ActionECRTagResource                 = "ecr:TagResource"
	ActionECRUntagResource               = "ecr:UntagResource"
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
	ActionECSListTagsForResource      = "ecs:ListTagsForResource"
	ActionECSTagResource              = "ecs:TagResource"
	ActionECSUntagResource            = "ecs:UntagResource"
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
	ActionCloudTrailLookupEvents   = "cloudtrail:LookupEvents"
	ActionCloudTrailCreateTrail    = "cloudtrail:CreateTrail"
	ActionCloudTrailDescribeTrails = "cloudtrail:DescribeTrails"
	ActionCloudTrailDeleteTrail    = "cloudtrail:DeleteTrail"
	ActionCloudTrailStartLogging   = "cloudtrail:StartLogging"
	ActionCloudTrailStopLogging    = "cloudtrail:StopLogging"
	// ActionCloudTrailInjectEvents is a Noctaxris lab extension (not an AWS CloudTrail API).
	// Gated by NOCTAXRIS_CLOUDTRAIL_INJECT=1.
	ActionCloudTrailInjectEvents = "cloudtrail:InjectEvents"
	// ActionCloudTrailInjectInsightsEvents seeds insight-shaped JSONL records (no ML engine).
	// Gated by NOCTAXRIS_CLOUDTRAIL_INJECT=1.
	ActionCloudTrailInjectInsightsEvents = "cloudtrail:InjectInsightsEvents"
	ActionCloudTrailPutEventSelectors    = "cloudtrail:PutEventSelectors"
	ActionCloudTrailGetEventSelectors    = "cloudtrail:GetEventSelectors"
	// ActionCloudTrailValidateLogs is a Noctaxris lab helper (digest sidecar check; not full AWS ValidateLogs).
	ActionCloudTrailValidateLogs = "cloudtrail:ValidateLogs"
)

// GuardDuty lab actions.
const (
	ActionGuardDutyCreateDetector = "guardduty:CreateDetector"
	ActionGuardDutyListDetectors  = "guardduty:ListDetectors"
	ActionGuardDutyListFindings   = "guardduty:ListFindings"
	ActionGuardDutyGetFindings    = "guardduty:GetFindings"
	// ActionGuardDutyInjectFindings is a Noctaxris lab extension (not an AWS GuardDuty API).
	// Gated by NOCTAXRIS_GUARDDUTY_INJECT=1.
	ActionGuardDutyInjectFindings = "guardduty:InjectFindings"
)

// Detective lab actions (CreateGraph/ListGraphs/AcceptInvitation per AWS Detective API; SearchGraph is lab).
const (
	ActionDetectiveCreateGraph      = "detective:CreateGraph"
	ActionDetectiveListGraphs       = "detective:ListGraphs"
	ActionDetectiveAcceptInvitation = "detective:AcceptInvitation"
	ActionDetectiveSearchGraph      = "detective:SearchGraph"
)

// Macie2 lab actions.
const (
	ActionMacieEnableMacie               = "macie2:EnableMacie"
	ActionMacieGetMacieSession           = "macie2:GetMacieSession"
	ActionMacieCreateClassificationJob   = "macie2:CreateClassificationJob"
	ActionMacieDescribeClassificationJob = "macie2:DescribeClassificationJob"
	ActionMacieListClassificationJobs    = "macie2:ListClassificationJobs"
	ActionMacieListFindings              = "macie2:ListFindings"
	ActionMacieGetFindings               = "macie2:GetFindings"
	// ActionMacieInjectFindings is a Noctaxris lab extension (not an AWS Macie API).
	// Gated by NOCTAXRIS_MACIE_INJECT=1. Seeds sensitive-data findings (canned S3 matches OK).
	ActionMacieInjectFindings = "macie2:InjectFindings"
)

// EC2 lab actions (nested container instances + VPC/SG/ENI metadata + VPC Flow Logs).
const (
	ActionEC2RunInstances                  = "ec2:RunInstances"
	ActionEC2DescribeInstances             = "ec2:DescribeInstances"
	ActionEC2DescribeImages                = "ec2:DescribeImages"
	ActionEC2TerminateInstances            = "ec2:TerminateInstances"
	ActionEC2StopInstances                 = "ec2:StopInstances"
	ActionEC2StartInstances                = "ec2:StartInstances"
	ActionEC2CreateVpc                     = "ec2:CreateVpc"
	ActionEC2DeleteVpc                     = "ec2:DeleteVpc"
	ActionEC2DescribeVpcs                  = "ec2:DescribeVpcs"
	ActionEC2CreateSubnet                  = "ec2:CreateSubnet"
	ActionEC2DeleteSubnet                  = "ec2:DeleteSubnet"
	ActionEC2DescribeSubnets               = "ec2:DescribeSubnets"
	ActionEC2CreateSecurityGroup           = "ec2:CreateSecurityGroup"
	ActionEC2DeleteSecurityGroup           = "ec2:DeleteSecurityGroup"
	ActionEC2DescribeSecurityGroups        = "ec2:DescribeSecurityGroups"
	ActionEC2AuthorizeSecurityGroupIngress = "ec2:AuthorizeSecurityGroupIngress"
	ActionEC2AuthorizeSecurityGroupEgress  = "ec2:AuthorizeSecurityGroupEgress"
	ActionEC2RevokeSecurityGroupIngress    = "ec2:RevokeSecurityGroupIngress"
	ActionEC2RevokeSecurityGroupEgress     = "ec2:RevokeSecurityGroupEgress"
	ActionEC2DescribeNetworkInterfaces     = "ec2:DescribeNetworkInterfaces"
	ActionEC2CreateNetworkInterface        = "ec2:CreateNetworkInterface"
	ActionEC2CreateFlowLogs                = "ec2:CreateFlowLogs"
	// ActionEC2InjectFlowLogs is a Noctaxris lab extension (not an AWS EC2 API).
	// Gated by NOCTAXRIS_VPCFLOW_INJECT=1.
	ActionEC2InjectFlowLogs = "ec2:InjectFlowLogs"
)

// Security Hub lab actions.
const (
	ActionSecurityHubBatchImportFindings = "securityhub:BatchImportFindings"
	ActionSecurityHubGetFindings         = "securityhub:GetFindings"
)

// Noctaxris Lab forensics actions (not AWS APIs).
// Gated by NOCTAXRIS_LAB_FORENSICS=1.
const (
	ActionLabFreezeClock   = "noctaxris-lab:FreezeClock"
	ActionLabUnfreezeClock = "noctaxris-lab:UnfreezeClock"
	ActionLabSetClock      = "noctaxris-lab:SetClock"
	ActionLabBulkSeed      = "noctaxris-lab:BulkSeed"
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
	ActionKinesisCreateStream             = "kinesis:CreateStream"
	ActionKinesisDeleteStream             = "kinesis:DeleteStream"
	ActionKinesisDescribeStream           = "kinesis:DescribeStream"
	ActionKinesisListStreams              = "kinesis:ListStreams"
	ActionKinesisPutRecord                = "kinesis:PutRecord"
	ActionKinesisPutRecords               = "kinesis:PutRecords"
	ActionKinesisGetShardIterator         = "kinesis:GetShardIterator"
	ActionKinesisGetRecords               = "kinesis:GetRecords"
	ActionKinesisPutResourcePolicy        = "kinesis:PutResourcePolicy"
	ActionKinesisGetResourcePolicy        = "kinesis:GetResourcePolicy"
	ActionKinesisDeleteResourcePolicy     = "kinesis:DeleteResourcePolicy"
	ActionKinesisRegisterStreamConsumer   = "kinesis:RegisterStreamConsumer"
	ActionKinesisDescribeStreamConsumer   = "kinesis:DescribeStreamConsumer"
	ActionKinesisListStreamConsumers      = "kinesis:ListStreamConsumers"
	ActionKinesisDeregisterStreamConsumer = "kinesis:DeregisterStreamConsumer"
	ActionKinesisSubscribeToShard         = "kinesis:SubscribeToShard"
	ActionKinesisUpdateShardCount         = "kinesis:UpdateShardCount"
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
	ActionSESCreateEmailIdentity          = "ses:CreateEmailIdentity"
	ActionSESGetEmailIdentity             = "ses:GetEmailIdentity"
	ActionSESDeleteEmailIdentity          = "ses:DeleteEmailIdentity"
	ActionSESListEmailIdentities          = "ses:ListEmailIdentities"
	ActionSESGetAccount                   = "ses:GetAccount"
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
	ActionSFNSendTaskSuccess      = "states:SendTaskSuccess"
	ActionSFNSendTaskFailure      = "states:SendTaskFailure"
	ActionSFNSendTaskHeartbeat    = "states:SendTaskHeartbeat"
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
	ActionCodePipelinePutApprovalResult      = "codepipeline:PutApprovalResult"
	ActionCodePipelineGetPipelineExecution   = "codepipeline:GetPipelineExecution"
	ActionCodePipelineListPipelineExecutions = "codepipeline:ListPipelineExecutions"
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
	ActionGlueCreateRegistry = "glue:CreateRegistry"
	ActionGlueGetRegistry    = "glue:GetRegistry"
	ActionGlueListRegistries = "glue:ListRegistries"
	ActionGlueDeleteRegistry = "glue:DeleteRegistry"
	ActionGlueCreateSchema   = "glue:CreateSchema"
	ActionGlueGetSchema      = "glue:GetSchema"
	ActionGlueListSchemas    = "glue:ListSchemas"
	ActionGlueDeleteSchema   = "glue:DeleteSchema"
	ActionGlueRegisterSchemaVersion = "glue:RegisterSchemaVersion"
	ActionGlueGetSchemaVersion      = "glue:GetSchemaVersion"
	ActionGlueListSchemaVersions    = "glue:ListSchemaVersions"
)

// Athena lab actions.
const (
	ActionAthenaStartQueryExecution = "athena:StartQueryExecution"
	ActionAthenaGetQueryExecution   = "athena:GetQueryExecution"
	ActionAthenaGetQueryResults     = "athena:GetQueryResults"
	ActionAthenaStopQueryExecution  = "athena:StopQueryExecution"
	ActionAthenaCreateWorkGroup     = "athena:CreateWorkGroup"
	ActionAthenaGetWorkGroup        = "athena:GetWorkGroup"
	ActionAthenaListWorkGroups      = "athena:ListWorkGroups"
	ActionAthenaDeleteWorkGroup     = "athena:DeleteWorkGroup"
	ActionAthenaUpdateWorkGroup     = "athena:UpdateWorkGroup"
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
	ActionWAFCreateIPSet     = "wafv2:CreateIPSet"
	ActionWAFGetIPSet        = "wafv2:GetIPSet"
	ActionWAFUpdateIPSet     = "wafv2:UpdateIPSet"
	ActionWAFDeleteIPSet     = "wafv2:DeleteIPSet"
	ActionWAFListIPSets      = "wafv2:ListIPSets"
)

// AWS Config lab actions.
const (
	ActionConfigPutConfigurationRecorder       = "config:PutConfigurationRecorder"
	ActionConfigPutDeliveryChannel             = "config:PutDeliveryChannel"
	ActionConfigStartConfigurationRecorder     = "config:StartConfigurationRecorder"
	ActionConfigDescribeComplianceByConfigRule = "config:DescribeComplianceByConfigRule"
	ActionConfigGetResourceConfigHistory       = "config:GetResourceConfigHistory"
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
	// ActionRoute53InjectQueryLogs is a Noctaxris lab extension (not an AWS Route 53 API).
	// Gated by NOCTAXRIS_ROUTE53_QUERY_LOG_INJECT=1.
	ActionRoute53InjectQueryLogs = "route53:InjectQueryLogs"
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
	ActionAppSyncListApiKeys         = "appsync:ListApiKeys"
	ActionAppSyncDeleteApiKey        = "appsync:DeleteApiKey"
	ActionAppSyncCreateDataSource    = "appsync:CreateDataSource"
	ActionAppSyncUpdateDataSource    = "appsync:UpdateDataSource"
	ActionAppSyncDeleteDataSource    = "appsync:DeleteDataSource"
	ActionAppSyncGetDataSource       = "appsync:GetDataSource"
	ActionAppSyncListDataSources     = "appsync:ListDataSources"
	ActionAppSyncCreateResolver      = "appsync:CreateResolver"
	ActionAppSyncUpdateResolver      = "appsync:UpdateResolver"
	ActionAppSyncDeleteResolver      = "appsync:DeleteResolver"
	ActionAppSyncGetResolver         = "appsync:GetResolver"
	ActionAppSyncListResolvers       = "appsync:ListResolvers"
	ActionAppSyncGetSchemaCreationStatus = "appsync:GetSchemaCreationStatus"
	ActionAppSyncGraphQL             = "appsync:GraphQL"
)

// API Gateway REST API (v1) lab actions.
const (
	ActionAPIGatewayCreateRestApi    = "apigateway:CreateRestApi"
	ActionAPIGatewayGetRestApi       = "apigateway:GetRestApi"
	ActionAPIGatewayGetRestApis      = "apigateway:GetRestApis"
	ActionAPIGatewayDeleteRestApi    = "apigateway:DeleteRestApi"
	ActionAPIGatewayCreateResource   = "apigateway:CreateResource"
	ActionAPIGatewayGetResources     = "apigateway:GetResources"
	ActionAPIGatewayDeleteResource   = "apigateway:DeleteResource"
	ActionAPIGatewayPutMethod        = "apigateway:PutMethod"
	ActionAPIGatewayGetMethod        = "apigateway:GetMethod"
	ActionAPIGatewayDeleteMethod     = "apigateway:DeleteMethod"
	ActionAPIGatewayPutIntegration   = "apigateway:PutIntegration"
	ActionAPIGatewayGetIntegration   = "apigateway:GetIntegration"
	ActionAPIGatewayCreateDeployment = "apigateway:CreateDeployment"
	ActionAPIGatewayCreateStage      = "apigateway:CreateStage"
	ActionAPIGatewayGetStage         = "apigateway:GetStage"
	ActionAPIGatewayCreateAuthorizer = "apigateway:CreateAuthorizer"
	ActionAPIGatewayGetAuthorizer    = "apigateway:GetAuthorizer"
	ActionAPIGatewayGetAuthorizers   = "apigateway:GetAuthorizers"
	ActionAPIGatewayDeleteAuthorizer = "apigateway:DeleteAuthorizer"
	ActionAPIGatewayCreateApiKey     = "apigateway:CreateApiKey"
	ActionAPIGatewayGetApiKey        = "apigateway:GetApiKey"
	ActionAPIGatewayGetApiKeys       = "apigateway:GetApiKeys"
	ActionAPIGatewayDeleteApiKey     = "apigateway:DeleteApiKey"
	ActionAPIGatewayCreateUsagePlan  = "apigateway:CreateUsagePlan"
	ActionAPIGatewayGetUsagePlan     = "apigateway:GetUsagePlan"
	ActionAPIGatewayGetUsagePlans    = "apigateway:GetUsagePlans"
	ActionAPIGatewayDeleteUsagePlan  = "apigateway:DeleteUsagePlan"
	ActionAPIGatewayCreateUsagePlanKey = "apigateway:CreateUsagePlanKey"
	ActionAPIGatewayGetUsagePlanKeys   = "apigateway:GetUsagePlanKeys"
	ActionAPIGatewayDeleteUsagePlanKey = "apigateway:DeleteUsagePlanKey"
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

// execute-api ManageConnections for WebSocket @connections PostToConnection lab lite.
const ActionExecuteAPIManageConnections = "execute-api:ManageConnections"

// Cognito User Pools lab actions.
const (
	ActionCognitoCreateUserPool                   = "cognito-idp:CreateUserPool"
	ActionCognitoDescribeUserPool                 = "cognito-idp:DescribeUserPool"
	ActionCognitoUpdateUserPool                   = "cognito-idp:UpdateUserPool"
	ActionCognitoListUserPools                    = "cognito-idp:ListUserPools"
	ActionCognitoDeleteUserPool                   = "cognito-idp:DeleteUserPool"
	ActionCognitoCreateUserPoolClient             = "cognito-idp:CreateUserPoolClient"
	ActionCognitoDescribeUserPoolClient           = "cognito-idp:DescribeUserPoolClient"
	ActionCognitoListUserPoolClients              = "cognito-idp:ListUserPoolClients"
	ActionCognitoDeleteUserPoolClient             = "cognito-idp:DeleteUserPoolClient"
	ActionCognitoAdminCreateUser                  = "cognito-idp:AdminCreateUser"
	ActionCognitoAdminGetUser                     = "cognito-idp:AdminGetUser"
	ActionCognitoAdminSetUserPassword             = "cognito-idp:AdminSetUserPassword"
	ActionCognitoAdminDeleteUser                  = "cognito-idp:AdminDeleteUser"
	ActionCognitoAdminDisableUser                 = "cognito-idp:AdminDisableUser"
	ActionCognitoListUsers                        = "cognito-idp:ListUsers"
	ActionCognitoSignUp                           = "cognito-idp:SignUp"
	ActionCognitoConfirmSignUp                    = "cognito-idp:ConfirmSignUp"
	ActionCognitoForgotPassword                   = "cognito-idp:ForgotPassword"
	ActionCognitoConfirmForgotPassword            = "cognito-idp:ConfirmForgotPassword"
	ActionCognitoResendConfirmationCode           = "cognito-idp:ResendConfirmationCode"
	ActionCognitoUpdateUserAttributes             = "cognito-idp:UpdateUserAttributes"
	ActionCognitoGetUserAttributeVerificationCode = "cognito-idp:GetUserAttributeVerificationCode"
	ActionCognitoVerifyUserAttribute              = "cognito-idp:VerifyUserAttribute"
	ActionCognitoInitiateAuth                     = "cognito-idp:InitiateAuth"
	ActionCognitoAdminInitiateAuth                = "cognito-idp:AdminInitiateAuth"
	ActionCognitoRevokeToken                      = "cognito-idp:RevokeToken"
	ActionCognitoAssociateSoftwareToken           = "cognito-idp:AssociateSoftwareToken"
	ActionCognitoVerifySoftwareToken              = "cognito-idp:VerifySoftwareToken"
	ActionCognitoRespondToAuthChallenge           = "cognito-idp:RespondToAuthChallenge"
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

// Cost and Usage Reports lab actions.
const (
	ActionCURPutReportDefinition       = "cur:PutReportDefinition"
	ActionCURModifyReportDefinition    = "cur:ModifyReportDefinition"
	ActionCURDescribeReportDefinitions = "cur:DescribeReportDefinitions"
	ActionCURDeleteReportDefinition    = "cur:DeleteReportDefinition"
	ActionCURTagResource               = "cur:TagResource"
	ActionCURUntagResource             = "cur:UntagResource"
	ActionCURListTagsForResource       = "cur:ListTagsForResource"
)

// IoT Core and IoT Data lab actions.
const (
	ActionIoTCreateThing              = "iot:CreateThing"
	ActionIoTDescribeThing            = "iot:DescribeThing"
	ActionIoTListThings               = "iot:ListThings"
	ActionIoTUpdateThing              = "iot:UpdateThing"
	ActionIoTDeleteThing              = "iot:DeleteThing"
	ActionIoTCreateKeysAndCertificate = "iot:CreateKeysAndCertificate"
	ActionIoTDescribeCertificate      = "iot:DescribeCertificate"
	ActionIoTListCertificates         = "iot:ListCertificates"
	ActionIoTUpdateCertificate        = "iot:UpdateCertificate"
	ActionIoTDeleteCertificate        = "iot:DeleteCertificate"
	ActionIoTCreatePolicy             = "iot:CreatePolicy"
	ActionIoTGetPolicy                = "iot:GetPolicy"
	ActionIoTListPolicies             = "iot:ListPolicies"
	ActionIoTDeletePolicy             = "iot:DeletePolicy"
	ActionIoTAttachPolicy             = "iot:AttachPolicy"
	ActionIoTDetachPolicy             = "iot:DetachPolicy"
	ActionIoTAttachThingPrincipal     = "iot:AttachThingPrincipal"
	ActionIoTListThingPrincipals      = "iot:ListThingPrincipals"
	ActionIoTDataUpdateThingShadow    = "iot-data:UpdateThingShadow"
	ActionIoTDataGetThingShadow       = "iot-data:GetThingShadow"
	ActionIoTDataDeleteThingShadow    = "iot-data:DeleteThingShadow"
	ActionIoTCreateTopicRule          = "iot:CreateTopicRule"
	ActionIoTGetTopicRule             = "iot:GetTopicRule"
	ActionIoTListTopicRules           = "iot:ListTopicRules"
	ActionIoTReplaceTopicRule         = "iot:ReplaceTopicRule"
	ActionIoTDeleteTopicRule          = "iot:DeleteTopicRule"
	ActionIoTEnableTopicRule          = "iot:EnableTopicRule"
	ActionIoTDisableTopicRule         = "iot:DisableTopicRule"
)

// CloudWatch Metrics and Alarms lab actions.
const (
	ActionCloudWatchPutMetricData       = "cloudwatch:PutMetricData"
	ActionCloudWatchListMetrics         = "cloudwatch:ListMetrics"
	ActionCloudWatchGetMetricStatistics = "cloudwatch:GetMetricStatistics"
	ActionCloudWatchGetMetricData       = "cloudwatch:GetMetricData"
	ActionCloudWatchPutMetricAlarm      = "cloudwatch:PutMetricAlarm"
	ActionCloudWatchDescribeAlarms      = "cloudwatch:DescribeAlarms"
	ActionCloudWatchDeleteAlarms        = "cloudwatch:DeleteAlarms"
	ActionCloudWatchSetAlarmState       = "cloudwatch:SetAlarmState"
)

// Lightsail lab actions.
const (
	ActionLightsailGetBlueprints   = "lightsail:GetBlueprints"
	ActionLightsailGetBundles      = "lightsail:GetBundles"
	ActionLightsailCreateInstances = "lightsail:CreateInstances"
	ActionLightsailGetInstance     = "lightsail:GetInstance"
	ActionLightsailGetInstances    = "lightsail:GetInstances"
	ActionLightsailStartInstance   = "lightsail:StartInstance"
	ActionLightsailStopInstance    = "lightsail:StopInstance"
	ActionLightsailRebootInstance  = "lightsail:RebootInstance"
	ActionLightsailDeleteInstance  = "lightsail:DeleteInstance"
	ActionLightsailCreateDisk      = "lightsail:CreateDisk"
	ActionLightsailGetDisk         = "lightsail:GetDisk"
	ActionLightsailGetDisks        = "lightsail:GetDisks"
	ActionLightsailDeleteDisk      = "lightsail:DeleteDisk"
	ActionLightsailAttachDisk      = "lightsail:AttachDisk"
	ActionLightsailDetachDisk      = "lightsail:DetachDisk"
	ActionLightsailAllocateStaticIp = "lightsail:AllocateStaticIp"
	ActionLightsailGetStaticIp      = "lightsail:GetStaticIp"
	ActionLightsailGetStaticIps     = "lightsail:GetStaticIps"
	ActionLightsailReleaseStaticIp  = "lightsail:ReleaseStaticIp"
	ActionLightsailAttachStaticIp   = "lightsail:AttachStaticIp"
	ActionLightsailDetachStaticIp   = "lightsail:DetachStaticIp"
	ActionLightsailCreateKeyPair    = "lightsail:CreateKeyPair"
	ActionLightsailGetKeyPair       = "lightsail:GetKeyPair"
	ActionLightsailGetKeyPairs      = "lightsail:GetKeyPairs"
	ActionLightsailDeleteKeyPair    = "lightsail:DeleteKeyPair"
	ActionLightsailOpenInstancePublicPorts  = "lightsail:OpenInstancePublicPorts"
	ActionLightsailCloseInstancePublicPorts = "lightsail:CloseInstancePublicPorts"
	ActionLightsailGetInstancePortStates    = "lightsail:GetInstancePortStates"
)

// Auto Scaling lab actions.
const (
	ActionASGCreateLaunchConfiguration    = "autoscaling:CreateLaunchConfiguration"
	ActionASGDescribeLaunchConfigurations = "autoscaling:DescribeLaunchConfigurations"
	ActionASGDeleteLaunchConfiguration    = "autoscaling:DeleteLaunchConfiguration"
	ActionASGCreateAutoScalingGroup       = "autoscaling:CreateAutoScalingGroup"
	ActionASGDescribeAutoScalingGroups    = "autoscaling:DescribeAutoScalingGroups"
	ActionASGUpdateAutoScalingGroup       = "autoscaling:UpdateAutoScalingGroup"
	ActionASGDeleteAutoScalingGroup       = "autoscaling:DeleteAutoScalingGroup"
	ActionASGSetDesiredCapacity           = "autoscaling:SetDesiredCapacity"
	ActionASGPutScalingPolicy             = "autoscaling:PutScalingPolicy"
	ActionASGDescribePolicies             = "autoscaling:DescribePolicies"
	ActionASGDeletePolicy                 = "autoscaling:DeletePolicy"
	ActionASGPutLifecycleHook             = "autoscaling:PutLifecycleHook"
	ActionASGDescribeLifecycleHooks       = "autoscaling:DescribeLifecycleHooks"
	ActionASGDeleteLifecycleHook          = "autoscaling:DeleteLifecycleHook"
	ActionASGAttachInstances              = "autoscaling:AttachInstances"
	ActionASGDetachInstances              = "autoscaling:DetachInstances"
	ActionASGDescribeAutoScalingInstances = "autoscaling:DescribeAutoScalingInstances"
	ActionASGAttachLoadBalancerTargetGroups = "autoscaling:AttachLoadBalancerTargetGroups"
	ActionASGDetachLoadBalancerTargetGroups = "autoscaling:DetachLoadBalancerTargetGroups"
	ActionASGDescribeLoadBalancerTargetGroups = "autoscaling:DescribeLoadBalancerTargetGroups"
)

// Elastic Beanstalk lab actions.
const (
	ActionBeanstalkCreateApplication           = "elasticbeanstalk:CreateApplication"
	ActionBeanstalkDescribeApplications        = "elasticbeanstalk:DescribeApplications"
	ActionBeanstalkDeleteApplication           = "elasticbeanstalk:DeleteApplication"
	ActionBeanstalkCreateApplicationVersion    = "elasticbeanstalk:CreateApplicationVersion"
	ActionBeanstalkCreateEnvironment           = "elasticbeanstalk:CreateEnvironment"
	ActionBeanstalkDescribeEnvironments        = "elasticbeanstalk:DescribeEnvironments"
	ActionBeanstalkTerminateEnvironment        = "elasticbeanstalk:TerminateEnvironment"
	ActionBeanstalkListAvailableSolutionStacks = "elasticbeanstalk:ListAvailableSolutionStacks"
)

// AWS Backup lab actions.
const (
	ActionBackupCreateBackupVault               = "backup:CreateBackupVault"
	ActionBackupDescribeBackupVault             = "backup:DescribeBackupVault"
	ActionBackupListBackupVaults                = "backup:ListBackupVaults"
	ActionBackupDeleteBackupVault               = "backup:DeleteBackupVault"
	ActionBackupCreateBackupPlan                = "backup:CreateBackupPlan"
	ActionBackupGetBackupPlan                   = "backup:GetBackupPlan"
	ActionBackupListBackupPlans                 = "backup:ListBackupPlans"
	ActionBackupDeleteBackupPlan                = "backup:DeleteBackupPlan"
	ActionBackupStartBackupJob                  = "backup:StartBackupJob"
	ActionBackupDescribeBackupJob               = "backup:DescribeBackupJob"
	ActionBackupListBackupJobs                  = "backup:ListBackupJobs"
	ActionBackupStopBackupJob                   = "backup:StopBackupJob"
	ActionBackupDescribeRecoveryPoint           = "backup:DescribeRecoveryPoint"
	ActionBackupListRecoveryPointsByBackupVault = "backup:ListRecoveryPointsByBackupVault"
	ActionBackupDeleteRecoveryPoint             = "backup:DeleteRecoveryPoint"
	ActionBackupCreateBackupSelection           = "backup:CreateBackupSelection"
	ActionBackupGetBackupSelection              = "backup:GetBackupSelection"
	ActionBackupListBackupSelections            = "backup:ListBackupSelections"
	ActionBackupDeleteBackupSelection           = "backup:DeleteBackupSelection"
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
	ActionCloudFrontCreateDistribution     = "cloudfront:CreateDistribution"
	ActionCloudFrontGetDistribution        = "cloudfront:GetDistribution"
	ActionCloudFrontGetDistributionConfig  = "cloudfront:GetDistributionConfig"
	ActionCloudFrontUpdateDistribution     = "cloudfront:UpdateDistribution"
	ActionCloudFrontListDistributions      = "cloudfront:ListDistributions"
	ActionCloudFrontDeleteDistribution     = "cloudfront:DeleteDistribution"
	ActionCloudFrontCreateInvalidation     = "cloudfront:CreateInvalidation"
	ActionCloudFrontGetInvalidation        = "cloudfront:GetInvalidation"
	ActionCloudFrontListInvalidations      = "cloudfront:ListInvalidations"
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
	ActionELBv2ModifyListener        = "elasticloadbalancing:ModifyListener"
	ActionELBv2ModifyRule            = "elasticloadbalancing:ModifyRule"
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
	ActionBedrockConverse    = "bedrock:Converse"
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
	ActionEMRAddJobFlowSteps   = "elasticmapreduce:AddJobFlowSteps"
	ActionEMRDescribeStep      = "elasticmapreduce:DescribeStep"
	ActionEMRListSteps         = "elasticmapreduce:ListSteps"
	ActionEMRCancelSteps       = "elasticmapreduce:CancelSteps"
	ActionEMRListInstanceGroups = "elasticmapreduce:ListInstanceGroups"
	ActionEMRListInstanceFleets = "elasticmapreduce:ListInstanceFleets"
	ActionEMRAddTags            = "elasticmapreduce:AddTags"
	ActionEMRRemoveTags         = "elasticmapreduce:RemoveTags"
	ActionEMRDescribeSecurityConfiguration = "elasticmapreduce:DescribeSecurityConfiguration"
	ActionEMRCreateSecurityConfiguration   = "elasticmapreduce:CreateSecurityConfiguration"
	ActionEMRDeleteSecurityConfiguration   = "elasticmapreduce:DeleteSecurityConfiguration"
	ActionEMRListSecurityConfigurations    = "elasticmapreduce:ListSecurityConfigurations"
)

// CodeBuild lab actions.
const (
	ActionCodeBuildCreateProject    = "codebuild:CreateProject"
	ActionCodeBuildUpdateProject    = "codebuild:UpdateProject"
	ActionCodeBuildDeleteProject    = "codebuild:DeleteProject"
	ActionCodeBuildListProjects     = "codebuild:ListProjects"
	ActionCodeBuildBatchGetProjects = "codebuild:BatchGetProjects"
	ActionCodeBuildStartBuild       = "codebuild:StartBuild"
	ActionCodeBuildStartBuildBatch  = "codebuild:StartBuildBatch"
	ActionCodeBuildStopBuild        = "codebuild:StopBuild"
	ActionCodeBuildBatchGetBuilds   = "codebuild:BatchGetBuilds"
	ActionCodeBuildListBuilds       = "codebuild:ListBuilds"
	ActionCodeBuildCreateWebhook    = "codebuild:CreateWebhook"
	ActionCodeBuildDeleteWebhook    = "codebuild:DeleteWebhook"
	ActionCodeBuildListWebhooks     = "codebuild:ListWebhooks"
)

// CodeCommit lab actions (filesystem-backed store; not git smart-HTTP).
const (
	ActionCodeCommitCreateRepository = "codecommit:CreateRepository"
	ActionCodeCommitGetRepository    = "codecommit:GetRepository"
	ActionCodeCommitListRepositories = "codecommit:ListRepositories"
	ActionCodeCommitDeleteRepository = "codecommit:DeleteRepository"
	ActionCodeCommitPutFile          = "codecommit:PutFile"
	ActionCodeCommitGetFile          = "codecommit:GetFile"
	ActionCodeCommitGetFolder        = "codecommit:GetFolder"
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
