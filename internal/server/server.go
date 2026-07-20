package server

import (
	"bytes"
	"context"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/Kyaxris-Labs/Noctaxris/internal/catalog"
	"github.com/Kyaxris-Labs/Noctaxris/internal/compute"
	"github.com/Kyaxris-Labs/Noctaxris/internal/config"
	"github.com/Kyaxris-Labs/Noctaxris/internal/kernel/audit"
	"github.com/Kyaxris-Labs/Noctaxris/internal/kernel/authn"
	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
	"github.com/google/uuid"
)

const (
	eventVersion    = "1.11"
	healthPath      = "/_noctaxris/health"
	requestIDHeader = "x-amz-request-id"
	maxBodyBytes    = 1 << 20  // 1 MiB
	maxS3BodyBytes  = 16 << 20 // 16 MiB lab PutObject
	sigv4Skew       = 15 * time.Minute
)

type Server struct {
	cfg   config.Config
	store *store.Store
	audit *audit.Writer
	now   func() time.Time

	// Lazy nested-engine client for ECS / Registry (DinD, cfg.DockerHost).
	computeOnce sync.Once
	compute     *compute.Client
	computeErr  error

	// Lazy Lambda invoker (DinD default or opt-in microVM).
	invokerOnce sync.Once
	invoker     compute.FunctionInvoker
	invokerErr  error
}

type awsError struct {
	XMLName xml.Name `xml:"Error"`
	Code    string   `xml:"Code"`
	Message string   `xml:"Message"`
	Type    string   `xml:"Type"`
}

type awsErrorResponse struct {
	XMLName   xml.Name `xml:"ErrorResponse"`
	XMLNS     string   `xml:"xmlns,attr"`
	Error     awsError `xml:"Error"`
	RequestID string   `xml:"RequestId"`
}

func New(cfg config.Config, st *store.Store, aud *audit.Writer) *Server {
	return &Server{
		cfg:   cfg,
		store: st,
		audit: aud,
		now:   time.Now,
	}
}

func (s *Server) Handler() http.Handler {
	return http.HandlerFunc(s.serveHTTP)
}

func (s *Server) ListenAndServe() error {
	srv := &http.Server{
		Addr:    s.cfg.ListenAddr,
		Handler: s.Handler(),
	}
	if s.cfg.TLSCertFile != "" && s.cfg.TLSKeyFile != "" {
		return srv.ListenAndServeTLS(s.cfg.TLSCertFile, s.cfg.TLSKeyFile)
	}
	return srv.ListenAndServe()
}

