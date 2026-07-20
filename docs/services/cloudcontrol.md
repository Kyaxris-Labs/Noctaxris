# Cloud Control API

**Status:** shipped (lab core)

CreateResource, GetResource, ListResources, and DeleteResource for a documented allowlist of already-emulated types (`AWS::S3::Bucket`, `AWS::IAM::Role`). Unknown types fail closed. Identity authz. Create/Delete return a sync `ProgressEvent` with `OperationStatus` SUCCESS (no async polling depth).

## Implemented

| Area | Actions |
|------|---------|
| Resources | `CreateResource`, `GetResource`, `ListResources`, `DeleteResource` |

### Authz notes

Identity `EvaluateFull` on `cloudcontrol:*`.

### Allowlist

| TypeName | Identifier |
|----------|------------|
| `AWS::S3::Bucket` | Bucket name |
| `AWS::IAM::Role` | Role name |

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

Live Compose smoke skipped when Docker is unavailable.

## Not yet / deferred

- Full CloudFormation type coverage
- Async GetResourceRequestStatus polling depth beyond simple SUCCESS
- Private registry types
