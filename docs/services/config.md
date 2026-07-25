# Config

**Status:** shipped (lab core)

Configuration recorder and delivery channel lite, StartConfigurationRecorder, and DescribeComplianceByConfigRule. Identity authz. Optional PassRole for recorder roleARN with `config.amazonaws.com` trust. Delivery channel `s3BucketName` must already exist in lab S3. Start requires a delivery channel and writes one JSON snapshot per delivery channel before setting the recording flag. When a delivery channel has `snsTopicARN`, Start Publishes `ConfigurationRecorderStarted` (best-effort), not a continuous history stream claim.

## Implemented

| Area | Actions |
|------|---------|
| Recorder | `PutConfigurationRecorder`, `StartConfigurationRecorder` (requires delivery channel + existing bucket) |
| Delivery | `PutDeliveryChannel` (bucket must exist; optional `snsTopicARN`) |
| History | On successful Start, `PutObject` snapshot JSON to `{prefix}AWSLogs/{accountId}/Config/noctaxris-config-snapshot-{recorder}-{millis}.json` (lab-shaped path; not identical to every AWS partition detail) |
| Compliance | `DescribeComplianceByConfigRule` returns `NOT_APPLICABLE` only for stored `config_rules` rows (empty list when the rule name is unknown). Rule evaluation is not implemented |
| Notify | StartConfigurationRecorder → SNS Publish `ConfigurationRecorderStarted` when delivery channel has `snsTopicARN` |

Snapshot body (`application/json`) lists allowlisted resource names from sqlite today: S3 bucket names and SQS queue names for the account. It is not the full AWS Config configuration item schema.

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
aws s3 ls "s3://config-lab/AWSLogs/000000000001/Config/" --endpoint-url "$EP"
aws s3 cp "s3://config-lab/AWSLogs/000000000001/Config/noctaxris-config-snapshot-default-*.json" - --endpoint-url "$EP"
aws configservice describe-compliance-by-config-rule --endpoint-url "$EP"
```

Omit roleARN or create a role trusted by `config.amazonaws.com` before PassRole checks. Expect DescribeCompliance to return an empty list until a config rule row exists; stored rules return `NOT_APPLICABLE` (no invented COMPLIANT rows). If the snapshot `PutObject` fails, Start returns an error and the recorder stays not recording.

## Not yet / deferred

- Continuous configuration history stream, periodic snapshots, and full AWS Config item schema
- Full managed rule catalog, remediations, aggregator, organization rules
