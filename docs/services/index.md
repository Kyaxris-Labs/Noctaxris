# Services

Per-service reference for the Noctaxris lab emulator. Status matches the root [README](../../README.md) Services table.

Each page covers what is implemented, how to verify with AWS CLI smoke, and what remains deferred.

| Service | Status | Doc |
|---------|--------|-----|
| [IAM](iam.md) | Shipped | Users, roles, policies, keys, groups, boundaries, IdPs, MFA |
| [STS](sts.md) | Shipped | All 11 actions (lab MFA on GetSessionToken) |
| [Organizations](organizations.md) | Shipped | Accounts, OUs, MoveAccount, SCP/RCP attach with OU-path inheritance |
| [KMS](kms.md) | Shipped | CMKs, key-policy-required crypto, cross-account dual eval, grants, lab aliases, deletion sweeper, key-material rotation |
| [S3](s3.md) | Shipped | Path-style objects, multipart, CopyObject, bucket encryption, versioning lite, cross-account dual eval |
| [DynamoDB](dynamodb.md) | Shipped | Tables, items, up to two lab GSIs, TTL, resource policies, cross-account dual eval, stream enablement |
| [DynamoDB Streams](dynamodbstreams.md) | Shipped | Enable stream, List/Describe, GetShardIterator/GetRecords, NEW_IMAGE or KEYS_ONLY |
| [SQS](sqs.md) | Shipped | Standard and FIFO queues, DelaySeconds, RedrivePolicy and RedriveAllowPolicy, policies, cross-account dual eval |
| [Lambda](lambda.md) | Shipped | Zip/Image, versions/aliases, layers on zip and Image, SQS and DynamoDB Streams ESM, Function URLs, sync+async Invoke, lab ECR Image pull, TLS DinD default, opt-in microVM |
| [SSM Parameter Store](ssm.md) | Shipped | String and SecureString, GetParametersByPath hierarchy, KMS via alias/aws/ssm, identity authz |
| [Secrets Manager](secretsmanager.md) | Shipped | CRUD, list, RotateSecret, recovery window, resource policies, cross-account dual eval, KMS via alias/aws/secretsmanager |
| [SNS](sns.md) | Shipped | Topic CRUD including FIFO, publish, SQS/Lambda/HTTP loopback subscribe, topic policies, cross-account dual eval |
| [EventBridge](eventbridge.md) | Shipped | Buses, rules, targets, PutEvents routing to SQS, Lambda, and SNS (RoleArn delivery sessions; lab InputPath) |
| [EventBridge Scheduler](scheduler.md) | Shipped | Schedule CRUD, rate/cron/at subset, Lambda/SQS/SNS targets, in-process ticker, PassRole |
| [EventBridge Pipes](pipes.md) | Shipped | Pipe CRUD; SQS or DynamoDB Streams source to Lambda or SQS; continuous in-process ticker + `PollPipeOnce`; RoleArn or target policy on deliver |
| [Amazon MQ](mq.md) | Shipped | Broker CRUD **control-plane stub** (ActiveMQ/RabbitMQ); loopback `stub://` endpoint; not a live broker |
| [Transfer Family](transfer.md) | Shipped | Server/user CRUD; SFTP-shaped sandbox under data root; Describe reports `OFFLINE` (no listener) |
| [ECR](ecr.md) | Shipped | Repository CRUD, auth token, policies, cross-account dual eval, Registry V2, DinD sync |
| [ECS](ecs.md) | Shipped | Task definitions, RunTask/list/stop, CreateService DesiredCount reconciler, PassRole, nested DinD default, opt-in microVM |
| [CloudTrail](cloudtrail.md) | Shipped | LookupEvents over local JSONL audit |
| [CloudWatch Logs](logs.md) | Shipped | Log groups/streams CRUD lite, Put/GetLogEvents, DescribeLogGroups/DescribeLogStreams |
| [Resource Groups Tagging API](resourcegroupstaggingapi.md) | Shipped | TagResources, UntagResources, GetResources |
| [Kinesis Data Streams](kinesis.md) | Shipped | Stream CRUD, Put/Get records, single-shard iterators |
| [Firehose](firehose.md) | Shipped | Delivery stream CRUD, PutRecord(s) to S3 or Lambda; RoleARN session or destination policy on Put |
| [SES](ses.md) | Shipped | Local catcher: verify, SendEmail/SendRawEmail, ListIdentities, SetIdentityNotificationTopic Bounce, GetSendStatistics |
| [AppConfig](appconfig.md) | Shipped | Application/Environment/Profile, hosted versions, GetConfiguration, AppConfigData session |
| [Step Functions](stepfunctions.md) | Shipped | State machine CRUD, StartExecution, Pass/Succeed/Fail/Task to Lambda |
| [CloudFormation](cloudformation.md) | Shipped | Stack CRUD lite for S3 bucket and IAM role resources |
| [CodeBuild](codebuild.md) | Shipped | Project CRUD lite, StartBuild on nested DinD, BatchGetBuilds/ListBuilds |
| [CodePipeline](codepipeline.md) | Shipped | Pipeline CRUD, StartPipelineExecution with nested CodeBuild StartBuild, GetPipelineState |
| [Batch](batch.md) | Shipped | Compute environment / queue / definition lite, SubmitJob on nested DinD |
| [Glue](glue.md) | Shipped | Data Catalog database and table CRUD (PartitionKeys + SerDe fields for Athena) |
| [WAF v2](wafv2.md) | Shipped | WebACL / rule group lite; AssociateWebACL; invoke DefaultAction gate; Evaluate helper |
| [Config](config.md) | Shipped | Recorder / delivery channel lite, StartRecorder SNS notify when snsTopicARN set, compliance stub over tagged resources |
| [ACM](acm.md) | Shipped | Request/Describe/List/DeleteCertificate, lab self-signed PEM |
| [Route 53](route53.md) | Shipped | Hosted zones, A/CNAME ChangeResourceRecordSets |
| [Cloud Map](servicediscovery.md) | Shipped | Namespace/service/instance register and DiscoverInstances |
| [Pricing](pricing.md) | Shipped | DescribeServices/GetAttributeValues/GetProducts over static catalog |
| [AppSync](appsync.md) | Shipped | GraphQL API CRUD, schema, Lambda data source, API_KEY, IAM, or Cognito User Pools auth |
| [API Gateway HTTP API](apigatewayv2.md) | Shipped | HTTP API Lambda proxy, NONE/JWT/IAM authorizers, optional CredentialsArn PassRole |
| [Cognito User Pools](cognito-idp.md) | Shipped | Pool/client CRUD, USER_PASSWORD_AUTH, RS256 tokens, JWKS on loopback |
| [CloudFront](cloudfront.md) | Shipped | Distribution CRUD **control-plane stub** (S3 or Gateway origin strings; no PoP) |
| [ELB v2](elbv2.md) | Shipped | Load balancer / target group / listener lite (lambda or ip targets; no EC2) |
| [S3 Vectors](s3vectors.md) | Shipped | Vector bucket/index CRUD, PutVectors/QueryVectors cosine or euclidean |
| [Cloud Control](cloudcontrol.md) | Shipped | Create/Get/List/DeleteResource for S3 bucket and IAM role |
| [BCM Data Exports](bcm-data-exports.md) | Shipped | Export definition CRUD plus sample file under data root |
| [Cost Explorer](ce.md) | Shipped | GetCostAndUsage / GetCostForecast over seeded amounts |
| [Budgets](budgets.md) | Shipped | Budget CRUD, SNS notify on CreateBudget for SNS subscribers |
| [CodeDeploy](codedeploy.md) | Shipped | Application / deployment group / deployment lite, optional ECS DesiredCount or Lambda PublishVersion hooks |
| [RDS](rds.md) | Shipped | Postgres Create/Describe/Delete, nested DinD when engine up, nested-network endpoint |
| [RDS Data API](rds-data.md) | Shipped | ExecuteStatement + txn lite; **stub executor** (explicit stub marker; no pgx); secretArn fail-closed |
| [ElastiCache](elasticache.md) | Shipped | Redis/Valkey cache cluster CRUD, nested DinD when engine up |
| [DocumentDB](docdb.md) | Shipped | docdb Create/Describe/Delete cluster, nested Mongo-compatible when engine up |
| [Athena](athena.md) | Shipped | Start/Get/Stop/GetQueryResults over Glue + lab S3 CSV/JSON subset; missing bucket fails closed |
| [OpenSearch](opensearch.md) | Shipped | Domain CRUD **control-plane stub** endpoint (`stub://`; no nested search) |
| [EMR](emr.md) | Shipped | RunJobFlow / Describe / List / Terminate **control-plane stub** (no Spark/Hadoop) |
| [Bedrock Runtime](bedrock-runtime.md) | Shipped | InvokeModel allowlist **canned** JSON stub |
| [Textract](textract.md) | Shipped | DetectDocumentText / AnalyzeDocument **canned** Blocks |
| [Transcribe](transcribe.md) | Shipped | Start/Get/List jobs; requires existing lab `s3://` object; **canned** transcript under data root |

