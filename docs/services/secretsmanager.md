# Secrets Manager

**Status:** shipped

Lab-complete Secrets Manager core: create, read, update, delete, describe, and list secrets. Resource policies on secrets, KMS encryption via KeyId or lab `alias/aws/secretsmanager`, and identity-or-resource-policy authz.

## Implemented

| Area | Actions |
|------|---------|
| Secrets | `CreateSecret`, `GetSecretValue`, `PutSecretValue`, `DeleteSecret`, `DescribeSecret`, `ListSecrets` |
| Resource policy | `PutResourcePolicy`, `GetResourcePolicy`, `DeleteResourcePolicy` |
| Payload | `SecretString` and/or `SecretBinary` (base64 on wire) |
| KMS | Optional `KmsKeyId` on create. Defaults to lab `alias/aws/secretsmanager` (per-account CMK seeded on first use) |

Secret metadata and sealed values live in SQLite. ARNs include a random six-character hex suffix (AWS-shaped). `DeleteSecret` removes the secret immediately (no recovery window).

### Authz notes

Secrets Manager uses `EvaluateDynamoDB` OR semantics via `authorizeDataplaneOR`: allow if identity **or** secret resource policy Allows. Explicit Deny in either wins. A resource policy alone can grant `GetSecretValue` (and other data-plane actions) without identity Allow. `CreateSecret` and `ListSecrets` use identity-only `EvaluateFull`. Org SCP/RCP filters apply before the union. When identity Allows, permissions boundary and session intersect.

Cross-account secret resource policy depth beyond same-account lab paths is deferred.

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

A principal with no identity Allow for `secretsmanager:GetSecretValue` can still read the secret when the secret resource policy Allows that action (same OR semantics as DynamoDB table policies). Handler tests cover this path.

```bash
aws secretsmanager delete-resource-policy \
  --secret-id "$SECRET" \
  --endpoint-url "$EP"

aws secretsmanager delete-secret \
  --secret-id "$SECRET" \
  --endpoint-url "$EP"
```

## Not yet / deferred

- Full Secrets Manager SAR beyond the lab set (`RotateSecret`, random password generation, version stages, tags, replication, filtering on `ListSecrets`, full pagination parity)
- Delete recovery window and scheduled deletion (lab deletes immediately)
- Cross-account secret resource policy depth beyond same-account lab paths
- True AWS-owned `alias/aws/secretsmanager` key (lab convenience alias is a per-account CMK approximation)
