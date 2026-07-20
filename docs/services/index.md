# Services

Per-service reference for the Noctaxris lab emulator. Status matches the root [README](../../README.md) Services table.

Each page covers what is implemented, how to verify with AWS CLI smoke, and what remains deferred.

| Service | Status | Doc |
|---------|--------|-----|
| [IAM](iam.md) | Shipped | Users, roles, policies, keys, groups, boundaries, IdPs, MFA |
| [STS](sts.md) | Shipped | All 11 actions (lab MFA on GetSessionToken) |
| [Organizations](organizations.md) | Shipped | Accounts, OUs, MoveAccount, SCP/RCP attach with OU-path inheritance |
| [KMS](kms.md) | Shipped | CMKs, key-policy-required crypto, cross-account dual eval, grants, lab aliases |
| [S3](s3.md) | Shipped | Path-style objects, multipart, CopyObject, bucket encryption, cross-account dual eval |
| [DynamoDB](dynamodb.md) | Shipped | Tables, items, one lab GSI, TTL, resource policies, cross-account dual eval |
| [SQS](sqs.md) | Shipped | Standard and FIFO queues, RedrivePolicy, policies, cross-account dual eval |
| [Lambda](lambda.md) | Shipped | Zip/Image, versions/aliases, layers, sync+async Invoke, cross-account policies, lab ECR Image pull, TLS DinD |
| [SSM Parameter Store](ssm.md) | Shipped | String and SecureString, KMS via alias/aws/ssm, identity authz |
| [Secrets Manager](secretsmanager.md) | Shipped | CRUD, list, resource policies, cross-account dual eval, KMS via alias/aws/secretsmanager |
| [SNS](sns.md) | Shipped | Topic CRUD, publish, subscribe, topic policies, cross-account dual eval, SQS and Lambda delivery |
| [EventBridge](eventbridge.md) | Shipped | Buses, rules, targets, PutEvents routing to SQS, Lambda, and SNS (RoleArn delivery sessions) |
| [ECR](ecr.md) | Shipped | Repository CRUD, auth token, policies, cross-account dual eval, Registry V2, DinD sync |
| [ECS](ecs.md) | Shipped | Task definitions, RunTask/list/stop, default cluster, PassRole, nested DinD |
| [CloudTrail](cloudtrail.md) | Shipped | LookupEvents over local JSONL audit |
| [CloudWatch Logs](logs.md) | Shipped | Log groups/streams, Put/GetLogEvents, DescribeLogGroups |
| [Resource Groups Tagging API](resourcegroupstaggingapi.md) | Shipped | TagResources, UntagResources, GetResources |
| [Kinesis Data Streams](kinesis.md) | Shipped | Stream CRUD, Put/Get records, single-shard iterators |
| [SES](ses.md) | Shipped | Local catcher: verify, SendEmail/SendRawEmail, ListIdentities, GetSendStatistics |
| [AppConfig](appconfig.md) | Shipped | Application/Environment/Profile, hosted versions, GetConfiguration, AppConfigData session |
| [Step Functions](stepfunctions.md) | Shipped | State machine CRUD, StartExecution, Pass/Succeed/Fail/Task to Lambda |

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
```

Expect body `ok`. Lambda Invoke needs the nested `noctaxris-engine` service from the same compose file.

Export root keys from `docker/.env`, then set a common CLI environment before any per-service smoke:

```bash
export AWS_ACCESS_KEY_ID=...
export AWS_SECRET_ACCESS_KEY=...
export AWS_DEFAULT_REGION=us-east-1
EP=http://127.0.0.1:4566
```

Prefer WSL or Linux for AWS CLI smoke against `http://127.0.0.1:4566`. On Windows, run the same commands inside WSL when Docker Desktop publishes that port on the Windows host.

Per-service CLI smoke lives on each shipped service page above.

## Cross-cutting

**Cross-account resource policies:** For S3, SQS, Lambda, ECR, SNS, Secrets Manager, and DynamoDB, same-account access stays identity **or** resource policy Allow. Cross-account access requires identity **and** resource policy Allow (empty resource policy denies). KMS stays key-policy-required, with cross-account callers needing both identity and key policy Allow. See each service Authz notes. EventBridge bus policy dual-eval depth remains deferred.

**Organizations SCP/RCP:** Member authorize loads policies attached to the account, each OU on the path to root, and the organization root. Management account is exempt. See [organizations.md](organizations.md).

**Condition keys:** Catalogs for lab-core services (IAM, STS, Organizations, KMS, S3, DynamoDB, SQS, Lambda, SSM, Secrets Manager, SNS, EventBridge, ECR, ECS) plus a global seed ship via `internal/catalog/conditionkeys` (servicereference snapshots and ADR-0005 §7 eval rules). Request context now populates `aws:SourceIp`, `aws:PrincipalArn`, `aws:PrincipalAccount`, `aws:RequestedRegion`, MFA keys, and `aws:ResourceTag/*` (plus matching service ResourceTag keys) when tags exist via the Tagging API. Broader operator matrix and every global key population remain open (partial today: StringEquals/Like/NotEquals, Null, IfExists variants).

**Post-v2:** MicroVM isolation (Firecracker-class) for Lambda, and later ECS if needed. Nested DinD remains the default. See [lambda.md](lambda.md).
