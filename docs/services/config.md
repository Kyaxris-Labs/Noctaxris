# Config

**Status:** shipped (lab core)

Configuration recorder and delivery channel lite, StartConfigurationRecorder, and DescribeComplianceByConfigRule. Identity authz. Optional PassRole for recorder roleARN with `config.amazonaws.com` trust. Delivery channel `s3BucketName` must already exist in lab S3. Start requires a delivery channel; it sets the recording flag only (no configuration history PutObject). When a delivery channel has `snsTopicARN`, Start Publishes `ConfigurationRecorderStarted` (best-effort), not a history-delivery claim.

## Implemented

| Area | Actions |
|------|---------|
| Recorder | `PutConfigurationRecorder`, `StartConfigurationRecorder` (requires delivery channel + existing bucket) |
| Delivery | `PutDeliveryChannel` (bucket must exist; optional `snsTopicARN`) |
| Compliance | `DescribeComplianceByConfigRule` returns `NOT_APPLICABLE` only for stored `config_rules` rows (empty list when the rule name is unknown). Rule evaluation is not implemented |
| Notify | StartConfigurationRecorder → SNS Publish `ConfigurationRecorderStarted` when delivery channel has `snsTopicARN` |

### Authz notes

Identity `EvaluateFull` on `config:*`. PassRole applies when ConfigurationRecorder.roleARN is set.

## How to verify / CLI smoke

Shared Compose and env setup: [index.md](index.md#shared-verification).

```bash
aws s3 mb s3://config-lab --endpoint-url "$EP"
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

Omit roleARN or create a role trusted by `config.amazonaws.com` before PassRole checks. Expect DescribeCompliance to return an empty list until a config rule row exists; stored rules return `NOT_APPLICABLE` (no invented COMPLIANT rows). StartConfigurationRecorder sets the recording flag only and does not write configuration history objects.

## Not yet / deferred

- Configuration history delivery to S3
- Full managed rule catalog, remediations, aggregator, organization rules
