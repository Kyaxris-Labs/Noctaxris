# Deferred work (later phases or versions)

Items intentionally not completed yet. Phase 3 shipped lab IAM + full STS routing. Phase 4 ships lab-complete KMS. Phase 5 ships lab-complete S3. Phase 6 ships lab-complete DynamoDB and SQS. Phase notes: [phases/index.md](phases/index.md).

## IAM (later)

- Permission boundaries CRUD and Evaluate intersection with identity policies
- Groups and group policy attachment
- Instance profiles
- Service-linked roles
- IAM OpenID Connect / SAML provider CRUD APIs (beyond env/file IdP config used by STS federation)
- Full IAM pagination, tagging, and API parity beyond the lab subset
- PassRole enforcement on service Create/Update APIs when those services exist (Lambda and related phases)

## Organizations (later)

- OUs, SCPs, RCPs, invites, ListAccounts, and related control-plane APIs beyond CreateAccount / DescribeCreateAccountStatus

## STS (later depth)

- MFA device registry and MFA-gated GetSessionToken
- Full AssumeRoot parity with AWS Organizations / IAM Identity Center style flows
- Rich DecodeAuthorizationMessage payloads generated from every deny path
- Production-grade GetDelegatedAccessToken and GetWebIdentityToken parity

## KMS (later)

Phase 4 covers lab-complete keys, key policies (explicit allow), Encrypt/Decrypt/GenerateDataKey*, grants, and aliases. Still deferred:

- Full KMS SAR / API parity beyond the lab set (ReEncrypt, Sign/Verify, GenerateMac/VerifyMac, GetPublicKey, asymmetric key specs, ImportKeyMaterial, custom key stores, multi-Region replica keys, ScheduleKeyDeletion/CancelKeyDeletion/PendingDeletion depth, automatic rotation APIs, tags, full pagination parity)
- Generated KMS condition-key catalog from SAR / servicereference codegen
- Cross-account key policy and grant flows beyond same-account lab paths
- AWS-managed key types and service-linked defaults (including convenience aliases such as `alias/aws/dynamodb` and `alias/aws/sqs`) beyond lab customer-managed CMK paths

## S3 (later)

Phase 5 covers lab-complete buckets, objects, bucket policies, SSE-S3/SSE-KMS, and path-style presigned GET/PUT. Still deferred:

- Full S3 SAR / API parity (multipart upload APIs, CopyObject, versioning, lifecycle, CORS, website, Object Lock, replication, access points, inventory, Select, Glacier, tagging APIs beyond basics, ACLs beyond default private, ListObjectVersions, etc.)
- Virtual-hosted-style endpoints and Transfer Acceleration
- Cross-account bucket policy depth beyond same-account lab paths
- Generated S3 condition-key catalog from SAR / servicereference codegen
- Presigned POST, multipart presign, and clock-skew edge cases beyond current authn
- Bucket default encryption configuration APIs (`PutBucketEncryption`) as a later convenience (Phase 5 uses per-request SSE headers)

## DynamoDB (later)

Phase 6 covers lab-complete tables, item CRUD, basic Query/Scan, BatchGet/BatchWrite, table resource policies, and encryption at rest with customer-managed KMS. Still deferred:

- Full DynamoDB SAR / API parity (GSI/LSI, Streams, Transactions, PartiQL, Contributors Insights, export/import, global tables, TTL APIs, continuous backups, PITR, on-demand vs provisioned billing depth, tags, full pagination parity)
- Generated DynamoDB condition-key catalog from SAR / servicereference codegen
- Cross-account table resource policy depth beyond same-account lab paths
- AWS-managed encryption key convenience aliases beyond lab customer-managed CMK smoke

## SQS (later)

Phase 6 covers lab-complete standard queues, send/receive/delete (including batch and visibility), queue policies via attributes, SSE-SQS, and SSE-KMS. Still deferred:

- Full SQS SAR / API parity (FIFO queues, message deduplication, delay queue depth, DLQ redrive allow policies, high-throughput FIFO, tags beyond basics)
- Generated SQS condition-key catalog from SAR / servicereference codegen
- Cross-account queue policy depth beyond same-account lab paths
- AWS-managed SSE-KMS convenience aliases beyond lab customer-managed CMK smoke

## Cross-cutting (later)

- Generated global and service condition-key catalogs from AWS Service Authorization Reference / servicereference JSON
- Lambda (Phase 7 roadmap)
- PassRole enforcement until Lambda (Phase 7)