func (s *Server) serveHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet && r.URL.Path == healthPath {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
		return
	}

	if isRegistryV2Path(r.URL.Path) {
		s.handleRegistryV2(w, r)
		return
	}

	requestID := newRequestID()
	eventID := newRequestID()
	readOnly := r.Method == http.MethodGet || r.Method == http.MethodHead

	bodyLimit := bodyLimitForRequest(r)
	body, err := readBody(r, bodyLimit)
	if err != nil {
		s.writeAWSError(w, requestID, http.StatusBadRequest, "InvalidRequest",
			"Unable to read request body.", readOnly, r, eventID, "", "", false)
		return
	}

	action := resolveAction(r, body)
	var verified *authn.Verified
	if isUnauthenticatedSTSAction(action) {
		// AWS STS federation APIs authenticate via SAML/OIDC token, not SigV4.
		verified = &authn.Verified{Region: federationRegion(r)}
	} else {
		var err error
		verified, err = authn.Verify(r, body, s.now(), sigv4Skew, s.lookupKey)
		if err != nil {
			code := authn.Code(err)
			if code == "" {
				code = authn.CodeInvalidClientTokenId
			}
			msg := defaultAuthnMessage(code)
			accessKeyID := ""
			if ak, ok := parseAccessKeyID(r.Header.Get("Authorization")); ok {
				accessKeyID = ak
			}
			s.writeAWSError(w, requestID, http.StatusForbidden, code, msg, readOnly, r, eventID, accessKeyID, "", false)
			return
		}
		verified.SourceIP = clientIP(r)
	}

	if action == "" && (strings.EqualFold(verified.Service, "lambda") || isLambdaRESTPath(r.URL.Path)) {
		restAction, body2 := resolveLambdaREST(r, body)
		if restAction != "" {
			s.handleLambda(w, r, body2, requestID, eventID, restAction, verified, readOnly)
			return
		}
	}

	if action == "" && (strings.EqualFold(verified.Service, "batch") || isBatchRESTPath(r.URL.Path)) {
		if restAction := resolveBatchREST(r); restAction != "" {
			s.handleBatch(w, r, body, requestID, eventID, restAction, verified, readOnly)
			return
		}
	}

	if action == "" && (verified.Service == "s3" || isS3PathStyleRequest(r, body, action)) {
		s.handleS3(w, r, body, requestID, eventID, verified, readOnly)
		return
	}

	if isOrgsDepthAction(action, verified.Service) {
		s.handleOrgsDepth(w, r, body, requestID, eventID, action, verified, readOnly)
		return
	}

	if verified.Service == "sns" || strings.HasPrefix(action, "sns:") {
		s.handleSNS(w, r, body, requestID, eventID, action, verified, readOnly)
		return
	}

	if verified.Service == "ses" || verified.Service == "email" || strings.HasPrefix(action, "ses:") {
		s.handleSES(w, r, body, requestID, eventID, action, verified, readOnly)
		return
	}

	if verified.Service == "cloudformation" || strings.HasPrefix(action, "cloudformation:") {
		s.handleCloudFormation(w, r, body, requestID, eventID, action, verified, readOnly)
		return
	}

	if verified.Service == "config" || strings.HasPrefix(action, "config:") {
		s.handleConfig(w, r, body, requestID, eventID, action, verified, readOnly)
		return
	}

	if verified.Service == "events" || strings.HasPrefix(action, "events:") {
		s.handleEventBridge(w, r, body, requestID, eventID, action, verified, readOnly)
		return
	}

	if verified.Service == "ecs" || strings.HasPrefix(action, "ecs:") {
		s.handleECS(w, r, body, requestID, eventID, action, verified, readOnly)
		return
	}

	switch action {
	case catalog.ActionSTSGetCallerIdentity, "GetCallerIdentity":
		s.handleGetCallerIdentity(w, r, requestID, eventID, verified)
	case catalog.ActionOrgsCreateAccount, "CreateAccount":
		s.handleCreateAccount(w, r, body, requestID, eventID, verified, readOnly)
	case catalog.ActionOrgsDescribeCreateAccountStatus, "DescribeCreateAccountStatus":
		s.handleDescribeCreateAccountStatus(w, r, body, requestID, eventID, verified, readOnly)
	case catalog.ActionSTSAssumeRole, "AssumeRole":
		s.handleAssumeRole(w, r, body, requestID, eventID, verified, readOnly)
	case catalog.ActionSTSGetSessionToken, "GetSessionToken":
		s.handleGetSessionToken(w, r, body, requestID, eventID, verified, readOnly)
	case catalog.ActionSTSGetFederationToken, "GetFederationToken":
		s.handleGetFederationToken(w, r, body, requestID, eventID, verified, readOnly)
	case catalog.ActionSTSGetAccessKeyInfo, "GetAccessKeyInfo":
		s.handleGetAccessKeyInfo(w, r, body, requestID, eventID, verified, readOnly)
	case catalog.ActionSTSDecodeAuthorizationMessage, "DecodeAuthorizationMessage":
		s.handleDecodeAuthorizationMessage(w, r, body, requestID, eventID, verified, readOnly)
	case catalog.ActionSTSAssumeRoot, "AssumeRoot":
		s.handleAssumeRoot(w, r, body, requestID, eventID, verified, readOnly)
	case catalog.ActionSTSAssumeRoleWithSAML, "AssumeRoleWithSAML":
		s.handleAssumeRoleWithSAML(w, r, body, requestID, eventID, verified, readOnly)
	case catalog.ActionSTSAssumeRoleWithWebIdentity, "AssumeRoleWithWebIdentity":
		s.handleAssumeRoleWithWebIdentity(w, r, body, requestID, eventID, verified, readOnly)
	case catalog.ActionSTSGetDelegatedAccessToken, "GetDelegatedAccessToken":
		s.handleGetDelegatedAccessToken(w, r, requestID, eventID, verified, readOnly)
	case catalog.ActionSTSGetWebIdentityToken, "GetWebIdentityToken":
		s.handleGetWebIdentityToken(w, r, body, requestID, eventID, verified, readOnly)
	case catalog.ActionIAMCreateUser, "CreateUser",
		catalog.ActionIAMGetUser, "GetUser",
		catalog.ActionIAMListUsers, "ListUsers",
		catalog.ActionIAMDeleteUser, "DeleteUser",
		catalog.ActionIAMCreateAccessKey, "CreateAccessKey",
		catalog.ActionIAMDeleteAccessKey, "DeleteAccessKey",
		catalog.ActionIAMListAccessKeys, "ListAccessKeys",
		catalog.ActionIAMUpdateAccessKey, "UpdateAccessKey",
		catalog.ActionIAMCreatePolicy, "CreatePolicy",
		catalog.ActionIAMGetPolicy, "GetPolicy",
		catalog.ActionIAMListPolicies, "ListPolicies",
		catalog.ActionIAMDeletePolicy, "DeletePolicy",
		catalog.ActionIAMAttachUserPolicy, "AttachUserPolicy",
		catalog.ActionIAMDetachUserPolicy, "DetachUserPolicy",
		catalog.ActionIAMAttachRolePolicy, "AttachRolePolicy",
		catalog.ActionIAMDetachRolePolicy, "DetachRolePolicy",
		catalog.ActionIAMListAttachedUserPolicies, "ListAttachedUserPolicies",
		catalog.ActionIAMListAttachedRolePolicies, "ListAttachedRolePolicies",
		catalog.ActionIAMPutUserPolicy, "PutUserPolicy",
		catalog.ActionIAMGetUserPolicy, "GetUserPolicy",
		catalog.ActionIAMDeleteUserPolicy, "DeleteUserPolicy",
		catalog.ActionIAMListUserPolicies, "ListUserPolicies",
		catalog.ActionIAMPutRolePolicy, "PutRolePolicy",
		catalog.ActionIAMGetRolePolicy, "GetRolePolicy",
		catalog.ActionIAMDeleteRolePolicy, "DeleteRolePolicy",
		catalog.ActionIAMListRolePolicies, "ListRolePolicies",
		catalog.ActionIAMCreateRole, "CreateRole",
		catalog.ActionIAMGetRole, "GetRole",
		catalog.ActionIAMListRoles, "ListRoles",
		catalog.ActionIAMDeleteRole, "DeleteRole",
		catalog.ActionIAMUpdateAssumeRolePolicy, "UpdateAssumeRolePolicy",
		catalog.ActionIAMCreateGroup, "CreateGroup",
		catalog.ActionIAMDeleteGroup, "DeleteGroup",
		catalog.ActionIAMGetGroup, "GetGroup",
		catalog.ActionIAMListGroups, "ListGroups",
		catalog.ActionIAMAddUserToGroup, "AddUserToGroup",
		catalog.ActionIAMRemoveUserFromGroup, "RemoveUserFromGroup",
		catalog.ActionIAMAttachGroupPolicy, "AttachGroupPolicy",
		catalog.ActionIAMDetachGroupPolicy, "DetachGroupPolicy",
		catalog.ActionIAMListAttachedGroupPolicies, "ListAttachedGroupPolicies",
		catalog.ActionIAMPutGroupPolicy, "PutGroupPolicy",
		catalog.ActionIAMGetGroupPolicy, "GetGroupPolicy",
		catalog.ActionIAMDeleteGroupPolicy, "DeleteGroupPolicy",
		catalog.ActionIAMListGroupPolicies, "ListGroupPolicies",
		catalog.ActionIAMPutUserPermissionsBoundary, "PutUserPermissionsBoundary",
		catalog.ActionIAMGetUserPermissionsBoundary, "GetUserPermissionsBoundary",
		catalog.ActionIAMDeleteUserPermissionsBoundary, "DeleteUserPermissionsBoundary",
		catalog.ActionIAMPutRolePermissionsBoundary, "PutRolePermissionsBoundary",
		catalog.ActionIAMGetRolePermissionsBoundary, "GetRolePermissionsBoundary",
		catalog.ActionIAMDeleteRolePermissionsBoundary, "DeleteRolePermissionsBoundary",
		catalog.ActionIAMCreateInstanceProfile, "CreateInstanceProfile",
		catalog.ActionIAMDeleteInstanceProfile, "DeleteInstanceProfile",
		catalog.ActionIAMGetInstanceProfile, "GetInstanceProfile",
		catalog.ActionIAMAddRoleToInstanceProfile, "AddRoleToInstanceProfile",
		catalog.ActionIAMRemoveRoleFromInstanceProfile, "RemoveRoleFromInstanceProfile",
		catalog.ActionIAMListInstanceProfiles, "ListInstanceProfiles",
		catalog.ActionIAMCreateOpenIDConnectProvider, "CreateOpenIDConnectProvider",
		catalog.ActionIAMDeleteOpenIDConnectProvider, "DeleteOpenIDConnectProvider",
		catalog.ActionIAMListOpenIDConnectProviders, "ListOpenIDConnectProviders",
		catalog.ActionIAMGetOpenIDConnectProvider, "GetOpenIDConnectProvider",
		catalog.ActionIAMCreateSAMLProvider, "CreateSAMLProvider",
		catalog.ActionIAMDeleteSAMLProvider, "DeleteSAMLProvider",
		catalog.ActionIAMListSAMLProviders, "ListSAMLProviders",
		catalog.ActionIAMGetSAMLProvider, "GetSAMLProvider",
		catalog.ActionIAMCreateVirtualMFADevice, "CreateVirtualMFADevice",
		catalog.ActionIAMEnableMFADevice, "EnableMFADevice",
		catalog.ActionIAMListMFADevices, "ListMFADevices",
		catalog.ActionIAMDeactivateMFADevice, "DeactivateMFADevice":
		s.handleIAM(w, r, body, requestID, eventID, action, verified, readOnly)
	case catalog.ActionKMSCreateKey, "CreateKey",
		catalog.ActionKMSDescribeKey, "DescribeKey",
		catalog.ActionKMSListKeys, "ListKeys",
		catalog.ActionKMSEnableKey, "EnableKey",
		catalog.ActionKMSDisableKey, "DisableKey",
		catalog.ActionKMSGetKeyPolicy, "GetKeyPolicy",
		catalog.ActionKMSPutKeyPolicy, "PutKeyPolicy",
		catalog.ActionKMSEncrypt, "Encrypt",
		catalog.ActionKMSDecrypt, "Decrypt",
		catalog.ActionKMSGenerateDataKey, "GenerateDataKey",
		catalog.ActionKMSGenerateDataKeyWithoutPlaintext, "GenerateDataKeyWithoutPlaintext",
		catalog.ActionKMSCreateGrant, "CreateGrant",
		catalog.ActionKMSListGrants, "ListGrants",
		catalog.ActionKMSRetireGrant, "RetireGrant",
		catalog.ActionKMSRevokeGrant, "RevokeGrant",
		catalog.ActionKMSCreateAlias, "CreateAlias",
		catalog.ActionKMSListAliases, "ListAliases",
		catalog.ActionKMSDeleteAlias, "DeleteAlias",
		catalog.ActionKMSUpdateAlias, "UpdateAlias",
		catalog.ActionKMSScheduleKeyDeletion, "ScheduleKeyDeletion",
		catalog.ActionKMSCancelKeyDeletion, "CancelKeyDeletion",
		catalog.ActionKMSEnableKeyRotation, "EnableKeyRotation",
		catalog.ActionKMSDisableKeyRotation, "DisableKeyRotation",
		catalog.ActionKMSGetKeyRotationStatus, "GetKeyRotationStatus",
		"ReEncrypt":
		s.handleKMS(w, r, body, requestID, eventID, action, verified, readOnly)
	case catalog.ActionDynamoDBCreateTable, "CreateTable",
		catalog.ActionDynamoDBDescribeTable, "DescribeTable",
		catalog.ActionDynamoDBDeleteTable, "DeleteTable",
		catalog.ActionDynamoDBListTables, "ListTables",
		catalog.ActionDynamoDBUpdateTable, "UpdateTable",
		catalog.ActionDynamoDBPutItem, "PutItem",
		catalog.ActionDynamoDBGetItem, "GetItem",
		catalog.ActionDynamoDBDeleteItem, "DeleteItem",
		catalog.ActionDynamoDBUpdateItem, "UpdateItem",
		catalog.ActionDynamoDBQuery, "Query",
		catalog.ActionDynamoDBScan, "Scan",
		catalog.ActionDynamoDBBatchGetItem, "BatchGetItem",
		catalog.ActionDynamoDBBatchWriteItem, "BatchWriteItem",
		catalog.ActionDynamoDBPutResourcePolicy, "PutResourcePolicy",
		catalog.ActionDynamoDBGetResourcePolicy, "GetResourcePolicy",
		catalog.ActionDynamoDBDeleteResourcePolicy, "DeleteResourcePolicy",
		catalog.ActionDynamoDBUpdateTimeToLive, "UpdateTimeToLive",
		catalog.ActionDynamoDBDescribeTimeToLive, "DescribeTimeToLive":
		s.handleDynamoDB(w, r, body, requestID, eventID, action, verified, readOnly)
	case catalog.ActionSQSCreateQueue, "CreateQueue",
		catalog.ActionSQSGetQueueUrl, "GetQueueUrl",
		catalog.ActionSQSGetQueueAttributes, "GetQueueAttributes",
		catalog.ActionSQSSetQueueAttributes, "SetQueueAttributes",
		catalog.ActionSQSDeleteQueue, "DeleteQueue",
		catalog.ActionSQSListQueues, "ListQueues",
		catalog.ActionSQSPurgeQueue, "PurgeQueue",
		catalog.ActionSQSSendMessage, "SendMessage",
		catalog.ActionSQSReceiveMessage, "ReceiveMessage",
		catalog.ActionSQSDeleteMessage, "DeleteMessage",
		catalog.ActionSQSSendMessageBatch, "SendMessageBatch",
		catalog.ActionSQSDeleteMessageBatch, "DeleteMessageBatch",
		catalog.ActionSQSChangeMessageVisibility, "ChangeMessageVisibility":
		s.handleSQS(w, r, body, requestID, eventID, action, verified, readOnly)
	case catalog.ActionSSMPutParameter, "PutParameter",
		catalog.ActionSSMGetParameter, "GetParameter",
		catalog.ActionSSMGetParameters, "GetParameters",
		catalog.ActionSSMGetParametersByPath, "GetParametersByPath",
		catalog.ActionSSMDeleteParameter, "DeleteParameter",
		catalog.ActionSSMDescribeParameters, "DescribeParameters":
		s.handleSSM(w, r, body, requestID, eventID, action, verified, readOnly)
	case catalog.ActionSecretsCreateSecret, "CreateSecret",
		catalog.ActionSecretsGetSecretValue, "GetSecretValue",
		catalog.ActionSecretsPutSecretValue, "PutSecretValue",
		catalog.ActionSecretsDeleteSecret, "DeleteSecret",
		catalog.ActionSecretsRestoreSecret, "RestoreSecret",
		catalog.ActionSecretsRotateSecret, "RotateSecret",
		catalog.ActionSecretsDescribeSecret, "DescribeSecret",
		catalog.ActionSecretsListSecrets, "ListSecrets",
		catalog.ActionSecretsPutResourcePolicy,
		catalog.ActionSecretsGetResourcePolicy,
		catalog.ActionSecretsDeleteResourcePolicy:
		s.handleSecretsManager(w, r, body, requestID, eventID, action, verified, readOnly)
	case catalog.ActionECRCreateRepository, "CreateRepository",
		catalog.ActionECRDescribeRepositories, "DescribeRepositories",
		catalog.ActionECRDeleteRepository, "DeleteRepository",
		catalog.ActionECRGetAuthorizationToken, "GetAuthorizationToken",
		catalog.ActionECRGetRepositoryPolicy, "GetRepositoryPolicy",
		catalog.ActionECRSetRepositoryPolicy, "SetRepositoryPolicy",
		catalog.ActionECRDeleteRepositoryPolicy, "DeleteRepositoryPolicy",
		catalog.ActionECRPutImage, "PutImage",
		catalog.ActionECRBatchGetImage, "BatchGetImage",
		catalog.ActionECRListImages, "ListImages",
		catalog.ActionECRBatchDeleteImage, "BatchDeleteImage":
		s.handleECR(w, r, body, requestID, eventID, action, verified, readOnly)
	case catalog.ActionEventsPutEvents, "PutEvents",
		catalog.ActionEventsCreateEventBus, "CreateEventBus",
		catalog.ActionEventsDeleteEventBus, "DeleteEventBus",
		catalog.ActionEventsDescribeEventBus, "DescribeEventBus",
		catalog.ActionEventsListEventBuses, "ListEventBuses",
		catalog.ActionEventsPutRule, "PutRule",
		catalog.ActionEventsDescribeRule, "DescribeRule",
		catalog.ActionEventsListRules, "ListRules",
		catalog.ActionEventsDeleteRule, "DeleteRule",
		catalog.ActionEventsEnableRule, "EnableRule",
		catalog.ActionEventsDisableRule, "DisableRule",
		catalog.ActionEventsPutTargets, "PutTargets",
		catalog.ActionEventsRemoveTargets, "RemoveTargets",
		catalog.ActionEventsListTargetsByRule, "ListTargetsByRule":
		s.handleEventBridge(w, r, body, requestID, eventID, action, verified, readOnly)
	case catalog.ActionLambdaCreateFunction, "CreateFunction",
		catalog.ActionLambdaGetFunction, "GetFunction",
		catalog.ActionLambdaDeleteFunction, "DeleteFunction",
		catalog.ActionLambdaListFunctions, "ListFunctions",
		catalog.ActionLambdaUpdateFunctionCode, "UpdateFunctionCode",
		catalog.ActionLambdaUpdateFunctionConfiguration, "UpdateFunctionConfiguration",
		catalog.ActionLambdaInvoke, "Invoke",
		catalog.ActionLambdaPublishVersion, "PublishVersion",
		catalog.ActionLambdaListVersionsByFunction, "ListVersionsByFunction",
		catalog.ActionLambdaCreateAlias,
		catalog.ActionLambdaUpdateAlias,
		catalog.ActionLambdaDeleteAlias,
		catalog.ActionLambdaGetAlias, "GetAlias",
		catalog.ActionLambdaListAliases,
		catalog.ActionLambdaPublishLayerVersion, "PublishLayerVersion",
		catalog.ActionLambdaGetLayerVersion, "GetLayerVersion",
		catalog.ActionLambdaListLayerVersions, "ListLayerVersions",
		catalog.ActionLambdaDeleteLayerVersion, "DeleteLayerVersion",
		catalog.ActionLambdaAddPermission, "AddPermission",
		catalog.ActionLambdaRemovePermission, "RemovePermission",
		catalog.ActionLambdaGetPolicy:
		s.handleLambda(w, r, body, requestID, eventID, action, verified, readOnly)
	case catalog.ActionECSRegisterTaskDefinition, "RegisterTaskDefinition",
		catalog.ActionECSDescribeTaskDefinition, "DescribeTaskDefinition",
		catalog.ActionECSListTaskDefinitions, "ListTaskDefinitions",
		catalog.ActionECSDeregisterTaskDefinition, "DeregisterTaskDefinition",
		catalog.ActionECSRunTask, "RunTask",
		catalog.ActionECSDescribeTasks, "DescribeTasks",
		catalog.ActionECSListTasks, "ListTasks",
		catalog.ActionECSStopTask, "StopTask",
		catalog.ActionECSDescribeClusters, "DescribeClusters",
		catalog.ActionECSListClusters, "ListClusters":
		s.handleECS(w, r, body, requestID, eventID, action, verified, readOnly)
	case catalog.ActionCloudTrailLookupEvents, "LookupEvents":
		s.handleCloudTrail(w, r, body, requestID, eventID, action, verified, readOnly)
	case catalog.ActionLogsCreateLogGroup, "CreateLogGroup",
		catalog.ActionLogsCreateLogStream, "CreateLogStream",
		catalog.ActionLogsPutLogEvents, "PutLogEvents",
		catalog.ActionLogsGetLogEvents, "GetLogEvents",
		catalog.ActionLogsDescribeLogGroups, "DescribeLogGroups":
		s.handleLogs(w, r, body, requestID, eventID, action, verified, readOnly)
	case catalog.ActionTaggingTagResources, "TagResources",
		catalog.ActionTaggingUntagResources, "UntagResources",
		catalog.ActionTaggingGetResources, "GetResources":
		s.handleTagging(w, r, body, requestID, eventID, action, verified, readOnly)
	case catalog.ActionKinesisCreateStream, "CreateStream",
		catalog.ActionKinesisDeleteStream, "DeleteStream",
		catalog.ActionKinesisDescribeStream, "DescribeStream",
		catalog.ActionKinesisListStreams, "ListStreams",
		catalog.ActionKinesisPutRecord, "PutRecord",
		catalog.ActionKinesisPutRecords, "PutRecords",
		catalog.ActionKinesisGetShardIterator, "GetShardIterator",
		catalog.ActionKinesisGetRecords, "GetRecords":
		s.handleKinesis(w, r, body, requestID, eventID, action, verified, readOnly)
	case catalog.ActionAppConfigCreateApplication, "CreateApplication",
		catalog.ActionAppConfigCreateEnvironment, "CreateEnvironment",
		catalog.ActionAppConfigCreateConfigurationProfile, "CreateConfigurationProfile",
		catalog.ActionAppConfigCreateHostedConfigurationVersion, "CreateHostedConfigurationVersion",
		catalog.ActionAppConfigGetConfiguration, "GetConfiguration",
		catalog.ActionAppConfigDataStartConfigurationSession, "StartConfigurationSession",
		catalog.ActionAppConfigDataGetLatestConfiguration, "GetLatestConfiguration":
		s.handleAppConfig(w, r, body, requestID, eventID, action, verified, readOnly)
	case catalog.ActionSFNCreateStateMachine, "CreateStateMachine",
		catalog.ActionSFNDeleteStateMachine, "DeleteStateMachine",
		catalog.ActionSFNDescribeStateMachine, "DescribeStateMachine",
		catalog.ActionSFNListStateMachines, "ListStateMachines",
		catalog.ActionSFNStartExecution, "StartExecution",
		catalog.ActionSFNDescribeExecution, "DescribeExecution",
		catalog.ActionSFNGetExecutionHistory, "GetExecutionHistory":
		s.handleSFN(w, r, body, requestID, eventID, action, verified, readOnly)
	case catalog.ActionCodeBuildCreateProject, "CreateProject",
		catalog.ActionCodeBuildStartBuild, "StartBuild",
		catalog.ActionCodeBuildBatchGetBuilds, "BatchGetBuilds",
		catalog.ActionCodeBuildListBuilds, "ListBuilds":
		s.handleCodeBuild(w, r, body, requestID, eventID, action, verified, readOnly)
	case catalog.ActionBatchCreateComputeEnvironment, "CreateComputeEnvironment",
		catalog.ActionBatchCreateJobQueue, "CreateJobQueue",
		catalog.ActionBatchRegisterJobDefinition, "RegisterJobDefinition",
		catalog.ActionBatchSubmitJob, "SubmitJob",
		catalog.ActionBatchDescribeComputeEnvironments, "DescribeComputeEnvironments",
		catalog.ActionBatchDescribeJobQueues, "DescribeJobQueues",
		catalog.ActionBatchDescribeJobDefinitions, "DescribeJobDefinitions",
		catalog.ActionBatchDescribeJobs, "DescribeJobs":
		s.handleBatch(w, r, body, requestID, eventID, action, verified, readOnly)
	case catalog.ActionCFNCreateStack, "CreateStack",
		catalog.ActionCFNDescribeStacks, "DescribeStacks",
		catalog.ActionCFNDeleteStack, "DeleteStack",
		catalog.ActionCFNListStacks, "ListStacks":
		s.handleCloudFormation(w, r, body, requestID, eventID, action, verified, readOnly)
	case catalog.ActionCodePipelineCreatePipeline, "CreatePipeline",
		catalog.ActionCodePipelineGetPipeline, "GetPipeline",
		catalog.ActionCodePipelineDeletePipeline, "DeletePipeline",
		catalog.ActionCodePipelineStartPipelineExecution, "StartPipelineExecution",
		catalog.ActionCodePipelineGetPipelineState, "GetPipelineState":
		s.handleCodePipeline(w, r, body, requestID, eventID, action, verified, readOnly)
	case catalog.ActionFirehoseCreateDeliveryStream, "CreateDeliveryStream",
		catalog.ActionFirehoseDeleteDeliveryStream, "DeleteDeliveryStream",
		catalog.ActionFirehoseDescribeDeliveryStream, "DescribeDeliveryStream",
		catalog.ActionFirehoseListDeliveryStreams, "ListDeliveryStreams",
		catalog.ActionFirehosePutRecord,
		catalog.ActionFirehosePutRecordBatch, "PutRecordBatch":
		s.handleFirehose(w, r, body, requestID, eventID, action, verified, readOnly)
	case catalog.ActionGlueCreateDatabase, "CreateDatabase",
		catalog.ActionGlueGetDatabase, "GetDatabase",
		catalog.ActionGlueGetDatabases, "GetDatabases",
		catalog.ActionGlueDeleteDatabase, "DeleteDatabase",
		catalog.ActionGlueCreateTable,
		catalog.ActionGlueGetTable, "GetTable",
		catalog.ActionGlueGetTables, "GetTables",
		catalog.ActionGlueDeleteTable:
		s.handleGlue(w, r, body, requestID, eventID, action, verified, readOnly)
	case catalog.ActionWAFCreateWebACL, "CreateWebACL",
		catalog.ActionWAFUpdateWebACL, "UpdateWebACL",
		catalog.ActionWAFGetWebACL, "GetWebACL",
		catalog.ActionWAFListWebACLs, "ListWebACLs",
		catalog.ActionWAFCreateRuleGroup, "CreateRuleGroup",
		catalog.ActionWAFAssociateWebACL, "AssociateWebACL",
		catalog.ActionWAFEvaluate, "Evaluate":
		s.handleWAFv2(w, r, body, requestID, eventID, action, verified, readOnly)
	case catalog.ActionConfigPutConfigurationRecorder, "PutConfigurationRecorder",
		catalog.ActionConfigPutDeliveryChannel, "PutDeliveryChannel",
		catalog.ActionConfigStartConfigurationRecorder, "StartConfigurationRecorder",
		catalog.ActionConfigDescribeComplianceByConfigRule, "DescribeComplianceByConfigRule":
		s.handleConfig(w, r, body, requestID, eventID, action, verified, readOnly)
	default:
		s.writeAWSError(w, requestID, http.StatusNotImplemented, "NotImplemented",
			"This API action is not implemented in Noctaxris Phase 7.", readOnly, r, eventID,
			verified.AccessKeyID, verified.AccountID, true)
	}
}

