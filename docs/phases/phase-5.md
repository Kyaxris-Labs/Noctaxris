# Phase 5 status

Phase 5 delivers a lab-complete S3 core: path-style buckets and objects, bucket policies, SSE-S3 and SSE-KMS (via Phase 4 KMS), and SigV4 query (presigned) GET/PUT.

## Delivered

| Area | Status |
|------|--------|
| CreateBucket, DeleteBucket, ListBuckets, HeadBucket | Done |
| PutObject, GetObject, DeleteObject, HeadObject, ListObjectsV2 | Done |
| PutBucketPolicy, GetBucketPolicy, DeleteBucketPolicy | Done |
| SSE-S3 (`AES256`) and SSE-KMS (`aws:kms`) Put/Get | Done |
| Path-style addressing only | Done |
| `EvaluateS3` (identity or bucket policy union, Deny-overrides) | Done |
| Presigned GET/PUT consume (query SigV4) | Done |
| Deferred S3 depth | [../deferred.md](../deferred.md) |

## Authz note

Same-account S3 uses identity **or** bucket policy Allow. Explicit Deny in either wins. Unlike KMS, a bucket policy alone can grant access.

## Smoke (Compose)

See [../verification.md](../verification.md) for `aws s3 mb/cp/ls`, SSE headers, and `aws s3 presign`.

## Explicitly not in Phase 5

See [../deferred.md](../deferred.md). No multipart, versioning, virtual-hosted-style, DynamoDB, SQS, or Lambda.
