# Phase 2 status

Phase 2 adds Organizations MVP, cross-account dual evaluation, and a minimal `sts:AssumeRole` path.

## Delivered

| Area | Status |
|------|--------|
| `organizations:CreateAccount` (sync SUCCEEDED) | Done |
| `organizations:DescribeCreateAccountStatus` | Done |
| Member `OrganizationAccountAccessRole` trusted by management root | Done |
| `authz.EvaluateCrossAccount` (identity + trust) | Done |
| Minimal `sts:AssumeRole` + temp credentials | Done |
| SigV4 session token required for temp keys | Done |
| Role-session `GetCallerIdentity` | Done |

## Smoke (Compose)

See [../services/organizations.md](../services/organizations.md) for CreateAccount and [../services/sts.md](../services/sts.md) for AssumeRole into `OrganizationAccountAccessRole`.

## Explicitly not in Phase 2

- OUs, SCPs, invites, ListAccounts
- Full STS surface beyond AssumeRole + GetCallerIdentity
- IAM user/role control-plane APIs
- S3, DynamoDB, SQS, KMS, Lambda
