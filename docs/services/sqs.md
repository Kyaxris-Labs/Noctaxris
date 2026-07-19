# SQS

**Status:** shipped

Lab-complete standard and FIFO queues: send/receive/delete (including batch and visibility), content-based and explicit deduplication, queue policies via attributes, SSE-SQS, SSE-KMS, and RedrivePolicy move-to-DLQ.

## Implemented

| Area | Actions |
|------|---------|
| Queues | `CreateQueue`, `GetQueueUrl`, `GetQueueAttributes`, `SetQueueAttributes`, `DeleteQueue`, `ListQueues`, `PurgeQueue` |
| FIFO | `FifoQueue` attribute, `.fifo` name suffix, `MessageGroupId`, content-based or explicit deduplication |
| Messages | `SendMessage`, `ReceiveMessage`, `DeleteMessage` |
| Batch / visibility | `SendMessageBatch`, `DeleteMessageBatch`, `ChangeMessageVisibility` |
| Redrive | `RedrivePolicy` moves messages to a dead-letter queue after `maxReceiveCount` |
| Policy | Queue policy via attributes |
| Encryption | SSE-SQS and SSE-KMS when queue attributes request encryption |

Queue and message metadata live in SQLite. Message bodies use SSE-SQS or SSE-KMS when configured.

### Authz notes

SQS uses `EvaluateSQS`: allow if identity **or** queue policy Allows. Explicit Deny in either wins. A queue policy alone can grant access (unlike KMS). Org SCP/RCP filters apply before the union. When identity Allows, permissions boundary and session intersect.

Cross-account queue policy depth beyond same-account lab paths is deferred.

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

## Not yet / deferred

- Full SQS SAR beyond the lab set (delay queue depth, DLQ redrive allow policies, high-throughput FIFO quotas, tags beyond basics)
- Cross-account queue policy depth beyond same-account lab paths
