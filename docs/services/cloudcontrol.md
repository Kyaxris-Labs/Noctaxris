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
| `AWS::S3::Bucket` | Bucket name | Update: `BucketEncryption` |
| `AWS::IAM::Role` | Role name | |
| `AWS::SQS::Queue` | Queue URL | |
| `AWS::DynamoDB::Table` | Table name | |
| `AWS::Lambda::Function` | Function name | ZipFile create |
| `AWS::KMS::Key` | Key id | |
| `AWS::SNS::Topic` | Topic ARN | |
| `AWS::Events::EventBus` | Bus name | |
| `AWS::Events::Rule` | `bus\|rule` | Update: pattern/state |
| `AWS::SSM::Parameter` | Parameter name | Update: Value/Type |
| `AWS::SecretsManager::Secret` | Secret name | |

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
- UpdateResource for every allowlisted type
- Private registry types