# Changelog

## v5 (lab cores shipped)

EventBridge Scheduler lite, Lambda SQS ESM and Function URLs, SNS FIFO/HTTP depth, ECS CreateService, DynamoDB Streams, EventBridge Pipes, and nine more lab services toward ~40 total. Verification bar is `go test ./...`. Per-service Compose CLI smoke lives on each `docs/services/` page (operator-run, not claimed as CI). Nested Amazon MQ broker and Cognito remain deferred.

### Included

- EventBridge Scheduler: distinct `scheduler` API with rate/cron/`at` subset, Lambda/SQS/SNS targets, PassRole, in-process ticker
- SNS FIFO topics (MessageGroupId / dedup) and HTTP(S) subscriptions to loopback catcher only (deny-by-default egress)
- Lambda SQS event source mapping with in-process ReceiveMessage → sync Invoke → DeleteMessage on success
- Lambda Function URLs lite (`NONE` or `AWS_IAM`) on `/lambda-url/ACCOUNT/FUNCTION`
- Lambda layers mounted under `/opt` for Image Invoke (parity with zip)
- ECS CreateService / UpdateService / DeleteService / DescribeServices / ListServices with DesiredCount reconciler
- DynamoDB Streams lite: enable stream, DescribeStream / GetShardIterator / GetRecords
- EventBridge Pipes lite: SQS or DynamoDB Streams source to Lambda or SQS target
- Amazon MQ lite: broker CRUD control-plane with loopback stub endpoint (no nested broker)
- Transfer Family lite: server/user CRUD and SFTP-shaped sandbox under the data root
- ACM lite: RequestCertificate and lab self-signed PEM
- Route 53 lite: hosted zones plus A/CNAME record changes
- Cloud Map lite: namespace/service/instance register and DiscoverInstances
- Pricing lite: GetProducts over a static embedded catalog
- AppSync lite: GraphQL API CRUD, schema, Lambda data source, API_KEY or IAM auth (no Cognito)
- CloudWatch Logs delete and DescribeLogStreams polish

Deferred depth: [docs/services/index.md](docs/services/index.md). Nested MQ broker, Cognito User Pools, API Gateway HTTP API, IoT Core, MSK, and live Firecracker guest boot remain deferred.

## v4 (lab cores shipped)

Opt-in microVM selection (DinD remains default), data-plane depth on KMS/S3/DynamoDB/SQS/Secrets/SSM, and eight more lab services toward ~30 total. Verification bar is `go test ./...`. Per-service Compose CLI smoke lives on each `docs/services/` page (operator-run, not claimed as CI). Live Firecracker guest boot needs a Linux+KVM host with kernel/rootfs assets (stubs and platform matrix ship on all hosts).

### Included

- Compute runtime selection: `NOCTAXRIS_COMPUTE_RUNTIME=dind|microvm` (default DinD). Opt-in microVM probes KVM and Firecracker binary, fails closed on WSL2 or missing assets, never falls through to host Docker
- Lambda zip/Image Invoke and ECS RunTask route to the microVM runner when opted in (fail-closed stubs until live guest boot on Linux+KVM)
- KMS on-read sweeper after DeletionDate and key-material rotation (enable rotates sealed material, lab auto-rotate by period)
- DynamoDB up to two lab GSIs per table via CreateTable / UpdateTable
- S3 versioning lite: Put/GetBucketVersioning, version-aware Get/Put, ListObjectVersions lite
- SQS DelaySeconds depth and DLQ RedriveAllowPolicy enforcement
- Secrets Manager RotateSecret (lab random replacement), recovery window on delete with RestoreSecret, on-read sweeper
- SSM GetParametersByPath with path hierarchy and Recursive
- CloudFormation lite: CreateStack / DescribeStacks / DeleteStack / ListStacks for AWS::S3::Bucket and AWS::IAM::Role, PassRole when RoleARN set
- CodeBuild nested: CreateProject, StartBuild, BatchGetBuilds, ListBuilds on DinD with PassRole for `codebuild.amazonaws.com`
- CodePipeline lite: CreatePipeline / StartPipelineExecution / GetPipelineState with a CodeBuild action that calls nested StartBuild, PassRole when roleArn set
- Batch lite: CreateComputeEnvironment, CreateJobQueue, RegisterJobDefinition, SubmitJob, Describe* on nested DinD with PassRole for service and job roles
- Firehose lite: delivery stream CRUD, PutRecord(s) to S3 and Lambda (async Invoke enqueue), PassRole when RoleARN set
- Glue Data Catalog lite: database and table CRUD over sqlite
- WAF v2 lite: WebACL / rule group shape, AssociateWebACL, labeled Evaluate helper
- Config lite: recorder / delivery channel, StartConfigurationRecorder, DescribeComplianceByConfigRule stub over tagged resources

