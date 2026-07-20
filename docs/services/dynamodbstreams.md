# DynamoDB Streams

**Status:** shipped (lab core)

Table streams with `NEW_IMAGE` or `KEYS_ONLY`, a single lab shard, and GetRecords iterators. Change records persist on PutItem, UpdateItem, and DeleteItem when the stream is enabled. Identity authz only.

## Implemented

| Area | Actions |
|------|---------|
| Enable | `CreateTable` / `UpdateTable` `StreamSpecification` (`StreamEnabled`, `StreamViewType`) |
| Control | `ListStreams`, `DescribeStream` |
| Consume | `GetShardIterator` (`TRIM_HORIZON`, `LATEST`, `AT_SEQUENCE_NUMBER`, `AFTER_SEQUENCE_NUMBER`), `GetRecords` |

Stream ARN: `arn:aws:dynamodb:REGION:ACCOUNT:table/NAME/stream/LABEL`. Lab shard id: `shardId-000000000000`. API target prefix: `DynamoDBStreams_20120810`.

### Authz notes

Identity `EvaluateFull` on `dynamodbstreams:*` against the stream ARN (or `*` for ListStreams and GetRecords). Org SCP/RCP filters apply. No stream resource policy path.

## How to verify / CLI smoke

Shared Compose and env setup: [index.md](index.md#shared-verification).

```bash
TABLE="noctaxris-stream-$RANDOM"
aws dynamodb create-table \
  --table-name "$TABLE" \
  --attribute-definitions AttributeName=pk,AttributeType=S \
  --key-schema AttributeName=pk,KeyType=HASH \
  --billing-mode PAY_PER_REQUEST \
  --stream-specification StreamEnabled=true,StreamViewType=NEW_IMAGE \
  --endpoint-url "$EP"

STREAM_ARN=$(aws dynamodb describe-table --table-name "$TABLE" --endpoint-url "$EP" \
  --query Table.LatestStreamArn --output text)

aws dynamodb put-item \
  --table-name "$TABLE" \
  --item '{"pk":{"S":"1"},"v":{"S":"hello"}}' \
  --endpoint-url "$EP"

aws dynamodbstreams list-streams --table-name "$TABLE" --endpoint-url "$EP"
aws dynamodbstreams describe-stream --stream-arn "$STREAM_ARN" --endpoint-url "$EP"

IT=$(aws dynamodbstreams get-shard-iterator \
  --stream-arn "$STREAM_ARN" \
  --shard-id shardId-000000000000 \
  --shard-iterator-type TRIM_HORIZON \
  --endpoint-url "$EP" \
  --query ShardIterator --output text)

aws dynamodbstreams get-records --shard-iterator "$IT" --endpoint-url "$EP"
```

## Not yet / deferred

- Global tables and parallel shard fan-out
- Lambda event source mapping for DynamoDB Streams (SQS ESM only in this version)
- OLD_IMAGE / NEW_AND_OLD_IMAGE view types
