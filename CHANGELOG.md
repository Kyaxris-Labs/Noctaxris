# Changelog

## Unreleased

### Compute path honesty

- Removed opt-in microVM / Firecracker selection path. Nested DinD via Compose `noctaxris-engine` is the only packaged compute plane
- `NOCTAXRIS_COMPUTE_RUNTIME` accepts only `dind` (or unset); `NOCTAXRIS_FIRECRACKER_BIN` removed
- Docs and README describe DinD-only compute; privilege reduction for the nested engine is planned

### Integration suites

- Real Compose-backed suites under `tests/`: Go / Node.js / Python AWS SDK round-trips, Terraform apply/destroy (S3 + IAM + DynamoDB + KMS), CloudFormation JSON/YAML stack CRUD for lab resource types
- Optional CI job `integration-suites` (`workflow_dispatch` or `tests/**` path filter); not a required PR gate
- Run guide: [tests/README.md](tests/README.md)
- CloudFormation CreateStack: provision S3/IAM resources before the SQLite write transaction (avoids `SQLITE_BUSY` under the open tx)
- IAM `ListInstanceProfilesForRole` (empty list when none; needed for Terraform `aws_iam_role` destroy)
- DynamoDB `DescribeContinuousBackups` lab stub; KMS `ListResourceTags` / `TagResource` / `UntagResource` (Terraform provider v5 post-create)
- CloudFormation YAML `TemplateBody`, lab intrinsics (`Ref` / `Fn::GetAtt` / `Fn::Sub` / `Fn::Join`), types SQS / DynamoDB / Lambda (`ZipFile`)

### CTF fidelity

- IAM managed policy versions: `CreatePolicyVersion`, `GetPolicyVersion`, `ListPolicyVersions`, `DeletePolicyVersion`, `SetDefaultPolicyVersion` (five-version cap; default document syncs into Evaluate)
- Lambda layers REST: map `/2018-10-31/layers/...` and `/2015-03-31/layers/...` for CLI Publish/Get/List/Delete layer version
- Function network: masquerade-off bridge so in-function SDK calls reach `host.docker.internal` lab API without WAN SNAT
- Compose `noctaxris-compute-init`: chown `noctaxris-compute` to UID `65532` before CreateFunction zip writes
- SNS schema upgrade: ALTER FIFO columns before creating the dedup index (old volumes no longer crash on boot)

### Platform depth

- RDS Data API: ExecuteStatement runs real SQL via nested `psql` (DinD exec) when a Postgres container was started; stub marker when DinD is unset (still no `pgx`)
- Nested DinD smoke: `docker/smoke-nested.sh` (manual only: Actions `workflow_dispatch` with `nested_smoke=true`, or run the script locally). Not on push/PR; a green PR proves `smoke-core` only. See [docs/ops.md](docs/ops.md).

### Security hardening (H1)

- JWT / AppSync issuers: lab Cognito JWKS only by default; remote JWKS behind `NOCTAXRIS_ALLOW_REMOTE_JWKS` + public host allowlist (no redirects / no RFC1918)
- `NOCTAXRIS_DOCKER_HOST` allowlist (default `tcp://noctaxris-engine:2376`); reject `unix://`, `npipe://`, `docker.sock`; TLS client PEMs required when host is set
- Compose volume split: API-only `noctaxris-data` vs `noctaxris-compute` for Lambda code (engine cannot read `master.key`)
- Image pull allowlist for DinD (lab registry + pinned lab bases; `NOCTAXRIS_IMAGE_PULL_ALLOWLIST` for extras with digest pins)
- SigV4 requires `host` in SignedHeaders; SNS HTTP limited to lab catcher on `:4566` (or `NOCTAXRIS_SNS_HTTP_ALLOWLIST`)
- Non-loopback listen without TLS fails closed unless `NOCTAXRIS_ALLOW_NONLOOPBACK_LISTEN=1`; Function URL / HTTP API `NONE` gated on non-loopback via `NOCTAXRIS_ALLOW_OPEN_DATA_PLANE`
- AppSync API keys hashed at rest (HMAC with master key); Gateway JWT enforces `nbf`; CredentialsArn evaluated at invoke; `iam:PassedToService` on PassRole
- Internal network reuse inspect; optional `NOCTAXRIS_INJECT_HOST_GATEWAY=0`; nested data start fail → `failed`; CodeBuild/Batch non-zero exit → Failed

