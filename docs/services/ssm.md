# SSM Parameter Store

**Status:** shipped

Lab-complete Parameter Store core: String and SecureString parameters, Put/Get/GetParameters/Delete/Describe, KMS encryption for SecureString via KeyId or lab `alias/aws/ssm`, and identity-only authz.

## Implemented

| Area | Actions |
|------|---------|
| Parameters | `PutParameter`, `GetParameter`, `GetParameters`, `DeleteParameter`, `DescribeParameters` |
| Types | `String` (plaintext at rest), `SecureString` (sealed under KMS) |
| KMS | Optional `KeyId` on Put. Defaults to lab `alias/aws/ssm` (per-account CMK seeded on first use) |
| Describe filters | `ParameterFilters` with `Key=Name`, `Option=BeginsWith` (optional), and `Values` prefix |

Parameter metadata lives in SQLite. SecureString values are sealed under the resolved CMK. Names normalize with a leading `/` when omitted.

### Authz notes

SSM uses identity `EvaluateFull` on parameter ARNs (or `*` for `DescribeParameters`). There is no parameter resource policy path. Org SCP/RCP filters apply. Permissions boundary and session intersect on the identity path.

Cross-account parameter access beyond same-account lab paths is deferred.

## How to verify / CLI smoke

Shared Compose and env setup: [index.md](index.md#shared-verification).

```bash
PARAM="/noctaxris-lab-$RANDOM"
aws ssm put-parameter \
  --name "$PARAM" \
  --value hello-ssm \
  --type String \
  --endpoint-url "$EP"

aws ssm get-parameter \
  --name "$PARAM" \
  --endpoint-url "$EP"
```

SecureString with default KMS alias:

```bash
SECURE="/noctaxris-secure-$RANDOM"
aws ssm put-parameter \
  --name "$SECURE" \
  --value secret-value \
  --type SecureString \
  --endpoint-url "$EP"

aws ssm get-parameter \
  --name "$SECURE" \
  --with-decryption \
  --endpoint-url "$EP"
```

Batch get, describe, delete:

```bash
aws ssm get-parameters \
  --names "$PARAM" "$SECURE" \
  --with-decryption \
  --endpoint-url "$EP"

aws ssm describe-parameters \
  --parameter-filters "Key=Name,Option=BeginsWith,Values=/noctaxris" \
  --endpoint-url "$EP"

aws ssm delete-parameter --name "$PARAM" --endpoint-url "$EP"
aws ssm delete-parameter --name "$SECURE" --endpoint-url "$EP"
```

## Not yet / deferred

- Full SSM SAR beyond the lab set (StringList types, `GetParametersByPath`, parameter policies, labels, tags, hierarchy, documents, sessions, automation, associations, OpsCenter, full pagination parity)
- Cross-account parameter access beyond same-account lab paths
- True AWS-owned `alias/aws/ssm` key (lab convenience alias is a per-account CMK approximation)
