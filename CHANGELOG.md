# Changelog

## Unreleased

- API Gateway REST: TOKEN/REQUEST Lambda authorizers (`Create`/`Get`/`GetAuthorizers`/`DeleteAuthorizer`; method `CUSTOM`/`TOKEN`/`REQUEST` + `AuthorizerId`); usage plans and API keys lite (`Create`/`Get`/`Delete` ApiKey and UsagePlan, UsagePlanKey associate); invoke fail-closed Allow/Deny and `x-api-key` when `apiKeyRequired` or stage is under a usage plan
- KMS: asymmetric lab keys — `CreateKey` `KeySpec`/`CustomerMasterKeySpec` `RSA_2048` (`KeyUsage` `SIGN_VERIFY`); sealed PKCS8 material; `Sign`/`Verify` (`RSASSA_PSS_SHA_256` primary, also `RSASSA_PKCS1_V1_5_SHA_256`); `GetPublicKey` PEM SPKI; symmetric keys reject Sign/Verify fail-closed
- Step Functions: `waitForTaskToken` Task pause (Resource `.waitForTaskToken` or Parameters with `WaitForTaskToken` / `$$.Task.Token`); `SendTaskSuccess` / `SendTaskFailure` / `SendTaskHeartbeat`; execution stays `RUNNING` until callback
- SSM: parameter version labels (`LabelParameterVersion`, `GetParameterHistory`); `GetParameter`/`GetParameters` resolve `Name:version` / `Name:label` or `Version`/`Label`; Put overwrite keeps history; max 10 labels/version with AWS-like move on reattach
- S3: bucket CORS (`Put`/`Get`/`DeleteBucketCors`), lifecycle configuration (`Put`/`Get`/`DeleteLifecycleConfiguration`, stored rules only), and `SelectObjectContent` lite (CSV/JSON, `SELECT * FROM s3object [LIMIT n]`, simplified JSON records; unsupported SQL fail closed)
- CodePipeline: Manual Approval pause on `StartPipelineExecution` (`InProgress` + token via `GetPipelineState`); `PutApprovalResult` Approved continues to CodeBuild / Rejected fails; `GetPipelineExecution` and `ListPipelineExecutions` store-state
- AppSync: management SAR (`List`/`Delete` ApiKeys; `Update`/`Delete`/`Get`/`List` DataSources and Resolvers; `GetSchemaCreationStatus` returns `SUCCESS` after `StartSchemaCreation`); GraphQL Lambda runtime unchanged
- Lightsail: stored-state disks (`Create/Get/GetDisks/Attach/Detach/DeleteDisk`), static IPs (`Allocate/Get/GetStaticIps/Attach/Detach/ReleaseStaticIp`), key pairs (`Create/Get/GetKeyPairs/DeleteKeyPair` dummy material), and public ports (`Open/CloseInstancePublicPorts`, `GetInstancePortStates`); no VMs
- EMR: CancelSteps (marks steps CANCELLED); persist InstanceGroups/Fleets from RunJobFlow; ListInstanceGroups/ListInstanceFleets; cluster AddTags/RemoveTags; Create/Describe/Delete/List SecurityConfiguration (stored JSON); still no Spark
- CUR: FOCUS enumerator + projection lite — in-process S3/Lambda usage counters; FOCUS CSV when `Format=FOCUS` or `AdditionalSchemaElements` includes `FOCUS`; `Format=Parquet` stages FOCUS NDJSON then DuckDB Parquet (same `NOCTAXRIS_CUR_EMIT` gate)
- Auto Scaling: scaling policies (`Put`/`Describe`/`Delete`), lifecycle hooks, `Attach`/`Detach`/`Describe` instances (membership without DinD), and ELBv2 target-group attach/detach/describe (ARN validated against the ELBv2 store)
- AWS Backup: selections (`Create`/`Get`/`List`/`DeleteBackupSelection` with `IamRoleArn` + `Resources`); `ListBackupJobs` filters; `StopBackupJob` no-op on completed; `DeleteRecoveryPoint` metadata removal
- ELBv2: `ModifyListener` (default forward / port / protocol) and `ModifyRule` (conditions + forward action); ALB rules accept `http-header`, `query-string`, and `source-ip` alongside path/host; lab `/alb/` matching uses headers, query, and peer source IP (still `:4566` only)
- Glue: Schema Registry lite (`Create/Get/List/Delete` Registry and Schema, `Register/Get/List` SchemaVersion); GetTable/GetTables resolve empty Columns from SchemaReference (AVRO/JSON)
- CloudFront: `GetDistributionConfig` (config + ETag), `UpdateDistribution` (IfMatch; mutates Enabled/origins/cache behaviors), and invalidation theatre (`CreateInvalidation`/`GetInvalidation`/`ListInvalidations`, Completed immediately)

