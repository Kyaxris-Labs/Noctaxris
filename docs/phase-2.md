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

With root keys from `docker/.env`:

```bash
aws organizations create-account --email member@example.com --account-name Member --endpoint-url http://127.0.0.1:4566
aws organizations describe-create-account-status --create-account-request-id <Id> --endpoint-url http://127.0.0.1:4566
aws sts assume-role --role-arn arn:aws:iam::<AccountId>:role/OrganizationAccountAccessRole --role-session-name admin --endpoint-url http://127.0.0.1:4566
```

Export the returned temporary credentials, then:

```bash
aws sts get-caller-identity --endpoint-url http://127.0.0.1:4566
```

Expect an assumed-role ARN in the member account.

## Explicitly not in Phase 2

- OUs, SCPs, invites, ListAccounts
- Full STS surface beyond AssumeRole + GetCallerIdentity
- IAM user/role control-plane APIs
- S3, DynamoDB, SQS, KMS, Lambda
