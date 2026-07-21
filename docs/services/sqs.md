# SQS

**Status:** shipped

Lab-complete standard and FIFO queues: send/receive/delete (including batch and visibility), content-based and explicit deduplication, queue policies via attributes, SSE-SQS, SSE-KMS, and RedrivePolicy move-to-DLQ.

## Implemented

| Area | Actions |
|------|---------|
| Queues | `CreateQueue`, `GetQueueUrl`, `GetQueueAttributes`, `SetQueueAttributes`, `DeleteQueue`, `ListQueues`, `PurgeQueue` |
| FIFO | `FifoQueue` attribute, `.fifo` name suffix, `MessageGroupId`, content-based or explicit deduplication |
| Messages | `SendMessage`, `ReceiveMessage` (honors `WaitTimeSeconds` 0–20), `DeleteMessage` |
| Batch / visibility | `SendMessageBatch`, `DeleteMessageBatch`, `ChangeMessageVisibility` |
| Redrive | `RedrivePolicy` moves messages to a dead-letter queue after `maxReceiveCount`. DLQ `RedriveAllowPolicy` (`allowAll`, `denyAll`, `byQueue` + `sourceQueueArns`) is enforced on redrive |
| Delay | Queue `DelaySeconds` and per-message `DelaySeconds` (up to 900) set `visible_after` before receive |
| Policy | Queue policy via attributes |
| Encryption | SSE-SQS and SSE-KMS when queue attributes request encryption |

Queue and message metadata live in SQLite. Message bodies use SSE-SQS or SSE-KMS when configured.

### Authz notes

SQS uses `EvaluateSQS` with the queue owner account from store metadata (or queue ARN account). Same-account access: allow if identity **or** queue policy Allows. Cross-account access: allow only when identity **and** queue policy both Allow (empty queue policy denies cross-account callers). Explicit Deny in either wins. Org SCP/RCP filters apply before evaluation. When identity Allows, permissions boundary and session intersect.

## How to verify / CLI smoke

Shared Compose and env setup: [index.md](index.md#shared-verification).

```bash
QUEUE="noctaxris-lab-$RANDOM"
QUEUE_URL=$(aws sqs create-queue --queue-name "$QUEUE" --endpoint-url "$EP" --query QueueUrl --output text)

aws sqs send-message \
  --queue-url "$QUEUE_URL" \
  --message-body hello-sqs \
  --endpoint-url "$EP"

MSG_JSON=$(aws sqs receive-message --queue-url "$QUEUE_URL" --endpoint-url "$EP" --output json)
HANDLE=$(echo "$MSG_JSON" | python3 -c 'import sys,json; print(json.load(sys.stdin)["Messages"][0]["ReceiptHandle"])')

aws sqs delete-message \
  --queue-url "$QUEUE_URL" \
  --receipt-handle "$HANDLE" \
  --endpoint-url "$EP"
```

FIFO plus RedrivePolicy:

```bash
FIFO="noctaxris-lab-$RANDOM.fifo"
FIFO_URL=$(aws sqs create-queue \
  --queue-name "$FIFO" \
  --attributes FifoQueue=true,ContentBasedDeduplication=true \
  --endpoint-url "$EP" --query QueueUrl --output text)

aws sqs send-message \
  --queue-url "$FIFO_URL" \
  --message-body hello-fifo \
  --message-group-id g1 \
  --endpoint-url "$EP"
```

Two-account cross-account send (member account B owns the queue, member account A user sends via dual eval):

```bash
QUEUE_URL=$(aws sqs create-queue --queue-name "$QUEUE" --endpoint-url "$EP" --profile account-b --query QueueUrl --output text)
QUEUE_ARN=$(aws sqs get-queue-attributes --queue-url "$QUEUE_URL" --attribute-names QueueArn \
  --endpoint-url "$EP" --profile account-b --query Attributes.QueueArn --output text)
aws sqs set-queue-attributes --queue-url "$QUEUE_URL" --endpoint-url "$EP" --profile account-b \
  --attributes Policy='{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":{"AWS":"arn:aws:iam::ACCOUNT_A:user/sender"},"Action":"sqs:SendMessage","Resource":"'"$QUEUE_ARN"'"}]}'
aws sqs send-message --queue-url "$QUEUE_URL" --message-body hello-xa --endpoint-url "$EP" --profile account-a
```

## Not yet / deferred

- Full SQS SAR beyond the lab set (high-throughput FIFO quotas, tags beyond basics, StartMessageMoveTask parity)