## Shared verification

Unit and integration tests:

```bash
go test ./... -count=1
```

Condition-key catalogs and ADR-0005 §7 evaluation:

```bash
go test ./internal/catalog/conditionkeys ./internal/kernel/authz -count=1
```

Compose up (publishes `127.0.0.1:4566` only, must not mount host `docker.sock`):

```bash
cp docker/.env.example docker/.env   # set root keys if needed
docker compose -f docker/compose.yaml --env-file docker/.env up --build -d
curl http://127.0.0.1:4566/_noctaxris/health
curl http://127.0.0.1:4566/_noctaxris/ready
```

Expect liveness body `ok` and readiness body `ready`. Lambda Invoke, ECS RunTask/CreateService scale-up, CodeBuild StartBuild, Batch SubmitJob, and nested RDS/ElastiCache/DocumentDB engines need the nested `noctaxris-engine` service from the same compose file. Skip live nested-compute or nested-data smoke when Docker is unavailable (unit tests still cover PassRole, control-plane CRUD, and compute-unavailable).

Export root keys from `docker/.env`, then set a common CLI environment before any per-service smoke:

```bash
export AWS_ACCESS_KEY_ID=...
export AWS_SECRET_ACCESS_KEY=...
export AWS_DEFAULT_REGION=us-east-1
EP=http://127.0.0.1:4566
```