### Security + edge (round 2)

- Compose default: leave `NOCTAXRIS_ALLOW_OPEN_DATA_PLANE` unset; keep `ALLOW_NONLOOPBACK_LISTEN` with loud docs for the container bind
- WAF Associate requires an existing Web ACL; invoke association evaluate errors fail closed (403)
- HTTP API (no CredentialsArn) and AppSync Lambda invoke require resource policy Allow for `apigateway.amazonaws.com` / `appsync.amazonaws.com`
- SNS HTTP allowlist applies private/metadata host rejects and does not follow redirects
- Remote JWKS dial pins to IPs re-checked at connect time; Function URL / AppSync IAM require matching SigV4 service names

### Compute (round 2)

- Lambda `Environment` rejects reserved keys; Invoke overlays minted execution-role credentials and lab endpoints last
- Batch `jobRoleArn` / CodeBuild `serviceRole` mint in-container AWS_* sessions; lab registry rewrite + auth pull on those paths
- Zip Invoke uses per-invoke scratch for event + merged layers; Timeout 1–900s and MemorySize 128–10240 MB clamps; nested CapDrop ALL
- ECS / CodeBuild / Batch host-gateway ExtraHosts default off (`NOCTAXRIS_INJECT_ECS_HOST_GATEWAY=1` to opt in); docs honesty for privileged DinD
- Unpacked Lambda/layer trees and invoke scratch use `0755`/`0644` so nested DinD can read API-owned (UID `65532`) bind mounts

## Nested data planes and ML stubs

Nested data planes (RDS Postgres, ElastiCache, DocumentDB) via DinD without host DB ports, RDS Data API stub on `:4566`, Athena over Glue and lab S3, OpenSearch/EMR control-plane stubs, and Bedrock/Textract/Transcribe shape stubs. At ship time, verification was `go test ./...` plus operator-run per-service Compose CLI smoke on each `docs/services/` page. Live CI contract (PR `smoke-core`, manual nested smoke): [docs/ops.md](docs/ops.md).

### Included

- Nested data-plane helper on the existing DinD TLS client (labeled containers, no host port publish)
- RDS lite: CreateDBInstance / DescribeDBInstances / DeleteDBInstance for Postgres, Secrets Manager master secret, nested start when engine is up
- RDS Data API lite: ExecuteStatement plus Begin/Commit/Rollback with resourceArn and secretArn validation (nested `psql` when DinD started Postgres; stub otherwise; no pgx)
- ElastiCache lite: Create/Describe/Delete cache cluster (redis or valkey), nested Valkey/Redis when DinD is up
- DocumentDB lite: Create/Describe/Delete DB cluster (`Engine=docdb`), nested Mongo-compatible when DinD is up (not Neptune)
- Athena lite: Start/Get/Stop/GetQueryResults over Glue catalog plus lab S3 CSV/JSON SELECT subset
- Glue catalog polish: PartitionKeys and StorageDescriptor SerDe/InputFormat fields for Athena
- OpenSearch lite: domain CRUD with loopback stub endpoint
- EMR lite: RunJobFlow / DescribeCluster / ListClusters / TerminateJobFlows control-plane stub
- Bedrock Runtime stub: InvokeModel allowlisted modelIds to canned JSON
- Textract stub: DetectDocumentText / AnalyzeDocument canned Blocks
- Transcribe stub: Start/Get/List transcription jobs with canned transcript under the data root

Deferred depth: [docs/services/index.md](docs/services/index.md). Wire-protocol `pgx` Data API executor, MemoryDB, Neptune, and real ML model runtimes remain deferred.

## Cognito, HTTP API, and edge stubs

Cognito User Pools and API Gateway HTTP API (JWT + IAM authorizers) on the existing loopback listener, AppSync Cognito auth, edge/governance stubs, and go-jose v4 for the single JWT stack. At ship time, verification was `go test ./...` plus operator-run per-service Compose CLI smoke on each `docs/services/` page. Live CI contract (PR `smoke-core`, manual nested smoke): [docs/ops.md](docs/ops.md).

