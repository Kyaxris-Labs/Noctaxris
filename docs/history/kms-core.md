# Lab-complete KMS (historical)

Historical ship snapshot: a lab-complete KMS core with customer-managed keys, sealed material, key-policy explicit-allow evaluation, Encrypt/Decrypt/GenerateDataKey, grants, and aliases.

## Delivered

| Area | Status |
|------|--------|
| CreateKey, DescribeKey, ListKeys, EnableKey, DisableKey | Done |
| GetKeyPolicy, PutKeyPolicy | Done |
| Encrypt, Decrypt, GenerateDataKey, GenerateDataKeyWithoutPlaintext | Done |
| CreateGrant, ListGrants, RetireGrant, RevokeGrant | Done |
| CreateAlias, ListAliases, DeleteAlias, UpdateAlias | Done |
| `EvaluateKMS` (identity + key policy explicit allow + grants) | Done |
| CMK material sealed at rest | Done |
| Deferred KMS depth | [../services/kms.md](../services/kms.md) |

## Authz note

KMS key policies are special. Identity Allow alone never authorizes a key-scoped operation. The key policy (or a matching grant) must explicitly allow the principal and action. CreateKey seeds a default policy that allows the account root (and the IAM user creator when applicable).

## Smoke (Compose)

See [../services/kms.md](../services/kms.md) for CreateKey, Encrypt/Decrypt, GenerateDataKey, and CreateAlias.

## Explicitly deferred at this ship

Current deferred depth: [../services/kms.md](../services/kms.md). At this ship there was no S3/SSE, DynamoDB, SQS, Lambda, or full KMS SAR parity.
