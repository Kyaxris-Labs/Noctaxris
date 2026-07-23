# CloudFormation

**Status:** shipped (lab core)

Create, describe, list, and delete stacks from a JSON or YAML template subset. Identity authz. Optional PassRole for `RoleARN` with `cloudformation.amazonaws.com` trust.

## Implemented

| Area | Actions |
|------|---------|
| Stacks | `CreateStack`, `DescribeStacks`, `DeleteStack`, `ListStacks` |
| Resources | `AWS::S3::Bucket`, `AWS::IAM::Role`, `AWS::SQS::Queue`, `AWS::DynamoDB::Table`, `AWS::Lambda::Function` |
| Template forms | JSON and YAML `TemplateBody` |
| Intrinsics (lab) | `Ref`, `Fn::GetAtt`, `Fn::Sub`, `Fn::Join` (YAML short forms `!Ref`, `!GetAtt`, `!Sub`, `!Join`) |

Unknown resource types fail closed with a clear validation error. Lambda lab resources require `Code.ZipFile` (inline source zipped at create). Resource create order follows Ref/GetAtt/Sub dependencies.

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

- ChangeSets, drift detection, nested stacks
- Full intrinsic matrix (`Fn::If`, `Fn::Select`, mappings, conditions, transforms)
- Broader resource type catalog beyond the lab set above