### Included

- go-jose upgrade to `github.com/go-jose/go-jose/v4` with shared RS256 JWKS verify helper (federation + Cognito/Gateway/AppSync)
- Cognito User Pools lite: pool/client CRUD, AdminCreateUser / SignUp, InitiateAuth USER_PASSWORD_AUTH, RS256 ID and access tokens, JWKS on `:4566`
- API Gateway HTTP API lite: Api/Integration/Authorizer/Route/Stage, Lambda AWS_PROXY, NONE/JWT/IAM authorizers, optional CredentialsArn PassRole, invoke on `/http-api/...`
- AppSync `AMAZON_COGNITO_USER_POOLS` Bearer JWT auth beside API_KEY and AWS_IAM
- WAF AssociateWebACL accepts lab HTTP API ARNs (fail closed on unknown). Function URL NONE CORS lite
- CloudFront distribution stub (S3 or Gateway origin strings)
- ELBv2 lite (lambda/ip targets only)
- S3 Vectors lite (put/query cosine or euclidean)
- Cloud Control lite for AWS::S3::Bucket and AWS::IAM::Role
- BCM Data Exports lite (sample file under data root)
- Cost Explorer lite (seeded GetCostAndUsage / GetCostForecast)
- Budgets lite (CRUD, notification stubs stored only)
- CodeDeploy lite (sync Succeeded, optional ECS DesiredCount hook, PassRole)

Deferred depth: [docs/services/index.md](docs/services/index.md). REST API v1, Cognito Identity Pools / Hosted UI, and real CloudFront PoPs remain deferred.

## Scheduler, Streams, Pipes, and edge lite

EventBridge Scheduler lite, Lambda SQS ESM and Function URLs, SNS FIFO/HTTP depth, ECS CreateService, DynamoDB Streams, EventBridge Pipes, and additional edge/control-plane lab services. At ship time, verification was `go test ./...` plus operator-run per-service Compose CLI smoke on each `docs/services/` page. Live CI contract (PR `smoke-core`, manual nested smoke): [docs/ops.md](docs/ops.md). Nested Amazon MQ broker remains deferred.

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
- AppSync lite: GraphQL API CRUD, schema, Lambda data source, API_KEY or IAM auth (Cognito User Pools auth landed later)
- CloudWatch Logs delete and DescribeLogStreams polish

Deferred depth: [docs/services/index.md](docs/services/index.md). Nested MQ broker, IoT Core, and MSK remain deferred.

## Data-plane depth and CI/CD lite

Data-plane depth on KMS/S3/DynamoDB/SQS/Secrets/SSM, and CI/CD plus catalog lab services. At ship time, verification was `go test ./...` plus operator-run per-service Compose CLI smoke on each `docs/services/` page. Live CI contract (PR `smoke-core`, manual nested smoke): [docs/ops.md](docs/ops.md). Nested compute stayed on DinD via Compose `noctaxris-engine`.

### Included

- Nested DinD compute path for Lambda Invoke and ECS RunTask (never falls through to host Docker)
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

Deferred depth: [docs/services/index.md](docs/services/index.md). Athena shipped in a later release.

## Multi-account honesty and audit services

Multi-account honesty (cross-account dual eval, OU SCP/RCP inheritance, request-context keys) plus first expansion wave services. At ship time, verification was `go test ./...` plus operator-run per-service Compose CLI smoke on each `docs/services/` page. Live CI contract (PR `smoke-core`, manual nested smoke): [docs/ops.md](docs/ops.md).

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

## Identity depth and messaging / container labs

Cleared in-scope deferred depth for the lab core, then shipped lab-complete SSM Parameter Store, Secrets Manager, SNS, EventBridge, ECR, and ECS. At ship time, verification was `go test ./...` plus operator-run per-service Compose CLI smoke on each `docs/services/` page. Live CI contract (PR `smoke-core`, manual nested smoke): [docs/ops.md](docs/ops.md).

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
