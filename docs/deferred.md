# Deferred work

Items intentionally not completed in the lab core. Lab-core history: [phases/index.md](phases/index.md).

**v2 target:** Clear the in-scope items below (and add lab-complete SNS, Secrets Manager, EventBridge, ECR, ECS, and SSM Parameter Store). See [CHANGELOG.md](../CHANGELOG.md).

**Post-v2:** MicroVM isolation is called out in its own section and is not a v2 delivery requirement.

## IAM (v2)

- Service-linked roles
- Full IAM pagination, tagging, and API parity beyond the lab subset
- PassRole on non-Lambda service configure APIs when those services gain role ARNs later

## Organizations (v2)

- Account invites and related control-plane APIs beyond the lab subset
- OU-path SCP and RCP inheritance (account placement in OUs is not stored yet)
- Apply SCP, RCP, and permissions-boundary evaluation on S3, KMS, DynamoDB, SQS, and Lambda PassRole paths (today EvaluateFull runs on the shared IAM/STS/Orgs authorize path)

## STS (v2 depth)

- Full AssumeRoot parity with AWS Organizations / IAM Identity Center style flows
- Rich DecodeAuthorizationMessage payloads generated from every deny path
- Production-grade GetDelegatedAccessToken and GetWebIdentityToken parity

## KMS (v2)

Lab core covers keys, key policies (explicit allow), Encrypt/Decrypt/GenerateDataKey*, grants, and aliases. Still open:

- Full KMS SAR / API parity beyond the lab set (ReEncrypt, Sign/Verify, GenerateMac/VerifyMac, GetPublicKey, asymmetric key specs, ImportKeyMaterial, custom key stores, multi-Region replica keys, ScheduleKeyDeletion/CancelKeyDeletion/PendingDeletion depth, automatic rotation APIs, tags, full pagination parity)
- Cross-account key policy and grant flows beyond same-account lab paths
- AWS-managed key types and service-linked defaults (including convenience aliases such as `alias/aws/dynamodb` and `alias/aws/sqs`) beyond lab customer-managed CMK paths

## S3 (v2)

Lab core covers buckets, objects, bucket policies, SSE-S3/SSE-KMS, and path-style presigned GET/PUT. Still open:

- Full S3 SAR / API parity (multipart upload APIs, CopyObject, versioning, lifecycle, CORS, website, Object Lock, replication, access points, inventory, Select, Glacier, tagging APIs beyond basics, ACLs beyond default private, ListObjectVersions, etc.)
- Virtual-hosted-style endpoints and Transfer Acceleration
- Cross-account bucket policy depth beyond same-account lab paths
- Presigned POST, multipart presign, and clock-skew edge cases beyond current authn
- Bucket default encryption configuration APIs (`PutBucketEncryption`) as a later convenience (lab core uses per-request SSE headers)

## DynamoDB (v2)

Lab core covers tables, item CRUD, basic Query/Scan, BatchGet/BatchWrite, table resource policies, and encryption at rest with customer-managed KMS. Still open:

- Full DynamoDB SAR / API parity (GSI/LSI, Streams, Transactions, PartiQL, Contributors Insights, export/import, global tables, TTL APIs, continuous backups, PITR, on-demand vs provisioned billing depth, tags, full pagination parity)
- Cross-account table resource policy depth beyond same-account lab paths
- AWS-managed encryption key convenience aliases beyond lab customer-managed CMK smoke

## SQS (v2)

Lab core covers standard queues, send/receive/delete (including batch and visibility), queue policies via attributes, SSE-SQS, and SSE-KMS. Still open:

- Full SQS SAR / API parity (FIFO queues, message deduplication, delay queue depth, DLQ redrive allow policies, high-throughput FIFO, tags beyond basics)
- Cross-account queue policy depth beyond same-account lab paths
- AWS-managed SSE-KMS convenience aliases beyond lab customer-managed CMK smoke

## Lambda (v2)

Lab core covers zip CreateFunction / Get / Delete / List / UpdateCode / UpdateConfiguration, sync Invoke, PassRole plus `lambda.amazonaws.com` trust, nested containers via an internal Compose engine (no host `docker.sock`), execution-role session injection, and platform egress deny for function containers. Still open for v2:

- Full Lambda SAR / API parity (layers, versions/aliases depth, event source mappings, concurrency, Function URLs, SnapStart, VPC ENI, recursive loop protection depth, tags, tracing, code signing, container image packaging, multi-runtime matrix)
- Async invoke, retries, DLQ / on-failure destinations
- Nested container engine hardening beyond lab DinD (rootless, TLS to engine by default)
- Cross-account function resource policy depth
- Runtimes other than `python3.12`

## Cross-cutting (v2)

- Condition-key catalogs for lab-core services (IAM, STS, Organizations, KMS, S3, DynamoDB, SQS, Lambda) plus global seed: **shipped** via `internal/catalog/conditionkeys` (servicereference snapshots + ADR-0005 §7 eval rules)
- Extend catalogs when new lab cores land (SSM, Secrets Manager, SNS, EventBridge, ECR, ECS)
- Broader operator matrix and request-context population for every global key (partial: StringEquals/Like/NotEquals, Null, IfExists variants)

## New lab cores (v2)

Not in lab core yet. Planned as lab-complete (not full SAR) after deferred clearance for existing services:

- SSM Parameter Store
- Secrets Manager
- SNS
- EventBridge
- ECR
- ECS

## Post-v2

- MicroVM isolation (Firecracker-class) for Lambda (and later ECS if needed). v2 keeps nested DinD.
