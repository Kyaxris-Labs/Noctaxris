# CloudFormation

**Status:** shipped (lab core)

Create, describe, list, and delete stacks from a JSON template subset. Identity authz. Optional PassRole for `RoleARN` with `cloudformation.amazonaws.com` trust.

## Implemented

| Area | Actions |
|------|---------|
| Stacks | `CreateStack`, `DescribeStacks`, `DeleteStack`, `ListStacks` |
| Resources | `AWS::S3::Bucket`, `AWS::IAM::Role` |

Unknown resource types fail closed with a clear validation error. Templates must be JSON (YAML not accepted).

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

## Not yet / deferred

- YAML templates and intrinsic function matrix
- ChangeSets, drift detection, nested stacks
- Broader resource type catalog