Prefer WSL or Linux for AWS CLI smoke against `http://127.0.0.1:4566`. On Windows, run the same commands inside WSL when Docker Desktop publishes that port on the Windows host.

Opt-in microVM (`NOCTAXRIS_COMPUTE_RUNTIME=microvm`) needs a Linux host with usable `/dev/kvm` and a Firecracker binary. On WSL2 or without KVM the path fails closed. Live guest zip/Image Invoke and ECS RunTask still await kernel/rootfs assets on that host. Leave the runtime unset (or `dind`) for normal labs.

Per-service CLI smoke lives on each shipped service page above.

## Cross-cutting

**Cross-account resource policies:** For S3, SQS, Lambda, ECR, SNS, Secrets Manager, and DynamoDB, same-account access stays identity **or** resource policy Allow. Cross-account access requires identity **and** resource policy Allow (empty resource policy denies). KMS stays key-policy-required, with cross-account callers needing both identity and key policy Allow. See each service Authz notes. EventBridge bus policy dual-eval depth remains deferred.

**Organizations SCP/RCP:** Member authorize loads policies attached to the account, each OU on the path to root, and the organization root. Management account is exempt. See [organizations.md](organizations.md).

**Condition keys:** Catalogs for lab-core services (IAM, STS, Organizations, KMS, S3, DynamoDB, SQS, Lambda, SSM, Secrets Manager, SNS, EventBridge, ECR, ECS) plus a global seed ship via `internal/catalog/conditionkeys` (servicereference snapshots and ADR-0005 §7 eval rules). Request context now populates `aws:SourceIp`, `aws:PrincipalArn`, `aws:PrincipalAccount`, `aws:RequestedRegion`, MFA keys, and `aws:ResourceTag/*` (plus matching service ResourceTag keys) when tags exist via the Tagging API. Broader operator matrix and every global key population remain open (partial today: StringEquals/Like/NotEquals, Null, IfExists variants).

**Compute runtime:** Nested DinD is the default for Lambda, ECS, CodeBuild, Batch, and nested data engines (RDS / ElastiCache / DocumentDB). Opt-in microVM selection for Lambda and ECS RunTask is documented on [lambda.md](lambda.md) and [ecs.md](ecs.md). WSL2 is DinD-only. Live Firecracker guest boot remains deferred. Athena runs in-process (no nested query engine).

**Nested data ports:** Compose publishes only `127.0.0.1:4566`. Postgres, Redis/Valkey, Mongo, and OpenSearch ports are not published on the host. Prefer RDS Data API on `:4566` for SQL labs.

**In-process workers:** EventBridge Scheduler uses an in-process ticker. Lambda SQS (and DynamoDB Streams when enabled) event source mappings use a continuous in-process poller. EventBridge Pipes use a continuous in-process ticker that calls `PollPipeOnce`. SNS HTTP delivery is allowlisted loopback only (no open SSRF).

**Edge identity:** Cognito JWKS and API Gateway / AppSync Cognito JWT verify share the go-jose v4 helper. Gateway invoke stays on the single published listener. No second host port for Cognito Hosted UI.