func bodyLimitForRequest(r *http.Request) int64 {
	if isLikelyNonS3Protocol(r) {
		return maxBodyBytes
	}
	return maxS3BodyBytes
}

func isLikelyNonS3Protocol(r *http.Request) bool {
	if r.Header.Get("X-Amz-Target") != "" {
		return true
	}
	if r.URL.Query().Get("Action") != "" {
		return true
	}
	ct := r.Header.Get("Content-Type")
	if strings.Contains(ct, "application/x-amz-json") {
		return true
	}
	if strings.Contains(ct, "application/x-www-form-urlencoded") {
		return true
	}
	return false
}

// isS3PathStyleRequest detects path-style S3 when Action/X-Amz-Target are absent.
func isS3PathStyleRequest(r *http.Request, body []byte, action string) bool {
	if action != "" {
		return false
	}
	if r.Header.Get("X-Amz-Target") != "" {
		return false
	}
	if r.URL.Query().Get("Action") != "" {
		return false
	}
	if len(body) > 0 {
		vals, err := url.ParseQuery(string(body))
		if err == nil && vals.Get("Action") != "" {
			return false
		}
	}
	ct := r.Header.Get("Content-Type")
	if strings.Contains(ct, "application/x-amz-json") {
		return false
	}
	if strings.Contains(ct, "application/json") {
		return false
	}
	if isLambdaRESTPath(r.URL.Path) {
		return false
	}
	return true
}

