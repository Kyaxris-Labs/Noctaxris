# Cloud Control API

**Status:** shipped (lab core)

CreateResource, GetResource, ListResources, UpdateResource, DeleteResource, and GetResourceRequestStatus for a documented allowlist aligned with CloudFormation lab types (except `AWS::CloudFormation::Stack` and `AWS::SQS::QueuePolicy`). Unknown types fail closed. Identity authz. Create/Update/Delete return a sync `ProgressEvent` with `OperationStatus` SUCCESS; tokens are recorded for `GetResourceRequestStatus`.

## Implemented

| Area | Actions |
|------|---------|
| Resources | `CreateResource`, `GetResource`, `ListResources`, `UpdateResource`, `DeleteResource`, `GetResourceRequestStatus` |

### Authz notes

Identity `EvaluateFull` on `cloudcontrol:*`.

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
| `AWS::Events::EventBus` | Bus name | Update rejected (no mutable lab props) |
| `AWS::Events::Rule` | `bus\|rule` | Update: `EventPattern`, `State`, `Description` |
| `AWS::SSM::Parameter` | Parameter name | Update: `Value`, `Type`, `KeyId` |
| `AWS::SecretsManager::Secret` | Secret name | Update: `Description`, `KmsKeyId` |
| `AWS::Logs::LogGroup` | Log group name | Update: empty patch only (no-op) |
| `AWS::IAM::User` / `AWS::IAM::Group` / `AWS::IAM::ManagedPolicy` | — | Create/Get/List/Delete; Update rejected |

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

## Not yet / deferred

- Async ProgressEvent polling depth beyond recorded SUCCESS tokens
- RFC6902 JSON Patch `PatchDocument` parity (lab uses property-object patches)
- UpdateResource for IAM User/Group/ManagedPolicy and EventBus mutable depth
- Private registry types