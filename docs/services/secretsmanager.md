# Secrets Manager

**Status:** shipped

Lab-complete Secrets Manager core: create, read, update, delete, describe, and list secrets. Resource policies on secrets, KMS encryption via KeyId or lab `alias/aws/secretsmanager`, and identity-or-resource-policy authz.

## Implemented

| Area | Actions |
|------|---------|
| Secrets | `CreateSecret`, `GetSecretValue`, `PutSecretValue`, `DeleteSecret`, `RestoreSecret`, `RotateSecret`, `DescribeSecret`, `ListSecrets` |
| Tags | `ListTagsForResource`, `TagResource`, `UntagResource` |
| Resource policy | `PutResourcePolicy`, `GetResourcePolicy`, `DeleteResourcePolicy` |
| Payload | `SecretString` and/or `SecretBinary` (base64 on wire). `CreateSecret` may omit both (metadata-only; use `PutSecretValue` for the first version, matching Terraform `aws_secretsmanager_secret` + `aws_secretsmanager_secret_version`) |
| KMS | Optional `KmsKeyId` on create. Defaults to lab `alias/aws/secretsmanager` (per-account CMK seeded on first use) |
| Delete / recovery | `DeleteSecret` schedules deletion with `RecoveryWindowInDays` (7–30, default 30). `ForceDeleteWithoutRecovery` deletes immediately. `RestoreSecret` clears a scheduled deletion. On-read sweeper hard-deletes after `DeletionDate` |
| Rotate | Lab `RotateSecret` replaces the secret string with a new random value (no Lambda rotation function) |

Secret metadata and sealed values live in SQLite. ARNs include a random six-character hex suffix (AWS-shaped).

### Authz notes

Secrets Manager uses `authorizeDataplaneOR` with the shared resource dual-eval helper (same OR/AND rules as DynamoDB table policies) and the secret owner account from the secret ARN. Same-account access: allow if identity **or** secret resource policy Allows. Cross-account access: allow only when identity **and** secret resource policy both Allow. Empty resource policy denies cross-account callers. Explicit Deny in either wins. `CreateSecret` and `ListSecrets` use identity-only `EvaluateFull`. Org SCP/RCP filters apply before evaluation. When identity Allows, permissions boundary and session intersect.

Plaintext paths also call `EvaluateKMS` on the secret CMK: `kms:Encrypt` for `CreateSecret` / `PutSecretValue` / `RotateSecret`, and `kms:Decrypt` for `GetSecretValue` (identity plus key policy, matching S3/DynamoDB SSE-KMS). A secretsmanager Allow alone is not enough when the CMK key policy or caller identity denies KMS.

Pass a full secret ARN as `SecretId` for cross-account `GetSecretValue`. Cross-account reads also need the trusting account key policy (and caller identity) to Allow `kms:Decrypt`.

## How to verify / CLI smoke

Shared Compose and env setup: [index.md](index.md#shared-verification).

```bash
SECRET="noctaxris-lab-$RANDOM"
aws secretsmanager create-secret \
  --name "$SECRET" \
  --secret-string hello-secrets \
  --endpoint-url "$EP"

aws secretsmanager get-secret-value \
  --secret-id "$SECRET" \
  --endpoint-url "$EP"

aws secretsmanager put-secret-value \
  --secret-id "$SECRET" \
  --secret-string updated-value \
  --endpoint-url "$EP"

aws secretsmanager describe-secret \
  --secret-id "$SECRET" \
  --endpoint-url "$EP"

aws secretsmanager list-secrets \
  --endpoint-url "$EP"
```

Resource policy lifecycle:

```bash
POLICY='{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Action":"secretsmanager:GetSecretValue","Resource":"*"}]}'

aws secretsmanager put-resource-policy \
  --secret-id "$SECRET" \
  --resource-policy "$POLICY" \
  --endpoint-url "$EP"

aws secretsmanager get-resource-policy \
  --secret-id "$SECRET" \
  --endpoint-url "$EP"
```

A principal with no identity Allow for `secretsmanager:GetSecretValue` can still pass the Secrets Manager dual-eval when the secret resource policy Allows that action (same OR semantics as DynamoDB table policies), but `GetSecretValue` still requires caller `kms:Decrypt` via EvaluateKMS on the secret CMK. Handler tests cover this path.

```bash
aws secretsmanager delete-resource-policy \
  --secret-id "$SECRET" \
  --endpoint-url "$EP"

aws secretsmanager delete-secret \
  --secret-id "$SECRET" \
  --recovery-window-in-days 7 \
  --endpoint-url "$EP"

aws secretsmanager restore-secret \
  --secret-id "$SECRET" \
  --endpoint-url "$EP"

aws secretsmanager rotate-secret \
  --secret-id "$SECRET" \
  --endpoint-url "$EP"
```

Two-account cross-account GetSecretValue (member account B owns the secret, member account A user reads via dual eval):

```bash
SECRET_ARN=$(aws secretsmanager create-secret --name "$SECRET" --secret-string hello-xa \
  --endpoint-url "$EP" --profile account-b --query ARN --output text)
aws secretsmanager put-resource-policy --secret-id "$SECRET" --endpoint-url "$EP" --profile account-b \
  --resource-policy '{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":{"AWS":"arn:aws:iam::ACCOUNT_A:user/reader"},"Action":"secretsmanager:GetSecretValue","Resource":"*"}]}'
aws secretsmanager get-secret-value --secret-id "$SECRET_ARN" --endpoint-url "$EP" --profile account-a
```

## Not yet / deferred

- Full Secrets Manager SAR beyond the lab set (Lambda-backed rotation, random password generation, version stages, tags, replication, filtering on `ListSecrets`, full pagination parity)
- Service-linked grant approximation that skips caller KMS (lab requires caller EvaluateKMS for CTF fidelity)
- True AWS-owned `alias/aws/secretsmanager` key (lab convenience alias is a per-account CMK approximation)