Deferred depth: [docs/services/index.md](docs/services/index.md). Athena remains deferred. Live Firecracker guest boot needs Linux+KVM plus kernel/rootfs assets.

## v3 (lab cores shipped)

Multi-account honesty (cross-account dual eval, OU SCP/RCP inheritance, request-context keys) plus first expansion wave services. Verification bar is `go test ./...`. Per-service Compose CLI smoke lives on each `docs/services/` page (operator-run, not claimed as CI).

### Included

- Cross-account resource dual evaluation for S3, SQS, Lambda, ECR, SNS, KMS, Secrets Manager, and DynamoDB (same-account identity or resource policy Allow, cross-account both must Allow)
- Organizations `MoveAccount` and SCP/RCP inheritance along the OU path from account parent to root (management account still exempt)
- Lambda Image pull of lab ECR `127.0.0.1:4566/ACCOUNT/REPO:tag` with Registry V2 auth (parity with ECS RunTask)
- Request-context population for cataloged keys including `aws:SourceIp`, `aws:PrincipalArn`, `aws:ResourceTag/*`, and matching service ResourceTag keys where tags exist
- CloudTrail `LookupEvents` over local `$DATAROOT/cloudtrail/events.jsonl` with time range and LookupAttributes filters
- CloudWatch Logs lab core: CreateLogGroup, CreateLogStream, PutLogEvents, GetLogEvents, DescribeLogGroups
- Resource Groups Tagging API lab core: TagResources, UntagResources, GetResources over a central ARN tag map
- Kinesis Data Streams lab core: Create/Delete/Describe/ListStreams, PutRecord(s), GetShardIterator, GetRecords (single shard)
- SES local catcher: VerifyEmailIdentity, SendEmail/SendRawEmail persist, ListIdentities, GetSendStatistics stub
- AppConfig lab core: CreateApplication/Environment/ConfigurationProfile, hosted versions, GetConfiguration, AppConfigData StartConfigurationSession/GetLatestConfiguration
- Step Functions lab core: Create/Delete/Describe/List state machines, StartExecution/DescribeExecution/GetExecutionHistory with Pass/Succeed/Fail/Task to Lambda

Deferred depth: [docs/services/index.md](docs/services/index.md).

## v2 (lab cores shipped)

Cleared in-scope deferred depth for the lab core (except microVMs), then shipped lab-complete SSM Parameter Store, Secrets Manager, SNS, EventBridge, ECR, and ECS. Verification bar is `go test ./...`. Per-service Compose CLI smoke lives on each `docs/services/` page (operator-run, not claimed as CI).

### Included

