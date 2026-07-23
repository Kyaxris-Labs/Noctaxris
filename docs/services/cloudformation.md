# CloudFormation

**Status:** shipped (lab core)

Create, describe, list, update, and delete stacks from a JSON or YAML template subset. ChangeSet lite (Add/Remove execute; Modify fail-closed). Nested stacks via `AWS::CloudFormation::Stack` with lab S3 `TemplateURL` only. Drift lite for allowlisted types. Identity authz. Optional PassRole for `RoleARN` with `cloudformation.amazonaws.com` trust.

## Implemented

| Area | Actions |
|------|---------|
| Stacks | `CreateStack`, `DescribeStacks`, `DeleteStack`, `ListStacks`, `UpdateStack` (implicit ChangeSet) |
| ChangeSets | `CreateChangeSet`, `DescribeChangeSet`, `ExecuteChangeSet` (Add/Remove; Modify rejected) |
| Drift | `DetectStackDrift`, `DescribeStackDriftDetectionStatus`, `DescribeStackResourceDrifts` (`IN_SYNC` / `MODIFIED` / `NOT_CHECKED`; nested `NOT_CHECKED`) |
| Resources | `AWS::S3::Bucket`, `AWS::S3::BucketPolicy`, `AWS::IAM::Role`, `AWS::IAM::ManagedPolicy`, `AWS::IAM::Policy`, `AWS::SQS::Queue`, `AWS::SQS::QueuePolicy`, `AWS::DynamoDB::Table`, `AWS::Lambda::Function`, `AWS::Lambda::Permission`, `AWS::KMS::Key`, `AWS::SNS::Topic`, `AWS::Events::EventBus`, `AWS::Events::Rule`, `AWS::SSM::Parameter`, `AWS::SecretsManager::Secret`, `AWS::CloudFormation::Stack` |
| Template forms | JSON and YAML `TemplateBody` |
| Intrinsics (lab) | `Ref`, `Fn::GetAtt`, `Fn::Sub`, `Fn::Join` (YAML short forms `!Ref`, `!GetAtt`, `!Sub`, `!Join`) |
| Ordering | Ref/GetAtt/Sub plus explicit `DependsOn` |

Unknown resource types and unknown property keys fail closed. Lambda lab resources require `Code.ZipFile`. Nested `TemplateURL` allows `s3://bucket/key` and path-style `http://127.0.0.1:4566/...` / `localhost:4566` only.

Policy and permission notes:

| Type | Lab behavior |
|------|----------------|
| `AWS::IAM::ManagedPolicy` | Creates a customer-managed policy; optional `Roles` / `Users` / `Groups` attach. `Path` other than `/` is rejected. `Description` is accepted and ignored. |
| `AWS::IAM::Policy` | Lab maps this to a customer-managed policy (not AWS inline embedding) so the same attach APIs apply; requires at least one of `Roles`, `Users`, or `Groups`. |
| `AWS::S3::BucketPolicy` | Calls `PutBucketPolicy`; physical id `{bucket}#BucketPolicy`. |
| `AWS::Lambda::Permission` | Calls `AddPermission` with `StatementId` (defaults to logical id). Wildcard `Principal` and properties such as `PrincipalOrgID` / `FunctionUrlAuthType` are rejected. Service principals (for example `s3.amazonaws.com`) with `SourceArn` / `SourceAccount` stay available for PassRole-style notification wiring. |
| `AWS::S3::Bucket` `NotificationConfiguration` | Optional. Maps CFN `LambdaConfigurations` / `QueueConfigurations` / `TopicConfigurations` (`Event` + destination ARN + optional `Filter.S3Key.Rules`) and `EventBridgeConfiguration` onto `PutBucketNotificationConfiguration`. Destinations and resource policies must already exist (use `DependsOn`); circular CreateStack graphs fail closed like AWS. |

### Authz notes

Identity `EvaluateFull` on `cloudformation:*`. When `RoleARN` is set on CreateStack, PassRole plus `cloudformation.amazonaws.com` trust is required.

## How to verify / CLI smoke

Shared Compose and env setup: [index.md](index.md#shared-verification).

```bash
aws cloudformation create-stack \
  --stack-name lab \
  --template-body '{"Resources":{"B":{"Type":"AWS::S3::Bucket","Properties":{"BucketName":"cfn-lab-cli-1"}}}}' \
  --endpoint-url "$EP"

aws cloudformation describe-stacks --stack-name lab --endpoint-url "$EP"
aws cloudformation list-stacks --endpoint-url "$EP"
aws cloudformation delete-stack --stack-name lab --endpoint-url "$EP"
```

CreateStack round-trip suite: [tests/cloudformation/](../../tests/cloudformation/) (see [tests/README.md](../../tests/README.md)).

## Not yet / deferred

- Full intrinsic matrix (`Fn::If`, `Fn::Select`, mappings, conditions, transforms)
- ChangeSet Modify / in-place property updates
- Nested stack drift comparison beyond `NOT_CHECKED`
- Broader resource type catalog beyond the lab set above
- `AWS::IAM::ManagedPolicy` custom `Path` values other than `/`
- `AWS::Lambda::Permission` function-URL and org-id properties (`FunctionUrlAuthType`, `PrincipalOrgID`, `InvokedViaFunctionUrl`, `EventSourceToken`)