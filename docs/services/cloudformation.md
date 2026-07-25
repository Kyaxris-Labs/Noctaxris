# CloudFormation

**Status:** shipped (lab core)

Create, describe, list, update, and delete stacks from a JSON or YAML template subset. ChangeSet execute supports Add, Remove, and allowlisted in-place Modify (Removals, then Modify in dependency order, then Add). Unknown Modify types or immutable property changes fail closed. Nested stacks via `AWS::CloudFormation::Stack` with lab S3 `TemplateURL` only. Drift lite for allowlisted types. Identity authz. Optional PassRole for `RoleARN` with `cloudformation.amazonaws.com` trust.

## Implemented

| Area | Actions |
|------|---------|
| Stacks | `CreateStack`, `DescribeStacks`, `DeleteStack`, `ListStacks`, `UpdateStack` (implicit ChangeSet) |
| ChangeSets | `CreateChangeSet`, `DescribeChangeSet`, `ExecuteChangeSet` (Add/Remove/Modify for allowlisted in-place updates) |
| Drift | `DetectStackDrift`, `DescribeStackDriftDetectionStatus`, `DescribeStackResourceDrifts` (`IN_SYNC` / `MODIFIED` / `NOT_CHECKED`; nested `NOT_CHECKED`) |
| Resources | `AWS::S3::Bucket`, `AWS::S3::BucketPolicy`, `AWS::IAM::Role`, `AWS::IAM::User`, `AWS::IAM::Group`, `AWS::IAM::ManagedPolicy`, `AWS::IAM::Policy`, `AWS::SQS::Queue`, `AWS::SQS::QueuePolicy`, `AWS::DynamoDB::Table`, `AWS::Lambda::Function`, `AWS::Lambda::Permission`, `AWS::KMS::Key`, `AWS::KMS::Alias`, `AWS::SNS::Topic`, `AWS::SNS::TopicPolicy`, `AWS::SNS::Subscription`, `AWS::Logs::LogGroup`, `AWS::Events::EventBus`, `AWS::Events::Rule`, `AWS::SSM::Parameter`, `AWS::SecretsManager::Secret`, `AWS::CloudFormation::Stack` |
| Template forms | JSON and YAML `TemplateBody` |
| Intrinsics (lab) | `Ref`, `Fn::GetAtt`, `Fn::Sub`, `Fn::Join` (YAML short forms `!Ref`, `!GetAtt`, `!Sub`, `!Join`) |
| Ordering | Ref/GetAtt/Sub plus explicit `DependsOn` |

Unknown resource types and unknown property keys fail closed. Lambda lab resources require `Code.ZipFile`. Nested `TemplateURL` allows `s3://bucket/key` and path-style `http://127.0.0.1:4566/...` / `localhost:4566` only.

Policy and permission notes:

| Type | Lab behavior |
|------|----------------|
| `AWS::IAM::ManagedPolicy` | Creates a customer-managed policy; optional `Roles` / `Users` / `Groups` attach. `Path` other than `/` is rejected. `Description` is accepted and ignored. |
| `AWS::IAM::Policy` | Lab maps this to a customer-managed policy (not AWS inline embedding) so the same attach APIs apply; requires at least one of `Roles`, `Users`, or `Groups`. |
| `AWS::IAM::User` / `AWS::IAM::Group` | Create user or group; optional inline `Policies`, `ManagedPolicyArns`, and (user) `Groups` membership. `Path` other than `/` is rejected. Login profiles, permissions boundaries, and tags beyond accept-and-ignore are unsupported. |
| `AWS::S3::BucketPolicy` | Calls `PutBucketPolicy`; physical id `{bucket}#BucketPolicy`. |
| `AWS::SQS::Queue` | Queue attribute props (`DelaySeconds`, FIFO flags, etc.) are passed to `CreateQueue`. |
| `AWS::SQS::QueuePolicy` | Sets queue `Policy` attribute; physical id `{queueUrl}#QueuePolicy`. |
| `AWS::SNS::TopicPolicy` | Sets topic `Policy` via `SetTopicAttributes` for each entry in `Topics`; physical id `{firstTopicArn}#TopicPolicy`. |
| `AWS::SNS::Subscription` | `TopicArn` + `Protocol` + `Endpoint` (`sqs` / `lambda` auto-confirm; HTTP uses lab allowlist). Optional `FilterPolicy` / `FilterPolicyScope` (lab exact-match / string-list subset), `RawMessageDelivery`, plus persisted `DeliveryPolicy` / `RedrivePolicy` attributes. Lab DLQ fan-out follows SNS subscription RedrivePolicy after retry exhaustion (exact AWS retry timing not claimed). |
| `AWS::Logs::LogGroup` | `CreateLogGroup` with optional `RetentionInDays` (AWS-allowed day values via Logs `PutRetentionPolicy`). KMS and data-protection properties are rejected. |
| `AWS::KMS::Alias` | `AliasName` + `TargetKeyId` (key id or ARN); resolves via `ResolveKeyID`. |
| `AWS::Lambda::Permission` | Calls `AddPermission` with `StatementId` (defaults to logical id). Optional `FunctionUrlAuthType` (`NONE` / `AWS_IAM`) and `InvokedViaFunctionUrl` add `lambda:FunctionUrlAuthType` / `lambda:InvokedViaFunctionUrl` conditions; wildcard `Principal` requires `FunctionUrlAuthType`. `PrincipalOrgID` and `EventSourceToken` are rejected (fail closed). Service principals (for example `s3.amazonaws.com`) with `SourceArn` / `SourceAccount` stay available for notification wiring. |
| `AWS::S3::Bucket` `NotificationConfiguration` | Optional. Maps CFN `LambdaConfigurations` / `QueueConfigurations` / `TopicConfigurations` (`Event` + destination ARN + optional `Filter.S3Key.Rules`) and `EventBridgeConfiguration` onto `PutBucketNotificationConfiguration`. Destinations and resource policies must already exist (use `DependsOn`); circular CreateStack graphs fail closed like AWS. |
| `AWS::Events::Rule` | Event pattern rules with optional `Targets`. `ScheduleExpression` is rejected (use the Scheduler service). |