- IoT Topic Rules engine lite: `CreateTopicRule` / `GetTopicRule` / `ListTopicRules` / `ReplaceTopicRule` / `DeleteTopicRule` / `EnableTopicRule` / `DisableTopicRule`; minimal `SELECT * FROM 'topic/filter'` (+/#); in-process actions (republish, SQS, SNS, S3, DynamoDB, Kinesis, Lambda) fail-closed on missing targets; MQTT non-`$aws/` publish dispatch when shared MQTT is on; tests use `PublishTopic` without Mosquitto
- Athena: WorkGroup CRUD (`Create`/`Get`/`List`/`Update`/`Delete`); `primary` seeded; `StartQueryExecution` honors DISABLED fail-closed and `EnforceWorkGroupConfiguration` / default `OutputLocation`
- WAFv2: IPSet CRUD (`CreateIPSet` / `GetIPSet` / `UpdateIPSet` / `DeleteIPSet` / `ListIPSets`) and `IPSetReferenceStatement.ARN` resolve for SourceIP matching (unknown ARN fail closed; inline Addresses kept)

## 1.3.1

Patch after 1.3.0: SQLite busy-retry for multi-connection store writes, and release publish gated on required CI. Docker Hub: `kyaxris/noctaxris` (`1.3.1`, `1.3`, `1`, `latest`). Cut steps: [docs/release.md](docs/release.md).

- SQLite: retry multi-connection `SQLITE_BUSY` deadlock victims on PutObject meta writes and SQS begin (`busy_timeout` does not wait on those)
- Release: `v*` tag publish runs `ci-required.yml` (unit, compose-static, govulncheck, race, image, smoke-core) and pushes to Docker Hub only when those gates succeed

## 1.3.0

Minor after 1.2.0: Floci-parity nested compute and data labs (EC2/ASG/EKS, RDS MySQL, Athena/CUR DuckDB, Neptune Neo4j), observe and edge depth (SES v2, SSM Run Command, NLB shim, Bedrock Converse), opt-in shared Kafka/MQTT brokers, and request-path speedups that keep durable fail-closed authz. Docker Hub: `kyaxris/noctaxris` (`1.3.0`, `1.3`, `1`, `latest`). Cut steps: [docs/release.md](docs/release.md).

### Nested compute and control-plane labs

- EC2: Query Run/Describe/Stop/Start/Terminate on DinD `noctaxris-ec2`; AMI→allowlisted image map; pending without engine; UserData once on create; IMDS lite sidecar (no host `:9169`); VPC/subnet/SG/ENI metadata CRUD (rules not enforced on DinD); VPC Flow unchanged under `ec2`
- Auto Scaling: DesiredCapacity reconciles to lab EC2 (`Pending` without engine; `InService` when running; ForceDelete terminates)
- EKS lite: Create/Describe/List/DeleteCluster REST; metadata-only ACTIVE (no nested k3s under restricted DinD)
- Lightsail, Elastic Beanstalk, AWS Backup: stored-state control-plane labs

### Nested data and analytics

- RDS: MySQL/MariaDB engines alongside Postgres; opt-in `compose.lab-nested-ports.yaml` + `NOCTAXRIS_NESTED_PORT_PUBLISH`
- RDS Data API: Execute/Batch/txn for nested `mysql` and `mariadb` (`go-sql-driver/mysql` or nested `mysql` CLI); Postgres path unchanged
- Neptune: default Gremlin Server; opt-in nested Neo4j Bolt (`NOCTAXRIS_NEPTUNE_ENGINE` / GraphEngine / tag)
- MemoryDB nested lab; MSK nested Redpanda (per-cluster default)
- Athena: optional nested DuckDB sidecar (`NOCTAXRIS_ATHENA_ENGINE` / `NOCTAXRIS_DUCKDB_URL`); in-process `BETWEEN` and range WHERE; Parquet Glue tables require DuckDB
- CUR: `Format=Parquet` emits real Parquet via nested DuckDB (NDJSON stage + `COPY ... FORMAT PARQUET`); CSV unchanged; fail-closed without DuckDB (no JSON stand-in)
- DynamoDB LSI + PartiQL lite; Kinesis EFO + UpdateShardCount

### Observe, messaging, and edge

- SES v2 REST on `/v2/email/*`: Create/List/Get/DeleteEmailIdentity, outbound-emails (Simple/Raw), GetAccount; shared identities/messages with v1; ECR registry excludes `/v2/email/*`
- SSM Run Command lite: SendCommand / GetCommandInvocation / ListCommandInvocations for AWS-RunShellScript via nested DinD exec
- Transfer: DescribeUser/ListUsers plus Import/Delete SSH public keys; lab Put/GetFile stays HTTP on `:4566` (not an SFTP listener)
- EMR: AddJobFlowSteps / DescribeStep / ListSteps (immediate COMPLETED lab stub)
- ELBv2: lab HTTP shim `/nlb/{account}/{name}/{port}/...` for network LBs (open-dataplane gate; not true L4)
- Bedrock Runtime: Converse canned JSON over allowlisted modelIds; ConverseStream 501
- CloudWatch Metrics/Alarms; CUR definitions; IoT Core/Data HTTP shadows
- Config: continuous history while recording covers S3 object PutObject/DeleteObject (`AWS::S3::Object` resourceId `bucket/key`)
- Lambda async SQS DLQ / OnFailure: honor destination `RedriveAllowPolicy` and foreign queue Policy via `sendLabDLQMessage`
- API edge: API Gateway REST v1 + WebSocket lite; HTTP_PROXY allowlist opt-in; SNS HTTPS egress gate

### Shared brokers

- Opt-in `NOCTAXRIS_SHARED_KAFKA=1` binds one MSK cluster process-wide to DinD Redpanda `noctaxris-lab-kafka:9092` (`compose.lab-brokers.yaml`)
- Opt-in `NOCTAXRIS_SHARED_MQTT=1` starts Mosquitto `noctaxris-lab-mqtt` plus API MQTT shadow bridge; default remains HTTP shadows only
- `NOCTAXRIS_BROKER_PORT_PUBLISH` narrow gate for engine PortBindings (`:9092` / `:1883`); no default host publish of broker ports

### Request path performance

- Access-key material cache after Unseal (always on); invalidate on Delete/UpdateAccessKey (including `*InAccount`); SigV4 still re-checks Status, ExpiresAt, and session token
- Verified plumbing carries SessionPolicy, MFA time, and federated caller fields so authorize paths avoid re-LookupAccessKeyRecord
- Fail-closed identity policy load errors; batched `IdentityPolicyDocsForUser`; shared OU path for SCP/RCP via `OrgFilterDocsForAccount`
- S3 notification emit after object lock release; SQS `SendMessage` prep outside `sqsMu` (re-GetQueue under lock)
- Does not cache Allow/Deny decisions or authz policy document sets

## 1.2.0

Minor after 1.1.2: forensic lab depth (CloudTrail inject and siblings), Critical/High security hardening, CodeBuild/CodeCommit nested builds, Lambda LTS runtimes plus Terraform microservice stacks, and nested RDS smoke fixes. Docker Hub: `kyaxris/noctaxris` (`1.2.0`, `1.2`, `1`, `latest`). Cut steps: [docs/release.md](docs/release.md).

### Forensic depth

- CloudTrail: lab `InjectEvents` / `InjectInsightsEvents` (opt-in `NOCTAXRIS_CLOUDTRAIL_INJECT`); richer live audit (`userName`, `sessionContext`, `eventCategory`/`managementEvent`, safer requestParameters, broader `resources[]`); service-correct error `eventSource`; sibling `kms:Decrypt` audit on Secrets/SSM/S3 SSE-KMS/DDB reads; AWSLogs hive S3 keys + optional `NOCTAXRIS_CLOUDTRAIL_GZIP`; Logs delivery timestamps from `eventTime`; `PutEventSelectors` / `GetEventSelectors` lite; digest sidecar + lab `ValidateLogs`; org trail flag (`IsOrganizationTrail`); LookupEvents lab attribute `SourceIPAddress`; Insights lite via `EventCategory=insight` (no ML)
- Athena: unwrap CloudTrail `Records[]`, gzip object read, `WHERE` `LIKE` and `json_extract` lite
- Lambda: Invoke ships `/aws/lambda/*` START/END/REPORT (+ stdout when available); richer Invoke CT; ESM poll Invoke audit
- Logs: JSON field-equality `filterPattern` subset for CT-shaped messages
- S3: delete markers + version-aware delete; Object Lock lite; server access logging; Get/Delete CT enrichment
- GuardDuty: Create/ListDetectors, List/GetFindings, lab `InjectFindings` (`NOCTAXRIS_GUARDDUTY_INJECT`)
- Security Hub: `BatchImportFindings` + `GetFindings` lite (ASFF-lite)
- Macie lite: EnableMacie/GetMacieSession; Create/Describe/ListClassificationJobs (sync COMPLETE); List/GetFindings; lab `InjectFindings` (`NOCTAXRIS_MACIE_INJECT`)
- ELBv2 / CloudFront: access logs to in-account S3 when enabled
- VPC Flow Logs lab seed: opaque IDs, CreateFlowLogs lite, inject (`NOCTAXRIS_VPCFLOW_INJECT`) → S3/Logs v2 ACCEPT/REJECT
- Config: continuous history while recording + `GetResourceConfigHistory` (S3 bucket create/delete hooks)
- IAM: AssumeRole CT enrichment; `GetAccessKeyLastUsed`; `GenerateCredentialReport` / `GetCredentialReport`
- Detective lite: CreateGraph/ListGraphs/AcceptInvitation; lab SearchGraph over CT + GuardDuty
- Lab forensics APIs (`NOCTAXRIS_LAB_FORENSICS`): FreezeClock/UnfreezeClock/SetClock/BulkSeed
- Messaging: SNS RedrivePolicy DLQ; EventBridge target DLQ + delivery history; Pipes DLQ
- Orgs: ListPolicies / ListPoliciesForTarget / ListParents / ListAccountsForParent; account-scoped Lookup
- Optionals: Route53 query log inject (config wiring fix); Firehose VPC Flow dest; SQS DLQ provenance; Control Tower honest stub

### Security hardening

- Organizations: management-account gate on mutate/list, CreateAccount, and AssumeRoot (closes shared org control-plane escalation)
- Compose / listen: refuse shipped example root key pair on non-loopback binds; smoke regenerates roots when `.env` still has the example pair; master key on sibling volume (`NOCTAXRIS_MASTER_KEY_FILE`, Compose `noctaxris-secrets`)
- Cognito: high-entropy single-use confirmation / forgot-password codes by default; `NOCTAXRIS_COGNITO_INSECURE_CODES=1` restores fixed `123456` / any-non-empty ConfirmSignUp for intentional insecure labs
- IAM: mutation authz uses request resource ARNs (not `Resource="*"` for most writes)
- CloudFormation / Cloud Control: provision-time PassRole and underlying-action authz
- Image pull: lab registry + pinned bases only; DinD `host.docker.internal` refs pinned to listen address (no silent `:4566`)
- OpenSearch: nested query host allowlist (SSRF fail-closed)
- Messaging DLQ: honor SQS Policy / RedriveAllowPolicy on lab redrive
- S3: reject Windows `\` key escape of data root; CloudTrail inject redaction / audit scrubbing for secrets
- Nested data plane: CapDrop ALL plus bootstrap CapAdd (`CHOWN` / `DAC_OVERRIDE` / `FOWNER` / `SETGID` / `SETUID`) so Postgres entrypoints start under DinD

### CodeBuild and CodeCommit

- CodeBuild: project CRUD (Create/Update/Delete/List/BatchGet); StartBuild / StartBuildBatch on nested DinD; webhooks Create/Delete/List + receive path; lab artifact/log handling; optional lab image under `docker/codebuild-lab/`
- CodeCommit: Create/Get/List/DeleteRepository; PutFile / GetFile lab store for nested build sources
- Nested ECS/CodeBuild IMDS sidecar for task/container credentials on Internal `noctaxris-ecs`

### Lambda runtimes and Terraform

- Zip CreateFunction allowlist: `python3.11`–`3.14`, `nodejs20.x` / `22.x` / `24.x`, `java21` / `java25` (SDK matrices + docs CLI smoke)
- Terraform: `lab-lambda-python` / `lab-lambda-nodejs` / `lab-lambda-java`; `lab-ms-serverless` (Lambda pipeline, provider `~> 5.0`); `lab-ms-ecs` (ECR + task/service, `live` DesiredCount); gates `TF_LAMBDA_RUNTIMES` / `TF_MS` / `TF_MS_LIVE`
- ECR/ECS: ListTags/Tag/Untag stubs; CreateService completed PRIMARY deployment; DescribeServices accepts service ARNs (Terraform waiters)
- CloudFormation sample YAML for Node 24 and Java 21 ZipFile

### Nested RDS Data API

- Smithy RPC-v2 paths (`POST /Execute`, `/BatchExecute`, transaction paths) accepted alongside `X-Amz-Target: AmazonRDSDataService.*` for current AWS CLI v2 / boto3
- Nested smoke logs API output when RDS never reaches `available`

## 1.1.2

Patch after 1.1.1: edge, search, analytics, governance, and workflow lab depth; Cognito and Lambda MQ gap closure; SDK/docs polish; CloudTrail continuous JSONL delivery. Docker Hub: `kyaxris/noctaxris` (`1.1.2`, `1.1`, `1`, `latest`). Cut steps: [docs/release.md](docs/release.md).

### Close remaining gaps

- Cognito: `ConfirmForgotPassword` (lab code `123456`); `UpdateUserAttributes` / `GetUserAttributeVerificationCode` / `VerifyUserAttribute` with `CustomMessage_UpdateUserAttribute` / `CustomMessage_VerifyUserAttribute`; UserMigration on `USER_SRP_AUTH` and `CustomMessage_Authentication` (SMS MFA) remain blocked
- Lambda MQ ESM: RabbitMQ AMQP 0-9-1 Dial + `basic.get` on allowlisted nested hosts (`github.com/rabbitmq/amqp091-go`); ActiveMQ stays dial-then-empty (AMQP 1.0)
- Terraform: skip from WSL when endpoint is loopback (`127.0.0.1`/`localhost`) unless `NOCTAXRIS_FORCE_WSL_TF=1`; do not widen Compose publish

### Polish and lab depth

- Cognito: lite `ForgotPassword` / `ResendConfirmationCode` with `CustomMessage_ForgotPassword` / `CustomMessage_ResendCode`; `CUSTOM_AUTH` SRP nesting (`SRP_A` → `PASSWORD_VERIFIER` → optional `CUSTOM_CHALLENGE`); UserMigration on `USER_SRP_AUTH` still deferred
- Lambda MQ ESM: RabbitMQ AMQP 0-9-1 allowlisted Dial + `basic.get` (superseded empty-batch wording for Rabbit; ActiveMQ still dial-empty); injectable `MQReceiveFunc` for tests
- Step Functions: ASL `Map` (sequential Iterator), non-Equals Choice (`StringGreaterThan` / `StringLessThan` / `NumericGreaterThan` / `NumericLessThan` / `IsPresent`), top-level `InputPath` / `ResultPath`
- CloudTrail: continuous JSONL delivery to in-account S3/Logs after `StartLogging` (cursor + fail-closed Put)
- WAFv2: `SizeConstraintStatement` and inline `IPSetReferenceStatement` on invoke path (`SourceIP` from XFF/RemoteAddr)
- Athena: in-process `JOIN` / `INNER JOIN`, `GROUP BY` + `COUNT(*)`, `ORDER BY` (no Trino)
- AppSync: nested GraphQL selections (depth ≤ 3) with per-type resolvers; JWT/PassRole unchanged
- Docs: Terraform lab-JSON surfaces stay SDK-only; skip `lab-fullstack` from WSL when Compose publishes `127.0.0.1:4566` only; nested OpenSearch/MQ/Firehose OS smoke remains skip-without-engine

### Stub depth: cron weekdays, Cognito CustomMessage, ELBv2 host-header

- Shared Scheduler/Secrets cron matcher accepts DOM `nW` (nearest weekday, month-bounded) and `LW` (last weekday of month)
- Cognito `AdminCreateUser` sync-Invokes `CustomMessage_AdminCreateUser` and stores rendered SMS/email bodies (lab stub `{####}`; no SES)
- ELBv2 `CreateRule` accepts `host-header` Conditions (exact or trailing `*` prefix), AND-combined with `path-pattern` when both set; lab `/alb/` match uses request Host
- SDK suite files renamed to feature-oriented names (`edge_test.go`, `data_lab_test.go`, `workflow_governance_test.go`, `nested_lab_test.go`; Node `edge|data|workflow|nested.test.mjs`)

### Gap closure / SDK coverage

- Firehose `DescribeDeliveryStream` returns `AmazonopensearchserviceDestinationDescription` and lab `OpenSearchDestinationDescription` (`DomainARN`, `IndexName`, `RoleARN`) for OpenSearch destinations
- Go / Node / Python SDK suites cover CloudFront edge, Transfer PutFile/GetFile, Glue crawlers, AppConfig StartDeployment, Config history GetObject, SFN Choice, CloudTrail trail delivery, Route53 Alias, ELBv2 CreateRule, and AppSync serviceRoleArn + multi-field resolvers (CloudFront/Route53/ELBv2/Transfer via SigV4 JSON lab protocol)
- Nested OpenSearch query facade, ActiveMQ, Firehose OpenSearch Describe, and Lambda MQ ESM SDK rows skip when engines are not Active/RUNNING (not a default CI gate)

### Edge, broker, search, analytics, and governance depth

- CloudFront: Create returns `Deployed` with lab `DomainName`; SigV4 edge GET `/cloudfront/{id}/{key...}` fetches first origin (S3 or internal HTTP API)
- Amazon MQ: nested ActiveMQ when DinD is up (same Internal AMQP posture as RabbitMQ); no DinD still fail-closed `CREATION_FAILED` + `stub://`
- OpenSearch: SigV4 lab query facade for `_doc` index and allowlisted `_search` to nested domain hosts only
- Athena: in-process `WHERE col = 'literal'` and `COUNT(*)`
- WAFv2: ByteMatch on `UriPath` / `SingleHeader` (`CONTAINS` / `EXACTLY`) on invoke paths
- Glue: crawler lite (`CreateCrawler` / `StartCrawler` / `GetCrawler` / `DeleteCrawler` / `ListCrawlers`) sync-infers CSV/JSON tables from S3
- Config: `StartConfigurationRecorder` writes a lab-shaped JSON snapshot to each delivery channel bucket before setting recording
- Transfer Family: servers report `ONLINE`; lab Put/Get/List file API on `:4566` under sandbox homes (not real SFTP)
- AppConfig: `StartDeployment` / `GetDeployment` / `ListDeployments`; GetConfiguration and AppConfigData serve the deployed version pointer

### Workflow, governance, and edge depth

- Step Functions: ASL `Choice` / `Wait` (Seconds 0–5) / `Parallel` (sequential branches, array merge)
- CloudTrail: `CreateTrail` / `DescribeTrails` / `DeleteTrail` / `StartLogging` / `StopLogging` with lab JSONL snapshot delivery to S3 and optional Logs
- Firehose: OpenSearch destination (`AmazonopensearchserviceDestinationConfiguration`) indexes to nested Active domains via allowlisted hosts
- Route 53: Type A `AliasTarget` to in-account CloudFront `DomainName` or ELB `DNSName`
- ELB v2: path-pattern and host-header `CreateRule` / `DescribeRules` / `DeleteRule`; lab `/alb/` listener matches rules (AND when both fields present) before default forward
- Lambda: Amazon MQ event source mapping (RUNNING nested broker; allowlisted hosts; injectable receive in unit tests)
- AppSync: optional data-source `serviceRoleArn` PassRole; flat multi-field Query GraphQL invoke

## 1.1.1

Patch after 1.1.0: residual lab-core depth, Node/Python SDK parity, and remaining Unreleased items. Docker Hub: `kyaxris/noctaxris` (`1.1.1`, `1.1`, `1`, `latest`). Cut steps: [docs/release.md](docs/release.md).

### Secrets cron L / # / day-of-week names

- Cron fields accept DOM `L`, DOW `N#M` / `SUN#1`, DOW `NL` / `FRIL`, and month/DOW names (`JUL`, `MON-FRI`); shared by Secrets RotationRules and Scheduler expressions

### Cognito CUSTOM_AUTH and CustomMessage body rendering

- `InitiateAuth` `CUSTOM_AUTH` sync-Invokes Define/Create/Verify challenge Lambdas; `RespondToAuthChallenge` `CUSTOM_CHALLENGE` with `ANSWER`
- `CustomMessage_SignUp` validates/renders/stores `smsMessage` / `emailMessage` / `emailSubject` (stub code `123456`; no SES)

### Cognito PreToken claim overrides and low-cost trigger Invokes

- PreTokenGeneration V1 `claimsOverrideDetails` (`claimsToAddOrOverride` / `claimsToSuppress`) applied to ID tokens; reserved claims ignored
- Sync Invoke for `CustomMessage_SignUp` and `UserMigration_Authentication` on password auth when the user is missing
- Trigger invoker returns Lambda payload bytes for claim/migration parsing; fail closed on Invoke or parse errors

### Secrets cron lists/ranges/steps and Duration jitter

- Cron fields accept lists, ranges, and steps (`1,13`, `1-5`, `*/6`, `2/10`, `1-10/2`)
- Optional RotationRules `Duration` picks a random offset in `[0, Duration)` for `NextRotationDate` (empty Duration keeps window start)

### ECS host-gateway Compose overlay

- Opt-in `docker/compose.lab-ecs-host-gateway.yaml` sets `NOCTAXRIS_INJECT_ECS_HOST_GATEWAY=1`; default Compose passes the env defaulting to `0`

### Node/Python SDK parity

- Node.js (`tests/sdk/nodejs`) and Python (`tests/sdk/python`) match Go lab coverage for APIGW HTTP CORS, S3→SQS notifications, Cognito `USER_SRP_AUTH` + LambdaConfig fail-closed, and EventBridge RoleArn-less SQS delivery
- Node engines `>=24`; clients include `apigatewayv2` and `cognito-idp`

### Integration suite deepen

- Go SDK: Cognito PreSignUp fail-closed, S3→SQS notifications, DynamoDB TransactWrite + OLD_IMAGE streams, EventBridge RoleArn-less SQS, Secrets RotationRules defer, HTTP API CORS preflight/UpdateApi, ECR Registry V2 chunked PATCH (live HTTP)
- Node/Python SSM StringList; Terraform lab-fullstack StringList; CloudFormation ChangeSet Modify SSM Value + Cloud Control UpdateResource IAM User
- API Gateway HTTP management responses use restJson1 camelCase + `application/json` (AWS SDK Go v2 CreateApi/CreateIntegration deserialize)

## 1.1.0

Minor release after 1.0.0: lab-core depth, inter-service wiring, CLI fidelity, and secure-default honesty. Docker Hub: `kyaxris/noctaxris` (`1.1.0`, `1.1`, `1`, `latest`). Cut steps: [docs/release.md](docs/release.md).

### Cognito SRP, trigger PassRole, and sync Lambda Invoke

- Cognito `InitiateAuth` `USER_SRP_AUTH` / `SRP_A` with `PASSWORD_VERIFIER` challenge (RFC 5054 3072-bit + Cognito HKDF); MFA still applies after a successful verifier
- Cognito `UpdateUserPool` (and Create) lab `RoleArn` + `LambdaConfig` trigger ARNs; PassRole for `cognito-idp.amazonaws.com` with pool `aws:SourceArn`
- Sync Lambda trigger Invoke for configured `PreSignUp`, `PostConfirmation`, `PreAuthentication`, `PostAuthentication`, and `PreTokenGeneration` (fail closed with `UnexpectedLambdaException`)

### Secrets RotationRules schedule, SSM StringList, RDS Data shapes

- Secrets Manager `RotationRules` (`AutomaticallyAfterDays` or lab `ScheduleExpression`: `rate(N days)`, `rate(N hours)` N≥4, `cron(0 H D M Dow *)`) plus optional `Duration` (`Nh`), `RotateImmediately=false` deferral, in-process due ticker, and `DescribeSecret` schedule fields
- SSM Parameter Store `StringList` type (comma-separated Value)
- RDS Data API `formatRecordsAs=JSON` and Batch `generatedFields` from `RETURNING` (`pgx`)

### ECR chunked PATCH, Lambda XA service principals, HTTP API CORS

- ECR Registry V2 chunked blob `PATCH` uploads (session under data root; finalize `PUT?digest=`; sequential `Content-Range` validated)
- Lambda function policies: service-principal grants with foreign-lab `SourceAccount` / `SourceArn` for XA S3/SNS/EventBridge notify (delivery Condition keys enforced)
- API Gateway HTTP API `CorsConfiguration` on CreateApi/UpdateApi (`AllowOrigins` / `AllowMethods` / `AllowHeaders` / `ExposeHeaders` / `MaxAge` / `AllowCredentials`); OPTIONS preflight without an OPTIONS route; CORS headers on invoke responses

### Cloud Control / CFN / Logs / SNS lab depth

- Cloud Control `UpdateResource`: IAM User (`Policies` / `ManagedPolicyArns` / `Groups`), Group (`Policies` / `ManagedPolicyArns`), ManagedPolicy (`PolicyDocument` + re-attach), EventBus `Policy`, LogGroup `RetentionInDays`
- CloudFormation: `AWS::Lambda::Permission` `FunctionUrlAuthType` / `InvokedViaFunctionUrl` (wildcard Principal requires FunctionUrlAuthType; `PrincipalOrgID` / `EventSourceToken` fail closed); `AWS::SNS::Subscription` `FilterPolicy` / `FilterPolicyScope` / `RawMessageDelivery` (+ persisted Delivery/Redrive attrs); `AWS::Logs::LogGroup` `RetentionInDays`
- CloudWatch Logs: `PutRetentionPolicy` / `DeleteRetentionPolicy`; DescribeLogGroups reports `retentionInDays`; expired events purged on put/get/describe
- SNS: subscription attributes + lab FilterPolicy match before fan-out; RawMessageDelivery for SQS

### DynamoDB Streams view types and TransactWrite fidelity

- DynamoDB Streams `StreamViewType`: `OLD_IMAGE` and `NEW_AND_OLD_IMAGES` (with `NEW_IMAGE` / `KEYS_ONLY`); GetRecords and Lambda ESM events include `OldImage` when present; ESM FilterCriteria matches `dynamodb.OldImage`
- `TransactWriteItems` appends stream records for successful Put/Delete/Update when the table stream is enabled
- `TransactWriteItems` `ClientRequestToken` idempotency (10-minute window; `IdempotentParameterMismatchException` on parameter change)

### PassRole `aws:SourceArn` on remaining configure paths

- Scheduler `CreateSchedule`/`UpdateSchedule`, Pipes `CreatePipe`, Secrets Manager Lambda `RotateSecret`, and API Gateway HTTP `CredentialsArn` / `AuthorizerCredentialsArn` PassRole now set trust `aws:SourceArn` to the schedule / pipe / secret / HTTP API ARN (alongside existing Lambda, EventBridge PutTargets, ECS, and Cognito trigger RoleArn paths).

### Out of lab scope and will-not-ship disposition

- Reclassified infinite-SAR / second-port / Insights-engine leftovers and recorded Blocked non-goals (host DB ports, open SNS HTTPS, APIGW HTTP_PROXY, non-lab private registries) as out of lab scope or will-not-ship on service docs and README; no Implement coding in this change

### Core leftovers (Cognito MFA, Transact Update, multi-shard Kinesis, Logs filter subset, Secrets four-step, EventBridge resource-policy delivery)

- Cognito TOTP MFA: `AssociateSoftwareToken` / `VerifySoftwareToken` / `RespondToAuthChallenge` (`SOFTWARE_TOKEN_MFA`); password `InitiateAuth` returns challenge session until TOTP succeeds
- DynamoDB `TransactWriteItems` `Update` (SET/REMOVE via `ApplyUpdateExpression`) plus Put/Delete/Update/ConditionCheck `ConditionExpression` (lab subset; fail-closed on unknown operators)
- Kinesis `CreateStream` `ShardCount` 1..4; partition-key hash routing; per-shard iterators/records; Lambda ESM polls all shards sequentially (no Enhanced Fan-Out)
- CloudWatch Logs lab `filterPattern` subset for `FilterLogEvents` and subscription/metric filters (space-AND, quoted phrases, `?`/`*` globs, optional `-term` exclude); account Logs resource policies for EventBridge delivery
- Secrets Manager multi-version stages (`AWSCURRENT` / `AWSPENDING` / `AWSPREVIOUS`), `UpdateSecretVersionStage`, and Lambda rotate `createSecret` → `setSecret` → `testSecret` → `finishSecret`
- EventBridge `PutTargets` may omit `RoleArn` for Logs/Kinesis/SFN when the destination resource policy Allows `events.amazonaws.com` (+ SourceArn/SourceAccount); empty policy skips delivery (fail-closed). Lab SFN `PutResourcePolicy` for RoleArn-less StartExecution

### Cognito token lifecycle

- Cognito `InitiateAuth` `REFRESH_TOKEN_AUTH` / `REFRESH_TOKEN` against hashed refresh tokens (lab rotation issues a new refresh token); `RevokeToken` public IdP; unsigned CLI/SDK shapes

### RDS Data API transactions and batch

- RDS Data API: real `BeginTransaction` / `CommitTransaction` / `RollbackTransaction` via held nested `pgx` sessions (fail closed with `DatabaseUnavailableException` when the instance is unavailable or dial fails); `ExecuteStatement` with `transactionId`; `BatchExecuteStatement` (auto-commit or txn-scoped)

### CloudFormation ChangeSet Modify and Cloud Control UpdateResource

- CloudFormation ChangeSet execute: allowlisted in-place `Modify` (Removals, then Modify in dependency order, then Add); unknown Modify types or immutable property changes fail closed
- Cloud Control `UpdateResource`: property-object `PatchDocument` for CFN-aligned allowlist mutable subsets; unknown patch keys fail closed; sync `ProgressEvent` SUCCESS with recorded tokens

### DynamoDB transactions, Kinesis Lambda ESM, Logs FilterLogEvents, Secrets Lambda rotate

- DynamoDB `TransactWriteItems` (`Put` / `Delete` / `Update` / `ConditionCheck`) and `TransactGetItems` (same-account; lab soft cap 25; duplicate keys cancel; SQLite all-or-nothing)
- Lambda event source mapping for Kinesis stream ARNs (all shards polled sequentially); in-process poller with optional `FilterCriteria` on decoded `data` / `partitionKey`; function role needs `kinesis:GetRecords` / `GetShardIterator` / `DescribeStream`
- CloudWatch Logs `FilterLogEvents` (optional stream names, time bounds, lab `filterPattern` subset, lab page cap)
- Secrets Manager optional `RotationLambdaARN` on `RotateSecret`: PassRole for `secretsmanager.amazonaws.com`, four-step async Invokes with `AWSPENDING` staging, then `finishSecret` promotion (default random rotate unchanged)

### Interservice depth (S3 notifications, EventBridge patterns, CFN policies)

- S3 bucket notifications: Put/Get configuration plus emit on PutObject / DeleteObject / CompleteMultipartUpload to Lambda (async), SQS, EventBridge (`aws.s3`), and SNS Publish (HTTP subscribers remain allowlist/catcher-only); destination authz re-checked on emit
- EventBridge content-based pattern operators: nested `detail`, `prefix` / `suffix`, `exists`, `anything-but`, `numeric`, `equals-ignore-case` (`wildcard` / `$or` / IP still rejected at PutRule)
- EventBridge delivery without RoleArn for SQS/Lambda/SNS/Logs/Kinesis/SFN: destination resource policy must Allow `events.amazonaws.com` (or account root) with `aws:SourceArn` / `aws:SourceAccount`; RoleArn session path unchanged
- CloudFormation: `AWS::IAM::ManagedPolicy`, `AWS::IAM::Policy`, `AWS::S3::BucketPolicy`, `AWS::Lambda::Permission`, and `AWS::S3::Bucket` `NotificationConfiguration`
- CloudFormation catalog: `AWS::SNS::TopicPolicy`, `AWS::SNS::Subscription`, `AWS::Logs::LogGroup`, `AWS::KMS::Alias`, `AWS::IAM::User`, `AWS::IAM::Group`; SQS queue attribute props wired on create; `ScheduleExpression` on `AWS::Events::Rule` fail-closed
- Lambda ESM `FilterCriteria`: Create/Update validate EventBridge-style patterns (max 5 Filters); SQS and DynamoDB Streams pollers apply filters before Invoke; SQS nested JSON `body` plus `messageId`; DynamoDB `eventName` / `Keys` / `NewImage`; operators `prefix` / `suffix` / `exists` / `anything-but` / `numeric` / `equals-ignore-case` (`$or` / `wildcard` / `cidr` rejected)

### Stabilization (CLI fidelity + compute honesty)

- Lambda `FunctionConfiguration.Layers` returns AWS Layer objects (`Arn`, `CodeSize`) instead of bare ARN strings
- API Gateway HTTP API REST `/v2/apis...` routed before lab ECR Registry `/v2/`; `GetIntegrations` / `GetRoutes` / `GetAuthorizers` list APIs
- Lambda REST: `AddPermission` (`/policy`) and Function URL config (`/2021-10-31/.../url`)
- Cognito `InitiateAuth` accepts unsigned AWS CLI requests (public IdP API)
- Restricted DinD is the Compose default (`privileged: false` + caps/cgroup); `compose.engine-privileged.yaml` remains host opt-in
- OpenSearch nested create: lab `FailureReason` on CreateFailed for mmap / memory-lock bootstrap; ops docs for Linux / Desktop / WSL `vm.max_map_count` (fail-closed; no host sysctl from the API container)
- Nested task HostConfig adds pids limit (1024); audit `events.jsonl` chmod enforced to `0644`

## 1.0.0

First public semver release. Docker Hub image: `kyaxris/noctaxris` (`1.0.0`, `latest`; nightlies via `nightly` / `nightly-YYYYMMDD`). Cut steps: [docs/release.md](docs/release.md).

### CI / supply chain

- `govulncheck` CI uses `go run ./scripts/govulncheck-ci` with an explicit allowlist for five Docker Engine CVEs reported against `github.com/docker/docker` with Fixed in: N/A (no client-module bump available). All other findings still fail the job. Residual risk and non-use of archive/`docker cp` APIs: [docs/security-defaults.md](docs/security-defaults.md)
- Nightly and tag-based Docker Hub publish workflows (canonical repo only; `DOCKERHUB_USERNAME` / `DOCKERHUB_TOKEN` secrets)
- Product version surface: `VERSION` file, image OCI labels, ldflags, `GET /_noctaxris/version`

### Storage and KMS custody

- Secrets Manager / SSM SecureString / DynamoDB SSE bind AWS-shaped KMS EncryptionContext on seal/unseal and EvaluateKMS (`SecretARN`+`SecretVersionId`, `PARAMETER_ARN`, `aws:dynamodb:tableName`+`aws:dynamodb:subscriberId`)
- DynamoDB `CreateTable` / `UpdateTable` SSE-KMS requires caller `kms:DescribeKey` and `kms:CreateGrant` with that table context
- Resource-based policies (KMS key, S3 bucket, Secrets, DynamoDB, ECR) require `Principal` on put; missing Principal matches none at eval (identity policies unchanged)
- S3 SSE-KMS persists customer `x-amz-server-side-encryption-context` pairs and rebuilds full context on Get/Copy/multipart
- Anonymous S3 GetObject / HeadObject opt-in: `NOCTAXRIS_ALLOW_ANONYMOUS_S3=1` plus bucket policy Principal `"*"` / `{"AWS":"*"}` Allow or object `x-amz-acl` `public-read` (default off; gate does not skip SigV4 for other S3 APIs)

### HTTP API Lambda authorizer

- HTTP API `CreateAuthorizer` REQUEST (Lambda) with CUSTOM routes; Deny short-circuits before integration
- Authorizer invoke uses CredentialsArn or `apigateway.amazonaws.com` resource-policy parity with AWS_PROXY
- Proxy responses strip hop-by-hop headers and `Set-Cookie` (opt-in `NOCTAXRIS_HTTP_API_ALLOW_SET_COOKIE=1`)
- Function URL CORS AllowOrigins allowlist; WAF Associate rejects REST / Cognito (no enforce path); ALB associate allowed with lab listener enforce

### ELB / CloudFront dataplane honesty

- ELBv2: `Type=network` rejected; Lambda `RegisterTargets` requires function resolve and `elasticloadbalancing.amazonaws.com` Allow
- ELB lab listener: `/alb/{account}/{name}/{port}/...` invokes registered Lambda (ALB event shape); open dataplane gate; WAF on LB ARN
- `DescribeTargetHealth` returns `healthy` when a listener forwards and permission Allows; `unused` without a listener; IP stays `unused`
- CloudFront **control-plane stub**: origins must exist (lab S3 / HTTP API); Status `InProgress` with DomainName omitted (not Deployed; no PoP)

### Compute path honesty

- Removed opt-in microVM / Firecracker selection path. Nested DinD via Compose `noctaxris-engine` is the only packaged compute plane
- `NOCTAXRIS_COMPUTE_RUNTIME` accepts only `dind` (or unset); `NOCTAXRIS_FIRECRACKER_BIN` removed
- Docs and README describe DinD-only compute; default engine is restricted DinD (`privileged: false` + caps/devices); `compose.engine-privileged.yaml` opt-in for broken hosts

### Nested engine integrity

- Default Compose engine is restricted DinD (`privileged: false` + `cap_add` / devices / security_opt + `cgroup: host` + `/sys/fs/cgroup` rw + dockerd `--ipv6=false`); opt-in `compose.engine-privileged.yaml` for hosts that cannot start nested containers
- Engine mounts `noctaxris-compute` `:ro` (API still unpacks read-write); compose-static tests lock the mount
- Nested Lambda/ECS/data-plane HostConfig: `Privileged=false`, empty CapAdd, CapDrop ALL, `no-new-privileges`
- Digest pins for Compose `docker:27-dind` and `busybox` init images
- Nested smoke covers zip Invoke + short ECS RunTask; privilege/compose-engine PRs must run `docker/smoke-nested.sh` (green `smoke-core` is not enough)

### Integration suites

- Real Compose-backed suites under `tests/`: Go / Node.js / Python AWS SDK round-trips, Terraform apply/destroy (S3 + IAM + DynamoDB + KMS), CloudFormation JSON/YAML stack CRUD for lab resource types
- Optional CI job `integration-suites` (`workflow_dispatch` or `tests/**` path filter); not a required PR gate
- `run-all.sh` folds SDK fullstack + Terraform `lab-fullstack` behind `NOCTAXRIS_ADVANCED=1`, and SDK Lambda Invoke behind `NOCTAXRIS_NESTED=1`
- Default SDK coverage includes SSM String and Secrets Manager create/get/delete; Go fullstack includes SecureString KMS decrypt-deny
- CFN suite expects YAML success (SQS, DynamoDB, Lambda ZipFile, multi-resource Ref/Sub)
- Nested smoke: weekly schedule + zip/Image Invoke + ECS RunTask; push/PR still proves `smoke-core` only
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

- RDS Data API: ExecuteStatement prefers `pgx` (`github.com/jackc/pgx/v5` v5.10.0) against the nested data-plane DSN only (typed OID fields + named `:name` parameters); falls back to nested `psql` (DinD exec, VARCHAR cells, literal parameter rewrite) when the wire dial fails; `DatabaseUnavailableException` without a nested container
- `NOCTAXRIS_RDS_DATA_PGX=0` forces nested-psql; nested DSN rejects loopback/IP/host-published endpoints
- Nested DinD smoke: `docker/smoke-nested.sh` (weekly schedule + Actions `workflow_dispatch` with `nested_smoke=true`, or run the script locally). Not on push/PR; a green PR proves `smoke-core` only. See [docs/ops.md](docs/ops.md).

### Security defaults and edge gates

- JWT / AppSync issuers: lab Cognito JWKS only by default; remote JWKS behind `NOCTAXRIS_ALLOW_REMOTE_JWKS` + public host allowlist (no redirects / no RFC1918)
- `NOCTAXRIS_DOCKER_HOST` allowlist (default `tcp://noctaxris-engine:2376`); reject `unix://`, `npipe://`, `docker.sock`; TLS client PEMs required when host is set
- Compose volume split: API-only `noctaxris-data` vs `noctaxris-compute` for Lambda code (engine cannot read `master.key`)
- Image pull allowlist for DinD (lab registry + pinned lab bases; `NOCTAXRIS_IMAGE_PULL_ALLOWLIST` for extras with digest pins)
- SigV4 requires `host` in SignedHeaders; SNS HTTP limited to lab catcher on `:4566` (or `NOCTAXRIS_SNS_HTTP_ALLOWLIST`)
- Non-loopback listen without TLS fails closed unless `NOCTAXRIS_ALLOW_NONLOOPBACK_LISTEN=1`; Function URL / HTTP API `NONE` gated on non-loopback via `NOCTAXRIS_ALLOW_OPEN_DATA_PLANE`
- AppSync API keys hashed at rest (HMAC with master key); Gateway JWT enforces `nbf`; CredentialsArn evaluated at invoke; `iam:PassedToService` on PassRole
- Internal network reuse inspect; optional `NOCTAXRIS_INJECT_HOST_GATEWAY=0`; nested data start fail → `failed`; CodeBuild/Batch non-zero exit → Failed
- Compose default: leave `NOCTAXRIS_ALLOW_OPEN_DATA_PLANE` unset; keep `ALLOW_NONLOOPBACK_LISTEN` with loud docs for the container bind
- WAF Associate requires an existing Web ACL; invoke association evaluate errors fail closed (403)
- HTTP API (no CredentialsArn) and AppSync Lambda invoke require resource policy Allow for `apigateway.amazonaws.com` / `appsync.amazonaws.com`
- SNS HTTP allowlist applies private/metadata host rejects and does not follow redirects
- Remote JWKS dial pins to IPs re-checked at connect time; Function URL / AppSync IAM require matching SigV4 service names

### Nested compute session and clamps

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

Deferred depth: [docs/services/index.md](docs/services/index.md). MemoryDB, Neptune, and real ML model runtimes remain deferred.

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

Multi-account honesty (cross-account dual eval, OU SCP/RCP inheritance, request-context keys) plus additional marketed services. At ship time, verification was `go test ./...` plus operator-run per-service Compose CLI smoke on each `docs/services/` page. Live CI contract (PR `smoke-core`, manual nested smoke): [docs/ops.md](docs/ops.md).

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
