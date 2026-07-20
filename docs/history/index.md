# Lab-core delivery history

Completed delivery slices for the **lab core**. This index is **history only**, not a live roadmap. Current lab scope is the root [README](../../README.md) Services table and [../../CHANGELOG.md](../../CHANGELOG.md). Smoke commands and deferred depth live under [../services/](../services/index.md).

| Focus | Doc |
|--------|-----|
| Scaffold, Docker, sealed secrets, health + auth gate | [scaffold.md](scaffold.md) |
| SigV4 + `GetCallerIdentity` | [sigv4.md](sigv4.md) |
| Organizations CreateAccount + cross-account `AssumeRole` | [organizations-mvp.md](organizations-mvp.md) |
| Lab IAM + all 11 STS actions (fail-closed federation) | [identity.md](identity.md) |
| Lab-complete KMS (keys, policies, crypto, grants, aliases) | [kms-core.md](kms-core.md) |
| Lab-complete S3 (objects, bucket policy, SSE, path-style presign) | [s3-core.md](s3-core.md) |
| Lab-complete DynamoDB + SQS (items, resource/queue policy, SSE) | [dynamodb-sqs-core.md](dynamodb-sqs-core.md) |
| Lab-complete Lambda (zip python3.12, PassRole, nested DinD, sync Invoke) | [nested-lambda-core.md](nested-lambda-core.md) |
