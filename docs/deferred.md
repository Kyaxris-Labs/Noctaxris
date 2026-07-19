# Deferred work (later phases or versions)

Items intentionally not completed in Phase 3. Phase 3 ships a lab-complete IAM subset and all 11 STS action routes with fail-closed federation. Phase notes: [phases/index.md](phases/index.md).

## IAM (later)

- Permission boundaries CRUD and Evaluate intersection with identity policies
- Groups and group policy attachment
- Instance profiles
- Service-linked roles
- IAM OpenID Connect / SAML provider CRUD APIs (beyond env/file IdP config used by STS federation)
- Full IAM pagination, tagging, and API parity beyond the lab subset
- PassRole enforcement on service Create/Update APIs when those services exist (KMS, S3, Lambda, and related phases)

## Organizations (later)

- OUs, SCPs, RCPs, invites, ListAccounts, and related control-plane APIs beyond CreateAccount / DescribeCreateAccountStatus

## STS (later depth)

- MFA device registry and MFA-gated GetSessionToken
- Full AssumeRoot parity with AWS Organizations / IAM Identity Center style flows
- Rich DecodeAuthorizationMessage payloads generated from every deny path
- Production-grade GetDelegatedAccessToken and GetWebIdentityToken parity

## Cross-cutting (later)

- Generated global and service condition-key catalogs from AWS Service Authorization Reference / servicereference JSON
- KMS, S3, DynamoDB, SQS, Lambda (later roadmap phases)
