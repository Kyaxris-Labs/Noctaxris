# Deferred work (later phases or versions)

Items intentionally not completed yet. Phase 3 shipped lab IAM + full STS routing. Phase 4 ships lab-complete KMS. Phase notes: [phases/index.md](phases/index.md).

## IAM (later)

- Permission boundaries CRUD and Evaluate intersection with identity policies
- Groups and group policy attachment
- Instance profiles
- Service-linked roles
- IAM OpenID Connect / SAML provider CRUD APIs (beyond env/file IdP config used by STS federation)
- Full IAM pagination, tagging, and API parity beyond the lab subset
- PassRole enforcement on service Create/Update APIs when those services exist (S3, Lambda, and related phases)

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
- AWS-managed key types and service-linked defaults used when S3 / DynamoDB / SQS land

## Cross-cutting (later)

- Generated global and service condition-key catalogs from AWS Service Authorization Reference / servicereference JSON
- S3, DynamoDB, SQS, Lambda (later roadmap phases)
- PassRole enforcement until Lambda (Phase 7)
