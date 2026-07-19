# Architecture (Phase 7)

One Go binary runs in the API container. There is no sidecar API gateway and no host Docker socket mount. Compose also starts `noctaxris-engine` (Docker-in-Docker) on the Compose network so Lambda Invoke can run nested function containers.

## Tree

```text
Noctaxris/
  cmd/noctaxris/
  docker/                         # API image + compose (noctaxris + noctaxris-engine)
  internal/config/
  internal/compute/               # nested DinD client, Internal network, one-shot Invoke
  internal/store/                 # users, IAM, KMS, S3, DynamoDB, SQS, Lambda, orgs, IdP
  internal/catalog/
  internal/kernel/audit/
  internal/kernel/authn/          # SigV4 header + query (presign)
  internal/kernel/authz/          # Evaluate*, CheckPassRole
  internal/kernel/identity/
  internal/kernel/federation/
  internal/kernel/sts/
  internal/services/iam/
  internal/services/kms/
  internal/services/s3/           # object AES-GCM helpers
  internal/services/dynamodb/     # item JSON + table crypto helpers
  internal/services/sqs/          # queue JSON + message SSE helpers
  internal/services/lambda/       # CreateFunction / Invoke JSON helpers
  internal/services/organizations/
  internal/server/
  docs/
```

## Request path

```text
HTTP request
  ├─ GET /_noctaxris/health → 200 ok
  └─ else
       ├─ AssumeRoleWithSAML / AssumeRoleWithWebIdentity → IdP crypto (no SigV4)
       ├─ else authn.Verify (SigV4 header or query)
       ├─ if S3 path-style REST (service s3 / empty Action) → EvaluateS3 then handler
       ├─ GetCallerIdentity → XML (no IAM Evaluate)
       ├─ DynamoDB / SQS → EvaluateDynamoDB / EvaluateSQS then handler
       ├─ Lambda → identity Evaluate, PassRole on role configure, then handler
       │    └─ Invoke → mint execution-role session, compute.RunInvoke via noctaxris-engine
       ├─ other STS / IAM / KMS / Organizations → existing Evaluate paths
       └─ unknown → 501 NotImplemented
```

Object bytes live under `$DATAROOT/s3/{account}/{bucket}/...`. Lambda zip contents live under `$DATAROOT/lambda/...` and are shared with DinD through the Compose data volume. Bucket metadata, object metadata (etag, SSE), DynamoDB tables/items, SQS queues/messages, and Lambda function metadata live in SQLite.

## Compute path

Compose sets `NOCTAXRIS_DOCKER_HOST=tcp://noctaxris-engine:2375`. The API process never mounts host `/var/run/docker.sock`. `noctaxris-engine` is privileged DinD so nested containers can start. Function containers attach to DinD network `noctaxris-fn` with `Internal: true` (no public internet route by default). Empty `NOCTAXRIS_DOCKER_HOST` disables compute so unit tests can run without DinD.

## Authz

- Same-account identity: `Evaluate`
- S3: `EvaluateS3`. Allow if identity **or** bucket policy Allows (Deny-overrides)
- DynamoDB: `EvaluateDynamoDB`. Allow if identity **or** table resource policy Allows (Deny-overrides)
- SQS: `EvaluateSQS`. Allow if identity **or** queue policy Allows (Deny-overrides)
- KMS: `EvaluateKMS`. Key-policy explicit Allow (or grant) plus identity
- Lambda configure: `CheckPassRole` (caller `iam:PassRole` plus `lambda.amazonaws.com` trust)
- Session policies: `EvaluateWithSession` intersection
- Cross-account AssumeRole: `EvaluateCrossAccount`

## Identity and data

- Env-injected keys are management account root
- IAM user secrets and KMS CMK material sealed at rest
- S3 SSE-S3 DEKs sealed with the data-root master key
- SSE-KMS uses Phase 4 GenerateDataKey / Decrypt under the caller
- DynamoDB item ciphertext uses table SSE (AWS-owned or customer-managed KMS)
- SQS message bodies use SSE-SQS or SSE-KMS when queue attributes request encryption
- Lambda Invoke injects temporary AWS_* credentials for the function execution role
