# Config

**Status:** shipped (lab core)

Configuration recorder and delivery channel lite, StartConfigurationRecorder, and DescribeComplianceByConfigRule stub over Tagging API resources. Identity authz. Optional PassRole for recorder roleARN with `config.amazonaws.com` trust. When a delivery channel has `snsTopicARN`, StartConfigurationRecorder Publishes a lab `ConfigurationHistoryDeliveryStarted` notification (best-effort).

## Implemented

| Area | Actions |
|------|---------|
| Recorder | `PutConfigurationRecorder`, `StartConfigurationRecorder` |
| Delivery | `PutDeliveryChannel` (optional `snsTopicARN`) |
| Compliance | `DescribeComplianceByConfigRule` |
| Notify | StartConfigurationRecorder → SNS Publish when delivery channel has `snsTopicARN` |

Compliance treats tagged resources as COMPLIANT. When no tags exist, a NOT_APPLICABLE stub row is returned.

### Authz notes

Identity `EvaluateFull` on `config:*`. PassRole applies when ConfigurationRecorder.roleARN is set.

## How to verify / CLI smoke

Shared Compose and env setup: [index.md](index.md#shared-verification).

```bash
aws configservice put-configuration-recorder \
  --configuration-recorder name=default,roleARN=arn:aws:iam::000000000001:role/config \
  --endpoint-url "$EP"
aws configservice put-delivery-channel \
  --delivery-channel name=default,s3BucketName=config-lab \
  --endpoint-url "$EP"
aws configservice start-configuration-recorder \
  --configuration-recorder-name default \
  --endpoint-url "$EP"
aws configservice describe-compliance-by-config-rule --endpoint-url "$EP"
```

Omit roleARN or create a role trusted by `config.amazonaws.com` before PassRole checks.

## Not yet / deferred

- Full managed rule catalog, remediations, aggregator, organization rules
