# Services

Per-service reference for the Noctaxris lab emulator. Status matches the root [README](../../README.md) Services table.

Each page covers what is implemented, how to verify with AWS CLI smoke, what remains deferred for lab-core, and what is out of lab scope or will not ship.

| Service | Status | Doc |
|---------|--------|-----|
| [IAM](iam.md) | Shipped | Users, roles, policies, managed policy versions (max five), keys, groups, boundaries, instance profiles including ListInstanceProfilesForRole, IdPs, MFA, GetAccessKeyLastUsed, credential report |
| [STS](sts.md) | Shipped | All 11 actions (lab MFA on GetSessionToken) |
| [Organizations](organizations.md) | Shipped | Accounts, OUs, MoveAccount, SCP/RCP attach with OU-path inheritance, ListPolicies / ListPoliciesForTarget / ListParents / ListAccountsForParent |
| [KMS](kms.md) | Shipped | CMKs, key-policy-required crypto, cross-account dual eval, grants, lab aliases, tags, deletion sweeper, key-material rotation |
| [S3](s3.md) | Shipped | Path-style objects, multipart, CopyObject, bucket encryption, versioning lite with delete markers, Object Lock lite, server access logging, bucket notifications (Lambda/SQS/EventBridge/SNS emit; empty=off), cross-account dual eval |
| [DynamoDB](dynamodb.md) | Shipped | Tables, items, up to two lab GSIs, BatchGet/BatchWrite, TransactWrite/TransactGet (same-account Put/Delete/Update/ConditionCheck + ConditionExpression, ClientRequestToken, stream append), TTL, DescribeContinuousBackups stub, resource policies, cross-account dual eval, stream enablement |
| [DynamoDB Streams](dynamodbstreams.md) | Shipped | Enable stream, List/Describe, GetShardIterator/GetRecords, NEW_IMAGE / OLD_IMAGE / NEW_AND_OLD_IMAGES / KEYS_ONLY; Lambda ESM + FilterCriteria (including OldImage) in [lambda.md](lambda.md) |
| [SQS](sqs.md) | Shipped | Standard and FIFO queues, DelaySeconds, RedrivePolicy and RedriveAllowPolicy (DLQ provenance attribute), policies, cross-account dual eval |
| [Lambda](lambda.md) | Shipped | Zip/Image (runtimes `python3.11`–`3.14`, `nodejs20.x`/`22.x`/`24.x`, `java21`/`java25`), versions/aliases, layers on zip and Image, SQS / DynamoDB Streams / multi-shard Kinesis / Amazon MQ ESM (RUNNING broker; allowlisted hosts; RabbitMQ `basic.get` bodies; ActiveMQ AMQP 1.0 `queue://noctaxris` bodies) with FilterCriteria (EventBridge operators), Function URLs, sync+async Invoke, service-principal XA AddPermission (SourceAccount/SourceArn), lab ECR Image pull, TLS DinD |
| [SSM Parameter Store](ssm.md) | Shipped | String, StringList, and SecureString, GetParametersByPath hierarchy, KMS via alias/aws/ssm, identity authz |
| [Secrets Manager](secretsmanager.md) | Shipped | CRUD, list, version stages (AWSCURRENT/AWSPENDING/AWSPREVIOUS), RotateSecret (random default or optional four-step Lambda rotator with PassRole for secretsmanager.amazonaws.com), RotationRules (AutomaticallyAfterDays or rate/cron ScheduleExpression including DOM `nW`/`LW` + optional Duration) with RotateImmediately=false and in-process due ticker, recovery window, resource policies, cross-account dual eval, KMS via alias/aws/secretsmanager |
| [SNS](sns.md) | Shipped | Topic CRUD including FIFO, publish, SQS/Lambda/HTTP loopback subscribe, lab FilterPolicy + RawMessageDelivery + RedrivePolicy DLQ, topic policies, XA Subscribe + foreign SQS delivery |
| [EventBridge](eventbridge.md) | Shipped | Buses, rules, targets (optional DeadLetterConfig SQS + store delivery history), PutEvents to SQS/Lambda/SNS/Logs/Kinesis/SFN; content filters (prefix/suffix/exists/anything-but/numeric/equals-ignore-case); RoleArn or resource-policy delivery for SQS/Lambda/SNS/Logs/Kinesis/SFN; InputPath + InputTransformer; bus-policy dual-eval |
| [EventBridge Scheduler](scheduler.md) | Shipped | Schedule CRUD, rate/cron/at subset (DOM `nW`/`LW`), Lambda/SQS/SNS/SFN targets, in-process ticker, PassRole with schedule `aws:SourceArn` |
| [EventBridge Pipes](pipes.md) | Shipped | Pipe CRUD; SQS / DynamoDB Streams / EventBridge bus source; optional Lambda enrichment; optional DeadLetterArn; ticker + RoleArn/target policy; PassRole with pipe `aws:SourceArn` |
| [Amazon MQ](mq.md) | Shipped | Broker CRUD; nested RabbitMQ or ActiveMQ when DinD up (`RUNNING`); no-DinD → `CREATION_FAILED` stub; Lambda ESM when RUNNING (Rabbit + ActiveMQ AMQP 1.0 bodies) |
| [Transfer Family](transfer.md) | Shipped | Server/user CRUD; ONLINE; lab Put/Get/List file API on `:4566` HTTP (JSON + `/transfer/.../home/...`; not real SFTP); PassRole on Role |
| [ECR](ecr.md) | Shipped | Repository CRUD, auth token, policies, cross-account dual eval, Registry V2 (monolithic PUT + chunked PATCH), DinD sync |
| [ECS](ecs.md) | Shipped | Task definitions, RunTask/list/stop, CreateService DesiredCount reconciler, PassRole, nested DinD |
| [EC2](ec2.md) | Shipped | Lab RunInstances nested DinD (`noctaxris-ec2`); Describe/Stop/Start/Terminate; AMI→allowlisted image map; VPC Flow under same service |
| [EKS](eks.md) | Shipped | Create/Describe/List/DeleteCluster REST; metadata-only ACTIVE + nested endpoint string; empty ListNodegroups; no live kubectl |
| [CloudTrail](cloudtrail.md) | Shipped | LookupEvents (incl. EventCategory=insight); CreateTrail + StartLogging continuous AWSLogs hive delivery (+ optional gzip); selectors lite; digests + ValidateLogs; org trail flag; lab InjectEvents / InjectInsightsEvents; richer audit + sibling KMS Decrypt |
| [GuardDuty](guardduty.md) | Shipped | Create/ListDetectors; List/GetFindings; lab InjectFindings (opt-in) |
| [Security Hub](securityhub.md) | Shipped | BatchImportFindings + GetFindings lite (ASFF-lite) |
| [Detective](detective.md) | Shipped | CreateGraph/ListGraphs/AcceptInvitation; lab SearchGraph over CT + GuardDuty |
| [Macie](macie.md) | Shipped | EnableMacie/GetMacieSession; classification job lite; List/GetFindings; lab InjectFindings with canned S3 matches (opt-in) |
| [Control Tower](controltower.md) | Stub | ListLandingZones empty; GetLandingZone not found |
| [VPC Flow Logs](vpcflow.md) | Shipped | CreateFlowLogs lite (opaque IDs); InjectFlowLogs to S3/Logs (opt-in); see also [EC2](ec2.md) |
| [CloudWatch Logs](logs.md) | Shipped | Groups/streams, Put/DeleteRetentionPolicy, Put/GetLogEvents, FilterLogEvents (lab filterPattern + JSON field equality), account resource policies, XA subscription filters (awslogs envelope), metric filters lite; Lambda `/aws/lambda/*` START/END/REPORT |
| [CloudWatch Metrics](cloudwatch.md) | Shipped | PutMetricData/ListMetrics/GetMetricStatistics/GetMetricData (MetricStat); Put/Describe/DeleteAlarms + SetAlarmState |
| [Resource Groups Tagging API](resourcegroupstaggingapi.md) | Shipped | TagResources, UntagResources, GetResources |
| [Kinesis Data Streams](kinesis.md) | Shipped | Stream CRUD with ShardCount 1..4, Put/Get records per shard, stream resource policy; Lambda ESM in [lambda.md](lambda.md) |
| [Firehose](firehose.md) | Shipped | Delivery stream CRUD, PutRecord(s) to S3, Lambda, nested OpenSearch (Active domain + RoleARN; skip-without-engine), or lab VPC Flow dest; RoleARN session or destination policy on Put |
| [SES](ses.md) | Shipped | Local catcher: verify, SendEmail/SendRawEmail, ListIdentities, SetIdentityNotificationTopic Bounce, GetSendStatistics |
| [AppConfig](appconfig.md) | Shipped | Application/Environment/Profile, hosted versions, StartDeployment (immediate DEPLOYED), GetConfiguration / AppConfigData from deployed pointer |
| [Step Functions](stepfunctions.md) | Shipped | State machine CRUD, StartExecution, Pass/Succeed/Fail/Choice (Equals + GreaterThan/LessThan/IsPresent)/Wait (0–5s)/Parallel/Map (sequential)/Task to Lambda/SQS/SNS/EventBridge; InputPath/ResultPath; lab resource policy for EventBridge RoleArn-less StartExecution |
| [CloudFormation](cloudformation.md) | Shipped | Stack CRUD; ChangeSet Add/Remove/allowlisted Modify; S3/IAM/SQS/DynamoDB/Lambda/KMS/SNS/Logs/Events/SSM/Secrets (+ BucketPolicy, ManagedPolicy, Permission+FunctionUrlAuthType, NotificationConfiguration, TopicPolicy, Subscription+FilterPolicy, LogGroup RetentionInDays, Alias, User/Group); JSON/YAML; lab intrinsics |
| [CodeBuild](codebuild.md) ([lab image](codebuild-lab-image.md)) | Shipped | Project CRUD, StartBuild/StartBuildBatch, StopBuild, BatchGetBuilds/ListBuilds, Create/Delete/ListWebhooks; override-lock; sources `NO_SOURCE`/`S3`/`CODECOMMIT`; S3 artifacts (workspace `/codebuild/src` tar when present, else logs); lab IMDS `FULL_URI` on `:9254`; lab webhook receive POST; optional tool image under `docker/codebuild-lab/` |
| [CodeCommit](codecommit.md) | Shipped | Lab repo/file store (Create/Get/List/DeleteRepository, PutFile/GetFile/GetFolder); CodeBuild `CODECOMMIT` StartBuild materializes; not git smart-HTTP |
| [CodePipeline](codepipeline.md) | Shipped | Pipeline CRUD, StartPipelineExecution with nested CodeBuild StartBuild, GetPipelineState |
| [Batch](batch.md) | Shipped | Compute environment / queue / definition lite, SubmitJob on nested DinD |
| [Glue](glue.md) | Shipped | Data Catalog database and table CRUD (PartitionKeys + SerDe fields for Athena); crawler lite Create/Start/Get/Delete/List |
| [WAF v2](wafv2.md) | Shipped | WebACL / rule group lite; AssociateWebACL (HTTP API / AppSync / Lambda / ALB); ByteMatch / SizeConstraint / inline IPSet; invoke DefaultAction gate; Evaluate helper |
| [Config](config.md) | Shipped | Recorder / delivery channel lite; Start writes lab snapshot JSON to delivery S3 then recording flag; continuous history while recording (S3 bucket create/delete + object Put/Delete) + GetResourceConfigHistory; SNS notify when snsTopicARN set; compliance stub |
| [ACM](acm.md) | Shipped | Request/Describe/List/DeleteCertificate, lab self-signed PEM |
| [Route 53](route53.md) | Shipped | Hosted zones, A/CNAME ChangeResourceRecordSets, AliasTarget to CloudFront/ELB; lab query log inject (opt-in) |
| [Cloud Map](servicediscovery.md) | Shipped | Private DNS (requires Vpc) / HTTP namespace, service/instance, Vpc-scoped DiscoverInstances |
| [Pricing](pricing.md) | Shipped | DescribeServices/GetAttributeValues/GetProducts over static catalog |
| [AppSync](appsync.md) | Shipped | GraphQL API CRUD, schema, Lambda data source (optional PassRole), nested selections (depth ≤ 3), API_KEY/IAM/Cognito auth |
| [API Gateway REST API](apigateway.md) | Shipped | REST v1 CreateRestApi/resource/method, PutIntegration AWS_PROXY+MOCK, CreateDeployment/Stage, execute `/restapis/{id}/{stage}/_user_request_/...`, NONE/AWS_IAM |
| [API Gateway HTTP API](apigatewayv2.md) | Shipped | HTTP API Lambda proxy, GetIntegrations/GetRoutes/GetAuthorizers, REST `/v2/apis` before ECR Registry `/v2/`, CorsConfiguration + OPTIONS preflight, NONE/JWT/IAM/CUSTOM authorizers, optional CredentialsArn PassRole, opt-in HTTP_PROXY/VPC_LINK allowlist |
| [API Gateway WebSocket](apigateway.md#websocket-implemented) | Shipped (lab lite) | CreateApi WEBSOCKET, `$connect`/`$disconnect`/`$default` AWS_PROXY, in-memory PostToConnection, HTTP lab stand-in on `/ws-api/...` |
| [Cognito User Pools](cognito-idp.md) | Shipped | Pool/client CRUD (UpdateUserPool), USER_PASSWORD_AUTH / USER_SRP_AUTH / CUSTOM_AUTH (optional SRP nesting) / refresh / RevokeToken, ForgotPassword / ConfirmForgotPassword / ResendConfirmationCode / UpdateUserAttributes / attribute verify, TOTP SOFTWARE_TOKEN_MFA, lab RoleArn + LambdaConfig PassRole and sync trigger Invoke (PostConfirmation / PreTokenGeneration / PreSignUp / Pre+PostAuthentication / CustomMessage SignUp/AdminCreateUser/ForgotPassword/ResendCode/UpdateUserAttribute/VerifyUserAttribute / UserMigration password / custom-auth Define/Create/Verify), RS256 tokens, JWKS on loopback |
| [CloudFront](cloudfront.md) | Shipped | Distribution CRUD; Deployed + lab DomainName; CacheBehavior PathPattern → TargetOriginId lite; optional Logging to in-account S3 on edge GET; SigV4 edge GET `/cloudfront/{id}/...` to S3 or HTTP API origin (no real PoP) |
| [ELB v2](elbv2.md) | Shipped | ALB + NLB lite: target group / listener; ALB path + host-header rules and `/alb/...` Lambda lab listener; NLB TCP/TLS control-plane + health; optional access_logs.s3.* to in-account S3 |
| [S3 Vectors](s3vectors.md) | Shipped | Vector bucket/index CRUD, PutVectors/QueryVectors cosine or euclidean |
| [Cloud Control](cloudcontrol.md) | Shipped | Create/Get/List/Update/DeleteResource + GetResourceRequestStatus; allowlist aligned with CFN lab types; UpdateResource mutable subsets (incl. IAM User/Group/ManagedPolicy, EventBus Policy, LogGroup RetentionInDays) |
| [BCM Data Exports](bcm-data-exports.md) | Shipped | Export definition CRUD plus sample file under data root |
| [Cost and Usage Reports](cur.md) | Shipped | Report definition CRUD; optional tiny CSV/JSON Put to S3 (no DuckDB) |
| [Cost Explorer](ce.md) | Shipped | GetCostAndUsage / GetCostForecast over seeded amounts |
| [Budgets](budgets.md) | Shipped | Budget CRUD, SNS notify on CreateBudget for SNS subscribers |
| [IoT Core / Data](iot.md) | Shipped | Things, lab CA-signed certs, policies, principals; HTTP shadows; opt-in MQTT shadow bridge (`NOCTAXRIS_SHARED_MQTT`) |
| [Lightsail](lightsail.md) | Shipped | Instance state machine; GetBlueprints/GetBundles; create/get/start/stop/reboot/delete |
| [Auto Scaling](autoscaling.md) | Shipped | Launch config + ASG CRUD; DesiredCapacity reconciles lab EC2 (Pending without engine; InService when running) |
| [Elastic Beanstalk](elastic-beanstalk.md) | Shipped | Application/version/environment lite; Ready/Green; ListAvailableSolutionStacks |
| [AWS Backup](backup.md) | Shipped | Vault/plan CRUD; StartBackupJob recovery-point metadata for S3/DDB ARNs |
| [CodeDeploy](codedeploy.md) | Shipped | Application / deployment group / deployment lite, optional ECS DesiredCount or Lambda PublishVersion hooks |
| [RDS](rds.md) | Shipped | Postgres / MySQL / MariaDB Create/Describe/Delete, nested DinD when engine up, nested-network endpoint |
| [RDS Data API](rds-data.md) | Shipped | Postgres-only ExecuteStatement / BatchExecuteStatement; Begin/Commit/Rollback via held `pgx`; prefer `pgx` else nested `psql`; typed OID fields on pgx; named `parameters`; `formatRecordsAs=JSON`; Batch `generatedFields` from `RETURNING`; secretArn fail-closed; MySQL/MariaDB resourceArn → BadRequest |
| [ElastiCache](elasticache.md) | Shipped | Redis/Valkey cache cluster CRUD, nested DinD when engine up |
| [MemoryDB](memorydb.md) | Shipped | Redis/Valkey cluster CRUD (JSON 1.1), nested DinD when engine up; Users/ACLs empty stubs |
| [DocumentDB](docdb.md) | Shipped | docdb Create/Describe/Delete; `creating` until nested Mongo-compatible starts |
| [Neptune](neptune.md) | Shipped | neptune Create/Describe/Delete; nested Gremlin Server `:8182` when DinD up; nested-network endpoint only |
| [Athena](athena.md) | Shipped | Start/Get/Stop/GetQueryResults over Glue + lab S3 CSV/JSON; WHERE equality/`!=`/`<>`/`IN`/LIKE/json_extract; COUNT(*), INNER JOIN, GROUP BY, ORDER BY; CloudTrail Records[] + gzip; GetObject/OutputLocation fail closed |
| [OpenSearch](opensearch.md) | Shipped | Domain CRUD; nested OpenSearch when DinD up (`Active`); else `CreateFailed` + `stub://`; SigV4 lab query facade `_doc` / allowlisted `_search` |
| [MSK](msk.md) | Shipped | Create/Describe/List/DeleteCluster + GetBootstrapBrokers; nested Redpanda when DinD up (`ACTIVE`); per-cluster or opt-in shared `noctaxris-lab-kafka:9092`; else `FAILED`; nested brokers only |
| [EMR](emr.md) | Shipped | RunJobFlow / Describe / List / Terminate **control-plane stub** (no Spark/Hadoop) |
| [Bedrock Runtime](bedrock-runtime.md) | Shipped | InvokeModel allowlist **canned** JSON stub |
| [Textract](textract.md) | Shipped | DetectDocumentText / AnalyzeDocument **canned** Blocks |
| [Transcribe](transcribe.md) | Shipped | Start/Get/List jobs; requires existing lab `s3://` object; **canned** transcript under data root |

## Shared verification

Unit tests (in-process; no Compose):

```bash
go test ./... -count=1
```

Compose-backed SDK / Terraform / CloudFormation suites (API must be ready on `127.0.0.1:4566`):

```bash
bash tests/run-all.sh
```

See [tests/README.md](../../tests/README.md). Go/Node/Python SDK suites include CloudFront edge, Transfer files, Glue crawlers, AppConfig deploy, Config history, SFN Choice, CloudTrail delivery, Route53 Alias, ELBv2 rules, AppSync PassRole, and EKS cluster CRUD rows when the API is up. Nested OpenSearch / ActiveMQ / Firehose OpenSearch / Lambda MQ ESM / Neptune / MSK rows skip when engines are not Active/RUNNING/available.

Condition-key catalogs and ADR-0005 §7 evaluation:

```bash
go test ./internal/catalog/conditionkeys ./internal/kernel/authz -count=1
```

Compose up (publishes `127.0.0.1:4566` only, must not mount host `docker.sock`). Compose binds `0.0.0.0` in-container; replace both `NOCTAXRIS_ROOT_*` values with unique lab credentials before `up` (the shipped `.env.example` pair is refused):

```bash
cp docker/.env.example docker/.env   # then set unique NOCTAXRIS_ROOT_* values
docker compose -f docker/compose.yaml --env-file docker/.env up --build -d
curl http://127.0.0.1:4566/_noctaxris/health
curl http://127.0.0.1:4566/_noctaxris/ready
```

Expect liveness body `ok` and readiness body `ready`. Lambda Invoke, ECS RunTask/CreateService scale-up, CodeBuild StartBuild, Batch SubmitJob, and nested RDS/ElastiCache/MemoryDB/DocumentDB engines need the nested `noctaxris-engine` service from the same compose file. Skip live nested-compute or nested-data smoke when Docker is unavailable (unit tests still cover PassRole, control-plane CRUD, and compute-unavailable).

Export root keys from `docker/.env`, then set a common CLI environment before any per-service smoke:

```bash
export AWS_ACCESS_KEY_ID=...
export AWS_SECRET_ACCESS_KEY=...
export AWS_DEFAULT_REGION=us-east-1
EP=http://127.0.0.1:4566
```

Prefer WSL or Linux for AWS CLI smoke against `http://127.0.0.1:4566`. On Windows, run the same commands inside WSL when Docker Desktop publishes that port on the Windows host.

**CI vs nested smoke:** Push/PR CI runs `smoke-core` (ready + STS/S3/KMS/DynamoDB). Nested DinD runs on a weekly schedule and via Actions `workflow_dispatch` with `nested_smoke=true`, or [docker/smoke-nested.sh](../../docker/smoke-nested.sh). A green PR does not prove Lambda Invoke, nested RDS/Data API, or other DinD paths. Full matrix: [ops.md](../ops.md).

Per-service CLI smoke lives on each shipped service page above.

## Cross-cutting

**Cross-account resource policies:** For S3, SQS, Lambda, ECR, SNS, Secrets Manager, DynamoDB, and EventBridge buses, same-account access stays identity **or** resource policy Allow. Cross-account access requires identity **and** resource policy Allow (empty resource policy denies). KMS stays key-policy-required, with cross-account callers needing both identity and key policy Allow. See each service Authz notes.

**Organizations SCP/RCP:** Member authorize loads policies attached to the account, each OU on the path to root, and the organization root. Management account is exempt. See [organizations.md](organizations.md).

**Condition keys:** Catalogs for lab-core services (IAM, STS, Organizations, KMS, S3, DynamoDB, SQS, Lambda, SSM, Secrets Manager, SNS, EventBridge, ECR, ECS) plus a global seed ship via `internal/catalog/conditionkeys` (servicereference snapshots and ADR-0005 §7 eval rules). Request context populates `aws:SourceIp`, `aws:PrincipalArn`, `aws:PrincipalAccount`, `aws:RequestedRegion`, `aws:username` / `aws:userid` / `aws:PrincipalType`, `aws:SecureTransport` (from TLS listen config), `aws:CurrentTime` / `aws:EpochTime`, MFA keys, and `aws:ResourceTag/*` (plus `ecr` / `ssm` / `secretsmanager` / `iam` / `ecs` ResourceTag prefixes) when tags exist via the Tagging API. Unrecognized Condition operators fail closed (same Deny class as catalog-unknown keys). Implemented operators: StringEquals/Like/NotEquals/NotLike (+IfExists), ArnEquals/ArnLike/ArnNotEquals/ArnNotLike (+IfExists), Null, Bool (+IfExists), IpAddress/NotIpAddress (+IfExists), Numeric* and Date* comparisons, and `ForAnyValue:` / `ForAllValues:` on String/Arn operators. Out of lab scope: Binary*, StringEqualsIgnoreCase, full multivalued request-context sets beyond single-valued lab keys.

**Compute runtime:** Nested DinD via Compose `noctaxris-engine` is the only packaged path for Lambda, ECS, EC2 lab instances, CodeBuild, Batch, and nested data engines (RDS / ElastiCache / MemoryDB / DocumentDB / Neptune / MQ / OpenSearch / MSK). Live Invoke/RunTask/RunInstances need a healthy engine. Default engine is restricted DinD (`privileged: false` with explicit caps and host cgroup); use `docker/compose.engine-privileged.yaml` only when nested smoke fails on the host. Athena runs in-process (no nested query engine).

**Nested data ports:** Compose publishes only `127.0.0.1:4566`. Nested `DataKind` engines are RDS (Postgres/MySQL/MariaDB when enabled), ElastiCache/MemoryDB (Valkey/Redis), DocumentDB (Mongo-compatible), Neptune (Gremlin Server), MQ (RabbitMQ or ActiveMQ), OpenSearch, and MSK (Redpanda); those ports are never published on the host. Prefer RDS Data API on `:4566` for SQL labs.

**In-process workers:** EventBridge Scheduler uses an in-process ticker. Lambda SQS, DynamoDB Streams, Kinesis (all shards polled sequentially), and Amazon MQ event source mappings use a continuous in-process poller. EventBridge Pipes use a continuous in-process ticker that calls `PollPipeOnce`. SNS HTTP delivery defaults to the loopback catcher; non-catcher URLs need `NOCTAXRIS_SNS_HTTP_EGRESS=1` plus allowlist (no open SSRF).

**Edge identity:** Cognito JWKS and API Gateway / AppSync Cognito JWT verify share the go-jose v4 helper. Gateway invoke stays on the single published listener. No second host port for Cognito Hosted UI.
