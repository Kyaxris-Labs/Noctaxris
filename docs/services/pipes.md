# EventBridge Pipes

**Status:** shipped (lab core)

Pipe CRUD with SQS, DynamoDB Streams, or EventBridge bus sources and Lambda or SQS targets. Optional Lambda `Enrichment` ARN runs sync before target delivery (ticker uses nested Invoke). An in-process ticker polls RUNNING pipes via `PollPipeOnce`. Identity authz plus PassRole when `RoleArn` is set (`pipes.amazonaws.com`). Source poll requires a RoleArn session Allow on source actions, or (SQS) a source queue policy Allow for `pipes.amazonaws.com`. EventBridge bus sources require RoleArn session Allow on `events:PutEvents` (lab stand-in for retrieve). Enrichment requires RoleArn session `lambda:InvokeFunction`. Target delivery uses a RoleArn session or the target resource policy Allow for `pipes.amazonaws.com`; foreign targets require both. SQS source/target ARNs may be cross-account (queue owner account).

## Implemented

| Area | Actions |
|------|---------|
| CRUD | `CreatePipe`, `DescribePipe`, `DeletePipe`, `ListPipes` |
| Sources | SQS queue ARN, DynamoDB Streams ARN, EventBridge bus ARN (cursor over `event_entries`) |
| Targets | SQS queue ARN, Lambda function ARN |
| Enrichment | Optional Lambda ARN; sync invoke result becomes the payload forwarded to the target (empty result keeps the original body) |
| DLQ | Optional `DeadLetterArn` (SQS) on create for enrichment or target delivery failures |
| Delivery | Continuous ticker + `PollPipeOnce` receive/get, deliver, delete SQS messages on success. Source + enrichment RoleArn (or SQS source policy); target RoleArn session or target resource policy Allow for `pipes.amazonaws.com` |

### Authz notes

Identity `EvaluateFull` on `pipes:*`. PassRole requires trust for `pipes.amazonaws.com` when `RoleArn` is present; configure-time PassRole sets trust `aws:SourceArn` to the pipe ARN. At poll time source actions require RoleArn session Allow (DynamoDB/EventBridge sources require RoleArn with a session Allow; SQS may use a source queue policy Allow for `pipes.amazonaws.com` instead). Enrichment Invoke always requires RoleArn. Target delivery mints a role session when `RoleArn` is set; without `RoleArn`, the SQS or Lambda target policy must Allow `pipes.amazonaws.com` (or account root). Foreign targets with RoleArn also require the destination resource policy.

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

# Source + destination policies required when RoleArn is omitted:
aws sqs set-queue-attributes --queue-url "$SRC_URL" --attributes "{\"Policy\":\"{\\\"Version\\\":\\\"2012-10-17\\\",\\\"Statement\\\":[{\\\"Effect\\\":\\\"Allow\\\",\\\"Principal\\\":{\\\"Service\\\":\\\"pipes.amazonaws.com\\\"},\\\"Action\\\":[\\\"sqs:ReceiveMessage\\\",\\\"sqs:DeleteMessage\\\"],\\\"Resource\\\":\\\"$SRC_ARN\\\"}]}\"}" --endpoint-url "$EP"
aws sqs set-queue-attributes --queue-url "$DST_URL" --attributes "{\"Policy\":\"{\\\"Version\\\":\\\"2012-10-17\\\",\\\"Statement\\\":[{\\\"Effect\\\":\\\"Allow\\\",\\\"Principal\\\":{\\\"Service\\\":\\\"pipes.amazonaws.com\\\"},\\\"Action\\\":\\\"sqs:SendMessage\\\",\\\"Resource\\\":\\\"$DST_ARN\\\"}]}\"}" --endpoint-url "$EP"

aws pipes create-pipe --name "lab-pipe" --source "$SRC_ARN" --target "$DST_ARN" --endpoint-url "$EP"
aws pipes list-pipes --endpoint-url "$EP"
aws pipes describe-pipe --name "lab-pipe" --endpoint-url "$EP"
aws pipes delete-pipe --name "lab-pipe" --endpoint-url "$EP"
```

Use the `QueueUrl` returned by `create-queue` (Noctaxris path-style URL). Do not invent Localstack-style hostnames.

Skip live Compose smoke when Docker is unavailable.

## Out of lab scope

- Filter partner matrix and enrichment HTTP/API destinations (out of lab scope; SQS / DynamoDB Streams / bus → Lambda/SQS core is shipped)
- Cross-account bus-source depth beyond RoleArn session Allow on `events:PutEvents` (out of lab scope)