func isLambdaRESTPath(path string) bool {
	return strings.HasPrefix(path, "/2015-03-31/")
}

// resolveLambdaREST maps AWS Lambda REST paths to catalog actions and injects
// FunctionName from the URL into the JSON body when missing (CLI REST shape).
func resolveLambdaREST(r *http.Request, body []byte) (action string, outBody []byte) {
	path := strings.TrimSuffix(r.URL.Path, "/")
	parts := strings.Split(strings.TrimPrefix(path, "/"), "/")
	if len(parts) < 2 || parts[0] != "2015-03-31" || parts[1] != "functions" {
		return "", body
	}
	name := ""
	if len(parts) >= 3 {
		name = parts[2]
	}
	switch r.Method {
	case http.MethodPost:
		if len(parts) == 2 {
			return catalog.ActionLambdaCreateFunction, body
		}
		if len(parts) == 4 && parts[3] == "invocations" {
			return catalog.ActionLambdaInvoke, invokeRESTBody(body, name)
		}
	case http.MethodGet:
		if len(parts) == 2 {
			return catalog.ActionLambdaListFunctions, body
		}
		if len(parts) == 3 {
			return catalog.ActionLambdaGetFunction, injectFunctionNameJSON(body, name)
		}
	case http.MethodDelete:
		if len(parts) == 3 {
			return catalog.ActionLambdaDeleteFunction, injectFunctionNameJSON(body, name)
		}
	case http.MethodPut:
		if len(parts) == 4 && parts[3] == "code" {
			return catalog.ActionLambdaUpdateFunctionCode, injectFunctionNameJSON(body, name)
		}
		if len(parts) == 4 && parts[3] == "configuration" {
			return catalog.ActionLambdaUpdateFunctionConfiguration, injectFunctionNameJSON(body, name)
		}
	}
	return "", body
}

