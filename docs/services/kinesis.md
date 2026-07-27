# Kinesis Data Streams

**Status:** shipped (lab core)

Lab streams with up to four shards, Put/Get records, shard iterators, enhanced fan-out consumers (lab long-poll subscribe), `UpdateShardCount` within 1..4, and Lambda event source mapping. Persisted in SQLite. Identity authz only.

## Implemented

| Area | Actions |
|------|---------|
| Stream CRUD | `CreateStream`, `DeleteStream`, `DescribeStream`, `ListStreams` |
| Produce | `PutRecord`, `PutRecords` |
| Consume | `GetShardIterator` (`TRIM_HORIZON`, `LATEST`, `AT_SEQUENCE_NUMBER`, `AFTER_SEQUENCE_NUMBER`), `GetRecords` |
| EFO | `RegisterStreamConsumer`, `DescribeStreamConsumer`, `ListStreamConsumers`, `DeregisterStreamConsumer`; `SubscribeToShard` lab path (JSON long-poll body with `SubscribeToShardEvent`, not HTTP/2 event stream push) |
| Scale | `UpdateShardCount` with `ScalingType=UNIFORM_SCALING` and `TargetShardCount` **1..4**; `DescribeStream` returns updated shard ids and hash-key ranges |
| Lambda ESM | Stream ARN as `CreateEventSourceMapping` source; in-process poller walks every shard with sequential `GetRecords` (merged under `BatchSize`); optional `FilterCriteria` (see [lambda.md](lambda.md)) |
| Resource policy | Stream `PutResourcePolicy` / `GetResourcePolicy` / `DeleteResourcePolicy`. EventBridge RoleArn-less targets require Allow for `events.amazonaws.com` on `kinesis:PutRecord` (empty policy skips delivery) |

`CreateStream` honors `ShardCount` **1..4** (`InvalidArgumentException` above 4). Partition keys map to shards with MD5 (AWS-shaped); same key always lands on the same shard for the current shard count. Shard ids are `shardId-000000000000` .. `shardId-000000000003`. Existing single-shard streams and records without a stored shard id stay on `shardId-000000000000`. Stream ARN: `arn:aws:kinesis:REGION:ACCOUNT:stream/NAME`. Consumer ARN: `arn:aws:kinesis:REGION:ACCOUNT:stream/NAME/consumer/CNAME:TIMESTAMP`.

`DescribeStream` returns one `Shards[]` entry per lab shard with equal hash-key ranges. Lab soft cap is five consumers per stream. `SubscribeToShard` returns a single JSON payload (not a durable HTTP/2 push stream); use `GetRecords` or re-subscribe for further reads. `UpdateShardCount` changes metadata and hash ranges immediately; previously written records keep their stored `shard_id` and are not remapped.

### Authz notes

Identity `EvaluateFull` on `kinesis:*` against the stream ARN (or consumer ARN for consumer APIs; `*` for ListStreams and GetRecords). Org SCP/RCP filters apply. Stream resource policy gates service-principal delivery (EventBridge); empty policy denies.

## How to verify / CLI smoke

Shared Compose and env setup: [index.md](index.md#shared-verification).

```bash
STREAM="noctaxris-kinesis-$RANDOM"

aws kinesis create-stream --stream-name "$STREAM" --shard-count 2 --endpoint-url "$EP"
aws kinesis describe-stream --stream-name "$STREAM" --endpoint-url "$EP"

aws kinesis put-record \
  --stream-name "$STREAM" \
  --partition-key pk1 \
  --data "hello" \
  --endpoint-url "$EP"

IT=$(aws kinesis get-shard-iterator \
  --stream-name "$STREAM" \
  --shard-id shardId-000000000000 \
  --shard-iterator-type TRIM_HORIZON \
  --endpoint-url "$EP" \
  --query ShardIterator --output text)

aws kinesis get-records --shard-iterator "$IT" --endpoint-url "$EP"

STREAM_ARN=$(aws kinesis describe-stream --stream-name "$STREAM" --endpoint-url "$EP" --query StreamDescription.StreamARN --output text)
aws kinesis register-stream-consumer --stream-arn "$STREAM_ARN" --consumer-name lab-app --endpoint-url "$EP"
aws kinesis update-shard-count --stream-name "$STREAM" --target-shard-count 3 --scaling-type UNIFORM_SCALING --endpoint-url "$EP"
aws kinesis describe-stream --stream-name "$STREAM" --endpoint-url "$EP"

aws kinesis delete-stream --stream-name "$STREAM" --endpoint-url "$EP"
```

## Not yet / deferred

- HTTP/2 `SubscribeToShard` event-stream framing (lab returns one JSON long-poll body)
- `SplitShard` / `MergeShards` as separate APIs (`UpdateShardCount` is the lab scale path)
- On-Demand capacity mode billing fantasy
- Server-side encryption depth beyond a cheap flag
- Kinesis Data Analytics (Firehose is a separate lab service: [firehose.md](firehose.md))

## Out of lab scope

- `ParallelizationFactor` on Lambda ESM (sequential multi-shard ESM remains the ESM path)
- Remapping historical records across shards after `UpdateShardCount`
