# Phase 1 status

Phase 1 adds SigV4 verification and the first real STS API: `GetCallerIdentity`.

## Delivered

| Area | Status |
|------|--------|
| SigV4 header + query verification | Done |
| Identity principal model | Done |
| Single-account IAM identity-policy Evaluate (unit tested) | Done |
| `sts:GetCallerIdentity` XML response | Done |
| HTTP wire-up: Verify then route | Done |
| Root CLI smoke against Compose endpoint | Required for gate |

## Behavior

1. `GET /_noctaxris/health` stays open (no SigV4).
2. All other paths require a valid SigV4 signature for a known access key.
3. `GetCallerIdentity` succeeds after SigV4 with no IAM permission check (matches AWS STS docs).
4. Any other action returns `501 NotImplemented` after successful authn.
5. Bad or missing signatures return `403` and a CloudTrail-shaped audit error line.

## CLI smoke

See [../services/sts.md](../services/sts.md) (`sts get-caller-identity`).

## Explicitly not in Phase 1

- IAM user/role control-plane APIs
- Remaining STS actions
- Multi-account / Organizations
- S3, DynamoDB, SQS, KMS, Lambda
