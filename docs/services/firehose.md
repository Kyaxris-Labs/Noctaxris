# Firehose

**Status:** shipped (lab core)

Delivery stream CRUD and PutRecord / PutRecordBatch. Destinations: S3 bucket (writes objects), Lambda ARN (persists the record and enqueues async Lambda Invoke), and OpenSearch domain (indexes documents to a nested Active domain). Identity authz. PassRole when RoleARN is present with `firehose.amazonaws.com` trust. Put delivery evaluates a RoleARN session (Scheduler-shaped) or requires a destination resource policy Allow for `firehose.amazonaws.com` (S3/Lambda). OpenSearch has no resource-policy surface yet, so Put requires RoleARN with `es:ESHttpPut` (fail closed).

## Implemented

| Area | Actions |
|------|---------|
| Control | `CreateDeliveryStream`, `DescribeDeliveryStream`, `ListDeliveryStreams`, `DeleteDeliveryStream` |
| Data | `PutRecord`, `PutRecordBatch` |

| Destination | Configuration | Put behavior |
|-------------|---------------|--------------|
| S3 | `S3DestinationConfiguration` or `ExtendedS3DestinationConfiguration` (BucketARN, optional Prefix) | Writes object under prefix |
| Lambda | `LambdaDestinationConfiguration` (`LambdaArn` or `FunctionArn`) | Stores record and enqueues async Invoke |
| OpenSearch | `AmazonopensearchserviceDestinationConfiguration` or lab `OpenSearchDestinationConfiguration` (DomainARN/DomainName, IndexName, RoleARN) | POST `/{index}/_doc` on nested allowlisted host only |

Create OpenSearch destination only when the domain exists, is `Active`, and has a non-`stub://` nested endpoint (`noctaxris-opensearch-*` / `noctaxris-data-opensearch-*` on port 9200).

`DescribeDeliveryStream` for an OpenSearch destination returns both `AmazonopensearchserviceDestinationDescription` (AWS shape) and lab `OpenSearchDestinationDescription` with `DomainARN`, `IndexName`, and `RoleARN`.

### Authz notes

Identity `EvaluateFull` on `firehose:*`. PassRole applies when a destination RoleARN is set on create. On Put, RoleARN mints a role session that must Allow `s3:PutObject`, `lambda:InvokeFunction`, or `es:ESHttpPut`. Without RoleARN, S3/Lambda destination policies must Allow `firehose.amazonaws.com`. OpenSearch Put without RoleARN is denied until a domain resource policy exists.

## How to verify / CLI smoke

Shared Compose and env setup: [index.md](index.md#shared-verification).

```bash
aws s3api create-bucket --bucket fh-lab --endpoint-url "$EP"
# Without RoleARN, set a bucket policy Allow for firehose.amazonaws.com on s3:PutObject,
# or create a role with firehose.amazonaws.com trust and identity Allow, then pass RoleARN.
aws firehose create-delivery-stream \
  --delivery-stream-name lab \
  --s3-destination-configuration BucketARN=arn:aws:s3:::fh-lab,RoleARN=arn:aws:iam::000000000001:role/fh \
  --endpoint-url "$EP"
echo hello | base64 | aws firehose put-record \
  --delivery-stream-name lab \
  --record Data=aGVsbG8= \
  --endpoint-url "$EP"
```

OpenSearch destination smoke (skip without DinD / Active nested OpenSearch engine — Create fails closed when the domain is missing, not `Active`, or still on `stub://`):

```bash
# Domain must already be Active with nested endpoint (see opensearch.md).
# Role must Allow es:ESHttpPut; PassRole + firehose.amazonaws.com trust required on RoleARN.
aws firehose create-delivery-stream \
  --delivery-stream-name lab-os \
  --amazonopensearchservice-destination-configuration \
    DomainARN=arn:aws:es:us-east-1:000000000001:domain/lab-os,IndexName=events,RoleARN=arn:aws:iam::000000000001:role/fh-os \
  --endpoint-url "$EP"
aws firehose describe-delivery-stream --delivery-stream-name lab-os --endpoint-url "$EP"
# Expect AmazonopensearchserviceDestinationDescription (and lab OpenSearchDestinationDescription)
# with DomainARN, IndexName, and RoleARN.
# PutRecord indexes via POST /{index}/_doc on the allowlisted nested host only.
```

## Not yet / deferred

- HTTP endpoint destinations
- Dynamic partitioning depth
- Synchronous RequestResponse Lambda delivery (async enqueue only)
- OpenSearch domain resource policy (RoleARN required until then)
