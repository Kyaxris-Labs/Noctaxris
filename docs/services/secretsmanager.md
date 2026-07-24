# Secrets Manager

**Status:** shipped

Lab-complete Secrets Manager core: create, read, update, delete, describe, and list secrets. Resource policies on secrets, KMS encryption via KeyId or lab `alias/aws/secretsmanager`, identity-or-resource-policy authz, and RotationRules scheduling (`AutomaticallyAfterDays` or lab `rate` / `cron` ScheduleExpression plus optional `Duration`) with an in-process due ticker.

## Implemented

| Area | Actions |
|------|---------|
| Secrets | `CreateSecret`, `GetSecretValue`, `PutSecretValue`, `DeleteSecret`, `RestoreSecret`, `RotateSecret`, `UpdateSecretVersionStage`, `DescribeSecret`, `ListSecrets` |
| Tags | `ListTagsForResource`, `TagResource`, `UntagResource` |
| Resource policy | `PutResourcePolicy`, `GetResourcePolicy`, `DeleteResourcePolicy` |
| Payload | `SecretString` and/or `SecretBinary` (base64 on wire). `CreateSecret` may omit both (metadata-only; use `PutSecretValue` for the first version, matching Terraform `aws_secretsmanager_secret` + `aws_secretsmanager_secret_version`) |
| KMS | Optional `KmsKeyId` on create. Defaults to lab `alias/aws/secretsmanager` (per-account CMK seeded on first use) |
| Delete / recovery | `DeleteSecret` schedules deletion with `RecoveryWindowInDays` (7–30, default 30). `ForceDeleteWithoutRecovery` deletes immediately. `RestoreSecret` clears a scheduled deletion. On-read sweeper hard-deletes after `DeletionDate` |
| Versions / stages | Multi-version rows with `AWSCURRENT`, `AWSPENDING`, and `AWSPREVIOUS`. `DescribeSecret` returns `VersionIdsToStages`. `GetSecretValue` accepts optional `VersionStage` / `VersionId`. `PutSecretValue` accepts optional `VersionStages` + `ClientRequestToken` (rotation `createSecret` path). `UpdateSecretVersionStage` moves or removes a label; moving `AWSCURRENT` clears `AWSPENDING` on the destination and labels the former current `AWSPREVIOUS` |
| Rotate (default) | Without a rotator, `RotateSecret` writes a new `AWSCURRENT` version with a random secret string (former current becomes `AWSPREVIOUS`) |
| Rotate (Lambda) | Optional `RotationLambdaARN` on `RotateSecret` persists the rotator. Caller needs `secretsmanager:RotateSecret` plus `iam:PassRole` on the function role (or optional `RotationRoleARN`) with trust `secretsmanager.amazonaws.com`. Lab runs four async Invokes: `createSecret` → `setSecret` → `testSecret` → `finishSecret`. Before the first invoke the lab creates an `AWSPENDING` version (`ClientRequestToken` as `VersionId`, random string). After all four succeed, `finishSecret` promotion moves `AWSCURRENT` onto that version. Mid-rotation failure leaves `AWSPENDING` and blocks a second `RotateSecret` until the label is cleared (`UpdateSecretVersionStage` or successful finish). `DescribeSecret` returns `RotationLambdaARN` when set |
| RotationRules | `RotateSecret` accepts `RotationRules` (`AutomaticallyAfterDays`, or `ScheduleExpression` — not both) and optional `Duration` / `RotateImmediately` (default `true`). Lab `ScheduleExpression`: `rate(N days)`, `rate(N hours)` (N≥4), or Secrets-shaped `cron(0 H D M Dow *)` (minutes must be `0`, year must be `*`). Cron fields accept digits, `*`, `?`, lists (`1,13`), ranges (`1-5`), steps (`*/6`, `2/10`, `1-10/2`), DOM `L` (last day of month), DOW `N#M` / `SUN#1` (nth weekday), DOW `NL` / `FRIL` (last weekday of month), and month/DOW names (`JUL`, `MON-FRI`). Optional `Duration` is `Nh` and must fit the schedule window (≤24h for day schedules; ≤ rate hours for hourly). When `Duration` is set, lab picks a random offset in `[0, Duration)` for `NextRotationDate` (fake-clock tests inject RNG). Empty `Duration` keeps the window start. `RotateImmediately=false` persists the schedule and returns the current version without rotating. In-process ticker rotates when `NextRotationDate` ≤ now and advances `NextRotationDate` / `LastRotatedDate`. `DescribeSecret` returns `RotationEnabled`, `RotationRules`, `NextRotationDate`, and `LastRotatedDate` when scheduled. |