- Condition-key catalogs for lab-core services from servicereference JSON plus global seed
- Authz Condition rules per ADR-0005 §7 (catalog-unknown deny, unpopulated known fail-closed on positive operators, AWS-faithful Null / StringNotEquals / IfExists)
- IAM groups (membership, managed and inline group policies), permissions boundaries for users and roles, instance profiles, OIDC and SAML IdP CRUD
- `EvaluateFull` identity plus boundary plus SCP and RCP on shared `authorize` and on S3/KMS/DynamoDB/SQS authorize plus Lambda PassRole
- Organizations ListAccounts, OUs, EnablePolicyType, SCP and RCP CreatePolicy / AttachPolicy / DetachPolicy / DescribePolicy
- IAM virtual MFA devices, lab MFA token validation, `GetSessionToken` without IAM permission gate, MFA sessions set `aws:MultiFactorAuthPresent`
- KMS ReEncrypt, ScheduleKeyDeletion/CancelKeyDeletion/PendingDeletion (cancel restores `Disabled` per AWS), rotation enable/status flags, lab `alias/aws/s3|dynamodb|sqs` convenience aliases
- S3 multipart upload (5 MiB minimum for non-final parts on Complete), CopyObject (same account), Put/Get/DeleteBucketEncryption with default SSE inheritance
- DynamoDB one lab GSI, Update/DescribeTimeToLive with lazy expiry on read
- SQS FIFO queues with deduplication and RedrivePolicy move-to-DLQ
- SSM Parameter Store String and SecureString parameters, Put/Get/GetParameters/Delete/Describe, KMS via KeyId or lab `alias/aws/ssm`, identity EvaluateFull authz
- Secrets Manager create/get/put/delete/describe/list, secret resource policies, KMS via lab `alias/aws/secretsmanager`, identity-or-resource-policy authz (immediate delete, no recovery window)
- Lambda versions and aliases with Invoke qualifier resolution (`name`, `name:version`, `name:alias`)
- Lambda layers (max 5 same-account ARNs, merged at `/opt` on zip Invoke)
- Lambda async Invoke (`InvocationType=Event`) with two lab retries and SQS DLQ / OnFailure destinations
- Lambda container image packaging (`PackageType=Image`, `Code.ImageUri`)
- Lambda zip runtimes `python3.11`, `python3.12`, `nodejs20.x`
- Nested DinD engine TLS on port 2376 (`NOCTAXRIS_DOCKER_CERT_PATH`)
- Same-account Lambda function resource policies (`AddPermission`, `RemovePermission`, `GetPolicy`) with identity-or-policy Invoke authz
- SNS topic CRUD, publish, subscribe, topic policies (`EvaluateSNS` identity-or-policy authz), confirmed sqs and lambda delivery with best-effort retry and failure logging. Lambda async `DestinationConfig.OnFailure` accepts SNS topic ARNs as well as SQS
- EventBridge buses, rules, targets, and PutEvents routing to SQS, Lambda, and SNS. PassRole on PutTargets RoleArn with `events.amazonaws.com` trust. RoleArn delivery mints a role session and requires role identity Allow. Without RoleArn, target resource policy must Allow `events.amazonaws.com` or account root. Matches recorded only when delivery is authorized
- ECR repository CRUD, GetAuthorizationToken, repository policies, image metadata APIs, and Docker Registry V2 on loopback with token auth. DinD sync on manifest put for ECS and Lambda Image pulls
- ECS task definitions (taskRoleArn and executionRoleArn required), RunTask/Describe/List/Stop, default cluster, PassRole with `ecs-tasks.amazonaws.com` trust, nested DinD on Internal `noctaxris-ecs` network, task-role credential injection, task status sync to STOPPED when the container exits

Deferred depth: [docs/services/index.md](docs/services/index.md).

## Lab core

First shippable service set for Docker-first local AWS-shaped labs.

### Included

- Secure Compose defaults: loopback publish, no host `docker.sock`, sealed secrets
- SigV4 (header and query), IAM lab APIs, all 11 STS actions, Organizations CreateAccount
- Lab-complete KMS (keys, policies, crypto, grants, aliases)
- Lab-complete S3 (path-style objects, bucket policy, SSE-S3/SSE-KMS, presigned GET/PUT)
- Lab-complete DynamoDB and SQS (items/messages, resource/queue policies, encryption options)
- Lab-complete Lambda (zip/Image, versions/aliases, layers, sync+async Invoke, resource policies, TLS DinD)

### Explicitly later

See [docs/services/](docs/services/index.md) for remaining SAR depth and later lab cores.
