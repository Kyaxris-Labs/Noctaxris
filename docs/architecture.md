# Architecture (Phase 6)

One Go binary runs in the container. There is no sidecar API gateway and no Docker socket mount.

## Tree

```text
Noctaxris/
  cmd/noctaxris/
  docker/
  internal/config/
  internal/store/                 # users, IAM, KMS, S3, DynamoDB, SQS, orgs, IdP
  internal/catalog/
  internal/kernel/audit/
  internal/kernel/authn/          # SigV4 header + query (presign)
  internal/kernel/authz/          # Evaluate, EvaluateCrossAccount, EvaluateWithSession, EvaluateKMS, EvaluateS3, EvaluateDynamoDB, EvaluateSQS
  internal/kernel/identity/
  internal/kernel/federation/
  internal/kernel/sts/
  internal/services/iam/
  internal/services/kms/
  internal/services/s3/           # object AES-GCM helpers
  internal/services/dynamodb/     # item JSON + table crypto helpers
  internal/services/sqs/          # queue JSON + message SSE helpers
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
       ├─ other STS / IAM / KMS / Organizations → existing Evaluate paths
       └─ unknown → 501 NotImplemented
```

Object bytes live under `$DATAROOT/s3/{account}/{bucket}/...`. Bucket metadata, object metadata (etag, SSE), DynamoDB tables/items, and SQS queues/messages live in SQLite.

## Authz

- Same-account identity: `Evaluate`
- S3: `EvaluateS3`. Allow if identity **or** bucket policy Allows (Deny-overrides)
- DynamoDB: `EvaluateDynamoDB`. Allow if identity **or** table resource policy Allows (Deny-overrides)
- SQS: `EvaluateSQS`. Allow if identity **or** queue policy Allows (Deny-overrides)
- KMS: `EvaluateKMS`. Key-policy explicit Allow (or grant) plus identity
- Session policies: `EvaluateWithSession` intersection
- Cross-account AssumeRole: `EvaluateCrossAccount`

## Identity and data

- Env-injected keys are management account root
- IAM user secrets and KMS CMK material sealed at rest
- S3 SSE-S3 DEKs sealed with the data-root master key
- SSE-KMS uses Phase 4 GenerateDataKey / Decrypt under the caller
- DynamoDB item ciphertext uses table SSE (AWS-owned or customer-managed KMS)
- SQS message bodies use SSE-SQS or SSE-KMS when queue attributes request encryption
