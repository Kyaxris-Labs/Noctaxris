# Cloud Control API

**Status:** shipped (lab core)

CreateResource, GetResource, ListResources, UpdateResource, DeleteResource, and GetResourceRequestStatus for a documented allowlist aligned with CloudFormation lab types (except `AWS::CloudFormation::Stack` and `AWS::SQS::QueuePolicy`). Unknown types fail closed. Identity authz. Create/Update/Delete return a sync `ProgressEvent` with `OperationStatus` SUCCESS; tokens are recorded for `GetResourceRequestStatus`.

## Implemented

| Area | Actions |
|------|---------|
| Resources | `CreateResource`, `GetResource`, `ListResources`, `UpdateResource`, `DeleteResource`, `GetResourceRequestStatus` |

### Authz notes

Identity `EvaluateFull` on `cloudcontrol:*`. Create provision uses the same underlying-action and PassRole hooks as CloudFormation (for example `iam:CreateRole`, `lambda:CreateFunction` + PassRole). UpdateResource gates privileged mutate paths with those hooks for IAM Role (including `iam:PutRolePolicy` / `iam:AttachRolePolicy` on policy replace), IAM User/Group policy and group membership, IAM ManagedPolicy document replace, Lambda UpdateFunctionConfiguration (+ PassRole when Role changes), and Events Rule `events:PutRule`. There is no Capabilities parameter; IAM types still require the caller (or effective principal) to hold the concrete IAM actions.

Remaining Update gaps (no per-action authz beyond `cloudcontrol:UpdateResource`; same as CFN Modify helpers that lack action checks for these types): `AWS::SSM::Parameter`, `AWS::S3::Bucket`, `AWS::SQS::Queue`, `AWS::SNS::Topic`, `AWS::SecretsManager::Secret`, `AWS::DynamoDB::Table`, `AWS::KMS::Key`, `AWS::KMS::Alias`, `AWS::Logs::LogGroup`, `AWS::Events::EventBus` policy patches.

### Allowlist

| TypeName | Identifier | Notes |
|----------|------------|-------|
| `AWS::S3::Bucket` | Bucket name | Update: `BucketEncryption`, `NotificationConfiguration` |
| `AWS::IAM::Role` | Role name | Update: `AssumeRolePolicyDocument`, `Policies`, `ManagedPolicyArns` |
| `AWS::SQS::Queue` | Queue URL | Update: `VisibilityTimeout`, `MessageRetentionPeriod`, delay/wait |
| `AWS::DynamoDB::Table` | Table name | Update: `SSESpecification` only |
| `AWS::Lambda::Function` | Function name | Create ZipFile; Update: `Timeout`, `MemorySize`, `Environment` (no `Role` patch) |
| `AWS::KMS::Key` | Key id | Update: `KeyPolicy`, `EnableKeyRotation` (`Description` accepted/ignored) |
| `AWS::KMS::Alias` | Alias name | Update: `TargetKeyId` |
| `AWS::SNS::Topic` | Topic ARN | Update: `DisplayName`, `KmsMasterKeyId` |
| `AWS::Events::EventBus` | Bus name | Update: `Policy` (`Tags` accepted/ignored) |
| `AWS::Events::Rule` | `bus\|rule` | Update: `EventPattern`, `State`, `Description` |
| `AWS::SSM::Parameter` | Parameter name | Update: `Value`, `Type`, `KeyId` |
| `AWS::SecretsManager::Secret` | Secret name | Update: `Description`, `KmsKeyId` |
| `AWS::Logs::LogGroup` | Log group name | Update: `RetentionInDays` (empty patch no-op) |
| `AWS::IAM::User` | User name | Update: `Policies`, `ManagedPolicyArns`, `Groups` |
| `AWS::IAM::Group` | Group name | Update: `Policies`, `ManagedPolicyArns` |
| `AWS::IAM::ManagedPolicy` | Policy ARN | Update: `PolicyDocument` (+ re-attach `Roles` / `Users` / `Groups`); `Description` accepted/ignored |

Lab `PatchDocument` is a JSON object of property keys (CreateResource DesiredState shape), not RFC6902. Unknown patch keys fail closed with `InvalidRequestException`.

## How to verify / CLI smoke

Shared Compose and env setup: [index.md](index.md#shared-verification).

```bash
aws cloudcontrol create-resource \
  --type-name AWS::S3::Bucket \
  --desired-state '{"BucketName":"lab-cc-bucket"}' \
  --endpoint-url "$EP"
aws cloudcontrol list-resources --type-name AWS::S3::Bucket --endpoint-url "$EP"
aws cloudcontrol delete-resource --type-name AWS::S3::Bucket --identifier lab-cc-bucket --endpoint-url "$EP"
```

## Out of lab scope

- Async ProgressEvent polling depth beyond recorded SUCCESS tokens (out of lab scope; sync SUCCESS tokens cover lab CC)
- RFC6902 JSON Patch `PatchDocument` parity (out of lab scope; lab uses property-object patches)
- Private registry types (out of lab scope)
