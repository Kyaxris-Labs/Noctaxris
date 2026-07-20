# Kinesis Data Streams

**Status:** shipped (lab core)

Lab streams with a single shard, Put/Get records, and shard iterators. Persisted in SQLite. Identity authz only.

## Implemented

| Area | Actions |
|------|---------|
| Stream CRUD | `CreateStream`, `DeleteStream`, `DescribeStream`, `ListStreams` |
| Produce | `PutRecord`, `PutRecords` |
| Consume | `GetShardIterator` (`TRIM_HORIZON`, `LATEST`, `AT_SEQUENCE_NUMBER`, `AFTER_SEQUENCE_NUMBER`), `GetRecords` |

Lab always uses shard id `shardId-000000000000` regardless of requested `ShardCount`. Stream ARN: `arn:aws:kinesis:REGION:ACCOUNT:stream/NAME`.

### Authz notes

Identity `EvaluateFull` on `kinesis:*` against the stream ARN (or `*` for ListStreams and GetRecords). Org SCP/RCP filters apply. No stream resource policy path.

## How to verify / CLI smoke

Shared Compose and env setup: [index.md](index.md#shared-verification).

```bash
STREAM="noctaxris-kinesis-$RANDOM"

aws kinesis create-stream --stream-name "$STREAM" --shard-count 1 --endpoint-url "$EP"
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

aws kinesis delete-stream --stream-name "$STREAM" --endpoint-url "$EP"
```

## Not yet / deferred

- Multi-shard split/merge, enhanced fan-out, consumers
- On-Demand capacity mode billing fantasy
- Server-side encryption depth beyond a cheap flag
- Kinesis Data Analytics (Firehose is a separate lab service: [firehose.md](firehose.md))