func injectFunctionNameJSON(body []byte, name string) []byte {
	if strings.TrimSpace(name) == "" {
		return body
	}
	params := jsonBodyMap(body)
	if params == nil {
		params = map[string]any{}
	}
	if existing, _ := params["FunctionName"].(string); strings.TrimSpace(existing) == "" {
		params["FunctionName"] = name
	}
	raw, err := json.Marshal(params)
	if err != nil {
		return body
	}
	return raw
}

func invokeRESTBody(body []byte, name string) []byte {
	event := any(map[string]any{})
	if len(body) > 0 && json.Valid(body) {
		_ = json.Unmarshal(body, &event)
	}
	raw, err := json.Marshal(map[string]any{
		"FunctionName": name,
		"Payload":      event,
	})
	if err != nil {
		return body
	}
	return raw
}

func (s *Server) writeAWSError(
	w http.ResponseWriter,
	requestID string,
	status int,
	code string,
	message string,
	readOnly bool,
	r *http.Request,
	eventID string,
	accessKeyID string,
	accountID string,
	knownKey bool,
) {
	w.Header().Set(requestIDHeader, requestID)
	w.Header().Set("Content-Type", "application/xml")
	w.WriteHeader(status)

	resp := awsErrorResponse{
		XMLNS: "http://noctaxris.amazonaws.com/doc/2026-07-18/",
		Error: awsError{
			Code:    code,
			Message: message,
			Type:    "Sender",
		},
		RequestID: requestID,
	}
	data, err := xml.Marshal(resp)
	if err == nil {
		_, _ = w.Write([]byte(xml.Header))
		_, _ = w.Write(data)
	}

	eventSource := "noctaxris.amazonaws.com"
	eventName := eventNameForRequest(r)
	if strings.Contains(strings.ToLower(eventName), "getcalleridentity") ||
		strings.Contains(r.Header.Get("X-Amz-Target"), "GetCallerIdentity") {
		eventSource = "sts.amazonaws.com"
		eventName = "GetCallerIdentity"
	}

	ev := audit.Event{
		EventVersion:       eventVersion,
		EventTime:          s.now().UTC().Format(time.RFC3339),
		EventSource:        eventSource,
		EventName:          eventName,
		SourceIPAddress:    clientIP(r),
		UserAgent:          r.UserAgent(),
		RequestID:          requestID,
		EventID:            eventID,
		EventType:          "AwsApiCall",
		RecipientAccountID: s.cfg.AccountID,
		ReadOnly:           readOnly,
		ErrorCode:          code,
		ErrorMessage:       message,
		RequestParameters: map[string]any{
			"httpMethod": r.Method,
			"path":       r.URL.Path,
		},
	}

	if knownKey {
		recipient := s.cfg.AccountID
		if accountID != "" {
			recipient = accountID
		}
		ev.RecipientAccountID = recipient
		ev.UserIdentity = map[string]any{
			"type":        "IAMUser",
			"accountId":   recipient,
			"accessKeyId": accessKeyID,
		}
	}

	_ = s.audit.Write(context.Background(), ev)
}

