# Firehose

**Status:** shipped (lab core)

Delivery stream CRUD and PutRecord / PutRecordBatch. Destinations: S3 bucket (writes objects) and Lambda ARN (persists the record and enqueues async Lambda Invoke). Identity authz. Optional PassRole when RoleARN is present with `firehose.amazonaws.com` trust.

## Implemented

| Area | Actions |
|------|---------|
| Control | `CreateDeliveryStream`, `DescribeDeliveryStream`, `ListDeliveryStreams`, `DeleteDeliveryStream` |
| Data | `PutRecord`, `PutRecordBatch` |

S3 destination accepts `S3DestinationConfiguration` or `ExtendedS3DestinationConfiguration` with BucketARN and optional Prefix. Lambda destination accepts `LambdaDestinationConfiguration` with `LambdaArn` or `FunctionArn` and enqueues an async invoke after the record is stored.

### Authz notes

Identity `EvaluateFull` on `firehose:*`. PassRole applies when a destination RoleARN is set.

## How to verify / CLI smoke

Shared Compose and env setup: [index.md](index.md#shared-verification).

```bash
aws s3api create-bucket --bucket fh-lab --endpoint-url "$EP"
aws firehose create-delivery-stream \
  --delivery-stream-name lab \
  --s3-destination-configuration BucketARN=arn:aws:s3:::fh-lab,RoleARN=arn:aws:iam::000000000001:role/fh \
  --endpoint-url "$EP"
echo hello | base64 | aws firehose put-record \
  --delivery-stream-name lab \
  --record Data=aGVsbG8= \
  --endpoint-url "$EP"
```

Create the IAM role with `firehose.amazonaws.com` trust before passing RoleARN, or omit RoleARN for local-only puts.

## Not yet / deferred

- Elasticsearch / OpenSearch / HTTP endpoint destinations
- Dynamic partitioning depth
- Synchronous RequestResponse Lambda delivery (async enqueue only)
