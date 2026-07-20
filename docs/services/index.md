# Services

Per-service reference for the Noctaxris lab emulator. Status matches the root [README](../../README.md) Services table.

Each page covers what is implemented, how to verify with AWS CLI smoke, and what remains deferred.

| Service | Status | Doc |
|---------|--------|-----|
| [IAM](iam.md) | Shipped | Users, roles, policies, keys, groups, boundaries, IdPs, MFA |
| [STS](sts.md) | Shipped | All 11 actions (lab MFA on GetSessionToken) |
| [Organizations](organizations.md) | Shipped | Accounts, OUs, SCP/RCP create and attach |
| [KMS](kms.md) | Shipped | CMKs, ReEncrypt, deletion lifecycle, rotation flags, grants, lab aliases |
| [S3](s3.md) | Shipped | Path-style objects, multipart, CopyObject, bucket encryption, presign |
| [DynamoDB](dynamodb.md) | Shipped | Tables, items, one lab GSI, TTL lazy expiry, batch, resource policies |
| [SQS](sqs.md) | Shipped | Standard and FIFO queues, deduplication, RedrivePolicy, policies, SSE |
| [Lambda](lambda.md) | Shipped | Zip/Image, versions/aliases, layers, sync+async Invoke, resource policies, TLS DinD |
| [SSM Parameter Store](ssm.md) | Shipped | String and SecureString, KMS via alias/aws/ssm, identity authz |
| [Secrets Manager](secretsmanager.md) | Shipped | CRUD, list, resource policies, KMS via alias/aws/secretsmanager |
| [SNS](sns.md) | Shipped | Topic CRUD, publish, subscribe, topic policies, SQS and Lambda delivery |
| [EventBridge](eventbridge.md) | Shipped | Buses, rules, targets, PutEvents routing to SQS, Lambda, and SNS |
| [ECR](ecr.md) | Shipped | Repository CRUD, auth token, policies, Registry V2 on loopback, DinD sync |
| [ECS](ecs.md) | Shipped | Task definitions, RunTask/list/stop, default cluster, PassRole, nested DinD |

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

**Condition keys (v2 depth):** Catalogs for lab-core services (IAM, STS, Organizations, KMS, S3, DynamoDB, SQS, Lambda, SSM, Secrets Manager, SNS, EventBridge, ECR, ECS) plus a global seed ship via `internal/catalog/conditionkeys` (servicereference snapshots and ADR-0005 §7 eval rules). Broader operator matrix and request-context population for every global key remains open (partial today: StringEquals/Like/NotEquals, Null, IfExists variants).

**Post-v2:** MicroVM isolation (Firecracker-class) for Lambda, and later ECS if needed. v2 keeps nested DinD. See [lambda.md](lambda.md).