func readBody(r *http.Request, limit int64) ([]byte, error) {
	if r.Body == nil {
		return nil, nil
	}
	defer r.Body.Close()
	data, err := io.ReadAll(io.LimitReader(r.Body, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > limit {
		return nil, fmt.Errorf("body too large")
	}
	r.Body = io.NopCloser(bytes.NewReader(data))
	return data, nil
}

func resolveAction(r *http.Request, body []byte) string {
	if target := r.Header.Get("X-Amz-Target"); target != "" {
		dot := strings.LastIndex(target, ".")
		if dot < 0 || dot+1 >= len(target) {
			return target
		}
		short := target[dot+1:]
		prefix := target[:dot]
		switch {
		case strings.EqualFold(prefix, "AWSLambda"):
			return lambdaAction(short)
		case strings.EqualFold(prefix, "TrentService"), strings.EqualFold(prefix, "AWSKMS"):
			return normalizeAction(short)
		case strings.EqualFold(prefix, "AmazonSSM"):
			return normalizeAction(short)
		case strings.EqualFold(prefix, "AWSEvents"):
			return eventsAction(short)
		case strings.EqualFold(prefix, "secretsmanager"):
			return secretsAction(short)
		case strings.HasPrefix(strings.ToLower(prefix), "amazonec2containerregistry_v"):
			return ecrAction(short)
		case strings.EqualFold(prefix, "ecr"):
			return ecrAction(short)
		case strings.EqualFold(prefix, "AmazonECS"),
			strings.HasPrefix(strings.ToLower(prefix), "amazonec2containerservice"):
			return ecsAction(short)
		case strings.Contains(strings.ToLower(prefix), "cloudtrail"):
			return cloudtrailAction(short)
		case strings.HasPrefix(strings.ToLower(prefix), "logs_"),
			strings.EqualFold(prefix, "Logs"):
			return logsAction(short)
		case strings.Contains(strings.ToLower(prefix), "resourcegroupstagging"),
			strings.EqualFold(prefix, "tagging"):
			return taggingAction(short)
		case strings.HasPrefix(strings.ToLower(prefix), "kinesis"):
			return kinesisAction(short)
		case strings.Contains(strings.ToLower(prefix), "appconfigdata"):
			return appconfigAction(short)
		case strings.Contains(strings.ToLower(prefix), "appconfig"):
			return appconfigAction(short)
		case strings.Contains(strings.ToLower(prefix), "stepfunctions"),
			strings.EqualFold(prefix, "AWSStepFunctions"):
			return sfnAction(short)
		case strings.HasPrefix(strings.ToLower(prefix), "codebuild"):
			return codebuildAction(short)
		case strings.HasPrefix(strings.ToLower(prefix), "awsbatch"),
			strings.EqualFold(prefix, "Batch"):
			return batchAction(short)
		case strings.HasPrefix(strings.ToLower(prefix), "codepipeline"):
			return codepipelineAction(short)
		case strings.HasPrefix(strings.ToLower(prefix), "firehose"):
			return firehoseAction(short)
		case strings.EqualFold(prefix, "AWSGlue"),
			strings.HasPrefix(strings.ToLower(prefix), "glue"):
			return glueAction(short)
		case strings.Contains(strings.ToLower(prefix), "waf"),
			strings.HasPrefix(strings.ToLower(prefix), "awswaf"):
			return wafAction(short)
		}
		return short
	}
	if v := r.URL.Query().Get("Action"); v != "" {
		return normalizeAction(v)
	}
	if len(body) > 0 {
		vals, err := url.ParseQuery(string(body))
		if err == nil {
			if v := vals.Get("Action"); v != "" {
				return normalizeAction(v)
			}
		}
	}
	return ""
}

func normalizeAction(action string) string {
	if strings.Contains(action, ":") {
		return action
	}
	switch action {
	case "GetCallerIdentity":
		return catalog.ActionSTSGetCallerIdentity
	case "AssumeRole":
		return catalog.ActionSTSAssumeRole
	case "GetSessionToken":
		return catalog.ActionSTSGetSessionToken
	case "GetFederationToken":
		return catalog.ActionSTSGetFederationToken
	case "AssumeRoleWithSAML":
		return catalog.ActionSTSAssumeRoleWithSAML
	case "AssumeRoleWithWebIdentity":
		return catalog.ActionSTSAssumeRoleWithWebIdentity
	case "AssumeRoot":
		return catalog.ActionSTSAssumeRoot
	case "DecodeAuthorizationMessage":
		return catalog.ActionSTSDecodeAuthorizationMessage
	case "GetAccessKeyInfo":
		return catalog.ActionSTSGetAccessKeyInfo
	case "GetDelegatedAccessToken":
		return catalog.ActionSTSGetDelegatedAccessToken
	case "GetWebIdentityToken":
		return catalog.ActionSTSGetWebIdentityToken
	case "CreateAccount":
		return catalog.ActionOrgsCreateAccount
	case "DescribeCreateAccountStatus":
		return catalog.ActionOrgsDescribeCreateAccountStatus
	case "ListAccounts":
		return catalog.ActionOrgsListAccounts
	case "CreateOrganizationalUnit":
		return catalog.ActionOrgsCreateOrganizationalUnit
	case "ListOrganizationalUnitsForParent":
		return catalog.ActionOrgsListOrganizationalUnitsForParent
	case "EnablePolicyType":
		return catalog.ActionOrgsEnablePolicyType
	case "AttachPolicy":
		return catalog.ActionOrgsAttachPolicy
	case "DetachPolicy":
		return catalog.ActionOrgsDetachPolicy
	case "DescribePolicy":
		return catalog.ActionOrgsDescribePolicy
	case "MoveAccount":
		return catalog.ActionOrgsMoveAccount
	case "CreateUser":
		return catalog.ActionIAMCreateUser
	case "GetUser":
		return catalog.ActionIAMGetUser
	case "ListUsers":
		return catalog.ActionIAMListUsers
	case "DeleteUser":
		return catalog.ActionIAMDeleteUser
	case "CreateAccessKey":
		return catalog.ActionIAMCreateAccessKey
	case "DeleteAccessKey":
		return catalog.ActionIAMDeleteAccessKey
	case "ListAccessKeys":
		return catalog.ActionIAMListAccessKeys
	case "UpdateAccessKey":
		return catalog.ActionIAMUpdateAccessKey
	case "CreatePolicy":
		return catalog.ActionIAMCreatePolicy
	case "GetPolicy":
		return catalog.ActionIAMGetPolicy
	case "ListPolicies":
		return catalog.ActionIAMListPolicies
	case "DeletePolicy":
		return catalog.ActionIAMDeletePolicy
	case "AttachUserPolicy":
		return catalog.ActionIAMAttachUserPolicy
	case "DetachUserPolicy":
		return catalog.ActionIAMDetachUserPolicy
	case "AttachRolePolicy":
		return catalog.ActionIAMAttachRolePolicy
	case "DetachRolePolicy":
		return catalog.ActionIAMDetachRolePolicy
	case "ListAttachedUserPolicies":
		return catalog.ActionIAMListAttachedUserPolicies
	case "ListAttachedRolePolicies":
		return catalog.ActionIAMListAttachedRolePolicies
	case "PutUserPolicy":
		return catalog.ActionIAMPutUserPolicy
	case "GetUserPolicy":
		return catalog.ActionIAMGetUserPolicy
	case "DeleteUserPolicy":
		return catalog.ActionIAMDeleteUserPolicy
	case "ListUserPolicies":
		return catalog.ActionIAMListUserPolicies
	case "PutRolePolicy":
		return catalog.ActionIAMPutRolePolicy
	case "GetRolePolicy":
		return catalog.ActionIAMGetRolePolicy
	case "DeleteRolePolicy":
		return catalog.ActionIAMDeleteRolePolicy
	case "ListRolePolicies":
		return catalog.ActionIAMListRolePolicies
	case "CreateRole":
		return catalog.ActionIAMCreateRole
	case "GetRole":
		return catalog.ActionIAMGetRole
	case "ListRoles":
		return catalog.ActionIAMListRoles
	case "DeleteRole":
		return catalog.ActionIAMDeleteRole
	case "UpdateAssumeRolePolicy":
		return catalog.ActionIAMUpdateAssumeRolePolicy
	case "CreateGroup":
		return catalog.ActionIAMCreateGroup
	case "DeleteGroup":
		return catalog.ActionIAMDeleteGroup
	case "GetGroup":
		return catalog.ActionIAMGetGroup
	case "ListGroups":
		return catalog.ActionIAMListGroups
	case "AddUserToGroup":
		return catalog.ActionIAMAddUserToGroup
	case "RemoveUserFromGroup":
		return catalog.ActionIAMRemoveUserFromGroup
	case "AttachGroupPolicy":
		return catalog.ActionIAMAttachGroupPolicy
	case "DetachGroupPolicy":
		return catalog.ActionIAMDetachGroupPolicy
	case "ListAttachedGroupPolicies":
		return catalog.ActionIAMListAttachedGroupPolicies
	case "PutGroupPolicy":
		return catalog.ActionIAMPutGroupPolicy
	case "GetGroupPolicy":
		return catalog.ActionIAMGetGroupPolicy
	case "DeleteGroupPolicy":
		return catalog.ActionIAMDeleteGroupPolicy
	case "ListGroupPolicies":
		return catalog.ActionIAMListGroupPolicies
	case "PutUserPermissionsBoundary":
		return catalog.ActionIAMPutUserPermissionsBoundary
	case "GetUserPermissionsBoundary":
		return catalog.ActionIAMGetUserPermissionsBoundary
	case "DeleteUserPermissionsBoundary":
		return catalog.ActionIAMDeleteUserPermissionsBoundary
	case "PutRolePermissionsBoundary":
		return catalog.ActionIAMPutRolePermissionsBoundary
	case "GetRolePermissionsBoundary":
		return catalog.ActionIAMGetRolePermissionsBoundary
	case "DeleteRolePermissionsBoundary":
		return catalog.ActionIAMDeleteRolePermissionsBoundary
	case "CreateInstanceProfile":
		return catalog.ActionIAMCreateInstanceProfile
	case "DeleteInstanceProfile":
		return catalog.ActionIAMDeleteInstanceProfile
	case "GetInstanceProfile":
		return catalog.ActionIAMGetInstanceProfile
	case "AddRoleToInstanceProfile":
		return catalog.ActionIAMAddRoleToInstanceProfile
	case "RemoveRoleFromInstanceProfile":
		return catalog.ActionIAMRemoveRoleFromInstanceProfile
	case "ListInstanceProfiles":
		return catalog.ActionIAMListInstanceProfiles
	case "CreateOpenIDConnectProvider":
		return catalog.ActionIAMCreateOpenIDConnectProvider
	case "DeleteOpenIDConnectProvider":
		return catalog.ActionIAMDeleteOpenIDConnectProvider
	case "ListOpenIDConnectProviders":
		return catalog.ActionIAMListOpenIDConnectProviders
	case "GetOpenIDConnectProvider":
		return catalog.ActionIAMGetOpenIDConnectProvider
	case "CreateSAMLProvider":
		return catalog.ActionIAMCreateSAMLProvider
	case "DeleteSAMLProvider":
		return catalog.ActionIAMDeleteSAMLProvider
	case "ListSAMLProviders":
		return catalog.ActionIAMListSAMLProviders
	case "GetSAMLProvider":
		return catalog.ActionIAMGetSAMLProvider
	case "CreateVirtualMFADevice":
		return catalog.ActionIAMCreateVirtualMFADevice
	case "EnableMFADevice":
		return catalog.ActionIAMEnableMFADevice
	case "ListMFADevices":
		return catalog.ActionIAMListMFADevices
	case "DeactivateMFADevice":
		return catalog.ActionIAMDeactivateMFADevice
	case "CreateKey":
		return catalog.ActionKMSCreateKey
	case "DescribeKey":
		return catalog.ActionKMSDescribeKey
	case "ListKeys":
		return catalog.ActionKMSListKeys
	case "EnableKey":
		return catalog.ActionKMSEnableKey
	case "DisableKey":
		return catalog.ActionKMSDisableKey
	case "GetKeyPolicy":
		return catalog.ActionKMSGetKeyPolicy
	case "PutKeyPolicy":
		return catalog.ActionKMSPutKeyPolicy
	case "Encrypt":
		return catalog.ActionKMSEncrypt
	case "Decrypt":
		return catalog.ActionKMSDecrypt
	case "GenerateDataKey":
		return catalog.ActionKMSGenerateDataKey
	case "GenerateDataKeyWithoutPlaintext":
		return catalog.ActionKMSGenerateDataKeyWithoutPlaintext
	case "CreateGrant":
		return catalog.ActionKMSCreateGrant
	case "ListGrants":
		return catalog.ActionKMSListGrants
	case "RetireGrant":
		return catalog.ActionKMSRetireGrant
	case "RevokeGrant":
		return catalog.ActionKMSRevokeGrant
	case "CreateAlias":
		return catalog.ActionKMSCreateAlias
	case "ListAliases":
		return catalog.ActionKMSListAliases
	case "DeleteAlias":
		return catalog.ActionKMSDeleteAlias
	case "UpdateAlias":
		return catalog.ActionKMSUpdateAlias
	case "ScheduleKeyDeletion":
		return catalog.ActionKMSScheduleKeyDeletion
	case "CancelKeyDeletion":
		return catalog.ActionKMSCancelKeyDeletion
	case "EnableKeyRotation":
		return catalog.ActionKMSEnableKeyRotation
	case "DisableKeyRotation":
		return catalog.ActionKMSDisableKeyRotation
	case "GetKeyRotationStatus":
		return catalog.ActionKMSGetKeyRotationStatus
	case "CreateTable":
		return catalog.ActionDynamoDBCreateTable
	case "DescribeTable":
		return catalog.ActionDynamoDBDescribeTable
	case "DeleteTable":
		return catalog.ActionDynamoDBDeleteTable
	case "ListTables":
		return catalog.ActionDynamoDBListTables
	case "UpdateTable":
		return catalog.ActionDynamoDBUpdateTable
	case "PutItem":
		return catalog.ActionDynamoDBPutItem
	case "GetItem":
		return catalog.ActionDynamoDBGetItem
	case "DeleteItem":
		return catalog.ActionDynamoDBDeleteItem
	case "UpdateItem":
		return catalog.ActionDynamoDBUpdateItem
	case "Query":
		return catalog.ActionDynamoDBQuery
	case "Scan":
		return catalog.ActionDynamoDBScan
	case "BatchGetItem":
		return catalog.ActionDynamoDBBatchGetItem
	case "BatchWriteItem":
		return catalog.ActionDynamoDBBatchWriteItem
	case "PutResourcePolicy":
		return catalog.ActionDynamoDBPutResourcePolicy
	case "GetResourcePolicy":
		return catalog.ActionDynamoDBGetResourcePolicy
	case "DeleteResourcePolicy":
		return catalog.ActionDynamoDBDeleteResourcePolicy
	case "CreateQueue":
		return catalog.ActionSQSCreateQueue
	case "GetQueueUrl":
		return catalog.ActionSQSGetQueueUrl
	case "GetQueueAttributes":
		return catalog.ActionSQSGetQueueAttributes
	case "SetQueueAttributes":
		return catalog.ActionSQSSetQueueAttributes
	case "DeleteQueue":
		return catalog.ActionSQSDeleteQueue
	case "ListQueues":
		return catalog.ActionSQSListQueues
	case "PurgeQueue":
		return catalog.ActionSQSPurgeQueue
	case "SendMessage":
		return catalog.ActionSQSSendMessage
	case "ReceiveMessage":
		return catalog.ActionSQSReceiveMessage
	case "DeleteMessage":
		return catalog.ActionSQSDeleteMessage
	case "SendMessageBatch":
		return catalog.ActionSQSSendMessageBatch
	case "DeleteMessageBatch":
		return catalog.ActionSQSDeleteMessageBatch
	case "ChangeMessageVisibility":
		return catalog.ActionSQSChangeMessageVisibility
	case "CreateTopic":
		return catalog.ActionSNSCreateTopic
	case "DeleteTopic":
		return catalog.ActionSNSDeleteTopic
	case "ListTopics":
		return catalog.ActionSNSListTopics
	case "GetTopicAttributes":
		return catalog.ActionSNSGetTopicAttributes
	case "SetTopicAttributes":
		return catalog.ActionSNSSetTopicAttributes
	case "Publish":
		return catalog.ActionSNSPublish
	case "Subscribe":
		return catalog.ActionSNSSubscribe
	case "Unsubscribe":
		return catalog.ActionSNSUnsubscribe
	case "ListSubscriptions":
		return catalog.ActionSNSListSubscriptions
	case "ListSubscriptionsByTopic":
		return catalog.ActionSNSListSubscriptionsByTopic
	case "GetSubscriptionAttributes":
		return catalog.ActionSNSGetSubscriptionAttributes
	case "PutParameter":
		return catalog.ActionSSMPutParameter
	case "GetParameter":
		return catalog.ActionSSMGetParameter
	case "GetParameters":
		return catalog.ActionSSMGetParameters
	case "GetParametersByPath":
		return catalog.ActionSSMGetParametersByPath
	case "DeleteParameter":
		return catalog.ActionSSMDeleteParameter
	case "DescribeParameters":
		return catalog.ActionSSMDescribeParameters
	case "CreateSecret":
		return catalog.ActionSecretsCreateSecret
	case "GetSecretValue":
		return catalog.ActionSecretsGetSecretValue
	case "PutSecretValue":
		return catalog.ActionSecretsPutSecretValue
	case "DeleteSecret":
		return catalog.ActionSecretsDeleteSecret
	case "RestoreSecret":
		return catalog.ActionSecretsRestoreSecret
	case "RotateSecret":
		return catalog.ActionSecretsRotateSecret
	case "DescribeSecret":
		return catalog.ActionSecretsDescribeSecret
	case "ListSecrets":
		return catalog.ActionSecretsListSecrets
	case "PutEvents":
		return catalog.ActionEventsPutEvents
	case "CreateEventBus":
		return catalog.ActionEventsCreateEventBus
	case "DeleteEventBus":
		return catalog.ActionEventsDeleteEventBus
	case "DescribeEventBus":
		return catalog.ActionEventsDescribeEventBus
	case "ListEventBuses":
		return catalog.ActionEventsListEventBuses
	case "PutRule":
		return catalog.ActionEventsPutRule
	case "DescribeRule":
		return catalog.ActionEventsDescribeRule
	case "ListRules":
		return catalog.ActionEventsListRules
	case "DeleteRule":
		return catalog.ActionEventsDeleteRule
	case "EnableRule":
		return catalog.ActionEventsEnableRule
	case "DisableRule":
		return catalog.ActionEventsDisableRule
	case "PutTargets":
		return catalog.ActionEventsPutTargets
	case "RemoveTargets":
		return catalog.ActionEventsRemoveTargets
	case "ListTargetsByRule":
		return catalog.ActionEventsListTargetsByRule
	case "CreateFunction":
		return catalog.ActionLambdaCreateFunction
	case "GetFunction":
		return catalog.ActionLambdaGetFunction
	case "DeleteFunction":
		return catalog.ActionLambdaDeleteFunction
	case "ListFunctions":
		return catalog.ActionLambdaListFunctions
	case "UpdateFunctionCode":
		return catalog.ActionLambdaUpdateFunctionCode
	case "UpdateFunctionConfiguration":
		return catalog.ActionLambdaUpdateFunctionConfiguration
	case "Invoke":
		return catalog.ActionLambdaInvoke
	case "PublishVersion":
		return catalog.ActionLambdaPublishVersion
	case "ListVersionsByFunction":
		return catalog.ActionLambdaListVersionsByFunction
	case "GetAlias":
		return catalog.ActionLambdaGetAlias
	case "RegisterTaskDefinition":
		return catalog.ActionECSRegisterTaskDefinition
	case "DescribeTaskDefinition":
		return catalog.ActionECSDescribeTaskDefinition
	case "ListTaskDefinitions":
		return catalog.ActionECSListTaskDefinitions
	case "DeregisterTaskDefinition":
		return catalog.ActionECSDeregisterTaskDefinition
	case "RunTask":
		return catalog.ActionECSRunTask
	case "DescribeTasks":
		return catalog.ActionECSDescribeTasks
	case "ListTasks":
		return catalog.ActionECSListTasks
	case "StopTask":
		return catalog.ActionECSStopTask
	case "DescribeClusters":
		return catalog.ActionECSDescribeClusters
	case "ListClusters":
		return catalog.ActionECSListClusters
	case "LookupEvents":
		return catalog.ActionCloudTrailLookupEvents
	case "CreateLogGroup":
		return catalog.ActionLogsCreateLogGroup
	case "CreateLogStream":
		return catalog.ActionLogsCreateLogStream
	case "PutLogEvents":
		return catalog.ActionLogsPutLogEvents
	case "GetLogEvents":
		return catalog.ActionLogsGetLogEvents
	case "DescribeLogGroups":
		return catalog.ActionLogsDescribeLogGroups
	case "TagResources":
		return catalog.ActionTaggingTagResources
	case "UntagResources":
		return catalog.ActionTaggingUntagResources
	case "GetResources":
		return catalog.ActionTaggingGetResources
	case "CreateStream":
		return catalog.ActionKinesisCreateStream
	case "DeleteStream":
		return catalog.ActionKinesisDeleteStream
	case "DescribeStream":
		return catalog.ActionKinesisDescribeStream
	case "ListStreams":
		return catalog.ActionKinesisListStreams
	case "PutRecord":
		return catalog.ActionKinesisPutRecord
	case "PutRecords":
		return catalog.ActionKinesisPutRecords
	case "GetShardIterator":
		return catalog.ActionKinesisGetShardIterator
	case "GetRecords":
		return catalog.ActionKinesisGetRecords
	case "VerifyEmailIdentity":
		return catalog.ActionSESVerifyEmailIdentity
	case "SendEmail":
		return catalog.ActionSESSendEmail
	case "SendRawEmail":
		return catalog.ActionSESSendRawEmail
	case "ListIdentities":
		return catalog.ActionSESListIdentities
	case "GetSendStatistics":
		return catalog.ActionSESGetSendStatistics
	case "CreateApplication":
		return catalog.ActionAppConfigCreateApplication
	case "CreateEnvironment":
		return catalog.ActionAppConfigCreateEnvironment
	case "CreateConfigurationProfile":
		return catalog.ActionAppConfigCreateConfigurationProfile
	case "CreateHostedConfigurationVersion":
		return catalog.ActionAppConfigCreateHostedConfigurationVersion
	case "GetConfiguration":
		return catalog.ActionAppConfigGetConfiguration
	case "StartConfigurationSession":
		return catalog.ActionAppConfigDataStartConfigurationSession
	case "GetLatestConfiguration":
		return catalog.ActionAppConfigDataGetLatestConfiguration
	case "CreateStateMachine":
		return catalog.ActionSFNCreateStateMachine
	case "DeleteStateMachine":
		return catalog.ActionSFNDeleteStateMachine
	case "DescribeStateMachine":
		return catalog.ActionSFNDescribeStateMachine
	case "ListStateMachines":
		return catalog.ActionSFNListStateMachines
	case "StartExecution":
		return catalog.ActionSFNStartExecution
	case "DescribeExecution":
		return catalog.ActionSFNDescribeExecution
	case "GetExecutionHistory":
		return catalog.ActionSFNGetExecutionHistory
	default:
		return action
	}
}

func isUnauthenticatedSTSAction(action string) bool {
	switch action {
	case catalog.ActionSTSAssumeRoleWithSAML, "AssumeRoleWithSAML",
		catalog.ActionSTSAssumeRoleWithWebIdentity, "AssumeRoleWithWebIdentity":
		return true
	default:
		return false
	}
}

func federationRegion(r *http.Request) string {
	_ = r
	return "us-east-1"
}

func defaultAuthnMessage(code string) string {
	switch code {
	case authn.CodeMissingAuthenticationToken:
		return "The request must contain a valid signature or access key."
	case authn.CodeInvalidClientTokenId:
		return "The security token included in the request is invalid."
	case authn.CodeSignatureDoesNotMatch:
		return "The request signature we calculated does not match the signature you provided."
	case authn.CodeRequestTimeTooSkewed:
		return "The difference between the request time and the server time is too large."
	default:
		return "The request was rejected because authentication failed."
	}
}

func parseAccessKeyID(authorization string) (string, bool) {
	const prefix = "Credential="
	idx := strings.Index(authorization, prefix)
	if idx < 0 {
		return "", false
	}
	rest := authorization[idx+len(prefix):]
	slash := strings.Index(rest, "/")
	if slash <= 0 {
		return "", false
	}
	return rest[:slash], true
}

func eventNameForRequest(r *http.Request) string {
	if target := r.Header.Get("X-Amz-Target"); target != "" {
		if i := strings.LastIndex(target, "."); i >= 0 && i+1 < len(target) {
			return target[i+1:]
		}
		return target
	}
	if v := r.URL.Query().Get("Action"); v != "" {
		return v
	}
	return r.Method + " " + r.URL.Path
}

func clientIP(r *http.Request) string {
	if host, _, ok := strings.Cut(r.RemoteAddr, ":"); ok && host != "" {
		return host
	}
	return r.RemoteAddr
}

func newRequestID() string {
	return uuid.NewString()
}
