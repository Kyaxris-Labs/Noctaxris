# SSM Parameter Store and Run Command

**Status:** shipped

Lab-complete Parameter Store core: String, StringList, and SecureString parameters, Put/Get/GetParameters/Delete/Describe, version history with labels (`LabelParameterVersion` / `GetParameterHistory`), KMS encryption for SecureString via KeyId or lab `alias/aws/ssm`, and identity-only authz.

Run Command lite: `SendCommand` / `GetCommandInvocation` / `ListCommandInvocations` for document `AWS-RunShellScript` against lab EC2 instances backed by nested DinD containers (`docker exec` via the compute client; no host `docker.sock`).

## Implemented

| Area | Actions |
|------|---------|
| Parameters | `PutParameter`, `GetParameter`, `GetParameters`, `GetParametersByPath`, `DeleteParameter`, `DescribeParameters` |
| Version labels | `LabelParameterVersion`, `GetParameterHistory`; Get by `Name:version` / `Name:label` or `Version` / `Label` fields |
| Tags | `ListTagsForResource`, `AddTagsToResource`, `RemoveTagsFromResource` (Parameter resources) |
| Types | `String` / `StringList` (plaintext at rest; StringList Value is comma-separated), `SecureString` (sealed under KMS) |
| Hierarchy | `GetParametersByPath` with `Path`, optional `Recursive`, and `WithDecryption` |
| KMS | Optional `KeyId` on Put. Defaults to lab `alias/aws/ssm` (per-account CMK seeded on first use) |
| Describe filters | `ParameterFilters` with `Key=Name`, `Option=Equals` (exact name) or `BeginsWith` (optional; prefix), and `Values` |
| Run Command lite | `SendCommand`, `GetCommandInvocation`, `ListCommandInvocations` |

Parameter metadata lives in SQLite. Each `PutParameter` (including overwrite) appends a version row. SecureString values are sealed under the resolved CMK. Names normalize with a leading `/` when omitted.

### Version labels

| Item | Behavior |
|------|----------|
| Attach | `LabelParameterVersion` with `Name`, `Labels`, optional `ParameterVersion` (defaults to latest) |
| Cap | Max 10 labels per version (`ParameterVersionLabelLimitExceeded`) |
| Move | Re-attaching an existing label moves it to the target version (AWS-like lite) |
| Invalid | Labels that fail AWS rules (leading digit, `aws`/`ssm` prefix, bad chars) return in `InvalidLabels` without failing the call |
| Resolve | `GetParameter` / `GetParameters` accept `name:version`, `name:label`, or request fields `Version` / `Label`; response includes `Selector` when used |
| History | `GetParameterHistory` lists versions oldest-first with `Labels` (and values when `WithDecryption` allows) |

### Run Command lite

| Item | Behavior |
|------|----------|
| Document | `AWS-RunShellScript` only |
| Parameters | `Parameters.commands` string list (joined with newlines, run as `sh -c`) |
| Targets | `InstanceIds` must exist in the lab EC2 store |
| Executor | Resolve instance `ContainerID`; nested DinD `Exec`; capture stdout/stderr/exit; honor `TimeoutSeconds` (minimum 30, default 3600) |
| Status | Invocation `Pending` → `InProgress` → `Success` / `Failed` / `TimedOut`; parent command counts refresh when invocations finish |
| Authz | Identity `EvaluateFull` on `ssm:SendCommand`, `ssm:GetCommandInvocation`, and `ssm:ListCommandInvocations` (resource `*`) |

Instances without a running nested container fail the invocation (no SSM agent queue).

### Authz notes

SSM uses identity `EvaluateFull` on parameter ARNs (or `*` for `DescribeParameters` and Run Command). There is no parameter resource policy path. Org SCP/RCP filters apply. Permissions boundary and session intersect on the identity path.

SecureString paths also call `EvaluateKMS` on the parameter CMK: `kms:Encrypt` on `PutParameter`, and `kms:Decrypt` on decrypted `GetParameter` / `GetParameters` / `GetParametersByPath` (`WithDecryption=true`). Seal/unseal and KMS authz bind EncryptionContext `PARAMETER_ARN` (parameter ARN). String parameters stay identity-only. An `ssm:GetParameter` Allow alone cannot decrypt a SecureString when KMS denies.

Cross-account parameter access beyond same-account lab paths is out of lab scope.

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

StringList (comma-separated Value):

```bash
LIST="/noctaxris-list-$RANDOM"
aws ssm put-parameter \
  --name "$LIST" \
  --value 'Monday,Wednesday,Friday' \
  --type StringList \
  --endpoint-url "$EP"

aws ssm get-parameter \
  --name "$LIST" \
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

aws ssm get-parameters-by-path \
  --path "/noctaxris" \
  --recursive \
  --with-decryption \
  --endpoint-url "$EP"

aws ssm describe-parameters \
  --parameter-filters "Key=Name,Option=BeginsWith,Values=/noctaxris" \
  --endpoint-url "$EP"

aws ssm describe-parameters \
  --parameter-filters "Key=Name,Option=Equals,Values=$PARAM" \
  --endpoint-url "$EP"

aws ssm delete-parameter --name "$PARAM" --endpoint-url "$EP"
aws ssm delete-parameter --name "$SECURE" --endpoint-url "$EP"
```

Version labels and history:

```bash
LABEL="/noctaxris-label-$RANDOM"
aws ssm put-parameter \
  --name "$LABEL" \
  --value ami-v1 \
  --type String \
  --endpoint-url "$EP"

aws ssm put-parameter \
  --name "$LABEL" \
  --value ami-v2 \
  --type String \
  --overwrite \
  --endpoint-url "$EP"

aws ssm label-parameter-version \
  --name "$LABEL" \
  --parameter-version 1 \
  --labels Test \
  --endpoint-url "$EP"

aws ssm label-parameter-version \
  --name "$LABEL" \
  --labels Production \
  --endpoint-url "$EP"

aws ssm get-parameter \
  --name "$LABEL:Production" \
  --endpoint-url "$EP"

aws ssm get-parameter-history \
  --name "$LABEL" \
  --endpoint-url "$EP"

aws ssm delete-parameter --name "$LABEL" --endpoint-url "$EP"
```

Run Command lite (requires a running lab EC2 instance with a nested container):

```bash
IID=$(aws ec2 describe-instances --endpoint-url "$EP" \
  --query 'Reservations[0].Instances[0].InstanceId' --output text)

CMD=$(aws ssm send-command \
  --document-name "AWS-RunShellScript" \
  --instance-ids "$IID" \
  --parameters 'commands=["echo hello-ssm"]' \
  --timeout-seconds 60 \
  --endpoint-url "$EP" \
  --query 'Command.CommandId' --output text)

aws ssm get-command-invocation \
  --command-id "$CMD" \
  --instance-id "$IID" \
  --endpoint-url "$EP"
```

## Deferred depth

- Additional SSM documents beyond `AWS-RunShellScript` (for example PowerShell)
- SSM agent message queue / ec2messages path for instances without nested containers
- `ListCommands`, `CancelCommand`, Output S3, CloudWatch Logs delivery
- WorkingDirectory and other RunShellScript parameters beyond `commands`

## Out of lab scope

- Full SSM SAR beyond the lab set (parameter policies, UnlabelParameterVersion, tags beyond Parameter resources, documents catalog, sessions, automation, associations, OpsCenter, full pagination parity)
- Cross-account parameter access beyond same-account lab paths (out of lab scope)
- True AWS-owned `alias/aws/ssm` key (out of lab scope; lab convenience alias is a per-account CMK approximation)
