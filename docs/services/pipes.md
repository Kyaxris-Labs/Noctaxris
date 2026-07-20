# EventBridge Pipes

**Status:** shipped (lab core)

Pipe CRUD with SQS or DynamoDB Streams sources and Lambda or SQS targets. In-process `PollPipeOnce` drains one batch (tests and operators call it; no separate enrichment matrix). Identity authz plus PassRole when `RoleArn` is set (`pipes.amazonaws.com`).

## Implemented

| Area | Actions |
|------|---------|
| CRUD | `CreatePipe`, `DescribePipe`, `DeletePipe`, `ListPipes` |
| Sources | SQS queue ARN, DynamoDB Streams ARN |
| Targets | SQS queue ARN, Lambda function ARN |
| Delivery | `PollPipeOnce` receive/get, deliver, delete SQS messages on success |

### Authz notes

Identity `EvaluateFull` on `pipes:*`. PassRole requires trust for `pipes.amazonaws.com` when `RoleArn` is present.

## How to verify / CLI smoke

Shared Compose and env setup: [index.md](index.md#shared-verification).

```bash
SRC="noctaxris-pipe-src-$RANDOM"
DST="noctaxris-pipe-dst-$RANDOM"
aws sqs create-queue --queue-name "$SRC" --endpoint-url "$EP"
aws sqs create-queue --queue-name "$DST" --endpoint-url "$EP"
SRC_ARN=$(aws sqs get-queue-attributes --queue-url "http://sqs.us-east-1.localhost.localstack.cloud:4566/000000000001/$SRC" \
  --attribute-names QueueArn --endpoint-url "$EP" --query Attributes.QueueArn --output text)
DST_ARN=$(aws sqs get-queue-attributes --queue-url "http://sqs.us-east-1.localhost.localstack.cloud:4566/000000000001/$DST" \
  --attribute-names QueueArn --endpoint-url "$EP" --query Attributes.QueueArn --output text)

aws pipes create-pipe --name "lab-pipe" --source "$SRC_ARN" --target "$DST_ARN" --endpoint-url "$EP"
aws pipes list-pipes --endpoint-url "$EP"
aws pipes describe-pipe --name "lab-pipe" --endpoint-url "$EP"
aws pipes delete-pipe --name "lab-pipe" --endpoint-url "$EP"
```

Lab delivery is exercised by unit tests via `PollPipeOnce`. Skip live Compose smoke when Docker is unavailable.

## Not yet / deferred

- Enrichment and filter partner matrix
- EventBridge bus as a source
- Continuous in-process ticker (call `PollPipeOnce` or wire a worker later)