Secret metadata and sealed values live in SQLite. ARNs include a random six-character hex suffix (AWS-shaped).

### Authz notes

Secrets Manager uses `authorizeDataplaneOR` with the shared resource dual-eval helper (same OR/AND rules as DynamoDB table policies) and the secret owner account from the secret ARN. Same-account access: allow if identity **or** secret resource policy Allows. Cross-account access: allow only when identity **and** secret resource policy both Allow. Empty resource policy denies cross-account callers. Explicit Deny in either wins. `CreateSecret` and `ListSecrets` use identity-only `EvaluateFull`. Org SCP/RCP filters apply before evaluation. When identity Allows, permissions boundary and session intersect.

Plaintext paths also call `EvaluateKMS` on the secret CMK: `kms:Encrypt` for `CreateSecret` / `PutSecretValue` / `RotateSecret`, and `kms:Decrypt` for `GetSecretValue` (identity plus key policy, matching S3/DynamoDB SSE-KMS). Seal/unseal and KMS authz bind EncryptionContext `SecretARN` (secret ARN) and `SecretVersionId` (lab version id string). A secretsmanager Allow alone is not enough when the CMK key policy or caller identity denies KMS. `PutResourcePolicy` requires every statement to name a `Principal` (missing Principal is rejected at put and would match-none at eval).

Pass a full secret ARN as `SecretId` for cross-account `GetSecretValue`. Cross-account reads also need the trusting account key policy (and caller identity) to Allow `kms:Decrypt`.

Lambda-backed rotate: PassRole uses service principal `secretsmanager.amazonaws.com` (not `lambda.amazonaws.com`) and sets trust `aws:SourceArn` to the secret ARN. The function execution role typically trusts both so CreateFunction and RotateSecret configure succeed (use a separate trust statement for `secretsmanager.amazonaws.com` when locking SourceArn to the secret). Lab `setSecret` / `testSecret` may no-op when the rotator returns success without updating an external system.

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

Schedule automatic rotation without rotating immediately:

```bash
aws secretsmanager rotate-secret \
  --secret-id "$SECRET" \
  --rotation-rules '{"AutomaticallyAfterDays": 7}' \
  --rotate-immediately false \
  --endpoint-url "$EP"

aws secretsmanager describe-secret \
  --secret-id "$SECRET" \
  --endpoint-url "$EP"
# RotationEnabled, RotationRules, NextRotationDate
```

Optional Lambda rotator (function role must trust `secretsmanager.amazonaws.com`; nested engine or invoke hook required for live Invoke):

```bash
aws secretsmanager rotate-secret \
  --secret-id "$SECRET" \
  --rotation-lambda-arn "$ROTATOR_ARN" \
  --endpoint-url "$EP"

aws secretsmanager describe-secret \
  --secret-id "$SECRET" \
  --endpoint-url "$EP"
# VersionIdsToStages: new VersionId has AWSCURRENT; prior has AWSPREVIOUS

aws secretsmanager get-secret-value \
  --secret-id "$SECRET" \
  --version-stage AWSCURRENT \
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

- Cron `W` (weekday nearest to DOM) and `LW` tokens

## Out of lab scope

- Random password generation APIs (out of lab scope)
- Replication, filtering on `ListSecrets`, full pagination parity (out of lab scope; rotate + stages cover lab)
- Service-linked grant approximation that skips caller KMS (out of lab scope; lab requires caller EvaluateKMS for authz fidelity)
- True AWS-owned `alias/aws/secretsmanager` key (out of lab scope; lab convenience alias is a per-account CMK approximation)
