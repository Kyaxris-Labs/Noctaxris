# EventBridge Pipes

**Status:** shipped (lab core)

Pipe CRUD with SQS, DynamoDB Streams, or EventBridge bus sources and Lambda or SQS targets. Optional Lambda `Enrichment` ARN runs sync before target delivery (ticker uses nested Invoke). An in-process ticker polls RUNNING pipes via `PollPipeOnce`. Identity authz plus PassRole when `RoleArn` is set (`pipes.amazonaws.com`). Delivery uses a RoleArn session or requires the target resource policy to Allow `pipes.amazonaws.com`.

## Implemented

| Area | Actions |
|------|---------|
| CRUD | `CreatePipe`, `DescribePipe`, `DeletePipe`, `ListPipes` |
| Sources | SQS queue ARN, DynamoDB Streams ARN, EventBridge bus ARN (cursor over `event_entries`) |
| Targets | SQS queue ARN, Lambda function ARN |
| Enrichment | Optional Lambda ARN; sync invoke result becomes the payload forwarded to the target (empty result keeps the original body) |
| Delivery | Continuous ticker + `PollPipeOnce` receive/get, deliver, delete SQS messages on success. RoleArn session EvaluateFull, or target resource policy Allow for `pipes.amazonaws.com` |

### Authz notes

Identity `EvaluateFull` on `pipes:*`. PassRole requires trust for `pipes.amazonaws.com` when `RoleArn` is present. At poll/delivery time the lab mints a role session when `RoleArn` is set; without `RoleArn`, the SQS or Lambda target policy must Allow `pipes.amazonaws.com` (or account root).

## How to verify / CLI smoke

Shared Compose and env setup: [index.md](index.md#shared-verification).

```bash
SRC="noctaxris-pipe-src-$RANDOM"
DST="noctaxris-pipe-dst-$RANDOM"
SRC_URL=$(aws sqs create-queue --queue-name "$SRC" --endpoint-url "$EP" --query QueueUrl --output text)
DST_URL=$(aws sqs create-queue --queue-name "$DST" --endpoint-url "$EP" --query QueueUrl --output text)
SRC_ARN=$(aws sqs get-queue-attributes --queue-url "$SRC_URL" \
  --attribute-names QueueArn --endpoint-url "$EP" --query Attributes.QueueArn --output text)
DST_ARN=$(aws sqs get-queue-attributes --queue-url "$DST_URL" \
  --attribute-names QueueArn --endpoint-url "$EP" --query Attributes.QueueArn --output text)

# Destination policy required when RoleArn is omitted:
aws sqs set-queue-attributes --queue-url "$DST_URL" --attributes "{\"Policy\":\"{\\\"Version\\\":\\\"2012-10-17\\\",\\\"Statement\\\":[{\\\"Effect\\\":\\\"Allow\\\",\\\"Principal\\\":{\\\"Service\\\":\\\"pipes.amazonaws.com\\\"},\\\"Action\\\":\\\"sqs:SendMessage\\\",\\\"Resource\\\":\\\"$DST_ARN\\\"}]}\"}" --endpoint-url "$EP"

aws pipes create-pipe --name "lab-pipe" --source "$SRC_ARN" --target "$DST_ARN" --endpoint-url "$EP"
aws pipes list-pipes --endpoint-url "$EP"
aws pipes describe-pipe --name "lab-pipe" --endpoint-url "$EP"
aws pipes delete-pipe --name "lab-pipe" --endpoint-url "$EP"
```

Use the `QueueUrl` returned by `create-queue` (Noctaxris path-style URL). Do not invent Localstack-style hostnames.

Skip live Compose smoke when Docker is unavailable.

## Not yet / deferred

- Filter partner matrix and enrichment HTTP/API destinations
- Cross-account bus sources