ChangeSet Modify in-place subsets (fail closed otherwise):

| Type | Mutable on Modify |
|------|-------------------|
| `AWS::SSM::Parameter` | `Value`, `Type`, `KeyId` (`Name` immutable) |
| `AWS::S3::Bucket` | `BucketEncryption`, `NotificationConfiguration` (`BucketName` immutable) |
| `AWS::S3::BucketPolicy` / `AWS::SQS::QueuePolicy` / `AWS::SNS::TopicPolicy` | Replace `PolicyDocument` |
| `AWS::IAM::Role` | Trust + replace inline `Policies` / `ManagedPolicyArns` |
| `AWS::IAM::ManagedPolicy` / `AWS::IAM::Policy` | Replace `PolicyDocument` (+ re-attach targets) |
| `AWS::SQS::Queue` | Queue attributes (`VisibilityTimeout`, retention, delay, …); name/FIFO immutable |
| `AWS::SNS::Topic` | `DisplayName`, `KmsMasterKeyId` |
| `AWS::Lambda::Function` | Config + ZipFile `Code`; `FunctionName` immutable |
| `AWS::Lambda::Permission` | Replace Sid permission |
| `AWS::Events::Rule` | Pattern/state/targets (`Name` / bus immutable) |
| `AWS::SecretsManager::Secret` | `Description`, `KmsKeyId` only (`SecretString` via Secrets APIs) |
| `AWS::DynamoDB::Table` | `SSESpecification` only (key schema / table name fail closed) |
| `AWS::KMS::Alias` | `TargetKeyId` |
| `AWS::KMS::Key` | `KeyPolicy`, `EnableKeyRotation` (`Description` accepted/ignored) |
| `AWS::Logs::LogGroup` | `RetentionInDays` (clear when omitted/zero); name immutable |
| `AWS::Events::EventBus` | `Policy`; name immutable; `Tags` ignored |

### Authz notes

Identity `EvaluateFull` on `cloudformation:*`. When `RoleARN` is set on CreateStack, PassRole plus `cloudformation.amazonaws.com` trust is required for the caller; resource provision then evaluates as that role.

Provision fails closed on underlying actions (for example `iam:CreateRole`, `lambda:CreateFunction`, `events:PutTargets`). Lambda `Role` and EventBridge target `RoleArn` require the same PassRole checks as the direct APIs. ChangeSet/UpdateStack Modify of `AWS::IAM::Role` requires `iam:PutRolePolicy` / `iam:AttachRolePolicy` (and delete/detach counterparts) when replacing inline or managed policies; `iam:UpdateAssumeRolePolicy` is required only when the trust document changes.

IAM resource types require `Capabilities` containing `CAPABILITY_IAM` or `CAPABILITY_NAMED_IAM`. Custom IAM names require `CAPABILITY_NAMED_IAM` (AWS-shaped `InsufficientCapabilitiesException`).

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

## Out of lab scope

- Full intrinsic matrix (`Fn::If`, `Fn::Select`, mappings, conditions, transforms) (out of lab scope; lab-fullstack types + ChangeSet Modify + nested create cover green suites)
- Nested stack drift comparison beyond `NOT_CHECKED` (out of lab scope)
- ChangeSet Modify for nested `AWS::CloudFormation::Stack` and other non-allowlisted type/property sets (out of lab scope; unknown Modify fails closed)
- Broader resource type catalog beyond the lab set above (out of lab scope)
- `AWS::IAM::ManagedPolicy` / `AWS::IAM::User` / `AWS::IAM::Group` custom `Path` values other than `/` (out of lab scope)
- `AWS::Logs::LogGroup` KMS key and data-protection properties (out of lab scope)
- `AWS::Lambda::Permission` `PrincipalOrgID` / `EventSourceToken` (out of lab scope; fail closed)
- `AWS::SNS::Subscription` exact AWS delivery retry / jitter timing (out of lab scope; FilterPolicy + RawMessageDelivery + lab RedrivePolicy DLQ after retry exhaustion are shipped)
- `AWS::Events::Rule` `ScheduleExpression` (out of lab scope; use Scheduler; CFN fails closed)
